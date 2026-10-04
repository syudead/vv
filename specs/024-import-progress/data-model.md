# Data model: Import progress and results

The rest of the model is unchanged: the existing SQLite tables are defined in
[internal/store/migrations](../../internal/store/migrations). `scans` is in
[00002_core.sql](../../internal/store/migrations/00002_core.sql), and the state
columns of `jobs` and `videos` are in the migrations in the same place. This
feature adds only the two tables below and two columns on `scans`.

All three belong to the rebuildable side of ARCHITECTURE.md "Rebuildable and user
data": rerunning the scan and preparation produces the same content.

## `scans.settled_at` and `scans.issues_revision` (added columns)

| Column | Type | Meaning |
| --- | --- | --- |
| `settled_at` | `integer null` | The time every target video of this scan settled (Unix seconds) |
| `issues_revision` | `integer not null default 0` | Incremented by one on every change to this scan's `scan_issues` ([`scan_issues` (new table)](#scan_issues-new-table)) |

Rules ([research.md R-4](research.md#r-4-done-only-when-the-scan-is-closed-and-the-set-has-no-remaining-jobs)):

- Only `refreshScanSettled` in `internal/store` writes it. Which transactions call
  it is in research.md R-4.

  | Condition | Effect |
  | --- | --- |
  | The latest scan has `state <> 'running'` and the videos in `scan_videos` have no claimable `queued` or `running` job | Set the current time if empty; leave an existing value unchanged |
  | Otherwise | Set to `null` |

- A scan that closed as `failed` also gets the value once the remaining
  preparation settles. The screen shows `failed` first.
- The migration runs in this order:
  1. Put into the latest scan's `scan_videos` the videos that still have a
     claimable `queued` or `running` job (the same condition as the carry-over in
     [`scan_videos` (new table)](#scan_videos-new-table)).
  2. In the migration that adds `scan_issues`, put into the latest scan's
     `scan_issues` the videos whose `probe_state`, `thumbnail_state`,
     `seek_thumbnail_state` or `preview_state` is currently `failed`, with the
     matching `*_failed` kind ([`scan_issues` (new table)](#scan_issues-new-table),
     [research.md R-11](research.md#r-11-migration-moves-only-results-derivable-from-existing-rows-into-the-latest-scan)).
  3. Set `settled_at` of closed scans to `finished_at`. The latest scan stays
     `null` if step 1 added any video.

## `scan_videos` (new table)

The set of target videos of the latest scan
([R-1](research.md#r-1-import-videos-stored-as-a-set-tied-to-the-scan-record),
[R-3](research.md#r-3-the-set-holds-only-the-latest-scan-and-is-replaced-when-a-new-scan-starts)).

| Column | Type | Meaning |
| --- | --- | --- |
| `scan_id` | `integer not null references scans(id) on delete cascade` | The scan |
| `video_id` | `integer not null references videos(id) on delete cascade` | The target video |

The primary key is `(scan_id, video_id)`.

**When a video joins**: in a transaction that inserts a `queued` row into `jobs`,
or that makes a `queued` job claimable. Inside that transaction, the video is
added to the latest scan (`max(scans.id)`; nothing is added when there is none)
with `insert ... on conflict do nothing`. The current sites below go through one
helper in `internal/store`. Any future migration that enqueues jobs follows the
same rule.

- Scan registration: the enqueue that follows the `UpsertVideo` result, and
  `EnsureJob` for unchanged files
- `EnqueueJob` (including the preview enqueued in the same transaction as the
  probe result; R-2)
- Re-enqueuing missing previews and seek thumbnails (`RequeueMissingPreview`,
  `requeueJob`)
- Retrying analysis (`RetryProbe`)
- Adding or relinking a media folder: videos with `queued` jobs that became
  claimable because a registered location now exists
- `StartScan`: videos carried over from the previous scan that still have a
  `queued` or `running` job

**Settled rule**: a video in the set with no `jobs` row in
`state in ('queued','running')`. A job with no registered location (not
claimable) does not count as remaining, as with today's `Processing`.

**When a video leaves**: when its video row is deleted, through
`on delete cascade` (the source file was deleted or its content changed). On a
move, the video row is kept because the content identifies it as the same video,
and it keeps counting at its new location. `StartScan` deletes the previous scan's
rows.

## `scan_issues` (new table)

Events of the latest scan that the user is told about
([R-6](research.md#r-6-issues-stored-as-one-row-per-event-and-grouped-per-video-on-read)).

| Column | Type | Meaning |
| --- | --- | --- |
| `id` | `integer primary key` | |
| `scan_id` | `integer not null references scans(id) on delete cascade` | The scan |
| `video_id` | `integer null references videos(id) on delete cascade` | The registered video; `null` for an unregistered file |
| `path` | `text not null` | The file's absolute path at the time of the event. Identifies unregistered files and is the displayed location |
| `kind` | `text not null` | One of the kinds below |
| `created_at` | `integer not null` | |

Uniqueness is enforced by two partial indexes: `(scan_id, video_id, kind)` when
`video_id` is not null, and `(scan_id, path, kind)` when `video_id` is null.

**Kinds** (an enum in `internal/domain`, which also decides the severity):

| `kind` | Severity | Recorded when |
| --- | --- | --- |
| `unreadable` | Failure | The scan could not read the file's metadata or content |
| `changed_during_import` | Failure | The file changed during registration |
| `register_failed` | Failure | Writing to the index or enqueuing jobs failed |
| `probe_failed` | Failure | The analysis job failed up to the retry limit |
| `thumbnail_failed` | Failure | The representative thumbnail job failed up to the limit |
| `seek_thumbnail_failed` | Failure | The seek thumbnail job failed up to the limit |
| `preview_failed` | Failure | The list preview job failed up to the limit |
| `thumbnail_first_frame` | Substitution | The representative thumbnail was made from the first frame |
| `seek_thumbnail_full_decode` | Substitution | The seek thumbnails were rebuilt from the whole video |

Rules:

- The three scan kinds are recorded from the branches of `scanner.Scan` that today
  only log: metadata cannot be read, the job for an unchanged file cannot be
  confirmed, `ingest` failed. When the scan knows an existing video for the file,
  `video_id` is filled.
- `*_failed` rows are inserted in the same transaction as `recordTerminalFailure`.
  When the same stage later succeeds, the transaction that writes the result
  deletes that video's `*_failed` row for that stage.
- Substitution rows are inserted in the transaction that writes that stage's
  success. When the same stage is rebuilt without substitution, the row is
  deleted.
- `StartScan` deletes the previous scan's rows (R-3).
- A transaction that inserts or deletes rows increments the latest scan's
  `issues_revision` by one within the same transaction. The screen can then reload
  on a change that leaves the grouped item count unchanged, for example a
  thumbnail failure added to a video that already has an analysis failure, or one
  item replaced by another.

**Grouped item** (the read shape; not stored): per `coalesce(video_id, path)`,
return the set of kinds, the severity (failure when any kind is a failure), and
the displayed location. The location is the video's current representative
location, or `path` for an unregistered file. Both are converted to the
registered folder's display name and a relative path by the same rule as
`domain.LocateVideoFolder`. Counts use the same unit (grouped items). An item
whose location is not inside any registered folder is left out of both the list
and the counts
([contracts/scan-api.md, `GET /api/scans/current/issues`](contracts/scan-api.md#get-apiscanscurrentissues)).
