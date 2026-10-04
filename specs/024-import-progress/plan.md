# Implementation Plan: Rebuild import progress and results around what the user wants to know

**Branch**: `feature/024-import-progress` | **Parent Issue**: #444

**Input**: The parent Issue. It is this feature's specification.

## Summary

The screen today shows the scan ratio (`Scan` total / completed) and the
remaining work per job kind (`Processing`) as they are. This feature replaces them
with one progress value, "how many of the latest import's target videos are
done", one line for the current activity, and a per-video list of issues.

| Part | Approach |
| --- | --- |
| Target and settled counts | The server holds the set of videos that belong to the latest scan. A video joins the set when a job is enqueued for it. "Settled" is decided on every read from the video having no remaining job ([research.md R-1](research.md#r-1-import-videos-stored-as-a-set-tied-to-the-scan-record) to [R-5](research.md#r-5-the-denominator-during-a-scan-adds-files-not-yet-registered)). |
| Completion | The status becomes `done` or `partial` only when the scan is closed and every target video is settled. That time is stored once (R-4). |
| Issues | The events below are stored as issues of the latest import and returned as one item per video ([R-6](research.md#r-6-issues-stored-as-one-row-per-event-and-grouped-per-video-on-read), [R-7](research.md#r-7-generation-functions-return-substitution-as-a-result-value)). |
| Current activity | `internal/app` holds it in memory and is told per scanned file and at each job start and end ([R-8](research.md#r-8-current-activity-held-in-memory-by-internalapp-not-stored)). |
| API | `Scan` is reshaped, and `/api/processing` and the SSE `processing` event are removed. The bottom-right indicator and the Settings page read the same `Scan`; only the Settings page reads the issue list, through a separate route ([contracts/scan-api.md](contracts/scan-api.md)). |
| Screen shape | Look, wording and layout are decided in `ui-design.md` by the design stage after this Plan (the parent Issue has the `ui` label). This Plan decides the values the screen reads and what they mean. |

The issue events are:

- files the scan could not read, and files it could not register
- jobs that failed up to the retry limit
- substitutions for the representative thumbnail and the seek thumbnails

## Technical Context

**Canonical definitions**:

- Boundaries and dependency direction, domain events, `/api/events`, the
  rebuildable-data classification: [ARCHITECTURE.md](../../ARCHITECTURE.md)
  ("Intended dependency direction", "Rebuildable and user data", the events and
  SSE paragraphs)
- Today's scan: [internal/scanner/scanner.go](../../internal/scanner/scanner.go)
  (`Scan`, `progressInterval`, per-file failure logging),
  [internal/app/scans.go](../../internal/app/scans.go) (`StartScan`, `run`,
  `RecoverInterrupted`), [internal/store/scans.go](../../internal/store/scans.go)
- Today's jobs and failures: [internal/domain/job.go](../../internal/domain/job.go)
  (`MaxJobAttempts`, `JobStateAfterFailure`, `Processing`),
  [internal/store/jobs.go](../../internal/store/jobs.go) (`EnqueueJob`,
  `EnsureJob`, `FailClaimedJob`, `recordTerminalFailure`, `Processing`),
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go)
  (writing results, re-enqueuing regeneration, `RetryProbe`),
  [internal/app/ingest.go](../../internal/app/ingest.go) (`JobFinished`, enqueuing
  the preview after analysis), [internal/jobs/worker.go](../../internal/jobs/worker.go)
- Today's substitutions: [internal/media/thumbnail.go](../../internal/media/thumbnail.go)
  (retry with the first frame),
  [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)
  (three-stage generation),
  [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)
- Today's HTTP and SSE: [api/openapi.yaml](../../api/openapi.yaml) (`startScan`,
  `getCurrentScan`, `getProcessing`, `streamEvents`, `Scan`, `Processing`,
  `VideoFolder`), [internal/httpapi/scans.go](../../internal/httpapi/scans.go),
  [internal/httpapi/events.go](../../internal/httpapi/events.go),
  [cmd/mdm/events.go](../../cmd/mdm/events.go)
- Today's screens: [web/src/shell/ScanProvider.tsx](../../web/src/shell/ScanProvider.tsx),
  [scanPresentation.ts](../../web/src/shell/scanPresentation.ts),
  [ScanProgressIndicator.tsx](../../web/src/shell/ScanProgressIndicator.tsx),
  [ScanNoticeProvider.tsx](../../web/src/shell/ScanNoticeProvider.tsx),
  [web/src/settings/ScanStatusSection.tsx](../../web/src/settings/ScanStatusSection.tsx),
  [web/e2e/scan-progress.e2e.ts](../../web/e2e/scan-progress.e2e.ts). Today's UI
  decisions are in [specs/012-scan-progress/ui-design.md](../012-scan-progress/ui-design.md).
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`,
  `task check-docs`, `task generate`, `task test-e2e`)

**Feature-specific context**:

- No dependency is added.
- SQLite gains two tables and two columns ([data-model.md](data-model.md)).
  Migrations take the next numbers in `internal/store/migrations`.
- The SPA ships inside the binary and updates with it, so the new `Scan` shape and
  the removal of `/api/processing` keep no compatibility with the old SPA (as with
  earlier contract changes). So that the screen does not break between
  implementation units, the server units only add new fields. The old fields and
  `/api/processing` are removed by the unit that switches the screen.
- Screen text is English and follows the English i18n feature
  (`specs/023-english-i18n`, [#463](https://github.com/syudead/vv/pull/463); merged
  to `main` and into the feature branch)
  ([docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)).
  - The screen units put their strings in the
    [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) catalog, replacing
    `shell.scan` and `settings.scanStatus` with the words in
    [ui-design.md](ui-design.md). The ESLint rule and the pseudo-locale screen
    tests keep passing unchanged.
  - `Scan.errorCode` and `errorPath` keep 023's shape, and the reason text is
    written with `scanErrorText`. As in 023, `Scan.error` is not shown on screen
    ([R-10](research.md#r-10-the-spa-builds-screen-text-from-kinds-the-server-returns)).
  - As in 023, server logs and `message` are written directly in English.

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"):
  pass.

  | Package | Role in this feature |
  | --- | --- |
  | `internal/domain` | Holds how `status` is decided, issue kinds and severity, and how the denominator and settled count are computed, as pure functions and enums. |
  | `internal/media` | Only returns whether it substituted, as a value; knows neither events nor the store (R-7). |
  | `internal/scanner` | Passes file failures and the current file to a reporter it declares. |
  | `internal/jobs` | Announces job start with `Started`, shaped like today's `Finished`. |
  | `internal/app` | Holds the current activity in memory and assembles the import state. |
  | `internal/httpapi` | Only converts that state into `gen` types. |

  Sibling packages do not import each other (the depguard rules do not change).
- **Domain events** (the events paragraph of ARCHITECTURE.md): pass.
  `domain.ScanActivityChanged` is added, and subscriptions are registered only in
  `cmd/mdm/events.go`. The store publishes only after commit, as today.
- **API source of truth** (ARCHITECTURE.md): pass. Change `api/openapi.yaml` and
  run `task generate`; generated files are not hand-edited (AGENTS.md).
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): pass.
  `scan_videos`, `scan_issues` and `scans.settled_at` are rebuildable by rerunning
  the scan and preparation. No user-data table is touched. The unit that adds a
  table adds it to the list.
- **Owner only** (requirement 11, `accessRoutes` in `internal/httpapi/auth.go`):
  pass. The new route is owner-only and is not added to the table of routes
  returned to guests. The SPA subscribes only for the owner, as today.
- **Documents change in the same PR** (core-beliefs.md, AGENTS.md): pass. Each
  server unit updates the scan, job and event paragraphs; the design stage and the
  screen unit record that `specs/012-scan-progress/ui-design.md` is no longer the
  source of truth for the current UI.

The verdicts are the same after Phase 1. Complexity Tracking has no violation to
list.

## Project Structure

### Documentation (this feature)

```text
specs/024-import-progress/
├── plan.md                # This file
│                          # No spec.md — the parent Issue is the specification
├── research.md            # Decisions on the target set, completion, denominator, issues, substitutions, current activity, API, wording, migration
├── data-model.md          # scans.settled_at, scan_videos, scan_issues
└── contracts/
    └── scan-api.md        # New Scan shape, issue list route, removal of /api/processing, SSE
```

There is no `quickstart.md`. Validation runs in each unit's automated tests and in
files added to the existing `web/e2e` fixtures
([web/e2e/media-fixtures.mjs](../../web/e2e/media-fixtures.mjs)); there is no
feature-specific manual procedure. `ui-design.md` is written by the design stage
after this Plan.

### Source Code

**Affected boundaries**:

| Boundary | What changes |
| --- | --- |
| `internal/domain` | How `status` is decided, issue kinds and severity, denominator and settled counting, `ScanActivityChanged` |
| `internal/store` | Migrations; the helper that adds to the set and each enqueue site; `refreshScanSettled`; recording and clearing issues; reading the issue list; making the probe result and the preview enqueue one transaction |
| `internal/scanner` | Reporting per-file failures and the current file; removing `progressInterval` |
| `internal/media`, `internal/app` (`Generator`, `Ingest`) | Passing the substitution marker |
| `internal/jobs` | `Started` |
| `internal/app` (`Scans`) | Holding the current activity and assembling the import state |
| `api/openapi.yaml`, `internal/httpapi` (`scans.go`, `events.go`), `cmd/mdm` (`events.go`, `main.go`) | API and wiring |
| `web/src/api`, `web/src/shell`, `web/src/settings`, `web/e2e` | Screens and tests |
| `ARCHITECTURE.md` | Documentation |

**New paths**:

- `internal/store/migrations/000NN_scan_import.sql` (`scan_videos`,
  `scans.settled_at`, `scans.issues_revision`)
- `internal/store/migrations/000NN_scan_issues.sql`

Both take the next number at implementation time.

**Structure decision**: Follows the existing layout
([ARCHITECTURE.md](../../ARCHITECTURE.md)). The import state is assembled in
`Scans` in `internal/app`, because the current activity lives in memory and a
store read alone cannot return it. Having httpapi read both the store and app and
assemble the state was rejected, because how `status` is decided would leak into
the HTTP layer.

## Implementation Work

### Record the import's target videos and count settled videos and completion on the server

**Scope**: Add [data-model.md](data-model.md) §1 and §2 with a migration.

- Adding to the set: add to `scan_videos` at every site that enqueues a job, and
  when adding or relinking a media folder makes jobs claimable. `StartScan`
  carries over and replaces the set
  ([R-1](research.md#r-1-import-videos-stored-as-a-set-tied-to-the-scan-record),
  [R-3](research.md#r-3-the-set-holds-only-the-latest-scan-and-is-replaced-when-a-new-scan-starts)).
- Enqueuing the preview: inside `ApplyProbeForJob`
  ([R-2](research.md#r-2-probe-result-and-preview-job-enqueue-in-one-transaction)).
- Completion time: call `refreshScanSettled` in every transaction that can change
  the number of remaining jobs
  ([R-4](research.md#r-4-done-only-when-the-scan-is-closed-and-the-set-has-no-remaining-jobs)).
  The migration carries over the latest scan's unfinished jobs
  ([data-model.md](data-model.md) §1).
- Status rules: put `status`, the denominator and settled counting in
  `internal/domain`
  ([R-5](research.md#r-5-the-denominator-during-a-scan-adds-files-not-yet-registered)).
  This unit passes 0 as the failure issue count.
- Assembly: `Scans` in `internal/app` assembles the import state.
- API: add `status`, `videos` and `settledAt` from
  [contracts/scan-api.md](contracts/scan-api.md) §2 to `Scan`. Keep the old fields
  and `/api/processing`. `/api/events` also sends `scan` on `ProcessingChanged`
  (§4).
- Documentation: update the scan, rebuildable-data and SSE descriptions in
  ARCHITECTURE.md.

**Dependencies**: None.

**Acceptance**: The following checks exist, and `task check` and
`task check-docs` pass.

- `internal/domain` tests: the five `status` values and their precedence; the
  denominator and settled counting.
- `internal/store` tests confirm:
  - Registering 10 new files gives `videos.total = 10`. Finishing the jobs one by
    one raises `settled` from 0 to 10 without ever decreasing.
  - `settled` does not decrease while the probe finishes and the preview is
    enqueued.
  - An unchanged video that has a job re-enqueued is counted as a target.
  - The previous scan's unfinished videos are carried over into the new scan.
  - Re-enqueuing a missing preview and retrying analysis add the video to the
    latest scan's targets and clear `settled_at`.
  - When deleting a media folder makes a closed scan's remaining jobs
    unclaimable, `settled_at` is set.
  - Migrating with unfinished jobs puts those videos in the target and leaves
    `settled_at` `null`.
  - When a target video's row is deleted, it leaves the denominator.
  - Re-enqueuing a `running` job and restarting does not count `settled` twice.
- `internal/app` tests: after the scan closes, `status = running` stays while jobs
  remain. When the last job's outcome is recorded, the status becomes `done`, and
  `settledAt` is later than the scan's end.
- The `GET /api/scans/current` response has `status`, `videos` and `settledAt`.

### Record import failures as issues and return the list

**Scope**: Add [data-model.md](data-model.md) §3 with a migration.

- Scan failures: `internal/app` records the per-file failures from
  `internal/scanner` (`unreadable`, `changed_during_import`, `register_failed`)
  through a reporter the scanner declares.
- Job failures: record failures up to the retry limit (`*_failed`) in the same
  transaction as `recordTerminalFailure`, and clear them on a later success
  ([R-6](research.md#r-6-issues-stored-as-one-row-per-event-and-grouped-per-video-on-read)).
- Previous scan's issues: cleared in `StartScan`.
- Migration: videos whose preparation is currently `failed` become issues of the
  latest scan ([data-model.md](data-model.md) §1, step 2).
- Counting: reflect the issue counts and the files that failed to register in the
  denominator, the settled count, and `status = partial`.
- Revision: increment `scans.issues_revision` on every change to the issue rows.
- API: add `Scan.issues` (including `revision`) and
  `GET /api/scans/current/issues` ([contracts/scan-api.md](contracts/scan-api.md)
  §2 and §3).
- Documentation: add `scan_issues` to the rebuildable-data list in ARCHITECTURE.md.

**Dependencies**: `Record the import's target videos and count settled videos and completion on the server`.

**Acceptance**: The following checks exist, and `task check` and
`task check-docs` pass.

- `internal/scanner` tests: scanning a folder with an unreadable file passes that
  path to the reporter as `unreadable`.
- `internal/store` tests confirm:
  - A video whose analysis fails up to the limit becomes a `probe_failed` issue,
    and `Scan.status` becomes `partial`.
  - A video that fails before the limit and later succeeds is not an issue.
  - A successful analysis retry clears the issue.
  - Starting a new scan clears the previous issues.
  - Two kinds on one video merge into one item.
  - Thousands of issues can be walked to the end with the cursor, without
    overlap.
  - When another kind is added to a video that already has an issue, the count
    stays the same and `issues.revision` increases.
  - Migrating with a video whose analysis failed makes it a `probe_failed` issue,
    and the latest import becomes `partial`
    ([R-11](research.md#r-11-migration-moves-only-results-derivable-from-existing-rows-into-the-latest-scan)).
- `internal/httpapi` tests: `GET /api/scans/current/issues` returns:
  - the order and `nextCursor`
  - 400 for an invalid `cursor`
  - 404 when no scan has ever run
  - 401 and 403 for a guest

### Record representative-thumbnail and seek-thumbnail substitutions as issues

**Scope**: Record substitutions as issues
([R-7](research.md#r-7-generation-functions-return-substitution-as-a-result-value)).

- `internal/media`: `Thumbnail` returns whether it used the first frame, and
  `GenerateSeekSprite` whether it built from the whole video, as values.
- `internal/app`: `Generator` and `Ingest` pass the value to the store call that
  writes the result.
- `internal/store`: record `thumbnail_first_frame` and
  `seek_thumbnail_full_decode` in the transaction that writes the success; clear
  them when the output is rebuilt without substitution
  ([data-model.md](data-model.md) §3).
- Documentation: add to the generation paragraph of ARCHITECTURE.md that
  substitutions are reported.

**Dependencies**: `Record import failures as issues and return the list`.

**Acceptance**: The following checks exist, and `task check` and
`task check-docs` pass.

- `internal/media` tests (when ffmpeg is present): for an input with no frame at
  the requested position, the returned value says the representative thumbnail
  used the first frame. For an input where per-segment extraction fails, the
  returned value says the sprite was built from the whole video.
- `internal/app` and `internal/store` tests: an import with only substitutions
  ends with `status = done` and `issues.substituted = 1`. The issue list shows that
  video's kind.

### Report the current activity during an import and update progress per file

**Scope**: Report the current activity and update progress per file
([R-8](research.md#r-8-current-activity-held-in-memory-by-internalapp-not-stored)).

- `internal/app`: hold the current activity.
- `internal/scanner`: report the current file and progress per file, and drop
  `progressInterval`.
- `internal/jobs`: add the `Started` hook.
- Events: add `domain.ScanActivityChanged` and subscribe `/api/events` to it in
  `cmd/mdm/events.go`.
- API: add `Scan.activity` ([contracts/scan-api.md](contracts/scan-api.md) §2 and
  §4).
- Documentation: update the events and SSE paragraphs of ARCHITECTURE.md.

**Dependencies**: `Record the import's target videos and count settled videos and completion on the server`.

**Acceptance**: The following checks exist, and `task check` and
`task check-docs` pass.

- `internal/app` tests for the current activity:
  - When two jobs overlap, the one that started later is shown; when it ends, the
    other one is shown again.
  - When everything ends, `activity` is omitted.
- `internal/scanner` tests: scanning five files delivers a progress report per
  file.
- `internal/httpapi` tests: a change in the current activity sends a `scan`
  event whose `activity` has the file name and kind.

### Rebuild the bottom-right indicator and the Settings summary around video-count progress and the current activity

**Scope**: Rebuild the bottom-right indicator and the Settings summary.

- Specification: follows `ui-design.md` (written by the design stage).
- Screen state: build `ScanProvider`, `scanPresentation.ts` and the completion
  notice (`ScanNoticeProvider`) only from `Scan.status`, `videos`, `issues`
  (counts), `settledAt` and `activity`.
  - The completion notice appears when `status` becomes `done`, `partial` or
    `failed`.
  - The list reloads on a change in `Scan.state`.
- Bottom-right indicator: rebuild `ScanProgressIndicator`.
- Settings: rebuild the summary in `ScanStatusSection` (status, progress, current
  activity, issue counts, time, scan failure reason and retry).
- Removing old values: remove the following and run `task generate`:
  - the old `Scan` fields
  - `/api/processing` and the SSE `processing` event
  - `ProcessingBreakdown` and its fetch
- Tests: update `web/e2e/scan-progress.e2e.ts` and the e2e fixtures that read old
  fields.
- Documentation: record in `specs/012-scan-progress/ui-design.md` that it is no
  longer the current source of truth, as the design stage instructs.

**Dependencies**:

- `Record import failures as issues and return the list`
- `Report the current activity during an import and update progress per file`

**Acceptance**: This unit changes a screen, so look and interaction are checked
at 360px, 768px and 1280px. The following checks exist, and `task check`,
`task check-docs` and `task test-e2e` pass.

- Web unit tests confirm:
  - No ratio is shown during `finding`.
  - During `running`, one progress value "M of N" and the current activity are
    shown.
  - A completion with `videos.total = 0` shows that nothing changed.
  - There is no element for job counts or per-stage breakdowns.
  - `role="status"` announces only done, partial failure and failure.
  - A change in the current activity changes neither the announcement nor the
    layout.
- e2e confirms:
  - In a 10-video import, progress increases without changing unit.
  - The import is not shown as done while preparation remains.
  - The completion time marks the end of preparation.
  - After a reload, the same status and progress appear.
  - A guest does not see the bottom-right indicator.

### Show the import issue list on the Settings page with links to the videos

**Scope**: Show the issue list on the Settings page.

- Specification: follows `ui-design.md`.
- Display: add the `GET /api/scans/current/issues` list to `ScanStatusSection`.
  The impact and reason wording is built from `kinds`
  ([R-10](research.md#r-10-the-spa-builds-screen-text-from-kinds-the-server-returns)).
- Fetching: add a fetch function to `web/src/api`. Reload when `Scan.id` or
  `issues.revision` changes ([contracts/scan-api.md](contracts/scan-api.md) §4).
- Navigation: a registered video's row leads to `/videos/{id}`.
- Long lists: the user can load the rest.

**Dependencies**:

- `Rebuild the bottom-right indicator and the Settings summary around video-count progress and the current activity`
- `Record representative-thumbnail and seek-thumbnail substitutions as issues`

**Acceptance**: This unit changes a screen, so look and interaction are checked
at 360px, 768px and 1280px. The following checks exist, and `task check`,
`task check-docs` and `task test-e2e` pass.

- Web unit tests confirm:
  - Failures and substitutions are distinguished by text or icon, not only
    colour.
  - Long file names and files with the same name can be told apart.
  - The rest of the list can be loaded.
  - A new import replaces the list.
  - Each row can be reached by keyboard and leads to the video.
- e2e confirms:
  - Importing a fixture with an unreadable file and a video that cannot be
    analyzed makes the whole import a partial failure.
  - The list shows the impact and reason of those two items.
  - The row of the video that cannot be analyzed leads to the playback screen.
  - After a reload, the same list appears.
