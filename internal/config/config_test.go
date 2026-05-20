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

func TestDailySystemPrompt_NoBannedStrings(t *testing.T) {
	prompt := DailySystemPrompt("my-ws", "/tmp/my-ws", "(activity)", "")
	banned := []string{"generativeplane", "getlamina.ai", "lamina", "aurelia", "axon-", "benaskins"}
	for _, b := range banned {
		if strings.Contains(prompt, b) {
			t.Errorf("DailySystemPrompt contains banned string %q", b)
		}
	}
}

func TestDailyDraftPrompt_NoBannedStrings(t *testing.T) {
	banned := []string{"generativeplane", "getlamina.ai", "lamina", "aurelia", "axon-", "benaskins"}
	for _, b := range banned {
		if strings.Contains(DailyDraftPrompt, b) {
			t.Errorf("DailyDraftPrompt contains banned string %q", b)
		}
	}
}

func TestDailySystemPrompt_MentionsWorkspaceName(t *testing.T) {
	prompt := DailySystemPrompt("alpaca", "/tmp/alpaca", "", "")
	if !strings.Contains(prompt, "alpaca") {
		t.Error("DailySystemPrompt should mention the workspace name")
	}
}

func TestDailySystemPrompt_SignalsShorterInterview(t *testing.T) {
	prompt := DailySystemPrompt("ws", "/tmp/ws", "", "")
	// The daily prompt should signal a smaller exchange budget than weekly.
	if strings.Contains(prompt, "8-10") {
		t.Error("DailySystemPrompt should not use the weekly 8-10 exchange budget")
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
