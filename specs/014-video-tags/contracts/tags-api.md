# Contract: Tag API

Parent Issue: #193.

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This document
covers only the routes, schemas and errors this feature adds. The error shape
(`Error {code, message}`), the same-origin check and JSON body reading stay as
they are (`internal/httpapi/router.go`, `readJSONBody` in `media_folders.go`).
Routes with a body are also added to `requiresJSONBody` and to the mapping in
`openapi_routes_test.go`. As with existing errors, `message` is Japanese that
the screen can show as is.

## Schemas

```yaml
TagRef:            # A tag attached to a video. The name is always the original name
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
    videoCount: { type: integer }  # videos currently in the library (data-model.md, [Video counts](../data-model.md#video-counts))

TagInput:          # The tag to use when adding
  type: object
  additionalProperties: false
  properties:
    id:   { type: integer, format: int64 }
    name: { type: string }
```

`TagInput` has exactly one of `id` and `name`. Both or neither is
`invalid_request` (400). It is not a `oneOf` because every existing schema is a
flat object with `additionalProperties: false`, and the repository has no
precedent for handling the Go and TypeScript types generated from `oneOf`.

`Video` gains a required `tags: TagRef[]`, sorted by the natural order of names
(`domain.CompareNatural`, then `id`). Every route that returns `Video` fills
it. Those are the five routes that currently call `progressFor`:

- `listVideos` (`GET /api/videos`)
- `getVideo` (`GET /api/videos/{id}`)
- `listFolderVideos` (`GET /api/folders/{rootId}/videos`)
- `getRelatedVideos` (`GET /api/videos/{id}/related`)
- `reprobeVideo` (`POST /api/videos/{id}/probe`)

A video with no tags returns an empty array, never `null`. The folder screen
receives the field but does not show it.

## Added error codes

| `code` | Status | Meaning |
| --- | --- | --- |
| `tag_not_found` | 404 | The specified tag no longer exists (deleted or merged in another tab) |
| `tag_name_taken` | 409 | The name is already another tag's name or synonym. `message` says which tag it is the name or synonym of |
| `tag_merge_required` | 409 | The name to add as a synonym is an existing tag's original name, and merging that tag was not accepted (including when the accepted tag differs from the tag that now has the name) |

