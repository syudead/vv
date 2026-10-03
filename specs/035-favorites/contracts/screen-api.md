# Contract: Screen API changes

Parent Issue: #574. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This file lists only the
endpoints, fields and values added. `task generate` regenerates `internal/httpapi/gen/` and
`web/src/api/gen/`. The external API (`api/external-v1.yaml`) does not change (out of scope).

## 0. Fields added to `Video` and `LibraryGroup`

```yaml
Video:
  properties:
    favorite:
      type: boolean
      description: |
        Whether the owner made this video a favorite. Present only in owner responses
        (specs/035-favorites/data-model.md §3)

LibraryGroup:
  properties:
    favorite:
      type: boolean
      description: |
        Whether the owner made this group a favorite. Independent of the member videos' favorites,
        and present only in owner responses (specs/035-favorites/data-model.md §3)
```

| Field | Where it appears |
| --- | --- |
| `Video.favorite` | Every response that returns videos (lists, `GET /api/videos/{id}`, related, versions, `GET /api/folders/{rootId}/videos`, probe retry, display name and thumbnail position responses), for the owner only. Not `required`, because guest responses omit it. Unlike `public`, `forAudience` drops `favorite` |
| `LibraryGroup.favorite` | Group items of `GET /api/library` and `GET /api/folders/{rootId}/group`, for the owner only. Guest responses omit it, like `watchState` |

The time a favorite was made is not in any response (the server decides the order).

## 1. `PUT /api/favorites`

Owner only (the default class for `/api/*`). Sets the favorite of the videos in `videoIds` and the groups in
`folders` to `favorite`
([research.md R-2](../research.md#r-2-one-owner-only-put-apifavorites-for-videos-and-folders-in-one-transaction),
[data-model.md §4](../data-model.md#4-writes-favoritestore)).

```yaml
/api/favorites:
  put:
    operationId: updateFavorites
    requestBody: FavoritesRequest
    responses:
      "200": FavoritesResponse
      "400": InvalidRequest

FavoritesRequest:
  required: [favorite]
  properties:
    videoIds:  { type: array, items: { type: integer, format: int64 } }
    folders:   { type: array, items: { $ref: VideoFolder } }   # group folders (rootId and path)
    favorite:  { type: boolean }

FavoritesResponse:
  required: [appliedVideos, appliedFolders]
  properties:
    appliedVideos:  { type: integer }   # distinct video ids in videoIds that are in the library now (ids in the same bundle each count)
    appliedFolders: { type: integer }   # folders in folders that are groups now
```

| Condition | Response |
| --- | --- |
| The total of `videoIds` and `folders` is not between 1 and 20000 (duplicates count once) | `400 invalid_request` (`too_many_videos`, `limit` 20000; the same as `POST /api/video-tags`) |
| A `path` in `folders` fails `ValidateFolderPath` | `400 invalid_request` (`invalid_folder_path`) |
| An id in `videoIds` is not in the library, or the video has an empty `content_key` | Not an error; not counted in `appliedVideos` |
| A `rootId` in `folders` is not a registered folder, or the folder is not a group now | Not an error; not counted in `appliedFolders` |
| Already in the requested state | Not an error; counted. Favoriting a favorite keeps its time |

Each element of `folders` becomes an absolute path with `domain.FolderDir(root.Path, path)`, as in
`GET /api/folders/{rootId}/group`, and the store turns it into the folder key. The change runs in one
transaction: all of it applies or none of it does. No `/api/events` notification follows the commit
([research.md R-6](../research.md#r-6-no-domain-event-screens-reuse-the-visibility-subscription-pattern)).
A group member in `videoIds` gets its own favorite. A group in `folders` does not favorite its members
(requirement 4).

## 2. List `favorite` parameter and new `VideoSort` values

Add `favorite` (boolean, default false) to `GET /api/videos`, `GET /api/folders/{rootId}/videos`,
`GET /api/library` and `GET /api/library/ids`. True limits the list to favorites. Each item is judged by its
own favorite (a video item by the video, a group item by the group), combined with `query`, `tag`, `watch`
and `playable` by AND. A favorite video in a group that is not a favorite becomes a video item
([data-model.md §5](../data-model.md#5-reads-and-lists)). The `description` of `GET /api/library` and
`GET /api/library/ids` gains this rule.

```yaml
VideoSort:
  enum: [..., playedAsc, playedDesc, favoritedAsc, favoritedDesc, random]
  description: |
    ..., favorited = date favorited (non-favorite items go last in either direction; a group item
    uses the time the group was made a favorite), ...
```

`GET /api/videos`, `GET /api/folders/{rootId}/videos` and `GET /api/library` accept them in `sort`.
`GET /api/library/ids` has no `sort` and does not change.

Guests (added to the table in
[guest-api.md §3](../../016-single-account-auth/contracts/guest-api.md#3-conditions-guests-cannot-use)): the rules apply
to the three endpoints a guest can read, `GET /api/videos`, `GET /api/folders/{rootId}/videos` and
`GET /api/library`. `GET /api/library/ids` is owner-only and still returns `401` to a guest whatever the
conditions.

| Condition | For a guest |
| --- | --- |
| `favorite` is true | `400 invalid_request` (`guest_filter_not_allowed`) |
| `sort` is `favoritedAsc` or `favoritedDesc` | `400 invalid_request` (`guest_filter_not_allowed`) |

The `message` of `guest_filter_not_allowed` gains "favorites".

## 3. Fields added to `GET /api/library/ids`

```yaml
VideoIdsResponse:
  properties:
    groups:
      type: array
      description: |
        Group items that match the conditions: each one's folder and the ids of all its members
        (also included in ids). Present only in the listLibraryIds response, and omitted when there are none
      items:
        $ref: LibraryGroupIds

LibraryGroupIds:
  required: [folder, videoIds]
  properties:
    folder:   { $ref: VideoFolder }
    videoIds: { type: array, items: { type: integer, format: int64 } }
```

`ids` is still the union of the video items' ids and every member id of the group items, in no set order.
`groups` lets the screen know the chosen groups, which the selection bar's favorite action sends as `folders`
([research.md R-7](../research.md#r-7-selection-keeps-chosen-groups-as-groups)). Other endpoints that return
`VideoIdsResponse`, such as `POST /api/video-tags/summary`, do not include `groups`.

## 4. Unchanged endpoints

`PUT /api/video-visibility`, `POST /api/video-tags` and `POST /api/video-bundles` still take a set of video
ids. The parameters of `GET /api/folders/{rootId}/group` do not change (its response gains `favorite`). The
external API and MCP do not change.

## 5. `web/src/api` changes

| Change | Detail |
| --- | --- |
| `videoSorts` | Add `favoritedAsc` and `favoritedDesc`. The screen's list criteria accept only sorts whose kind is in `sortKinds`, through `isListSort` (as in 033 §3) |
| `ListFilterParams` | Add `favorite?: boolean`. `listVideos`, `listLibrary`, `listLibraryIds` and `listFolderVideos` send `favorite=true` only when it is true |
| `favorites.ts` | `updateFavorites(videoIds, folders, favorite)` (`PUT /api/favorites`) and the result subscriptions ([research.md R-6](../research.md#r-6-no-domain-event-screens-reuse-the-visibility-subscription-pattern)) |
| Generated `Video` and `LibraryGroup` types | `favorite` is optional, so the Vitest fixtures need no update |
