# imago

Interactive CLI that produces blog posts from structured interviews. Two-phase flow: interview then section-by-section draft review.

Read `docs/plans/2026-03-15-imago-design.md` for the full design.

## Architecture

Standalone Go CLI binary. Composes axon modules — not an axon module itself.

- **TUI**: Bubble Tea (charmbracelet/bubbletea) reactive terminal UI
- **LLM**: axon-loop + axon-talk/ollama, local inference
- **Tools**: axon-tool definitions for research, infrastructure queries, publishing
- **Memory**: axon-memo for editorial voice development (agent slug: `imago`)
- **Research**: axon-task executor for Claude Code subprocess sessions

## Build

```bash
go build -o bin/imago ./cmd/imago
```

## Conventions

- Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `infra:`, `config:`
- No Python — Go only
- Tests must not leak to live services — clear production env vars in tests
- CLI is a thin wrapper — no direct store access
