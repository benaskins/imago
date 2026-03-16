// Package tools defines all tool.ToolDef implementations for imago.
// These tools give the journalist agent access to local files, git history,
// web research, infrastructure status, publishing, and editorial memory.
package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"

	tool "github.com/benaskins/axon-tool"
)

// Config holds external configuration for tools that depend on
// environment-specific URLs, paths, or credentials.
type Config struct {
	SiteDir     string       // path to generativeplane.com site directory
	SyndURL     string       // synd server base URL
	SyndToken   string       // auth token for synd API
	MemoURL     string       // axon-memo service URL
	SearXNGURL  string       // SearXNG instance URL
	DispatchURL string       // research dispatch worker URL
	WireToken   string       // shared auth token for wire proxy / dispatch
	HTTPClient  *http.Client // optional custom HTTP client for outbound requests
}

// All returns the complete tool map for imago, keyed by tool name.
func All(cfg Config) map[string]tool.ToolDef {
	var fetchOpts []tool.PageFetcherOption
	var searxOpts []tool.SearXNGOption
	if cfg.HTTPClient != nil {
		fetchOpts = append(fetchOpts, tool.WithHTTPClient(cfg.HTTPClient))
		searxOpts = append(searxOpts, tool.WithSearXNGHTTPClient(cfg.HTTPClient))
	}

	defs := []tool.ToolDef{
		RepoOverview(),
		ReadFiles(),
		ReadFile(),
		GitLog(),
		ReadPost(cfg.SiteDir),
		ListPosts(cfg.SiteDir),
		FetchPage(fetchOpts...),
		Search(cfg.SearXNGURL, searxOpts...),
		AureliaStatus(),
		AureliaShow(),
		Lamina(),
		SubmitDraft(cfg.SyndURL, cfg.SyndToken),
		Recall(cfg.MemoURL),
		ListDir(),
	}

	if cfg.DispatchURL != "" {
		defs = append(defs, ResearchDispatch(cfg.DispatchURL, cfg.WireToken))
	}

	m := make(map[string]tool.ToolDef)
	for _, td := range defs {
		m[td.Name] = td
	}
	return m
}

// ---------------------------------------------------------------------------
// Repository exploration
// ---------------------------------------------------------------------------

