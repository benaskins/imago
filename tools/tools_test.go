package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tool "github.com/benaskins/axon-tool"
)

func newToolContext() *tool.ToolContext {
	return &tool.ToolContext{
		Ctx: context.Background(),
	}
}

// ---------------------------------------------------------------------------
// repo_overview
// ---------------------------------------------------------------------------

func TestRepoOverview(t *testing.T) {
	// Create a temp repo with a README
	tmp := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# Test Repo\n\nA test."), 0o644)
	os.Mkdir(filepath.Join(tmp, "cmd"), 0o755)
	os.WriteFile(filepath.Join(tmp, "cmd", "main.go"), []byte("package main"), 0o644)

	// Make a commit so git log works
	exec.Command("git", "-C", tmp, "add", ".").Run()
	exec.Command("git", "-C", tmp, "commit", "-m", "init").Run()

	td := RepoOverview()
	result := td.Execute(newToolContext(), map[string]any{"dir": tmp})

	if !strings.Contains(result.Content, "## Directory tree") {
		t.Error("missing directory tree section")
	}
	if !strings.Contains(result.Content, "cmd/") {
		t.Error("missing cmd dir in tree")
	}
	if !strings.Contains(result.Content, "## Recent commits") {
		t.Error("missing commits section")
	}
	if !strings.Contains(result.Content, "init") {
		t.Error("missing commit message")
	}
	if !strings.Contains(result.Content, "## README.md") {
		t.Error("missing README section")
	}
	if !strings.Contains(result.Content, "# Test Repo") {
		t.Error("missing README content")
	}
}

func TestDirTree(t *testing.T) {
	tmp := t.TempDir()
	os.Mkdir(filepath.Join(tmp, "src"), 0o755)
	os.WriteFile(filepath.Join(tmp, "src", "main.go"), []byte(""), 0o644)
	os.Mkdir(filepath.Join(tmp, ".git"), 0o755) // should be filtered
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte(""), 0o644)

	tree, err := dirTree(tmp, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tree, "src/") {
		t.Error("missing src dir")
	}
	if !strings.Contains(tree, "main.go") {
		t.Error("missing main.go")
	}
	if strings.Contains(tree, ".git") {
		t.Error("should filter hidden dirs")
	}
}

// ---------------------------------------------------------------------------
// read_files
// ---------------------------------------------------------------------------

func TestReadFiles(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("content a"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("content b"), 0o644)

	td := ReadFiles()

	t.Run("reads multiple files", func(t *testing.T) {
		result := td.Execute(newToolContext(), map[string]any{
			"paths": []any{
				filepath.Join(tmp, "a.txt"),
				filepath.Join(tmp, "b.txt"),
			},
		})
		if !strings.Contains(result.Content, "content a") {
			t.Error("missing content a")
		}
		if !strings.Contains(result.Content, "content b") {
			t.Error("missing content b")
		}
	})

	t.Run("handles missing file", func(t *testing.T) {
		result := td.Execute(newToolContext(), map[string]any{
			"paths": []any{filepath.Join(tmp, "missing.txt")},
		})
		if !strings.Contains(result.Content, "Error:") {
			t.Error("expected error for missing file")
		}
	})

	t.Run("rejects more than 5", func(t *testing.T) {
		paths := make([]any, 6)
		for i := range paths {
			paths[i] = "file.txt"
		}
		result := td.Execute(newToolContext(), map[string]any{"paths": paths})
		if !strings.Contains(result.Content, "max 5") {
			t.Errorf("expected max 5 error, got: %q", result.Content)
		}
	})

	t.Run("rejects empty paths", func(t *testing.T) {
		result := td.Execute(newToolContext(), map[string]any{"paths": []any{}})
		if !strings.Contains(result.Content, "non-empty") {
			t.Errorf("expected non-empty error, got: %q", result.Content)
		}
	})
}

// ---------------------------------------------------------------------------
// read_file
// ---------------------------------------------------------------------------

func TestReadFile(t *testing.T) {
	td := ReadFile()
	if td.Name != "read_file" {
		t.Fatalf("unexpected name: %s", td.Name)
	}

	t.Run("reads existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "hello.txt")
		if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}
		result := td.Execute(newToolContext(), map[string]any{"path": path})
		if result.Content != "hello world" {
			t.Errorf("unexpected content: %q", result.Content)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		result := td.Execute(newToolContext(), map[string]any{"path": "/nonexistent/file.txt"})
		if !strings.Contains(result.Content, "Error reading file") {
			t.Errorf("expected error, got: %q", result.Content)
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		result := td.Execute(newToolContext(), map[string]any{})
		if !strings.Contains(result.Content, "Error: path is required") {
			t.Errorf("expected error, got: %q", result.Content)
		}
	})
}

// ---------------------------------------------------------------------------
// git_log
// ---------------------------------------------------------------------------

