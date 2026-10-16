# Contract: Screen API for smart folders

Parent Issue: #1139. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml).
This file lists only what this feature adds or changes. `task generate`
regenerates `internal/httpapi/gen/` and `web/src/api/gen/`. The external API
(`api/external-v1.yaml`) and MCP do not change; the AI organizing Issue adds
smart folders there.

Reads (`GET`) are for the owner and guests (`security` includes `{}`), as
`GET /api/groups/{id}`, so `accessRoutes` gains them; every write is owner
only. `GET /api/smart-folders/cards` is declared before
`GET /api/smart-folders/{id}` so `cards` is not read as an id. The `POST` and `PATCH` routes join `requiresJSONBody`. Fields marked
owner only are left out for a guest.

## Routes added

| Route | Purpose |
| --- | --- |
| `GET /api/smart-folders` | Every folder shown to the viewer with its parent id, for the parent picker and the tree |
| `POST /api/smart-folders` | Create a folder |
| `GET /api/smart-folders/{id}` | One folder, its trail of ancestors and its first page of items |
| `GET /api/smart-folders/{id}/items` | A later page of items |
| `GET /api/smart-folders/cards` | The card values of the folders `ids` names (1 to 60, comma-separated), for the cards on screen ([data-model.md, Cards read](../data-model.md#cards-read)) |
| `PATCH /api/smart-folders/{id}` | Change the name, rule, sort or parent |
| `DELETE /api/smart-folders/{id}` | Delete the folder and its descendants |

## Schemas added

```yaml
SmartRule:                         # data-model.md, smart_folders.rule
  additionalProperties: false
  properties:
    q:            { type: string, maxLength: 500 }
    watch:        { type: string, enum: [unwatched, inProgress, watched] }
    playable:     { type: boolean }
    favorite:     { type: boolean }
    tags:         { type: array, minItems: 1, maxItems: 16, items: { type: integer, format: int64 } }
    tagKind:      { type: integer, format: int64 }
    person:       { type: integer, format: int64 }
    organization: { type: integer, format: int64 }
    addedWithin:  { type: integer, minimum: 1, maximum: 36500 }
    minDuration:  { type: integer, minimum: 0, maximum: 360000 }
    maxDuration:  { type: integer, minimum: 0, maximum: 360000 }

SmartRuleRef:                      # a reference that no longer exists
  required: [kind, id]
  properties:
    kind: { type: string, enum: [tag, tagKind, person, organization] }
    id:   { type: integer, format: int64 }

SmartFolderSummary:                # a folder item in the library and on a folder's page (R-14)
  required: [id, name]
  properties:
    id:       { type: integer, format: int64 }
    parentId: { type: integer, format: int64 }
    name:     { type: string }
    favorite: { type: boolean }    # owner only

LibrarySmartFolder:                # a folder card; the value fields of LibraryGroup, without folder and videoIds
  required: [id, name, videoCount, sizeBytes, previews, tags]
  properties:
    id:       { type: integer, format: int64 }
    parentId: { type: integer, format: int64 }
    name:     { type: string }
    # videoCount, sizeBytes, addedAt, durationMs, previews, openVideoId, tags,
    # watchedCount, watchState, lastPlayedAt, favorite: as LibraryGroup; over the play order in
    # SmartFolderDetail, over the matching videos in SmartFolderCards (data-model.md, Contents)
    # (openVideoId and addedAt absent for an empty folder)

SmartFolderCards:
  required: [cards]
  properties:
    cards: { type: array, items: { $ref: LibrarySmartFolder } }   # in the order of ids; absent ids left out

SmartFolderSettings:               # owner only
  required: [rule, sort, missing]
  properties:
    rule:    { $ref: SmartRule }
    sort:    { $ref: VideoSort }
    missing: { type: array, items: { $ref: SmartRuleRef } }   # on this folder and its ancestors

SmartFolderItem:
  required: [kind]
  properties:
    kind:        { type: string, enum: [video, group, smartFolder] }
    video:       { $ref: Video }
    group:       { $ref: LibraryGroup }
    smartFolder: { $ref: SmartFolderSummary }

SmartFolderDetail:
  required: [smartFolder, trail, items]
  properties:
    smartFolder: { $ref: LibrarySmartFolder }
    settings:    { $ref: SmartFolderSettings }   # owner only
    trail:       { type: array, items: { $ref: GroupRef } }   # ancestors, outermost first
    items:       { type: array, items: { $ref: SmartFolderItem } }
    nextCursor:  { type: string }

SmartFolderCreate:
  required: [name, rule]
  properties:
    name:     { type: string }     # 1 to 100 characters after trimming
    rule:     { $ref: SmartRule }
    sort:     { $ref: VideoSort }  # default addedDesc
    parentId: { type: integer, format: int64 }

SmartFolderChange:                 # at least one member
  properties:
    name:     { type: string }
    rule:     { $ref: SmartRule }
    sort:     { $ref: VideoSort }
    parentId: { type: [integer, "null"], format: int64 }   # null makes it a root
```

## Schemas and parameters changed

| Schema or route | Change |
| --- | --- |
| `LibraryItem` | `kind` gains `smartFolder` with `smartFolder: SmartFolderSummary` ([data-model.md, Library items](../data-model.md#library-items)) |
| `listVideos`, `listLibrary`, `listLibraryIds` | Gain `addedWithin`, `minDuration` and `maxDuration` with the ranges of `SmartRule`; guests may send them. A value out of range, or `minDuration` over `maxDuration`, is `400 invalid_request` |
| `FavoritesRequest`, `FavoritesResponse` | Gain `smartFolderIds` |
| `PlaylistItemsAdd`, `PlaylistItem` | Gain `smartFolderIds` and the `smartFolder` kind, played in the folder's play order at play time |
| `PlaylistStep` | Gains `smartFolderId` (`int64`, optional): the folder of the step's item when that item is a smart folder item, absent for a video or a group item. `next`, `previousItem` and `nextItem` of `PlaylistPlayback` carry it, so the video page opens a step with `?smartFolder` from the playback response alone, without the playlist panel's response ([ui-design.md, Video page in a playlist](../ui-design.md#video-page-in-a-playlist)). Added by the 2026-10-16 revision of the design; "Play a video in the smart folder it was opened from" (#1181) implements it |
| `GET /api/videos/{id}`, `/related`, `/group-members` | Gain a `smartFolder` query: the folder's play order is the context for `Up next`, the neighbours and the member window, as `group` is (048 R-9). `group` wins when both are sent; a folder that does not hold the video is ignored |
| `Video` | Gains `smartFolder: { id, name, position, count }` when the context is a folder |
| `/api/events` | Gains the kind `smartFolder` with `{ ids }` ([R-12](../research.md#r-12-writes-that-can-change-a-match-publish-smartfolderschanged)) |

## Statuses

| Route | Status | `code` | When |
| --- | --- | --- | --- |
| `POST`, `PATCH /api/smart-folders…` | 400 | `invalid_smart_rule` | `rule` fails `domain.SmartRule`, or `sort` is `random` |
| `POST`, `PATCH /api/smart-folders…` | 400 | `invalid_request` | The name is empty or over 100 characters after trimming, as for a group |
| `POST`, `PATCH /api/smart-folders…` | 404 | `smart_folder_not_found` | `parentId` names no folder |
| `PATCH /api/smart-folders/{id}` | 409 | `smart_folder_cycle` | `parentId` is the folder or one of its descendants |
| `GET /api/smart-folders/cards` | 400 | `invalid_request` | `ids` is missing, empty, not integers, or names over 60 ids |
| Every `/api/smart-folders/{id}` route | 404 | `not_found` | No such folder, or a guest opens a folder with nothing visible |

A `rule` naming an id that does not exist is accepted and reported in
`missing`, so a filter whose tag was just deleted can still be saved.
