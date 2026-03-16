# imago

A terminal app that interviews you and writes a blog post. Two phases: interview, then section-by-section editing. Runs entirely on local LLMs via Ollama.

## How it works

1. **Interview** — imago asks questions, follows threads, pushes back on vague answers. It can use tools to look up code, check services, or search the web mid-conversation.
2. **Draft** — when you say `/draft`, imago writes a complete post from the interview material.
3. **Edit** — the draft is split into sections by headings. You review each one, give feedback, correct facts. The agent revises with the full interview as ground truth.
4. **Final review** — all sections assembled, one last pass over the whole piece.
5. **Save** — `/done` writes the markdown to `~/Documents/imago/`.

See [a real session](docs/example-session.md) that produced the first imago blog post in 26 minutes.

## Requirements

- [Ollama](https://ollama.com) with `qwen3:32b` (or configure a different model)
- Go 1.26+

## Build

```bash
go build -o bin/imago ./cmd/imago
```

Or with the justfile:

```bash
just build     # build to bin/
just install   # build + copy to ~/.local/bin/
just test      # run tests
```

## Configuration

All optional, via environment variables:

| Variable | Purpose |
|---|---|
| `DEV` | Workspace root directory for tool context |
| `SYND_SITE_DIR` | Path to site directory for post reading |
| `SYND_SERVICE_URL` | Synd server URL for draft submission |
| `MEMO_SERVICE_URL` | axon-memo URL for editorial memory |
| `SEARXNG_URL` | SearXNG instance URL for web search |

Without these, imago still works — you just won't have tools that depend on external services.

## Commands

| Command | Phase | Action |
|---|---|---|
| `/draft` | Interview | Transition to draft generation |
| `/keep` or `/k` | Edit | Approve current section |
| `/done` | Final review | Save and exit |

## Dependencies

Built on [axon](https://github.com/benaskins) modules:

- **axon-loop** — conversation loop with tool dispatch
- **axon-talk** — Ollama adapter
- **axon-tool** — tool definitions

Terminal UI via [Bubble Tea](https://github.com/charmbracelet/bubbletea), markdown rendering via [Glamour](https://github.com/charmbracelet/glamour).