func TestGitLog(t *testing.T) {
	// Check that git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	td := GitLog()
	if td.Name != "git_log" {
		t.Fatalf("unexpected name: %s", td.Name)
	}

	t.Run("valid repo", func(t *testing.T) {
		dir := t.TempDir()
		// Init a git repo and make a commit
		run := func(args ...string) {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=test",
				"GIT_AUTHOR_EMAIL=test@test.com",
				"GIT_COMMITTER_NAME=test",
				"GIT_COMMITTER_EMAIL=test@test.com",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v failed: %v\n%s", args, err, out)
			}
		}
		run("init")
		run("commit", "--allow-empty", "-m", "initial commit")
		run("commit", "--allow-empty", "-m", "second commit")

		result := td.Execute(newToolContext(), map[string]any{"dir": dir, "count": "1"})
		if !strings.Contains(result.Content, "second commit") {
			t.Errorf("expected 'second commit' in output, got: %q", result.Content)
		}
	})

	t.Run("invalid dir", func(t *testing.T) {
		result := td.Execute(newToolContext(), map[string]any{"dir": "/nonexistent"})
		if !strings.Contains(result.Content, "Error") {
			t.Errorf("expected error, got: %q", result.Content)
		}
	})

	t.Run("default count", func(t *testing.T) {
		dir := t.TempDir()
		cmd := exec.Command("git", "init")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init failed: %v\n%s", err, out)
		}
		cmd = exec.Command("git", "commit", "--allow-empty", "-m", "test")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit failed: %v\n%s", err, out)
		}
		// No count param — should default to 20
		result := td.Execute(newToolContext(), map[string]any{"dir": dir})
		if !strings.Contains(result.Content, "test") {
			t.Errorf("expected commit message in output, got: %q", result.Content)
		}
	})
}

// ---------------------------------------------------------------------------
// All()
// ---------------------------------------------------------------------------

func TestAll(t *testing.T) {
	cfg := Config{
		MemoURL:    "http://localhost:8081",
		SearXNGURL: "http://localhost:8082",
	}
	m := All(cfg)

	expected := []string{
		"repo_overview", "read_files",
		"read_file", "git_log",
		"fetch_page", "search",
		"recall",
		"list_dir",
	}

	notExpected := []string{"aurelia_status", "aurelia_show", "lamina", "submit_draft", "read_post", "list_posts"}
	for _, name := range notExpected {
		if _, ok := m[name]; ok {
			t.Errorf("workspace-specific tool %q should not be wired", name)
		}
	}

	for _, name := range expected {
		if _, ok := m[name]; !ok {
			t.Errorf("missing tool: %s", name)
		}
	}

	if len(m) != len(expected) {
		t.Errorf("expected %d tools, got %d", len(expected), len(m))
	}
}

// ---------------------------------------------------------------------------
// Config-dependent error paths
// ---------------------------------------------------------------------------

func TestIsGitHubRepo(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"benaskins/axon", true},
		{"https://github.com/benaskins/axon", true},
		{"https://github.com/benaskins/axon.git", true},
		{"https://github.com/benaskins/axon/tree/main/stream", true},
		{"/Users/benaskins/dev/lamina/axon", false},
		{"./relative/path", false},
		{"just-a-name", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isGitHubRepo(tt.input); got != tt.want {
				t.Errorf("isGitHubRepo(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeGitHubRepo(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"benaskins/axon", "benaskins/axon"},
		{"https://github.com/benaskins/axon", "benaskins/axon"},
		{"https://github.com/benaskins/axon.git", "benaskins/axon"},
		{"https://github.com/benaskins/axon/tree/main/stream", "benaskins/axon"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeGitHubRepo(tt.input); got != tt.want {
				t.Errorf("normalizeGitHubRepo(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeBase64Content(t *testing.T) {
	encoded := "SGVsbG8gV29ybGQ="  // "Hello World"
	got, err := decodeBase64Content(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got != "Hello World" {
		t.Errorf("got %q, want %q", got, "Hello World")
	}
}

func TestRepoOverview_BackwardCompatDir(t *testing.T) {
	tmp := t.TempDir()
	exec.Command("git", "init", tmp).Run()
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# Test"), 0o644)
	exec.Command("git", "-C", tmp, "add", ".").Run()
	exec.Command("git", "-C", tmp, "commit", "-m", "init").Run()

	td := RepoOverview()
	// Use "dir" param (old name) for backward compatibility
	result := td.Execute(newToolContext(), map[string]any{"dir": tmp})
	if !strings.Contains(result.Content, "## README.md") {
		t.Error("backward compat with 'dir' param failed")
	}
}

func TestSearchNoURL(t *testing.T) {
	td := Search("")
	result := td.Execute(newToolContext(), map[string]any{"query": "test"})
	if !strings.Contains(result.Content, "not configured") {
		t.Errorf("expected config error, got: %q", result.Content)
	}
}

func TestRecallNoURL(t *testing.T) {
	td := Recall("")
	result := td.Execute(newToolContext(), map[string]any{"query": "test"})
	if !strings.Contains(result.Content, "not configured") {
		t.Errorf("expected config error, got: %q", result.Content)
	}
}

