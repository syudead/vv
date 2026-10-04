# Contract: Folder grouping, groups on the video page, and tag responses

Source of truth: `api/openapi.yaml`. This document covers only the routes it
adds and the schemas that change. Folders are addressed (`rootId` and `path`)
as by the existing `FolderRootId` and `FolderPath` parameters.

## 1. Folder grouping

```yaml
FolderGrouping:
  required: [mode, grouped, taggable]
  properties:
    mode: { type: string, enum: [auto, ungroup, groupDirect] }  # auto = no override
    grouped: { type: boolean }   # whether this folder's direct videos are a group now
    taggable: { type: boolean }  # a group, and not the registered folder itself (§2)
```

- `FolderSummary` gains `grouping: FolderGrouping`. It is always present in the
  owner's responses and omitted for guests (an optional field). The folder
  screen menu decides its items from it and is not shown to guests.
- The routes that change state in this section and §2 are owner only (the
  default `security`).
- `PUT /api/folders/{rootId}/grouping?path=…`, body
  `{ "mode": "auto" | "ungroup" | "groupDirect" }`:

| Status | When |
| --- | --- |
| 200 | The `FolderGrouping` after the change. `auto` removes the override. Setting the same value again is also 200 |
| 400 | Invalid path or `mode` |
| 404 | The folder does not exist (the same check as `getFolder`) |

- Saving the override and rebuilding the index happen in one transaction
  ([data-model.md §3](../data-model.md#3-when-the-index-is-rebuilt)). The later
  request wins (Edge Case `同時操作`).

## 2. Turning a group into a tag

`POST /api/folders/{rootId}/grouping/tag?path=…` (no body)

| Status | `code` | When |
| --- | --- | --- |
| 200 | — | `{ "tag": TagRef, "created": boolean, "grouping": FolderGrouping }`. `created` is true when the tag was newly created |
| 400 | `invalid_request` | The folder name breaks the tag name rules (`NormalizeTagName`). No tag or override is created |
| 404 | — | The folder does not exist |
| 409 | `conflict` | The folder is not a group now, or it is the registered folder itself (the registered folder's name is not matched as a folder-derived tag, [data-model.md §4](../data-model.md#4-folder-derived-tags)) |

- `FolderGrouping` gains `taggable: boolean` (required). The screen shows
  `グループをタグに変える` only when it is true.
- Looking up or creating the tag, saving `ungroup` and rebuilding happen in one
  transaction ([data-model.md §4](../data-model.md#4-folder-derived-tags)).

## 3. Groups of videos and related videos

```yaml
VideoGroupRef:          # Video.group in GET /api/videos/{id} (only for members)
  required: [folder, name, position, count]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    position: { type: integer }   # 1-based
    count: { type: integer }

RelatedGroup:           # RelatedVideos.group (only for members)
  required: [folder, name, items]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    items: { type: array, items: { $ref: Video } }   # every member, in order, including the current video
```

- `Video.group` is only in the `GET /api/videos/{id}` response (as `location`
  is), never in list entries.
- For guests, `position`, `count`, `group.items` and previous/next are built
  from public members only. When only one member is public, `group` is omitted
  and the response is the same as for a video outside any group
  ([data-model.md §7](../data-model.md#7-visibility-per-audience)).
- `GET /api/videos/{id}/related` for a member:
  - It includes `group`. The 20-item limit on `items` does not apply to the
    group (Edge Case `大きなグループ`).
  - `nextId` and `prevId` are the next and previous members in the group's
    order. The last member has no `nextId` and the first no `prevId`.
  - `items` (related videos) is built by removing members of the same group
    from the input of `domain.OrderRelated` (videos in the same folder and
    videos added close in time) **first**, then ordering as now. The 20-item
    limit applies afterwards, so related videos remain even for groups of more
    than 20. The count passed to `VideosAddedNear` grows to allow for the
    removed members.
- Responses for videos outside a group are unchanged (second half of
  requirement 27, acceptance criterion 17).

## 4. Changes to tag responses

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
  ([data-model.md §4](../data-model.md#4-folder-derived-tags)).
- `remove` in `POST /api/video-tags` removes only tags attached by hand. The
  response (`tag`, `applied`) does not change.
