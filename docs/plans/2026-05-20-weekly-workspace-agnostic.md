# imago weekly: workspace-agnostic redesign

**Status:** Complete (2026-05-20)
**Date:** 2026-05-20
**Source:** Brainstorm session (Claude Code transcript `f9192c3b...`, 20 May 15:47), revised 2026-05-20 to correct "monorepo" to "workspace".

## Goal

Adapt `imago weekly` so it can run against any workspace (a root directory containing one or more sibling git repos), not just the lamina workspace. Decouple from generativeplane.com, remove the hardcoded `$DEV` root and lamina-specific site detection, produce generic markdown that lives inside the workspace root.

`imago daily <path>` is intended as a follow-up; weekly lands first.

## Non-goals (this iteration)

- Base `imago` interview mode. Its prompts still reference axon-loop/aurelia; separate cleanup later.
- `imago daily <path>`, deferred until weekly lands.
- A wrapper that replicates the old "publish to generativeplane via synd" workflow. Out of scope; can be built externally as a small script if wanted.
- Manifest-driven workspace scoping (e.g. respecting `repos.yaml`). Walk-only for now.

## Final design

**Command shape**

```
imago weekly <path>
```

`<path>` is required and must point to a workspace root (a directory containing one or more git repos as direct or nested children).

**Scope.** Activity collection walks `<path>` recursively (bounded depth, do not descend into a found `.git`) and treats every directory containing `.git` as a repo. No reliance on `$DEV`. No sibling-of-`<path>` walking, no `~/dev/sites` detection.

**Output.** `<path>/.imago/weekly/weekly-YYYY-MM-DD.md`, with today's date as filename. One workspace-level post covering all repos found under `<path>`.

**"Since" derivation.** Latest prior `weekly-*.md` in `<path>/.imago/weekly/`, else 7 days ago. Self-bootstrapping.

**Prompts.** Strip generativeplane.com / lamina / aurelia / axon-* / getlamina.ai references from `WeeklySystemPromptTemplate` and the draft prompt. Generic framing: "research journalist interviewing a builder about a week of work in `<workspace name>`," where workspace name defaults to `filepath.Base(path)`.

**Tools.** Strip lamina-specific tools (`aurelia_status`, `lamina`, anything workspace-y in the lamina sense). Keep generic ones (web search, fetch page, generic git inspection like `repo_overview`).

**Removed.**
- synd submission, `SYND_SITE_DIR`, `SYND_SERVICE_URL` env reads.
- `~/Documents/imago/` output path.
- `$DEV`-rooted repo discovery (replaced with `<path>`-rooted).
- `detectNewSites` and the `$DEV/sites/*` hardcode.
- Hestia remote scanning (already gone, commit `8c012be`).

**Preserved.** Bubble Tea TUI, three-phase flow (interview, section review, final review), session persistence, section-by-section review loop, sibling-walk discovery semantics (just re-rooted).

## Coupling analysis (pre-design notes)

**Cosmetic (easy to parameterise)**
- `internal/config/config.go` lines 191, 203–204, 256–257 hard-code prompts mentioning lamina/aurelia/axon-*, getlamina.ai/generativeplane.com, and `github.com/benaskins/*` link patterns.
- Weekly mode publishes to a synd service (`SYND_SITE_DIR`, `SYND_SERVICE_URL`) and derives "since" from filenames in that site dir.
- Default Cloudflare gateway name is `axon-gate`.

**Structural (parameterise, not remove)**
- `internal/collect/collect.go` `discoverRepos` walks `$DEV` looking for `.git` directories up to 3 levels deep. Re-root at `<path>` and keep the walk depth bound.
- `detectNewSites` (collect.go:236-267) is hardcoded to `$DEV/sites/*`. Drop entirely; sites are a lamina concern.

**Generic / portable as-is**
Date derivation, git log/diff/tag gathering, "new repo" detection by first commit date, markdown rendering, Bubble Tea interview flow, session persistence, section-by-section review.

## Specific symbols and paths

- `cmd/imago/main.go`: CLI entrypoint; add path-arg validation (exists, is a directory, contains at least one git repo).
- `internal/collect/collect.go`
  - `Run(Config)`: take `Config.WorkspacePath` instead of relying on `$DEV`.
  - `discoverRepos`: keep, re-root at `Config.WorkspacePath`.
  - `detectNewSites` (lines 236-267): **drop**.
  - `scanLocal`: re-root at workspace path.
  - `RepoActivity`: unchanged shape, but now scoped to the passed workspace.
- `internal/config/config.go`
  - `WeeklySystemPromptTemplate`: genericize, take workspace name as parameter.
  - Draft prompt template: genericize alongside.
- `tools/tools.go`: 15 tool definitions; drop `aurelia_status`, `lamina`.
- Env vars to remove: `SYND_SITE_DIR`, `SYND_SERVICE_URL`, reliance on `$DEV`.
- Old output dir to remove: `~/Documents/imago/`.
- New output convention: `<path>/.imago/weekly/weekly-YYYY-MM-DD.md`.

## Iterate plan (six commit-sized steps)

1. **`collect` refactor: workspace path injection.** Replace `$DEV` lookups with `Config.WorkspacePath`. Re-root `discoverRepos` and `scanLocal` at that path. Drop `detectNewSites`. Update tests.

2. **CLI: require path arg for `imago weekly`.** Validate `<path>` exists, is a directory, and contains at least one git repo (direct or nested). Wire through to collect.

3. **Genericize prompts.** Rewrite `WeeklySystemPromptTemplate` and draft prompt template to take workspace name. Remove all generativeplane/lamina/axon-*/site references. Tests assert no banned strings appear.

4. **Trim tools for weekly mode.** Audit `tools/tools.go`, remove lamina-specific entries (or build a weekly-mode subset). Update wiring in `main.go`.

5. **Output path: `<path>/.imago/weekly/weekly-YYYY-MM-DD.md`.** Replace synd submission and `~/Documents/imago/` writes. Create dirs as needed. Update since-derivation to read from the new location.

6. **Remove dead code.** synd client calls, `SYND_*` env reads, related plumbing. `doc.go` / README updates.

## Open questions

- None blocking. The transcript ended before final user confirmation that base `imago` interview mode is out of scope for this iteration, but the direction was clear.

## Behavioral gap during transition

Replacing the old form means the existing weekly to generativeplane workflow breaks until a wrapper script is built. Accepted trade-off.
