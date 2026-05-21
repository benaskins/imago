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
