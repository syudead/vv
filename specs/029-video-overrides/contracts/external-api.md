# Contract: Display name and thumbnail position in external API v1 and MCP

Source of truth: `api/external-v1.yaml`. This document covers only the two
added operations, the changes to `ExternalVideo`, and the two MCP tools. The
common rules (Bearer, the error shape, `VideoRef`, `index`) stay as in
[specs/026-external-api/contracts/external-api.md §1](../../026-external-api/contracts/external-api.md).
The decision is
[research.md R-9](../research.md#r-9-the-external-api-adds-two-bulk-operations-and-mcp-the-same-two-tools).
Following the compatibility policy, only fields and operations are added.

## 0. `ExternalVideo` changes

| Field | Type | Rule |
| --- | --- | --- |
| `title` | string (existing) | The effective title (the display name if there is one). Its description is rewritten |
| `fileTitle` | string (`required`) | The representative location's file name without its extension |
| `displayName` | string \| null (`required`) | The display name. `null` when unset |
| `thumbnailPositionMs` | integer \| null (`required`) | The representative thumbnail position. `null` when unset |

`VideoTagsItem.video` (`VideoTagsVideo`) does not change.

## 1. `POST /api/v1/video-display-names`

`operationId: updateVideoDisplayNames`.

```json
{ "items": [ { "video": { "path": "/media/a.mp4" }, "displayName": "旅行 2024 夏" },
             { "video": { "id": 12 }, "displayName": null } ] }
```

`displayName` is a string or `null` (`required`). `null` and a string that is
empty after trimming clear the name. All items are applied in one transaction;
if any item has an error, nothing is applied.

| Situation | Response |
| --- | --- |
| Success | `200`: `{ items: [{ video: { id, contentKey }, title, fileTitle, displayName }] }` (in `items` order, with the applied values) |
| `items` has 0 or more than 20000 entries | `400 invalid_request` / `too_many_videos`, `limit` |
| `video` is not exactly one of id, content key or path | `400 invalid_request` |
| Control characters, or more than 200 code points | `400 invalid_request` / `display_name_control_characters` or `display_name_too_long` (`limit`); `index` is the position in `items` |
| A video cannot be found | `404 not_found` / `video_not_found`, `index`. Nothing is applied |

The body limit is 32 MiB, the same as `POST /api/v1/video-tags`.

## 2. `POST /api/v1/video-thumbnails`

`operationId: updateVideoThumbnails`.

```json
{ "items": [ { "video": { "path": "/media/a.mp4" }, "positionMs": 12500 },
             { "video": { "contentKey": "…" }, "positionMs": null } ] }
```

`positionMs` is an integer or `null` (`required`). `null` clears (back to the
automatic position). Every item is resolved and its position validated first;
if any has an error, nothing is applied. After validation, images are made and
recorded one item at a time in `items` order (the same processing as the
screen's `PUT /api/videos/{id}/thumbnail-position`;
[screen-api.md §2](screen-api.md#2-put-apivideosidthumbnail-position)).

| Situation | Response |
| --- | --- |
| Every item was applied | `200`: `{ items: [{ video: { id, contentKey }, thumbnailPositionMs }] }` (in `items` order) |
| `items` has 0 or more than 20 entries | `400 invalid_request` / `too_many_videos`, `limit: 20` |
| A `video` reference is invalid | `400 invalid_request` |
| A position is negative or at least the duration | `400 invalid_request` / `thumbnail_position_out_of_range`, `index`, `limit` = that video's duration |
| A video's analysis has not finished | `409 conflict` / `duration_unknown`, `index` |
| A video cannot be found | `404 not_found` / `video_not_found`, `index` |
| No location of a video opens | `404 not_found` / `file_unavailable`, `index` |
| No image could be made for one item partway | `409 conflict` / `thumbnail_frame_unavailable`, `index`. Items before `index` are applied; that item and the rest are not |

`Error.code` gets `conflict`, and `ErrorReason` gets
`display_name_control_characters`, `display_name_too_long`,
`duration_unknown`, `thumbnail_position_out_of_range`,
`thumbnail_frame_unavailable` and `file_unavailable`.

## 3. MCP tools

Added to the table in
[specs/026-external-api/contracts/mcp.md §2](../../026-external-api/contracts/mcp.md).
Input and output have the same shape as the request bodies and responses of
the operations above.

| Tool | Operation | Hints |
| --- | --- | --- |
| `update_video_display_names` | `POST /api/v1/video-display-names` | `destructiveHint: true` (`null` clears), `idempotentHint: true` |
| `update_video_thumbnails` | `POST /api/v1/video-thumbnails` | `destructiveHint: true`, `idempotentHint: true` |

The output of `list_videos` and `get_video` includes the `ExternalVideo`
changes (§0) as is. There are now 8 tools.