// RepoOverview returns a tool that gives a full overview of a repository
// in a single call: directory tree, recent commits, and key docs.
func RepoOverview() tool.ToolDef {
	return tool.ToolDef{
		Name:        "repo_overview",
		Description: "Get a full overview of a repository in one call. Returns directory tree (2 levels deep), last 10 commits, and contents of key docs (README.md, CLAUDE.md, AGENTS.md). Use this instead of multiple list_dir/read_file/git_log calls when exploring a new repo.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"dir"},
			Properties: map[string]tool.PropertySchema{
				"dir": {
					Type:        "string",
					Description: "Path to the repository root directory.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			dir, _ := args["dir"].(string)
			if dir == "" {
				return tool.ToolResult{Content: "Error: dir is required."}
			}

			var sb strings.Builder

			// Directory tree (2 levels)
			sb.WriteString("## Directory tree\n\n")
			if tree, err := dirTree(dir, 2, ""); err == nil {
				sb.WriteString(tree)
			} else {
				sb.WriteString(fmt.Sprintf("Error: %v\n", err))
			}

			// Recent commits
			sb.WriteString("\n## Recent commits\n\n")
			cmd := exec.CommandContext(ctx.Ctx, "git", "log", "--oneline", "-10")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err == nil {
				sb.WriteString(string(out))
			} else {
				sb.WriteString("(not a git repository or git error)\n")
			}

			// Key documentation files
			keyDocs := []string{"README.md", "CLAUDE.md", "AGENTS.md"}
			for _, name := range keyDocs {
				path := filepath.Join(dir, name)
				if data, err := os.ReadFile(path); err == nil {
					sb.WriteString(fmt.Sprintf("\n## %s\n\n", name))
					content := string(data)
					// Truncate very long docs
					if len(content) > 3000 {
						content = content[:3000] + "\n\n... (truncated)"
					}
					sb.WriteString(content)
					sb.WriteString("\n")
				}
			}

			return tool.ToolResult{Content: sb.String()}
		},
	}
}

// dirTree builds a text representation of a directory tree up to maxDepth.
func dirTree(root string, maxDepth int, prefix string) (string, error) {
	if maxDepth < 0 {
		return "", nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	// Filter out hidden dirs and common noise
	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}
		visible = append(visible, e)
	}

	for i, e := range visible {
		connector := "├── "
		childPrefix := prefix + "│   "
		if i == len(visible)-1 {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		if e.IsDir() {
			sb.WriteString(prefix + connector + e.Name() + "/\n")
			if maxDepth > 0 {
				child, _ := dirTree(filepath.Join(root, e.Name()), maxDepth-1, childPrefix)
				sb.WriteString(child)
			}
		} else {
			sb.WriteString(prefix + connector + e.Name() + "\n")
		}
	}

	return sb.String(), nil
}

// ReadFiles returns a tool that reads multiple files in a single call.
// Limited to 5 files per request to keep context manageable.
func ReadFiles() tool.ToolDef {
	return tool.ToolDef{
		Name:        "read_files",
		Description: "Read up to 5 files in a single call. Use after repo_overview to dig into specific files without burning multiple tool calls.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"paths"},
			Properties: map[string]tool.PropertySchema{
				"paths": {
					Type:        "array",
					Description: "List of file paths to read (max 5). Each element is a string path.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			rawPaths, ok := args["paths"].([]any)
			if !ok || len(rawPaths) == 0 {
				return tool.ToolResult{Content: "Error: paths must be a non-empty array of strings."}
			}
			if len(rawPaths) > 5 {
				return tool.ToolResult{Content: "Error: max 5 files per request."}
			}

			var sb strings.Builder
			for i, raw := range rawPaths {
				path, ok := raw.(string)
				if !ok || path == "" {
					sb.WriteString(fmt.Sprintf("## [%d] (invalid path)\n\nError: not a string.\n\n", i+1))
					continue
				}
				sb.WriteString(fmt.Sprintf("## %s\n\n", path))
				data, err := os.ReadFile(path)
				if err != nil {
					sb.WriteString(fmt.Sprintf("Error: %v\n\n", err))
					continue
				}
				content := string(data)
				if len(content) > 10000 {
					content = content[:10000] + "\n\n... (truncated at 10000 chars)"
				}
				sb.WriteString(content)
				sb.WriteString("\n\n")
			}
			return tool.ToolResult{Content: sb.String()}
		},
	}
}

// ---------------------------------------------------------------------------
// File / code tools
// ---------------------------------------------------------------------------

// ReadFile returns a tool that reads a local file and returns its contents.
func ReadFile() tool.ToolDef {
	return tool.ToolDef{
		Name:        "read_file",
		Description: "Read a local file and return its contents. Use for reading source code, config files, design docs, or any text file on disk.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]tool.PropertySchema{
				"path": {
					Type:        "string",
					Description: "Absolute or relative path to the file to read.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			path, _ := args["path"].(string)
			if path == "" {
				return tool.ToolResult{Content: "Error: path is required."}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error reading file: %v", err)}
			}
			return tool.ToolResult{Content: string(data)}
		},
	}
}

// ListDir returns a tool that lists the contents of a directory.
func ListDir() tool.ToolDef {
	return tool.ToolDef{
		Name:        "list_dir",
		Description: "List the contents of a directory. Returns file and directory names with type indicators.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]tool.PropertySchema{
				"path": {
					Type:        "string",
					Description: "Absolute path to the directory to list.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			path, _ := args["path"].(string)
			if path == "" {
				return tool.ToolResult{Content: "Error: path is required."}
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error reading directory: %v", err)}
			}
			var sb strings.Builder
			for _, e := range entries {
				if e.IsDir() {
					sb.WriteString(e.Name() + "/\n")
				} else {
					sb.WriteString(e.Name() + "\n")
				}
			}
			if sb.Len() == 0 {
				return tool.ToolResult{Content: "(empty directory)"}
			}
			return tool.ToolResult{Content: sb.String()}
		},
	}
}

// GitLog returns a tool that runs git log in a given directory.
func GitLog() tool.ToolDef {
	return tool.ToolDef{
		Name:        "git_log",
		Description: "Show recent git commit history for a repository. Returns one-line summaries of recent commits.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"dir"},
			Properties: map[string]tool.PropertySchema{
				"dir": {
					Type:        "string",
					Description: "Path to the git repository directory.",
				},
				"count": {
					Type:        "string",
					Description: "Number of commits to show (default: 20).",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			dir, _ := args["dir"].(string)
			if dir == "" {
				return tool.ToolResult{Content: "Error: dir is required."}
			}
			count, _ := args["count"].(string)
			if count == "" {
				count = "20"
			}
			cmd := exec.CommandContext(ctx.Ctx, "git", "log", "--oneline", "-"+count)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error running git log: %v\n%s", err, string(out))}
			}
			return tool.ToolResult{Content: string(out)}
		},
	}
}

// ReadPost returns a tool that reads a published post by ID from the site directory.
// It reads the HTML file, strips tags, and returns the text content.
func ReadPost(siteDir string) tool.ToolDef {
	return tool.ToolDef{
		Name:        "read_post",
		Description: "Read a published post from generativeplane.com. Reads the HTML, strips tags, and returns the text content. Use 'latest' to read the most recently published post.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"post_id"},
			Properties: map[string]tool.PropertySchema{
				"post_id": {
					Type:        "string",
					Description: "The post UUID, or 'latest' for the most recent post.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			postID, _ := args["post_id"].(string)
			if postID == "" {
				return tool.ToolResult{Content: "Error: post_id is required."}
			}

			dir := siteDir
			if dir == "" {
				return tool.ToolResult{Content: "Error: site directory not configured."}
			}

			postsDir := filepath.Join(dir, "posts")

			if postID == "latest" {
				resolved, err := latestPostID(postsDir)
				if err != nil {
					return tool.ToolResult{Content: fmt.Sprintf("Error finding latest post: %v", err)}
				}
				postID = resolved
			}

			htmlPath := filepath.Join(postsDir, postID, "index.html")
			data, err := os.ReadFile(htmlPath)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error reading post: %v", err)}
			}

			text, err := stripHTML(string(data))
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error stripping HTML: %v", err)}
			}

			return tool.ToolResult{Content: fmt.Sprintf("Post %s:\n\n%s", postID, text)}
		},
	}
}

// ListPosts returns a tool that lists published posts in the site directory.
func ListPosts(siteDir string) tool.ToolDef {
	return tool.ToolDef{
		Name:        "list_posts",
		Description: "List published posts on generativeplane.com. Returns post IDs and titles.",
		Parameters: tool.ParameterSchema{
			Type:       "object",
			Properties: map[string]tool.PropertySchema{},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			dir := siteDir
			if dir == "" {
				return tool.ToolResult{Content: "Error: site directory not configured."}
			}

			postsDir := filepath.Join(dir, "posts")
			entries, err := os.ReadDir(postsDir)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error reading posts directory: %v", err)}
			}

			type postInfo struct {
				id      string
				title   string
				modTime time.Time
			}
			var posts []postInfo

			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				id := e.Name()
				htmlPath := filepath.Join(postsDir, id, "index.html")
				title := extractTitle(htmlPath)
				info, err := e.Info()
				var modTime time.Time
				if err == nil {
					modTime = info.ModTime()
				}
				posts = append(posts, postInfo{id: id, title: title, modTime: modTime})
			}

			sort.Slice(posts, func(i, j int) bool {
				return posts[i].modTime.After(posts[j].modTime)
			})

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Published posts (%d):\n\n", len(posts)))
			for _, p := range posts {
				sb.WriteString(fmt.Sprintf("- %s  %s\n", p.id, p.title))
			}
			return tool.ToolResult{Content: sb.String()}
		},
	}
}

