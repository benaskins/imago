package tui

import (
	"strings"
	"testing"

	loop "github.com/benaskins/axon-loop"

	"github.com/benaskins/imago/internal/config"
)

func TestAssembleDraft(t *testing.T) {
	sections := []string{
		"# Title\n\nIntro paragraph.",
		"## Section One\n\nContent one.",
		"## Section Two\n\nContent two.",
	}

	got := assembleDraft(sections)
	if !strings.Contains(got, "# Title") {
		t.Error("missing title")
	}
	if !strings.Contains(got, "## Section One") {
		t.Error("missing section one")
	}
	if !strings.Contains(got, "## Section Two") {
		t.Error("missing section two")
	}
	// Sections should be separated by double newline
	if !strings.Contains(got, ".\n\n## Section One") {
		t.Error("sections not properly joined")
	}
}

func TestInterviewTranscript(t *testing.T) {
	m := Model{
		messages: []loop.Message{
			{Role: "system", Content: config.SystemPrompt},
			{Role: "assistant", Content: "What do you want to write about?"},
			{Role: "user", Content: "Building local AI tools."},
			{Role: "assistant", Content: "Tell me more."},
			{Role: "user", Content: "It's about composability."},
			{Role: "user", Content: config.DraftPrompt}, // should be excluded
		},
	}

	transcript := m.interviewTranscript()

	if strings.Contains(transcript, config.SystemPrompt) {
		t.Error("transcript should not contain system prompt")
	}
	if strings.Contains(transcript, config.DraftPrompt) {
		t.Error("transcript should not contain draft prompt")
	}
	if !strings.Contains(transcript, "Building local AI tools.") {
		t.Error("transcript should contain user messages")
	}
	if !strings.Contains(transcript, "What do you want to write about?") {
		t.Error("transcript should contain assistant messages")
	}
	if !strings.Contains(transcript, "**Author:**") {
		t.Error("transcript should label user as Author")
	}
	if !strings.Contains(transcript, "**Interviewer:**") {
		t.Error("transcript should label assistant as Interviewer")
	}
}

func TestRevisionPreview_Short(t *testing.T) {
	content := "A short response."
	got := revisionPreview(content)
	if got != content {
		t.Errorf("short content should pass through, got: %q", got)
	}
}

func TestRevisionPreview_Long(t *testing.T) {
	content := "Line one\nLine two\nLine three\nLine four\nLine five"
	got := revisionPreview(content)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("long content should be truncated, got: %q", got)
	}
	if strings.Contains(got, "Line four") {
		t.Error("preview should not contain later lines")
	}
}
