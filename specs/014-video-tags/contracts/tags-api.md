# Contract: tags API

Parent Issue: #193.

The source of truth for the API is [api/openapi.yaml](../../../api/openapi.yaml). This document
covers only the added routes, schemas and errors. The error shape (`Error {code, message}`), the
same-origin check and the JSON body reader stay as they are (`internal/httpapi/router.go`, and
`readJSONBody` in `media_folders.go`). Every route with a body is also added to the
`requiresJSONBody` mapping and to `openapi_routes_test.go`. As for existing errors, `message` is
Japanese text that the screen can show as is.

## 1. Schemas

```yaml
TagRef:            # A tag on a video. The name is always the original name
  type: object
  required: [id, name]
  properties:
    id:   { type: integer, format: int64 }
    name: { type: string }

Tag:               # One entry on the management page and in suggestions
  type: object
  required: [id, name, synonyms, videoCount]
  properties:
    id:         { type: integer, format: int64 }
    name:       { type: string }
    synonyms:   { type: array, items: { type: string } }   # natural order of names
    videoCount: { type: integer }  # number of videos now in the library (data-model.md §5)

TagInput:          # How an add/remove request names a tag
  type: object
  additionalProperties: false
  properties:
    id:   { type: integer, format: int64 }
    name: { type: string }
```

`TagInput` has exactly one of `id` and `name`. Both, or neither, returns `invalid_request` (400).
It is not a `oneOf`: every existing schema is a flat object with `additionalProperties: false`,
and the repository has no precedent for handling the Go and TypeScript types generated from
`oneOf`.

`Video` gets a required `tags: TagRef[]`. The order is the natural order of names
(`domain.CompareNatural`, then `id` on a tie). Every route that returns `Video` fills this field.
Today these are the five routes that call `progressFor`:

- `listVideos` (`GET /api/videos`)
- `getVideo` (`GET /api/videos/{id}`)
- `listFolderVideos` (`GET /api/folders/{rootId}/videos`)
- `getRelatedVideos` (`GET /api/videos/{id}/related`)
- `reprobeVideo` (`POST /api/videos/{id}/probe`)

A video with no tags returns an empty array, never `null`. The folder page receives this field
but does not show it.

## 2. Added error `code` values

| code | Status | Meaning |
| --- | --- | --- |
| `tag_not_found` | 404 | The tag no longer exists (deleted or merged in another tab) |
| `tag_name_taken` | 409 | The name is already another tag's name or synonym. `message` says which tag, and whether it is that tag's name or a synonym |
| `tag_merge_required` | 409 | The name to add as a synonym is an existing tag's original name, and merging that tag was not approved (including when the approved tag differs from the tag that now has the name) |

