package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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

	// Determine mode from subcommand.
	period := ""
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "weekly":
			period = "weekly"
		case "daily":
			period = "daily"
		}
	}

	// Period modes require a workspace path argument.
	var workspacePath string
	if period != "" {
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: imago %s <workspace-path>\n", period)
			os.Exit(2)
		}
		workspacePath = os.Args[2]
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

	// Check for incomplete session (filtered by kind).
	sessionDir := home + "/.local/share/imago/sessions"
	sessionKind := "post"
	if period != "" {
		sessionKind = period
	}
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

	model := tui.New(client, mcfg, allTools, sess, sessionDir)

	if period != "" {
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

		outputDir := collect.PeriodDir(workspacePath, period)
		previous := collect.PreviousPost(outputDir, period)
		workspaceName := filepath.Base(workspacePath)

		switch period {
		case "weekly":
			systemPrompt := config.WeeklySystemPrompt(workspaceName, workspacePath, report.Markdown, previous)
			model.WithWeeklyMode(systemPrompt, outputDir)
		case "daily":
			systemPrompt := config.DailySystemPrompt(workspaceName, workspacePath, report.Markdown, previous)
			model.WithDailyMode(systemPrompt, outputDir)
		}

		slog.Info(period+" mode", "model", mcfg.InterviewModel)
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
