// Package collect gathers git activity across local repositories for weekly
// update posts. It scans the local dev directory and produces a structured
// markdown report.
package collect

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Report holds the collected activity data.
type Report struct {
	Since    time.Time
	Repos    []RepoActivity
	Markdown string
}

// RepoActivity holds git activity for a single repository.
type RepoActivity struct {
	Name        string
	Path        string
	Machine     string // "local"
	Commits     []string
	Diffstat    string
	Tags        []string
	IsNew       bool
	CommitCount int
}

// Config holds configuration for the collection pass.
type Config struct {
	WorkspacePath string // workspace root containing sibling git repos
	Period        string // "weekly" or "daily"
}

// PeriodDir returns the directory under a workspace path where posts of
// the given period are stored (e.g. <workspace>/.imago/weekly).
func PeriodDir(workspacePath, period string) string {
	return filepath.Join(workspacePath, ".imago", period)
}

// AudienceDir returns the period output directory for the given
// audience. The default audience "self" writes to PeriodDir; other
// audiences nest under PeriodDir/<audience>/ so they don't collide
// with self-audience output.
func AudienceDir(workspacePath, period, audience string) string {
	dir := PeriodDir(workspacePath, period)
	if audience == "" || audience == "self" {
		return dir
	}
	return filepath.Join(dir, audience)
}

// ValidateWorkspace returns an error if path is not a directory containing
// at least one git repo (direct or nested, up to the discoverRepos depth bound).
func ValidateWorkspace(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("workspace path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path %q is not a directory", path)
	}
	repos, err := discoverRepos(path)
	if err != nil {
		return fmt.Errorf("workspace path %q: discover: %w", path, err)
	}
	if len(repos) == 0 {
		return fmt.Errorf("workspace path %q contains no git repositories", path)
	}
	return nil
}

// Run performs the full collection pass: workspace scan and markdown generation.
func Run(cfg Config) (*Report, error) {
	since := deriveSinceDate(PeriodDir(cfg.WorkspacePath, cfg.Period), cfg.Period)

	localRepos, err := scanLocal(cfg.WorkspacePath, since)
	if err != nil {
		return nil, fmt.Errorf("collect: workspace scan: %w", err)
	}

	// Filter to repos with activity.
	var active []RepoActivity
	for _, r := range localRepos {
		if r.CommitCount > 0 {
			active = append(active, r)
		}
	}

	// Sort by commit count descending.
	sort.Slice(active, func(i, j int) bool {
		return active[i].CommitCount > active[j].CommitCount
	})

	report := &Report{
		Since: since,
		Repos: active,
	}
	report.Markdown = renderMarkdown(report)

	return report, nil
}

// deriveSinceDate finds the most recent <period>-YYYY-MM-DD.md file in
// dir and returns its date. Falls back to a period-appropriate window
// (1 day for daily, 7 days for anything else) when no prior file exists.
func deriveSinceDate(dir, period string) time.Time {
	fallback := periodFallback(period)
	if dir == "" {
		return fallback
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fallback
	}

	re := regexp.MustCompile(`^` + regexp.QuoteMeta(period) + `-(\d{4}-\d{2}-\d{2})\.md$`)
	var latest time.Time

	for _, e := range entries {
		if m := re.FindStringSubmatch(e.Name()); m != nil {
			if t, err := time.Parse("2006-01-02", m[1]); err == nil {
				if t.After(latest) {
					latest = t
				}
			}
		}
	}

	if latest.IsZero() {
		return fallback
	}
	return latest
}

func periodFallback(period string) time.Time {
	if period == "daily" {
		return time.Now().AddDate(0, 0, -1)
	}
	return time.Now().AddDate(0, 0, -7)
}

// scanLocal discovers git repos under devDir and gathers activity.
func scanLocal(devDir string, since time.Time) ([]RepoActivity, error) {
	repos, err := discoverRepos(devDir)
	if err != nil {
		return nil, err
	}

	var results []RepoActivity
	for _, path := range repos {
		activity := gatherActivity(path, since, "local")
		results = append(results, activity)
	}
	return results, nil
}


// discoverRepos finds git repositories under a root directory.
func discoverRepos(root string) ([]string, error) {
	var repos []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}

		// Don't descend too deep.
		rel, _ := filepath.Rel(root, path)
		if strings.Count(rel, string(filepath.Separator)) > 3 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() && d.Name() == ".git" {
			repos = append(repos, filepath.Dir(path))
			// Don't descend into .git, but DO continue walking
			// sibling directories — parent repos (like ~/dev/sites)
			// may contain child repos (like ~/dev/sites/getlamina.ai).
			return filepath.SkipDir
		}

		// Don't descend into node_modules.
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}

		return nil
	})

	return repos, err
}

