# Weekly Update Feature Design

Automate an end-of-week blog post for generativeplane.com covering everything achieved since the last weekly update. Deterministic data collection, then conversational writing with editorial review.

## What it does

`imago weekly` gathers git activity across all repos on both machines (local and hestia), pre-loads it as structured context, then runs the existing interview-draft-review flow. The interview phase uses local Ollama models (free, fast round trips). The draft and revision phases use Claude Opus via Cloudflare AI Gateway (strong editorial synthesis, few calls).

## Architecture

```
imago weekly
  │
  ├── 1. Collection (deterministic)
  │     ├── derive "since" date from latest weekly-*.md
  │     ├── scan ~/dev on this machine
  │     ├── ssh hestia, scan ~/dev
  │     ├── deduplicate, merge
  │     └── structured markdown report
  │
  ├── 2. Interview (Ollama / Qwen)
  │     ├── system prompt: weekly framing + collection report + previous weekly
  │     ├── editorial direction: theme grouping, axon connections, new repos/sites
  │     └── existing tools available for drill-down
  │
  └── 3. Draft + Review (Claude Opus via Cloudflare AI Gateway)
        ├── explicit structure: opening reflection, themed sections, closing editorial
        ├── section-by-section review
        └── submit to synd
```

## New axon-talk adapter: `axon-talk/anthropic`

New package in axon-talk. Implements `loop.LLMClient` against the Anthropic Messages API, routed through Cloudflare AI Gateway.

### Why a new adapter

The existing `axon-talk/cloudflare` package speaks the OpenAI-compatible chat completions API that Workers AI exposes. The Anthropic Messages API is a different wire format — content blocks instead of choices, tool use blocks instead of function calls, different SSE event structure. A separate adapter is cleaner than overloading the existing one.

### Gateway routing

Cloudflare AI Gateway proxies to multiple providers via path:

```
https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway}/workers-ai  ← existing
https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway}/anthropic   ← new
```

All LLM usage (Qwen via Workers AI, Opus via Anthropic) flows through the same gateway. Unified logging, cost tracking, and caching in one Cloudflare dashboard.

### Interface

```go
client := anthropic.NewClient(gatewayBaseURL, anthropicAPIKey)

// Model selection via req.Model field — same as other adapters
req := &loop.Request{
    Model:    "claude-opus-4-6",  // or claude-sonnet-4-6, etc.
    Messages: msgs,
    Stream:   true,
    Tools:    tools,
}
```

### Anthropic Messages API specifics

- Request body: `model`, `messages` (content blocks), `tools`, `max_tokens`, `stream`
- Messages use content blocks: `[{"type": "text", "text": "..."}]` not plain strings
- Tool use: `{"type": "tool_use", "id": "...", "name": "...", "input": {...}}` in assistant messages
- Tool results: `{"type": "tool_result", "tool_use_id": "...", "content": "..."}` in user messages
- Streaming: SSE with `content_block_start`, `content_block_delta`, `content_block_stop`, `message_delta`, `message_stop` events
- Auth header: `x-api-key` (not Bearer token)
- API version header: `anthropic-version: 2023-06-01`

### Scope

- `Chat()` method: streaming and non-streaming
- Message conversion: `loop.Message` to/from Anthropic content blocks
- Tool conversion: `tool.ToolDef` to Anthropic tool format, tool results round-trip
- Model passthrough: `req.Model` sent directly — supports Opus, Sonnet, Haiku
- No caching, batching, or beta features in v1

## Collection script

### Location

`internal/collect/collect.go` — a Go package, not a shell script. Called from `cmd/imago/main.go` before TUI starts.

### Since date derivation

Reads the site directory (`$SYND_SITE_DIR` / generativeplane.com), finds the most recent `weekly-*.md` file, parses the date from the filename. Falls back to 7 days ago if no weekly exists.

### Data sources

**Local machine (`~/dev`):**
- `find ~/dev -name .git -type d -maxdepth 4` to discover repos
- Per repo: `git log --oneline --since=<date>`, `git diff --stat <since>..HEAD`, `git tag --sort=-creatordate`

**Hestia (`ssh hestia`):**
- Same discovery and per-repo commands, run over SSH
- `aurelia status` for running services

### Deduplication

Same repo on both machines (matched by remote origin URL) → merge commit lists, note which machine. Repos only on one machine → flag as such.

### Output format

Structured markdown, readable by both humans and LLMs:

```markdown
## Activity since March 15, 2026

### Repos with activity (28 repos, 2 machines)

#### axon-synd (46 commits)
- Machines: hestia, local
- Key commits:
  - feat: add Threads syndicator
  - feat: render markdown links in short-form posts
  - fix: remove hardcoded studio.internal defaults
  - ...
- Diffstat: 42 files changed, 2,841 insertions, 891 deletions
- Tags: v0.3.0

#### aurelia (38 commits)
...

### New repos
- musicbox (created 2026-03-12, 30 commits) — on hestia only

### New sites published
- sailorgrift.com, theyhavenames.org

### Aurelia services (hestia)
- Running: chat, auth, task-runner, memory, synd, deploy-gate, analytics
- Infrastructure: traefik, vault
```

### Performance

Collection runs sequentially — local scan, then SSH scan. No parallelism needed; the whole thing should complete in under 10 seconds. The SSH connection reuses a single session for all commands (ControlMaster or a single `ssh hestia 'script'`).

## Weekly system prompt

New `WeeklySystemPromptTemplate` in `internal/config/config.go`.

