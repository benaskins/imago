# imago

A terminal app that interviews you and writes a blog post. Two phases: interview, then section-by-section editing. Runs entirely on local LLMs via Ollama.

## How it works

1. **Interview** — imago asks questions, follows threads, pushes back on vague answers. It can use tools to look up code, check services, or search the web mid-conversation.
2. **Draft** — when you say `/draft`, imago writes a complete post from the interview material.
3. **Edit** — the draft is split into sections by headings. You review each one, give feedback, correct facts. The agent revises with the full interview as ground truth.
4. **Final review** — all sections assembled, one last pass over the whole piece.
5. **Save** — `/done` writes the markdown to `~/Documents/imago/` (interview mode) or `<workspace>/.imago/weekly/` (weekly mode).

## Modes

- `imago`: interview mode for a single post.
- `imago weekly <workspace-path>`: weekly update mode. Walks `<workspace-path>` for sibling git repos, collects the past week of activity, interviews you with the data, and writes the post to `<workspace-path>/.imago/weekly/weekly-YYYY-MM-DD.md`.
- `imago daily <workspace-path>`: daily journal mode. Same as weekly but scoped to the last 24h, with a shorter interview and a brief journal-entry output written to `<workspace-path>/.imago/daily/daily-YYYY-MM-DD.md`.

### Audiences

Every mode accepts `--audience <name>` (default `self`). The audience selects the prompt set that drives the interview and the draft.

- `--audience self`: personal voice, journal/essay output (the default).
- `--audience manager`: short manager-facing status update (What shipped / In progress / Blockers / Next). Available on `daily`. Output goes to `<workspace>/.imago/daily/manager/`.

```bash
imago daily ~/dev --audience manager
```

Audiences are embedded prompt templates under `internal/config/audiences/<name>/<mode>/`. Adding a new audience is two files (system and draft).

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
| `DEV` | Workspace root directory for base interview mode tool context |
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
