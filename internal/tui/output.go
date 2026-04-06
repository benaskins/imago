package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a title to a URL-friendly slug.
func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}

// extractTitle pulls the first # heading from markdown content.
func extractTitle(md string) string {
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}
	return ""
}

// outputDir returns the directory for imago drafts, creating it if needed.
func outputDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "imago")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return dir, nil
}

// writeDraft writes the final markdown to ~/Documents/imago/<slug>.md
// and returns the full path.
func writeDraft(markdown string) (string, error) {
	dir, err := outputDir()
	if err != nil {
		return "", err
	}

	title := extractTitle(markdown)
	slug := slugify(title)
	path := filepath.Join(dir, slug+".md")

	// Don't overwrite — append a number if needed
	if _, err := os.Stat(path); err == nil {
		for i := 2; ; i++ {
			candidate := filepath.Join(dir, fmt.Sprintf("%s-%d.md", slug, i))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				path = candidate
				break
			}
		}
	}

	if err := os.WriteFile(path, []byte(markdown), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
