// Package logging provides file-based session logging for imago.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Setup creates a session log file and configures slog to write to it.
// Returns a cleanup function that closes the file.
func Setup() (func(), error) {
	dir := os.ExpandEnv("$HOME/.local/share/imago/logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	name := time.Now().Format("2006-01-02T15-04-05") + ".log"
	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))

	slog.Info("session started", "log_file", path)

	return func() {
		slog.Info("session ended")
		f.Close()
	}, nil
}
