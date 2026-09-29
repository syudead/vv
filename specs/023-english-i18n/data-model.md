# Data Model: failure codes

This feature adds only three columns, which hold the failure-reason codes for probing and scanning.
Existing columns keep their shape and their existing values, in particular the free-text
`videos.probe_error`, `scans.error` and `jobs.last_error` (Requirement 8). Both tables hold
rebuildable data
([ARCHITECTURE.md "Rebuildable and user data"](../../ARCHITECTURE.md#rebuildable-and-user-data)),
and that classification does not change.

- One migration, `internal/store/migrations/00016_failure_codes.sql`, adds the three columns as
  nullable `text`.
- Existing rows stay `null` (a failure from before the upgrade, without a code). The migration does
  not parse old free text to fill in codes
  ([research.md R-6](research.md#r-6-failure-reasons-are-stored-as-machine-readable-codes-the-ui-does-not-show-free-text)).
- Code values use the same spelling in the `internal/domain` constants and in the
  `api/openapi.yaml` enums ([contracts/error-api.md §2 and §3](contracts/error-api.md)).
- The type that wraps a failure with its code also lives in `internal/domain`; callers extract it
  with `errors.As`. A failure that cannot be classified is `internal`.

## 1. `videos.probe_error_code`

Written together with `probe_error` (English free text) when `probe_state` becomes `failed`. Every
place that resets `probe_error` to `null` (a successful probe, the start of a re-probe) resets it to
`null` as well.

| Code | Situation (where the failure is created) |
| --- | --- |
| `file_unavailable` | The file cannot be verified before or after probing, or is not a regular file (`internal/media/probe.go`, `assets.go`) |
| `probe_unavailable` | `ffprobe` cannot be started (`internal/media/probe.go`) |
| `probe_failed` | `ffprobe` failed, including a broken or unsupported file and a timeout (same location) |
| `invalid_metadata` | The `ffprobe` output cannot be parsed, or the duration is unreadable or invalid (same location) |
| `internal` | Anything else (failures in `internal/app` or `internal/store`, an unknown job kind, and similar) |

`jobs.last_error` does not appear in the API, so it gets no code column. Its text becomes English.

## 2. `scans.error_code` and `scans.error_path`

Written together with `error` (English free text) when `state` becomes `failed` (`FinishScan`, and
`FailInterruptedScans` at startup). `error_path` is set only when the reason is tied to a specific
location; otherwise it is `null`.

| Code | `error_path` | Situation (where the failure is created) |
| --- | --- | --- |
| `media_folder_unreadable` | The media folder | The media folder cannot be read (start of the walk in `internal/scanner`, the check after the import) |
| `media_folder_not_directory` | The media folder | The media folder is not a directory, or is a symbolic link (same locations) |
| `location_unreadable` | The unreadable location | A location could not be read during the walk, or the parent directory cannot be verified after the import (same locations) |
| `interrupted` | — | A stop request ended the scan, or the application stopped mid-import (`internal/app/scans.go`, `FailInterruptedScans` in `internal/store/scans.go`) |
| `internal` | — | Anything else (progress cannot be recorded, a panic during processing, and similar) |
