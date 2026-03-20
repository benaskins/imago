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

func TestParseRemoteOutput(t *testing.T) {
	output := `===REPO=== axon-synd
===PATH=== /Users/benaskins/dev/lamina/axon-synd
===COUNT=== 46
===COMMITS===
ae4906b docs: add README
db695d9 docs: add package documentation
9217d8a feat: render markdown links
===DIFFSTAT===
42 files changed, 2841 insertions(+), 891 deletions(-)
===TAGS===
v0.3.0
v0.2.0
===END===
===REPO=== aurelia
===PATH=== /Users/benaskins/dev/lamina/aurelia
===COUNT=== 38
===COMMITS===
fix: reuse existing API token
refactor: adopt auto-wired pattern
===DIFFSTAT===
15 files changed, 500 insertions(+), 200 deletions(-)
===TAGS===
===END===
===REPO=== inactive-repo
===PATH=== /Users/benaskins/dev/inactive-repo
===COUNT=== 0
===END===`

	repos := parseRemoteOutput(output)

	if len(repos) != 3 {
		t.Fatalf("got %d repos, want 3", len(repos))
	}

	// First repo.
	if repos[0].Name != "axon-synd" {
		t.Errorf("repos[0].Name = %q", repos[0].Name)
	}
	if repos[0].CommitCount != 46 {
		t.Errorf("repos[0].CommitCount = %d, want 46", repos[0].CommitCount)
	}
	if len(repos[0].Commits) != 3 {
		t.Errorf("repos[0].Commits = %d, want 3", len(repos[0].Commits))
	}
	if !strings.Contains(repos[0].Diffstat, "42 files changed") {
		t.Errorf("repos[0].Diffstat = %q", repos[0].Diffstat)
	}
	if len(repos[0].Tags) != 2 {
		t.Errorf("repos[0].Tags = %v, want 2 tags", repos[0].Tags)
	}
	if repos[0].Machine != "hestia" {
		t.Errorf("repos[0].Machine = %q, want hestia", repos[0].Machine)
	}

	// Second repo — no tags.
	if repos[1].Name != "aurelia" {
		t.Errorf("repos[1].Name = %q", repos[1].Name)
	}
	if len(repos[1].Tags) != 0 {
		t.Errorf("repos[1].Tags = %v, want empty", repos[1].Tags)
	}

	// Third repo — no activity.
	if repos[2].CommitCount != 0 {
		t.Errorf("repos[2].CommitCount = %d, want 0", repos[2].CommitCount)
	}
}

func TestMergeRepos(t *testing.T) {
	local := []RepoActivity{
		{Name: "axon-synd", Path: "/local/axon-synd", Machine: "local", CommitCount: 40, Commits: []string{"a", "b"}},
		{Name: "imago", Path: "/local/imago", Machine: "local", CommitCount: 10, Commits: []string{"x"}},
	}
	remote := []RepoActivity{
		{Name: "axon-synd", Path: "/remote/axon-synd", Machine: "hestia", CommitCount: 46, Commits: []string{"a", "b", "c", "d"}},
		{Name: "musicbox", Path: "/remote/musicbox", Machine: "hestia", CommitCount: 30, Commits: []string{"m1"}, IsNew: true},
	}

	merged := mergeRepos(local, remote)

	byName := make(map[string]RepoActivity)
	for _, r := range merged {
		byName[r.Name] = r
	}

	if len(merged) != 3 {
		t.Fatalf("got %d repos, want 3", len(merged))
	}

	// axon-synd: merged, higher count from hestia.
	synd := byName["axon-synd"]
	if synd.Machine != "local, hestia" {
		t.Errorf("axon-synd machine = %q, want 'local, hestia'", synd.Machine)
	}
	if synd.CommitCount != 46 {
		t.Errorf("axon-synd commits = %d, want 46 (higher from hestia)", synd.CommitCount)
	}

	// imago: local only.
	if byName["imago"].Machine != "local" {
		t.Errorf("imago machine = %q, want local", byName["imago"].Machine)
	}

	// musicbox: hestia only, new.
	mb := byName["musicbox"]
	if mb.Machine != "hestia" {
		t.Errorf("musicbox machine = %q, want hestia", mb.Machine)
	}
	if !mb.IsNew {
		t.Error("musicbox should be marked as new")
	}
}

func TestRenderMarkdown(t *testing.T) {
	since, _ := time.Parse("2006-01-02", "2026-03-15")
	report := &Report{
		Since: since,
		Repos: []RepoActivity{
			{
				Name:        "axon-synd",
				Machine:     "local, hestia",
				CommitCount: 46,
				Commits:     []string{"ae4906b docs: add README", "9217d8a feat: render markdown links"},
				Diffstat:    "42 files changed, 2841 insertions(+), 891 deletions(-)",
				Tags:        []string{"v0.3.0"},
			},
			{
				Name:        "musicbox",
				Machine:     "hestia",
				CommitCount: 30,
				Commits:     []string{"abc1234 feat: WASM bridge"},
				IsNew:       true,
			},
		},
		NewSites: []string{"sailorgrift.com"},
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
	if !strings.Contains(md, "Machines: local, hestia") {
		t.Error("should list machines")
	}
	if !strings.Contains(md, "#### musicbox (30 commits) [NEW]") {
		t.Error("should tag new repos")
	}
	if !strings.Contains(md, "### New repos") {
		t.Error("should have new repos section")
	}
	if !strings.Contains(md, "### New sites published") {
		t.Error("should have new sites section")
	}
	if !strings.Contains(md, "sailorgrift.com") {
		t.Error("should list new sites")
	}
	if !strings.Contains(md, "Tags: v0.3.0") {
		t.Error("should list tags")
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
