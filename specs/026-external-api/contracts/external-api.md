# Contract: External API v1

Parent Issue: #493 (Requirements 5-7, 10 and 11). The source of truth is `api/external-v1.yaml`,
created during implementation
([research.md R-5](../research.md#r-5-the-external-api-contract-is-a-separate-openapi-document-only-go-is-generated)).
This contract fixes the operations and rules that document must satisfy.

## 1. Common rules

- The base path is `/api/v1`. Every operation has `security: bearerAuth` (`type: http`,
  `scheme: bearer`).
- Authentication
  ([research.md R-3](../research.md#r-3-the-external-api-lives-under-apiv1-the-boundary-gets-a-bearer-class)):
  a missing, malformed, invalid or revoked `Authorization: Bearer <token>` returns
  `401 unauthenticated` with `WWW-Authenticate: Bearer`. Cookies are not read.
- No same-origin check applies. Operations that take a body require
  `Content-Type: application/json`
  ([R-4](../research.md#r-4-bearer-requests-skip-the-same-origin-check)).
- The viewer is always the owner, so private videos are returned too (Acceptance criterion 2).
- Errors use the same shape as the UI API: `{ code, message, reason?, limit?, index? }`. `index`
  exists only in this API. It names the position (from 0) in `videos` of §4 that caused the error.
  Only the `code` and `reason` values listed here go into the enums of `external-v1.yaml`.

## 2. Videos

```yaml
ExternalVideo:
  required: [id, contentKey, title, durationMs, addedAt, locations, tags]
  properties:
    id: { type: integer, format: int64 }
    contentKey: { type: string }
    title: { type: string }
    durationMs: { type: [integer, "null"] }        # null before probing
    addedAt: { type: string, format: date-time }
    locations:                                    # current locations under media folders; representative first
      type: array
      items:
        required: [path, fileName]
        properties:
          path: { type: string }                  # absolute path
          fileName: { type: string }
    tags:
      type: array
      items: { $ref: ExternalVideoTag }

ExternalVideoTag:                                 # domain.VideoTag
  required: [id, name, manual, fromFolder]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }
    manual: { type: boolean }                     # attached by hand (this API attaches and detaches these)
    fromFolder: { type: boolean }                 # derived from ancestor folder names (this API does not remove it)
```

### `GET /api/v1/videos`

| Parameter | Default | Rule |
| --- | --- | --- |
| `cursor` | Start | The previous `nextCursor`. An unparsable value returns `400 invalid_request` / `invalid_cursor` |
| `limit` | 100 | 1 to `domain.MaxLimit` (200). Out of range returns `400 invalid_request` |

`200`: `{ items: ExternalVideo[], nextCursor: string }`. The order is fixed: ascending
`(addedAt, id)`. When nothing follows, `nextCursor` is the empty string
([R-6](../research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)).

The list reads every video from the start. It does not track changes. A client that wants new
videos reads the list again (Acceptance criterion 3). When videos are added, deleted or moved during
paging, the next request does not fail and returns what follows at that moment. The API does not
promise to prevent misses or duplicates across one full read (Edge cases).

Like `lookup`, the list returns only videos with at least one location under a media folder. A video
that lost every location under the media folders and remains only with locations outside them is
treated like a deleted row. It does not appear in the list, and no deletion marker (tombstone) is
returned. The Issue does not ask for notification of removed videos (webhooks are out of scope). A
client learns that a video it holds is gone from the `404` of `lookup` (documented in
`docs/how-to/external-api.md`).

### `GET /api/v1/videos/lookup`

Takes exactly one of `id`, `contentKey` and `path`. Zero or two or more return
`400 invalid_request`.

- `200`: `ExternalVideo`.
- `404 not_found` / `video_not_found`: the video does not exist, or has no location under a media
  folder.

`path` is an absolute path. It is not normalized, and it is compared byte for byte with the `path`
of the current locations. Locations are stored in the file system's spelling, including NFD
(`TestScanPreservesPathAndNormalizesTitleToNFC` in `internal/scanner`). A `path` returned by the list
therefore finds the video when passed back unchanged.

## 3. Tags

### `GET /api/v1/tags`

`200`: `{ items: [{ id, name, synonyms: string[], videoCount }] }`. Same order as the UI's
`ListTags`.

## 4. Video tags

### `POST /api/v1/video-tags`

```json
{ "videos": [{ "id": 1 }, { "contentKey": "…" }, { "path": "/media/a.mp4" }],
  "action": "add",
  "tags": ["名前", "シノニム"] }
```

Only manually attached tags are attached or detached. Folder-derived tags stay, also after
`replace`. The rules are in
[research.md R-7](../research.md#r-7-tag-operations-are-a-strict-bulk-operation-added-to-tagstore).

| Case | Response |
| --- | --- |
| Success | `200`: `{ items: [{ video: { id, contentKey }, tags: ExternalVideoTag[] }] }` (in `videos` order, tags after the operation) |
| A `videos` element does not have exactly one of id, content key and path, or `action` is invalid | `400 invalid_request` |
| `videos` has 0 elements or more than 20000 | `400 invalid_request` / `too_many_videos`, `limit` |
| `tags` has 0 elements for `add` or `remove`, or more than 100 | `400 invalid_request` / `too_many_tags`, `limit` |
| A name is empty, contains control characters or is too long | `400 invalid_request` / `tag_name_empty`, `tag_name_control_characters` or `tag_name_too_long`; `index` is the position in `tags` |
| A video cannot be found | `404 not_found` / `video_not_found`, `index`. Nothing is applied |

For name errors, `index` points into `tags`. Every other `index` points into `videos`.

## 5. Scans

```yaml
ExternalScan:
  required: [id, status, startedAt, finishedAt, videos, settledVideos, errorCode]
  properties:
    id: { type: integer, format: int64 }
    status: { enum: [finding, running, done, partial, failed] }   # same as Scan.status in domain
    startedAt: { type: string, format: date-time }
    finishedAt: { type: [string, "null"], format: date-time }
    videos: { type: [integer, "null"] }            # null while finding (the count is not finished)
    settledVideos: { type: [integer, "null"] }     # same; after counting, 0 ≤ settledVideos ≤ videos
    errorCode: { type: [string, "null"] }
```

`videos` and `settledVideos` carry numbers only when `domain.ImportProgress.Counted` is true. When it
is false, both are `null`. This is the same test the UI API uses to omit `videos`. A client can tell
`finding` right after the walk starts from a `done` scan with 0 targets
(`{ videos: 0, settledVideos: 0 }`).

- `POST /api/v1/scans`: `201` when a new scan starts, `200` when the running scan is returned.
  - The choice comes from `started` of `ScanStore.StartScan`, which decides in one transaction
    whether to create a scan row or return the running one. `app.Scans.StartScan` changes to return
    `(domain.Scan, started bool, error)` and passes it through.
  - The UI's `POST /api/scans` does not read `started` and keeps returning `202`.
  - The handler does not read `GET /api/v1/scans/current` before responding, because a scan can
    start or finish in between.
  - Without media folders, it returns `409 media_folders_not_configured`, like the UI API.
- `GET /api/v1/scans/current`: `200 ExternalScan`. If no scan has ever run, `404 not_found` /
  `no_scan`.
