# Contract: How library list items are built (groups matched only in part)

Source of truth: `api/openapi.yaml`. Paths, parameters and response schemas do not change from
[017's library-api.md](../../017-folder-groups/contracts/library-api.md). This document records only
the delta in the rules by which `GET /api/library` and `GET /api/library/ids` gather matched videos
into items. §1 here replaces items 1–2 of
[017 data-model.md §5](../../017-folder-groups/data-model.md#5-library-items); item 3 onward and §6
and §7 stay in force.

## 1. How `GET /api/library` builds items

1. **Matched members**: `chosen` (scope and search expression) joined with `videos` and filtered by
   the tag AND (the OR over sources from 017 §4) gives the "matched videos". A single video must
   satisfy both the search query and the tags (requirement 3). Playability is not part of this
   (requirement 6, [research.md R-2](../research.md#r-2-only-the-search-query-and-tags-decide-playability-still-applies-to-items-as-before)).
2. **Visible members and groups**: unchanged. `gm` is the members with a location the viewer may
   see, and `live` is the groups whose `gm` count is at least the viewer's minimum (owner 1, guest 2).
3. **Gathering into items**: for each `live` group, compare the number of matched members with the
   `gm` count.

   | Matched members | Items |
   | --- | --- |
   | Equal to `gm` (all members matched) | One group item. Its values are built from all `gm` members as before (requirement 2). |
   | At least 1 and fewer than `gm` (only some matched) | Each matched member becomes a video item (requirement 1). Its location is the `chosen` location, and video item values are built as before. |
   | 0 | Nothing. |

   Members of groups that are not `live`, and videos not in any group, become video items when they
   match, as before.

   With no search query and no tags, every `gm` member is in `chosen`, so every group counts as "all
   members matched" and the list is the same as today (requirement 4).
4. **Per-item filters**: playability is judged per video for a video item and, for a group item, by
   whether any one member is playable (the same result as today). Watch status is applied to the
   item's watch status as before (requirement 6). A member shown on its own has its own watch status
   (requirement 5).
5. **Sorting, keyset and count**: as before, done on item values. The key of a member shown on its
   own is the video `id` (the same as a video item), and the key of a group item is the `id` of the
   first member in `gm` order. A group's card and its members' items never appear together, so keys do
   not collide between items. `total` is the number of items (requirement 5).

For guests, `gm` is only the public members, so "all members" is counted over public members. If only
some public members match, each matched public member is shown on its own (requirement 7).

The Edge Cases in the parent Issue follow from these rules: a single matched member is still shown on
its own; 11 of 12 matched is "not equal", so the 11 are shown one by one; members shown on their own
may not be adjacent after sorting.

## 2. `GET /api/library/ids`

For items that passed items 1–4 of §1, it returns the video's `id` for a video item and the `id` of
every `gm` member for a group item (requirement 8). Members of a group matched only in part are video
items, so only the matched members' `id`s are included. It is built from the same `with` clause as
`GET /api/library`, so the rule is not held twice
([R-1](../research.md#r-1-the-decision-is-made-in-the-sql-stage-that-builds-items)).

## 3. What does not change

- `GET /api/folders/{rootId}/group`: built from all `gm` members regardless of filters (017 §3). It is
  the path by which the screen refetches a group card still in the list, and the card is not
  rearranged when the matches change while the list is open (Edge Case in the parent Issue).
- Response schemas (`LibraryPage`, `LibraryItem`, `LibraryGroup`, `VideoIdsResponse`), parameters,
  the error shape and the cursor shape.
- `GET /api/videos` and the folder screen (still a one-video-per-row list).

## 4. Documents and tests to update

| Target | Change |
| --- | --- |
| `description` of `listLibrary` and `listLibraryIds` in `api/openapi.yaml` | Replace "a group appears when at least one member matches" with the §1 rules and reference this document. Regenerate with `task generate`. |
| The `GET /api/library` paragraph of `ARCHITECTURE.md` ("a group appears when any member matches") | Change to the §1 rules and add this document as a reference. |
| Comments in `internal/store/library_items.go` | Point the references to 017 §5 at this document. |
| The `ownerGroups` premise in `web/e2e/guest.e2e.ts` (the owner's `ゲスト` search shows `非公開だけ` as a group) | No longer holds. `公開あり` matches all 5 members, so it is a group card; `非公開だけ` matches only D, so it becomes D's video card. Keep the step that selects a group by searching for a term that matches every member (such as the folder name). |
