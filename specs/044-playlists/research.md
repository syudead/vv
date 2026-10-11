# Research: Named playlists and continuous playback

Parent Issue: #787. Inherited decisions:

| Topic | Source of truth |
| --- | --- |
| Boundaries, user data versus the rebuildable index, the authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| User key, bundles, same-path succession | [030 data-model, User key](../030-video-versions/data-model.md#user-key), [Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| Folder key, group membership and order, the member a group opens at | [017 data-model, Assignment rules](../017-folder-groups/data-model.md#assignment-rules), `GroupOpenIndex` in [internal/domain/library_item.go](../../internal/domain/library_item.go) |
| Favorites: keys, bulk write shape, selection of videos and groups | [035 research, R-1](../035-favorites/research.md#r-1-two-tables-video_favorites-by-user-key-folder_favorites-by-folder-key), [R-2](../035-favorites/research.md#r-2-one-owner-only-put-apifavorites-for-videos-and-folders-in-one-transaction), [R-7](../035-favorites/research.md#r-7-selection-keeps-chosen-groups-as-groups) |
| Group autoplay notice and the end-of-playback layer | [017 ui-design, Autoplay notice](../017-folder-groups/ui-design.md#autoplay-notice) |
| Owner-only screens and routes | [016 ui-design, Guest degradation](../016-single-account-auth/ui-design.md#guest-degradation), [016 auth-api, Three access classes](../016-single-account-auth/contracts/auth-api.md#three-access-classes) |
| Adding a dependency | [dependency-updates.md](../../docs/how-to/dependency-updates.md) |

This file records only the decisions this feature adds.

## R-1: Two tables; items keyed like favorites, with no foreign key to the library

**Decision**: Add `playlists` and `playlist_items`
([data-model.md, Migration](data-model.md#migration)). An item holds either a
video's user key or a group's folder key, the same keys `video_favorites` and
`folder_favorites` use, and has no foreign key to `videos` or `folder_groups`.
`playlist_items` references `playlists` with `on delete cascade`, so deleting
a playlist removes its items and nothing else.

| Option | Verdict |
| --- | --- |
| **Own tables, user key and folder key** | Chosen |
| Item keyed by `videos.id` or `folder_groups.id` | Rejected: a video row goes when its last location goes, and `rebuildFolderIndex` replaces every group id, so items would vanish on a rescan |
| One table per item kind | Rejected: the order runs across videos and groups in one sequence, so one `position` column has to cover both |

**Rationale**: The Edge Case "the file is gone / the group is dissolved or
renamed, and comes back" is the property favorites already have through these
keys: an item whose key resolves to nothing is not shown, and it is shown at
its old position again once the key resolves (R-4).

## R-2: Bundling and succession re-key every member's items and merge duplicates

**Decision**: Same-path succession moves the `from` key's items to the `to`
key. Bundling moves every member's items to the bundle key; no row stays under
a member's content key. Removing the last-but-one member moves the bundle
key's items to the remaining member; removing a member while others remain
leaves the items with the bundle. Whenever a move would put two items with
one key in one playlist, the one at the earlier position stays and the
playlist is renumbered
([data-model.md, Succession and bundling](data-model.md#succession-and-bundling)).

| Option | Verdict |
| --- | --- |
| **Every member's items move to the bundle key** | Chosen |
| Add `playlist_items` to `userDataTables`, which copies the representative's rows only | Rejected: an item of a non-representative member would vanish from the playlist when it is bundled, against the Edge Case "carried over to the bundle" |
| Copy to the bundle key and keep the members' rows, as favorites do | Rejected: the hidden copies hold positions in the dense order (R-3), and an item the owner removed from the playlist would come back when its member leaves the bundle (requirement 5) |
| Keep the later position on a duplicate | Rejected: the earlier one is where the owner first placed the video, and it keeps the list from jumping on a background rescan |

**Rationale**: A favorite is one flag per bundle, so taking the
representative's is a choice between equal values. A playlist item is a place
in an order, and dropping a member's place loses something the owner put
there. A member taken out of a bundle that remains loses the item, because the
item is the bundle's from the moment of bundling.

## R-3: Dense integer positions, reordered by "move after"

**Decision**: `position` is 0, 1, 2, … within a playlist. Reordering is one
request that moves one item to just after another item, or to the top
(`POST /api/playlists/{id}/items/{itemId}/move`), and the store renumbers the
playlist in its transaction. New items go to the end.

| Option | Verdict |
| --- | --- |
| **Move one item after another** | Chosen |
| Replace the whole order with a list of item ids | Rejected: items hidden by R-4 are not on the screen, so the screen cannot place them, and a stale tab would undo another tab's move |
| Fractional positions without renumbering | Rejected: a playlist holds a hand-built number of items, so renumbering one playlist per move costs nothing, and integers never run out of precision |

**Rationale**: A move expressed relative to a visible neighbour keeps hidden
items next to the items they were next to, so they return at their old place.

## R-4: Hidden items are resolved at read time, never deleted

**Decision**: A video item is shown while a video in the library has its user
key; a group item is shown while a folder with its key is a group
(`folder_groups.path_key`). Reads leave out the rest; no path deletes them.
Numbers on screen ("3 / 7") count the shown items.

| Option | Verdict |
| --- | --- |
| **Leave out at read time** | Chosen |
| Delete items when content or a group leaves | Rejected: the Edge Case asks for the item to return at its position |
| Show them greyed out | Rejected: the Edge Case says the item is not shown |

## R-5: One owner-only bulk add for videos and groups, appended in request order

**Decision**: `POST /api/playlists/{id}/items` takes `videoIds` and `folders`
in the shape of `PUT /api/favorites`, resolves them the same way, skips what
does not resolve and what the playlist already holds, and appends the rest in
the request's order (videos, then folders). The response reports how many were
added and how many were already there
([contracts/screen-api.md, Add items](contracts/screen-api.md#post-apiplaylistsiditems)).

| Option | Verdict |
| --- | --- |
| **One request, skip unresolved and existing** | Chosen |
| `409` when an item is already in the playlist | Rejected: acceptance criterion 2 expects a repeated add to change nothing, and a bulk add from a selection would fail on one overlap |
| One request per item | Rejected: a selection of hundreds becomes hundreds of transactions with partial failure |

**Rationale**: The add sits next to the favorite toggle on the card, the
selection bar and the video page (requirement 2), which already send
`{videoIds, folders}`, so the screens build the request from the selection
they already have.

## R-6: The server answers the playlist position and the next video from the latest order

**Decision**: A playlist playback is the video page with `?playlist=<id>&item=<itemId>`.
The page reads `GET /api/playlists/{id}/playback?item=&video=`
([contracts/screen-api.md, Playback](contracts/screen-api.md#get-apiplaylistsidplayback))
when it opens, again when the video ends, before it shows the notice, and
once more when the notice finds its next video gone, so an item that became
unplayable is skipped (Edge Case). The
response gives the position among the shown items, the next video to play
continuously, and the start video of the previous and next items. The order
rules live in one `internal/domain` function
([data-model.md, Playback order](data-model.md#playback-order)).

| Option | Verdict |
| --- | --- |
| **Server read, refetched at the end** | Chosen |
| The page computes the next video from `GET /api/playlists/{id}` | Rejected: the page would need every group's members and watch states and the skip rules, which already live on the server for groups |
| Read once when the page opens | Rejected: the Edge Case "reordered in another screen during playback" asks for the latest order |
| Context in `location.state` | Rejected: a reload or a shared tab would drop out of the playlist; the query string survives both |

**Rationale**: The next step depends on the group's current members, their
watch states and which items are shown, all of which the store already reads
for one group (`GroupOpenIndex`).

## R-7: A playlist playback overrides the group and folder neighbours

**Decision**: While the page has a playlist context, the arrows, the notice and
the end-of-playback layer follow the playlist response, not `nextId` and
`prevId` of the related videos. Without the context, the page is unchanged
(requirement 8, acceptance criterion 8). A step that changes only `item`
restarts playback as a change of video does. When the playlist or the current item
is gone (`404`), the page drops the context and ends after the current video.

| Option | Verdict |
| --- | --- |
| **Playlist response replaces the neighbours** | Chosen |
| Group neighbours inside a group item, playlist neighbours at its edges | Rejected: two sources for one arrow; the playlist response already gives the next member inside a group item |
| Treat a step to the same video as already there (only `item` changes) | Rejected: a video item followed by a group item that starts at the same video (Edge Case "a group member also added alone") would leave the ended player on screen; the page restarts playback whenever `item` changes, as it does when the video id changes |
| On a removed current item, continue from its old position | Rejected: the position is gone with the item, and a guess can replay or skip items; the Edge Case stops on a deleted playlist the same way |

## R-8: Owner only through the existing rules

**Decision**: Every playlist route is owner only by the `/api/*` default, so
`accessRoutes` gains nothing. `/playlists` and `/playlists/{id}` join
`ownerOnlyPath` in the gate, the sidebar entry is `ownerOnly`, and the add
actions appear only where the favorite toggle does. The video page reads the
playback response only for the owner, so a guest who opens a public video with
`?playlist=` plays it as now.

| Option | Verdict |
| --- | --- |
| **Existing default and gate** | Chosen |
| A guest class for playlist reads | Rejected: requirement 10 and the out-of-scope sharing |

## R-9: Drag reordering with `@dnd-kit/core` and `@dnd-kit/sortable`

**Decision**: Add `@dnd-kit/core` and `@dnd-kit/sortable` to `web/` for the
playlist screen's reordering, with their keyboard sensor, through Renovate's
rules ([dependency-updates.md](../../docs/how-to/dependency-updates.md)). How
the handle and the drop look is decided by `ui-design.md`.

| Option | Verdict |
| --- | --- |
| **dnd-kit** | Chosen |
| Native HTML drag and drop | Rejected: no touch support on phones and no keyboard path |
| Move up and move down buttons only | Rejected: moving an item across a long list takes one press per place |

**Rationale**: dnd-kit works with pointer, touch and keyboard and announces
moves to screen readers, and it renders no markup of its own, so the rows stay
registry components.

## R-10: No domain event and no live update

**Decision**: Playlist writes publish no event. The playlist screens read on
open and after their own writes; the video page reads the playback response as
R-6 says.

| Option | Verdict |
| --- | --- |
| **No event** | Chosen |
| A `playlist` event on `/api/events` | Rejected: no requirement shows another tab's playlist change live, and the only cross-screen case is covered by the refetch at the end of a video |

## R-11: A playlist name is 1 to 100 characters after trimming; names may repeat

**Decision**: The store trims the name; empty or longer than 100 characters
(counted in Unicode code points, as SQLite's `length()` counts) is
`400 invalid_request`. Two playlists may share a name (Edge Case).
`domain.ValidatePlaylistName` holds the rule.

| Option | Verdict |
| --- | --- |
| **1 to 100, duplicates allowed** | Chosen |
| No length limit | Rejected: the name is a screen heading and a menu item; a limit keeps both from growing without bound |
| Unique names | Rejected: the Edge Case allows the same name |
