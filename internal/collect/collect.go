// Package collect gathers git activity across repositories for weekly
// update posts. It scans local and remote machines, deduplicates repos,
// and produces a structured markdown report.
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
	Since   time.Time
	Repos   []RepoActivity
	NewSites []string
	Markdown string
}

// RepoActivity holds git activity for a single repository.
type RepoActivity struct {
	Name      string
	Path      string
	Machine   string // "local", "hestia", or "local, hestia"
	Commits   []string
	Diffstat  string
	Tags      []string
	IsNew     bool
	CommitCount int
}

// Config holds configuration for the collection pass.
type Config struct {
	SiteDir string // path to generativeplane.com site directory
	DevDir  string // local ~/dev directory
}

// Run performs the full collection pass: local scan, remote scan,
// dedup, and markdown generation.
func Run(cfg Config) (*Report, error) {
	since := deriveSinceDate(cfg.SiteDir)

	localRepos, err := scanLocal(cfg.DevDir, since)
	if err != nil {
		return nil, fmt.Errorf("collect: local scan: %w", err)
	}

	remoteRepos, err := scanRemote("hestia", since)
	if err != nil {
		// Remote scan failure is non-fatal — report what we have.
		remoteRepos = nil
	}

	merged := mergeRepos(localRepos, remoteRepos)

	// Filter to repos with activity.
	var active []RepoActivity
	for _, r := range merged {
		if r.CommitCount > 0 {
			active = append(active, r)
		}
	}

	// Sort by commit count descending.
	sort.Slice(active, func(i, j int) bool {
		return active[i].CommitCount > active[j].CommitCount
	})

	// Detect new sites.
	newSites := detectNewSites(cfg.DevDir, since)

	report := &Report{
		Since:    since,
		Repos:    active,
		NewSites: newSites,
	}
	report.Markdown = renderMarkdown(report)

	return report, nil
}

// deriveSinceDate finds the most recent weekly-*.md file in the site
// directory and parses the date from the filename. Falls back to 7
// days ago if no weekly exists.
func deriveSinceDate(siteDir string) time.Time {
	if siteDir == "" {
		return time.Now().AddDate(0, 0, -7)
	}

	entries, err := os.ReadDir(siteDir)
	if err != nil {
		return time.Now().AddDate(0, 0, -7)
	}

	re := regexp.MustCompile(`^weekly-(\d{4}-\d{2}-\d{2})\.md$`)
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
		return time.Now().AddDate(0, 0, -7)
	}
	return latest
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

// scanRemote discovers git repos on a remote machine via SSH.
func scanRemote(host string, since time.Time) ([]RepoActivity, error) {
	sinceStr := since.Format("2006-01-02")

	// Single SSH command that discovers repos and gathers activity for each.
	script := fmt.Sprintf(`
set -e
for gitdir in $(find ~/dev -name .git -type d -maxdepth 4 2>/dev/null); do
  repo=$(dirname "$gitdir")
  name=$(basename "$repo")
  echo "===REPO=== $name"
  echo "===PATH=== $repo"
  cd "$repo"
  count=$(git log --oneline --since="%s" 2>/dev/null | wc -l | tr -d ' ')
  echo "===COUNT=== $count"
  if [ "$count" -gt 0 ]; then
    echo "===COMMITS==="
    git log --oneline --since="%s" 2>/dev/null | head -50
    echo "===DIFFSTAT==="
    first=$(git log --reverse --since="%s" --format="%%H" 2>/dev/null | head -1)
    if [ -n "$first" ]; then
      git diff --stat "$first^..HEAD" 2>/dev/null || git diff --stat "$first..HEAD" 2>/dev/null || echo "(no diffstat)"
    fi
    echo "===TAGS==="
    git tag --sort=-creatordate 2>/dev/null | head -5
  fi
  echo "===END==="
done
`, sinceStr, sinceStr, sinceStr)

	cmd := exec.Command("ssh", host, "bash", "-c", fmt.Sprintf("'%s'", strings.ReplaceAll(script, "'", "'\\''")))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w: %s", host, err, string(out))
	}

	return parseRemoteOutput(string(out)), nil
}

