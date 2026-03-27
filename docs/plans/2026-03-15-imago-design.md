# Imago Design

Imago is an interactive CLI that produces blog posts from structured interviews. It runs as a Bubble Tea TUI, uses multiple LLM backends, and saves finished drafts to disk.

## What it does

Two modes, same two-phase flow.

**`imago`** (interview mode). You run it when you want to write a post. It interviews you using a research journalist persona, can use tools mid-conversation to look things up, then produces a draft for section-by-section review.

**`imago weekly`** (weekly update mode). Collects git activity across local and remote repos, scans for new sites, then interviews you about the week's work with the activity report as context. Produces a weekly update post for generativeplane.com.

Both modes follow the same pipeline: interview, draft, section review, final review, save to disk.

## Architecture

```
imago (CLI binary)
├── cmd/imago          entry point, client selection, mode dispatch
├── internal/tui       Bubble Tea reactive terminal UI (interview, draft, review phases)
├── internal/config    model config, system prompts, draft prompts
├── internal/session   JSON session persistence (~/.local/share/imago/sessions/)
├── internal/collect   git activity collection for weekly mode
├── internal/logging   structured logging setup
└── tools/             axon-tool definitions for research and publishing
```

### Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| axon-loop | v0.6.0 | Conversation loop with streaming and tool dispatch |
| axon-talk | v0.4.2 | Ollama, Cloudflare Workers AI, and Anthropic adapters |
| axon-tool | v0.1.6 | Tool definition primitives, PageFetcher |
| axon-wire | v0.0.0 | HTTP client that routes through wire proxy |
| bubbletea | v1.3.10 | Reactive terminal UI framework |
| lipgloss | v1.1.1 | Terminal styling |
| glamour | v1.0.0 | Markdown rendering in terminal |
| bubbles | v1.0.0 | textarea, viewport components |

axon-talk, axon-tool, and axon-wire use local `replace` directives pointing to `/Users/benaskins/dev/lamina/`.

## LLM clients

Three providers, selected by environment:

| Provider | Env vars required | Used for |
|----------|-------------------|----------|
| **Ollama** | (default fallback) | Regular interview mode, local inference |
| **Cloudflare Workers AI** | `CLOUDFLARE_ACCOUNT_ID` + `CLOUDFLARE_AXON_GATE_TOKEN` | Regular mode via axon-gate gateway |
| **Anthropic (via CF AI Gateway)** | `CLOUDFLARE_ACCOUNT_ID` + `CLOUDFLARE_AI_GATEWAY_TOKEN` | Weekly mode, Opus for both phases |
| **Anthropic (direct)** | `ANTHROPIC_API_KEY` | Weekly mode fallback (no gateway) |

### Model config

| Mode | Interview model | Draft model |
|------|----------------|-------------|
| Regular (Ollama) | qwen3:32b | qwen3:32b |
| Regular (Cloudflare) | @cf/qwen/qwen3-30b-a3b-fp8 | @cf/qwen/qwen3-30b-a3b-fp8 |
| Weekly | claude-opus-4-6 (configurable via `IMAGO_DRAFT_MODEL`) | same |

## TUI

Bubble Tea TUI with three phases:

**Interview phase.** Conversation view with imago's questions and your responses, styled with Lip Gloss. Tool use appears as collapsed lines (e.g. `↳ search query=...`) expandable with Tab. Text input at the bottom. Type `/draft` to transition.

**Draft phase.** LLM generates a full draft from the interview transcript. The draft is split into sections by markdown headings. Each section rendered via Glamour. Controls: `/keep` to approve, or type feedback to revise. Revision uses a separate conversation with full context (interview transcript, full draft, current section). When all sections are approved, transitions to review.

**Review phase.** Full assembled article rendered in a bordered viewport. Type feedback for whole-article revisions, `/done` to save. Draft is written to `~/Documents/imago/<slug>.md`.

## Tools

15 tools defined in `tools/tools.go`:

| Tool | Purpose |
|------|---------|
| `repo_overview` | Full repo overview: tree, commits, key docs. Works with local paths and GitHub repos |
| `read_files` | Read multiple files in a single call |
| `read_file` | Read a single local file |
| `list_dir` | List directory contents |
| `git_log` | Recent commit history for a repo |
| `read_post` | Read an existing post from the site directory |
| `list_posts` | List published posts on the site |
| `fetch_page` | Fetch and extract content from a URL (routes through axon-wire proxy) |
| `search` | Search the web via SearXNG |
| `aurelia_status` | Query running services via aurelia CLI |
| `aurelia_show` | Get details about a specific aurelia service |
| `lamina` | Run lamina CLI commands |
| `submit_draft` | Submit finished draft to synd server API |
| `recall` | Recall memories from axon-memo |
| `research` | Dispatch parallel URL fetches via research worker (conditional on `AXON_DISPATCH_URL`) |

### Tool config (environment)

| Env var | Purpose |
|---------|---------|
| `SYND_SITE_DIR` | Path to generativeplane.com site directory |
| `SYND_SERVICE_URL` | Synd server base URL |
| `~/.config/synd/token` | Auth token for synd API (read from file) |
| `MEMO_SERVICE_URL` | axon-memo service URL |
| `SEARXNG_URL` | SearXNG instance URL |
| `AXON_DISPATCH_URL` | Research dispatch worker URL |
| `AXON_WIRE_TOKEN` | Shared auth token for wire proxy / dispatch |
| `AXON_WIRE_URL` | Wire proxy URL (used by axon-wire HTTP client) |

## Session persistence

Sessions are saved as JSON to `~/.local/share/imago/sessions/<timestamp>.json` after every turn. On startup, imago checks for an incomplete session matching the current mode and offers to resume.

Session state includes: messages, phase, sections, approved flags, session kind (post/weekly).

## Weekly collection

The `collect` package (`internal/collect/`) gathers git activity for weekly updates:

1. Scans `~/dev` (or `$DEV`) for git repos up to 4 levels deep
2. SSHs to `hestia` to scan `~/dev` there
3. Merges and deduplicates repos by name across machines
4. Detects new repos and new sites created since the last weekly post
5. Derives the "since" date from the most recent `weekly-*.md` file in the site directory
6. Produces a structured markdown report injected into the weekly system prompt

## System prompts

### Regular mode
Research journalist persona. Interviews the builder, researches projects mid-conversation using tools, asks one question at a time, pushes back on generic answers. 8-10 substantive exchanges before suggesting draft transition.

### Weekly mode
Same journalist persona but with the activity report pre-loaded. Knows what happened, asks about why it matters. Priority repos (lamina, aurelia, axon-*, public sites) discussed first. Editorial direction: group by theme not repo, connect to axon, highlight [NEW] repos and sites.

### Draft prompts
Both modes produce first-person blog posts in the subject's voice. Weekly mode adds structure requirements (opening reflection, themed sections, closing editorial) and linking rules (GitHub repos, sites, external tools).

## What's not built yet

From the original design, these are not yet implemented:

- **axon-memo integration**: `recall` tool is defined but editorial memory (voice development loop, keep signals, corrections, consolidation) is not wired up
- **axon-task / claude research**: the `claude` tool for spawning read-only Claude Code sessions is not implemented
- **synd submission from TUI**: `submit_draft` tool exists but the review phase saves to disk rather than submitting via API
- **Phase-specific model switching** in regular mode: both phases currently use the same model (draft model override only works in weekly mode)
- `internal/draft/` and `internal/interview/` directories exist but are empty
