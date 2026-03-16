package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	loop "github.com/benaskins/axon-loop"
	cf "github.com/benaskins/axon-talk/cloudflare"
	"github.com/benaskins/axon-talk/ollama"
	"github.com/benaskins/axon-wire"

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

	// Check for incomplete session
	var sess *session.State
	if prev := session.FindIncomplete(); prev != nil {
		fmt.Printf("Found incomplete session from %s. Resume? (y/n) ", prev.UpdatedAt.Format("Jan 2 15:04"))
		var answer string
		fmt.Scanln(&answer)
		if answer == "y" || answer == "yes" {
			sess = prev
		}
	}

	model := tui.New(client, allTools, sess)

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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
