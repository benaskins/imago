package config

import (
	"strings"
	"testing"
)

func TestWeeklySystemPrompt_NoBannedStrings(t *testing.T) {
	prompt := WeeklySystemPrompt("my-workspace", "/tmp/my-workspace", "(activity report)", "")
	banned := []string{
		"generativeplane",
		"getlamina.ai",
		"lamina",
		"aurelia",
		"axon-",
		"benaskins",
	}
	for _, b := range banned {
		if strings.Contains(prompt, b) {
			t.Errorf("WeeklySystemPrompt contains banned string %q", b)
		}
	}
}

func TestWeeklyDraftPrompt_NoBannedStrings(t *testing.T) {
	banned := []string{
		"generativeplane",
		"getlamina.ai",
		"lamina",
		"aurelia",
		"axon-",
		"benaskins",
	}
	for _, b := range banned {
		if strings.Contains(WeeklyDraftPrompt, b) {
			t.Errorf("WeeklyDraftPrompt contains banned string %q", b)
		}
	}
}

func TestWeeklySystemPrompt_MentionsWorkspaceName(t *testing.T) {
	prompt := WeeklySystemPrompt("alpaca", "/tmp/alpaca", "", "")
	if !strings.Contains(prompt, "alpaca") {
		t.Error("WeeklySystemPrompt should mention the workspace name")
	}
}

func TestWeeklySystemPrompt_MentionsWorkspacePath(t *testing.T) {
	prompt := WeeklySystemPrompt("alpaca", "/tmp/ws-root", "", "")
	if !strings.Contains(prompt, "/tmp/ws-root") {
		t.Error("WeeklySystemPrompt should mention the workspace path so the model knows where to look")
	}
}

func TestWeeklySystemPrompt_PreviousWeeklySectionConditional(t *testing.T) {
	with := WeeklySystemPrompt("ws", "/tmp/ws", "", "previous post content")
	without := WeeklySystemPrompt("ws", "/tmp/ws", "", "")
	if !strings.Contains(with, "previous post content") {
		t.Error("expected previous weekly content to be embedded")
	}
	if strings.Contains(without, "Previous weekly post") {
		t.Error("expected no previous weekly section when input is empty")
	}
}
