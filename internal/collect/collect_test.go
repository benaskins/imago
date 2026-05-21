package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeriveSinceDate_WeeklyFromFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-08.md"), []byte("# Week 1"), 0644)
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-15.md"), []byte("# Week 2"), 0644)
	os.WriteFile(filepath.Join(dir, "other-file.md"), []byte("# Not weekly"), 0644)

	got := deriveSinceDate(dir, "weekly")

	want, _ := time.Parse("2006-01-02", "2026-03-15")
	if !got.Equal(want) {
		t.Errorf("deriveSinceDate(weekly) = %v, want %v", got, want)
	}
}

func TestDeriveSinceDate_DailyFromFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "daily-2026-05-18.md"), []byte("# d"), 0644)
	os.WriteFile(filepath.Join(dir, "daily-2026-05-20.md"), []byte("# d"), 0644)
	os.WriteFile(filepath.Join(dir, "weekly-2026-05-19.md"), []byte("# w"), 0644)

	got := deriveSinceDate(dir, "daily")

	want, _ := time.Parse("2006-01-02", "2026-05-20")
	if !got.Equal(want) {
		t.Errorf("deriveSinceDate(daily) = %v, want %v", got, want)
	}
}

func TestRun_WeeklyDerivesSinceFromWorkspace(t *testing.T) {
	workspace := t.TempDir()
	weeklyDir := filepath.Join(workspace, ".imago", "weekly")
	if err := os.MkdirAll(weeklyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(weeklyDir, "weekly-2026-03-15.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := Run(Config{WorkspacePath: workspace, Period: "weekly"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want, _ := time.Parse("2006-01-02", "2026-03-15")
	if !report.Since.Equal(want) {
		t.Errorf("Since = %v, want %v", report.Since, want)
	}
}

func TestRun_DailyDerivesSinceFromWorkspace(t *testing.T) {
	workspace := t.TempDir()
	dailyDir := filepath.Join(workspace, ".imago", "daily")
	if err := os.MkdirAll(dailyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dailyDir, "daily-2026-05-20.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := Run(Config{WorkspacePath: workspace, Period: "daily"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want, _ := time.Parse("2006-01-02", "2026-05-20")
	if !report.Since.Equal(want) {
		t.Errorf("Since = %v, want %v", report.Since, want)
	}
}

func TestDeriveSinceDate_WeeklyNoFilesFallsBack7Days(t *testing.T) {
	got := deriveSinceDate(t.TempDir(), "weekly")
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	diff := got.Sub(sevenDaysAgo)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("deriveSinceDate(weekly) = %v, want ~%v", got, sevenDaysAgo)
	}
}

func TestDeriveSinceDate_DailyNoFilesFallsBack1Day(t *testing.T) {
	got := deriveSinceDate(t.TempDir(), "daily")
	oneDayAgo := time.Now().AddDate(0, 0, -1)
	diff := got.Sub(oneDayAgo)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("deriveSinceDate(daily) = %v, want ~%v", got, oneDayAgo)
	}
}

func TestDeriveSinceDate_EmptyDir(t *testing.T) {
	got := deriveSinceDate("", "weekly")

	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	diff := got.Sub(sevenDaysAgo)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("deriveSinceDate = %v, want ~%v", got, sevenDaysAgo)
	}
}

func TestPeriodDir(t *testing.T) {
	if got := PeriodDir("/tmp/ws", "weekly"); got != filepath.Join("/tmp/ws", ".imago", "weekly") {
		t.Errorf("PeriodDir weekly = %q", got)
	}
	if got := PeriodDir("/tmp/ws", "daily"); got != filepath.Join("/tmp/ws", ".imago", "daily") {
		t.Errorf("PeriodDir daily = %q", got)
	}
}

func TestAudienceDir_SelfMatchesPeriodDir(t *testing.T) {
	want := PeriodDir("/tmp/ws", "daily")
	if got := AudienceDir("/tmp/ws", "daily", "self"); got != want {
		t.Errorf("AudienceDir self = %q, want %q", got, want)
	}
	if got := AudienceDir("/tmp/ws", "daily", ""); got != want {
		t.Errorf("AudienceDir empty audience = %q, want %q", got, want)
	}
}

func TestAudienceDir_NonSelfNestsUnderPeriod(t *testing.T) {
	want := filepath.Join("/tmp/ws", ".imago", "daily", "manager")
	if got := AudienceDir("/tmp/ws", "daily", "manager"); got != want {
		t.Errorf("AudienceDir manager = %q, want %q", got, want)
	}
}

