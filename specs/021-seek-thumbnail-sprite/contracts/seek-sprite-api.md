# Seek sprite HTTP contract

**Feature**: parent Issue #389 | **Plan**: [plan.md](../plan.md)

The source of truth for the machine-readable schema is
[api/openapi.yaml](../../../api/openapi.yaml). This file describes only the differences this
feature makes. This contract replaces the `positionMs` JPEG response of the old contract
`specs/009-seek-thumbnail-preview/contracts/seek-thumbnail.md` (removed from `main` as a
document of a finished feature; it remains only in the git history).

## 1. Meaning of `Video.seekThumbnailUrl`

`Video.seekThumbnailUrl` (`/api/videos/{id}/seek-thumbnail?v=<content-derived version>`) becomes
the versioned URL of the route that returns the layout. The condition for including it
(`probeState = done` and a positive `durationMs`) and the meaning of `seekThumbnailState`
([specs/020 contracts/processing-api.md §2](../../020-seek-thumbnail-stage/contracts/processing-api.md#2-meaning-of-seekthumbnailstate))
do not change.

## 2. `GET /api/videos/{id}/seek-thumbnail` (`getVideoSeekThumbnail`)

Returns the layout of the finished sprite. `positionMs` is removed.

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `v` | string | no | The content-derived version that `Video.seekThumbnailUrl` returned |

### Success

- Status: `200 OK`, `Content-Type: application/json`
- `Cache-Control: private, no-cache` and `ETag` (derived from the body content). A matching
  `If-None-Match` returns `304` (guest-api.md §5)
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
    frameWidth:  { type: integer, minimum: 2 }                    # frame width (px); the same for every frame of a video
    frameHeight: { type: integer, minimum: 2 }                    # frame height (px)
    sheets:                                                       # versioned sheet URLs, in sheet order, at most 6
      type: array
      minItems: 1
      maxItems: 6
      items: { type: string }
```

- Frame `k` (`0 <= k < frameCount`) is on sheet `floor(k / (columns * rows))`, at position
  `k mod (columns * rows)` counted from the top left (column `k mod columns`, row
  `floor((k mod (columns * rows)) / columns)`).
- The frame for position `p` ms is `min(floor(p / intervalMs), frameCount - 1)`. The client does
  not hold the interval as a fixed value (Requirement 3).
- The part of the last sheet beyond `frameCount` is black; the client never points there.
- How the values are chosen: [research.md R-1](../research.md#r-1-at-most-600-frames-on-6-sheets-the-interval-widens-only-past-50-min).

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| Invalid `id` | `400` | `invalid_request` |
| No such video, or a video hidden from guests | `404` | `not_found` |
| Analysis not finished, no duration, or `seekThumbnailState` is not `done` (pending, generating or failed) | `409` | `conflict` |
| `sprite.json` cannot be read or has the wrong shape | `500` | `internal` |

## 3. `GET /api/videos/{id}/seek-thumbnail/{sheet}` (`getVideoSeekThumbnailSheet`)

Returns the JPEG that `sheets[sheet]` of the layout points to. `sheet` is an integer starting at
0.

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `v` | string | no | The content-derived version contained in `sheets` of the layout |

### Success

- Status: `200 OK`, `Content-Type: image/jpeg`
- `Cache-Control: private, no-cache` and `ETag` (derived from the image content). A matching
  `If-None-Match` returns `304`
- Body: a JPEG `columns × frameWidth` wide and `rows × frameHeight` high

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id` or `sheet` is not an integer, or is negative | `400` | `invalid_request` |
| No such video, a video hidden from guests, or `sheet` not below the sheet count | `404` | `not_found` |
| Analysis not finished, no duration, or `seekThumbnailState` is not `done` | `409` | `conflict` |
| The image cannot be read | `500` | `internal` |

## 4. Authentication class

Both routes also serve guests (add `GET /api/videos/{id}/seek-thumbnail/{sheet}` to the table in
[internal/httpapi/auth.go](../../../internal/httpapi/auth.go)). A video hidden from guests returns
`404`, as on the other generated-asset routes.

## 5. Direct playback and live transcoding

On both playback paths, the position that selects the frame is the logical time of the source
video (seek bar position × `durationMs`). The actual start position of live transcoding
([docs/design-docs/live-transcode-seek.md](../../../docs/design-docs/live-transcode-seek.md)) plays
no part. This keeps the 009 decision (Requirement 7).