// parseRemoteOutput parses the structured output from the SSH script.
func parseRemoteOutput(output string) []RepoActivity {
	var repos []RepoActivity
	var current *RepoActivity
	var section string

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "===REPO=== "):
			if current != nil {
				repos = append(repos, *current)
			}
			current = &RepoActivity{
				Name:    strings.TrimPrefix(line, "===REPO=== "),
				Machine: "hestia",
			}
			section = ""
		case strings.HasPrefix(line, "===PATH=== "):
			if current != nil {
				current.Path = strings.TrimPrefix(line, "===PATH=== ")
			}
		case strings.HasPrefix(line, "===COUNT=== "):
			if current != nil {
				fmt.Sscanf(strings.TrimPrefix(line, "===COUNT=== "), "%d", &current.CommitCount)
			}
		case line == "===COMMITS===":
			section = "commits"
		case line == "===DIFFSTAT===":
			section = "diffstat"
		case line == "===TAGS===":
			section = "tags"
		case line == "===END===":
			if current != nil {
				repos = append(repos, *current)
				current = nil
			}
			section = ""
		default:
			if current == nil {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			switch section {
			case "commits":
				current.Commits = append(current.Commits, trimmed)
			case "diffstat":
				if current.Diffstat != "" {
					current.Diffstat += "\n"
				}
				current.Diffstat += trimmed
			case "tags":
				current.Tags = append(current.Tags, trimmed)
			}
		}
	}
	if current != nil {
		repos = append(repos, *current)
	}
	return repos
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
			return filepath.SkipDir
		}

		// Don't descend into .git directories.
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
	cmd := exec.Command("git", "log", "--oneline", "--since="+sinceStr)
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
	cmd = exec.Command("git", "log", "--reverse", "--since="+sinceStr, "--format=%H")
	cmd.Dir = repoPath
	if out, err := cmd.Output(); err == nil {
		hashes := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(hashes) > 0 && hashes[0] != "" {
			first := hashes[0]
			diffCmd := exec.Command("git", "diff", "--stat", first+"^..HEAD")
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

// mergeRepos deduplicates repos found on multiple machines by matching
// on repo name. When the same repo exists on both machines, the commit
// lists are merged and the machine field is updated.
func mergeRepos(local, remote []RepoActivity) []RepoActivity {
	byName := make(map[string]*RepoActivity)

	for i := range local {
		byName[local[i].Name] = &local[i]
	}

	for _, r := range remote {
		if existing, ok := byName[r.Name]; ok {
			// Same repo on both machines — merge.
			existing.Machine = "local, hestia"
			// Use the higher commit count (they should be similar
			// if both are in sync, but take the max).
			if r.CommitCount > existing.CommitCount {
				existing.CommitCount = r.CommitCount
				existing.Commits = r.Commits
				existing.Diffstat = r.Diffstat
			}
			if len(r.Tags) > len(existing.Tags) {
				existing.Tags = r.Tags
			}
			if r.IsNew && !existing.IsNew {
				existing.IsNew = true
			}
		} else {
			// Only on remote.
			rc := r
			byName[r.Name] = &rc
		}
	}

	var result []RepoActivity
	for _, v := range byName {
		result = append(result, *v)
	}
	return result
}

// detectNewSites checks for site directories that were created recently.
func detectNewSites(devDir string, since time.Time) []string {
	sitesDir := filepath.Join(devDir, "sites")
	entries, err := os.ReadDir(sitesDir)
	if err != nil {
		return nil
	}

	var newSites []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Check if the git repo was created after the since date.
		gitDir := filepath.Join(sitesDir, e.Name(), ".git")
		if _, err := os.Stat(gitDir); err != nil {
			continue
		}
		cmd := exec.Command("git", "log", "--reverse", "--format=%aI")
		cmd.Dir = filepath.Join(sitesDir, e.Name())
		if out, err := cmd.Output(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) > 0 {
				if firstDate, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[0])); err == nil {
					if firstDate.After(since) {
						newSites = append(newSites, e.Name())
					}
				}
			}
		}
	}
	return newSites
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
		fmt.Fprintf(&b, "- Machines: %s\n", r.Machine)

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
			fmt.Fprintf(&b, "- %s (%d commits) — %s\n", r.Name, r.CommitCount, r.Machine)
		}
		fmt.Fprintln(&b)
	}

	if len(report.NewSites) > 0 {
		fmt.Fprintf(&b, "### New sites published\n\n")
		for _, s := range report.NewSites {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		fmt.Fprintln(&b)
	}

	return b.String()
}

// PreviousWeekly reads the most recent weekly-*.md file from the site
// directory and returns its content.
func PreviousWeekly(siteDir string) string {
	if siteDir == "" {
		return ""
	}

	entries, err := os.ReadDir(siteDir)
	if err != nil {
		return ""
	}

	re := regexp.MustCompile(`^weekly-(\d{4}-\d{2}-\d{2})\.md$`)
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

	data, err := os.ReadFile(filepath.Join(siteDir, latest))
	if err != nil {
		return ""
	}
	return string(data)
}
