package config

import (
	"strings"
	"testing"
)

var bannedStrings = []string{
	"generativeplane",
	"getlamina.ai",
	"lamina",
	"aurelia",
	"axon-",
	"benaskins",
}

// renderSelf renders the named template from the self audience for the
// given mode with sample data.
func renderSelf(t *testing.T, mode, kind string, data PromptData) string {
	t.Helper()
	aud, err := LoadAudience("self", mode)
	if err != nil {
		t.Fatalf("LoadAudience(self, %s): %v", mode, err)
	}
	var tpl *Template
	switch kind {
	case "system":
		tpl = aud.System
	case "draft":
		tpl = aud.Draft
	default:
		t.Fatalf("unknown kind %q", kind)
	}
	out, err := tpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return out
}

func samplePeriodData(workspaceName, workspacePath, activity, previousSection string) PromptData {
	return PromptData{
		Date:            Today(),
		WorkspaceName:   workspaceName,
		WorkspacePath:   workspacePath,
		ActivityReport:  activity,
		PreviousSection: previousSection,
	}
}

func TestWeeklySystemPrompt_NoBannedStrings(t *testing.T) {
	prompt := renderSelf(t, "weekly", "system", samplePeriodData("my-workspace", "/tmp/my-workspace", "(activity report)", ""))
	for _, b := range bannedStrings {
		if strings.Contains(prompt, b) {
			t.Errorf("self/weekly system contains banned string %q", b)
		}
	}
}

func TestWeeklyDraftPrompt_NoBannedStrings(t *testing.T) {
	prompt := renderSelf(t, "weekly", "draft", PromptData{})
	for _, b := range bannedStrings {
		if strings.Contains(prompt, b) {
			t.Errorf("self/weekly draft contains banned string %q", b)
		}
	}
}

func TestWeeklySystemPrompt_MentionsWorkspaceName(t *testing.T) {
	prompt := renderSelf(t, "weekly", "system", samplePeriodData("alpaca", "/tmp/alpaca", "", ""))
	if !strings.Contains(prompt, "alpaca") {
		t.Error("self/weekly system should mention the workspace name")
	}
}

func TestWeeklySystemPrompt_MentionsWorkspacePath(t *testing.T) {
	prompt := renderSelf(t, "weekly", "system", samplePeriodData("alpaca", "/tmp/ws-root", "", ""))
	if !strings.Contains(prompt, "/tmp/ws-root") {
		t.Error("self/weekly system should mention the workspace path so the model knows where to look")
	}
}

func TestDailySystemPrompt_NoBannedStrings(t *testing.T) {
	prompt := renderSelf(t, "daily", "system", samplePeriodData("my-ws", "/tmp/my-ws", "(activity)", ""))
	for _, b := range bannedStrings {
		if strings.Contains(prompt, b) {
			t.Errorf("self/daily system contains banned string %q", b)
		}
	}
}

func TestDailyDraftPrompt_NoBannedStrings(t *testing.T) {
	prompt := renderSelf(t, "daily", "draft", PromptData{})
	for _, b := range bannedStrings {
		if strings.Contains(prompt, b) {
			t.Errorf("self/daily draft contains banned string %q", b)
		}
	}
}

func TestDailySystemPrompt_MentionsWorkspaceName(t *testing.T) {
	prompt := renderSelf(t, "daily", "system", samplePeriodData("alpaca", "/tmp/alpaca", "", ""))
	if !strings.Contains(prompt, "alpaca") {
		t.Error("self/daily system should mention the workspace name")
	}
}

func TestDailySystemPrompt_SignalsShorterInterview(t *testing.T) {
	prompt := renderSelf(t, "daily", "system", samplePeriodData("ws", "/tmp/ws", "", ""))
	if strings.Contains(prompt, "8-10") {
		t.Error("self/daily system should not use the weekly 8-10 exchange budget")
	}
}

func TestWeeklySystemPrompt_PreviousSectionConditional(t *testing.T) {
	with := renderSelf(t, "weekly", "system", samplePeriodData("ws", "/tmp/ws", "", PreviousPostSection("weekly", "previous post content")))
	without := renderSelf(t, "weekly", "system", samplePeriodData("ws", "/tmp/ws", "", ""))
	if !strings.Contains(with, "previous post content") {
		t.Error("expected previous weekly content to be embedded")
	}
	if strings.Contains(without, "Previous weekly post") {
		t.Error("expected no previous weekly section when input is empty")
	}
}
