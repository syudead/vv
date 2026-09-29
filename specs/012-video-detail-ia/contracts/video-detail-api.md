# Contract: API changes for the video detail page

The source of truth is [api/openapi.yaml](../../../api/openapi.yaml). This document lists only the
fields, routes and error meanings that this feature adds. Each unit of work moves its own part into
that file.

The shape of `Error` (`code`, `message`) and the meaning of existing routes do not change. The
`Error.code` enum gains 3 values: `probe_not_failed`, `open_unavailable` and `file_missing`. A 403
uses the existing `forbidden`.

## Added `Video` fields

Both fields are optional and appear only in the response of `GET /api/videos/{id}`. They do not
appear in these responses:

- `GET /api/videos`
- `GET /api/folders/{rootId}/videos`
- Related videos

| Field | Type | Meaning |
| --- | --- | --- |
| `location` | object | The representative location. A video with no location under a media folder is a 404, so this response always has it. |
| `location.path` | string | Absolute path of the representative location, as the server sees it. Inside a container, it is the path inside the container. |
| `location.openable` | boolean | `true` when all of these hold: the requester is loopback, `Host` is a loopback name, and the server can start the default application (the same conditions as the 403 and the 409 `open_unavailable` of "Open file"). |
| `folder` | object | The folder that holds the representative location (`rootId`, `path`, `rootName`). The breadcrumbs in the playback page header use it. `rootName` is the media folder's display name (same rule as `FolderSummary.name`) and appears only in this response. Omitted when the location is in no media folder. `folder` itself also appears in list responses (list-api.md of 013). |
| `seekThumbnailState` | `pending` \| `done` \| `failed` | State of the seek preview. Present only under the same conditions as the existing `seekThumbnailUrl`: probed, positive length, has a video codec, has a content key. |

The representative location is chosen as in the existing `GetVideo`: the location with the smallest
path among those under a media folder.

`seekThumbnailState` is derived as follows:

- `done`: the storage directory (`thumbnails/seek/<prefix>/<contentKey>/`) exists.
- `pending`: the storage directory does not exist, and `thumbnail_state` is `pending` or the
  video's `thumbnail` job is `queued` or `running`.
- `failed`: any other case (the job is `failed`, or the job row was deleted after its retention
  period).

When the thumbnail job fails terminally at a step before the representative thumbnail,
`thumbnail_state` also becomes `failed` in the same transaction (Structural Decisions 13 of the
plan). So a `failed` job never leaves `thumbnail_state` at `pending`.

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
      description: The next video in the same directory in natural order. Omitted when there is none.
    prevId:
      type: integer
      format: int64
      description: |
        The previous video in the same directory in natural order. Omitted when
        there is none. Not necessarily in items.
```

Each `Video` in `items` carries `progress`, as in the list.

The order is as follows, up to 20 items:

1. Videos directly in the same directory as the representative location that come after this video
   in natural file-name order (`domain.CompareNatural`).
2. Videos directly in the same directory that come before this video.
3. Videos in neither 1 nor 2, by smallest difference in date added. On a tie, the larger `id` comes
   first.

Notes:

- The order in 1 and 2 is total. When `CompareNatural` returns 0, the byte order of the file name
  decides; when that is also equal, the smaller `id` comes first.
- A video with 2 locations in the same directory sorts by the file name of the location with the
  smaller path and counts as 1 item.
- The file name of this video used in the comparison for 1 and 2 is that of its representative
  location.
- This video itself is excluded. Videos with no location under a media folder are excluded.
- `nextId` is the `id` of the first item of 1, omitted when 1 is empty. The page uses it as the
  "Next video".
- `prevId` is the `id` of the last item of 2 (the video just before this one), omitted when 2 is
  empty. The page uses it as the "Previous video" at the left edge of the player. 2 is in ascending
  order, so the 20-item cut can leave it out of `items`. The page navigates by `id` alone and
  hides the title when it is unknown.
  - Rejected: the page treats `items[0]` as the next video. When 1 is empty, `items[0]` is an
    earlier video or one close in date added, which breaks Requirement 14 ("when there is no next
    video, show only 'Watch again'"). Also, related items have no `location`, so the page cannot
    compare directories.

Errors:

| Status | Condition | `code` |
| --- | --- | --- |
| 404 | Unknown id, or no location under a media folder | `not_found` |

## Probe again: `POST /api/videos/{id}/probe`

The request has no body.

- Success is 202 with the updated `Video` (`probeState: pending`).
- The server does the following in one transaction (the same shape as `RequeuePreviewRepair`):
  - Resets `probe_state` to `pending` and clears `probe_error`, with `where probe_state='failed'`.
    When 0 rows are updated, the result is 409.
  - Resets `thumbnail_state` to `pending` unless it is `done`.
  - When `thumbnail_state` is `done` but the seek preview directory does not exist, leaves the
    state as is and enqueues a `thumbnail` job. The existing `thumbnailHandler` does not rebuild
    an existing representative thumbnail; it builds only the seek preview.
  - Resets `preview_state` to `pending` when it is `failed`.
  - Enqueues a `probe` job. Also enqueues a `thumbnail` job when `thumbnail_state` was reset or the
    seek preview directory does not exist. For both, it deletes the finished rows of that kind, then
    inserts with `on conflict … do nothing`.
- The existing `probeHandler` enqueues the preview job after a successful probe.
- `probe_state='failed'` means the video's probe job has finished. A terminal failure is recorded
  in the same transaction that sets the job to `failed` (Structural Decisions 13 of the plan). So
  deduplication against a still-running old job never drops the new job enqueued here.

| Status | Condition | `code` |
| --- | --- | --- |
| 404 | Unknown id, or no location under a media folder | `not_found` |
| 409 | `probeState` is not `failed` (probing or probed) | `probe_not_failed` |

The page treats a 409 as "a retry is already running": it refetches the video and switches to the
stage display.

## Open file: `POST /api/videos/{id}/open`

The request has no body. It takes no path.

- Success is 204. The server returns it once it has started a child process that opens the
  representative location in the default application of the server PC.
- The server does not check whether the application opened.

Checks run from the top of the table down; the first match is returned.

| Status | Condition | `code` |
| --- | --- | --- |
| 404 | Unknown id, or no location under a media folder | `not_found` |
| 409 | The server cannot start the default application (command not found, no display; regardless of requester) | `open_unavailable` |
| 403 | The requester is not loopback, or `Host` is not a loopback name (`localhost`, `127.0.0.1`, `[::1]`) | `forbidden` (existing) |
| 409 | The representative location has no file (moved or deleted) | `file_missing` |
| 500 | The child process could not be started | `internal` |

The existing same-origin check for POST (`mutationBoundary`) also applies to this route. The `Host`
check keeps DNS rebinding from bypassing it.

The 403 and the 409 `open_unavailable` use the same conditions as `location.openable`. The page
hides the link when `openable: false` so that users never see these 2 errors.
