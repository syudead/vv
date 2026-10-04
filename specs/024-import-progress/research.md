# Research: Rebuild import progress and results around what the user wants to know

Inherited decisions: the tech stack, the dependency direction and event delivery
follow [ARCHITECTURE.md](../../ARCHITECTURE.md) and
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
and this feature does not change them. This file records only the decisions this
feature adds. Table and column shapes are in [data-model.md](data-model.md); the
HTTP and SSE shapes are in [contracts/scan-api.md](contracts/scan-api.md).

## R-1: Import videos stored as a set tied to the scan record

**Decision**: Store the set of videos that belong to the latest scan (the newest
`scans` row) in `scan_videos`. A video joins the latest scan when a job (`jobs`)
is enqueued for it, inside the same transaction. "Settled" is not stored: it is
derived on read from "the video has no `queued` or `running` job left". The job
state itself is the source of truth, so a stop and restart never counts a video
twice.

**Rationale**: The denominator in requirement 1 is "the videos that need
preparing in this import", which is not limited to files the scan found changed.
It also covers jobs re-enqueued for unchanged videos, jobs carried over from the
previous scan, and jobs enqueued outside the import action (regenerating
previews and seek thumbnails, retrying analysis, adding or relinking a media
folder) (Edge Cases). Every one of these entry points is a place where
`internal/store` inserts a row into `jobs`, so adding the video to the set there
misses none.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Keep no set; count videos with a `queued` or `running` job each time | Rejected: a video leaves the denominator when its jobs finish, so the denominator shrinks instead of "settled" growing. That is not progress. |
| Store a per-video "settled at" column | Rejected: a second source of truth beside the job state, which has to be reconciled after every restart and retry. |

## R-2: Probe result and preview job enqueue in one transaction

**Decision**: Enqueue the preview job inside `ApplyProbeForJob`. Today
`internal/app` writes the probe result and then calls `EnqueueJob(preview)` in a
separate transaction.

**Rationale**: Acceptance criterion 3 says the settled count never decreases.
Under the R-1 rule, between the two transactions the video has no job left, so it
counts as settled for a moment and then drops back. The probe job stays `running`
until `CompleteClaimedJob`, so enqueuing in the same transaction as the result
leaves no gap.

**Alternatives considered**: Add a rule that a probed video is not settled until
its preview job is enqueued. Rejected: it infers "the job that should be enqueued
next" from combinations of state columns, and the rule must change whenever a
stage is added.

## R-3: The set holds only the latest scan and is replaced when a new scan starts

**Decision**: In the `StartScan` transaction, delete the previous scan's
`scan_videos` and `scan_issues` rows. Videos that still have a `queued` or
`running` job move into the new scan's set.

**Rationale**: Requirement 7 (the issue list covers only the latest import) and
the Edge Case "a new import starts before the previous preparation finishes".
History of past imports is out of scope, so there is no reason to keep old rows.

**Alternatives considered**: Keep rows per scan and filter to the latest scan on
read. Rejected: rows grow without bound for history that is out of scope, and the
carry-over decision has to read the old scan's rows every time.

## R-4: Done only when the scan is closed and the set has no remaining jobs

**Decision**: A pure function in `internal/domain` derives the status shown to
the user from these inputs:

- the scan state (`running`, `done`, `failed`)
- whether the scan has finished counting its target files
- the number of videos in the set, and how many of them are settled
- the issue counts, split into failures and substitutions

There are five statuses: `finding`, `running`, `done`, `partial`, `failed`.

- `failed` means the scan itself failed, and takes precedence over all others.
- `done` and `partial` apply only when the scan is closed and no job remains.
  Which of the two depends on whether there is a failure issue.

The completion time is stored in `scans.settled_at`. `internal/store` calls
`refreshScanSettled` before commit in every transaction that can change the
number of remaining jobs: the transactions that publish `ProcessingChanged` today,
job claiming, recording a job's success or failure, and finishing a scan. This
includes transactions that enqueue jobs, delete video rows, and make jobs
unclaimable by deleting or relinking a media folder. `refreshScanSettled` sets the
time only when it is empty, provided the latest scan is closed and the set has no
remaining job; when jobs remain it resets the value to `null`. The call hangs off
the change record (`changes`) that collects events for after commit, so a new
write path cannot forget it.

