package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/benaskins/axon-talk/ollama"

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

	client, err := ollama.NewClientFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to ollama: %v\n", err)
		os.Exit(1)
	}

	// Load tool config from environment
	syndToken := ""
	if data, err := os.ReadFile(os.ExpandEnv("$HOME/.config/synd/token")); err == nil {
		syndToken = strings.TrimSpace(string(data))
	}

	cfg := tools.Config{
		SiteDir:    envOrDefault("SYND_SITE_DIR", "/Users/benaskins/dev/sites/generativeplane.com"),
		SyndURL:    envOrDefault("SYND_SERVICE_URL", ""),
		SyndToken:  syndToken,
		MemoURL:    envOrDefault("MEMO_SERVICE_URL", ""),
		SearXNGURL: envOrDefault("SEARXNG_URL", ""),
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
