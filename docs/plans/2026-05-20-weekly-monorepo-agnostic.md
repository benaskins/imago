# imago weekly: monorepo-agnostic redesign

**Status:** Drafted, not started
**Date:** 2026-05-20
**Source:** Brainstorm session (Claude Code transcript `f9192c3b...`, 20 May 15:47)

## Goal

Adapt `imago weekly` so it can run against any git repo, not just the lamina workspace. Decouple from generativeplane.com, drop sibling-repo workspace assumptions, produce generic markdown that lives inside the target repo.

`imago daily <path>` is intended as a follow-up; weekly lands first.

## Non-goals (this iteration)

- Base `imago` interview mode. Its prompts still reference axon-loop/aurelia; separate cleanup later.
- `imago daily <path>`, deferred until weekly lands.
- A wrapper that replicates the old "publish to generativeplane via synd" workflow. Out of scope; can be built externally as a small script if wanted.

## Final design

**Command shape**

```
imago weekly <path>
```

`<path>` is required and must point to a single git repo.

**Scope.** Activity collection runs against that one repo only. No sibling-walking, no `~/dev/sites` detection, no workspace shape.

**Output.** `<path>/.imago/weekly/weekly-YYYY-MM-DD.md`, with today's date as filename.

**"Since" derivation.** Latest prior `weekly-*.md` in `<path>/.imago/weekly/`, else 7 days ago. Self-bootstrapping.

**Prompts.** Strip generativeplane.com / lamina / aurelia / axon-* / getlamina.ai references from `WeeklySystemPromptTemplate` and the draft prompt. Generic framing: "research journalist interviewing a builder about a week of work on `<repo>`."

**Tools.** Strip lamina-specific tools (`aurelia_status`, `lamina`, anything workspace-y). Keep generic ones (web search, fetch page, generic git inspection like `repo_overview`).

**Removed.**
- synd submission, `SYND_SITE_DIR`, `SYND_SERVICE_URL` env reads.
- `~/Documents/imago/` output path.
- Hestia remote scanning (already gone, commit `8c012be`).

**Preserved.** Bubble Tea TUI, three-phase flow (interview, section review, final review), session persistence, section-by-section review loop.

## Coupling analysis (pre-design notes)

**Cosmetic (easy to parameterise)**
- `internal/config/config.go` lines 191, 203–204, 256–257 hard-code prompts mentioning lamina/aurelia/axon-*, getlamina.ai/generativeplane.com, and `github.com/benaskins/*` link patterns.
- Weekly mode publishes to a synd service (`SYND_SITE_DIR`, `SYND_SERVICE_URL`) and derives "since" from filenames in that site dir.
- Default Cloudflare gateway name is `axon-gate`.

**Structural (bigger lift)**
- `internal/collect/collect.go` models a *workspace of sibling repos*, not a monorepo. `discoverRepos` walks `$DEV` looking for `.git` directories up to 3 levels deep, treating each as a `RepoActivity`. To be dropped.
- `detectNewSites` (collect.go:236-267) is hardcoded to `$DEV/sites/*`. To be dropped.

**Generic / portable as-is**
Date derivation, git log/diff/tag gathering, "new repo" detection by first commit date, markdown rendering, Bubble Tea interview flow, session persistence, section-by-section review.

## Specific symbols and paths

- `cmd/imago/main.go`: CLI entrypoint; add path-arg validation here.
- `internal/collect/collect.go`
  - `Run(Config)`: reshape around `Config.RepoPath`.
  - `discoverRepos`: **drop**.
  - `detectNewSites` (lines 236-267): **drop**.
  - `scanLocal`: replace with per-path scanner.
  - `RepoActivity`: type produced by discovery; reshape for single-repo case.
- `internal/config/config.go`
  - `WeeklySystemPromptTemplate`: genericize.
  - Draft prompt template: genericize alongside.
- `tools/tools.go`: 15 tool definitions; drop `aurelia_status`, `lamina`.
- Env vars to remove: `SYND_SITE_DIR`, `SYND_SERVICE_URL`.
- Old output dir to remove: `~/Documents/imago/`.
- New output convention: `<path>/.imago/weekly/weekly-YYYY-MM-DD.md`.

## Iterate plan (six commit-sized steps)

1. **`collect` refactor: single-repo mode.** Reshape `Run(Config)` around `Config.RepoPath`. Drop `discoverRepos` / `detectNewSites` / sibling-walk. Add since-derivation from `<path>/.imago/weekly/`. Update tests.

2. **CLI: require path arg for `imago weekly`.** Validate `<path>` exists and is a git repo. Wire through to collect.

3. **Genericize prompts.** Rewrite `WeeklySystemPromptTemplate` and draft prompt template. Remove all generativeplane/lamina/axon-*/site references. Tests assert no banned strings appear.

4. **Trim tools for weekly mode.** Audit `tools/tools.go`, remove lamina-specific entries (or build a weekly-mode subset). Update wiring in `main.go`.

5. **Output path: `<path>/.imago/weekly/weekly-YYYY-MM-DD.md`.** Replace synd submission and `~/Documents/imago/` writes. Create dirs as needed.

6. **Remove dead code.** synd client calls, `SYND_*` env reads, related plumbing. `doc.go` / README updates.

## Open questions

- None blocking. The transcript ended before final user confirmation that base `imago` interview mode is out of scope for this iteration, but the direction was clear.

## Behavioral gap during transition

Replacing the old form means the existing weekly to generativeplane workflow breaks until a wrapper script is built. Accepted trade-off.
