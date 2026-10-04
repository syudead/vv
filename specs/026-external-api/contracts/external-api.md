# Contract: External API v1

Parent Issue: #493 (requirements 5–7, 10 and 11). Source of truth: `api/external-v1.yaml`, created at
implementation time
([research.md R-5](../research.md#r-5-the-external-api-contract-is-a-separate-openapi-document-generating-go-only)).
This file fixes the operations and rules that document must satisfy.

## Common rules

- The base is `/api/v1`. Every operation is `security: bearerAuth` (`type: http`, `scheme: bearer`).
- Authentication
  ([research.md R-3](../research.md#r-3-the-external-api-lives-under-apiv1-and-the-boundary-gains-a-bearer-class)):
  a missing, malformed, invalid or revoked `Authorization: Bearer <token>` returns
  `401 unauthenticated` with `WWW-Authenticate: Bearer`. Cookies are not read.
- No same-origin check. Operations that take a body require `Content-Type: application/json`
  ([R-4](../research.md#r-4-bearer-requests-skip-the-same-origin-check)).
- The viewer is always the owner, and private videos are returned too (acceptance criterion 2).
- Errors have the same shape as the screen API, `{ code, message, reason?, limit?, index? }`. `index`
  exists only in this API and gives the position (from 0) in [Video tags](#video-tags)'s `videos` that caused the error. Only
  the `code` and `reason` values listed here go into the enums in `external-v1.yaml`.

## Videos

```yaml
ExternalVideo:
  required: [id, contentKey, title, durationMs, addedAt, locations, tags]
  properties:
    id: { type: integer, format: int64 }
    contentKey: { type: string }
    title: { type: string }
    durationMs: { type: [integer, "null"] }        # null before analysis
    addedAt: { type: string, format: date-time }
    locations:                                    # current locations under registered folders; the representative one first
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
    manual: { type: boolean }                     # attached by hand (what this API attaches and detaches)
    fromFolder: { type: boolean }                 # derived from an ancestor folder name (this API cannot detach it)
```

### `GET /api/v1/videos`

| Parameter | Default | Rule |
| --- | --- | --- |
| `cursor` | Start | The previous `nextCursor`. An unparsable value is `400 invalid_request` / `invalid_cursor`. |
| `limit` | 100 | 1 to `domain.MaxLimit` (200). Out of range is `400 invalid_request`. |

`200`: `{ items: ExternalVideo[], nextCursor: string }`. The order is fixed at `(addedAt, id)`
ascending. When there is nothing more, `nextCursor` is the empty string
([R-6](../research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)).

The list is for reading everything from the start; it does not track changes. A user who wants new
videos reads the list again (acceptance criterion 3). If videos are added, removed or moved during
paging, the next request does not fail and returns the continuation as of that moment, but there is
no promise against missed or duplicated items across a full read (Edge Cases).

Like `lookup`, the list returns only videos that have at least one location under a registered
folder. A video that lost every location under the registered folders and remains only with locations
outside them is left out of the list, the same as a video whose row is gone, and no item (tombstone)
reports that it is gone. The Issue does not ask for notification of removed videos (webhooks are out
of scope too). The user learns that a held video is gone from a `404` on `lookup` (documented in
`docs/how-to/external-api.md`).

### `GET /api/v1/videos/lookup`

Takes exactly one of `id`, `contentKey` and `path`. Zero, or two or more, is `400 invalid_request`.

| Status | Meaning |
| --- | --- |
| `200` | `ExternalVideo`. |
| `404 not_found` / `video_not_found` | The video does not exist, or has no location under a registered folder. |

`path` is an absolute path, compared byte for byte with the current location's `path` without
normalisation. Locations are stored in the file system's spelling (including NFD; see
`TestScanPreservesPathAndNormalizesTitleToNFC` in `internal/scanner`), so a `path` returned by the
list can be passed back as is.

## Tags

### `GET /api/v1/tags`

`200`: `{ items: [{ id, name, synonyms: string[], videoCount }] }`, in the same order as the
screen's `ListTags`.

## Video tags

### `POST /api/v1/video-tags`

```json
{ "videos": [{ "id": 1 }, { "contentKey": "…" }, { "path": "/media/a.mp4" }],
  "action": "add",
  "tags": ["名前", "シノニム"] }
```

Only manually attached tags are attached and detached; folder-derived tags remain (after `replace`
too). The rules are in
[research.md R-7](../research.md#r-7-tag-operations-are-added-to-tagstore-as-a-strict-batch-operation).

| Situation | Response |
| --- | --- |
| Success | `200`: `{ items: [{ video: { id, contentKey }, tags: ExternalVideoTag[] }] }` (in `videos` order, tags after the operation) |
| An element of `videos` is not exactly one of id, content key and path, or `action` is invalid | `400 invalid_request` |
| `videos` has 0 entries or more than 20000 | `400 invalid_request` / `too_many_videos`, `limit` |
| `tags` has 0 entries for `add` or `remove`, or more than 100 | `400 invalid_request` / `too_many_tags`, `limit` |
| A name is empty, contains control characters or is too long | `400 invalid_request` / `tag_name_empty`, `tag_name_control_characters`, `tag_name_too_long`; `index` is the position in `tags` |
| A video cannot be resolved | `404 not_found` / `video_not_found`, `index`. Nothing is applied. |

For name errors, `index` points into `tags`. For every other error, `index` points into `videos`.

## Scans

```yaml
ExternalScan:
  required: [id, status, startedAt, finishedAt, videos, settledVideos, errorCode]
  properties:
    id: { type: integer, format: int64 }
    status: { enum: [finding, running, done, partial, failed] }   # same as Scan.status in domain
    startedAt: { type: string, format: date-time }
    finishedAt: { type: [string, "null"], format: date-time }
    videos: { type: [integer, "null"] }            # null while finding (the count is not finished)
    settledVideos: { type: [integer, "null"] }     # likewise; once counted, 0 ≤ settledVideos ≤ videos
    errorCode: { type: [string, "null"] }
```

`videos` and `settledVideos` hold numbers only when `domain.ImportProgress.Counted` is true; when it is
false both are `null` (the same test the screen API uses to omit `videos`). This distinguishes
`finding` right after a scan starts from `done` with no videos, `{ videos: 0, settledVideos: 0 }`.

| Operation | Behaviour |
| --- | --- |
| `POST /api/v1/scans` | `201` when a new scan was started, `200` when the running one is returned. Which one is decided by `started` from `ScanStore.StartScan`, which decides in one transaction whether to create a scan row or return the running one; `app.Scans.StartScan` changes to return `(domain.Scan, started bool, error)` and passes it through. The screen's `POST /api/scans` ignores `started` and still returns `202`. The response is not decided by reading state through `GET /api/v1/scans/current` first (a scan may start or finish in between). With no media folders, it returns `409 media_folders_not_configured`, as the screen API does. |
| `GET /api/v1/scans/current` | `200 ExternalScan`. If no scan has ever run, `404 not_found` / `no_scan`. |