```
You are a research journalist interviewing a builder to write a weekly
update for generativeplane.com. You have a detailed activity report and
the previous weekly post for reference.

Your job is to understand what matters — not just what changed. The raw
data tells you what happened; the interview tells you why it matters
and what connects it.

[collection report injected here]

[previous weekly post injected here]

Editorial direction:
- Group work by theme, not by repository — find the narrative threads
- Connect things back to axon when the relationship isn't obvious
- Highlight new repos and new sites — these are milestones
- The final post should have three parts:
  1. Opening reflection — one paragraph that frames the week. Not a summary. A thought.
  2. Themed sections — what was built, grouped by narrative thread
  3. Closing editorial — ties it together. An observation about the work, the tools, the process.

Interview rules:
- You have the activity data. Don't ask "what did you work on?" — you know.
- Ask about the why, the surprises, the things that didn't work
- Ask what connects the threads — the subject sees patterns you don't
- Push back on generic answers
- One question at a time
- 8-10 substantive exchanges before suggesting a transition to drafting
```

### Draft prompt

New `WeeklyDraftPrompt` — same voice rules as the existing `DraftPrompt`, plus:

```
Structure the post as:
1. Opening reflection (one paragraph — a thought that frames the week)
2. Themed sections with ## headings (NOT one section per repo)
3. Closing editorial (one paragraph — an observation, not a summary)

The previous weekly post is included for voice reference. Match its
register — opinionated, precise, unsentimental. Let strange details
stay. No gendered pronouns for AI systems.
```

## Dual-client architecture

### Changes to imago

`cmd/imago/main.go` constructs two LLM clients:

```go
interviewClient := selectOllamaClient()      // always local
draftClient := selectAnthropicClient()        // Claude via Cloudflare AI Gateway
```

`tui.New()` accepts both. Interview phase uses `interviewClient`. Draft, revision, and review phases use `draftClient`.

### ModelConfig changes

```go
type ModelConfig struct {
    InterviewProvider Provider
    DraftProvider     Provider
    InterviewModel    string
    DraftModel        string
    // ... existing options fields
}
```

For weekly mode:
- `InterviewProvider`: ollama, `InterviewModel`: qwen3:32b
- `DraftProvider`: anthropic, `DraftModel`: claude-opus-4-6

For regular post mode: unchanged (both Ollama).

### Environment variables

```
ANTHROPIC_API_KEY          — Anthropic API key
CLOUDFLARE_ACCOUNT_ID      — already set
CLOUDFLARE_GATEWAY         — gateway name (e.g. "axon-gate")
```

## CLI entry point

`imago weekly` subcommand. Runs collection, loads weekly prompt, enters TUI.

```bash
imago              # existing: regular blog post interview
imago weekly       # new: weekly update flow
```

Both share the same TUI, tools, session persistence, and synd integration. The difference is:
- Collection report pre-loaded into system prompt
- Weekly-specific system and draft prompts
- Dual LLM clients (Ollama for interview, Opus for draft)
- Session `kind: "weekly"` for resume

## Session changes

```go
type State struct {
    ID        string        `json:"id"`
    Kind      string        `json:"kind"`       // "post" or "weekly"
    Phase     string        `json:"phase"`
    Messages  []loop.Message `json:"messages"`
    // ... existing fields
    Collection string       `json:"collection"` // raw collection report (weekly only)
}
```

Resume logic filters by kind — `imago weekly` only offers to resume weekly sessions.

## Estimated cost per weekly post

Opus pricing: $15/M input, $75/M output. Cloudflare AI Gateway: no markup.

| Phase | API calls | Input tokens | Output tokens |
|-------|-----------|-------------|---------------|
| Interview (Ollama) | 10-15 | — | — |
| Draft generation (Opus) | 1 | ~17k | ~3k |
| Section revisions (Opus) | 3-5 | ~80k total | ~4k total |
| Final review (Opus) | 1-2 | ~30k total | ~3k total |
| **Total Opus** | **5-8** | **~130k** | **~10k** |

**Estimated cost: $2-3 per post, ~$10-15/month for weekly publishing.**

Interview phase is free (local Ollama). The expensive model only touches writing.

## Implementation plan

Each step is one commit.

1. **axon-talk/anthropic adapter** — new package, implements `loop.LLMClient` against Anthropic Messages API via Cloudflare AI Gateway. Streaming + non-streaming. Tool support. Tests against recorded responses.

2. **Collection package** — `internal/collect/` in imago. Discovers repos, gathers git activity, SSHs to hestia, deduplicates, outputs structured markdown. Tests with fixture repos.

3. **Weekly prompts** — `WeeklySystemPromptTemplate`, `WeeklyDraftPrompt` in `internal/config/config.go`. Include collection report injection and previous weekly post.

4. **Dual-client wiring** — extend `ModelConfig` for per-phase providers. `tui.New()` accepts interview + draft clients. Draft/revision/review phases use the draft client.

5. **CLI subcommand** — `imago weekly` runs collection, constructs both clients, loads weekly prompts, enters TUI.

6. **Session kind** — add `kind` and `collection` fields to session state. Resume filters by kind.

7. **End-to-end test** — run `imago weekly` against the real collection data, verify the full flow works. Run through slopguard.

## Prerequisites

- Anthropic API key
- Anthropic provider configured on Cloudflare AI Gateway
- SSH access to hestia (already confirmed: `ssh hestia`, keys exchanged, trusted boundary)
