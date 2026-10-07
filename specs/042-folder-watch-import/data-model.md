# Data model: Auto-import changed media folders

The rest of the model is unchanged: the schema is in
[internal/store/migrations](../../internal/store/migrations), and the import
tables are described in
[024 data-model](../024-import-progress/data-model.md). No table is added; the
dirty-directory set and the watch problem live in memory only
([research.md R-8](research.md#r-8-an-interrupted-watch-batch-is-closed-not-resumed)).

## Migration

`00033_scan_origin.sql` adds `scans.origin` and fills `'manual'` for every
existing row.

## `scans.origin` (added column)

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `origin` | text, `check (origin in ('manual', 'watch'))`, default `'manual'` | no | Who started the scan: the owner (or startup resume, or an external client) for `manual`, the folder watcher for `watch` ([R-4](research.md#r-4-a-watch-batch-is-a-scan-row-with-origin-watch)) |

## `settings` key `library.auto_import`

| Value | Meaning |
| --- | --- |
| absent | Auto-import is on (the Issue's default) |
| `true` | On |
| `false` | Off |

## Rules

| Rule | Enforced in |
| --- | --- |
| At most one scan runs, whatever its origin | `scans_single_running_idx` (unchanged) |
| A watch scan removes a location only when its path is directly in a dirty directory, or under a dirty subtree, and the batch did not find it ([R-2](research.md#r-2-a-change-marks-a-directory-dirty-and-only-dirty-directories-are-re-read)) | `internal/scanner` |
| Every addition of a watch scan is written before its first removal ([R-3](research.md#r-3-a-batch-waits-for-quiet-imports-settled-files-then-removes)) | `internal/scanner` |
| A superseded watch scan writes no removal and closes `done` ([R-5](research.md#r-5-a-manual-scan-or-a-media-folder-change-supersedes-a-watch-batch)) | `internal/app` (`Scans`), `internal/scanner` |
| Starting a watch scan keeps the previous scan's `scan_issues` rows whose path is outside its dirty set, re-attached to the new scan; rows inside the set are dropped and re-evaluated. A manual scan still clears them all | `internal/store` (`startScan`) |
| A `done` watch scan that was not superseded applies ready successions ([R-9](research.md#r-9-a-finished-watch-batch-applies-same-path-content-changes)) | `internal/store` (`FinishScan`) |
| Startup closes an interrupted scan of either origin and resumes only a `manual` one ([R-8](research.md#r-8-an-interrupted-watch-batch-is-closed-not-resumed)) | `internal/app` (`Scans`) |

## What does not change

`scan_videos` keeps attaching to the latest scan, so jobs queued by a watch
scan count toward that scan's progress; the screen decides from `origin`
whether to show it. User data stays keyed by content key, so a delete followed
by a later re-copy of the same file across two batches restores tags and
playback position, as two manual scans already do.
