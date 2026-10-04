# Contract: Tentative attach in the external API v1 and MCP

Source of truth: `api/external-v1.yaml`. This document describes only the fields
this feature adds. The shared rules (Bearer, the error shape, `VideoRef`, `index`,
count limits) stay as in
[specs/026-external-api/contracts/external-api.md](../../026-external-api/contracts/external-api.md).
The decision is [research.md R-7](../research.md#r-7-the-external-api-adds-tentative-to-the-request-and-skipped-names-to-the-response).
Following the compatibility policy, the change only adds fields; it adds no
operation.

## Schema changes

| Schema | Added field | Rule |
| --- | --- | --- |
| `Tag` (`GET /api/v1/tags`) | `tentative: boolean` (`required`) | Same as the screen's `Tag` |
| `ExternalVideoTag` | `tentative: boolean` (`required`) | The state of a tag on a video |
| `VideoTagsRequest` | `tentative: boolean` (optional, `false` when omitted) | [Changes to `POST /api/v1/video-tags`](#changes-to-post-apiv1video-tags) |
| `VideoTagsResponse` | `skippedTags: string[]` (`required`) | [Changes to `POST /api/v1/video-tags`](#changes-to-post-apiv1video-tags). An empty array when no name was skipped |

## Changes to `POST /api/v1/video-tags`

```json
{ "videos": [{ "path": "/media/a.mp4" }],
  "action": "add",
  "tags": ["猫", "高画質"],
  "tentative": true }
```

| Case | Behaviour |
| --- | --- |
| `tentative` is `false` or omitted | As today. A missing name is created as a confirmed tag; if the name is a rejected name, it is removed from the list (requirements 3 and 15) |
| `tentative` is `true` with `add` or `replace`: a name matching an existing tag's name or synonym | As today: attaches that tag. Its state does not change, tentative or confirmed (requirement 2) |
| `tentative` is `true` with `add` or `replace`: a name matching no tag | Unless it is in the rejected-name list, it is created as a **tentative tag** and attached. When one request attaches the same name to several videos, only one tag is created (edge case) |
| `tentative` is `true` with `add` or `replace`: a rejected name (exact spelling match; requirement 13) | Skipped: no tag is created or attached, and the normalized name is returned once in `skippedTags`, in `tags` order. The other names are processed and the request succeeds with `200` (requirements 11 and 12) |
| `tentative` is `true` with `replace` | A skipped name is treated as if it had never been sent and does not enter the replacement set. If every name in `tags` is rejected, the result equals replacing with an empty set (edge case) |
| `tentative` is `true` with `remove` | As today (a name matching no tag does nothing). `skippedTags` is an empty array |

Response:

```json
{ "items": [{ "video": { "id": 1, "contentKey": "…" },
              "tags": [{ "id": 3, "name": "猫", "manual": true, "fromFolder": false, "tentative": false }] }],
  "skippedTags": ["高画質"] }
```

Error `code` and `reason` values do not change. A `tentative` that is not a
boolean returns `400 invalid_request` (a malformed body, handled by the current
`readJSONBody`).

## MCP

The input of `update_video_tags` is derived from the `VideoTagsRequest` type, so
`tentative` arrives as is. The tool description gains: "with `tentative: true`,
tags created by this call are tentative, and rejected names are skipped and
returned in `skippedTags`". The output of `list_tags`, `get_video` and
`list_videos` carries the [Schema changes](#schema-changes) as is. The number of tools and their
annotations do not change
([specs/026-external-api/contracts/mcp.md](../../026-external-api/contracts/mcp.md)).

## `docs/how-to/external-api.md`

The section on tagging videos (`動画にタグを付ける`) gains a description of
`tentative` and `skippedTags` and a scraper example (attach names an LLM produced
with `tentative: true`, and the user sorts them out on the tag management page).
Confirming and rejecting tentative tags and the rejected-name list are screen
operations and are not part of this API.
