package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	loop "github.com/benaskins/axon-loop"
	"github.com/benaskins/axon-talk/anthropic"
	cf "github.com/benaskins/axon-talk/cloudflare"
	"github.com/benaskins/axon-talk/ollama"
	"github.com/benaskins/axon-wire"

	"github.com/benaskins/imago/internal/collect"
	"github.com/benaskins/imago/internal/config"
	"github.com/benaskins/imago/internal/logging"
	"github.com/benaskins/imago/internal/session"
	"github.com/benaskins/imago/internal/tui"
	"github.com/benaskins/imago/tools"
)

func main() {
	cleanup, err := logging.Setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging setup failed: %v\n", err)
	} else {
		defer cleanup()
	}

	// Determine mode from subcommand.
	weekly := len(os.Args) > 1 && os.Args[1] == "weekly"

	// Select LLM client: Cloudflare if configured, otherwise Ollama.
	client := selectLLMClient()

	// Build HTTP client — routes through wire proxy if AXON_WIRE_URL is set.
	httpClient := wire.NewClient()

	// Load tool config from environment
	syndToken := ""
	if data, err := os.ReadFile(os.ExpandEnv("$HOME/.config/synd/token")); err == nil {
		syndToken = strings.TrimSpace(string(data))
	}

	cfg := tools.Config{
		SiteDir:     envOrDefault("SYND_SITE_DIR", ""),
		SyndURL:     envOrDefault("SYND_SERVICE_URL", ""),
		SyndToken:   syndToken,
		MemoURL:     envOrDefault("MEMO_SERVICE_URL", ""),
		SearXNGURL:  envOrDefault("SEARXNG_URL", ""),
		DispatchURL: envOrDefault("AXON_DISPATCH_URL", ""),
		WireToken:   envOrDefault("AXON_WIRE_TOKEN", ""),
		HTTPClient:  httpClient,
	}

	allTools := tools.All(cfg)

	// Check for incomplete session (filtered by kind).
	sessionKind := "post"
	if weekly {
		sessionKind = "weekly"
	}
	var sess *session.State
	if prev := session.FindIncomplete(sessionKind); prev != nil {
		fmt.Printf("Found incomplete %s session from %s. Resume? (y/n) ", sessionKind, prev.UpdatedAt.Format("Jan 2 15:04"))
		var answer string
		fmt.Scanln(&answer)
		if answer == "y" || answer == "yes" {
			sess = prev
		}
	}

	mcfg := config.DefaultModelConfig()
	slog.Info("model config", "provider", mcfg.Provider, "interview", mcfg.InterviewModel, "draft", mcfg.DraftModel)

	model := tui.New(client, mcfg, allTools, sess)

	if weekly {
		// Run collection pass.
		fmt.Println("Collecting activity data...")
		report, err := collect.Run(collect.Config{
			SiteDir: cfg.SiteDir,
			DevDir:  envOrDefault("DEV", os.ExpandEnv("$HOME/dev")),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d active repos since %s\n", len(report.Repos), report.Since.Format("Jan 2"))

		// Build weekly system prompt with collection data and previous post.
		previousWeekly := collect.PreviousWeekly(cfg.SiteDir)
		systemPrompt := config.WeeklySystemPrompt(report.Markdown, previousWeekly)
		model.WithWeeklyMode(systemPrompt)

		// Set up Claude Opus as the draft client via Cloudflare AI Gateway.
		if draftClient := selectAnthropicClient(); draftClient != nil {
			model.WithDraftClient(draftClient)
			mcfg.DraftModel = envOrDefault("IMAGO_DRAFT_MODEL", "claude-opus-4-6")
			mcfg.DraftProvider = config.ProviderAnthropic
			mcfg.DraftOptions = map[string]any{"max_tokens": 4096}
			slog.Info("weekly mode", "draft_provider", "anthropic", "draft_model", mcfg.DraftModel)
		} else {
			slog.Warn("ANTHROPIC_API_KEY not set — weekly draft will use interview model")
		}
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
// env vars are set, otherwise falls back to local Ollama.
func selectLLMClient() loop.LLMClient {
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	token := os.Getenv("CLOUDFLARE_AXON_GATE_TOKEN")

	if accountID != "" && token != "" {
		baseURL := "https://gateway.ai.cloudflare.com/v1/" + accountID + "/axon-gate/workers-ai"
		slog.Info("using Cloudflare Workers AI", "gateway", "axon-gate")
		return cf.NewClient(baseURL, token)
	}

	client, err := ollama.NewClientFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to ollama: %v\n", err)
		os.Exit(1)
	}
	slog.Info("using Ollama for inference")
	return client
}

// selectAnthropicClient returns a Claude client via Cloudflare AI Gateway,
// or nil if the required env vars are not set.
func selectAnthropicClient() loop.LLMClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")

	if apiKey == "" {
		return nil
	}

	if accountID != "" {
		gateway := envOrDefault("CLOUDFLARE_GATEWAY", "axon-gate")
		baseURL := "https://gateway.ai.cloudflare.com/v1/" + accountID + "/" + gateway + "/anthropic"
		slog.Info("using Anthropic via Cloudflare AI Gateway", "gateway", gateway)
		var opts []anthropic.Option
		if gwToken := os.Getenv("CLOUDFLARE_AI_GATEWAY_TOKEN"); gwToken != "" {
			opts = append(opts, anthropic.WithGatewayToken(gwToken))
		}
		return anthropic.NewClient(baseURL, apiKey, opts...)
	}

	// Direct Anthropic API (no gateway).
	slog.Info("using Anthropic API directly")
	return anthropic.NewClient("https://api.anthropic.com", apiKey)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
