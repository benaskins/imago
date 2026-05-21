package config

import (
	"fmt"
	"testing"
	"time"
)

func todayFormatted() string {
	return time.Now().Format("2 January 2006")
}

func TestSelfInterviewSystem_Parity(t *testing.T) {
	t.Setenv("DEV", "/tmp/test-workspace")

	expected := SystemPrompt()

	aud, err := LoadAudience("self", "interview")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.System.Render(PromptData{
		Date:          todayFormatted(),
		WorkspacePath: "/tmp/test-workspace",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("interview/system parity mismatch\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfInterviewSystem_ParityFallback(t *testing.T) {
	t.Setenv("DEV", "")

	expected := SystemPrompt()
	aud, err := LoadAudience("self", "interview")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.System.Render(PromptData{
		Date:          todayFormatted(),
		WorkspacePath: "(workspace not configured — set $DEV)",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("interview/system fallback parity mismatch\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfInterviewDraft_Parity(t *testing.T) {
	aud, err := LoadAudience("self", "interview")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.Draft.Render(PromptData{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != DraftPrompt {
		t.Errorf("interview/draft parity mismatch\n--- expected ---\n%q\n--- actual ---\n%q", DraftPrompt, actual)
	}
}

func TestSelfInterviewRevision_Parity(t *testing.T) {
	aud, err := LoadAudience("self", "interview")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	data := PromptData{
		InterviewTranscript: "transcript body",
		FullDraft:           "draft body",
		CurrentSection:      "section body",
	}
	expected := fmt.Sprintf(RevisionPromptTemplate, data.InterviewTranscript, data.FullDraft, data.CurrentSection)
	actual, err := aud.Revision.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("interview/revision parity mismatch\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfInterviewReview_Parity(t *testing.T) {
	aud, err := LoadAudience("self", "interview")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	data := PromptData{
		InterviewTranscript: "transcript body",
		FullArticle:         "article body",
	}
	expected := fmt.Sprintf(ReviewPromptTemplate, data.InterviewTranscript, data.FullArticle)
	actual, err := aud.Review.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("interview/review parity mismatch\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfDailySystem_Parity_NoPrevious(t *testing.T) {
	expected := DailySystemPrompt("alpaca", "/tmp/alpaca", "(activity report body)", "")
	aud, err := LoadAudience("self", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.System.Render(PromptData{
		WorkspaceName:   "alpaca",
		WorkspacePath:   "/tmp/alpaca",
		Date:            todayFormatted(),
		ActivityReport:  "(activity report body)",
		PreviousSection: "",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("daily/system parity mismatch (no previous)\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfDailySystem_Parity_WithPrevious(t *testing.T) {
	expected := DailySystemPrompt("alpaca", "/tmp/alpaca", "(activity)", "previous daily body")
	aud, err := LoadAudience("self", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.System.Render(PromptData{
		WorkspaceName:   "alpaca",
		WorkspacePath:   "/tmp/alpaca",
		Date:            todayFormatted(),
		ActivityReport:  "(activity)",
		PreviousSection: "\n## Previous daily entry (voice and structure reference)\n\nprevious daily body",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("daily/system parity mismatch (with previous)\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfDailyDraft_Parity(t *testing.T) {
	aud, err := LoadAudience("self", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.Draft.Render(PromptData{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != DailyDraftPrompt {
		t.Errorf("daily/draft parity mismatch")
	}
}

func TestSelfWeeklySystem_Parity_NoPrevious(t *testing.T) {
	expected := WeeklySystemPrompt("alpaca", "/tmp/alpaca", "(activity)", "")
	aud, err := LoadAudience("self", "weekly")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.System.Render(PromptData{
		WorkspaceName:   "alpaca",
		WorkspacePath:   "/tmp/alpaca",
		Date:            todayFormatted(),
		ActivityReport:  "(activity)",
		PreviousSection: "",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("weekly/system parity mismatch (no previous)\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfWeeklySystem_Parity_WithPrevious(t *testing.T) {
	expected := WeeklySystemPrompt("alpaca", "/tmp/alpaca", "(activity)", "previous weekly body")
	aud, err := LoadAudience("self", "weekly")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.System.Render(PromptData{
		WorkspaceName:   "alpaca",
		WorkspacePath:   "/tmp/alpaca",
		Date:            todayFormatted(),
		ActivityReport:  "(activity)",
		PreviousSection: "\n## Previous weekly post (voice and structure reference)\n\nprevious weekly body",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != expected {
		t.Errorf("weekly/system parity mismatch (with previous)\n--- expected ---\n%q\n--- actual ---\n%q", expected, actual)
	}
}

func TestSelfWeeklyDraft_Parity(t *testing.T) {
	aud, err := LoadAudience("self", "weekly")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	actual, err := aud.Draft.Render(PromptData{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if actual != WeeklyDraftPrompt {
		t.Errorf("weekly/draft parity mismatch")
	}
}

func TestLoadAudience_UnknownAudience(t *testing.T) {
	if _, err := LoadAudience("nobody", "daily"); err == nil {
		t.Error("expected error for unknown audience")
	}
}

func TestLoadAudience_UnknownMode(t *testing.T) {
	if _, err := LoadAudience("self", "yearly"); err == nil {
		t.Error("expected error for unknown mode")
	}
}