// ---------------------------------------------------------------------------
// Web tools
// ---------------------------------------------------------------------------

// FetchPage returns a tool that fetches a URL and returns extracted text.
// It uses axon-tool's PageFetcher without LLM extraction (raw text mode).
func FetchPage(opts ...tool.PageFetcherOption) tool.ToolDef {
	return tool.ToolDef{
		Name:        "fetch_page",
		Description: "Fetch a web page and return its extracted text content. Use to read articles, documentation, or any web page.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"url"},
			Properties: map[string]tool.PropertySchema{
				"url": {
					Type:        "string",
					Description: "The URL of the web page to fetch.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			urlStr, _ := args["url"].(string)
			if urlStr == "" {
				return tool.ToolResult{Content: "Error: url is required."}
			}
			fetcher := tool.NewPageFetcher(nil, opts...) // no LLM extraction
			text, err := fetcher.FetchAndExtract(ctx.Ctx, urlStr, "")
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error fetching page: %v", err)}
			}
			return tool.ToolResult{Content: text}
		},
	}
}

// Search returns a tool that searches the web via SearXNG.
func Search(searxngURL string, opts ...tool.SearXNGOption) tool.ToolDef {
	return tool.ToolDef{
		Name:        "search",
		Description: "Search the web using SearXNG. Returns titles, URLs, and snippets for the top results.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]tool.PropertySchema{
				"query": {
					Type:        "string",
					Description: "The search query.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			query, _ := args["query"].(string)
			if query == "" {
				return tool.ToolResult{Content: "Error: query is required."}
			}
			if searxngURL == "" {
				return tool.ToolResult{Content: "Error: SearXNG URL not configured."}
			}
			client := tool.NewSearXNGClient(searxngURL, opts...)
			results, err := client.Search(ctx.Ctx, query, 5)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Search failed: %v", err)}
			}
			if len(results) == 0 {
				return tool.ToolResult{Content: fmt.Sprintf("No results found for %q.", query)}
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Search results for %q:\n\n", query))
			for i, r := range results {
				sb.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
			}
			return tool.ToolResult{Content: sb.String()}
		},
	}
}

