# Contract: Seek sprite HTTP interface

**Feature**: parent Issue #389 | **Plan**: [plan.md](../plan.md)

Source of truth for the machine-readable schema:
[api/openapi.yaml](../../../api/openapi.yaml). This document describes only
what this feature changes. It replaces the `positionMs` JPEG response of the old
contract `specs/009-seek-thumbnail-preview/contracts/seek-thumbnail.md` (removed
from `main` as the document of a finished feature; it remains only in git
history).

## Meaning of `Video.seekThumbnailUrl`

`Video.seekThumbnailUrl`
(`/api/videos/{id}/seek-thumbnail?v=<content-derived version>`) becomes the
versioned URL of the path that returns the layout information. The condition
for it to appear (`probeState = done` and a positive `durationMs`) and the
meaning of `seekThumbnailState`
([specs/020 contracts/processing-api.md, Meaning of `seekThumbnailState`](../../020-seek-thumbnail-stage/contracts/processing-api.md#meaning-of-seekthumbnailstate))
do not change.

## `GET /api/videos/{id}/seek-thumbnail` (`getVideoSeekThumbnail`)

Returns the layout information of the finished sprite. `positionMs` is removed.

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `v` | string | no | The content-derived version returned in `Video.seekThumbnailUrl` |

### Success

- Status: `200 OK`, `Content-Type: application/json`
- `Cache-Control: private, no-cache` and `ETag` (made from the body content).
  When `If-None-Match` matches, `304` (guest-api.md, [Cache of generated files](../../016-single-account-auth/contracts/guest-api.md#cache-of-generated-files))
- Body: `SeekThumbnailSprite`

```yaml
SeekThumbnailSprite:
  type: object
  required: [intervalMs, frameCount, columns, rows, frameWidth, frameHeight, sheets]
  additionalProperties: false
  properties:
    intervalMs:  { type: integer, format: int64, minimum: 5000 }  # frame k covers positions [k*intervalMs, (k+1)*intervalMs)
    frameCount:  { type: integer, minimum: 1, maximum: 600 }      # number of frames across all sheets
    columns:     { type: integer, minimum: 1 }                    # columns per sheet (10)
    rows:        { type: integer, minimum: 1 }                    # rows per sheet (10)
    frameWidth:  { type: integer, minimum: 2 }                    # width of one frame (px); the same for every frame of a video
    frameHeight: { type: integer, minimum: 2 }                    # height of one frame (px)
    sheets:                                                       # versioned sheet URLs, in order, at most 6
      type: array
      minItems: 1
      maxItems: 6
      items: { type: string }
```

Frame `k` (`0 <= k < frameCount`) is on sheet `floor(k / (columns * rows))`,
at index `k mod (columns * rows)` counted from the top left (column
`k mod columns`, row `floor((k mod (columns * rows)) / columns)`). The frame for
position `p` ms is `min(floor(p / intervalMs), frameCount - 1)`; the client does
not hold the interval as a constant (requirement 3). The part of the last sheet
beyond `frameCount` is black, and the client never points there. How the values
are chosen: [research.md R-1](../research.md#r-1-limits-and-interval-rule).

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id` is invalid | `400` | `invalid_request` |
| The video does not exist, or is not shown to guests | `404` | `not_found` |
| Probe unfinished or no duration, or `seekThumbnailState` is not `done` (waiting, generating, failed) | `409` | `conflict` |
| `sprite.json` cannot be read or has the wrong shape | `500` | `internal` |

## `GET /api/videos/{id}/seek-thumbnail/{sheet}` (`getVideoSeekThumbnailSheet`)

Returns the JPEG that `sheets[sheet]` of the layout information points to.
`sheet` is an integer starting at 0.

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `v` | string | no | The content-derived version contained in the layout information's `sheets` |

### Success

- Status: `200 OK`, `Content-Type: image/jpeg`
- `Cache-Control: private, no-cache` and `ETag` (made from the image content).
  When `If-None-Match` matches, `304`
- Body: a JPEG `columns × frameWidth` wide and `rows × frameHeight` high

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id` or `sheet` is not an integer, or is negative | `400` | `invalid_request` |
| The video does not exist, is not shown to guests, or `sheet` is at least the sheet count | `404` | `not_found` |
| Probe unfinished or no duration, or `seekThumbnailState` is not `done` | `409` | `conflict` |
| The image cannot be read | `500` | `internal` |

## Authentication class

Both paths are also returned to guests (add
`GET /api/videos/{id}/seek-thumbnail/{sheet}` to the table in
[internal/httpapi/auth.go](../../../internal/httpapi/auth.go)). A video not
shown to guests returns `404`, as on the other generated-file paths.

## Direct delivery and live transcoding

On both playback paths, the position that picks the frame is the source video's
logical time (seek bar position × `durationMs`); the actual start position of a
live transcode
([docs/design-docs/live-transcode-seek.md](../../../docs/design-docs/live-transcode-seek.md))
plays no part. This keeps 009's decision (requirement 7).
