# Contract: Folder grouping, the group on the video page, and the tag response

`api/openapi.yaml` is the source of truth. This document lists only the added routes and the changed
schemas. A folder is addressed (`rootId` and `path`) exactly as by the existing `FolderRootId` and
`FolderPath` parameters.

## 1. Folder grouping

```yaml
FolderGrouping:
  required: [mode, grouped, taggable]
  properties:
    mode: { type: string, enum: [auto, ungroup, groupDirect] }  # auto = no override
    grouped: { type: boolean }   # whether the videos directly in this folder form a group now
    taggable: { type: boolean }  # a group, and not the media folder itself (§2)
```

- Add `grouping: FolderGrouping` to `FolderSummary`. It is always present in the owner's response
  and omitted for a guest (an optional field). The folder page menu uses it to choose its items and
  is not shown to a guest.
- The mutating routes in this section and §2 are owner-only (the default `security`).
- `PUT /api/folders/{rootId}/grouping?path=…`, body `{ "mode": "auto" | "ungroup" | "groupDirect" }`
  - 200: the updated `FolderGrouping`. `auto` removes the override. Setting the same value again
    also returns 200.
  - 400: an invalid path or `mode`. 404: the folder does not exist (same check as `getFolder`).
  - Saving the override and rebuilding the index run in one transaction
    ([data-model.md §3](../data-model.md#3-rebuild-points)). The later request wins (Edge case
    "Concurrent operations").

## 2. Turning a group into a tag

`POST /api/folders/{rootId}/grouping/tag?path=…` (no body)

| Status | Meaning |
| --- | --- |
| 200 | `{ "tag": TagRef, "created": boolean, "grouping": FolderGrouping }`. `created` is true when a new tag was created. |
| 400 `invalid_request` | The folder name violates the tag name rules (`NormalizeTagName`). Neither a tag nor an override is created. |
| 404 | The folder does not exist. |
| 409 `conflict` | The folder is not a group now, or it is the media folder itself. The media folder's own name is not matched as a folder-derived tag ([data-model.md §4](../data-model.md#4-folder-derived-tags)). |

- Add `taggable: boolean` (required) to `FolderGrouping`. The UI shows "Turn the group into a tag"
  only when it is true.
- Resolving or creating the tag, saving `ungroup`, and the rebuild run in one transaction
  ([data-model.md §4](../data-model.md#4-folder-derived-tags)).

## 3. The group of a video and of its related videos

```yaml
VideoGroupRef:          # Video.group in GET /api/videos/{id} (members only)
  required: [folder, name, position, count]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    position: { type: integer }   # 1-based
    count: { type: integer }

RelatedGroup:           # RelatedVideos.group (members only)
  required: [folder, name, items]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    items: { type: array, items: { $ref: Video } }   # all members, in order, including the current video
```

- `Video.group` appears only in the `GET /api/videos/{id}` response (like `location`). List items do
  not carry it.
- For a guest, `position`, `count`, `group.items` and the previous/next videos are built from the
  public members only. When only one member is public, `group` is omitted, and the response equals
  that of a video outside any group ([data-model.md §7](../data-model.md#7-visibility-per-viewer)).
- `GET /api/videos/{id}/related` for a member:
  - It includes `group`. The `items` limit of 20 does not apply to the group (Edge case "Large
    group").
  - `nextId` and `prevId` are the next and previous videos in the group order. The last member has
    no `nextId`, and the first member has no `prevId`.
  - `items` (related videos) are ordered by the current ordering after members of the same group are
    removed **first** from the `domain.OrderRelated` input (videos in the same folder and videos
    with a close added time). The limit of 20 applies afterwards, so a group of more than 20 videos
    still leaves related videos. The count passed to `VideosAddedNear` also grows by the number of
    removed videos.
- The response for a video outside any group is unchanged (second half of Requirement 27,
  Acceptance criterion 17).

## 4. Changes to the tag response

```yaml
VideoTag:               # elements of Video.tags and LibraryGroup.tags (replaces TagRef)
  required: [id, name, manual, fromFolder]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }        # the primary name
    manual: { type: boolean }     # attached by hand
    fromFolder: { type: boolean } # attached from an ancestor folder name
```

- For a guest, `Video.tags` and `LibraryGroup.tags` stay empty arrays as today, and folder-derived
  tags are not included either.
- Add `manualCount` (required, the number of videos with the tag attached by hand) to
  `VideoTagsSummaryItem`. `count` is the number of videos with the tag from either source.
- `Tag.videoCount`, filtering by `tag`, and tag name matching in the search box include
  folder-derived tags ([data-model.md §4](../data-model.md#4-folder-derived-tags)).
- `remove` in `POST /api/video-tags` removes only tags attached by hand. The response (`tag`,
  `applied`) does not change.
