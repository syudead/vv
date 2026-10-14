# Research: Drag and drop to reorder, group, merge and bundle

Inherited decisions: groups, their items, nesting, dissolving and the `group`
event ([048 research](../048-groups/research.md)); version bundles and user
keys ([030 data-model, User key](../030-video-versions/data-model.md#user-key));
drag reordering with `@dnd-kit`
([044 R-9](../044-playlists/research.md#r-9-drag-reordering-with-dnd-kitcore-and-dnd-kitsortable)).
This file records only the decisions this feature adds.

## R-1: Undo restores the rows the operation changed, held by the server

**Decision**: Each drag operation reads, in its own transaction, the rows it
is about to change (the before image) and the same rows after the change (the
after image); undo writes the before image back when the current rows still
equal the after image, and fails whole otherwise.

| Option | Restores a merged or dissolved group with its id, favorites and playlist items | Detects a change made elsewhere | Verdict |
| --- | --- | --- | --- |
| **Row images, guarded by the after image** | Yes | Yes, exactly | Chosen |
| Inverse calls from the client (remove what was added, add what was removed) | No: a deleted group comes back with a new id and without its favorites and playlist items | No | Rejected |
| Compare `groups.updated_at` before undoing | Yes | Not for bundles, favorites or playlists; two writes in one millisecond look equal | Rejected |
| Persistent history table | Yes | Yes | Rejected: undo lives only as long as its toast (requirement 12, Edge Case), so nothing has to survive a restart |

**Rationale**: Merging (requirement 8), removing the last item and bundling
delete rows that no public route can recreate, so only the server can put them
back. Comparing the whole after image is what the Edge Case "changed in
another screen" asks for, and it covers every table the operation wrote.

## R-2: The undo journal lives in `internal/app`, in memory, for 10 minutes

**Decision**: `app.Organizer` keeps each operation's images under a random
token for 10 minutes, at most 100 entries, and forgets a token once it is
used; the client forgets it when the toast closes or the screen changes.

| Option | Verdict |
| --- | --- |
| **In memory in `internal/app`** | Chosen |
| In memory in `internal/store` | Rejected: the store is an adapter that runs transactions; a token's lifetime is a use-case rule |
| Send the images to the client and back | Rejected: a merge of a 20000-item group sends the whole group twice, and the server would have to validate images it did not write |

**Rationale**: The images are opaque bytes the store writes and reads, so the
application layer holds them without importing the store
([ARCHITECTURE.md, Intended dependency direction](../../ARCHITECTURE.md#intended-dependency-direction)).
The client's lifetime is the shorter one, so the server's 10 minutes only
bounds memory.

## R-3: One route per drag operation, each in one transaction

**Decision**: Reorder, move, group, merge and bundle are new routes under
`/api/organize/`, and each runs its whole change (a move's removal and
addition, a group's creation and its place in the parent) in one transaction
([contracts/screen-api.md](contracts/screen-api.md)).

| Option | Verdict |
| --- | --- |
| **New compound routes** | Chosen |
| Chain the existing group routes from the client | Rejected: a move would be a remove and an add, so a failure between them loses the item, and undo would need two journals |
| One route with an operation field | Rejected: the five bodies share nothing but the token in the answer, and the generated client would lose their types |

**Rationale**: The Edge Case "a save failed" asks for the state before the
operation, which only one transaction gives.

## R-4: Dragged cards are named by video and group id, in screen order

**Decision**: Every operation names what is dragged as `{kind, id}` with a
video id or a group id, in the order the cards appear on screen, and names
the group they were dragged out of as `from`.

| Option | Verdict |
| --- | --- |
| **Video and group ids plus `from`** | Chosen |
| Group item ids on the group page, video and group ids elsewhere | Rejected: two shapes for one operation, and the library and folder screen have no item ids |

**Rationale**: The library and folder screens know only video and group
ids; `from` lets the server find the items in that group by user key and
group id, as add does
([048 data-model, Edits](../048-groups/data-model.md#edits)).
Screen order keeps requirement 11's relative order without a second field.

## R-5: A merge hands the dropped group's favorites, playlist items and parents to the target

**Decision**: Merging X into Y appends X's items to Y in order, makes Y a
favorite when X was one, replaces X with Y in every playlist and every parent
group at X's position, and then deletes X; where Y is already there, or where
Y would become its own ancestor, X's entry is removed instead.

| Option | Verdict |
| --- | --- |
| **Parents keep their place with Y** | Chosen |
| Parents lose X, as dissolving does | Rejected: requirement 8 replaces X with Y in favorites and playlists; a parent that held X losing it would be the one place the merge empties |

**Rationale**: The parent group is the same kind of reference as a favorite
or a playlist item, so it follows requirement 8's rule. A merged automatic
group writes `ungroup` for its folder, as dissolving does
([048 R-4](../048-groups/research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup)).

## R-6: "Make a group" puts the new group where the target was

**Decision**: The new group holds the target video first and the dragged
videos after it in screen order, is named by the target's title cut to 100
characters, and on the group page takes the target's item position while the
target and the dragged items leave that group.

| Option | Verdict |
| --- | --- |
| **Target first, at the target's place** | Chosen |
| Dragged videos first | Rejected: the target card is the one that stays in place, so the group reads as "the target, and what joined it" |
| Append the new group at the end of the parent | Rejected: requirement 5 places it at the original position |

**Rationale**: In the library and on the folder screen the new group has no
parent and appears among the library's top groups
([048 R-6](../048-groups/research.md#r-6-the-library-top-level-and-partial-search-matches-over-descendants)).

## R-7: One drag layer on `@dnd-kit` for cards and rows, over and between

**Decision**: A shared drag layer in `web/src/videoList` wraps the existing
`@dnd-kit` packages for the grid and the list: the pointer over a card's
middle drops onto it, near its leading or trailing edge drops between cards
where reordering is allowed, and a `DragOverlay` shows the dragged card with
the selection count.

| Option | Verdict |
| --- | --- |
| **Extend the `@dnd-kit` setup of `videoList/reorder.tsx`** | Chosen |
| Native HTML drag and drop | Rejected: no touch events on phones (requirement 14) |
| A second library for grid drag | Rejected: `@dnd-kit` already supports grids with a custom collision rule, and two drag systems on one screen would fight over the pointer |

**Rationale**: The screens are not virtualized, so every card is a mounted
drop target. The sensors are those of `useReorderSensors`, except that a card
needs 8 px of pointer travel instead of 4 so a click or a hover scrub never
starts a drag (`UI品質`); touch keeps the 200 ms press, which leaves a swipe
to scrolling. Keyboard users keep the list view's handle for reordering and
the 048 menus for everything else. The look of the indicators is decided by
`ui-design.md`.

## R-8: The screen applies an operation at once and puts it back on failure

**Decision**: The screen shows the result of a drop before the server
answers; a failure restores what it showed before the drop and says the
operation failed, and success shows the undo toast.

| Option | Verdict |
| --- | --- |
| **Apply at once, restore on failure** | Chosen |
| Wait for the server, then show the result | Rejected: the card would jump back to its old place for the length of a round trip |

**Rationale**: The group page's reorder already works this way
(`groups/useGroupEdits.ts`), and the `group` event reads the server's state
afterwards
([048 R-13](../048-groups/research.md#r-13-a-group-event-tells-open-screens-to-read-again)).

## R-9: The undo toast is the existing toast with an action

**Decision**: `useToast` in `ui/Toast.tsx` gains an optional action (label
and handler) and a duration for it, and the undo toast uses them.

| Option | Verdict |
| --- | --- |
| **An action on the existing toast** | Chosen |
| A separate undo bar | Rejected: a second notification surface would compete with the toast stack the shell already places by width |