// ---------------------------------------------------------------------------
// Infrastructure tools
// ---------------------------------------------------------------------------

// AureliaStatus returns a tool that runs `aurelia status`.
func AureliaStatus() tool.ToolDef {
	return tool.ToolDef{
		Name:        "aurelia_status",
		Description: "Check the status of all services managed by aurelia. Shows running/stopped state, health, and uptime.",
		Parameters: tool.ParameterSchema{
			Type:       "object",
			Properties: map[string]tool.PropertySchema{},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			cmd := exec.CommandContext(ctx.Ctx, "aurelia", "status")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error running aurelia status: %v\n%s", err, string(out))}
			}
			return tool.ToolResult{Content: string(out)}
		},
	}
}

// AureliaShow returns a tool that runs `aurelia show <service>`.
func AureliaShow() tool.ToolDef {
	return tool.ToolDef{
		Name:        "aurelia_show",
		Description: "Get detailed information about a specific service managed by aurelia, including config, health check details, and dependencies.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"service"},
			Properties: map[string]tool.PropertySchema{
				"service": {
					Type:        "string",
					Description: "The name of the service to inspect.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			service, _ := args["service"].(string)
			if service == "" {
				return tool.ToolResult{Content: "Error: service is required."}
			}
			cmd := exec.CommandContext(ctx.Ctx, "aurelia", "show", service)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error running aurelia show: %v\n%s", err, string(out))}
			}
			return tool.ToolResult{Content: string(out)}
		},
	}
}

