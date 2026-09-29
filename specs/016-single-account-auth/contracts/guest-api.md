# Contract: guest responses and the visibility-flag API

Parent Issue: #135.

[api/openapi.yaml](../../../api/openapi.yaml) is the source of truth for the API. This document
covers only how responses differ when a "guests too" request
([auth-api.md §1](auth-api.md#1-three-access-classes)) runs as a guest, and the route that toggles the
visibility flag. List parameters and response shapes stay as in
[013 list-api.md](../../013-library-search/contracts/list-api.md) and
[014 tags-api.md](../../014-video-tags/contracts/tags-api.md); this document adds only the
differences.

## 1. What guests see

A guest response contains only public videos, as defined in
[data-model.md §3](../data-model.md#3-audience-and-the-public-video-condition). This covers list items and
`total`, `getVideo`, related videos, folder lists with their counts and previews, streaming,
thumbnails, seek previews, hover previews and live transcoding (Requirement 16).

Guest responses drop the following fields (Requirement 18).

| Schema | Guest handling |
| --- | --- |
| `Video.location` (absolute path on the server) | Omitted (stays optional) |
| `Video.progress` | Omitted |
| `Video.tags` | Empty array (stays required) |
| `Video.probeError` (the `ffprobe` error text contains the file's absolute path) | Omitted |
| `FolderSummary.rootPath` (absolute path of the registered folder) | Omitted, so it changes from required to optional |

- `Video.folder` (path relative to the registered folder) and `FolderSummary.name` stay,
  because folder browsing shows them.
- For guests, the UI builds the registered folder's display name from `name`, not from
  `rootPath`.
- Video and folder `id` and `rootId` values are the same sequential numbers the owner sees.
  A public video's `id` hints at the approximate size of the library; Requirement 16 of the
  parent Issue accepts this.

## 2. Videos hidden from guests

When `getVideo`, `getRelatedVideos`, `streamVideo`, `getVideoPreview`, `transcodeVideo`,
`getVideoThumbnail` or `getVideoSeekThumbnail` runs as a guest and points to a non-public
video, it returns the same response as for a missing video: the existing `404 not_found`
"Video not found." (Requirement 11). Body, headers and cache directives match the missing
case.

Folders behave the same way.

- For guests, `listRootFolders` omits registered folders that contain no location of a
  public video.
- `getFolder` and `listFolderVideos` return the same `404` as a missing folder when the
  folder contains no location of a public video. This includes the registered folder itself
  (a request without `path`). Today the registered folder itself always "exists"; the new
  check stops guests from enumerating registered-folder `rootId` values.

## 3. Filters guests cannot use

These list filters depend on owner data (playback progress and tags), so guest requests
reject them.

| Filter | Guest handling |
| --- | --- |
| `watch` other than `all` | `400 invalid_request` |
| `sort` of `playedAsc` or `playedDesc` | `400 invalid_request` |
| `tag` | `400 invalid_request` |
| `query` | Matches title and relative path only, not tag names or synonyms (drops the matching in [014 data-model.md §7](../../014-video-tags/data-model.md#7-tag-name-matching-in-the-search-box)) |

For guests, the UI hides these options. When one remains in the URL, the UI resets it to the
default before sending the request.

## 4. Toggling the visibility flag

`Video` gains a required `public: boolean`. Every route that returns `Video` fills it.

| Route | Body | Success | Error |
| --- | --- | --- | --- |
| `PUT /api/video-visibility` | `{ videoIds, public }` | 200 `{ applied }` | 400 |

- The route is "owner only". Both the detail screen (one video) and the selection bar
  (several videos) use it.
- The `videoIds` count limit, duplicate handling, the meaning of `applied` and the
  single-transaction apply match tag assignment
  ([014 tags-api.md §4](../../014-video-tags/contracts/tags-api.md#4-adding-and-removing-tags)).
- A video already in the requested state is not an error.
- Add the route to `requiresJSONBody` and to the mapping in `openapi_routes_test.go`.

## 5. Cache for generated media

Successful thumbnail, seek-preview and hover-preview responses drop the current
`public, max-age=31536000, immutable` (`cacheImmutable` in `internal/httpapi/router.go`).
Owner and guest responses both carry `private, no-cache` and an `ETag`.

- `private` keeps shared caches, such as a reverse proxy, from storing an owner response and
  serving it to someone else.
- `no-cache` makes the browser revalidate with the server on every use. After logout, or
  after a video becomes non-public, the browser cache stops serving non-public generated media
  (Acceptance criterion 12, Edge case "making a video non-public"). Unchanged content returns
  `304`, so bandwidth barely grows.
- The video file itself (`private, max-age=0, must-revalidate`) and live transcoding
  (`no-store`) already behave this way and stay unchanged.

## 6. Streaming after a video becomes non-public

When a video becomes non-public, the server cuts off streaming, live transcoding and preview
responses in progress for guests (Edge case "a public video is made non-public while being
watched"). Lists exclude the video from the next fetch. Responses in progress for the owner
continue.
