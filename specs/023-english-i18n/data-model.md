# Data Model: Failure reason codes

This feature adds only three columns, holding the failure reason codes for
probing and importing. Existing columns, in particular the free-text
`videos.probe_error`, `scans.error` and `jobs.last_error`, keep their shape and
existing values (requirement 8). Both tables are rebuildable data
([Running VVMDM, "Data and recovery"](../../docs/how-to/running-vv.md#data-and-recovery)),
and that classification does not change.

The migration is a single file, `internal/store/migrations/00016_failure_codes.sql`,
which only adds the three columns as nullable `text`. Existing rows stay `null`
(failures from before the upgrade, without a code); past free text is not
parsed to fill in codes
([research.md R-6](research.md#r-6-store-a-machine-readable-failure-code-and-show-no-free-text-on-screen)).

Code values are spelled the same in the `internal/domain` constants and the
`api/openapi.yaml` enums ([contracts/error-api.md, `Video.probeErrorCode`](contracts/error-api.md#videoprobeerrorcode) and [`Scan.errorCode` and `Scan.errorPath`](contracts/error-api.md#scanerrorcode-and-scanerrorpath)).
The type that wraps a failure with a code also lives in `internal/domain` and is
extracted with `errors.As`. A failure that cannot be classified is `internal`.

## `videos.probe_error_code`

Written together with `probe_error` (English free text) when `probe_state`
becomes `failed`. Where `probe_error` is reset to `null` (a successful probe,
the start of a re-probe), it is reset to `null` too.

| Code | Situation (where the failure is created) |
| --- | --- |
| `file_unavailable` | The file cannot be checked before or after the probe, or is not a regular file (`internal/media/probe.go`, `assets.go`) |
| `probe_unavailable` | `ffprobe` cannot be started (`internal/media/probe.go`) |
| `probe_failed` | `ffprobe` failed, including broken or unsupported files and timeouts (same as above) |
| `invalid_metadata` | The `ffprobe` output cannot be parsed, or the duration is unreadable or invalid (same as above) |
| `internal` | Anything else (failures in `internal/app` or `internal/store`, an unknown job kind, and so on) |

`jobs.last_error` is not exposed by the API, so it gets no code column. Its text
becomes English.

## `scans.error_code` and `scans.error_path`

Written together with `error` (English free text) when `state` becomes `failed`
(`FinishScan`, and `FailInterruptedScans` at startup). `error_path` is set only
when the reason is tied to a specific place, and is `null` otherwise.

| Code | `error_path` | Situation (where the failure is created) |
| --- | --- | --- |
| `media_folder_unreadable` | The media folder | The media folder cannot be read (start of the walk in `internal/scanner`, the check after import) |
| `media_folder_not_directory` | The media folder | The media folder is not a directory, or is a symbolic link (same as above) |
| `location_unreadable` | The unreadable place | A place could not be read during the walk, or a parent directory could not be checked after import (same as above) |
| `interrupted` | — | Cut off by a stop instruction, or the application stopped during import (`internal/app/scans.go`, `FailInterruptedScans` in `internal/store/scans.go`) |
| `internal` | — | Anything else (progress could not be recorded, a panic during processing, and so on) |
