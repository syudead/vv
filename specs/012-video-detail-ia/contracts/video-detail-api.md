# Contract: API changes for the video detail screen

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This document
covers only the fields, endpoints and error meanings this feature adds. Each
implementation unit moves its own part into the schema.

The shape of `Error` (`code`, `message`) and the meaning of existing endpoints do
not change. Three values are added to the `Error.code` enum: `probe_not_failed`,
`open_unavailable` and `file_missing`. 403 uses the existing `forbidden`.

## Added `Video` fields

Both fields are optional and appear only in the response of
`GET /api/videos/{id}`. They are not in these responses:

- `GET /api/videos`
- `GET /api/folders/{rootId}/videos`
- Related videos

| Field | Type | Meaning |
| --- | --- | --- |
| `location` | object | The representative location. A video with no location under a registered folder is a 404 anyway, so this response always has it |
| `location.path` | string | The absolute path of the representative location, as the server sees it (inside a container, the path inside the container) |
| `location.openable` | boolean | `true` when all of these hold: the request comes from loopback, `Host` is a loopback name, and the server runs where it can start the default application (the same conditions as 403 and 409 `open_unavailable` of "open file") |
| `folder` | object | The folder that holds the representative location (`rootId`, `path`, `rootName`). Used for the breadcrumb in the player screen heading. `rootName` is the registered folder's display name (the same rule as `FolderSummary.name`) and appears only in this response. Omitted when the location is in no registered folder. `folder` itself also appears in list responses (013's list-api.md) |
| `seekThumbnailState` | `pending` \| `done` \| `failed` | The state of the seek preview. Present only under the same conditions as the existing `seekThumbnailUrl` (probed, positive duration, has a video codec, has a content key) |

The representative location is chosen as in the existing `GetVideo`: the location
under a registered folder with the smallest path.

`seekThumbnailState` is derived as follows:

| Value | Condition |
| --- | --- |
| `done` | The storage directory (`thumbnails/seek/<prefix>/<contentKey>/`) exists |
| `pending` | The storage directory does not exist, and `thumbnail_state` is `pending` or the video's `thumbnail` job is `queued` or `running` |
| `failed` | Anything else (the job is `failed`, or the job row is gone after its retention period) |

When the thumbnail job fails terminally at a stage before the representative
thumbnail, `thumbnail_state` also becomes `failed` in the same transaction (the
plan's Structural Decisions 13). So a `failed` job never leaves `thumbnail_state`
at `pending`.

## Related videos: `GET /api/videos/{id}/related`

The response is 200 with `RelatedVideos`.

```yaml
RelatedVideos:
  type: object
  required: [items]
  additionalProperties: false
  properties:
    items:
      type: array
      maxItems: 20
      items: { $ref: "#/components/schemas/Video" }
    nextId:
      type: integer
      format: int64
      description: The next video in natural order in the same directory. Omitted when there is none
    prevId:
      type: integer
      format: int64
      description: |
        The previous video in natural order in the same directory. Omitted when there is none. Not necessarily in items
```

Each `Video` in `items` carries `progress`, as in the list.

The order is as follows, up to 20 items:

1. Videos directly in the same directory as the representative location that
   come after this video in natural order of file name (`domain.CompareNatural`)
2. Videos directly in the same directory that come before this video
3. Videos not in 1 or 2, by the smallest difference in date added. On equal
   differences, the larger `id` comes first

Notes:

- The order in 1 and 2 is total. When `CompareNatural` returns 0, the byte order
  of the file name decides, and then the smaller `id` comes first.
- A video with two locations in the same directory is sorted by the file name of
  the location with the smaller path, and counts as one item.
- This video's own file name used for the comparison in 1 and 2 is that of the
  representative location.
- This video itself is excluded, as are videos with no location under a
  registered folder.
- `nextId` is the `id` of the first item of 1, omitted when 1 is empty. The screen
  uses it as "next video".
- `prevId` is the `id` of the last item of 2 (right before this video), omitted
  when 2 is empty. The screen uses it as "previous video" at the left end of the
  player. 2 is in ascending order, so it can be cut at 20 items and not be in
  `items`. The screen navigates by `id` only, and does not show a title it does
  not know.
  - Rejected: the screen treating `items[0]` as the next video. When 1 is empty,
    `items[0]` is a preceding video or one with a close date added, which breaks
    requirement 14 ("when there is no next video, only `もう一度見る`"). Related
    video items also have no `location`, so the screen cannot compare directories
    either.

Errors:

| Status | Situation | `code` |
| --- | --- | --- |
| 404 | Unknown id, or no location under a registered folder | `not_found` |

## Probe again: `POST /api/videos/{id}/probe`

The request has no body.

- Success is 202 with the updated `Video` (`probeState: pending`).
- In one transaction, the server does the following (the same shape as
  `RequeuePreviewRepair`):
  - With `where probe_state='failed'`, set `probe_state` back to `pending` and
    clear `probe_error`. If 0 rows are updated, return 409.
  - If `thumbnail_state` is not `done`, set it back to `pending`.
  - If `thumbnail_state` is `done` but the seek preview's storage directory does
    not exist, leave the state and enqueue a `thumbnail` job. The existing
    `thumbnailHandler` does not rebuild the representative thumbnail when it
    exists and builds only the seek preview.
  - If `preview_state` is `failed`, set it back to `pending`.
  - Enqueue a `probe` job. When `thumbnail_state` was set back, or when the seek
    preview's storage directory does not exist, also enqueue a `thumbnail` job.
    For both, delete the finished rows of that kind first, then insert with
    `on conflict … do nothing`.
- The preview job is enqueued by the existing `probeHandler` after a successful
  probe.
- `probe_state='failed'` means the video's probe job has finished, because a
  terminal failure is recorded in the same transaction that marks the job
  `failed` (the plan's Structural Decisions 13). So the new job enqueued here is
  never skipped by duplicate prevention against an old job that is still running.

| Status | Situation | `code` |
| --- | --- | --- |
| 404 | Unknown id, or no location under a registered folder | `not_found` |
| 409 | `probeState` is not `failed` (probing or probed) | `probe_not_failed` |

The screen treats 409 as "already being retried", refetches the video and moves
to the stage display.

## Open file: `POST /api/videos/{id}/open`

The request has no body and accepts no path.

- Success is 204, returned once a child process that opens the representative
  location with the server PC's default application has started.
- Whether the application actually opened is not checked.

The checks run from the top of the table, and the first match is returned.

| Status | Situation | `code` |
| --- | --- | --- |
| 404 | Unknown id, or no location under a registered folder | `not_found` |
| 409 | The server runs where it cannot start the default application (command not found, no display; regardless of the requester) | `open_unavailable` |
| 403 | The request does not come from loopback, or `Host` is not a loopback name (`localhost`, `127.0.0.1`, `[::1]`) | `forbidden` (existing) |
| 409 | The representative location has no file (moved or deleted) | `file_missing` |
| 500 | The child process could not be started | `internal` |

The existing same-origin check for POST (`mutationBoundary`) applies to this
endpoint unchanged. The `Host` check keeps DNS rebinding from getting around it.

403 and 409 `open_unavailable` are decided with the same conditions as
`location.openable`. The screen shows no link when `openable: false` so that the
user never sees these two.
