# Contract: Work information in external API v1 and MCP

Source of truth: `api/external-v1.yaml`, operation `updateVideoWorkInfo` and
the `workInfo` field of `ExternalVideo`. This document covers only what this
feature adds; the common rules (Bearer, the error shape, `VideoRef`, `index`)
stay as in
[specs/026-external-api/contracts/external-api.md, Common rules](../../026-external-api/contracts/external-api.md#common-rules).
The decision is
[research.md R-6](../research.md#r-6-the-external-api-gets-one-bulk-operation-and-mcp-one-tool-workinfo-joins-externalvideo).
Following the compatibility policy, only fields, an operation, reasons and an
error property are added.

## `ExternalVideo` changes

| Field | Type | Rule |
| --- | --- | --- |
| `workInfo` | `WorkInfo` (`required`) | The six fields of [screen-api.md, `WorkInfo` schema](screen-api.md#workinfo-schema); in this API an unset single-value field is `null` rather than absent, and `performers` is `[]` when none, so a client can read every property without checking for presence |
| `updatedAt` | date-time (existing) | Its description gains work information to the list of edits that advance it |

`VideoTagsItem.video` does not change.

## `POST /api/v1/video-work-info`

`operationId: updateVideoWorkInfo`.

```json
{ "items": [ { "video": { "path": "/media/a.mp4" },
               "workInfo": { "code": "ABC-123", "releaseDate": "2024-06-01",
                             "maker": "Example Studio", "performers": ["山田 花子"],
                             "description": "…", "sourceUrl": "https://example.com/abc-123" } },
             { "video": { "id": 12 }, "workInfo": {} } ] }
```

Each item's `workInfo` is a
[`WorkInfoInput`](screen-api.md#workinfoinput-schema), shared with the screen
API's request body, and replaces the stored object: a field absent, `null` or empty after
trimming is cleared, and `{}` clears everything. All items are applied in one
transaction; if any item has an error, nothing is applied. A later item for
the same video wins.

| Situation | Response |
| --- | --- |
| Success | `200`: `{ items: [{ video: { id, contentKey }, workInfo }] }` in `items` order, with the applied values in the `ExternalVideo` shape |
| `items` has 0 or more than 20000 entries | `400 invalid_request` / `too_many_videos`, `limit` |
| `video` is not exactly one of id, content key or path | `400 invalid_request` |
| A field breaks its rule ([screen-api.md, `PUT /api/videos/{id}/work-info`](screen-api.md#put-apivideosidwork-info)) | `400 invalid_request` with the same `reason`, `field` and `limit`; `index` is the position in `items` |
| A video cannot be found | `404 not_found` / `video_not_found`, `index`. Nothing is applied |

The body limit is 32 MiB, the same as `POST /api/v1/video-tags`. Repeating a
request leaves the state unchanged and returns `200`.

`Error` gains the optional `field`, and `ErrorReason` gets
`work_info_control_characters`, `work_info_too_long`,
`work_info_invalid_date`, `work_info_invalid_url` and `too_many_performers`.

## MCP tools

Added to the table in
[specs/026-external-api/contracts/mcp.md, Tools](../../026-external-api/contracts/mcp.md#tools).
Input and output have the same shape as the request body and the response of
the operation above.

| Tool | Operation | Hints |
| --- | --- | --- |
| `update_video_work_info` | `POST /api/v1/video-work-info` | `destructiveHint: true` (a cleared field is lost), `idempotentHint: true` |

The output of `list_videos` and `get_video` includes `workInfo` as is. There
are now 15 tools.

## Client use

`docs/how-to/external-api.md` gains a section "Set work information" with the
request above, the field rules as a table, and a note that a scraper writes
the whole object it holds.
