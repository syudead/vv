# Contract: Library list (video and group entries)

Source of truth: `api/openapi.yaml`. This document covers only the routes and
schemas it adds. The error shape, parameter validation and cursor opacity
follow [013's list-api.md](../../013-library-search/contracts/list-api.md) and
[014's tags-api.md §5](../../014-video-tags/contracts/tags-api.md). The query
itself is in [data-model.md §5](../data-model.md#5-library-items).

Who may use each route (`security`) matches `GET /api/videos`:

| Route | Who |
| --- | --- |
| `GET /api/library`, one group | Owner and guests (`security` includes `{}`) |
| `GET /api/library/ids` | Owner only (as the current `listVideoIds`) |

Differences in guest responses follow
[data-model.md §7](../data-model.md#7-visibility-per-audience) and
[016 guest-api.md](../../016-single-account-auth/contracts/guest-api.md)
(`watch`, played sort orders and `tag` are 400).

`GET /api/videos` stays a list of single videos (used by search results at the
folder screen root). `GET /api/videos/ids` is removed in the unit that switches
the library to `GET /api/library/ids`.

## 1. `GET /api/library`

- Parameters: the same as `GET /api/videos` (`query`, `watch`, `playable`,
  `sort`, `seed`, `cursor`, `limit`, `tag`).
- Response `LibraryPage`:

```yaml
LibraryPage:
  required: [items, total]
  properties:
    items: { type: array, items: { $ref: LibraryItem } }
    total: { type: integer }        # number of entries (cards) after filtering
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
  # watchedCount and watchState are always present for the owner and omitted for guests (data-model.md §7)
  properties:
    folder: { $ref: VideoFolder }   # the group's folder; the key that identifies the group
    name: { type: string }
    videoCount: { type: integer }
    watchedCount: { type: integer }
    watchState: { type: string, enum: [unwatched, inProgress, watched] }
    durationMs: { type: integer, format: int64 }   # omitted when no member's duration is known
    sizeBytes: { type: integer, format: int64 }
    addedAt: { type: string, format: date-time }
    lastPlayedAt: { type: string, format: date-time }  # omitted when none
    previews: { type: array, maxItems: 4, items: { $ref: FolderPreview } }  # up to 4 members with generated thumbnails, in order. Used for the card's folder artwork; a list row uses the first as its thumbnail
    openVideoId: { type: integer, format: int64 }  # the member opened on press (data-model.md §6)
    videoIds: { type: array, items: { type: integer, format: int64 } }  # every member, in order
    tags: { type: array, items: { $ref: VideoTag } }  # union of the members' tags (sources united too)
```

A group's `id` is not exposed; a group is addressed by `folder`. Exposing `id`
is rejected: the screen would keep a value reassigned on every rebuild as its
key for selection and refetching.

## 2. `GET /api/library/ids`

- Parameters: those of `GET /api/library` without `sort`, `seed`, `cursor` and
  `limit`.
- Response: the existing `VideoIdsResponse` (`ids`, `missingTagIds`). `ids`
  holds the `id`s of matching video entries and the `id`s of **every member**
  of matching groups. Limits and errors are the same as the current
  `GET /api/videos/ids`.

## 3. One group

`GET /api/folders/{rootId}/group?path=…`

- Refetches a group card still in the list (Structural Decisions 10 in the
  plan).

| Status | `code` | When |
| --- | --- | --- |
| 200 | — | `LibraryGroup`, built from every member of the group regardless of filters |
| 400 | — | The path breaks the rules (as `FolderPath`) |
| 404 | `not_found` | The folder is not a group now, or does not exist. The screen removes that card from the list |