A name that is empty, contains a control character, or exceeds 100 code points
is the existing `invalid_request` (400)
([data-model.md, Name rules](../data-model.md#name-rules)).

## Tag management

| Route | Body | Success | Errors |
| --- | --- | --- | --- |
| `GET /api/tags` | — | 200 `{ items: Tag[] }`, in the natural order of names, including tags with 0 videos | — |
| `POST /api/tags` | `{ name }` | 201 `Tag` | 400, 409 `tag_name_taken` |
| `PATCH /api/tags/{id}` | `{ name }` | 200 `Tag` (the same name as now returns it unchanged) | 400, 404 `tag_not_found`, 409 `tag_name_taken` |
| `DELETE /api/tags/{id}` | — | 204 | 404 `tag_not_found` |
| `POST /api/tags/{id}/merge` | `{ sourceId }` | 200 `Tag` (the target; the source's name and synonyms become the target's synonyms) | 400 (`sourceId` equals `id`), 404 `tag_not_found` (either is missing) |
| `POST /api/tags/{id}/synonyms` | `{ name, mergeTagId? }` | 200 `Tag` | 400, 404 `tag_not_found`, 409 `tag_name_taken`, 409 `tag_merge_required` |
| `DELETE /api/tags/{id}/synonyms?name=…` | — | 204 (unchanged 204 when the name is not a synonym of this tag) | 404 `tag_not_found` (the tag is missing) |

- Adding a name that is already a synonym of this tag returns 200 without
  changes. Adding this tag's original name is 409 `tag_name_taken`
  ([data-model.md, Write rules](../data-model.md#write-rules)).
- `mergeTagId` is the `id` of the tag whose merge the user accepted. When the
  name is the original name of another tag S and `mergeTagId` equals S's `id`,
  S is merged into this tag in the same transaction, and S's name becomes a
  synonym of this tag (acceptance criterion 17). When `mergeTagId` is missing or
  differs from S, the route returns `tag_merge_required` and changes nothing.
  When the name is not another tag's original name, `mergeTagId` is ignored.
- The screen confirms using the counts from `GET /api/tags` and sends the `id`
  of the tag it showed in the confirmation as `mergeTagId`. Acceptance is not a
  boolean because, if another tab renames or creates a tag after the
  confirmation and the name moves to a different tag, a boolean would merge a tag
  the user never confirmed. On `tag_merge_required` the screen refetches the tag
  list and confirms again.
- Removing a synonym does not put the name in the path, so the API needs no
  convention for carrying names containing `/` or `%` as one path segment.
- The delete and merge confirmations use `videoCount` from `GET /api/tags`. No
  route is added for the confirmation.

## Adding and removing tags on videos

| Route | Body | Success | Errors |
| --- | --- | --- | --- |
| `POST /api/video-tags` | `{ videoIds, action: "add", tag: TagInput }` | 200 `{ tag: TagRef, applied }` | 400, 404 `tag_not_found` (`tag.id` is missing) |
| `POST /api/video-tags` | `{ videoIds, action: "remove", tag: TagInput }` | 200 `{ tag: TagRef, applied }` | 400 (specified by `name`), 404 `tag_not_found` |
| `POST /api/video-tags/summary` | `{ videoIds }` | 200 `{ total, items: [{ tag: TagRef, count }] }` | 400 |

- A single video on the video page and several videos in the selection bar use
  the same `POST /api/video-tags`.
- `videoIds` holds 1 to 20000 entries; duplicates count once. 20000 covers
  selecting every video at the assumed scale (10,000 videos), and even with
  19-digit `id`s the body fits in `readJSONBody`'s 1 MiB (20000 × 20 bytes).
  Having no limit is rejected: the point at which the body limit returns 400
  would then depend on the number of digits in the `id`s, which cannot be
  explained to the user. The server passes the ids to `json_each` as one
  argument, which stays clear of SQLite's argument limit.
- Removal cannot be specified by `name`. The screen only offers tags that are
  attached, and it has their `id`s.
- Adding with `tag: { name }` looks the name up including synonyms and creates
  the tag when it is missing
  ([data-model.md, Name lookup](../data-model.md#name-lookup) and [Write rules](../data-model.md#write-rules)).
- `applied` is the number of `videoIds` that are currently in the library.
  Videos that already had the tag, or did not have it, are counted too (adding or
  removing again is not an error).
- In `summary`, `total` is the number of `videoIds` in the library and `count`
  is how many of those have the tag. A tag with `count < total` is "attached to
  some" (requirement 2). `items` holds only tags attached to at least one video,
  in the natural order of names.
- Processing is one transaction: it applies to all or to none.

## List filter and Select all

The list in #195's
[list-api.md](../../013-library-search/contracts/list-api.md) gains:

| Name | Type | Default | Meaning |
| --- | --- | --- | --- |
| `tag` | Array of integers (`tag=3&tag=8`, at most 16) | Empty | Only videos that have every listed tag (AND). Nonexistent `id`s are ignored |

`VideoPage` gains an optional `missingTagIds: integer[]`: the `id`s in `tag` that
did not exist, omitted when there are none. On receiving it, the screen reports
that the tag no longer exists, refetches the tag list and removes the `id` from
the URL ([list-url.md, Parameters](list-url.md#parameters), Edge Case
`ほかの画面での並行した変更`). A missing `id` is not a `404` because the same
Edge Case requires that a filter on a nonexistent tag left in the URL be ignored
and the list be shown with the other conditions.

- Added to `listVideos` (`GET /api/videos`). Not added to `listFolderVideos`
  (tag filtering on the folder screen is out of scope).
- `total` is the count with every condition applied, including `tag`.
- 17 or more is `400`. The screen never adds more than 16
  ([list-url.md, Adding and removing a tag](list-url.md#adding-and-removing-a-tag)).

The `query` of `listVideos` and `listFolderVideos` matches, in addition to the
title and relative path, the original names and synonyms of the tags attached
to a video ([data-model.md, Matching tag names in the search box](../data-model.md#matching-tag-names-in-the-search-box)).
Syntax, limits and the matching form follow #195's
[list-api.md, Query syntax](../../013-library-search/contracts/list-api.md#query-syntax)
unchanged, as do the parameters and the response shape. The location shown for
a list entry follows the rules of #195's
[list-api.md, Listed location and `Video.folder`](../../013-library-search/contracts/list-api.md#listed-location-and-videofolder).
For a video that satisfies the expression by tag names alone, the first
location is a path within the scope.

For Select all, this route is added:

| Route | Parameters | Success |
| --- | --- | --- |
| `GET /api/videos/ids` | `query`, `watch`, `playable`, `tag` of `listVideos` | 200 `{ ids: integer[], missingTagIds?: integer[] }` |

- The returned set of `id`s equals the set of `id`s across all pages of
  `listVideos` with the same conditions. The order is unspecified.
- `missingTagIds` means the same as in the list. When it is not empty, the
  screen does not build a selection from `ids`. As with the list, it reports the
  missing tag, refetches the tag list and removes the `id` from the URL. A
  selection is built when the user presses Select all again on the corrected
  list. This prevents a bulk add or remove on a wider set that lacks a condition
  when an active filter tag disappears after the list was opened.
- Go's `ServeMux` tells it apart from `/api/videos/{id}` by its rule that a
  literal segment takes precedence. `openapi_routes_test.go` gains a check that
  `{id}` does not capture this route.
