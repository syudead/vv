# Contract: folder browsing API

The source of truth is [api/openapi.yaml](../../../api/openapi.yaml); the implementation unit
"folder browsing API" moves this difference into it. This document records only the routes, types
and error meanings this feature adds. The existing `Video`, `VideoPage`, `VideoSort` and `Error`
definitions do not change.

## Identifier: registered folder id and relative path

A folder is addressed by the pair of **the registered media folder id (`rootId`) and the path
relative to it (`path`)**. Responses carry the absolute path, but requests never accept one.

- `path` is a `/`-separated relative path. An empty string means the registered folder itself.
- A `path` with an empty segment (`a//b`), a leading or trailing `/`, a `.` or `..` segment, or
  NUL is 400 `invalid_request`.
- On an OS whose separator is `\` (Windows), a `\` inside a segment is also 400. A segment such as
  `safe\..\outside` passes the per-`/` check, but when converted to an OS path it is read as
  `..` and points outside the registered folder. On an OS whose separator is `/`, `\` is part of
  a file name and is accepted.
- The server converts `path` to OS separators (`filepath.FromSlash`) and joins it to the
  registered folder path. The validation above ensures the result never leaves the registered
  folder.

## Definition of "the folder exists"

`(rootId, path)` exists when either condition holds (parent Issue Requirement 11):

- `path` is empty and `rootId` is registered (it exists even with no videos).
- `path` is not empty, and at least 1 video location (`video_locations`) under the registered
  folder lies below it (at any depth).

The server does not read the disk. It derives existence from the scanned index alone.

## Types

### FolderSummary

One folder card. Shared by the top-level list and the subfolder list of a folder.

| Field | Type | Meaning |
| --- | --- | --- |
| `rootId` | int64 | Registered folder id |
| `path` | string | Path relative to the registered folder. Empty string for the registered folder itself |
| `name` | string | Display name. The last segment of `path`; when `path` is empty, the last segment of the registered folder's absolute path (the absolute path itself for a registered folder with no last segment, such as `/`) |
| `rootPath` | string | Absolute path of the registered folder (tells registered folders with the same name apart; parent Issue Requirement 2) |
| `videoCount` | int | Count of videos **directly** in the folder (grandchildren and deeper are excluded; Requirement 5) |
| `folderCount` | int | Count of **direct** subfolders |
| `previews` | FolderPreview[] | Up to 4 of the direct videos that have a generated thumbnail (Requirement 4) |

### FolderPreview

| Field | Type | Meaning |
| --- | --- | --- |
| `videoId` | int64 | Video id |
| `thumbnailUrl` | string | Versioned URL, the same as the existing `Video.thumbnailUrl` |

`previews` picks up to 4 of the direct videos with `thumbnailState = done`, in ascending order of
the location path. With none, it is an empty array (Edge case "thumbnail not generated").

### FolderListing

| Field | Type | Meaning |
| --- | --- | --- |
| `folder` | FolderSummary | The open folder itself. The breadcrumb names also come from it |
| `folders` | FolderSummary[] | Direct subfolders, in natural order of the name (below) |

### RootFolderListing

| Field | Type | Meaning |
| --- | --- | --- |
| `folders` | FolderSummary[] | Every registered media folder, including those with no videos |

## Sort order

- Subfolders and top-level folders are sorted by `name` in **natural order** (Requirement 7).
  - Digit runs compare as numbers (`2` < `10`).
  - Everything else compares case-insensitively.
  - Ties are broken by the original string, and finally by `rootPath`.
  - The server decides the order. The client does not re-sort.
- Videos follow the existing `VideoSort` (`addedDesc`, `titleAsc`). The title for `titleAsc` is
  the title of the location in that folder.
  - Addendum (013-library-search): `VideoSort` grew to 13 values, and `titleAsc` became natural
    order. For the values and their meaning,
    [013's contracts/list-api.md §3](../../013-library-search/contracts/list-api.md#3-videosort-values)
    is authoritative.

## Routes

### `GET /api/folders`

Top level. Returns the `RootFolderListing` of the registered media folders.

- 200: `RootFolderListing`. With 0 registrations, `folders` is an empty array.

### `GET /api/folders/{rootId}`

- query `path` (string, empty string when omitted)
- 200: `FolderListing`
- 400 `invalid_request`: `path` violates the rules above
- 404 `not_found`: `rootId` is not registered, or `(rootId, path)` does not exist (Edge case "the
  open folder disappears"; the client shows "not found" instead of an empty grid)

### `GET /api/folders/{rootId}/videos`

Returns only the direct videos, with the same cursor method as the existing list (Requirements 3,
6 and 12).

- query `path` (as above), `sort`, `cursor`, `limit` (same meaning and range as the existing
  `listVideos`)
- 200: the existing `VideoPage`. `total` is the count of direct videos (= `videoCount`)
- 400 `invalid_request`: invalid `path`, invalid sort order, or a cursor that cannot be parsed
  (for the validation of the condition values added in 013, see the addendum below)
- 404 `not_found`: the same conditions as `GET /api/folders/{rootId}`

> Addendum (013-library-search): this route now accepts `scope` (`direct` or `subtree`, default
> `direct`), `query`, `watch`, `playable` and `seed`. With `scope=subtree`, everything below the
> folder is covered. `total` is the count of all items after the search terms, the filters and
> the scope are applied (for `direct` without filters, it still equals `videoCount`). For the sort
> values, the subtree scope, the filters, `total` and the item's `folder`,
> [013's contracts/list-api.md](../../013-library-search/contracts/list-api.md) is authoritative.

The `Video` in `items` keeps its existing shape. It differs from the list in only 2 points:

- `title` and `sizeBytes` come from the location in that folder (Edge case "the same content in
  several places"). `id`, the thumbnail and the playback position belong to the video, so they
  are the same from any folder.
- When the same video has 2 or more locations in the same folder, only 1 `Video` is returned,
  and its title comes from the location that is first in ascending path order.

## Unchanged

- The meaning and responses of `GET /api/videos` and `GET /api/videos/{id}`.
- The playback, thumbnail and playback position routes. A video card on the folder page navigates
  to the existing `/videos/{id}`.
- No new `Error.code` is added.
