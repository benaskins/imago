package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeriveSinceDate_FromWeeklyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-08.md"), []byte("# Week 1"), 0644)
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-15.md"), []byte("# Week 2"), 0644)
	os.WriteFile(filepath.Join(dir, "other-file.md"), []byte("# Not weekly"), 0644)

	got := deriveSinceDate(dir)

	want, _ := time.Parse("2006-01-02", "2026-03-15")
	if !got.Equal(want) {
		t.Errorf("deriveSinceDate = %v, want %v", got, want)
	}
}

func TestDeriveSinceDate_NoWeeklyFiles(t *testing.T) {
	dir := t.TempDir()

	got := deriveSinceDate(dir)

	// Should fall back to ~7 days ago.
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	diff := got.Sub(sevenDaysAgo)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("deriveSinceDate = %v, want ~%v", got, sevenDaysAgo)
	}
}

func TestDeriveSinceDate_EmptyDir(t *testing.T) {
	got := deriveSinceDate("")

	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	diff := got.Sub(sevenDaysAgo)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("deriveSinceDate = %v, want ~%v", got, sevenDaysAgo)
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

func TestPreviousWeekly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-08.md"), []byte("# Week 1\nOld content"), 0644)
	os.WriteFile(filepath.Join(dir, "weekly-2026-03-15.md"), []byte("# Week 2\nLatest content"), 0644)

	got := PreviousWeekly(dir)

	if got != "# Week 2\nLatest content" {
		t.Errorf("PreviousWeekly = %q", got)
	}
}

func TestPreviousWeekly_NoFiles(t *testing.T) {
	got := PreviousWeekly(t.TempDir())
	if got != "" {
		t.Errorf("PreviousWeekly = %q, want empty", got)
	}
}

func TestPreviousWeekly_EmptyDir(t *testing.T) {
	got := PreviousWeekly("")
	if got != "" {
		t.Errorf("PreviousWeekly = %q, want empty", got)
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
