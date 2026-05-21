# imago --audience flag

**Status:** Complete (2026-05-21)
**Date:** 2026-05-21

## Goal

Decouple imago from the blog-post format. Add `--audience <name>` that selects which prompts (interview + draft) drive the conversation and which voice/structure the draft takes. First real audience beyond the implicit default: `manager` — short daily status updates for the user's manager.

## Shape

- `--audience <name>` flag on every mode. Default `self` preserves today's behavior.
- First non-default audience: `manager`, supported for `daily` mode.
- Audience definitions live as embedded markdown files under `internal/config/audiences/<audience>/<mode>/{system.md, draft.md}` (`interview` mode also has `revision.md` + `review.md`).
- Templating: `text/template` with named placeholders (`{{.Date}}`, `{{.WorkspaceName}}`, `{{.ActivityReport}}`, `{{.PreviousPost}}`, `{{.WorkspacePath}}`, `{{.InterviewTranscript}}`, `{{.FullDraft}}`, `{{.CurrentSection}}`, `{{.FullArticle}}`).
- Output paths: `self` keeps current path (`<ws>/.imago/<period>/<period>-YYYY-MM-DD.md`). Non-self audiences nest under a subdir: `<ws>/.imago/<period>/<audience>/<period>-YYYY-MM-DD.md`.
- `PreviousPost` lookup respects audience — manager update sees prior manager update, not the self one.
- Session kind becomes `<mode>:<audience>` (e.g. `daily:manager`) so incomplete sessions don't cross-resume.
- Unknown audience for the chosen mode → fail fast with the list of supported `(mode, audience)` pairs.

## Design notes

- **Registry.** `config.LoadAudience(name, mode) (AudienceTemplates, error)` returns parsed `*template.Template` values for system/draft (+ revision/review for interview mode). Backed by `//go:embed audiences/*`.
- **Render helpers.** `tpl.Render(data PromptData) string`. `PromptData` is a single struct holding every placeholder; templates use only what they reference.
- **Migration discipline.** Before deleting any existing `const SystemPromptTemplate`/`DailySystemPromptTemplate`/etc, add a parity test that the new template render produces the same output as the existing `fmt.Sprintf` path for representative inputs. Only delete the consts once parity holds.
- **CLI parsing.** main.go currently hand-parses `os.Args`. Move to `flag.NewFlagSet` per subcommand so `--audience` slots in cleanly without breaking positional `<workspace-path>` parsing.
- **`manager` daily prompts** (first cut, refined in step 5):
  - Interview: 2-3 exchanges max. Ask about decisions made, blockers hit, what's queued for tomorrow. Not surprises, not personal reflection. Tone: professional, concise.
  - Draft: `## What shipped` / `## In progress` / `## Blockers` / `## Next` sections with terse bullets. ~150-300 words. No opening reflection paragraph. Title format: `Manager update: <date>`.

## Iterate plan (six commit-sized steps)

1. **`config`: embed scaffolding + `self` audience parity.** Create `internal/config/audiences/self/{interview,daily,weekly}/...md` containing the current prompt content converted to `text/template` syntax. Add `LoadAudience`, `AudienceTemplates`, `PromptData`. Keep existing const-based functions in place. Add tests asserting rendered output equals `fmt.Sprintf` output byte-for-byte for each prompt.

2. **`config`: cut over callers, delete legacy consts.** Replace calls to `SystemPrompt()`, `DailySystemPrompt(...)`, `WeeklySystemPrompt(...)`, `DraftPrompt`, `DailyDraftPrompt`, `WeeklyDraftPrompt`, `RevisionPromptTemplate`, `ReviewPromptTemplate` with the new registry. Delete the const-based helpers and templates. Tests still green.

3. **CLI + TUI: thread audience through.** Move subcommand parsing in `cmd/imago/main.go` to `flag.NewFlagSet` per mode. Add `--audience` (default `self`). Plumb the chosen audience into `tui.Model.WithDailyMode` / `WithWeeklyMode` (rename signatures to accept the loaded `AudienceTemplates`). Session kind becomes `<mode>:<audience>`. Validate `(mode, audience)` against the registry; fail fast with available pairs.

4. **Audience-aware output paths.** Add `collect.AudienceDir(workspace, period, audience)` returning `<ws>/.imago/<period>` for `self` and `<ws>/.imago/<period>/<audience>` otherwise. Update `PreviousPost` to scan the audience-specific dir. Update `tui` write helpers to use the new path. Tests for both branches.

5. **Add `manager` daily audience.** Author `internal/config/audiences/manager/daily/system.md` + `draft.md` per the manager-prompt sketch above. Registry test that `manager/daily` loads. Rendering test that the draft prompt contains the `What shipped` / `In progress` / `Blockers` / `Next` section labels and excludes journal-entry phrasing.

6. **Docs.** Update `AGENTS.md` (modes section: explain `--audience`, list supported pairs, point to the audiences directory for adding new ones), `CLAUDE.md` if conventions need clarifying, and `README.md` if it advertises the daily command. Mark this plan complete.
