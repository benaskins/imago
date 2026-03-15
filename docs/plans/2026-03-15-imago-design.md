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

**Interview screen.** Conversation view — imago's questions and your responses. When imago uses a tool (fetching a page, reading a post), the tool invocation and result appear inline so you can see what it's doing. Standard text input at the bottom.

**Draft screen.** Shows the current section of the draft with approve/revise/rewrite controls. Navigation between sections. Final approval submits to synd.

## LLM strategy

Two models, switched per-phase via axon-loop's per-request model field:

- **Interview phase:** `qwen3:32b` — fast responses, keeps conversational momentum
- **Draft phase:** `qwen3:235b` — stronger synthesis and writing, slower is acceptable

Both run locally on Ollama. The axon-talk Ollama adapter handles the connection. Model names are configurable.

## Tools

Defined as `tool.ToolDef` implementations:

| Tool | Purpose |
|------|---------|
| `read_post` | Read an existing generativeplane.com post for tone/context reference |
| `fetch_page` | Fetch and extract content from a URL (research) |
| `list_posts` | List published posts on the site |
| `submit_draft` | Submit the finished draft to synd server API |

axon-tool already provides `PageFetcher` and `SearXNG` search. `read_post` and `list_posts` read from the site directory. `submit_draft` calls the synd HTTP API.

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
| bubbletea | latest | Reactive terminal UI framework |
| lipgloss | latest | Terminal styling |
| glamour | latest | Markdown rendering in terminal |

## Implementation plan

1. Scaffold CLI with Bubble Tea — basic chat input/output, no LLM yet
2. Wire axon-loop + axon-talk/ollama — interview conversation works end-to-end
3. Add interview tools — read_post, fetch_page, list_posts with visible tool use in TUI
4. Add axon-memo integration — recall at session start, extract at session end
5. Build draft phase UI — section-by-section review with keep/revise/rewrite
6. Add submit_draft tool — synd API integration
7. System prompt tuning — interview quality, pushback behaviour, voice development
