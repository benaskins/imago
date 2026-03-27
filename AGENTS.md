# imago

Interactive CLI that produces blog posts from structured interviews. Bubble Tea TUI with three phases: interview, section-by-section draft review, and final article review.

## Build & Test

```bash
go test ./...
go vet ./...
just build     # builds to bin/imago
just install   # copies to ~/.local/bin/imago
```

## Modes

- `imago`: interview mode. Uses Ollama or Cloudflare Workers AI. Research journalist interviews you, generates a draft, you review section by section.
- `imago weekly`: weekly update mode. Collects git activity from local + hestia, interviews you with the data, produces a weekly post for generativeplane.com. Uses Anthropic (Opus) via Cloudflare AI Gateway.

## Structure

```
cmd/imago/main.go       entry point, LLM client selection, mode dispatch
internal/tui/           Bubble Tea model, interview/draft/review phases
internal/config/        model config, system prompts, draft prompts
internal/session/       JSON session persistence (~/.local/share/imago/sessions/)
internal/collect/       git activity collection for weekly mode
internal/logging/       structured logging
tools/tools.go          15 axon-tool definitions (repo_overview, search, fetch_page, etc.)
```

## Key dependencies

- axon-loop (conversation loop + streaming + tool dispatch)
- axon-talk (Ollama, Cloudflare Workers AI, Anthropic adapters)
- axon-tool (tool definitions, PageFetcher)
- axon-wire (HTTP client routing through wire proxy)
- bubbletea, lipgloss, glamour, bubbles (TUI)

All axon modules use local `replace` directives to `/Users/benaskins/dev/lamina/`.

## Environment

| Var | Purpose |
|-----|---------|
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account (selects Workers AI or gateway) |
| `CLOUDFLARE_AXON_GATE_TOKEN` | Workers AI auth via axon-gate |
| `CLOUDFLARE_AI_GATEWAY_TOKEN` | AI Gateway auth (weekly mode) |
| `ANTHROPIC_API_KEY` | Direct Anthropic API fallback |
| `SYND_SITE_DIR` | Site directory for post listing and weekly date derivation |
| `SYND_SERVICE_URL` | Synd server URL for draft submission |
| `MEMO_SERVICE_URL` | axon-memo URL for recall |
| `SEARXNG_URL` | SearXNG instance for web search |
| `AXON_DISPATCH_URL` | Research dispatch worker (enables parallel URL fetching) |
| `AXON_WIRE_URL` | Wire proxy URL |
| `AXON_WIRE_TOKEN` | Wire proxy / dispatch auth token |
| `DEV` | Workspace root (defaults to ~/dev) |
| `IMAGO_DRAFT_MODEL` | Override draft model in weekly mode (default: claude-opus-4-6) |

## Flow

1. Interview: LLM asks questions, uses tools to research, you answer
2. `/draft`: LLM generates full markdown draft from transcript
3. Section review: draft split by headings, approve (`/keep`) or give feedback per section
4. Final review: full article view, give feedback or `/done`
5. Draft saved to `~/Documents/imago/<slug>.md`

## Session persistence

Sessions auto-save after every turn to `~/.local/share/imago/sessions/`. Incomplete sessions can be resumed on next launch.
