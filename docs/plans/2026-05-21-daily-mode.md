# imago daily mode

**Status:** Complete (2026-05-21)
**Date:** 2026-05-21

## Goal

Add `imago daily <path>` mode. Same workspace-agnostic machinery as `imago weekly` (path-driven, single workspace root, self-bootstrapping since-date). Shorter interview, shorter output.

## Shape

- Time window: 24h, derived from latest prior `daily-*.md` in `<path>/.imago/daily/`, else 24h ago.
- Interview: ~3-5 exchanges (vs weekly's ~8-10).
- Output target: ~200-400 words. One brief reflection + bullets of what happened. No themed sections.
- Output file: `<path>/.imago/daily/daily-YYYY-MM-DD.md`.
- Uses Opus (same as weekly) via Cloudflare AI Gateway.

## Design notes

- Factor period concept into `collect.PeriodDir(workspace, period)`; drop the standalone `WeeklyDir`.
- Add `Config.Period` field; `Run` derives since from `PeriodDir(WorkspacePath, Period)`.
- `tui.Model.WithDailyMode(systemPrompt, outputDir)` mirrors `WithWeeklyMode`, sets `sessionKind="daily"`, uses `DailyDraftPrompt`.
- `tui.writeDailyDraft(markdown, dir)` mirrors `writeWeeklyDraft`, writes `daily-YYYY-MM-DD.md`.
- Daily fallback for since is "24h ago" not "7 days ago".

## Iterate plan (four commit-sized steps)

1. **`collect`: period-aware since derivation.** Add `PeriodDir(workspace, period)` and `Config.Period`. Replace `WeeklyDir`. Adjust `Run` to compute since with period-appropriate fallback (24h for daily, 7d for weekly). Update tests.

2. **`config`: daily prompts.** Add `DailySystemPrompt(workspaceName, workspacePath, collectionReport, previousDaily)` and `DailyDraftPrompt`. Genericized; ~3-5 exchanges; ~200-400 word target; brief reflection + bullets. Banned-string tests.

3. **TUI + CLI wiring.** Add `WithDailyMode` and `writeDailyDraft`. Detect `daily` subcommand in `main.go`; share validation with weekly; wire through.

4. **Docs.** Update AGENTS.md and README.md to document `imago daily <workspace-path>`.
