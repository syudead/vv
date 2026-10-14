# Data model: Drag and drop to reorder, group, merge and bundle

Parent Issue: #1075. No table or column is added and no migration is
written: the operations below write the tables of
[048 data-model](../048-groups/data-model.md) and
[030 data-model](../030-video-versions/data-model.md), and the undo journal is
memory only ([R-2](research.md#r-2-the-undo-journal-lives-in-internalapp-in-memory-for-10-minutes)).
The rest of the model is unchanged
([internal/store/migrations/](../../internal/store/migrations/)).

## Operations

Each operation is one transaction of the new `OrganizeStore` role, publishes
`GroupsChanged` (and `VideoBundleChanged` for a bundle) after commit, and
returns its images. Dragged cards resolve as R-4 says; a video or group that
does not resolve, or a dragged card not in `from`, is
`ErrGroupTargetNotFound` and nothing is written
([048 R-12](../048-groups/research.md#r-12-edits-are-positional-and-fail-whole-when-a-target-is-gone)).

| Operation | Rule |
| --- | --- |
| Reorder | The dragged items of group G, in screen order, move together after one item of G, or to the top; positions stay `0 … n-1` |
| Move | Appends the dragged cards to `to` in screen order, skipping those already there; when `from` is set, removes them from `from`. Without `to`, only the removal happens (dropped on the library). Removing a folder member writes its exclusion and removing the last item dissolves `from`, as 048's Remove does |
| Group | Creates a group named by the target's title cut to 100 characters, holding the target and then the dragged videos ([R-6](research.md#r-6-make-a-group-puts-the-new-group-where-the-target-was)); with `from`, the new group takes the target's position in `from` and the target and the dragged items leave `from` |
| Merge | Walks the dragged cards in screen order: a group is merged as [R-5](research.md#r-5-a-merge-hands-the-dropped-groups-favorites-playlist-items-and-parents-to-the-target) says, a video is appended to the target as Move does, so a mixed selection is one transaction and one token |
| Bundle | `VersionStore.Bundle`'s rules with the target as the representative ([030 data-model](../030-video-versions/data-model.md)); already bundled videos bring their whole bundle (Edge Case) |
| Undo | Writes the before image back when the current rows equal the after image, then rebuilds the folder index and renumbers touched playlists as the operation did; otherwise `ErrUndoConflict` and nothing is written |

`domain.ValidateNesting` applies to Move, Group and Merge: a dragged group
that is `to` or one of `to`'s ancestors is `ErrGroupCycle` (requirement 15,
Edge Case). Merging a group into itself is the same error.

## Undo image

An image is every row, with its id, that the operation wrote, deleted or
renumbered, in the tables below, read inside the operation's transaction
before and after the change. The store serialises it to bytes; only the store
reads them back ([R-1](research.md#r-1-undo-restores-the-rows-the-operation-changed-held-by-the-server)).

| Operation | Rows in the image |
| --- | --- |
| Reorder | `group_items` of G |
| Move | `groups`, `group_items` (by `group_id` and by `child_id`), `group_exclusions` and `group_favorites` of `from`, `to` and every ancestor the dissolve cascade can reach; `playlist_items` of every playlist holding one of them; `folder_group_overrides` of their folders |
| Group | `groups` of the new group; `group_items` of the new group and of `from` |
| Merge | Move's rows for the dragged groups, their parents and the target |
| Bundle | `video_bundles` and `video_bundle_members` of every absorbed bundle and the new one; `group_items`, `group_exclusions` and `playlist_items` of every absorbed key and the new key, by whole group or playlist; the user data rows `copyUserData` writes under the new key; `video_version_candidates` the bundle prunes |

Every image also covers the rows that reference a group the undo would delete or recreate: `group_items` by `child_id`, `group_favorites` and `playlist_items` by `group_id`. A reference added after the operation (the new group favorited, added to a playlist or put in another group) therefore differs from the after image, and the undo is `ErrUndoConflict` instead of a cascade that drops it.

Restoring a deleted group or bundle reinserts it with its old id; SQLite's
`autoincrement` never hands that id out again, so it is free.

## Rules

| Rule | Enforced in |
| --- | --- |
| Nesting and the "own descendant" drop | `domain.ValidateNesting` |
| The new group's name from the target's title | `domain.ValidateGroupName` after cutting to 100 characters |
| Merge's replacement in playlists and parents | `domain.PlanGroupMerge`, applied by `OrganizeStore` |
| An undo token is used once and lives 10 minutes, at most 100 at a time | `app.Organizer` |
| After every operation and every undo, the positions of each touched group are `0 … n-1` | `internal/store/invariants_test.go` (existing rule) |

## What does not change

The existing group, version and favorite routes keep their behaviour and
return no undo token: the menus and dialogs of 048 and 030 stay as they are,
and only drops go through the operations above.
