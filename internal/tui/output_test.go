package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Building Local AI Tools", "building-local-ai-tools"},
		{"The Glue-Code Problem", "the-glue-code-problem"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
		{"symbols!@#$%here", "symbols-here"},
		{"", "untitled"},
		{"   ", "untitled"},
	}

	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractTitle(t *testing.T) {
	md := "# My Great Post\n\nSome content.\n\n## Section"
	got := extractTitle(md)
	if got != "My Great Post" {
		t.Errorf("extractTitle = %q, want %q", got, "My Great Post")
	}

	got = extractTitle("No headings here")
	if got != "" {
		t.Errorf("extractTitle with no heading = %q, want empty", got)
	}
}

func TestWritePeriodDraft_Weekly(t *testing.T) {
	dir := t.TempDir()
	md := "# Week notes\n\nbody"
	path, err := writePeriodDraft(md, dir, "weekly")
	if err != nil {
		t.Fatalf("writePeriodDraft: %v", err)
	}
	want := "weekly-" + time.Now().Format("2006-01-02") + ".md"
	if filepath.Base(path) != want {
		t.Errorf("expected filename %q, got %q", want, filepath.Base(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != md {
		t.Errorf("file content mismatch")
	}
}

func TestWritePeriodDraft_Daily(t *testing.T) {
	dir := t.TempDir()
	path, err := writePeriodDraft("# Daily notes\n\nbody", dir, "daily")
	if err != nil {
		t.Fatalf("writePeriodDraft: %v", err)
	}
	want := "daily-" + time.Now().Format("2006-01-02") + ".md"
	if filepath.Base(path) != want {
		t.Errorf("expected filename %q, got %q", want, filepath.Base(path))
	}
}

func TestWritePeriodDraft_CreatesDir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, ".imago", "daily")
	if _, err := writePeriodDraft("# hi", nested, "daily"); err != nil {
		t.Fatalf("writePeriodDraft: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("expected dir to be created: %v", err)
	}
}

func TestWriteDraft(t *testing.T) {
	// Use a temp dir instead of ~/Documents
	tmpDir := t.TempDir()

	// Temporarily override outputDir
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	md := "# Test Post\n\nContent here."
	path, err := writeDraft(md)
	if err != nil {
		t.Fatalf("writeDraft: %v", err)
	}

	if filepath.Base(path) != "test-post.md" {
		t.Errorf("expected test-post.md, got %s", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != md {
		t.Errorf("file content mismatch")
	}

	// Second write should not overwrite
	path2, err := writeDraft(md)
	if err != nil {
		t.Fatalf("writeDraft second: %v", err)
	}
	if path2 == path {
		t.Error("second write should not overwrite first")
	}
	if filepath.Base(path2) != "test-post-2.md" {
		t.Errorf("expected test-post-2.md, got %s", filepath.Base(path2))
	}
}
