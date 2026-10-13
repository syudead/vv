# Data model: Set a local image as the thumbnail and edit the current thumbnail with ComfyUI

Parent Issue: #980.

The rest of the model is unchanged: the tables are canonical in
[internal/store/migrations/](../../internal/store/migrations/), and
`thumbnail_images` itself is in
[045 data-model](../045-comfyui-thumbnails/data-model.md#thumbnail_images).
This file covers the two columns this feature adds to it and the values the
session and the API gain.

## Migration

`00041_thumbnail_image_sources.sql` (the last migration on `main` is
`00040_thumbnail_images.sql`; the implementation takes the next free number if
another feature lands first). Up adds the two columns below; down drops them.

## `thumbnail_images` (changed)

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `source` | blob | Yes | The applied image fitted to the video's aspect with its long side at most 2048 px, as a JPEG ([R-3](research.md#r-3-the-row-keeps-the-applied-images-source-at-up-to-2048-px-and-its-origin)); null in rows written before this migration |
| `origin` | text | No | `generated` or `uploaded`; the migration fills existing rows with `generated` |

**Relationships**: Unchanged.

## Rules

| Rule | Enforced in |
| --- | --- |
| Every apply writes `image`, `source` and `origin` in the one transaction that sets `thumbnail_state` to `done` | `app.Ingest.ApplyThumbnailImage`, `OverrideStore` |
| A position change and a revert delete the row, source included | Unchanged: `OverrideStore.SetThumbnailPosition`, `RevertThumbnailImage` |
| The thumbnail job republishes `image`; it never reads `source` | Unchanged: `app.Ingest.Thumbnail` |
| An edit from the current thumbnail reads `source`, or `image` when `source` is null | `app.ThumbnailEdits.Start` ([R-4](research.md#r-4-an-edit-from-the-current-thumbnail-starts-from-the-stored-source-or-from-the-thumbnails-frame)) |
| No row has an empty `source` (null is allowed) or an `origin` outside the two values | The writer; `internal/store/invariants_test.go` |

## Values added to `domain`

| Value | Content |
| --- | --- |
| `ThumbnailImageOrigin` | `generated`, `uploaded` |
| `Video.ThumbnailImageOrigin` | The row's `origin`; empty when no image is applied. Answered as `thumbnailImage` ([R-5](research.md#r-5-thumbnailimage-names-the-applied-images-origin-in-place-of-thumbnailgenerated)) |
| `ThumbnailEditSource` | `frame`, `thumbnail` ([R-4](research.md#r-4-an-edit-from-the-current-thumbnail-starts-from-the-stored-source-or-from-the-thumbnails-frame)) |
| `ThumbnailUploadMaxBytes`, `ThumbnailUploadMaxPixels` | 64 MiB, 120 million ([R-2](research.md#r-2-an-upload-is-a-png-jpeg-or-still-webp-of-at-most-64-mib-and-120-megapixels-decoded-one-at-a-time)) |
| `ErrThumbnailUploadTooLarge`, `ErrThumbnailUploadInvalid` | The two `400` reasons of the upload route |

## Edit session (changed)

The session of [045 data-model](../045-comfyui-thumbnails/data-model.md#edit-session)
gains two fields; its states and their transitions are unchanged.

| Field | Meaning |
| --- | --- |
| `Source` | `frame` or `thumbnail`: what the owner chose |
| `PositionMs` | The position the frame was taken at when the image is a frame (chosen, recorded, automatic, or `0` after the first-frame fallback); absent when it is a stored source |

The session's `Frame` holds the chosen image whichever its source, and the
mask is checked against its size as before.

## What does not change

`video_overrides` keeps its position while an image is applied, and the
`thumbnailUrl` version keeps its `-g<revision>` form, so every screen that
shows a thumbnail picks up an upload through the existing field. Seek sprites,
card scrub frames and hover previews keep coming from the video (the Issue's
`対象外`).
