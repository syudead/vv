# Research: Favorite videos and groups

Parent Issue: #574. Inherited decisions:

| Topic | Source of truth |
| --- | --- |
| Tech stack, boundaries, dependency direction, rebuildable index versus user data | [ARCHITECTURE.md](../../ARCHITECTURE.md), [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) |
| User key (`bundle:<id>` for a bundle, otherwise `content_key`) | [specs/030-video-versions/data-model.md, User key](../030-video-versions/data-model.md#user-key) |
| Folder key (`domain.FolderKey`) and manual group settings | [specs/017-folder-groups/data-model.md, Migration](../017-folder-groups/data-model.md#migration) |
| How library items are built | [specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md) |

This file records only the decisions this feature adds.

## R-1: Two tables: `video_favorites` by user key, `folder_favorites` by folder key

**Decision**: Add `video_favorites(content_key primary key, favorited_at)` and
`folder_favorites(path primary key, favorited_at)`
([data-model.md, Migration](data-model.md#migration)). A video's key is `userKeyExpr` (the bundle's key for a
bundle member), the same as `public_videos`. A group's key is `domain.FolderKey(<absolute folder path>)`, the
same as `folder_group_overrides`. Neither table has a foreign key to `videos`, `folder_groups` or
`media_folders`. Same-path succession (`moveUserData`) and bundling and unbundling (`userDataTables`) also copy
`video_favorites`.

| Option | Verdict |
| --- | --- |
| **Two tables keyed like the existing user data** | Chosen |
| One table like `public_videos` with a "video or folder" column | Rejected: the keys mean different things (a content identifier and a folder path), and the succession and bundling paths copy only video rows, so with separate tables each path reads only its own table |
| Columns on `videos` and `folder_groups` | Rejected: both are rebuildable indexes whose rows are deleted by a rescan and a folder index rebuild (ARCHITECTURE.md "Rebuildable and user data") |

**Rationale**: Requirements 2 and 3 and the Edge Cases (keep the record after a move, a rescan, unregistering,
or the folder ceasing to be a group) are properties that playback positions, public flags and manual group
settings already have. Keying favorites the same way puts them on the same paths (`userKeysForVideoIDs`,
`moveUserData`, `userDataTables`, tables an index rebuild does not touch). The Edge Case "one record per
bundle once #572 lands" is met by the user key, because 030 has already landed.

## R-2: One owner-only `PUT /api/favorites` for videos and folders in one transaction

**Decision**: Add `PUT /api/favorites` (`videoIds`, `folders`, `favorite`). `SetFavorites` on the new role
type `FavoriteStore` (holds only the `sql` connection, like `VisibilityStore`) writes in one transaction
([contracts/screen-api.md, `PUT /api/favorites`](contracts/screen-api.md#put-apifavorites),
[data-model.md, Writes (`FavoriteStore`)](data-model.md#writes-favoritestore)). `videoIds` are resolved with
`userKeysForVideoIDs` to the keys of videos currently in the library. `folders` are turned into absolute paths
from the registered folder id and relative path, and only paths in `folder_groups.path_key` (folders that are
groups for the owner now) are written. An id that does not resolve, an unregistered `rootId` and a folder that
is not a group now are not errors; they are left out of `appliedVideos` and `appliedFolders` in the response.
Items already in the requested state are counted (Edge Case: "do not fail when some are already favorites").

| Option | Verdict |
| --- | --- |
| **One endpoint, one transaction, skip what does not resolve** | Chosen |
| `PUT /api/video-favorites` for videos and `PUT /api/folders/{rootId}/favorite` for folders | Rejected: a bulk change becomes two transactions, and the screen has to handle one succeeding without the other |
| `404 not_folder_group` for a folder that is not a group | Rejected: one miss fails the whole bulk change, so when a group is dissolved right after "Select all" nothing is favorited |
| Write without checking that the folder is a group now | Rejected: the API could then favorite a plain folder, leaving records for something out of scope |

**Rationale**: The multiple selection of requirement 6 sends videos and groups at once. The visibility toggle
(`PUT /api/video-visibility`) skips ids that are not in the library and reports a partial result through the
counts in its response, and the screen (`api/visibility.ts`) already handles that shape. Using the same shape
lets the screen reuse its refetch mechanism (R-5). When a single card action returns `appliedFolders` 0, the
screen learns that the folder is no longer a group, refetches the group card and removes it (as in 017
"Refresh and removal"). Plain folders are out of scope (parent Issue), so a folder that is not a group now is
not written.

## R-3: Favorites-only filter applied where `libraryItemsCTE` builds items

**Decision**: Add `VideoQuery.FavoriteOnly` (`favorite=true`). For `GET /api/videos` and
`GET /api/folders/{rootId}/videos` it is a per-video condition in `filteredFrom` (a `video_favorites` row
exists). For `GET /api/library` and `GET /api/library/ids`, the videos matched by search terms and tags
(`matched`) and the groups whose members all match (`whole`) are built as today, and the item-building step
changes as follows ([data-model.md, Reads and lists](data-model.md#reads-and-lists)):

| Item | Built from |
| --- | --- |
| Group item | Groups in `whole` when there is no filter, or whose folder is in `folder_favorites` |
| Video item | Matched videos that do not belong to a `whole` group, or, under the filter, whose group is not in `folder_favorites`; under the filter, only those in `video_favorites` |

Playability and watch state still apply to the items, and `total` and `GET /api/library/ids` count the same
items.

| Option | Verdict |
| --- | --- |
| **Check the group's own favorite where items are built** | Chosen |
| Add favorites to `matched` and build items with the 027 rule | Rejected: fails requirements 8 and 9, as the rationale explains |
| Apply the filter only after items are built (the watch-state step) | Rejected: the item of a group that is not a favorite drops out, so its favorite member videos are not listed (acceptance criterion 4) |

**Rationale**: Requirement 8 judges each item by its own favorite, and requirement 9 lists the favorite
members of a group that is not a favorite as separate items. Reusing the 027 rule (a partial match lists
members one by one) by adding `video_favorites` to `matched` would hide a favorite group G whose members are
not favorites (requirement 8, acceptance criterion 5), and would make G a group item when all its members are
favorites (requirement 9). Checking the group's own favorite where items are built is the smallest change that
meets both.

## R-4: `favoritedAsc` and `favoritedDesc` sorts with non-favorites last

**Decision**: Add `favoritedAsc` and `favoritedDesc` to `VideoSort`. The value is
`video_favorites.favorited_at` for a video item, `folder_favorites.favorited_at` for a group item, and NULL
for a non-favorite, so the `listOrder.nullable` rule applies (rows without a value go last in either
direction, and the cursor has the same shape). Favoriting again after removing gives a new time. Favoriting
something that is already a favorite keeps its time.

**Rationale**: The Edge Case "non-favorite items follow the favorite items as one block" has the same shape as
last played (`played*`, videos without a record go last), so `listOrders`, `itemOrderValues` and the cursor
rules apply unchanged.

**Alternatives considered**: Ordering non-favorites by another value such as the date added. The cursor would
carry two values and change the 013 keyset shape (one value and the id). Rejected.

## R-5: Favorites hidden from guests; filter and sort return `400`

**Decision**: Add `FavoriteOnly` and `favorited*` to `Audience.CheckVideoQuery`, which returns
`ErrGuestQueryNotAllowed` (`400 guest_filter_not_allowed`) for a guest. `Video.favorite` and
`LibraryGroup.favorite` are set only in owner responses; `forAudience` and the group response builder omit
them for a guest. `PUT /api/favorites` is owner-only (the `/api/*` default). The screen normalises the
criteria with `guestListCriteria` and shows guests neither the controls nor the marks.

**Rationale**: Requirement 12 and acceptance criterion 10 settle this directly. Treating favorites like watch
state, the last-played sorts and tags means the contract (guest-api.md, [Conditions guests cannot use](../016-single-account-auth/contracts/guest-api.md#conditions-guests-cannot-use)) and the screen's normalisation each
gain one line in the same place.

**Alternatives considered**: Always return `favorite: false` to guests. The requirement is to hide the state,
and unlike `public`, which has no meaning for a guest, the field itself is the owner's data, so omitting it is
the correct contract. Rejected.

## R-6: No domain event; screens reuse the visibility subscription pattern

**Decision**: `web/src/api/favorites.ts` holds `updateFavorites` (`PUT /api/favorites`), the result
subscriptions (`subscribeFavorites`, and `subscribeFavoritesStale` for a partial result), and the update of
the list snapshot (`applyFavoritesToListSnapshot`). In the same shape as `api/visibility.ts`, the list
(`useVideos`) replaces `favorite` on video items in place and refetches group items with
`GET /api/folders/{rootId}/group` (the 017 "Refresh and removal" path: when `appliedFolders` is below the
number requested, the affected groups are refetched and removed on 404). The video page refetches with
`GET /api/videos/{id}` after success (as in 033 R-8). No new domain event and no new `/api/events` kind. A
card unfavorited in a favorites-only list stays in the list and disappears on the next load (Edge Case: "show
the latest state on the next refetch").

| Option | Verdict |
| --- | --- |
| **Reuse the visibility subscriptions; refetch the video page** | Chosen |
| Add an event like `domain.VideoOverrideChanged` and announce it as `video` on `/api/events` | Rejected: every toggle would refetch list items, so favoriting 20,000 at once would mean 20,000 refetches |
| Remove an unfavorited card from a favorites-only list at once | Rejected: the selection bar's "Remove tag" refetches (library-ui.md, [List layout](../../docs/design-docs/library-ui.md#list-layout)), but a list that moves on a single card action shifts the card positions and stops repeated presses; leave it to the next load |

**Rationale**: The visibility toggle already solves "replace without refetching", "refetch on a partial
result" and "settle in the order sent", and favoriting has the same properties: one owner action, the success
response gives the result, and other tabs catch up on their next refetch. A server notification would only
sync other tabs, which the Edge Cases do not ask for.

## R-7: Selection keeps chosen groups as groups

**Decision**: The library selection (`LibraryPage`) holds, besides the set of video ids, the groups chosen
with a group card's checkbox (the folder and its member ids). When any member leaves the selection, that
group leaves the chosen groups. "Select all" takes the chosen groups from `groups`, added to the
`GET /api/library/ids` response (the folder and member ids of each group item,
[contracts/screen-api.md, Fields added to `GET /api/library/ids`](contracts/screen-api.md#fields-added-to-get-apilibraryids)). The selection
bar's favorite action sends `videoIds` = the selected ids that are not members of a chosen group, and
`folders` = the chosen groups. Adding and removing tags, visibility and bundling still send the set of video
ids.

| Option | Verdict |
| --- | --- |
| **The screen keeps chosen groups and splits what it sends** | Chosen |
| The server removes members of `folders` from `videoIds` | Rejected: what the screen sends and the result diverge, and `appliedVideos` no longer means "of the ids sent" |
| Derive chosen groups from loaded group items whose members are all selected | Rejected: groups on pages not loaded but included by "Select all" would be favorited as videos |

**Rationale**: Requirement 6 and acceptance criterion 7 ask that a chosen group gets the group favorite and
its members do not. Today the selection is only a set of video ids (017 "Pressing and selection"), and when
"Select all" includes pages that are not listed, the screen does not know which ids are group members.

## R-8: Favoriting does not advance `video_edits`

**Decision**: `FavoriteStore` does not call `touchEditedAt`. `Video.updatedAt` does not change with a
favorite.

**Rationale**: 033 R-2 limits advancing the edit time to the four operations that write a video's information,
and does not advance it for playback positions. A favorite is not video information (display name, tags,
visibility, representative thumbnail); like a playback position it is the owner's own mark, with its own time
(`favorited_at`) and sort. Advancing the edit time of every video each time 20,000 are favorited at once would
hide when a video was last edited (the same reason as 033 R-2).

**Alternatives considered**: Advance it, as the visibility setting does. Visibility is a video setting that
changes who can see it; a favorite is a mark only the owner has. Rejected. The parent Issue says neither, so
the PR body asks for confirmation.
