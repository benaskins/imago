package config

import (
	"slices"
	"strings"
	"testing"
)

func TestLoadAudience_SelfInterviewHasAllTemplates(t *testing.T) {
	aud, err := LoadAudience("self", "interview")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	if aud.System == nil {
		t.Error("expected System template")
	}
	if aud.Draft == nil {
		t.Error("expected Draft template")
	}
	if aud.Revision == nil {
		t.Error("expected Revision template (shared at audience level)")
	}
	if aud.Review == nil {
		t.Error("expected Review template (shared at audience level)")
	}
}

func TestLoadAudience_SelfDailyInheritsRevisionAndReview(t *testing.T) {
	aud, err := LoadAudience("self", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	if aud.System == nil || aud.Draft == nil {
		t.Error("expected daily-specific system + draft")
	}
	if aud.Revision == nil || aud.Review == nil {
		t.Error("expected revision/review to be inherited from audience root")
	}
}

func TestLoadAudience_ManagerDailyHasAllTemplates(t *testing.T) {
	aud, err := LoadAudience("manager", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	if aud.System == nil || aud.Draft == nil {
		t.Error("expected manager/daily system and draft templates")
	}
	if aud.Revision == nil || aud.Review == nil {
		t.Error("expected revision/review inherited from self via the fallback chain")
	}
}

func TestManagerDailyDraft_StatusSections(t *testing.T) {
	aud, err := LoadAudience("manager", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	out, err := aud.Draft.Render(PromptData{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, section := range []string{"What shipped", "In progress", "Blockers", "Next"} {
		if !strings.Contains(out, section) {
			t.Errorf("manager/daily draft should mention %q section heading", section)
		}
	}
	// The self/daily draft uses these structural cues; manager/daily should not.
	for _, journalish := range []string{"What happened", "Daily notes:"} {
		if strings.Contains(out, journalish) {
			t.Errorf("manager/daily draft should not contain self/daily structural cue %q", journalish)
		}
	}
}

func TestManagerDailySystem_ShortInterview(t *testing.T) {
	aud, err := LoadAudience("manager", "daily")
	if err != nil {
		t.Fatalf("LoadAudience: %v", err)
	}
	out, err := aud.System.Render(samplePeriodData("ws", "/tmp/ws", "(activity)", ""))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Manager-facing should signal an even tighter interview than self/daily (3-5).
	if !strings.Contains(out, "2 to 3 exchanges") {
		t.Errorf("manager/daily system should signal 2-3 exchanges; got prompt without that signal")
	}
	// self/daily system invites "honest, specific reflection"; manager/daily should not.
	if strings.Contains(out, "honest, specific reflection") {
		t.Errorf("manager/daily system should not invite personal reflection")
	}
}

func TestAvailableAudiences_DailyIncludesManager(t *testing.T) {
	got := AvailableAudiences("daily")
	if !contains(got, "manager") {
		t.Errorf("expected 'manager' in available daily audiences, got %v", got)
	}
}

func TestAvailableAudiences_InterviewExcludesManager(t *testing.T) {
	got := AvailableAudiences("interview")
	if contains(got, "manager") {
		t.Errorf("manager has no interview mode but appeared: %v", got)
	}
}

func TestAvailableAudiences_IncludesSelf(t *testing.T) {
	got := AvailableAudiences("interview")
	if !contains(got, "self") {
		t.Errorf("expected 'self' in available interview audiences, got %v", got)
	}
}

func TestAvailableAudiences_DailyIncludesSelf(t *testing.T) {
	got := AvailableAudiences("daily")
	if !contains(got, "self") {
		t.Errorf("expected 'self' in available daily audiences, got %v", got)
	}
}

func TestAvailableAudiences_UnknownMode(t *testing.T) {
	got := AvailableAudiences("yearly")
	if len(got) != 0 {
		t.Errorf("expected no audiences for unknown mode, got %v", got)
	}
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
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

func TestPreviousPostSection_Empty(t *testing.T) {
	if got := PreviousPostSection("weekly", ""); got != "" {
		t.Errorf("expected empty string for empty previous post, got %q", got)
	}
}

func TestPreviousPostSection_WeeklyHeading(t *testing.T) {
	got := PreviousPostSection("weekly", "body")
	if !strings.Contains(got, "## Previous weekly post") {
		t.Errorf("expected weekly heading, got %q", got)
	}
	if !strings.HasSuffix(got, "body") {
		t.Errorf("expected previous post body at end, got %q", got)
	}
}

func TestPreviousPostSection_DailyHeading(t *testing.T) {
	got := PreviousPostSection("daily", "body")
	if !strings.Contains(got, "## Previous daily entry") {
		t.Errorf("expected daily heading, got %q", got)
	}
}

func TestResolveWorkspacePath_Fallback(t *testing.T) {
	t.Setenv("DEV", "")
	if got := ResolveWorkspacePath(); !strings.Contains(got, "set $DEV") {
		t.Errorf("expected fallback message, got %q", got)
	}
}

func TestResolveWorkspacePath_FromEnv(t *testing.T) {
	t.Setenv("DEV", "/tmp/workspace-x")
	if got := ResolveWorkspacePath(); got != "/tmp/workspace-x" {
		t.Errorf("expected env value, got %q", got)
	}
}
