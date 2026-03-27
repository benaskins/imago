package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	face "github.com/benaskins/axon-face"
	loop "github.com/benaskins/axon-loop"

	"github.com/benaskins/imago/internal/config"
)

// stubClient is a no-op LLM client for testing.
type stubClient struct{}

func (c stubClient) Chat(_ context.Context, _ *loop.Request, fn func(loop.Response) error) error {
	return fn(loop.Response{Done: true})
}

func TestNewModel(t *testing.T) {
	m := New(stubClient{}, config.OllamaModelConfig(), nil, nil, t.TempDir())

	if m.phase != phaseInterview {
		t.Errorf("expected interview phase, got %d", m.phase)
	}
	if m.AgentName != "imago" {
		t.Errorf("expected agent name 'imago', got %q", m.AgentName)
	}
	if len(m.Messages) != 1 || m.Messages[0].Role != loop.RoleSystem {
		t.Error("expected system message in Messages")
	}
}

func TestNewModelResumeSession(t *testing.T) {
	sess := face.NewSession()
	sess.Messages = []loop.Message{
		{Role: loop.RoleSystem, Content: "system prompt"},
		{Role: loop.RoleUser, Content: "hello"},
		{Role: loop.RoleAssistant, Content: "hi there"},
	}

	m := New(stubClient{}, config.OllamaModelConfig(), nil, sess, t.TempDir())

	if len(m.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(m.Messages))
	}
	if len(m.Entries) != 2 {
		t.Errorf("expected 2 entries (user+agent), got %d", len(m.Entries))
	}
}

func TestViewInterview(t *testing.T) {
	m := New(stubClient{}, config.OllamaModelConfig(), nil, nil, t.TempDir())

	// Initialize viewport with a window size
	resize := tea.WindowSizeMsg{Width: 80, Height: 24}
	m.Chat.HandleResize(resize)

	v := m.View()
	if v == "" {
		t.Error("view should not be empty after resize")
	}
}

func TestWithWeeklyMode(t *testing.T) {
	m := New(stubClient{}, config.OllamaModelConfig(), nil, nil, t.TempDir())
	m.WithWeeklyMode("weekly system prompt")

	if m.sessionKind != "weekly" {
		t.Errorf("expected weekly kind, got %q", m.sessionKind)
	}
	if m.draftPrompt != config.WeeklyDraftPrompt {
		t.Error("expected weekly draft prompt")
	}
	if m.Messages[0].Content != "weekly system prompt" {
		t.Error("expected system prompt to be replaced")
	}
}

func TestPhaseSwitch(t *testing.T) {
	m := New(stubClient{}, config.OllamaModelConfig(), nil, nil, t.TempDir())
	resize := tea.WindowSizeMsg{Width: 80, Height: 24}
	m.Chat.HandleResize(resize)

	// Simulate phase switch message
	result, _ := m.Update(phaseSwitchMsg{})
	updated := result.(Model)

	if updated.phase != phaseDraft {
		t.Errorf("expected draft phase after switch, got %d", updated.phase)
	}
}

func TestShowCurrentSection(t *testing.T) {
	m := New(stubClient{}, config.OllamaModelConfig(), nil, nil, t.TempDir())
	resize := tea.WindowSizeMsg{Width: 80, Height: 24}
	m.Chat.HandleResize(resize)

	m.phase = phaseDraft
	m.sections = []string{"# Title\n\nIntro.", "## Section\n\nBody."}
	m.approved = []bool{false, false}
	m.sectionHistory = make([][]loop.Message, 2)
	m.revisionEntries = make([][]face.Entry, 2)
	m.sectionIndex = 0

	m.showCurrentSection()

	if len(m.Entries) == 0 {
		t.Error("expected entries after showCurrentSection")
	}
	// Check that section content appears in entries
	found := false
	for _, e := range m.Entries {
		if strings.Contains(e.Content, "# Title") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected section content in entries")
	}
}

func TestShowReview(t *testing.T) {
	m := New(stubClient{}, config.OllamaModelConfig(), nil, nil, t.TempDir())
	resize := tea.WindowSizeMsg{Width: 80, Height: 24}
	m.Chat.HandleResize(resize)

	m.phase = phaseReview
	m.finalMarkdown = "# Article\n\nContent here."

	m.showReview()

	if len(m.Entries) == 0 {
		t.Error("expected entries after showReview")
	}
	found := false
	for _, e := range m.Entries {
		if strings.Contains(e.Content, "# Article") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected article content in review entries")
	}
}
