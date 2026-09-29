# Data Model: import progress and results

[internal/store/migrations](../../internal/store/migrations) is canonical for the existing SQLite
tables. `scans` is in [00002_core.sql](../../internal/store/migrations/00002_core.sql); the state
columns of `jobs` and `videos` are in the other migrations in the same directory. This feature adds
only the two tables below and two columns on `scans`. No other table changes.

All three additions belong to the rebuildable side of ARCHITECTURE.md "Rebuildable and user data".
Rerunning the scan and the preparation produces the same data.

## 1. `scans.settled_at` and `scans.issues_revision` (new columns)

| Column | Type | Meaning |
| --- | --- | --- |
| `settled_at` | `integer null` | Time when every target video of this scan settled (Unix seconds) |
| `issues_revision` | `integer not null default 0` | Number incremented by 1 whenever this scan's `scan_issues` change (§3) |

Rules ([research.md R-4](research.md#r-4-done-means-the-scan-closed-and-no-job-remains-in-the-set)):

- Only `refreshScanSettled` in `internal/store` writes it. research.md R-4 lists the transactions
  that call it.
  - When the latest scan has `state <> 'running'` and no video in `scan_videos` has a startable
    `queued` or `running` job: set the current time if the value is empty. Leave an existing value
    unchanged.
  - Otherwise: set it to `null`.
- A scan that closed as `failed` also gets the value once the remaining preparation settles. The
  screen shows `failed` first.
- The migration runs in this order:
  1. Insert into the latest scan's `scan_videos` every video that still has a startable `queued`
     or `running` job (the same condition as the carry-over in §2).
  2. In the migration that adds `scan_issues`, insert into the latest scan's `scan_issues` every
     video whose `probe_state`, `thumbnail_state`, `seek_thumbnail_state` or `preview_state` is
     currently `failed`, with the matching `*_failed` kind (§3,
     [research.md R-11](research.md#r-11-the-migration-moves-into-the-latest-scan-only-the-results-the-current-rows-reveal)).
  3. Copy `finished_at` into `settled_at` for closed scans. The latest scan stays `null` if step 1
     inserted any video.

## 2. `scan_videos` (new table)

The set of target videos of the latest scan
([R-1](research.md#r-1-store-the-import-targets-as-a-set-of-videos-tied-to-the-scan-record),
[R-3](research.md#r-3-keep-the-set-for-the-latest-scan-only-and-replace-it-when-a-new-scan-starts)).

| Column | Type | Meaning |
| --- | --- | --- |
| `scan_id` | `integer not null references scans(id) on delete cascade` | Scan |
| `video_id` | `integer not null references videos(id) on delete cascade` | Target video |

The primary key is `(scan_id, video_id)`.

**When a video joins**: in the transaction that inserts a `queued` row into `jobs`, or in the
transaction that makes a `queued` job startable. Inside that transaction, the store runs
`insert ... on conflict do nothing` into the latest scan (`max(scans.id)`; nothing is added when no
scan exists). The current call sites below go through one helper in `internal/store`. Future
migrations that enqueue jobs follow the same rule.

- Scan registration: enqueueing based on the result of `UpsertVideo`, and `EnsureJob` for
  unchanged files
- `EnqueueJob` (including the preview enqueued in the same transaction as the probe result; R-2)
- Requeueing missing previews and seek thumbnails (`RequeueMissingPreview`, `requeueJob`)
- Probe retry (`RetryProbe`)
- Adding or remapping a media folder: videos whose `queued` jobs became startable because a
  registered location now exists
- `StartScan`: videos carried over from the previous scan that still have `queued` or `running`
  jobs

**Settled**: a video in the set that has no `jobs` row with `state in ('queued','running')`. A job
without a registered location (not startable) does not count as remaining, as in the current
`Processing`.

**When a video leaves**: deleting the video row removes it through `on delete cascade` (the source
file was deleted or its content changed). On a move, the video row stays because the content
identifies it as the same video, so it keeps counting under the new location. `StartScan` deletes
the previous scan's rows.

## 3. `scan_issues` (new table)

Events in the latest scan that the user is told about
([R-6](research.md#r-6-store-issues-as-one-row-per-event-and-merge-them-per-video-on-read)).

| Column | Type | Meaning |
| --- | --- | --- |
| `id` | `integer primary key` | |
| `scan_id` | `integer not null references scans(id) on delete cascade` | Scan |
| `video_id` | `integer null references videos(id) on delete cascade` | Registered video. `null` for an unregistered file |
| `path` | `text not null` | Absolute path of the file at the time of the event. Identifies unregistered files and serves as the displayed location |
| `kind` | `text not null` | One of the kinds below |
| `created_at` | `integer not null` | |

Uniqueness uses two partial indexes: `(scan_id, video_id, kind)` when `video_id` is non-null, and
`(scan_id, path, kind)` when `video_id` is null.

**Kinds** (an enum in `internal/domain`; the domain also decides the severity):

| `kind` | Severity | Recorded when |
| --- | --- | --- |
| `unreadable` | failed | The scan could not read the file's metadata or content |
| `changed_during_import` | failed | The file changed during registration |
| `register_failed` | failed | Writing to the index or enqueueing a job failed |
| `probe_failed` | failed | The probe job failed up to the retry limit |
| `thumbnail_failed` | failed | The main thumbnail job failed up to the limit |
| `seek_thumbnail_failed` | failed | The seek thumbnail job failed up to the limit |
| `preview_failed` | failed | The list preview job failed up to the limit |
| `thumbnail_first_frame` | substituted | The main thumbnail was made from the first frame |
| `seek_thumbnail_full_decode` | substituted | The seek thumbnails were rebuilt from the whole video |

Rules:

- The three scan kinds are recorded from the `scanner.Scan` branches that today only log: metadata
  could not be read, the job for an unchanged file could not be verified, and `ingest` failed.
  When the scan knows an existing video, `video_id` is set.
- `*_failed` rows are inserted in the same transaction as `recordTerminalFailure`. When the same
  stage later succeeds, the transaction that writes the result deletes that video's `*_failed` row
  for that stage.
- Substitution rows are inserted in the transaction that writes the stage's success. When the
  same stage is rebuilt without substitution, that row is deleted.
- `StartScan` deletes the previous scan's rows (R-3).
- Every transaction that inserts or deletes rows also increments the latest scan's
  `issues_revision` by 1. The screen can then reload on this number even when the merged entry
  count does not change. Examples: a video with a probe failure also gets a thumbnail failure; one
  entry is replaced by another.

**Merged entry** (the read shape; not stored): one entry per `coalesce(video_id, path)` with the
set of kinds, the severity (failed if any kind is failed) and the displayed location. The location
is the current primary location for a video, and `path` for an unregistered file. Both are
converted to the registered folder's display name and a relative path with the same rule as
`domain.LocateVideoFolder`. Counts use the same unit (merged entries). An entry whose location is
outside every registered folder appears neither in the list nor in the counts
([contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)).
