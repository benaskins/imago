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

func newTestModel(t *testing.T, sess *face.Session) Model {
	t.Helper()
	return New(stubClient{}, config.OpenAIModelConfig(), nil, sess, t.TempDir(), "interview:self", mustInterviewAudience(t), "test system prompt")
}

func TestNewModel(t *testing.T) {
	m := newTestModel(t, nil)

	if m.phase != phaseInterview {
		t.Errorf("expected interview phase, got %d", m.phase)
	}
	if m.AgentName != "imago" {
		t.Errorf("expected agent name 'imago', got %q", m.AgentName)
	}
	if len(m.Messages) != 1 || m.Messages[0].Role != loop.RoleSystem {
		t.Error("expected system message in Messages")
	}
	if m.sessionKind != "interview:self" {
		t.Errorf("expected sessionKind 'interview:self', got %q", m.sessionKind)
	}
}

func TestNewModelResumeSession(t *testing.T) {
	sess := face.NewSession()
	sess.Messages = []loop.Message{
		{Role: loop.RoleSystem, Content: "system prompt"},
		{Role: loop.RoleUser, Content: "hello"},
		{Role: loop.RoleAssistant, Content: "hi there"},
	}

	m := newTestModel(t, sess)

	if len(m.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(m.Messages))
	}
	if len(m.Entries) != 2 {
		t.Errorf("expected 2 entries (user+agent), got %d", len(m.Entries))
	}
}

func TestViewInterview(t *testing.T) {
	m := newTestModel(t, nil)

	// Initialize viewport with a window size
	resize := tea.WindowSizeMsg{Width: 80, Height: 24}
	m.Chat.HandleResize(resize)

	v := m.View()
	if v == "" {
		t.Error("view should not be empty after resize")
	}
}

func TestNew_WeeklyAudience(t *testing.T) {
	weeklyAud := mustAudience(t, "self", "weekly")
	m := New(stubClient{}, config.OpenAIModelConfig(), nil, nil, t.TempDir(), "weekly:self", weeklyAud, "weekly system prompt")
	m.WithPeriodOutput(t.TempDir())

	if m.sessionKind != "weekly:self" {
		t.Errorf("expected weekly:self kind, got %q", m.sessionKind)
	}
	expected, _ := weeklyAud.Draft.Render(config.PromptData{})
	if m.draftPrompt != expected {
		t.Error("expected weekly draft prompt rendered from weekly audience")
	}
	if m.Messages[0].Content != "weekly system prompt" {
		t.Error("expected weekly system prompt")
	}
	if m.periodOutputDir == "" {
		t.Error("expected periodOutputDir to be set")
	}
}

func TestNew_DailyAudience(t *testing.T) {
	dailyAud := mustAudience(t, "self", "daily")
	m := New(stubClient{}, config.OpenAIModelConfig(), nil, nil, t.TempDir(), "daily:self", dailyAud, "daily system prompt")
	m.WithPeriodOutput(t.TempDir())

	if m.sessionKind != "daily:self" {
		t.Errorf("expected daily:self kind, got %q", m.sessionKind)
	}
	expected, _ := dailyAud.Draft.Render(config.PromptData{})
	if m.draftPrompt != expected {
		t.Error("expected daily draft prompt rendered from daily audience")
	}
	if m.Messages[0].Content != "daily system prompt" {
		t.Error("expected daily system prompt")
	}
}

func mustInterviewAudience(t *testing.T) *config.AudienceTemplates {
	t.Helper()
	return mustAudience(t, "self", "interview")
}

func mustAudience(t *testing.T, audience, mode string) *config.AudienceTemplates {
	t.Helper()
	a, err := config.LoadAudience(audience, mode)
	if err != nil {
		t.Fatalf("LoadAudience(%q, %q): %v", audience, mode, err)
	}
	return a
}

func TestPhaseSwitch(t *testing.T) {
	m := newTestModel(t, nil)
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
	m := newTestModel(t, nil)
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
	m := newTestModel(t, nil)
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
