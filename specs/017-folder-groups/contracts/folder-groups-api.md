# Contract: Folder grouping, groups on the video page, and tag responses

Source of truth: `api/openapi.yaml`. This document covers only the routes it
adds and the schemas that change. Folders are addressed (`rootId` and `path`)
as by the existing `FolderRootId` and `FolderPath` parameters.

## Folder grouping

```yaml
FolderGrouping:
  required: [mode, grouped, taggable]
  properties:
    mode: { type: string, enum: [auto, ungroup, groupDirect] }  # auto = no override
    grouped: { type: boolean }   # whether this folder's direct videos are a group now
    taggable: { type: boolean }  # a group, and not the registered folder itself (see "Turning a group into a tag")
```

- `FolderSummary` gains `grouping: FolderGrouping`. It is always present in the
  owner's responses and omitted for guests (an optional field). The folder
  screen menu decides its items from it and is not shown to guests.
- The routes that change state in this section and [Turning a group into a tag](#turning-a-group-into-a-tag) are owner only (the
  default `security`).
- `PUT /api/folders/{rootId}/grouping?path=…`, body
  `{ "mode": "auto" | "ungroup" | "groupDirect" }`:

| Status | When |
| --- | --- |
| 200 | The `FolderGrouping` after the change. `auto` removes the override. Setting the same value again is also 200 |
| 400 | Invalid path or `mode` |
| 404 | The folder does not exist (the same check as `getFolder`) |

- Saving the override and rebuilding the index happen in one transaction
  ([data-model.md, When the index is rebuilt](../data-model.md#when-the-index-is-rebuilt)). The later
  request wins (Edge Case `同時操作`).

## Turning a group into a tag

`POST /api/folders/{rootId}/grouping/tag?path=…` (no body)

| Status | `code` | When |
| --- | --- | --- |
| 200 | — | `{ "tag": TagRef, "created": boolean, "grouping": FolderGrouping }`. `created` is true when the tag was newly created |
| 400 | `invalid_request` | The folder name breaks the tag name rules (`NormalizeTagName`). No tag or override is created |
| 404 | — | The folder does not exist |
| 409 | `conflict` | The folder is not a group now, or it is the registered folder itself (the registered folder's name is not matched as a folder-derived tag, [data-model.md, Folder-derived tags](../data-model.md#folder-derived-tags)) |

- `FolderGrouping` gains `taggable: boolean` (required). The screen shows
  `グループをタグに変える` only when it is true.
- Looking up or creating the tag, saving `ungroup` and rebuilding happen in one
  transaction ([data-model.md, Folder-derived tags](../data-model.md#folder-derived-tags)).

## Groups of videos and related videos

```yaml
VideoGroupRef:          # Video.group in GET /api/videos/{id} (only for members)
  required: [folder, name, position, count]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    position: { type: integer }   # 1-based
    count: { type: integer }

RelatedGroup:           # RelatedVideos.group (only for members)
  required: [folder, name, items, offset, total]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    items: { type: array, maxItems: 100, items: { $ref: Video } }   # consecutive members around the current video, in order
    offset: { type: integer }   # 0-based position of items[0] in the group's order
    total: { type: integer }    # number of members

GroupMemberPage:        # GET /api/videos/{id}/group-members?offset=&limit=
  required: [items, offset, total]
  properties:
    items: { type: array, maxItems: 200, items: { $ref: Video } }
    offset: { type: integer }
    total: { type: integer }
```

- `Video.group` is only in the `GET /api/videos/{id}` response (as `location`
  is), never in list entries.
- For guests, `position`, `count`, `group.items`, `group.total` and
  previous/next are built from public members only. When only one member is public, `group` is omitted
  and the response is the same as for a video outside any group
  ([data-model.md, Visibility per audience](../data-model.md#visibility-per-audience)).
- `GET /api/videos/{id}/related` for a member:
  - It includes `group`. The 20-item limit on `items` does not apply to the
    group (Edge Case `大きなグループ`). `group.items` is a window of up to 100
    consecutive members with the current video in the middle, moved inwards
    at either end of the group, so the response does not grow with the
    group (issue 674). `offset` and `total` place the window in the whole
    group.
  - `nextId` and `prevId` are the next and previous members in the group's
    order. The last member has no `nextId` and the first no `prevId`.
  - `items` (related videos) is built by removing members of the same group
    from the input of `domain.OrderRelated` (videos in the same folder and
    videos added close in time) **first**, then ordering as now. The 20-item
    limit applies afterwards, so related videos remain even for groups of more
    than 20. The count passed to `VideosAddedNear` grows to allow for the
    removed members.
- `GET /api/videos/{id}/group-members` returns up to `limit` (1 to 200,
  default 100) members from the 0-based `offset` (default 0) in the group's
  order, in the same form and with the same audience rules as `group.items`.
  An `offset` at or past `total` returns empty `items`. A video that is not a
  member, or whose group is not shown to the viewer, is `404`; an `offset`
  below 0 or a `limit` outside the range is `400`. The video page reads the
  members outside the first window with it.
- `GET /api/videos/{id}` reads only the member ids to fill `position` and
  `count`.
- Responses for videos outside a group are unchanged (second half of
  requirement 27, acceptance criterion 17).

## Changes to tag responses

```yaml
VideoTag:               # Element of Video.tags and LibraryGroup.tags (replaces TagRef)
  required: [id, name, manual, fromFolder]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }        # the original name
    manual: { type: boolean }     # has a part attached by hand
    fromFolder: { type: boolean } # attached from an ancestor folder's name
```

- For guests, `Video.tags` and `LibraryGroup.tags` stay empty arrays, with no
  folder-derived tags either.
- `VideoTagsSummaryItem` gains `manualCount` (required, the number of videos
  with the tag attached by hand). `count` is the number with the tag from
  either source.
- `Tag.videoCount`, the `tag` filter and tag name matching in the search box
  include folder-derived tags
  ([data-model.md, Folder-derived tags](../data-model.md#folder-derived-tags)).
- `remove` in `POST /api/video-tags` removes only tags attached by hand. The
  response (`tag`, `applied`) does not change.
