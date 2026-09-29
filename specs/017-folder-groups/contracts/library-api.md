# Contract: Library list (video and group items)

`api/openapi.yaml` is the source of truth. This document lists only the added routes and schemas.
Error shapes, parameter validation and cursor opacity follow
[013 list-api.md](../../013-library-search/contracts/list-api.md) and
[014 tags-api.md §5](../../014-video-tags/contracts/tags-api.md). The query itself is in
[data-model.md §5](../data-model.md#5-library-items).

Access per route (`security`) matches `GET /api/videos`:

| Route | Access |
| --- | --- |
| `GET /api/library`, the single group route | Owner and guest (`security` includes `{}`) |
| `GET /api/library/ids` | Owner only (same as the current `listVideoIds`) |

Differences in the guest response follow [data-model.md §7](../data-model.md#7-visibility-per-viewer)
and [016 guest-api.md](../../016-single-account-auth/contracts/guest-api.md) (`watch`, the played
sort and `tag` return 400).

`GET /api/videos` stays a per-video list (the root search results of the folder page use it).
`GET /api/videos/ids` is removed in the unit that switches the library to `GET /api/library/ids`.

## 1. `GET /api/library`

- Parameters: the same as `GET /api/videos` (`query`, `watch`, `playable`, `sort`, `seed`, `cursor`,
  `limit`, `tag`).
- Response `LibraryPage`:

```yaml
LibraryPage:
  required: [items, total]
  properties:
    items: { type: array, items: { $ref: LibraryItem } }
    total: { type: integer }        # number of items (cards) after filtering
    nextCursor: { type: string }
    missingTagIds: { type: array, items: { type: integer, format: int64 } }

LibraryItem:
  required: [kind]
  properties:
    kind: { type: string, enum: [video, group] }
    video: { $ref: Video }          # only when kind = video
    group: { $ref: LibraryGroup }   # only when kind = group

LibraryGroup:
  required: [folder, name, videoCount, sizeBytes, addedAt, previews, openVideoId, videoIds, tags]
  # watchedCount and watchState are always present for the owner and omitted for a guest (data-model.md §7)
  properties:
    folder: { $ref: VideoFolder }   # the group's folder; this is the key that identifies the group
    name: { type: string }
    videoCount: { type: integer }
    watchedCount: { type: integer }
    watchState: { type: string, enum: [unwatched, inProgress, watched] }
    durationMs: { type: integer, format: int64 }   # omitted when no member's length is known
    sizeBytes: { type: integer, format: int64 }
    addedAt: { type: string, format: date-time }
    lastPlayedAt: { type: string, format: date-time }  # omitted when absent
    previews: { type: array, maxItems: 4, items: { $ref: FolderPreview } }  # up to 4 members with a generated thumbnail, in order. The card uses them for the folder artwork; the list row uses the first as its thumbnail
    openVideoId: { type: integer, format: int64 }  # the member opened on press (data-model.md §6)
    videoIds: { type: array, items: { type: integer, format: int64 } }  # all members, in order
    tags: { type: array, items: { $ref: VideoTag } }  # union of the members' tags (sources are unioned too)
```

The response does not expose a group `id`; a group is addressed by `folder`. Rejected: exposing
`id`, because the UI would then key selection and refetches on a value that every rebuild
reassigns.

## 2. `GET /api/library/ids`

- Parameters: those of `GET /api/library` without `sort`, `seed`, `cursor` and `limit`.
- Response: the existing `VideoIdsResponse` (`ids`, `missingTagIds`). `ids` holds the `id` of each
  matching video item and the `id` of **every member** of each matching group. Limits and errors are
  the same as the current `GET /api/videos/ids`.

## 3. Single group

`GET /api/folders/{rootId}/group?path=…`

- The route refetches the card of a group that is still in the list (Structural Decision 10 in the
  plan).
- 200: `LibraryGroup`, built from all members of the group regardless of filters.
- 400: the path breaks the rules (same as `FolderPath`).
- 404 `not_found`: the folder is not a group now, or does not exist. The UI removes that card from
  the list.
