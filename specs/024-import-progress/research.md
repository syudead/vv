# Research: rebuild import progress and results around what the user wants to know

The tech stack, dependency direction and event distribution follow
[ARCHITECTURE.md](../../ARCHITECTURE.md) and
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md). This
feature does not change them. This document records only the decisions this feature adds.
[data-model.md](data-model.md) has the table and column shapes, and
[contracts/scan-api.md](contracts/scan-api.md) has the HTTP and SSE shapes.

## R-1: Store the import targets as a set of videos tied to the scan record

| | |
| --- | --- |
| **Decision** | Store the set of videos that belong to the latest scan (the newest `scans` row) in `scan_videos`. A video joins the latest scan when a job (`jobs`) is enqueued, in the same transaction. "Settled" is not stored. A read derives it: the video has no `queued` or `running` job left. The job state is the single source of truth, so nothing is counted twice after a stop or a restart. |
| **Why** | The denominator in Requirement 1 is "videos that need preparation in that import", not only files the scan found changed. It includes jobs requeued for unchanged videos, jobs carried over from the previous import, and jobs enqueued outside the import action: rebuilding previews and seek thumbnails, probe retry, and adding or remapping a media folder (Edge cases). Every one of these entry points is a place where `internal/store` inserts into `jobs`, so adding to the set there misses nothing. |
| **Rejected** | Counting videos with `queued` or `running` jobs on each read, without a set: a video leaves the denominator when its jobs finish, so the denominator shrinks instead of "settled" growing. That is not progress. A per-video "settled at" column: it becomes a second source of truth next to the job state and needs realigning after every restart or retry. |

## R-2: Enqueue the preview job in the same transaction as the probe result

| | |
| --- | --- |
| **Decision** | `ApplyProbeForJob` also enqueues the preview job. Today `internal/app` writes the probe result and then calls `EnqueueJob(preview)` in a separate transaction. |
| **Why** | Acceptance criterion 3: "the settled count never decreases". Under the R-1 rule, no job remains between the two transactions, so the video counts as settled for an instant and then drops back. The probe job stays `running` until `CompleteClaimedJob`, so enqueueing in the same transaction as the result leaves no gap. |
| **Rejected** | Adding a rule that a probed video is not settled until its preview job is enqueued: it guesses "the job that should be enqueued next" from combinations of state columns, and the rule needs fixing whenever a stage is added. |

## R-3: Keep the set for the latest scan only, and replace it when a new scan starts

| | |
| --- | --- |
| **Decision** | The `StartScan` transaction deletes the previous scan's `scan_videos` and `scan_issues`. Videos that still have `queued` or `running` jobs at that point move into the new scan's set. |
| **Why** | Requirement 7 (the issue list covers the latest import only) and the Edge cases entry "starting a new import before the previous preparation finishes". Past import history is out of scope, so there is no reason to keep old rows. |
| **Rejected** | Keeping rows per scan and filtering to the latest on read: rows grow without bound for history that is out of scope, and the carry-over check has to read old scan rows every time. |

## R-4: "Done" means the scan closed and no job remains in the set

The user-facing state comes from a pure function in `internal/domain` with these inputs:

- The scan state (`running`, `done`, `failed`)
- Whether the scan has finished counting its target files
- The set size and the number of settled videos in it
- The issue counts, split into failed and substituted

The function returns one of five states: `finding`, `running`, `done`, `partial`, `failed`.

- `failed` means the scan itself failed, and it takes precedence over the others.
- `done` and `partial` apply only when the scan is closed and no job remains. Whether any failed
  issue exists decides between them.

