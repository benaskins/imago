package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	loop "github.com/benaskins/axon-loop"
	"github.com/benaskins/axon-talk/anthropic"
	"github.com/benaskins/axon-talk/openai"
	"github.com/benaskins/axon-wire"

	face "github.com/benaskins/axon-face"

	"github.com/benaskins/imago/internal/collect"
	"github.com/benaskins/imago/internal/config"
	"github.com/benaskins/imago/internal/tui"
	"github.com/benaskins/imago/tools"
)

func main() {
	home, _ := os.UserHomeDir()
	cleanup, err := face.SetupLogging(home + "/.local/share/imago/logs")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging setup failed: %v\n", err)
	} else {
		defer cleanup()
	}

	period, audience, workspacePath := parseArgs(os.Args[1:])
	mode := period
	if mode == "" {
		mode = "interview"
	}
	if period != "" {
		if err := collect.ValidateWorkspace(workspacePath); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
	}

	// Select LLM client.
	var client loop.LLMClient
	if period != "" {
		// Period modes (weekly/daily): use Opus for the entire flow.
		client = selectAnthropicClient()
		if client == nil {
			fmt.Fprintf(os.Stderr, "%s mode requires Anthropic credentials (CLOUDFLARE_AI_GATEWAY_TOKEN + CLOUDFLARE_ACCOUNT_ID, or ANTHROPIC_API_KEY)\n", period)
			os.Exit(1)
		}
	} else {
		// Regular mode: Cloudflare Workers AI or Ollama.
		client = selectLLMClient()
	}

	// Build HTTP client — routes through wire proxy if AXON_WIRE_URL is set.
	httpClient := wire.NewClient()

	cfg := tools.Config{
		MemoURL:     envOrDefault("MEMO_SERVICE_URL", ""),
		SearXNGURL:  envOrDefault("SEARXNG_URL", ""),
		DispatchURL: envOrDefault("AXON_DISPATCH_URL", ""),
		WireToken:   envOrDefault("AXON_WIRE_TOKEN", ""),
		HTTPClient:  httpClient,
	}

	allTools := tools.All(cfg)

	// Load the active mode's audience. Fail fast with the available
	// audiences for this mode if the user picked one we don't have.
	activeAudience, err := config.LoadAudience(audience, mode)
	if err != nil {
		available := config.AvailableAudiences(mode)
		fmt.Fprintf(os.Stderr, "audience %q is not available for mode %q. available: %s\n", audience, mode, strings.Join(available, ", "))
		os.Exit(2)
	}
	sessionKind := mode + ":" + audience

	// Check for incomplete session (filtered by kind).
	sessionDir := home + "/.local/share/imago/sessions"
	var sess *face.Session
	if prev := face.FindIncomplete(sessionDir); prev != nil {
		prevKind, _ := prev.State["kind"].(string)
		if prevKind == sessionKind {
			fmt.Printf("Found incomplete %s session from %s. Resume? (y/n) ", sessionKind, prev.UpdatedAt.Format("Jan 2 15:04"))
			var answer string
			fmt.Scanln(&answer)
			if answer == "y" || answer == "yes" {
				sess = prev
			}
		}
	}

	mcfg := config.DefaultModelConfig()

	if period != "" {
		// Period modes: Opus for everything.
		opusModel := envOrDefault("IMAGO_DRAFT_MODEL", "claude-opus-4-6")
		mcfg.Provider = config.ProviderAnthropic
		mcfg.DraftProvider = config.ProviderAnthropic
		mcfg.InterviewModel = opusModel
		mcfg.DraftModel = opusModel
		mcfg.InterviewOptions = map[string]any{"max_tokens": 4096}
		mcfg.DraftOptions = map[string]any{"max_tokens": 8192}
		mcfg.RevisionOptions = map[string]any{"max_tokens": 8192}
	}

	slog.Info("model config", "provider", mcfg.Provider, "interview", mcfg.InterviewModel, "draft", mcfg.DraftModel)

	// Render the initial system prompt and (for period modes) collect
	// the activity report.
	var systemPrompt, outputDir string
	if period == "" {
		systemPrompt, err = activeAudience.System.Render(config.PromptData{
			Date:          config.Today(),
			WorkspacePath: config.ResolveWorkspacePath(),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "render interview system prompt: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Collecting activity data...")
		report, err := collect.Run(collect.Config{
			WorkspacePath: workspacePath,
			Period:        period,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d active repos since %s\n", len(report.Repos), report.Since.Format("Jan 2"))

		outputDir = collect.AudienceDir(workspacePath, period, audience)
		previous := collect.PreviousPost(outputDir, period)
		workspaceName := filepath.Base(workspacePath)

		systemPrompt, err = activeAudience.System.Render(config.PromptData{
			Date:            config.Today(),
			WorkspaceName:   workspaceName,
			WorkspacePath:   workspacePath,
			ActivityReport:  report.Markdown,
			PreviousSection: config.PreviousPostSection(period, previous),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "render %s system prompt: %v\n", period, err)
			os.Exit(1)
		}
	}

	model := tui.New(client, mcfg, allTools, sess, sessionDir, sessionKind, activeAudience, systemPrompt)
	if outputDir != "" {
		model.WithPeriodOutput(outputDir)
	}
	if period != "" {
		slog.Info(period+" mode", "audience", audience, "model", mcfg.InterviewModel)
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// selectLLMClient returns a Cloudflare Workers AI client if the gateway
// env vars are set, otherwise falls back to a local OpenAI-compatible server (e.g. llama-server).
func selectLLMClient() loop.LLMClient {
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	token := os.Getenv("CLOUDFLARE_AXON_GATE_TOKEN")

	if accountID != "" && token != "" {
		baseURL := "https://gateway.ai.cloudflare.com/v1/" + accountID + "/axon-gate/workers-ai"
		slog.Info("using Cloudflare Workers AI", "gateway", "axon-gate")
		var opts []openai.Option
		if gwToken := os.Getenv("CLOUDFLARE_AI_GATEWAY_TOKEN"); gwToken != "" {
			opts = append(opts, openai.WithGatewayToken(gwToken))
		}
		return openai.NewClient(baseURL, token, opts...)
	}

	baseURL := envOrDefault("LLM_BASE_URL", "http://localhost:8080/v1")
	apiKey := os.Getenv("LLM_API_KEY")
	slog.Info("using OpenAI-compatible server", "base_url", baseURL)
	return openai.NewClient(baseURL, apiKey)
}

// selectAnthropicClient returns a Claude client via Cloudflare AI Gateway,
// or nil if the required env vars are not set. The Anthropic API key is
// optional — the gateway can hold it server-side.
func selectAnthropicClient() loop.LLMClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	gwToken := os.Getenv("CLOUDFLARE_AI_GATEWAY_TOKEN")

	if accountID != "" && gwToken != "" {
		gateway := envOrDefault("CLOUDFLARE_GATEWAY", "axon-gate")
		baseURL := "https://gateway.ai.cloudflare.com/v1/" + accountID + "/" + gateway + "/anthropic"
		slog.Info("using Anthropic via Cloudflare AI Gateway", "gateway", gateway)
		return anthropic.NewClient(baseURL, apiKey,
			anthropic.WithGatewayToken(gwToken))
	}

	if apiKey != "" {
		// Direct Anthropic API (no gateway).
		slog.Info("using Anthropic API directly")
		return anthropic.NewClient("https://api.anthropic.com", apiKey)
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseArgs parses the imago command line. Supported shapes:
//
//	imago [--audience NAME]
//	imago daily [--audience NAME] <workspace-path>
//	imago weekly [--audience NAME] <workspace-path>
//
// Returns the period ("" for interview mode), the chosen audience
// (default "self"), and the workspace path (empty for interview mode).
func parseArgs(args []string) (period, audience, workspacePath string) {
	audience = "self"
	switch {
	case len(args) > 0 && (args[0] == "daily" || args[0] == "weekly"):
		period = args[0]
		fs := flag.NewFlagSet(period, flag.ExitOnError)
		fs.StringVar(&audience, "audience", "self", "audience prompt set to use")
		fs.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: imago %s [--audience NAME] <workspace-path>\n", period)
			fs.PrintDefaults()
		}
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			fs.Usage()
			os.Exit(2)
		}
		workspacePath = fs.Arg(0)
	default:
		fs := flag.NewFlagSet("imago", flag.ExitOnError)
		fs.StringVar(&audience, "audience", "self", "audience prompt set to use")
		_ = fs.Parse(args)
	}
	return period, audience, workspacePath
}
