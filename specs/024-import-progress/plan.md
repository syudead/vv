# Implementation Plan: rebuild import progress and results around what the user wants to know

**Branch**: `feature/024-import-progress` | **Parent Issue**: #444

**Input**: The parent Issue. It is this feature's specification.

## Summary

The current screen shows the scan ratio (`Scan` total / completed) and the remaining work per job
kind (`Processing`) as they are. This feature replaces them with one progress value ("M of N
target videos of the latest import settled"), one line for the current activity, and a per-video
issue list.

- **Targets and settled count**: the server holds the set of videos that belong to the latest
  scan. A video joins the set when a job is enqueued for it. Each read decides "settled" from the
  absence of remaining jobs for the video
  ([research.md R-1](research.md#r-1-store-the-import-targets-as-a-set-of-videos-tied-to-the-scan-record) to
  [R-5](research.md#r-5-while-scanning-add-target-files-not-yet-registered-to-the-denominator)).
- **Completion**: the state becomes "done" or "some failed" only when the scan has closed and
  every target video has settled. That time is stored once (R-4).
- **Issues**: the events below are stored as issues of the latest import and returned merged into
  one entry per video
  ([R-6](research.md#r-6-store-issues-as-one-row-per-event-and-merge-them-per-video-on-read),
  [R-7](research.md#r-7-the-generator-functions-return-substitution-as-a-result-value)).
  - Files the scan could not read, and files that could not be registered
  - Jobs that failed up to the retry limit
  - Substitutions for the main thumbnail and the seek thumbnails
- **Current activity**: `internal/app` holds it in memory. It is reported per scanned file and on
  each job start and finish ([R-8](research.md#r-8-current-activity-is-not-stored-internalapp-keeps-it-in-memory)).
- **API**: `Scan` is rebuilt, and `/api/processing` and the SSE `processing` event are removed.
  The bottom-right indicator and the Settings screen read the same `Scan`. Only the Settings
  screen reads the issue list, through a separate route
  ([contracts/scan-api.md](contracts/scan-api.md)).
- **Screen shape**: the design stage after this Plan decides the look, text and layout in
  `ui-design.md` (the parent Issue has the `ui` label). This Plan decides the values the screen
  reads and what they mean.

## Technical Context

**Canonical definitions**:

- Boundaries and dependency direction, domain events, `/api/events`, and the rebuildable data
  classification: [ARCHITECTURE.md](../../ARCHITECTURE.md) ("Intended dependency direction",
  "Rebuildable and user data", and the events and SSE paragraph)
- Current scan: [internal/scanner/scanner.go](../../internal/scanner/scanner.go)
  (`Scan`, `progressInterval`, per-file failure logging),
  [internal/app/scans.go](../../internal/app/scans.go) (`StartScan`, `run`, `RecoverInterrupted`),
  [internal/store/scans.go](../../internal/store/scans.go)
- Current jobs and failures: [internal/domain/job.go](../../internal/domain/job.go)
  (`MaxJobAttempts`, `JobStateAfterFailure`, `Processing`),
  [internal/store/jobs.go](../../internal/store/jobs.go) (`EnqueueJob`, `EnsureJob`,
  `FailClaimedJob`, `recordTerminalFailure`, `Processing`),
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go)
  (result writes, rebuild requeues, `RetryProbe`),
  [internal/app/ingest.go](../../internal/app/ingest.go) (`JobFinished`, the preview enqueue after
  probing), [internal/jobs/worker.go](../../internal/jobs/worker.go)
- Current substitutions: [internal/media/thumbnail.go](../../internal/media/thumbnail.go) (retry
  with the first frame), [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)
  (three-tier generation),
  [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)
- Current HTTP and SSE: [api/openapi.yaml](../../api/openapi.yaml) (`startScan`,
  `getCurrentScan`, `getProcessing`, `streamEvents`, `Scan`, `Processing`, `VideoFolder`),
  [internal/httpapi/scans.go](../../internal/httpapi/scans.go),
  [internal/httpapi/events.go](../../internal/httpapi/events.go),
  [cmd/mdm/events.go](../../cmd/mdm/events.go)
- Current screen: [web/src/shell/ScanProvider.tsx](../../web/src/shell/ScanProvider.tsx),
  [scanPresentation.ts](../../web/src/shell/scanPresentation.ts),
  [ScanProgressIndicator.tsx](../../web/src/shell/ScanProgressIndicator.tsx),
  [ScanNoticeProvider.tsx](../../web/src/shell/ScanNoticeProvider.tsx),
  [web/src/settings/ScanStatusSection.tsx](../../web/src/settings/ScanStatusSection.tsx),
  [web/e2e/scan-progress.e2e.ts](../../web/e2e/scan-progress.e2e.ts). The current UI decisions are
  in [specs/012-scan-progress/ui-design.md](../012-scan-progress/ui-design.md).
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`,
  `task generate`, `task test-e2e`)

**Feature-specific context**:

- No new dependencies.
- SQLite gains two tables and two columns ([data-model.md](data-model.md)). The migrations use the
  next numbers in `internal/store/migrations`.
- The new `Scan` shape and the removal of `/api/processing` do not keep compatibility with the old
  SPA, because the SPA ships inside the binary and updates with it (the same treatment as earlier
  contract changes). To keep the screen working between implementation units, the server units
  only add new fields. The unit that switches the screen removes the old fields and
  `/api/processing`.
- The screen text is English and follows the mechanism of the English localization feature
  (`specs/023-english-i18n`, [#463](https://github.com/syudead/vv/pull/463); merged into `main` and
  into the feature branch) ([docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)).
  - The screen units put text in the catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts).
    They replace `shell.scan` and `settings.scanStatus` with the text in
    [ui-design.md](ui-design.md). The ESLint rules and the pseudo-locale screen tests keep
    passing as they are.
  - `Scan.errorCode` and `errorPath` keep the 023 shape, and `scanErrorText` writes the reason
    text. `Scan.error` stays off the screen, as in 023
    ([R-10](research.md#r-10-the-spa-builds-the-screen-text-from-kinds-the-server-returns)).
  - Server logs and `message` are written directly in English, as in 023.

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"): pass.
  - `internal/domain`: owns the state rule (`status`), issue kinds and severities, and the
    denominator and settled counting, as pure functions and enums.
  - `internal/media`: only returns whether it substituted, as a value. It knows neither events nor
    the store (R-7).
  - `internal/scanner`: passes file failures and the current file to a reporter it declares.
  - `internal/jobs`: reports job start through `Started`, shaped like the existing `Finished`.
  - `internal/app`: holds the current activity in memory and assembles the import state.
  - `internal/httpapi`: only converts it to the `gen` types.
  - Sibling packages do not import each other (the depguard rules do not change).
- **Domain events** (ARCHITECTURE.md events paragraph): pass. `domain.ScanActivityChanged` is
  added, and subscriptions are registered only in `cmd/mdm/events.go`. The store still emits only
  after commit.
- **Canonical API** (ARCHITECTURE.md): pass. Change `api/openapi.yaml` and run `task generate`; do
  not hand-edit generated files (AGENTS.md).
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): pass. `scan_videos`,
  `scan_issues` and `scans.settled_at` are rebuildable by rerunning the scan and preparation. No
  user data table is touched. The unit that adds each table adds it to that list.
- **Owner-only exposure** (Requirement 11, `accessRoutes` in `internal/httpapi/auth.go`): pass.
  The new route is owner-only and is not added to the table of routes returned to guests. The SPA
  subscribes only for the owner, as today.
- **Docs change with the code in the same PR** (core-beliefs.md, AGENTS.md): pass. Each server unit
  updates the scan, job and event paragraphs. The design stage and the screen units record that
  `specs/012-scan-progress/ui-design.md` is no longer the canonical UI.

The result is the same after Phase 1. Complexity Tracking has no violations to list.

## Project Structure

### Documentation (this feature)

```text
specs/024-import-progress/
├── plan.md                # This file
│                          # No spec.md — the parent Issue is the specification
├── research.md            # Decisions: target set, completion, denominator, issues, substitution, current activity, API, text, migration
├── data-model.md          # scans.settled_at, scan_videos, scan_issues
└── contracts/
    └── scan-api.md        # New Scan shape, issue list route, /api/processing removal, SSE
```

No `quickstart.md`. Verification uses each unit's automated tests and files added to the existing
`web/e2e` fixtures ([web/e2e/media-fixtures.mjs](../../web/e2e/media-fixtures.mjs)). There is no
feature-specific manual procedure. The design stage after this Plan creates `ui-design.md`.

### Source Code

**Affected boundaries**:

- `internal/domain`: the `status` rule, issue kinds and severities, denominator and settled
  counting, `ScanActivityChanged`
- `internal/store`: migrations, the set-insert helper and every enqueue site,
  `refreshScanSettled`, recording and clearing issues, reading the issue list, and merging the
  probe result and the preview enqueue into one transaction
- `internal/scanner`: per-file failure and current-file reporting, removal of `progressInterval`
- `internal/media`, `internal/app` (`Generator`, `Ingest`): passing the substitution marker
- `internal/jobs`: `Started`
- `internal/app` (`Scans`): holding the current activity and assembling the import state
- `api/openapi.yaml`, `internal/httpapi` (`scans.go`, `events.go`), `cmd/mdm` (`events.go`,
  `main.go`)
- `web/src/api`, `web/src/shell`, `web/src/settings`, `web/e2e`
- `ARCHITECTURE.md`

**New paths**:

- `internal/store/migrations/000NN_scan_import.sql` (`scan_videos`, `scans.settled_at`,
  `scans.issues_revision`)
- `internal/store/migrations/000NN_scan_issues.sql`

Both use the next number available at implementation time.

**Structure decision**: follows the existing layout ([ARCHITECTURE.md](../../ARCHITECTURE.md)).

| | |
| --- | --- |
| **Decision** | `Scans` in `internal/app` assembles the import state. |
| **Why** | The current activity lives in memory, so a store read alone cannot return it. |
| **Rejected** | `internal/httpapi` reading both the store and the app and assembling the state: the `status` rule would leak into the HTTP layer. |

## Implementation Work

### Record the import's target videos and count settled videos and completion on the server

**Scope**: add §1 and §2 of [data-model.md](data-model.md) through a migration.
- Set membership: every job enqueue site, and adding or remapping a media folder that makes jobs
  startable, adds to `scan_videos`. `StartScan` carries over and replaces the set
  ([R-1](research.md#r-1-store-the-import-targets-as-a-set-of-videos-tied-to-the-scan-record),
  [R-3](research.md#r-3-keep-the-set-for-the-latest-scan-only-and-replace-it-when-a-new-scan-starts)).
- Preview enqueue: done inside `ApplyProbeForJob`
  ([R-2](research.md#r-2-enqueue-the-preview-job-in-the-same-transaction-as-the-probe-result)).
- Settle time: every transaction that can change the number of remaining jobs calls
  `refreshScanSettled` ([R-4](research.md#r-4-done-means-the-scan-closed-and-no-job-remains-in-the-set)).
  The migration carries the latest scan's unfinished jobs over ([data-model.md](data-model.md) §1).
- State rule: put `status`, the denominator and settled counting in `internal/domain`
  ([R-5](research.md#r-5-while-scanning-add-target-files-not-yet-registered-to-the-denominator)). This
  unit passes 0 as the failed-issue count.
- Assembly: `Scans` in `internal/app` assembles the import state.
- API: add `status`, `videos` and `settledAt` from [contracts/scan-api.md](contracts/scan-api.md)
  §2 to `Scan`. Keep the old fields and `/api/processing`. `/api/events` also sends `scan` on
  `ProcessingChanged` (§4).
- Docs: update the ARCHITECTURE.md text on scans, rebuildable data and SSE.

**Dependencies**: None.

**Acceptance**: the checks below exist, and `task check` and `task check-docs` pass.
- `internal/domain` tests: the five `status` states and their precedence; denominator and settled
  counting.
- `internal/store` tests verify:
  - Registering 10 new files gives `videos.total = 10`. Finishing the jobs one by one raises
    `settled` from 0 to 10 without ever decreasing.
  - `settled` does not decrease while a probe finishes and its preview is enqueued.
  - An unchanged video counts as a target when a job is requeued for it.
  - Unfinished videos from the previous import carry over to the new scan.
  - Requeueing a missing preview and retrying a probe add the video to the latest scan's targets
    and clear `settled_at`.
  - When removing a media folder makes a closed scan's remaining jobs unstartable, `settled_at` is
    set.
  - Migrating with unfinished jobs puts those videos in the targets and leaves `settled_at` as
    `null`.
  - Deleting a target video's row removes it from the denominator.
  - Requeueing a `running` job and restarting does not count it twice in `settled`.
- `internal/app` tests: `status = running` holds while jobs remain after the scan closes. It
  becomes `done` when the last job outcome is recorded, and `settledAt` is later than the scan's
  finish.
- The `GET /api/scans/current` response has `status`, `videos` and `settledAt`.

### Record import failures as issues and return the list

**Scope**: add §3 of [data-model.md](data-model.md) through a migration.
- Scan failures: `internal/app` records the per-file failures of `internal/scanner`
  (`unreadable`, `changed_during_import`, `register_failed`) through a reporter the scanner
  declares.
- Job failures: record failures up to the retry limit (`*_failed`) in the same transaction as
  `recordTerminalFailure`. A later success clears them
  ([R-6](research.md#r-6-store-issues-as-one-row-per-event-and-merge-them-per-video-on-read)).
- Previous scan's issues: `StartScan` deletes them.
- Migration: videos with a currently `failed` preparation become issues of the latest scan
  ([data-model.md](data-model.md) §1, step 2).
- Counting: reflect the issue counts and the files that could not be registered in the
  denominator, the settled count and `status = partial`.
- Revision: increment `scans.issues_revision` whenever issue rows change.
- API: add `Scan.issues` (including `revision`) and `GET /api/scans/current/issues`
  ([contracts/scan-api.md](contracts/scan-api.md) §2 and §3).
- Docs: add `scan_issues` to the rebuildable data list in ARCHITECTURE.md.

**Dependencies**: `Record the import's target videos and count settled videos and completion on the server`.

**Acceptance**: the checks below exist, and `task check` and `task check-docs` pass.
- `internal/scanner` tests: scanning a folder with an unreadable file passes that path to the
  reporter as `unreadable`.
- `internal/store` tests verify:
  - A video whose probe fails up to the limit becomes a `probe_failed` issue, and `Scan.status`
    becomes `partial`.
  - A video that fails below the limit and succeeds later is not an issue.
  - A successful probe retry clears the issue.
  - Starting a new scan clears the previous issues.
  - Two kinds on one video merge into one entry.
  - A cursor walks thousands of issues to the end without overlap.
  - Adding another kind to a video with an issue raises `issues.revision` without changing the
    count.
  - Migrating with a video whose probe failed makes it a `probe_failed` issue, and the latest
    import becomes `partial`
    ([R-11](research.md#r-11-the-migration-moves-into-the-latest-scan-only-the-results-the-current-rows-reveal)).
- `internal/httpapi` tests: for `GET /api/scans/current/issues`, verify these responses:
  - Ordering and `nextCursor`
  - 400 for an invalid `cursor`
  - 404 when no scan has ever run
  - 401 and 403 for a guest

### Record main-thumbnail and seek-thumbnail substitutions as issues

**Scope**: record substitutions as issues
([R-7](research.md#r-7-the-generator-functions-return-substitution-as-a-result-value)).
- `internal/media`: `Thumbnail` returns whether it used the first frame, and `GenerateSeekSprite`
  returns whether it built from the whole video, as values.
- `internal/app`: `Generator` and `Ingest` pass that value to the store call that writes the
  result.
- `internal/store`: the transaction that writes the success records `thumbnail_first_frame` and
  `seek_thumbnail_full_decode`. A rebuild without substitution clears them
  ([data-model.md](data-model.md) §3).
- Docs: add substitution reporting to the generation paragraph in ARCHITECTURE.md.

**Dependencies**: `Record import failures as issues and return the list`.

**Acceptance**: the checks below exist, and `task check` and `task check-docs` pass.
- `internal/media` tests (when ffmpeg is available): an input with no frame at the requested
  position returns a value showing the main thumbnail used the first frame. An input where
  per-segment extraction fails returns a value showing the sprite was built from the whole video.
- `internal/app` and `internal/store` tests: an import with only substitutions gives
  `status = done` and `issues.substituted = 1`. The issue list shows that video's kind.

### Report the current activity during an import and update progress per file

**Scope**: report the current activity and update progress per file
([R-8](research.md#r-8-current-activity-is-not-stored-internalapp-keeps-it-in-memory)).
- `internal/app`: holds the current activity.
- `internal/scanner`: reports the current file and progress per file, and drops
  `progressInterval`.
- `internal/jobs`: adds the `Started` hook.
- Events: add `domain.ScanActivityChanged` and subscribe `/api/events` to it in
  `cmd/mdm/events.go`.
- API: add `Scan.activity` ([contracts/scan-api.md](contracts/scan-api.md) §2 and §4).
- Docs: update the events and SSE paragraphs in ARCHITECTURE.md.

**Dependencies**: `Record the import's target videos and count settled videos and completion on the server`.

**Acceptance**: the checks below exist, and `task check` and `task check-docs` pass.
- `internal/app` tests for the current activity:
  - When two jobs overlap, it shows the one that started later. When that one finishes, it falls
    back to the remaining one.
  - When everything finishes, `activity` is omitted.
- `internal/scanner` tests: scanning 5 files delivers a progress report per file.
- `internal/httpapi` tests: a change in the current activity sends a `scan` event whose
  `activity` has the file name and kind.

### Rebuild the bottom-right indicator and the Settings summary around video-count progress and the current activity

**Scope**: rebuild the bottom-right indicator and the Settings summary.
- Spec: follow `ui-design.md` (created by the design stage).
- Screen state: `ScanProvider`, `scanPresentation.ts` and the completion notice
  (`ScanNoticeProvider`) build only from `Scan.status`, `videos`, `issues` (counts), `settledAt`
  and `activity`.
  - The completion notice appears when `status` becomes `done`, `partial` or `failed`.
  - The list reloads on changes to `Scan.state`.
- Bottom-right indicator: rebuild `ScanProgressIndicator`.
- Settings: rebuild the `ScanStatusSection` summary (state, progress, current activity, issue
  counts, time, scan failure reason and retry).
- Remove old values: remove the items below and run `task generate`.
  - The old `Scan` fields
  - `/api/processing` and the SSE `processing` event
  - `ProcessingBreakdown` and its fetch
- Tests: update `web/e2e/scan-progress.e2e.ts` and the e2e fixtures that read old fields.
- Docs: update `specs/012-scan-progress/ui-design.md` to say it is no longer canonical, as the
  design stage instructs.

**Dependencies**:
- `Record import failures as issues and return the list`
- `Report the current activity during an import and update progress per file`

**Acceptance**: this unit changes the screen, so check the look and interaction at 360 px, 768 px
and 1280 px widths. The checks below exist, and `task check`, `task check-docs` and
`task test-e2e` pass.
- Web unit tests verify:
  - No ratio appears during `finding`.
  - During `running`, one progress value "M of N videos" and the current activity appear.
  - A completion with `videos.total = 0` shows that nothing changed.
  - No element shows job counts or per-stage breakdowns.
  - `role="status"` announces only done, some failed and failed.
  - A change in the current activity changes neither the announcement nor the layout.
- e2e tests verify:
  - In a 10-video import, progress increases without changing its unit.
  - "Done" does not appear while preparation remains.
  - The completion time marks the end of preparation.
  - The same state and progress appear after a reload.
  - Guests do not see the bottom-right indicator.

### Show the import issue list on the Settings screen and link to the videos

**Scope**: show the issue list on the Settings screen.
- Spec: follow `ui-design.md`.
- Display: add the `GET /api/scans/current/issues` list to `ScanStatusSection`. Build the impact
  and reason text from `kinds`
  ([R-10](research.md#r-10-the-spa-builds-the-screen-text-from-kinds-the-server-returns)).
- Fetch: add a fetch function to `web/src/api`. Reload when `Scan.id` or `issues.revision`
  changes ([contracts/scan-api.md](contracts/scan-api.md) §4).
- Navigation: rows of registered videos link to `/videos/{id}`.
- Long lists: the user can load the rest.

**Dependencies**:
- `Rebuild the bottom-right indicator and the Settings summary around video-count progress and the current activity`
- `Record main-thumbnail and seek-thumbnail substitutions as issues`

**Acceptance**: this unit changes the screen, so check the look and interaction at 360 px, 768 px
and 1280 px widths. The checks below exist, and `task check`, `task check-docs` and
`task test-e2e` pass.
- Web unit tests verify:
  - Failures and substitutions differ by text or icon, not by color alone.
  - Long file names and files with the same name can be told apart.
  - The rest of the list can be loaded.
  - A new import replaces the list.
  - Each row is reachable by keyboard and navigates to the video.
- e2e tests verify:
  - Importing a fixture with an unreadable file and an unprobeable video makes the whole import
    "some failed".
  - The list shows the impact and reason for those 2 entries.
  - The row of the unprobeable video navigates to the player.
  - The same list appears after a reload.