| | |
| --- | --- |
| **Decision** | The pure function above decides the state. The settle time is stored in `scans.settled_at`. `internal/store` calls `refreshScanSettled` before commit in every transaction that can change the number of remaining jobs: the transactions that emit `ProcessingChanged` today, job claiming, recording job outcomes, and scan completion. This includes transactions that enqueue jobs, delete video rows, and make jobs unstartable by removing or remapping a media folder. When the latest scan is closed and no job remains in the set, `refreshScanSettled` sets the time only if it is empty. When jobs remain, it resets the value to `null`. The call hangs off the change record (`changes`) that collects events for after commit, so new write paths cannot forget it. |
| **Why** | Requirements 2, 4 and 6. The state is derived from stored rows every time, so it is the same after a reload or a restart (Requirement 10). The only value that needs storing is "the time everything settled", which cannot be derived later. |
| **Rejected** | Deriving the settle time on each read from the maximum `updated_at` of the set's jobs: job rows disappear on requeue or video deletion, so the time changes after the fact. Storing the state as a column: it can disagree with the job state. Having `internal/app` evaluate the state only after scan completion and job outcomes: when removing or remapping a media folder, or deleting a video row, leaves no remaining work, nothing calls it and the time stays missing. |

## R-5: While scanning, add "target files not yet registered" to the denominator

