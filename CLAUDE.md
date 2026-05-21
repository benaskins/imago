@AGENTS.md

## Conventions
- Bubble Tea model architecture — TUI code in internal/tui/, config in internal/config/
- face.Chat handles the reusable chat component (from axon-face)
- Three modes: `imago` (interview), `imago weekly` (workspace weekly post), `imago daily` (workspace daily entry)
- `--audience <name>` selects the prompt set on any mode; `self` is the default. Audiences are embedded text/template files under internal/config/audiences/
- Tool definitions in tools/tools.go — 15 axon-tool definitions
- Sessions auto-save to ~/.local/share/imago/sessions/; session kind is `<mode>:<audience>`

## Constraints
- Composition root — assembles axon-face, axon-loop, axon-talk, axon-tool, axon-wire
- Uses local replace directives during development (managed by `lamina apps wire`)
- Do not add HTTP server code — this is a CLI app, not a service
- Do not import axon (server toolkit) directly

## Testing
- `go test ./...` and `just test`
- `just build` to build, `just install` to install to ~/.local/bin