// Lamina returns a tool that runs a lamina CLI command.
func Lamina() tool.ToolDef {
	return tool.ToolDef{
		Name:        "lamina",
		Description: "Run a lamina workspace management command. Supports commands like 'repo status', 'deps', 'doctor', 'test', etc.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"args"},
			Properties: map[string]tool.PropertySchema{
				"args": {
					Type:        "string",
					Description: "The lamina command arguments, e.g. 'repo status', 'deps', 'doctor'.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			cmdArgs, _ := args["args"].(string)
			if cmdArgs == "" {
				return tool.ToolResult{Content: "Error: args is required."}
			}
			parts := strings.Fields(cmdArgs)
			cmd := exec.CommandContext(ctx.Ctx, "lamina", parts...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error running lamina: %v\n%s", err, string(out))}
			}
			return tool.ToolResult{Content: string(out)}
		},
	}
}

// ---------------------------------------------------------------------------
// Publishing tools
// ---------------------------------------------------------------------------

// SubmitDraft returns a tool that POSTs a draft to the synd server API.
func SubmitDraft(syndURL, syndToken string) tool.ToolDef {
	return tool.ToolDef{
		Name:        "submit_draft",
		Description: "Submit a finished draft to the synd publishing pipeline. Creates a draft post that will go through the approval flow (Signal notification, review, publish, deploy).",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"title", "body"},
			Properties: map[string]tool.PropertySchema{
				"title": {
					Type:        "string",
					Description: "The post title.",
				},
				"body": {
					Type:        "string",
					Description: "The post body in Markdown.",
				},
				"abstract": {
					Type:        "string",
					Description: "A short abstract or summary of the post.",
				},
				"tags": {
					Type:        "string",
					Description: "Comma-separated list of tags.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			if syndURL == "" {
				return tool.ToolResult{Content: "Error: synd service URL not configured."}
			}
			if syndToken == "" {
				return tool.ToolResult{Content: "Error: synd auth token not configured."}
			}

			title, _ := args["title"].(string)
			body, _ := args["body"].(string)
			abstract, _ := args["abstract"].(string)
			tagsStr, _ := args["tags"].(string)

			if title == "" {
				return tool.ToolResult{Content: "Error: title is required."}
			}
			if body == "" {
				return tool.ToolResult{Content: "Error: body is required."}
			}

			var tags []string
			if tagsStr != "" {
				for _, t := range strings.Split(tagsStr, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}

			payload := map[string]any{
				"kind":  "long",
				"title": title,
				"body":  body,
			}
			if abstract != "" {
				payload["abstract"] = abstract
			}
			if len(tags) > 0 {
				payload["tags"] = tags
			}

			jsonData, err := json.Marshal(payload)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error encoding payload: %v", err)}
			}

			url := strings.TrimRight(syndURL, "/") + "/api/posts"
			req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodPost, url, bytes.NewReader(jsonData))
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error creating request: %v", err)}
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+syndToken)

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error submitting draft: %v", err)}
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return tool.ToolResult{Content: fmt.Sprintf("Synd API returned %d: %s", resp.StatusCode, string(respBody))}
			}

			return tool.ToolResult{Content: fmt.Sprintf("Draft submitted successfully.\n%s", string(respBody))}
		},
	}
}

// ---------------------------------------------------------------------------
// Memory tools
// ---------------------------------------------------------------------------

// Recall returns a tool that queries axon-memo's recall API for editorial memories.
func Recall(memoURL string) tool.ToolDef {
	return tool.ToolDef{
		Name:        "recall",
		Description: "Recall editorial memories from past interviews. Searches axon-memo for relevant memories about writing style, editorial preferences, past topics, and corrections.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]tool.PropertySchema{
				"query": {
					Type:        "string",
					Description: "What to recall — a topic, style question, or editorial preference.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			query, _ := args["query"].(string)
			if query == "" {
				return tool.ToolResult{Content: "Error: query is required."}
			}
			if memoURL == "" {
				return tool.ToolResult{Content: "Error: memo service URL not configured."}
			}

			url := strings.TrimRight(memoURL, "/") + "/api/recall"
			payload := map[string]any{
				"agent_slug": "imago",
				"query":      query,
			}
			jsonData, err := json.Marshal(payload)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error encoding request: %v", err)}
			}

			req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodPost, url, bytes.NewReader(jsonData))
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error creating request: %v", err)}
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error recalling memories: %v", err)}
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

			if resp.StatusCode != http.StatusOK {
				slog.Error("recall API error", "status", resp.StatusCode, "body", string(respBody))
				return tool.ToolResult{Content: fmt.Sprintf("Recall failed (HTTP %d): %s", resp.StatusCode, string(respBody))}
			}

			return tool.ToolResult{Content: string(respBody)}
		},
	}
}

// ---------------------------------------------------------------------------
// Cloud research tools
// ---------------------------------------------------------------------------