| | |
| --- | --- |
| **Decision** | While scanning, the denominator is the sum of: the set size; the number of files that could not be registered (issues); and the target files the scan counted but has not tried to register yet (today's `scans.total` − `completed` − `failed`). The settled count is the sum of: the settled videos in the set; and the number of files that could not be registered (they ended as failures, so they count as settled). Until the scan finishes counting its targets (today, while `total` stays 0 during enumeration), the state is `finding` and no denominator is returned. |
| **Why** | Requirement 1: "do not show a ratio while the targets are still unknown", and show "M of 10" from partway through registration. Enumeration counts every root before registration starts ([internal/scanner/scanner.go](../../internal/scanner/scanner.go), `Scan`), so the count after enumeration is a reliable estimate of the denominator. The denominator can shrink by 1 only when a carried-over video's file changed and is registered again. The settled count never decreases, so acceptance criterion 3 holds. |
| **Rejected** | Withholding the denominator until the scan closes: harmless for a scan that ends in seconds, but it cannot show "M of 10" while many files are being registered. |

## R-6: Store issues as one row per event, and merge them per video on read

| | |
| --- | --- |
| **Decision** | `scan_issues` stores one row per event (latest scan, video or file, kind). The API returns one entry per video (per file path when unregistered). The merged entry's severity is "failed" if it contains any failed kind, else "substituted". Entries are ordered failed first, then by file name within the same severity, and the list is paginated by cursor. Recording points: a job failing up to the limit is recorded in the same transaction as `recordTerminalFailure`; a file failure during the scan goes through a reporter that `internal/scanner` declares; a substitution comes from the marker the generator returns (R-7). When the same stage later succeeds, the transaction that writes the success deletes that video's failure row for that stage. |
| **Why** | Requirement 5, and the Edge cases entries "a video has both a failure and a substitution" and "there are thousands of issues". With one row per event, each writer inserts only its own event, and the merge and severity rules live in one read path. Deleting a video row removes its issues through `on delete cascade`, so a video that leaves the target is not counted as an issue (Edge cases). A video that succeeds on probe retry leaves the list (acceptance criterion 9). |
| **Rejected** | One row per video, updated on every event: writers need to know the severity rule and how to merge with the existing row, and undoing only one stage on a later success gets hard. |

## R-7: The generator functions return substitution as a result value

| | |
| --- | --- |
| **Decision** | The generator functions in `internal/media` return whether they substituted. `Thumbnail`: whether the frame at the requested position was unavailable and the first frame was used. `GenerateSeekSprite`: whether per-segment extraction also failed and the sprite was built from the whole video. `Generator` in `internal/app` receives the value and passes it to the store call that writes the result. An input that cannot use the index and takes the per-segment extraction path does not count as a substitution. |
| **Why** | Requirement 5 covers two cases: "the main thumbnail uses the first frame instead" and "the seek thumbnails were rebuilt from the whole video". Per-segment extraction is the normal path for inputs without a usable index, and the visual quality is the same ([seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)). Returning a value keeps `internal/media` unaware of events and the store, so the dependency direction does not change. |
| **Rejected** | `internal/media` calling a reporter on each substitution, like a log: it brings the context of which video and which scan into `internal/media`. |

## R-8: Current activity is not stored; `internal/app` keeps it in memory

| | |
| --- | --- |
| **Decision** | `internal/app` holds a value for the current activity. Contents: what is being done (`registering`, `probe`, `thumbnail`, `seekThumbnail`, `preview`), the video id (if any) and the file path. Reporters: the scanner reports once per file; the worker reports on job start and finish (`Started` in `jobs` and the existing `Finished`). With several running at once, it shows the one that started last and falls back to one of the remaining running ones when that finishes. Changes emit `domain.ScanActivityChanged`. `/api/events` coalesces this change like the existing `scan` and reads the value at send time. The scanner stops reporting counts only every 20 files (`progressInterval`) and reports progress once per file. |
| **Why** | Requirement 3 is "the user can tell processing has not stalled". There is no reason to keep it across a restart; after a restart, showing whatever is running then is enough. Sends are coalesced per connection ([internal/httpapi/events.go](../../internal/httpapi/events.go)), so per-file reports do not pile stale values onto slow connections. |
| **Rejected** | Writing the current activity to a `scans` column: it adds a write per file and per job, and a restart leaves a stale value behind. |

## R-9: The API rebuilds `/api/scans` and removes `/api/processing` and the SSE `processing` event

| | |
| --- | --- |
| **Decision** | Rebuild `Scan` around the user-facing state, counts, issue counts, settle time and current activity ([contracts/scan-api.md](contracts/scan-api.md)). `GET /api/scans/current/issues` returns the issue list. The route names (`/api/scans`, SSE `scan`) stay. The per-job-kind counts (`Processing`) leave the API. The scan-stage state `state` stays for reloading the list and for retry, and is not shown on screen. |
| **Why** | Requirement 8: the bottom-right indicator and the Settings screen show the summary and the whole of the same values. Both reading the same single response achieves this. Route names are invisible to users, so renaming gains nothing; it would rewrite every e2e test and test fixture. |
| **Rejected** | A new `/api/imports`: same behavior, and only more rewriting of callers and fixtures. Keeping `Processing` and adding the new values: the screen could again assemble per-stage numbers, and the API would not enforce the replacement in Requirement 1. |

## R-10: The SPA builds the screen text from kinds the server returns

| | |
| --- | --- |
| **Decision** | The API returns the issue impact and reason, the current activity and the scan failure reason as kinds (enum values). The SPA turns them into user-facing text. |
| **Why** | The English localization feature ([#463](https://github.com/syudead/vv/pull/463), `specs/023-english-i18n`; [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)) collects screen strings in the SPA catalog `web/src/i18n/en.ts`, does not show the API `message` on screen, and returns failure reasons as codes for the screen to phrase. Returning kinds keeps that policy and lets the SPA alone decide the text. The catalog can hold a `Record` keyed by kind, so type checking catches a missing kind. The scan failure reason carries over `errorCode` and `errorPath`, which 023 added to `Scan`. |
| **Rejected** | The server returning English sentences: it conflicts with the 023 policy (do not show `message` on screen), and adding a language would change the API. |

## R-11: The migration moves into the latest scan only the results the current rows reveal

| | |
| --- | --- |
| **Decision** | The migration inserts two things into the latest scan ([data-model.md §1](data-model.md)): videos with unfinished jobs go into `scan_videos`; videos with any preparation currently `failed` become `*_failed` issues. The set of videos the scan registered and the file failures during the scan are not in the pre-migration rows, so they are not restored. Right after the migration, the latest import shows `partial` if failures remain and `running` if unfinished jobs exist. The counts cover only the moved videos and issues. The next import replaces them (R-3). |
| **Why** | Showing the newest pre-migration scan as "0 videos, no issues" would report a failed import as done. Preparation failures remain as state on the video rows, so the impact on the user (Requirement 5, "what state the video is in") shows correctly. The registered count cannot serve as the denominator: today's `scans.total` counts "changed files", not the video set. |
| **Rejected** | Adding a "result unknown" state only for the pre-migration scan: a one-time migration would bring a sixth state, with its own text and display, into the screen. |
