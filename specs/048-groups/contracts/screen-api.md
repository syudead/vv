# Contract: Screen API for groups

Parent Issue: #1040. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml).
This file lists only the routes and schemas this feature adds or changes.
`task generate` regenerates `internal/httpapi/gen/` and `web/src/api/gen/`.
The external API (`api/external-v1.yaml`) and MCP expose no group and do not
change.

Reads (`GET`) are for the owner and guests (`security` includes `{}`), as
`GET /api/library`, so `accessRoutes` gains them; every write is owner only by
the `/api/*` default. The `POST` and `PATCH` routes join `requiresJSONBody`.

## Routes added

| Route | Purpose |
| --- | --- |
| `GET /api/groups` | Every group shown to the viewer, for the "Add to group" picker |
| `POST /api/groups` | Create a group from videos and groups |
| `GET /api/groups/{id}` | One group, its trail and its first page of items |
| `GET /api/groups/{id}/items` | A later page of items |
| `PATCH /api/groups/{id}` | Rename |
| `DELETE /api/groups/{id}` | Dissolve |
| `POST /api/groups/{id}/items` | Add videos and groups |
| `POST /api/groups/{id}/items/remove` | Remove items |
| `POST /api/groups/{id}/items/{itemId}/move` | Reorder one item |
| `POST /api/groups/{id}/tag` | Turn the group into a tag |

## Schemas added

```yaml
GroupMembers:                     # body of create and add
  properties:
    videoIds: { type: array, maxItems: 20000, items: { type: integer, format: int64 } }
    groupIds: { type: array, maxItems: 20000, items: { type: integer, format: int64 } }

GroupCreate:
  allOf: [GroupMembers]
  required: [name]
  properties:
    name: { type: string }        # 1 to 100 characters after trimming

GroupItem:
  required: [id, kind]
  properties:
    id: { type: integer, format: int64 }   # the item, not the video or group
    kind: { type: string, enum: [video, group] }
    video: { $ref: Video }                 # kind video
    group: { $ref: LibraryGroup }          # kind group

GroupRef:
  required: [id, name]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }

GroupDetail:
  required: [group, trail, items]
  properties:
    group: { $ref: LibraryGroup }
    trail: { type: array, items: { $ref: GroupRef } }   # the valid prefix of ?trail, outermost first
    items: { type: array, items: { $ref: GroupItem } }  # first page, position order, shown items only
    nextCursor: { type: string }

GroupItemPage:
  required: [items]
  properties:
    items: { type: array, items: { $ref: GroupItem } }
    nextCursor: { type: string }

GroupListPage:                    # GET /api/groups
  required: [groups]
  properties:
    groups: { type: array, items: { $ref: LibraryGroup } }   # natural name order, at every depth
    nextCursor: { type: string }

GroupItemMove:
  properties:
    after: { type: integer, format: int64 }   # item id; absent moves to the top

GroupItemRemove:
  required: [itemIds]
  properties:
    itemIds: { type: array, minItems: 1, maxItems: 20000, items: { type: integer, format: int64 } }
```

## Statuses

| Route | Status | When |
| --- | --- | --- |
| `GET /api/groups?q=&cursor=&limit=` | `200` | `GroupListPage`: the groups shown to the viewer, at every depth, whose name contains the trimmed `q` ignoring case (every group when `q` is empty); `limit` 1 to 200, default 100 |
| | `400 invalid_cursor` | Unreadable cursor |
| `POST /api/groups` | `201` | `LibraryGroup` of the new group |
| | `400 invalid_request` | Empty or over-long name, or no member |
| | `404 group_target_not_found` | A video or group in the body does not exist; nothing is created |
| `GET /api/groups/{id}` | `200` | `GroupDetail` |
| | `404 not_found` | Missing, or not shown to the viewer |
| `GET /api/groups/{id}/items?cursor=&limit=` | `200` | `GroupItemPage`; `limit` 1 to 200, default 100 |
| | `400 invalid_cursor` | Unreadable cursor |
| `PATCH /api/groups/{id}` | `200` | `LibraryGroup` with the trimmed name |
| `DELETE /api/groups/{id}` | `204` | Dissolved, with every ancestor the dissolve leaves with no item ([R-4](../research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup), [data-model.md, Edits](../data-model.md#edits)) |
| `POST /api/groups/{id}/items` | `200` | `LibraryGroup` after the add |
| | `404 group_target_not_found` | A video or group in the body does not exist |
| | `409 group_cycle` | A group in the body is this group or one of its ancestors |
| `POST /api/groups/{id}/items/remove` | `200` | `LibraryGroup` after the removal, when an item is left |
| | `204` | No item was left, so the group was dissolved as `DELETE` does ([R-4](../research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup), [data-model.md, Edits](../data-model.md#edits)); no body |
| | `404 group_target_not_found` | An item id is not in the group; nothing is removed |
| `POST /api/groups/{id}/items/{itemId}/move` | `204` | Moved |
| | `400 invalid_request` | `after` is the item itself |
| | `404 group_target_not_found` | The item or `after` is not in the group |
| `POST /api/groups/{id}/tag` | `200` | `{ tag: TagRef, created: boolean }` |
| | `400 invalid_request` | The name breaks the tag name rule; nothing changes |
| Every `{id}` route | `404 not_found` | The group does not exist |

`?trail=1,5` on `GET /api/groups/{id}` lists the groups above `{id}` the
screen came through. The answer keeps the pairs that are a parent and its
child item, up to the first that is not, and ends at `{id}`'s parent
([R-8](../research.md#r-8-a-group-page-at-groupsid-with-a-trail-in-the-url)).

## Routes and schemas changed

| Target | Change |
| --- | --- |
| `LibraryGroup` | Gains `id` (required); `folder` becomes optional and is present only for an automatic group, so the screen can tell the two apart |
| `LibraryGroupIds` (`GET /api/library/ids`) | `folder` becomes `id` |
| `VideoGroupRef` (`Video.group`) | `folder` becomes `id`; gains optional `folder` as `LibraryGroup` |
| `RelatedGroup` | `folder` becomes `id` |
| `GET /api/videos/{id}`, `GET /api/videos/{id}/related`, `GET /api/videos/{id}/group-members` | Gain the optional query `group` ([R-9](../research.md#r-9-the-video-page-follows-the-group-it-was-opened-from)); a `group` that does not hold the video falls back as R-9 says |
| `FavoritesRequest`, `FavoritesResponse` (`PUT /api/favorites`) | `folders` becomes `groupIds`; `appliedFolders` becomes `appliedGroups` |
| `PlaylistItemsAdd` (`POST /api/playlists/{id}/items`) | `folders` becomes `groupIds` |
| `FolderGrouping` | `grouped` and `taggable` become optional `groupId`, the folder's automatic group |
| `PUT /api/folders/{rootId}/grouping` | `ungroup` dissolves the folder's group; `auto` and `groupDirect` run the sync ([R-4](../research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup)) |
| `GET /api/folders/{rootId}/group`, `POST /api/folders/{rootId}/grouping/tag` | Removed; `GET /api/groups/{id}` and `POST /api/groups/{id}/tag` replace them |
| Error codes | Add `group_target_not_found`, `group_cycle`; remove `not_folder_group`, `root_group_not_taggable`, `folder_not_group` |

## `/api/events`

| Kind | When |
| --- | --- |
| `group` | After a group write or a sync that changed groups; carries their ids and their ancestors' ids ([R-13](../research.md#r-13-a-group-event-tells-open-screens-to-read-again)). Guests receive it too, with no name |
