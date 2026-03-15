# Imago Design

Imago is an interactive CLI that produces blog posts from structured interviews. It runs locally, uses local LLMs via Ollama, and submits drafts to the synd publishing pipeline.

## What it does

You run `imago` when you want to write a post. It interviews you, researches as needed, then produces a draft for review. Two phases, clean boundary between them.

**Phase 1 — Interview.** Imago asks questions, follows threads, pushes back when answers sound rehearsed or generic. It can use tools mid-interview — fetch web pages, read existing posts for context, look up details. Tool use is visible in the TUI. The interview ends when imago has enough material or you say you're done.

**Phase 2 — Draft review.** Imago produces a full draft, then presents it section by section. For each section you can keep, revise, or ask it to rewrite. When you approve the last section, it submits to synd as a draft via the server API.

## Architecture

```
imago (CLI binary)
├── TUI          — Bubble Tea reactive terminal UI
├── axon-loop    — LLM conversation loop with tool dispatch
├── axon-talk    — Ollama adapter for local inference
├── axon-memo    — editorial memory across sessions (agent slug: imago)
└── axon-tool    — tool definitions for research and publishing
```

Imago is an assembled application, not a library. It composes axon modules but is not an axon module itself.

## TUI

Bubble Tea (charmbracelet/bubbletea) reactive TUI. Two distinct screens:

**Interview screen.** Conversation view — imago's questions and your responses, rendered with Lip Gloss styling. Tool use appears as a single collapsed line (e.g. `↳ fetching generativeplane.com/posts/...`) that can be expanded. Standard text input at the bottom.

**Draft screen.** Each section rendered as styled markdown via Glamour. Controls per section: keep, or type a directive to revise ("cut the last sentence", "add the detail about 4o"). Imago rewrites and re-renders. Repeat until you move on. Final section approval submits to synd.

## LLM strategy

Two models, switched per-phase via axon-loop's per-request model field:

- **Interview phase:** `qwen3:32b` — fast responses, keeps conversational momentum
- **Draft phase:** `qwen3:235b` — stronger synthesis and writing, slower is acceptable

Both run locally on Ollama. The axon-talk Ollama adapter handles the connection. Model names are configurable.

## Tools

Defined as `tool.ToolDef` implementations:

| Tool | Purpose |
|------|---------|
| `read_post` | Read an existing post for tone/context reference |
| `list_posts` | List published posts on the site |
| `fetch_page` | Fetch and extract content from a URL |
| `search` | Search the web via SearXNG |
| `recall` | Recall memories from past interviews via axon-memo |
| `read_file` | Read a local file — source code, config, design docs |
| `git_log` | Check recent commit history for a repo |
| `aurelia_status` | Query running services, health, uptime |
| `aurelia_show` | Get details about a specific service |
| `lamina` | Run lamina CLI commands — repo status, dependency graph, health checks |
| `claude` | Spawn a Claude Code session for deep code research |
| `submit_draft` | Submit finished draft to synd server API |

axon-tool already provides `PageFetcher` and `SearXNG` search. `read_post` and `list_posts` read from the site directory. `aurelia_status` and `aurelia_show` shell out to the aurelia CLI. `submit_draft` calls the synd HTTP API.

### Claude research tasks

The `claude` tool uses axon-task's executor in-process — the same pattern as axon-chat. An embedded executor with a research-oriented prompt builder spawns Claude Code as a subprocess in `--print` mode with read-only permissions (`Read`, `Glob`, `Grep`). No `Edit`, no `Write`, no destructive `Bash` — the journalist's source can read the codebase but not change it.

This lets imago delegate technical questions ("how does the Cloudflare deploy work in axon-synd?") to Claude, which reads the code and reports back. The result appears in the TUI as a collapsed tool use, same as any other tool.

## Editorial memory

Imago gets its own agent slug (`imago`) in axon-memo. Memories accumulate across sessions:

