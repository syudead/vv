# Data model: Seek thumbnail stage

Parent Issue #388. The existing index (`videos`, `jobs`) and the storage of
generated files stay as in [ARCHITECTURE.md](../../ARCHITECTURE.md) and
[internal/artifacts/store.go](../../internal/artifacts/store.go). This file
covers only the added column and job kind, their state transitions, the claim
condition, and the migration.

## `videos.seek_thumbnail_state`

| Column | Type | Values | Meaning |
| --- | --- | --- | --- |
| `seek_thumbnail_state` | `text not null default 'pending'` | `pending` / `done` / `failed` | State of seek thumbnail generation. Same shape as `thumbnail_state` and `preview_state` |

`domain.Video` and `domain.IndexedVideo` gain `SeekThumbnailState` (the
existing type `domain.SeekThumbnailState`, whose values become the stored
values), and `domain.UpsertResult` gains `NeedsSeekThumbnail`
(`seek_thumbnail_state <> 'done'`). `thumbnail_state` now means the cover JPEG
only.

The column is a rebuildable index (ARCHITECTURE.md, "Rebuildable and user
data").

## `jobs.kind = 'seek_thumbnail'`

`domain.JobSeekThumbnail = "seek_thumbnail"` is added to the `jobs.kind` CHECK
(SQLite cannot alter a CHECK in place, so the table is rebuilt as in `00006`).
The partial unique index `jobs_pending_kind_video_idx` (one unfinished row per
`(kind, video_id)`) keeps working as is.

The order of `domain.JobKinds` is `probe, thumbnail, seek_thumbnail, preview`.
There is one worker per stage, and `seek_thumbnail` also has one worker that
runs jobs serially.

`domain.Processing` gains `SeekThumbnail`, included in `Remaining()`.
`IngestStore.Processing` only adds one case to the existing `group by kind`.

## State transitions

| Trigger | Operation | `seek_thumbnail_state` | Jobs queued |
| --- | --- | --- | --- |
| Ingest of new content (`UpsertVideo`) | Insert the row | `pending` (default) | `probe`, `thumbnail`, `seek_thumbnail` (`NeedsSeekThumbnail`) |
| Rescan of an existing video (`ensurePendingJobs`) | If `pending` | Unchanged | `seek_thumbnail` (`EnsureJob`; `failed` is not requeued) |
| `seek_thumbnail` job succeeds | `SetSeekThumbnailStateForJob(done)` | `done` (only when the content key, location and location generation at claim time are still the same) | None |
| `seek_thumbnail` job fails terminally | `recordTerminalFailure` | `failed` (only when `<> 'done'` and the identity is the same) | None |
| `thumbnail` job fails terminally | `recordTerminalFailure` | Unchanged | None |
| Probe retry (`RetryProbe`) | If `failed` | `pending` | `probe`; `thumbnail` (when `thumbnail_state` was reset); `seek_thumbnail` (when reset) |
| `done` but the stored files are missing (`Catalog.SeekThumbnailState`) | `RequeueMissingSeekThumbnails(id, contentKey)` | `pending` (only when `done` and the content key matches, in one transaction) | `seek_thumbnail` |
| The video row is deleted | Cascade | The row is gone | `ContentUnreferenced` removes the generated files (existing) |

`Catalog.SeekThumbnailState` derives the state: if the stored files exist, it
is `done`. If not and the column is `done`, it asks
`RequeueMissingSeekThumbnails` and returns `pending` (the response is `pending`
even if nothing could be queued, the same as `RequeueMissingPreview` for the
hover preview); otherwise it is the column's value. `ThumbnailJobActive` is no
longer used and is removed.

`RetryProbe` loses its argument for whether the stored files exist. A video
that is `done` with missing stored files is requeued by the row above when
`GET /api/videos/{id}` is called from the playback screen.

`Ingest.SeekThumbnails` (a new handler) proceeds in the same order as
`Ingest.Thumbnail`:

1. `GetVideo`
2. `JobIdentityCurrent`
3. `CheckSource`
4. Inside the per-content lock: `PublishSeekThumbnails` (writes nothing when
   the stored files exist), then `SetSeekThumbnailStateForJob(done)`; if that
   is not applied (the video disappeared during generation),
   `removeIfUnreferencedLocked`.
5. Finally, `removeIfUnreferencedLocked`.

`Ingest.Thumbnail` no longer calls `PublishSeekThumbnails`.

## Claim condition

`domain.ClaimConditionFor` becomes the following. `claimConditionSQL` in
`internal/store` translates each condition into SQL and is written to reach the
same decision as `Allows`.

| Kind | Registered location | Probe finished | No claimable `thumbnail` job |
| --- | --- | --- | --- |
| `probe` | Required | — | — |
| `thumbnail` | Required | Required | — |
| `seek_thumbnail` | Required | Required | Required |
| `preview` | Required | — | — |

"No claimable `thumbnail` job" means there is no `thumbnail` row with
`state in ('queued', 'running')` and a registered location (the same range that
`IngestStore.Processing` counts). It also counts `thumbnail` jobs that cannot be
claimed yet because they wait for the probe, so right after a scan
`seek_thumbnail` does not start until every probe and cover thumbnail has
finished. This satisfies requirement 1 (cover first) and requirement 3 (during
a scan, the full-video decode for seek thumbnails does not compete with the
flow of cover JPEGs).

The condition is evaluated only at claim time and does not stop a running
`seek_thumbnail`. If a new `thumbnail` is queued after the claim (a scan or a
folder change), that cover JPEG runs alongside the one running seek thumbnail
job (requirement 1 does not allow it to wait). They overlap only until that one
job finishes; the next `seek_thumbnail` is not claimed until no `thumbnail`
remains. No more than two ffmpeg processes that read the whole video (seek
thumbnail and hover preview) run at once; what overlaps is the cover JPEG,
which takes one frame with an input-side seek (0.1–0.3 seconds each), the same
way the probe's `ffprobe` overlaps today.

Wake-up (`cmd/mdm/events.go`): the `seek_thumbnail` worker wakes when
`JobsQueued` includes its kind (the existing general rule), and also when the
`Stage` of `VideoIngestChanged` is `probe`, `thumbnail`, or empty (the video
row was removed). A media folder change publishes `JobsQueued` for all kinds as
before.

## Migration

`internal/store/migrations/00014_seek_thumbnail_stage.sql`:

- Up: add `seek_thumbnail_state` to `videos` (default `pending`, with CHECK).
  Rebuild `jobs` with `seek_thumbnail` in the `kind` CHECK (same procedure as
  `00006`, keeping existing rows and indexes). Queue `seek_thumbnail` as
  `queued` for every video with `probe_state <> 'pending'` and at least one
  location (the same shape as the preview requeue in `00006`). `thumbnail` rows
  are not deleted.
- Down: delete the `seek_thumbnail` rows, rebuild `jobs` with the original
  CHECK, and drop the column.

Every existing video becomes `pending` regardless of `thumbnail_state`. For a
video whose files are already complete, the `seek_thumbnail` job sees the
stored files in `PublishSeekThumbnails` and ends without writing, so nothing is
rebuilt (requirement 5). A video whose seek thumbnails failed after its cover
JPEG (still `thumbnail_state = done`) is retried independently here for the
first time (parent Issue edge cases).

A `thumbnail` job that is `queued` / `running` at migration time creates only
the cover JPEG in the new version. That video's seek thumbnails are created by
the `seek_thumbnail` job the migration queued.