// gatherActivity collects git data for a single local repo.
func gatherActivity(repoPath string, since time.Time, machine string) RepoActivity {
	name := filepath.Base(repoPath)
	activity := RepoActivity{
		Name:    name,
		Path:    repoPath,
		Machine: machine,
	}

	sinceStr := since.Format("2006-01-02")

	// Commits since date.
	cmd := exec.Command("git", "log", "--oneline", "--since="+sinceStr) // #nosec G204 -- git invoked with derived date arg
	cmd.Dir = repoPath
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			activity.Commits = lines
			activity.CommitCount = len(lines)
		}
	}

	if activity.CommitCount == 0 {
		return activity
	}

	// Diffstat.
	cmd = exec.Command("git", "log", "--reverse", "--since="+sinceStr, "--format=%H") // #nosec G204 -- git invoked with derived date arg
	cmd.Dir = repoPath
	if out, err := cmd.Output(); err == nil {
		hashes := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(hashes) > 0 && hashes[0] != "" {
			first := hashes[0]
			diffCmd := exec.Command("git", "diff", "--stat", first+"^..HEAD") // #nosec G204 -- git invoked with derived commit hash
			diffCmd.Dir = repoPath
			if diffOut, err := diffCmd.Output(); err == nil {
				activity.Diffstat = strings.TrimSpace(string(diffOut))
			}
		}
	}

	// Tags.
	cmd = exec.Command("git", "tag", "--sort=-creatordate")
	cmd.Dir = repoPath
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			if len(lines) > 5 {
				lines = lines[:5]
			}
			activity.Tags = lines
		}
	}

	// Check if repo was created within the period.
	cmd = exec.Command("git", "log", "--reverse", "--format=%aI")
	cmd.Dir = repoPath
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 {
			if firstDate, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[0])); err == nil {
				if firstDate.After(since) {
					activity.IsNew = true
				}
			}
		}
	}

	return activity
}


// renderMarkdown produces the structured markdown report.
func renderMarkdown(report *Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Activity since %s\n\n", report.Since.Format("January 2, 2006"))

	totalCommits := 0
	for _, r := range report.Repos {
		totalCommits += r.CommitCount
	}
	fmt.Fprintf(&b, "### %d repos with activity (%d total commits)\n\n", len(report.Repos), totalCommits)

	for _, r := range report.Repos {
		newTag := ""
		if r.IsNew {
			newTag = " [NEW]"
		}
		fmt.Fprintf(&b, "#### %s (%d commits)%s\n", r.Name, r.CommitCount, newTag)
		fmt.Fprintf(&b, "- Key commits:\n")
		shown := r.Commits
		if len(shown) > 10 {
			shown = shown[:10]
		}
		for _, c := range shown {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
		if len(r.Commits) > 10 {
			fmt.Fprintf(&b, "  - ... and %d more\n", len(r.Commits)-10)
		}

		if r.Diffstat != "" {
			// Just show the summary line (last line of diffstat).
			lines := strings.Split(r.Diffstat, "\n")
			summary := lines[len(lines)-1]
			if strings.Contains(summary, "changed") {
				fmt.Fprintf(&b, "- Diffstat: %s\n", strings.TrimSpace(summary))
			}
		}

		if len(r.Tags) > 0 {
			fmt.Fprintf(&b, "- Tags: %s\n", strings.Join(r.Tags, ", "))
		}

		fmt.Fprintln(&b)
	}

	// New repos.
	var newRepos []RepoActivity
	for _, r := range report.Repos {
		if r.IsNew {
			newRepos = append(newRepos, r)
		}
	}
	if len(newRepos) > 0 {
		fmt.Fprintf(&b, "### New repos\n\n")
		for _, r := range newRepos {
			fmt.Fprintf(&b, "- %s (%d commits)\n", r.Name, r.CommitCount)
		}
		fmt.Fprintln(&b)
	}

	return b.String()
}

// PreviousPost reads the most recent <period>-*.md file from dir and
// returns its content, or empty string if none.
func PreviousPost(dir, period string) string {
	if dir == "" {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	re := regexp.MustCompile(`^` + regexp.QuoteMeta(period) + `-(\d{4}-\d{2}-\d{2})\.md$`)
	var latest string
	var latestDate time.Time

	for _, e := range entries {
		if m := re.FindStringSubmatch(e.Name()); m != nil {
			if t, err := time.Parse("2006-01-02", m[1]); err == nil {
				if t.After(latestDate) {
					latestDate = t
					latest = e.Name()
				}
			}
		}
	}

	if latest == "" {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(dir, latest))
	if err != nil {
		return ""
	}
	return string(data)
}