- **Semantic / durable** — voice preferences, editorial rules (e.g. "no gendered pronouns for AI", "unsentimental, let strange details in"), tone description
- **Episodic** — past interview topics, what angles worked, what got cut
- **Corrections** — editorial feedback from draft review ("that's a trope", "too sentimental") stored as durable semantic memories with high importance

These memories are injected into the system prompt at the start of each session via axon-memo's recall API. The voice develops over time through accumulated corrections — the same mechanism as any axon-memo agent.

## System prompt

The system prompt establishes imago's role:

- You are a journalist conducting an interview to produce a blog post
- You ask one question at a time
- You follow interesting threads and push back on rehearsed or generic answers
- You have access to tools for research — use them when a claim needs context or a reference would strengthen the piece
- You do not write during the interview — you gather material
- When you have enough, you say so and transition to drafting

Editorial memories are appended to the system prompt at session start.

## Voice development

The voice is not configured — it emerges from accumulated editorial feedback. Three mechanisms:

**Keep signals.** When a section is approved without revision, imago extracts why it worked at end of session — sentence structure, register, what grounded what. These are stored as episodic memories in axon-memo.

**Published delta.** After a post is published via synd, imago reads the final version and compares it to the submitted draft. Revisions made between approval and publication are a learning signal — stored as episodic memories.

**Consolidation.** axon-memo's consolidation pipeline identifies patterns across episodic memories and distills them into durable semantic memories. Individual corrections ("too sentimental", "that's a trope") consolidate into editorial principles ("prefer understatement", "name strange details without explaining their significance").

The feedback loop:

```
interview → draft → corrections (episodic) → publication → delta (episodic)
                                    ↓
                            consolidation (periodic)
                                    ↓
                         editorial principles (semantic/durable)
                                    ↓
                    injected into next session's system prompt
```

The voice emerges from the gap between what imago produces and what the user accepts. The agent finds the register by watching the user edit.

## Synd integration

Draft submission uses the existing synd server API:

```
POST /api/posts
{
  "kind": "long",
  "body": "<markdown>",
  "title": "<title>",
  "abstract": "<abstract>",
  "tags": [...]
}
```

The synd server handles the rest — Signal notification, approval gate, publishing, Cloudflare deploy, syndication.

## Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| axon-loop | latest | Conversation loop with tool dispatch |
| axon-talk | latest | Ollama LLM adapter |
| axon-tool | latest | Tool definition primitives + PageFetcher |
| axon-memo | latest | Long-term editorial memory |
| axon-task | latest | Claude Code research task executor |
| bubbletea | latest | Reactive terminal UI framework |
| lipgloss | latest | Terminal styling |
| glamour | latest | Markdown rendering in terminal |

## Implementation plan

Each step validates that axon modules compose cleanly. Friction points surface here.

1. **Bubble Tea scaffold** — basic chat input/output with Lip Gloss styling, no LLM. Proves the TUI framework works.
2. **axon-loop + axon-talk/ollama** — interview conversation end-to-end. Type a message, get a response from qwen3:32b, streamed into the TUI. Proves loop and talk compose.
3. **axon-tool integration** — read_file, git_log, fetch_page, search, list_posts, read_post. Tool calls visible in TUI as collapsed lines. Proves tool dispatch works inside Bubble Tea's event loop.
4. **Infrastructure tools** — aurelia_status, aurelia_show, lamina. Shell out to CLIs, capture output.
5. **axon-task / claude** — embed executor in-process, research-only prompt builder, read-only permissions. Proves axon-task composes outside axon-chat.
6. **axon-memo** — recall at session start (inject editorial memories into system prompt), extract at session end (keep signals, corrections). Proves memo composes as a client library.
7. **Draft phase UI** — switch to Glamour-rendered markdown, section-by-section review with directives. Model switch to qwen3:235b.
8. **Synd integration** — submit_draft tool calls synd server API. End-to-end: interview → draft → submit → Signal notification.
9. **Voice development loop** — published delta comparison, consolidation trigger. The feedback loop that builds editorial judgement over time.
