# Contract: Folder browsing API

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml); the
implementation unit "Folder browsing API" moves this change into it. This document
covers only the endpoints, types and error meanings this feature adds. The
existing definitions of `Video`, `VideoPage`, `VideoSort` and `Error` do not
change.

## Identifier: registered folder id and relative path

A folder is identified by the pair of **the registered media folder's id
(`rootId`) and the relative path from it (`path`)**. The absolute path appears in
responses but is never accepted in requests.

- `path` is a `/`-separated relative path; the empty string means the registered
  folder itself.
- A `path` with an empty segment (`a//b`), a leading or trailing `/`, a `.` or
  `..` segment, or NUL is 400 `invalid_request`.
- On an OS whose separator is `\` (Windows), a `\` inside a segment is also 400.
  A segment such as `safe\..\outside` passes the per-`/` check, but once turned
  into an OS path it is read as `..` and points outside the registered folder. On
  an OS whose separator is `/`, `\` is part of a file name and is accepted.
- The server converts `path` to the OS separator (`filepath.FromSlash`) and joins
  it to the registered folder's path. The validation above means the result never
  lands outside the registered folder.

## Definition of "the folder exists"

`(rootId, path)` exists when either holds (parent Issue requirement 11):

- `path` is empty and `rootId` is registered (it exists even without videos).
- `path` is not empty and at least one video location (`video_locations`) under
  the registered folder lies below it (at any depth).

The disk is not consulted; existence is derived from the scanned index only.

## Types

### FolderSummary

One folder card. Used both for the top-level list and for a folder's child
folders.

| Field | Type | Meaning |
| --- | --- | --- |
| `rootId` | int64 | The registered folder's id |
| `path` | string | Relative path from the registered folder. Empty for the registered folder itself |
| `name` | string | Display name. The last segment of `path`; when `path` is empty, the last segment of the registered folder's absolute path (for a registered folder with no last segment, such as `/`, the absolute path itself) |
| `rootPath` | string | The registered folder's absolute path (tells registered folders with the same name apart; parent Issue requirement 2) |
| `videoCount` | int | The number of videos **directly** in the folder (grandchildren and below excluded; requirement 5) |
| `folderCount` | int | The number of **direct** child folders |
| `previews` | FolderPreview[] | Up to 4 of the direct videos that have a generated thumbnail (requirement 4) |

### FolderPreview

| Field | Type | Meaning |
| --- | --- | --- |
| `videoId` | int64 | The video's id |
| `thumbnailUrl` | string | The same versioned URL as the existing `Video.thumbnailUrl` |

`previews` picks up to 4 of the direct videos with `thumbnailState = done`, in
ascending order of the location's path. With none, it is an empty array (Edge Case
"thumbnail not generated").

### FolderListing

| Field | Type | Meaning |
| --- | --- | --- |
| `folder` | FolderSummary | The open folder itself. The breadcrumb names come from here too |
| `folders` | FolderSummary[] | Direct child folders, in natural order of the name (below) |

### RootFolderListing

| Field | Type | Meaning |
| --- | --- | --- |
| `folders` | FolderSummary[] | Every registered media folder, including those without videos |

## Order

- Child folders and top-level folders are sorted by **natural order** of `name`.
  Digit runs compare as numbers (`2` < `10`), everything else compares
  case-insensitively, ties fall back to the original string, and finally to
  `rootPath` (requirement 7). The server decides the order; the client does not
  re-sort.
- Videos follow the existing `VideoSort` (`addedDesc`, `titleAsc`). The title for
  `titleAsc` is the title of the location in that folder.
  - Addendum (013-library-search): `VideoSort` grew to 13 values, and `titleAsc`
    became natural order. The values and their meaning are owned by
    [013's contracts/list-api.md §3](../../013-library-search/contracts/list-api.md#3-videosort-values).

## Endpoints

### `GET /api/folders`

The top level. Returns the `RootFolderListing` of the registered media folders.

- 200: `RootFolderListing`. With zero registrations, `folders` is an empty array.

### `GET /api/folders/{rootId}`

- query `path` (string; the empty string when omitted)
- 200: `FolderListing`
- 400 `invalid_request`: `path` breaks the rules above
- 404 `not_found`: `rootId` is not registered, or `(rootId, path)` does not exist
  (Edge Case "the open folder disappears"; the client shows `見つかりません` instead
  of an empty grid)

### `GET /api/folders/{rootId}/videos`

Returns only the direct videos, with the same cursor scheme as the existing list
(requirements 3, 6 and 12).

- query `path` (as above), `sort`, `cursor` and `limit` (same meaning and range as
  the existing `listVideos`)
- 200: the existing `VideoPage`. `total` is the number of direct videos
  (= `videoCount`)
- 400 `invalid_request`: an invalid `path` or sort, or a cursor that cannot be
  interpreted (for the checks on the condition values added in 013, see the
  addendum below)
- 404 `not_found`: the same conditions as `GET /api/folders/{rootId}`

> Addendum (013-library-search): this endpoint now accepts `scope` (`direct` |
> `subtree`, default `direct`), `query`, `watch`, `playable` and `seed`. With
> `scope=subtree` everything below the folder is covered, and `total` is the count
> of all items after the query, the filters and the scope are applied (for
> `direct` without filters it still equals `videoCount`). The sort values, the
> subtree scope, the filters, `total` and the item's `folder` are owned by
> [013's contracts/list-api.md](../../013-library-search/contracts/list-api.md).

The `Video` in `items` keeps its existing shape, and differs from the list in only
two points:

- `title` and `sizeBytes` come from the location in that folder (Edge Case "the
  same content in several places"). `id`, the thumbnail and the playback position
  belong to the video, so they are the same from any folder.
- When the same video has two or more locations in the same folder, only one
  `Video` is returned, with the title from the location that comes first in
  ascending path order.

## What does not change

- The meaning and responses of `GET /api/videos` and `GET /api/videos/{id}`.
- The playback, thumbnail and playback-position endpoints. Video cards on the
  folder screen navigate to the existing `/videos/{id}`.
- No new `Error.code` is added.
