# Contract: Processing status and seek thumbnail state

Parent Issue #388. [api/openapi.yaml](../../../api/openapi.yaml) is the source of truth; this
document lists only the changes.

## 1. `Processing.seekThumbnail`

Add `seekThumbnail` (integer, required) to `Processing` in the `GET /api/processing` response and in
the `processing` event of `/api/events`.

| Field | Meaning |
| --- | --- |
| `probe` | Remaining probes (unchanged) |
| `thumbnail` | Remaining representative thumbnails (the description changes from "remaining thumbnails and seek previews") |
| `seekThumbnail` | Remaining seek thumbnails: `seek_thumbnail` jobs in `queued` / `running` for videos with a registered location |
| `preview` | Remaining list previews (unchanged) |

Preparation is finished when all of them are 0 (including `seekThumbnail`). The web
`processingRemaining` and the "Preparing" check use this sum. The processing status breakdown
becomes four columns: probe, thumbnail, seek, preview.

## 2. Meaning of `seekThumbnailState`

The meaning of the values of `Video.seekThumbnailState` (present only in `GET /api/videos/{id}`,
under the same condition as `seekThumbnailUrl`) changes to match
[data-model.md §3](../data-model.md#3-state-transitions). The schema (enum `pending` / `done` / `failed`)
does not change.

| Value | Meaning |
| --- | --- |
| `done` | The artifact exists |
| `pending` | Waiting for generation, or generating. When the stored state is `done` but the artifact is missing, the server queues a regeneration and returns `pending` |
| `failed` | Generation failed and reached the retry limit |

Description of `reprobeVideo` (`POST /api/videos/{id}/probe`):

1. Reset the probe state to `pending` and queue a probe job.
2. Reset each of the representative thumbnail, the seek thumbnail and the list preview that is
   `failed` to `pending`.
3. Queue the representative thumbnail and seek thumbnail jobs only when they were reset. (The list
   preview job is queued after a successful probe.)

The statement "also queues a thumbnail job when the seek preview artifact is missing" is removed.
