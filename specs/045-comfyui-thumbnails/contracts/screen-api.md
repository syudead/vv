# Contract: ComfyUI settings, thumbnail edits and the applied image in the screen API

Source of truth: `api/openapi.yaml`. This document covers the routes and the
`Video` field this feature adds and the one existing route whose behaviour
changes. Every route is owner only: none is listed in `accessRoutes`, so the
boundary answers `401` to a guest (requirement 15), and
`TestAccessClassificationMatchesOpenAPISecurity` checks the `security` of each.
Error bodies follow [023 error-api](../../023-english-i18n/contracts/error-api.md);
this feature adds the code `provider_failed` and the
reasons named in the tables below.

## `Video` change

| Field | Type | Rule |
| --- | --- | --- |
| `thumbnailGenerated` | boolean | `true` while an applied image is the thumbnail. Only in owner responses |
| `thumbnailUrl` | string (existing) | The version becomes `<content key prefix>-g<revision>` while an image is applied ([data-model.md, Values added to `domain`](../data-model.md#values-added-to-domain)); guests get the same string |

## `GET /api/settings/comfyui`, `PUT /api/settings/comfyui`

`getComfyUISettings`, `updateComfyUISettings`. The connection
([R-9](../research.md#r-9-the-api-key-is-write-only)). `GET` sends
`Cache-Control: no-store`.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `provider` | Both | `self_hosted`, `cloud` | Yes | Which provider |
| `url` | Both | string | For `self_hosted` | `http` or `https` base URL |
| `apiKey` | `PUT` only | string | No | Absent keeps the saved key; `""` clears it |
| `apiKeySet` | `GET` only | boolean | Yes | A key is saved |

`GET` before any save answers `200` with `provider` absent.

| Status | `code` / reason | When |
| --- | --- | --- |
| `400` | `invalid_request` / `comfyui_url_invalid` | `self_hosted` without a valid URL |
| `400` | `invalid_request` / `comfyui_api_key_missing` | `cloud` with no key saved or sent |

## `POST /api/settings/comfyui/check`

`checkComfyUI`. Tests the saved connection with the requests in
[R-5](../research.md#r-5-provider-differences-stay-inside-the-adapter) and
answers `200` `CheckResult` (the schema the Ollama check uses), extended with
`missing`. The `message` of an unavailable result names the cause: not
reachable, not ComfyUI, the key is rejected, nothing is saved, or files are
missing (acceptance criteria 1 and 2). When the provider answers but lacks
what the bundled workflows need
([research.md R-13](../research.md#r-13-the-connection-test-checks-the-workflows-nodes-and-model-files)),
`missing` lists each item.

| Field of a `missing` item | Type | Meaning |
| --- | --- | --- |
| `kind` | `node`, `model` | A node type or a model file |
| `name` | string | The node's `class_type` or the file name |
| `folder` | string | For a model: its folder under ComfyUI's `models` |
| `source` | string | For a model: its download page ([workflow.md, Required files](workflow.md#required-files)) |

## `GET /api/thumbnail-edits/readiness`

`getThumbnailEditReadiness`. What the video page needs to enable its entry
(requirement 14): `{ connection: boolean }`, where `connection` means
settings are saved, not that the check passed.

## `POST /api/videos/{id}/thumbnail-edits`

`startThumbnailEdit`. Body `{ positionMs }`. Extracts the frame and answers
`201` `ThumbnailEdit` with state `ready`; an existing session of the video
ends first ([R-11](../research.md#r-11-a-new-session-replaces-the-previous-one-a-run-is-limited-to-10-minutes)).

| Status | `code` / reason | When |
| --- | --- | --- |
| `400` | `invalid_request` / `thumbnail_position_out_of_range` | As the position override |
| `404` | `not_found` / `video_not_found`, `file_unavailable` | As the position override |
| `409` | `conflict` / `thumbnail_edit_not_ready` | No connection saved |
| `409` | `conflict` / `thumbnail_frame_unavailable` | `ffmpeg` produced no frame |

## `ThumbnailEdit`

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | string | The session |
| `videoId` | integer | The video |
| `state` | [`ThumbnailEditState`](../data-model.md#edit-session) | The state |
| `frameUrl` | string | `GET /api/thumbnail-edits/{editId}/frame`, the PNG |
| `frameWidth`, `frameHeight` | integer | The frame's pixel size, which the mask must match |
| `elapsedMs` | integer | Since the run was submitted; absent in `ready` |
| `progress` | `{ value, max }` | Step progress; absent when the provider gave none |
| `error` | `{ reason, message }` | In `failed`: a reason of [workflow.md, Failure reasons](workflow.md#failure-reasons) and the provider's text when it gave one |
| `resultUrl` | string | `GET /api/thumbnail-edits/{editId}/result`, in `succeeded` |

## `GET /api/thumbnail-edits/{editId}`

`getThumbnailEdit`. Answers `200` `ThumbnailEdit` and counts as the page's
heartbeat ([R-4](../research.md#r-4-an-edit-is-an-in-memory-session-per-video-read-by-polling)).
An ended session answers `404 thumbnail_edit_not_found`, as do the frame and
result routes.

## `POST /api/thumbnail-edits/{editId}/runs`

`runThumbnailEdit`. `multipart/form-data` with `prompt` (text, up to 4000
characters), `mask` (PNG) and `reference` (PNG, JPEG or WebP, up to 20 MiB),
each optional. Cancels a running run, snapshots the connection, picks the
workflow variant ([workflow.md, Workflows](workflow.md#workflows)), and
answers `202` `ThumbnailEdit` in `queued` once the prompt is submitted.

| Status | `code` / reason | When |
| --- | --- | --- |
| `400` | `invalid_request` / `thumbnail_edit_mask_invalid` | The mask is not a PNG of the frame's size |
| `400` | `invalid_request` / `thumbnail_edit_reference_invalid` | The reference is not a decodable image |
| `409` | `conflict` / `thumbnail_edit_not_ready` | The connection was removed since the session started |
| `502` | `provider_failed` / a failure reason | Upload or submit failed; the session goes to `failed` with the same reason |

## `POST /api/thumbnail-edits/{editId}/cancel`

`cancelThumbnailEdit`. Answers `200` `ThumbnailEdit` in `cancelled`, or the
current state when no run is in progress. The thumbnail does not change
(acceptance criterion 7).

## `POST /api/thumbnail-edits/{editId}/apply`

`applyThumbnailEdit`. Fits and applies the result
([R-6](../research.md#r-6-the-applied-image-is-user-data-in-sqlite-the-thumbnail-file-is-rebuilt-from-it),
[R-7](../research.md#r-7-the-generated-image-is-fitted-to-the-videos-frame-before-it-is-stored)),
ends the session and answers `200` `Video`.

| Status | `code` / reason | When |
| --- | --- | --- |
| `409` | `conflict` / `thumbnail_edit_no_result` | The state is not `succeeded` |
| `409` | `conflict` / `content_changed` | The video is gone or holds other content ([R-10](../research.md#r-10-apply-revert-and-the-position-override-are-ordered-by-the-generation-lock)); the session ends |
| `409` | `conflict` / `thumbnail_image_invalid` | The result cannot be decoded |

## `DELETE /api/thumbnail-edits/{editId}`

`discardThumbnailEdit`. Cancels a run in progress, drops the session and
answers `204`; an ended session also answers `204`. The page calls it on
discard and when it unmounts.

## `DELETE /api/videos/{id}/thumbnail-image`

`revertThumbnailImage`. Removes the applied image and rebuilds the thumbnail
from the frame (requirement 13); answers `200` `Video`, also when no image was
applied. Errors are those of the position override.

## `PUT /api/videos/{id}/thumbnail-position` (changed)

Setting or clearing a position also removes an applied image, in the same
transaction. Nothing else changes.

## Client use

The video page reads `readiness` to enable its entry, starts an edit at the
player's position, and while a run is in progress reads the session every
second and shows `state`, `elapsedMs` and `progress`; with a result it reads
every 60 seconds until the owner applies, runs again or discards.
