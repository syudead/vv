# Contract: Processing status and seek thumbnail state

Parent Issue #388. Source of truth:
[api/openapi.yaml](../../../api/openapi.yaml). This document describes only
what changes.

## 1. `Processing.seekThumbnail`

`seekThumbnail` (integer, required) is added to `Processing` in the
`GET /api/processing` response and in the `processing` event of `/api/events`.

| Field | Meaning |
| --- | --- |
| `probe` | Remaining probe work (unchanged) |
| `thumbnail` | Remaining cover thumbnail work (description changes from "remaining thumbnail and seek preview work") |
| `seekThumbnail` | Remaining seek thumbnail work: `seek_thumbnail` jobs in `queued` / `running` for videos with a registered location |
| `preview` | Remaining list preview work (unchanged) |

When all are 0 (including `seekThumbnail`), preparation is finished. The web
`processingRemaining` and the "preparing" check use this sum. The processing
status breakdown becomes four columns: probe, thumbnail, seek, preview.

## 2. Meaning of `seekThumbnailState`

The meaning of the values of `Video.seekThumbnailState` (present only in
`GET /api/videos/{id}`, under the same condition as `seekThumbnailUrl`) is
revised to match [data-model.md §3](../data-model.md#3-state-transitions). The
schema (enum `pending` / `done` / `failed`) does not change.

| Value | Meaning |
| --- | --- |
| `done` | The stored files exist |
| `pending` | Waiting for generation, or generating. When the saved state is `done` but the stored files are missing, the server queues a rebuild and returns `pending` |
| `failed` | Generation failed and reached the retry limit |

Description of `reprobeVideo` (`POST /api/videos/{id}/probe`): it resets the
probe state to `pending` and queues a probe job; it resets whichever of cover
thumbnail, seek thumbnail and list preview are `failed` to `pending`, and
queues the cover thumbnail and seek thumbnail jobs only for those it reset (the
list preview is queued after the probe succeeds). The sentence "also queue the
thumbnail job when the seek preview's stored files are missing" is removed.