**Rationale**: Requirements 2, 4 and 6. The status is derived from stored rows
every time, so it is the same after a reload or a restart (requirement 10). The
only value that must be stored is "the time everything settled", which cannot be
derived later.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Derive the completion time on each read from the maximum `updated_at` of the set's jobs | Rejected: job rows disappear on re-enqueue and video deletion, so the time changes after the fact. |
| Store the status as a column | Rejected: it can disagree with the job state. |
| Have `internal/app` evaluate only after a scan finishes and after each job outcome | Rejected: when deleting or relinking a media folder or deleting a video row leaves no remaining job, nothing calls it and the time is never set. |

## R-5: The denominator during a scan adds files not yet registered

**Decision**: During a scan, the denominator is the sum of:

- the number of videos in the set
- the number of "registration failed" file issues
- the number of target files the scan counted but has not yet tried to register
  (today's `scans.total` − `completed` − `failed`)

The settled count is the sum of:

- the settled videos in the set
- the number of "registration failed" file issues (they ended as failures, so
  they count as settled)

Until the scan finishes counting its targets (today's "enumerating with `total`
still 0"), the status is `finding` and no denominator is returned.

**Rationale**: Requirement 1 says not to show a ratio while the target is still
unknown, and to show "N of 10" while registration is in progress. The scan's
enumeration counts all roots before it starts registering (`Scan` in
[internal/scanner/scanner.go](../../internal/scanner/scanner.go)), so the count
after enumeration is a reliable estimate of the denominator. The denominator can
drop by one only when a carried-over video's file changed and is registered
again. The settled count never decreases, so acceptance criterion 3 holds.

**Alternatives considered**: Return no denominator until the scan closes.
Rejected: harmless for a scan that ends in seconds, but it cannot show "N of 10"
while many files are being registered.

## R-6: Issues stored as one row per event and grouped per video on read

**Decision**: Store one row per event in `scan_issues` (latest scan, video or
file, kind). The API returns one item per video (per file path when the file is
not registered):

- An item's severity is "failure" when it contains at least one failure kind, and
  "substitution" otherwise.
- Failures come first; within a severity, items are ordered by file name.
- The list is paged with a cursor.

Issues are recorded at these points:

| Event | Where it is recorded |
| --- | --- |
| A job fails up to the retry limit | In the same transaction as `recordTerminalFailure` |
| A file fails during the scan | Through a reporter that `internal/scanner` declares |
| A substitution | From the marker the generation result returns (R-7) |

When the same stage later succeeds, the transaction that writes the success
deletes that video's failure row for that stage.

**Rationale**: Requirement 5 and the Edge Cases "one video has both a failure and
a substitution" and "there are thousands of issues". With one row per event, each
writer inserts only its own event, and the grouping and severity rules live in one
place, the read. Deleting a video row deletes its issues through
`on delete cascade`, so a video that left the target is not counted as an issue
either (Edge Cases). A video whose analysis succeeds on retry drops out of the
list (acceptance criterion 9).

**Alternatives considered**: One row per video, updated on each event. Rejected:
every writer would need to know the severity rules and how to merge with the
existing row, and undoing only part of the row when one stage succeeds becomes
hard.

## R-7: Generation functions return substitution as a result value

**Decision**: The generation functions in `internal/media` return whether they
substituted, as a value:

- `Thumbnail`: the frame at the requested position could not be taken and the
  first frame was used.
- `GenerateSeekSprite`: per-segment extraction also failed and the sprite was
  built from the whole video.

`Generator` in `internal/app` receives the value and passes it to the store call
that writes the result. The path where an input that cannot be built from the
index goes on to per-segment extraction is not counted as a substitution.

**Rationale**: Requirement 5 covers two cases: "the representative thumbnail was
substituted with the first frame" and "the seek thumbnails were rebuilt from the
whole video". Per-segment extraction is the normal path for inputs whose index
cannot be used, and the visual quality does not change
([seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)).
Returning a value keeps `internal/media` unaware of events and the store, so the
dependency direction does not change.

**Alternatives considered**: `internal/media` calls a reporter on every
substitution, like logging. Rejected: it brings the context of which video and
which scan into `internal/media`.

## R-8: Current activity held in memory by `internal/app`, not stored

**Decision**: `internal/app` holds a value for the current activity.

| Aspect | Decision |
| --- | --- |
| What it holds | What is happening (`registering`, `probe`, `thumbnail`, `seekThumbnail`, `preview`), the video id (when there is one), the file path |
| Who reports it | The scan reports per file; the worker reports at job start and end (`Started` in `jobs`, and today's `Finished`) |
| Several running at once | Show the one that started last; when it ends, fall back to one of the others still running |
| How a change is announced | Publish `domain.ScanActivityChanged` |

`/api/events` coalesces this change the same way as today's `scan` and reads the
value at send time.

Reporting the scan count only every 20 files (`progressInterval`) stops; progress
is reported per file.

**Rationale**: Requirement 3 is that the user can tell processing has not stalled,
so there is no point keeping the value across a restart. After a restart, showing
whatever is running at that moment is enough. Sends are coalesced per connection
([internal/httpapi/events.go](../../internal/httpapi/events.go)), so reporting per
file does not pile up stale values on a slow connection.

**Alternatives considered**: Write the current activity to a `scans` column.
Rejected: it adds a write per file and per job, and a stale value survives a
restart.

## R-9: API reshapes `/api/scans` and removes `/api/processing` and the SSE `processing` event

**Decision**: Reshape `Scan` into the user-facing status, video counts, issue
counts, completion time and current activity
([contracts/scan-api.md](contracts/scan-api.md)). The issue list is returned by
`GET /api/scans/current/issues`.

- The route names (`/api/scans`, the SSE `scan` event) stay.
- The per-job-kind counts (`Processing`) leave the API.
- The scan-stage state `state` stays for reloading the list and for retry, and is
  not shown on screen.

**Rationale**: Requirement 8 is that the bottom-right indicator and the Settings
page show a summary and the whole of the same values. Both reading the same single
response achieves that. Route names are invisible to users, so renaming them gains
nothing; renaming would rewrite every e2e and test fixture.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Add a new `/api/imports` | Rejected: same behaviour, only more caller and fixture rewrites. |
| Keep `Processing` and add the new values | Rejected: the screen could still rebuild per-stage numbers, so the API would not guarantee the replacement requirement 1 asks for. |

## R-10: The SPA builds screen text from kinds the server returns

**Decision**: The API returns the issue impact and reason, the current activity,
and the scan failure reason as kinds (enum values). The SPA turns them into user
wording.

**Rationale**: The English i18n feature ([#463](https://github.com/syudead/vv/pull/463),
`specs/023-english-i18n`) takes this approach
([docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)):

- Screen strings live in the SPA catalog `web/src/i18n/en.ts`.
- The API `message` is not shown on screen.
- Failure reasons are returned as codes, and the screen turns them into words.

Returning kinds keeps that approach and lets the SPA alone decide the wording. The
catalog can hold a `Record` keyed by kind, so a missing kind is caught by the type
check. The scan failure reason keeps the `errorCode` and `errorPath` that 023
added to `Scan`.

**Alternatives considered**: The server returns English sentences. Rejected: it
conflicts with 023's rule that `message` is not shown, and adding a language would
change the API.

## R-11: Migration moves only results derivable from existing rows into the latest scan

**Decision**: The migration puts two things into the latest scan
([data-model.md, `scans.settled_at` and `scans.issues_revision` (added columns)](data-model.md#scanssettled_at-and-scansissues_revision-added-columns)):

| Videos | Destination |
| --- | --- |
| Videos with unfinished jobs | `scan_videos` |
| Videos with any preparation currently `failed` | `*_failed` issues |

The set of videos the scan registered and the file failures during the scan are
not in the pre-migration rows, so they are not restored. The latest import right
after migration therefore shows:

- `partial` when a failure remains
- `running` when unfinished jobs remain

The counts cover only the moved videos and issues. The next import replaces them
(R-3).

**Rationale**: Showing the latest pre-migration scan as "0 videos, no issues"
would wrongly report a failed import as complete. Preparation failures remain as
state on the video rows, so the impact on the user (requirement 5's "what state is
that video in") is shown correctly. The registered count cannot serve as the
denominator, because today's `scans.total` is the number of changed files, not a
set of videos.

**Alternatives considered**: Add an "unknown result" status only for the
pre-migration scan. Rejected: a one-off migration would bring a sixth status, with
its wording and display, into the screen.