// ResearchDispatch returns a tool that fans out multiple URL fetches in
// parallel via the Cloudflare research-dispatch worker. Use when the agent
// needs to fetch several pages at once — one call instead of sequential
// fetch_page calls.
func ResearchDispatch(dispatchURL, wireToken string) tool.ToolDef {
	return tool.ToolDef{
		Name:        "research",
		Description: "Fetch multiple web pages in parallel via the cloud research worker. Use when you need to read several URLs at once — much faster than calling fetch_page repeatedly. Returns all results in one response. Optionally summarise each page.",
		Parameters: tool.ParameterSchema{
			Type:     "object",
			Required: []string{"urls"},
			Properties: map[string]tool.PropertySchema{
				"urls": {
					Type:        "array",
					Description: "List of URLs to fetch in parallel (max 20).",
				},
				"summarize": {
					Type:        "string",
					Description: "Set to 'true' to get AI-generated summaries of each page alongside the raw content.",
				},
			},
		},
		Execute: func(ctx *tool.ToolContext, args map[string]any) tool.ToolResult {
			rawURLs, ok := args["urls"].([]any)
			if !ok || len(rawURLs) == 0 {
				return tool.ToolResult{Content: "Error: urls must be a non-empty array of strings."}
			}
			if len(rawURLs) > 20 {
				return tool.ToolResult{Content: "Error: max 20 URLs per dispatch."}
			}

			summarize := false
			if s, ok := args["summarize"].(string); ok && s == "true" {
				summarize = true
			}

			type task struct {
				URL       string `json:"url"`
				Summarize bool   `json:"summarize,omitempty"`
			}
			tasks := make([]task, 0, len(rawURLs))
			for _, raw := range rawURLs {
				u, ok := raw.(string)
				if !ok || u == "" {
					continue
				}
				tasks = append(tasks, task{URL: u, Summarize: summarize})
			}

			payload, err := json.Marshal(map[string]any{"tasks": tasks})
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error encoding request: %v", err)}
			}

			url := strings.TrimRight(dispatchURL, "/") + "/dispatch"
			req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodPost, url, bytes.NewReader(payload))
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error creating request: %v", err)}
			}
			req.Header.Set("Content-Type", "application/json")
			if wireToken != "" {
				req.Header.Set("X-Wire-Token", wireToken)
			}

			client := &http.Client{Timeout: 60 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error dispatching research: %v", err)}
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

			if resp.StatusCode != http.StatusOK {
				return tool.ToolResult{Content: fmt.Sprintf("Dispatch returned %d: %s", resp.StatusCode, string(respBody))}
			}

			var result struct {
				Results []struct {
					URL     string `json:"url"`
					Status  int    `json:"status"`
					Body    string `json:"body"`
					Summary string `json:"summary,omitempty"`
					Error   string `json:"error,omitempty"`
				} `json:"results"`
			}
			if err := json.Unmarshal(respBody, &result); err != nil {
				return tool.ToolResult{Content: fmt.Sprintf("Error decoding response: %v", err)}
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Fetched %d pages:\n\n", len(result.Results)))
			for _, r := range result.Results {
				sb.WriteString(fmt.Sprintf("## %s (HTTP %d)\n\n", r.URL, r.Status))
				if r.Error != "" {
					sb.WriteString(fmt.Sprintf("Error: %s\n\n", r.Error))
					continue
				}
				if r.Summary != "" {
					sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", r.Summary))
				}
				content := r.Body
				if len(content) > 5000 {
					content = content[:5000] + "\n\n... (truncated)"
				}
				sb.WriteString(content)
				sb.WriteString("\n\n")
			}

			return tool.ToolResult{Content: sb.String()}
		},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// stripHTML tokenizes HTML and extracts text content.
func stripHTML(s string) (string, error) {
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var sb strings.Builder
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			err := tokenizer.Err()
			if err == io.EOF {
				return strings.TrimSpace(sb.String()), nil
			}
			return "", err
		case html.TextToken:
			text := strings.TrimSpace(tokenizer.Token().Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString("\n")
			}
		}
	}
}

// extractTitle reads an HTML file and returns the content of the <title> tag,
// stripping any site suffix like " — Generative Plane".
func extractTitle(htmlPath string) string {
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		return "(untitled)"
	}
	tokenizer := html.NewTokenizer(strings.NewReader(string(data)))
	inTitle := false
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return "(untitled)"
		case html.StartTagToken:
			tn, _ := tokenizer.TagName()
			if string(tn) == "title" {
				inTitle = true
			}
		case html.TextToken:
			if inTitle {
				title := strings.TrimSpace(tokenizer.Token().Data)
				// Strip site suffix
				if idx := strings.Index(title, " — "); idx > 0 {
					title = title[:idx]
				}
				// Also try ASCII dash
				if idx := strings.Index(title, " - "); idx > 0 {
					title = title[:idx]
				}
				return title
			}
		}
	}
}

// latestPostID finds the most recently modified post directory.
func latestPostID(postsDir string) (string, error) {
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		return "", fmt.Errorf("reading posts directory: %w", err)
	}

	var latestID string
	var latestTime time.Time

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestID = e.Name()
		}
	}

	if latestID == "" {
		return "", fmt.Errorf("no posts found")
	}
	return latestID, nil
}
