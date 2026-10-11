# Contract: Screen API for playlists

Parent Issue: #787. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml).
This file lists only the routes and schemas added; no existing route changes.
`task generate` regenerates `internal/httpapi/gen/` and `web/src/api/gen/`. The
external API (`api/external-v1.yaml`) and MCP do not change (out of scope).

Every route is owner only by the `/api/*` default, so `accessRoutes` gains
nothing and a guest gets `401`
([research.md R-8](../research.md#r-8-owner-only-through-the-existing-rules)).
The `POST` and `PATCH` routes join `requiresJSONBody`.

| Route | Purpose |
| --- | --- |
| `GET /api/playlists` | The playlists |
| `POST /api/playlists` | Create one |
| `GET /api/playlists/{id}` | One playlist and its shown items |
| `PATCH /api/playlists/{id}` | Rename |
| `DELETE /api/playlists/{id}` | Delete |
| `POST /api/playlists/{id}/items` | Add videos and groups |
| `POST /api/playlists/{id}/items/{itemId}/move` | Reorder one item |
| `DELETE /api/playlists/{id}/items/{itemId}` | Remove one item |
| `GET /api/playlists/{id}/playback` | Position and next steps for the video page |

## Schemas

```yaml
PlaylistSummary:
  required: [id, name, itemCount, updatedAt]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }
    itemCount: { type: integer }        # shown items (R-4)
    updatedAt: { type: string, format: date-time }

Playlist:
  allOf: [PlaylistSummary]
  required: [items]
  properties:
    items: { type: array, items: { $ref: PlaylistItem } }   # position order, shown items only

PlaylistItem:
  required: [id, kind]
  properties:
    id: { type: integer, format: int64 }
    kind: { type: string, enum: [video, group] }
    video: { $ref: Video }          # kind video
    group: { $ref: LibraryGroup }   # kind group; openVideoId is where the item starts

PlaylistName:
  required: [name]
  properties:
    name: { type: string, minLength: 1, maxLength: 100 }   # trimmed by the server (R-11)
```

## `GET /api/playlists`

`200 {items: PlaylistSummary[]}`, `updatedAt` descending. No paging: the list
serves the sidebar screen and the add menu.

## `POST /api/playlists`, `PATCH /api/playlists/{id}`, `DELETE /api/playlists/{id}`

| Route | Success | Failures |
| --- | --- | --- |
| `POST` with `PlaylistName` | `201 PlaylistSummary` | `400 invalid_request` for a name empty after trimming or over 100 characters |
| `PATCH` with `PlaylistName` | `200 PlaylistSummary` | `400` as above; `404 not_found` |
| `DELETE` | `204` | `404 not_found` |

## `GET /api/playlists/{id}`

`200 Playlist`; `404 not_found`. `video` and `group` carry the owner's fields
(`progress`, `watchState`, `watchedCount`, `favorite`), as library items do.
The same response serves the playlist screen and the video page's playlist
panel ([ui-design.md, Playlist panel](../ui-design.md#playlist-panel)); the
panel needs nothing the screen does not.

## `POST /api/playlists/{id}/items`

```yaml
PlaylistItemsAdd:
  properties:
    videoIds: { type: array, maxItems: 20000, items: { type: integer, format: int64 } }
    folders: { type: array, maxItems: 20000, items: { $ref: VideoFolder } }   # rootId and path, as PUT /api/favorites
PlaylistItemsAdded:
  required: [added, existing]
  properties:
    added: { type: integer }      # appended to the end
    existing: { type: integer }   # resolved but already in the playlist
```

| Condition | Response |
| --- | --- |
| Some ids or folders resolve | `200 PlaylistItemsAdded`; ids not in the library and folders not a group now count in neither number, and a repeated id or folder counts once ([R-5](../research.md#r-5-one-owner-only-bulk-add-for-videos-and-groups-appended-in-request-order)) |
| Both arrays empty, or a `folders.path` that breaks the `FolderPath` rules | `400 invalid_request` |
| Over 20000 distinct ids and folders together | `400 too_many_videos`, as `PUT /api/favorites` |
| The playlist does not exist | `404 not_found` |

## `POST /api/playlists/{id}/items/{itemId}/move`

Body `{afterItemId: integer | null}`; `null` moves the item to the top.
`204`; `404 not_found` when the playlist, the item or `afterItemId` is not in
it; `400` when `afterItemId` equals `itemId`
([R-3](../research.md#r-3-dense-integer-positions-reordered-by-move-after)).

## `DELETE /api/playlists/{id}/items/{itemId}`

`204`; `404 not_found` when either is gone. The video or group, its favorite
and its playback position do not change (acceptance criterion 4).

## `GET /api/playlists/{id}/playback`

Query: `item` (required, the item being played), `video` (required, the video
open on the page).

```yaml
PlaylistPlayback:
  required: [playlist, itemId, position, count]
  properties:
    playlist: { $ref: PlaylistSummary }
    itemId: { type: integer, format: int64 }
    position: { type: integer }   # 1-based among shown items; 0 when the current item is not shown (data-model.md, Playback order)
    count: { type: integer }
    next: { $ref: PlaylistStep }          # absent at the end of the playlist
    previousItem: { $ref: PlaylistStep }  # start of the previous shown item
    nextItem: { $ref: PlaylistStep }      # start of the next shown item
PlaylistStep:
  required: [itemId, video]
  properties:
    itemId: { type: integer, format: int64 }
    video: { $ref: Video }
```

| Condition | Response |
| --- | --- |
| Playlist and item exist | `200`, decided by [data-model.md, Playback order](../data-model.md#playback-order) |
| The playlist or the item is gone | `404 not_found`; the page drops the playlist context ([R-7](../research.md#r-7-a-playlist-playback-overrides-the-group-and-folder-neighbours)) |
| `item` or `video` missing or not an integer | `400 invalid_request` |

## Client use

| Screen | Calls |
| --- | --- |
| Playlists screen (`/playlists`) | `GET /api/playlists`, `POST /api/playlists` |
| Playlist screen (`/playlists/{id}`) | `GET`, `PATCH`, `DELETE /api/playlists/{id}`; `move` and item `DELETE`. Play opens `/videos/{videoId}?playlist={id}&item={itemId}` with `state: {from: "/playlists/{id}", autoplay: true}`, where `videoId` is the video or the group's `openVideoId` |
| Add menu (card, selection bar, video page) | `GET /api/playlists` when opened; `POST /api/playlists/{id}/items`; creating a playlist from the menu is `POST /api/playlists` then the add |
| Video page with `playlist` and `item`, owner only | `GET /api/playlists/{id}/playback` on open, when the video ends, and when the notice finds its next video gone; `GET /api/playlists/{id}` alongside each of those reads, for the aside's playlist panel ([ui-design.md, Playlist panel](../ui-design.md#playlist-panel)); the pair is used only when both carry the same `updatedAt`, and is read again when they differ ([ui-design.md, One order](../ui-design.md#one-order)), so the panel and `next` show the same order. A `404` from either read drops the context. Moving to a step keeps `playlist`, sets `item` to the step's `itemId`, and carries `state.from` over |