func TestRenderMarkdown(t *testing.T) {
	since, _ := time.Parse("2006-01-02", "2026-03-15")
	report := &Report{
		Since: since,
		Repos: []RepoActivity{
			{
				Name:        "axon-synd",
				Machine:     "local",
				CommitCount: 46,
				Commits:     []string{"ae4906b docs: add README", "9217d8a feat: render markdown links"},
				Diffstat:    "42 files changed, 2841 insertions(+), 891 deletions(-)",
				Tags:        []string{"v0.3.0"},
			},
			{
				Name:        "musicbox",
				Machine:     "local",
				CommitCount: 30,
				Commits:     []string{"abc1234 feat: WASM bridge"},
				IsNew:       true,
			},
		},
	}

	md := renderMarkdown(report)

	if !strings.Contains(md, "March 15, 2026") {
		t.Error("should contain formatted since date")
	}
	if !strings.Contains(md, "2 repos with activity (76 total commits)") {
		t.Error("should contain repo count and total commits")
	}
	if !strings.Contains(md, "#### axon-synd (46 commits)") {
		t.Error("should contain axon-synd heading")
	}
	if !strings.Contains(md, "#### musicbox (30 commits) [NEW]") {
		t.Error("should tag new repos")
	}
	if !strings.Contains(md, "### New repos") {
		t.Error("should have new repos section")
	}
	if !strings.Contains(md, "Tags: v0.3.0") {
		t.Error("should list tags")
	}
}

func TestRenderMarkdown_NoNewSitesSection(t *testing.T) {
	since, _ := time.Parse("2006-01-02", "2026-03-15")
	report := &Report{
		Since: since,
		Repos: []RepoActivity{{Name: "r", Machine: "local", CommitCount: 1, Commits: []string{"abc x"}}},
	}
	md := renderMarkdown(report)
	if strings.Contains(md, "New sites") {
		t.Error("renderMarkdown should not emit a New sites section")
	}
}

func TestRun_WorkspacePath_EmptyWorkspace(t *testing.T) {
	workspace := t.TempDir()
	report, err := Run(Config{WorkspacePath: workspace})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(report.Repos) != 0 {
		t.Errorf("expected no active repos in empty workspace, got %d", len(report.Repos))
	}
}

func TestValidateWorkspace_OK(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "repo", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWorkspace(ws); err != nil {
		t.Errorf("ValidateWorkspace: unexpected error: %v", err)
	}
}

func TestValidateWorkspace_DoesNotExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := ValidateWorkspace(missing); err == nil {
		t.Error("ValidateWorkspace: expected error for missing path")
	}
}

func TestValidateWorkspace_NotDirectory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWorkspace(f); err == nil {
		t.Error("ValidateWorkspace: expected error for non-directory")
	}
}

func TestValidateWorkspace_NoGitRepos(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWorkspace(ws); err == nil {
		t.Error("ValidateWorkspace: expected error when workspace contains no git repos")
	}
}

func TestRun_WorkspacePath_DiscoversRepos(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "repo-a", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "repo-b", ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	repos, err := discoverRepos(workspace)
	if err != nil {
		t.Fatalf("discoverRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("got %d repos under workspace, want 2: %v", len(repos), repos)
	}
}

func TestRenderMarkdown_NoNewReposOrSites(t *testing.T) {
	since, _ := time.Parse("2006-01-02", "2026-03-15")
	report := &Report{
		Since: since,
		Repos: []RepoActivity{
			{Name: "axon", Machine: "local", CommitCount: 5, Commits: []string{"abc feat: something"}},
		},
	}

	md := renderMarkdown(report)

	if strings.Contains(md, "### New repos") {
		t.Error("should not have new repos section when none are new")
	}
	if strings.Contains(md, "### New sites") {
		t.Error("should not have new sites section when none exist")
	}
}

func TestPreviousPost_Weekly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-08.md"), []byte("# Week 1\nOld content"), 0644)
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-15.md"), []byte("# Week 2\nLatest content"), 0644)

	got := PreviousPost(dir, "weekly")
	if got != "# Week 2\nLatest content" {
		t.Errorf("PreviousPost(weekly) = %q", got)
	}
}

func TestPreviousPost_Daily(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "daily-2026-05-19.md"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(dir, "daily-2026-05-20.md"), []byte("latest"), 0644)
	os.WriteFile(filepath.Join(dir, "weekly-2026-05-19.md"), []byte("weekly"), 0644)

	got := PreviousPost(dir, "daily")
	if got != "latest" {
		t.Errorf("PreviousPost(daily) = %q, want %q", got, "latest")
	}
}

func TestPreviousPost_NoFiles(t *testing.T) {
	if got := PreviousPost(t.TempDir(), "weekly"); got != "" {
		t.Errorf("PreviousPost = %q, want empty", got)
	}
}

func TestPreviousPost_EmptyDir(t *testing.T) {
	if got := PreviousPost("", "weekly"); got != "" {
		t.Errorf("PreviousPost = %q, want empty", got)
	}
}

func TestRenderMarkdown_TruncatesLongCommitLists(t *testing.T) {
	commits := make([]string, 15)
	for i := range commits {
		commits[i] = "abc1234 commit message"
	}

	since, _ := time.Parse("2006-01-02", "2026-03-15")
	report := &Report{
		Since: since,
		Repos: []RepoActivity{
			{Name: "busy-repo", Machine: "local", CommitCount: 15, Commits: commits},
		},
	}

	md := renderMarkdown(report)

	if !strings.Contains(md, "... and 5 more") {
		t.Error("should truncate and show remaining count")
	}
}
