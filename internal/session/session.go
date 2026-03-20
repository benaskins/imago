// Package session provides conversation persistence for imago.
// Sessions are saved as JSON files after every turn so work is
// never lost. On startup, an incomplete session can be resumed.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	loop "github.com/benaskins/axon-loop"
)

const sessionDir = ".local/share/imago/sessions"

// State represents a persisted session.
type State struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`  // "post" or "weekly"
	Phase      string         `json:"phase"` // "interview" or "draft"
	Messages   []loop.Message `json:"messages"`
	Sections   []string       `json:"sections,omitempty"`
	Approved   []bool         `json:"approved,omitempty"`
	Collection string         `json:"collection,omitempty"` // raw collection report (weekly only)
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Complete   bool           `json:"complete"`
}

func dir() string {
	return filepath.Join(os.Getenv("HOME"), sessionDir)
}

func pathFor(id string) string {
	return filepath.Join(dir(), id+".json")
}

// New creates a new session with a timestamped ID.
func New(kind string) *State {
	if kind == "" {
		kind = "post"
	}
	now := time.Now()
	return &State{
		ID:        now.Format("2006-01-02T15-04-05"),
		Kind:      kind,
		Phase:     "interview",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Save writes the session to disk.
func (s *State) Save() error {
	s.UpdatedAt = time.Now()

	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := os.WriteFile(pathFor(s.ID), data, 0o644); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	return nil
}

// MarkComplete marks the session as finished.
func (s *State) MarkComplete() error {
	s.Complete = true
	return s.Save()
}

// FindIncomplete returns the most recent incomplete session matching
// the given kind, or nil. Pass "" to match any kind.
func FindIncomplete(kind string) *State {
	entries, err := os.ReadDir(dir())
	if err != nil {
		return nil
	}

	// Sort by name descending (newest first since names are timestamps).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir(), e.Name()))
		if err != nil {
			continue
		}

		var s State
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		if !s.Complete && (kind == "" || s.Kind == kind) {
			return &s
		}
	}

	return nil
}
