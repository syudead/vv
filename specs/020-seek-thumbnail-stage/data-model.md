# Data model: Seek thumbnail stage

Parent Issue #388. The existing index (`videos`, `jobs`) and the artifact store stay as described in
[ARCHITECTURE.md](../../ARCHITECTURE.md) and
[internal/artifacts/store.go](../../internal/artifacts/store.go). This document covers only the
added column and job kind, their state transitions, the claim condition and the migration.

## 1. `videos.seek_thumbnail_state`

| Column | Type | Values | Meaning |
| --- | --- | --- | --- |
| `seek_thumbnail_state` | `text not null default 'pending'` | `pending` / `done` / `failed` | Generation state of the seek thumbnails. Same shape as `thumbnail_state` and `preview_state` |

Add `SeekThumbnailState` to `domain.Video` and `domain.IndexedVideo` (the existing type
`domain.SeekThumbnailState`; its values become the stored values). Add `NeedsSeekThumbnail`
(`seek_thumbnail_state <> 'done'`) to `domain.UpsertResult`. `thumbnail_state` then covers only the
representative JPEG.

This is a rebuildable index (ARCHITECTURE.md "Rebuildable and user data").

## 2. `jobs.kind = 'seek_thumbnail'`

`domain.JobSeekThumbnail = "seek_thumbnail"`. Add it to the `jobs.kind` CHECK. SQLite cannot alter a
CHECK in place, so the table is rebuilt as in `00006`. The partial unique index
`jobs_pending_kind_video_idx` (one unfinished row per `(kind, video_id)`) keeps working unchanged.

`domain.JobKinds` is ordered `probe, thumbnail, seek_thumbnail, preview`. There is one worker per
stage, so `seek_thumbnail` also runs one job at a time, serially.

Add `SeekThumbnail` to `domain.Processing` and include it in `Remaining()`. `IngestStore.Processing`
only needs one more case in the existing `group by kind`.

## 3. State transitions

| Origin | Operation | `seek_thumbnail_state` | Jobs queued |
| --- | --- | --- | --- |
| Importing new content (`UpsertVideo`) | Row insert | `pending` (default) | `probe`, `thumbnail`, `seek_thumbnail` (`NeedsSeekThumbnail`) |
| Rescanning an existing video (`ensurePendingJobs`) | If `pending` | Unchanged | `seek_thumbnail` (`EnsureJob`; `failed` is not requeued) |
| `seek_thumbnail` job succeeds | `SetSeekThumbnailStateForJob(done)` | `done` (only when the content key, location and location generation at claim time are still the same) | None |
| `seek_thumbnail` job fails terminally | `recordTerminalFailure` | `failed` (only when `<> 'done'` and the identity is the same) | None |
| `thumbnail` job fails terminally | `recordTerminalFailure` | Unchanged | None |
| Probe retry (`RetryProbe`) | If `failed` | `pending` | `probe`; `thumbnail` (when `thumbnail_state` was reset); `seek_thumbnail` (when reset) |
| `done` but the artifact is missing (`Catalog.SeekThumbnailState`) | `RequeueMissingSeekThumbnails(id, contentKey)` | `pending` (only when `done` and the content key is the same, in one transaction) | `seek_thumbnail` |
| Video row deleted | Cascade | The row is gone | `ContentUnreferenced` deletes the artifacts (existing) |

`Catalog.SeekThumbnailState` derives the state as follows:

- The artifact exists: `done`.
- The artifact is missing and the column is `done`: request `RequeueMissingSeekThumbnails` and
  return `pending` (the response is `pending` even if queuing fails, as with `RequeueMissingPreview`
  for the hover preview).
- Otherwise: the column value.

`ThumbnailJobActive` is no longer used and is removed.

`RetryProbe` no longer takes an argument for whether the artifact exists. For a `done` video with a
missing artifact, `GET /api/videos/{id}` from opening the video page requeues it through the row
above.

`Ingest.SeekThumbnails` (a new handler) proceeds in the same order as `Ingest.Thumbnail`:

1. `GetVideo`.
2. `JobIdentityCurrent`.
3. `CheckSource`.
4. Inside the per-content lock: `PublishSeekThumbnails` (writes nothing when the artifact exists) →
   `SetSeekThumbnailStateForJob(done)`. If that does not apply (the video was deleted during
   generation), `removeIfUnreferencedLocked`.
5. Finally, `removeIfUnreferencedLocked`.

`Ingest.Thumbnail` no longer calls `PublishSeekThumbnails`.

## 4. Claim condition

`domain.ClaimConditionFor` becomes the table below. `claimConditionSQL` in `internal/store` maps
each condition to SQL and is written to decide the same way as `Allows`.

| Kind | Registered location | Probe finished | No claimable `thumbnail` job |
| --- | --- | --- | --- |
| `probe` | Required | — | — |
| `thumbnail` | Required | Required | — |
| `seek_thumbnail` | Required | Required | Required |
| `preview` | Required | — | — |

"No claimable `thumbnail` job" means no `thumbnail` row with `state in ('queued', 'running')` and a
registered location exists (the same range that `IngestStore.Processing` counts). It also counts
`thumbnail` jobs that cannot be claimed yet because they wait for a probe. So right after a scan,
`seek_thumbnail` does not start until every probe → representative thumbnail finishes. This
satisfies Requirement 1 (the representative comes first) and Requirement 3 (during a scan, the
representative JPEG flow does not compete with the full decode for seek thumbnails).

The condition is checked only at claim time and does not stop a running `seek_thumbnail`:

- If a new `thumbnail` is queued after the claim (by a scan or a folder change), its representative
  JPEG runs alongside the one running seek job (Requirement 1 does not allow it to wait).
- They overlap only until that one job finishes. The next `seek_thumbnail` is not claimed until no
  `thumbnail` remains.
- No more than 2 full-read ffmpeg processes (seek thumbnail, hover preview) run at once. The
  overlap is a representative JPEG that takes one frame with input-side seeking (0.1 to 0.3 s
  each), the same way the probe's `ffprobe` already overlaps today.

Wake-up (`cmd/mdm/events.go`): the `seek_thumbnail` worker wakes when `JobsQueued` includes its kind
(the existing general rule). It also wakes when the `Stage` of `VideoIngestChanged` is `probe`,
`thumbnail` or empty (a video row was deleted). A media folder change still emits `JobsQueued` for
every kind.

## 5. Migration

`internal/store/migrations/00014_seek_thumbnail_stage.sql`:

- Up:
  1. Add `seek_thumbnail_state` to `videos` (default `pending`, with a CHECK).
  2. Rebuild `jobs` with `seek_thumbnail` in the `kind` CHECK (the same steps as `00006`, keeping
     the existing rows and indexes).
  3. Queue a `seek_thumbnail` job as `queued` for each video with `probe_state <> 'pending'` and at
     least one location (the same shape as the preview requeue in `00006`).
  4. Keep the `thumbnail` rows.
- Down: delete the `seek_thumbnail` rows, rebuild `jobs` with the original CHECK, and drop the
  column.

Every existing video becomes `pending` regardless of `thumbnail_state`. For a finished video,
`PublishSeekThumbnails` sees the artifact and ends the `seek_thumbnail` job without writing, so no
regeneration happens (Requirement 5). A video whose seek thumbnails alone failed after the
representative JPEG (still `thumbnail_state = done`) now gets its first independent retry (parent
Issue Edge cases).

A `thumbnail` job in `queued` / `running` at migration time builds only the representative JPEG in
the new version. That video's seek thumbnails come from the `seek_thumbnail` job the migration
queued.