An empty name, a name with control characters, or a name over 100 code points returns the
existing `invalid_request` (400) ([data-model.md §2](../data-model.md#2-name-rules)).

## 3. Tag management

| Route | Body | Success | Errors |
| --- | --- | --- | --- |
| `GET /api/tags` | — | 200 `{ items: Tag[] }`. Natural order of names, including tags with 0 videos | — |
| `POST /api/tags` | `{ name }` | 201 `Tag` | 400, 409 `tag_name_taken` |
| `PATCH /api/tags/{id}` | `{ name }` | 200 `Tag` (the current name returns the tag unchanged) | 400, 404 `tag_not_found`, 409 `tag_name_taken` |
| `DELETE /api/tags/{id}` | — | 204 | 404 `tag_not_found` |
| `POST /api/tags/{id}/merge` | `{ sourceId }` | 200 `Tag` (the target. The source's name and synonyms become synonyms of the target) | 400 (`sourceId` equals `id`), 404 `tag_not_found` (either one is missing) |
| `POST /api/tags/{id}/synonyms` | `{ name, mergeTagId? }` | 200 `Tag` | 400, 404 `tag_not_found`, 409 `tag_name_taken`, 409 `tag_merge_required` |
| `DELETE /api/tags/{id}/synonyms?name=…` | — | 204 (if the name is not a synonym of this tag, 204 with no change) | 404 `tag_not_found` (the tag is missing) |

- Adding a name that is already a synonym of this tag returns 200 with no change. Adding this
  tag's own original name returns 409 `tag_name_taken`
  ([data-model.md §4](../data-model.md#4-write-rules)).
- `mergeTagId` is the `id` of the tag whose merge the user approved. When the name is the
  original name of another tag S and `mergeTagId` equals S's `id`, S merges into this tag in the
  same transaction. The merge makes S's name a synonym of this tag (Acceptance criterion 17).
- When `mergeTagId` is missing or differs from S, the route returns `tag_merge_required` and
  changes nothing. When the name is not another tag's original name, `mergeTagId` is ignored.
- The screen confirms with the video count from `GET /api/tags`, then sends the `id` of the tag
  it showed in the confirmation as `mergeTagId`. Approval is not a boolean: a rename or create in
  another tab after the confirmation can move the name to another tag, and a boolean would merge
  a tag the user never confirmed. On `tag_merge_required`, the screen refetches the tag list and
  confirms again.
- Removing a synonym does not put the name in the path. This avoids another convention for
  treating a name with `/` or `%` as one path segment.
- The delete and merge confirmations use `videoCount` from `GET /api/tags`. No route is added for
  the confirmation.

## 4. Adding and removing tags

| Route | Body | Success | Errors |
| --- | --- | --- | --- |
| `POST /api/video-tags` | `{ videoIds, action: "add", tag: TagInput }` | 200 `{ tag: TagRef, applied }` | 400, 404 `tag_not_found` (`tag.id` is missing) |
| `POST /api/video-tags` | `{ videoIds, action: "remove", tag: TagInput }` | 200 `{ tag: TagRef, applied }` | 400 (tag given by `name`), 404 `tag_not_found` |
| `POST /api/video-tags/summary` | `{ videoIds }` | 200 `{ total, items: [{ tag: TagRef, count }] }` | 400 |

- A single video on the playback page and several videos in the selection bar use the same
  `POST /api/video-tags`.
- `videoIds` has 1 to 20000 entries. Duplicates count once. 20000 covers selecting every video at
  the assumed scale (10,000 videos). Even with 19-digit `id` values, the body fits in the 1 MiB
  limit of `readJSONBody` (20000 × 20 bytes). The server passes the list to `json_each` as one
  argument, so SQLite's parameter limit does not apply.
- Rejected: no upper limit on `videoIds`. The point where the body limit returns 400 would then
  depend on the digit count of the `id` values, which users cannot be told.
- Removal cannot name the tag by `name`. The screen only removes tags that are attached, and it
  has their `id`.
- Adding with `tag: { name }` looks the name up including synonyms and creates the tag if none
  matches ([data-model.md §3 and §4](../data-model.md#3-name-lookup)).
- `applied` is the number of `videoIds` that are videos now in the library. Videos that already
  had the tag, or did not have it, also count (a duplicate add or remove is not an error).
- In `summary`, `total` is the number of `videoIds` that are in the library, and `count` is how
  many of them have the tag. A tag with `count < total` is "on only some" (Requirement 2). `items`
  lists only tags on at least one video, in natural order of names.
- The operation is one transaction: it applies to all videos or to none.

## 5. List filter and "Select all"

The list in #195's [list-api.md](../../013-library-search/contracts/list-api.md) gets the
following parameter.

| Name | Type | Default | Meaning |
| --- | --- | --- | --- |
| `tag` | Array of integer (`tag=3&tag=8`, at most 16) | Empty | Keep only videos that have every tag (AND). An `id` that does not exist is ignored |

`VideoPage` gets an optional `missingTagIds: integer[]`: the `tag` values that did not exist,
omitted when there are none. On it, the screen reports that the tag no longer exists, refetches
the tag list and removes the `id` from the URL
([list-url.md §1](list-url.md#1-parameters), Edge case "Concurrent changes on other screens"). A
missing `id` is not a `404`, because the same Edge case requires that a filter on a missing tag
left in the URL is ignored and the list is shown with the other conditions only.

- The parameter is added to `listVideos` (`GET /api/videos`), not to `listFolderVideos` (tag
  filtering on the folder page is out of scope).
- `total` is the count with every condition applied, `tag` included.
- 17 or more values return `400`. The screen never adds more than 16
  ([list-url.md §2](list-url.md#2-pressing-and-removing-a-tag)).

The `query` of `listVideos` and `listFolderVideos` matches the title and the relative path, and
also the original names and synonyms of the tags on the video
([data-model.md §7](../data-model.md#7-tag-name-matching-in-the-search-box)). The syntax, limits and match
form stay as in #195's
[list-api.md §1](../../013-library-search/contracts/list-api.md#1-search-syntax). The parameters
and the response shape do not change. The location shown for a list item still follows #195's
[list-api.md §4](../../013-library-search/contracts/list-api.md#4-the-listed-location-and-videofolder).
For a video that matches the expression by tag names alone, the first location is a path inside
the scope.

"Select all" gets this route:

| Route | Parameters | Success |
| --- | --- | --- |
| `GET /api/videos/ids` | `query`, `watch`, `playable`, `tag` of `listVideos` | 200 `{ ids: integer[], missingTagIds?: integer[] }` |

- The returned `id` set equals the set of `id` values across all pages of `listVideos` with the
  same conditions. The order is unspecified.
- `missingTagIds` means the same as in the list. When it is not empty, the screen does not build
  a selection from `ids`. As with the list, it reports that the tag is gone, refetches the tag
  list and removes the `id` from the URL.
- The selection is built when the user presses "Select all" again on the corrected list. This
  prevents a bulk add or remove on a wider set that lacks a condition, when an active tag was
  deleted after the list opened.
- Go's `ServeMux` tells this route apart from `/api/videos/{id}` by its rule that a literal
  segment wins. `openapi_routes_test.go` gets a check that `{id}` does not capture this route.
