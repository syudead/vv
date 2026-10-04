# UI design: Favorite videos and groups

**Feature**: [parent Issue #574](https://github.com/syudead/vv/issues/574) ·
[plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) ·
[research.md R-2](research.md#r-2-one-owner-only-put-apifavorites-for-videos-and-folders-in-one-transaction) ·
[R-6](research.md#r-6-no-domain-event-screens-reuse-the-visibility-subscription-pattern) ·
[R-7](research.md#r-7-selection-keeps-chosen-groups-as-groups)

The visual rules come from the following documents and are not decided again here.

| Topic | Source |
| --- | --- |
| Colours, interaction states, width breakpoints, list and video page layout | [Library UI](../../docs/design-docs/library-ui.md) ([List layout](../../docs/design-docs/library-ui.md#list-layout) for cards, group cards, list view, selection bar and toolbar; [Video page layout](../../docs/design-docs/library-ui.md#video-page-layout) for the video page) |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css). Refer to them by name; do not copy values |
| Contrast pairs under test | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| Card box and how the selection checkbox appears (hover, focus, `hover:none` devices) | The current [`web/src/videoList/VideoCard.tsx`](../../web/src/videoList/VideoCard.tsx) and [`web/src/library/GroupCard.tsx`](../../web/src/library/GroupCard.tsx), [specs/017-folder-groups/ui-design.md "Group card"](../017-folder-groups/ui-design.md#group-card) |
| Precedent for a mark on cards (the public mark) | [specs/016-single-account-auth/ui-design.md "Visibility toggle", "Card"](../016-single-account-auth/ui-design.md#card) |
| Video page facts row and its right-hand group of secondary actions | [specs/012-video-detail-ia/ui-design.md "Video facts"](../012-video-detail-ia/ui-design.md#video-facts), [specs/029-video-overrides/ui-design.md "Capture button"](../029-video-overrides/ui-design.md#capture-button) and the current [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx) |
| Selection bar layout and the menu precedent ("Visibility") | [specs/016-single-account-auth/ui-design.md "Selection bar"](../016-single-account-auth/ui-design.md#selection-bar), [specs/030-video-versions/ui-design.md "Bundle action"](../030-video-versions/ui-design.md#bundle-action) and the current [`web/src/library/SelectionBar.tsx`](../../web/src/library/SelectionBar.tsx), [`VisibilityMenu.tsx`](../../web/src/library/VisibilityMenu.tsx) |
| Filter popover and sort menu | [specs/013-library-search/ui-design.md "Sort and direction", "Filter menu"](../013-library-search/ui-design.md#sort-and-direction), [specs/033-video-dates/ui-design.md "Sort and direction"](../033-video-dates/ui-design.md#sort-and-direction) and the current [`web/src/videoList/FilterMenu.tsx`](../../web/src/videoList/FilterMenu.tsx), [`SortControls.tsx`](../../web/src/videoList/SortControls.tsx), [`listCriteria.ts`](../../web/src/videoList/listCriteria.ts) |
| Guest degradation | [specs/016-single-account-auth/ui-design.md "Guest degradation"](../016-single-account-auth/ui-design.md#guest-degradation) |
| Where screen text lives and its format | [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The English here is a proposal showing intent; after implementation the catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth |

This feature adds five things to the screen, all shown to the owner only. The only new tokens are the colour of
the filled heart, `favorite`, and the shadow of marks on thumbnails, `drop-shadow-mark` (see "Mark" and
"Colour" below). No radius token is added.

1. The **toggle on cards and rows** in the library and folder screens (video cards and rows, group cards and
   rows; requirements 6 and 7, acceptance criteria 1 and 3)
2. The **toggle in the secondary actions row** of the video page (requirements 6 and 7, acceptance criteria 1
   and 6)
3. The **"Favorite" menu** in the selection bar (requirement 6, acceptance criterion 7)
4. **"Favorites only"** in the filter (requirements 8, 9 and 11, acceptance criteria 4, 5 and 9)
5. **"Date favorited"** in the sort (requirements 10 and 11, acceptance criterion 8)

Unchanged: plain folder cards (`FolderCard`; out of scope); the group line on the video page ("Group line";
the video page entry point of requirement 6 covers only the favorite of the **video being played**, and group
favorites are set from the library's group cards and from multiple selection); the related-video and member
rows; the end-of-playback layer; the sidebar (no favorites-only screen or entry point; `UI品質`:
`要求を満たしたことにならない変更`). The guest screens do not change anywhere ("Guest degradation").

## Words

The English text is chosen only to avoid clashing with words the same screen already uses.

| Place | English (proposal) |
| --- | --- |
| Accessible name of the toggle (card and row, video) | Favorite "{title}" |
| Accessible name of the toggle (card and row, group) | Favorite group "{name}" |
| Accessible name and tooltip of the video page toggle | Favorite |
| Toggle state | `aria-pressed` (true means favorite) |
| Failure line on the video page | Couldn't change the favorite: {reason} |
| Failure toast on cards and rows | Couldn't change the favorite: {reason} |
| Selection bar trigger | Favorite |
| Selection bar menu items | Add to favorites / Remove from favorites |
| Toast after adding in bulk | Added {N} items to favorites (N is `appliedVideos + appliedFolders`; "item" when 1) |
| Toast after removing in bulk | Removed {N} items from favorites |
| Toast when a bulk change fails | Couldn't change the favorites: {reason} |
| Reason for the bulk limit | The current `too_many_videos` sentence (the same as tags and visibility). The count is the total of `videoIds` and `folders` sent |
| Filter checkbox | Favorites only |
| Sort kind | Date favorited |
| Sort direction wording | oldest first / newest first |

- "Favorite" is used as both verb and noun; "Like", "Save" and "Star" are not used. "Star" reads as a graded
  rating (out of scope), "Save" reads as a download, and "Like" reads as a reaction others can see. A favorite
  is a mark only the owner has, with only on and off (requirements 1 and 12).
- The unit in bulk toasts is **items**, not videos. A group is favorited as one item (requirement 6), so
  calling `appliedVideos + appliedFolders` videos would contradict the selected count (which counts members).
  The library's count line also counts items as items.
- The sort is "Date favorited", not "Recently favorited". The parent Issue's term is `お気に入りにした日時`,
  and the "Date …" form matches "Date added", "Date modified" and "Date created" in the menu, so the direction
  wording (oldest first / newest first) is the same as for those three. "Recently played" says "Recently"
  because playback happens many times and the sort has to say "last played"; a favorite has the one time it was
  made.

## Mark

The mark is one lucide `Heart`: filled (`fill-current`) when on, outline only when off. Every entry point uses
the same icon and the same fill rule, and the toggle itself shows the current state (requirement 7).

- **A heart, not a star**, so that the icon says there is no graded rating (requirement 1, out of scope
  "rating"). A star is established as the icon of a five-step rating, and even a single star suggests a score.
  The "favorite" of photo apps and media servers has converged on the heart.
- **The on colour is the favorites-only pink token `favorite`** (`text-favorite` + `fill-current`), the same on
  cards, rows and the video page. The maintainer set the value to `#ff6f9c` (it is recorded here because the
  token was not yet in `index.css`). The implementation PR puts it once in `@theme` of
  [`web/src/index.css`](../../web/src/index.css) as `--color-favorite`, and from then on that is the source of
  the value (the design.md rule). The value must meet three conditions, checked by adding pairs to `pairs` in
  `tokens.test.ts` in the implementation PR ("Colour"):
  - 4.5 or more on dark surfaces: `favorite` on `surface` (rows), on `accent-soft` (the `active` surface on the
    video page), on `navbar` (the surface of a card without a thumbnail), and on `bg`.
  - Distinguishable from `danger`. `danger` is a vermilion red (toward orange); `favorite` is a pink shifted
    toward purple. A value that looks like the same red family (a hue close to `danger`) is not used.
  - Read as "favorite" at a glance. The fill of favorites in photo apps and media servers has converged on red
    to pink, and users read the meaning from that colour.
- **The earlier `text-link` (the pressed and selected state colour) is not used.** The first proposal used the
  cyan `link` and let the reason for avoiding red (the clash with `danger`) decide the colour. On a real device
  the cyan heart belonged to the same family as the selection checkbox and the focus outline, blended into the
  "selected" and "focused" marks, and was recognisable as a favorite only from the heart shape. The favorite
  mark should read through both shape and colour, so its colour is one used only by this feature's mark.
  `danger` is still not used, for the earlier reason: red is the semantic colour for danger and failure, and
  on a card it would compete with the "can't play" warning (library-ui.md, [Visual values in one CSS location, with contrast guaranteed by tests](../../docs/design-docs/library-ui.md#visual-values-in-one-css-location-with-contrast-guaranteed-by-tests)). Hence a pink outside the
  `danger` family, added as its own token.
- **Off is an outline-only white heart** (`text-fg`), with a dark shadow (`drop-shadow-mark`) on cards; on rows
  and the video page it is `text-fg-muted` with neither shadow nor surface. So that cards without a favorite are
  not noisier than today, the off mark appears only on pointer hover or focus (see "Card" below).
- `aria-pressed` carries the state in addition to the fill. The state does not rely on the colour difference
  (`favorite` versus `fg`) alone; the fill tells them apart.

## Card

The toggle goes on the library grid (`VideoCard`, `GroupCard`) and on video cards in the folder screen and its
search results, for the owner only.

### Placement

- At the **top right** of the thumbnail frame (`top-1.5 right-1.5`), paired with the selection checkbox (top
  left). The hit area `size-7` is larger than the checkbox (`size-5`, `top-2 left-2`), so the inset is 2px
  tighter, which puts the outer edge of the heart's stroke at about the same distance from the corner as the
  checkbox box's outer edge (about 9px and 8px). Video cards use `z-20` (the same as the checkbox); group cards
  use `z-30` (the same as their checkbox, above the raised thumbnail).
- The component is a `button` (`type="button"`, `aria-pressed`) placed **outside** the card link (`Link`),
  inside the same `article` (like the checkbox; no button inside a link). DOM order is checkbox → link →
  **toggle** → tag row. Tab still reaches the card link (open) first, and the favorite next (`UI品質`:
  `操作の優先順位`).
- The hit area is a `size-7` (28px) square, and the heart inside is `size-5.5` (22px, lucide's default stroke
  width 2). **No surface and no border, only the heart** sits on the thumbnail, and both the outline and the
  filled heart get the dark shadow `drop-shadow-mark` so they read on bright thumbnails. The shadow is added to
  `@theme` as `--drop-shadow-mark` (its value lives in `index.css`; translucent black, outside `pairs`). It is a
  step larger than the checkbox (`size-5`) because the checkbox is mostly pressed in selection mode, while this
  is pressed in the everyday list.
- **The earlier `size-6 rounded-sm bg-navbar/90 backdrop-blur-sm` surface is not used.** The first proposal
  wrapped the heart in the same surface as the duration mark, guaranteeing that the stroke reads on any
  thumbnail. On a real device that surface was one more dark square in a corner, like the duration and public
  marks; a card that was on became "a small heart inside a square", and the favorite mark ended up with the same
  weight as the duration mark (`UI品質`: `視覚的階層`, noticeable at a glance when on). Without the surface and
  readable through the shadow, a card that is on has only a pink heart in the corner, more noticeable than the
  duration mark and less than the title. The off heart is a white outline with a shadow and appears only on
  hover, so it is not noisy without a surface. The heart grows from 16px to 22px because without a surface a
  16px outline heart looks thinner than the duration mark's text and relies entirely on the shadow on bright
  thumbnails.
- No row is added, and the position and spacing of the title, tag row, duration mark, public mark and progress
  bar do not change (`UI品質`: `情報密度`, `余白のリズム`). The public mark (`Globe`, inside the duration surface
  at the bottom right) is in a different corner, so they do not overlap.
- **Rejected placements**:

  | Placement | Why rejected |
  | --- | --- |
  | Inside the duration mark (where the public mark is) | It is inside the link, so it cannot be a pressable mark and fails requirement 7 (state and action in the same place) |
  | Right end of the title row | It competes with two-line titles for width, and more cards would truncate the title to one line |
  | A menu of actions shown on hover | It adds one step before favoriting, which is "a selection screen every time you favorite" |

### When the mark is shown

| State | Grid card | List view row |
| --- | --- | --- |
| On | Always visible (`opacity-100`) | Always visible |
| Off | `opacity-0`; `opacity-100` when the card is hovered, focus is inside the card (`group-focus-within`), or on `hover:none` devices (the same conditions as the checkbox) | Visible when the row is hovered, focus is inside the row, or on `hover:none` devices |

- The off heart is not always shown because of `UI品質`: `付いていないときは、カードの見た目を今より騒がしくしない`. Only
  cards that are favorites have a heart, so scanning the list shows the favorites (`UI品質`: `視覚的階層`).
- In selection mode (one or more selected) it appears under the same conditions and can be pressed (the plan's
  acceptance). It is not always shown like the checkbox because the checkbox alone is enough as the mark of
  selection mode.
- Relation to the hover preview (video and folder artwork) is the same as the checkbox: when the pointer enters
  this button the preview stops (treated like `data-preview-checkbox`; a preview does not start from over the
  button). There is no surface, so nothing needs to change during the preview; the heart sits with its shadow
  over the moving image.
- It also appears over the full-card warning of an unplayable video (`bg-overlay`) (`z-20`, in front of the
  warning). A favorite has nothing to do with playability.

### Pressing

- Pressing sends one `PUT /api/favorites`. Video cards and rows send `videoIds: [id]`, group cards and rows
  send `folders: [group.folder]`, and `favorite` is the opposite of the current state. No confirmation dialog
  (`UI品質`).
- Pressing does not trigger the card link (open) or the selection checkbox. The `click` stops propagating, and
  in selection mode it does not toggle the selection.
- While sending, the button is `aria-disabled` and the heart becomes `LoaderCircle` (`animate-spin`, stopped
  under reduced motion). Pressing again sends nothing.
- The mark changes after the response. Video cards and rows replace the list item's `favorite` in place; group
  cards and rows are refetched with `GET /api/folders/{rootId}/group` and replaced (R-6, 017 "Refresh and
  removal"). The card's look does not change during the refetch. When `appliedFolders` is 0 (no longer a
  group) and the refetch returns 404, the card is removed from the list (without notice, as in 017).
- On failure the toast "Couldn't change the favorite: {reason}" appears and the mark does not change. A card has
  no room for a failure line.
- Favoriting a group does not change the marks on its member video cards, and favoriting a member does not
  change the group card's mark (requirement 4, acceptance criterion 3).
- Unfavoriting a card **in a favorites-only list** does not remove it from the list at once; the heart only
  becomes an outline. It disappears when the list is refetched (conditions change, the page reloads) (R-6).
  Removing it at once would shift the neighbouring cards and stop repeated presses. Toggling in a list sorted by
  "Date favorited" does not reorder it at once either, for the same reason.
- Returning from the video page restores the list snapshot (`listSnapshot`) instead of refetching, so that
  card's mark in the snapshot is in the state set on the video page (acceptance criterion 1, applied to
  `listSnapshot`). After opening a video from a favorites-only list, unfavoriting it and going back, the card
  stays in the same position with an outline heart until a refetch (the Acceptance of the plan's "Put the
  favorite toggle on the video page"). A state changed in another tab is picked up on the next list refetch
  (Edge Case).

### List view row

In the list view (library only), the video row (`VideoRow`) and the group row (`GroupRow`) gain a `w-8` column
**right after** the title column, holding the same `button` (`size-6`, heart `size-4`). Neither surface nor
shadow (the row surface is `surface`, not a thumbnail). Off is `text-fg-muted`, `text-fg` on hover; on is the
`text-favorite` fill (the same colour as cards; "Mark"). The visibility conditions are those in the table above.
Unlike the row's checkbox (always shown faintly), the off state is hidden: two faint marks on one row make it
hard to read which is the selection. The row's size and placement do not change in this revision. The card's
heart grows to 22px in exchange for dropping the surface over the thumbnail; a row has neither surface nor
thumbnail, and the 16px heart in the `size-6` column sits at the same height as the title text.

## Video page

On the video page (`/videos/:id`), the toggle goes **first** in the group of actions at the right end of the
facts row (`actionsRef` in `VideoFacts`, `ml-auto`), to the left of "Use current frame as thumbnail" (`UI品質`:
`操作の優先順位`: after playback → title → tags → visibility toggle, at the same level as secondary actions such
as opening the file).

- The component is `IconButton` (`size="sm"`, ghost, lucide `Heart`) with `aria-pressed`. Off is
  `text-fg-muted` like the other two, `text-fg` on hover. On keeps the `IconButton` `active` surface
  (`bg-accent-soft`) and only colours and fills the heart with `text-favorite` (the same colour as cards and
  rows; "Mark"). The `active` text colour `text-link` is not used on this button. `data-active:text-link` on
  `IconButton` is more specific than a plain `text-favorite`, so it is overridden with `text-favorite!`, as the
  off state uses `text-fg-muted!` (with the plain class the heart stays cyan). The surface stays so that the
  "pressed" state looks the same as the other `IconButton`s in the group; the colour matches for requirement 7
  (the same icon and fill at every entry point). The accessible name and tooltip are "Favorite". Placement and
  size do not change in this revision.
- It goes first because it is the only stateful action in the group, and at the first position the eye reaches
  moving right from the facts row the state is readable. The "first" in 029 "Capture button" remains first among
  its three actions (capture, open, copy); this document adds one more to their left.
- For the owner, the group is shown with only this button even for a video with no location or no player (as
  029 already does with the capture button).
- Pressing sends `PUT /api/favorites` (`videoIds: [id]`). While sending, the button is `aria-disabled` and the
  icon becomes `LoaderCircle` (spinning). On `200`, the video is refetched with `GET /api/videos/{id}` (R-6,
  plan), and the refetched `favorite` changes the fill and `aria-pressed`. During the refetch and on failure the
  previous state stays (as in 033 "Refresh after edits"). No toast: the change of fill is the result.
- On failure, "Couldn't change the favorite: {reason}" appears in the line right below the facts row (the
  `role="alert"` line shared with capture and open). It clears on the next action or when moving to another
  video.
- A state changed on a list card reaches the video on the video page through the `useVideoDetail`
  subscription (other tabs on their next refetch).
- A watched video keeps its mark, and playback starts from the beginning (requirement 5, acceptance criterion
  6). The end-of-playback layer and the next-up notice do not change.
- For a video with `Video.group` (a group member), this toggle favorites only that video (requirement 6). The
  group line ("Group line") has no favorite.

## Selection bar

"Favorite" (lucide `Heart` + text + `ChevronDown`, `Button` ghost `sm`) goes **right after** "Remove tag",
before "Visibility". The order is count → Add tag → Remove tag → **Favorite** → Visibility → (with two or more)
Bundle as versions → divider → Select all → clear.

- Pressing opens `ui/Menu` upward (`side="top"`, like "Visibility") with two items: "Add to favorites"
  (`Heart`) and "Remove from favorites" (lucide `HeartOff`). Like "Visibility", it is not two buttons, because
  at `sm` and up the single row would hold five or six text buttons, and the group before the "Select all"
  divider would become too long and catch the eye first. It is not one toggle button, because with "Select all"
  including unloaded pages the current state is unknown and the button's fill cannot be decided (the same
  reason as 016).
- It sits right after the two tag actions because favorites and tags both "mark" items, while visibility
  "changes who can see". With one more action before visibility, "Visibility" and "Bundle as versions" move
  right.
- The current state of the selected items is not shown. Both items are always enabled, and items already in the
  state are not errors (Edge Case, [contracts/screen-api.md, `PUT /api/favorites`](contracts/screen-api.md#put-apifavorites)).
- Confirming sends one `PUT /api/favorites`. `folders` is the chosen groups (chosen by a group card's
  checkbox, or `groups` in the "Select all" response), and `videoIds` is the selected ids that are not members
  of a chosen group (R-7). Deselecting even one member of a group sends the remaining members as videos instead
  of the group (the same idea as "selected only while every member is in the selection" in 017 "Pressing and
  selection").
- The toast "Added N items to favorites" or "Removed N items from favorites" (N is
  `appliedVideos + appliedFolders`) appears and the selection stays. Card marks change after the response
  (videos in place, groups by refetch). A failure shows the toast "Couldn't change the favorites: {reason}" and
  keeps the selection.
- The limit (20,000) is checked against **the number sent** (the total of `videoIds` and `folders`). When it
  is exceeded, the trigger is `disabled` with the same reason as tags and visibility. The selected count
  `count` (which counts members) is not used. A single group with more than 20,000 members stays disabled for
  tags and visibility, while favorites can be pressed as one entry in `folders` (plan).
- "Select all" is disabled when the selection equals the set `ids` in the response and the chosen groups equal
  `groups` in the response (plan). After deselecting one member and selecting it again, the ids match but the
  group is not chosen as a group, so it can be pressed again.

### Layout

- **`sm` and up**: the current single row gains "Favorite". Where it fits, it stays one row as wide as its
  content. When the row does not fit the screen width (inside `px-4`), it does not overflow the screen under
  the current `nowrap`; the bar takes the full screen width and wraps to a second row in this order:
  1. Move "Select all" and clear, after the divider, to the **right end of the second row**. The first row
     holds the count and the actions.
  2. When the first row still does not fit (640–767px with a count like "12 videos selected" and five actions
     including "Bundle as versions" for two or more selected; five text buttons and their gaps alone exceed the
     first row), split the actions at the same break as below `sm`: keep "Add tag" and "Remove tag" on the first
     row with the count, and put "Favorite" onward (Favorite → Visibility → Bundle as versions) in order on the
     second row, before the divider, "Select all" and clear. The second row is aligned right.

  When step 1 is enough, the actions are not split, because everything before the divider is the group of
  "what to do with the selection" (library-ui.md, [List layout](../../docs/design-docs/library-ui.md#list-layout)). When step 2 splits them, it only splits into the two tag
  actions and the group from "Favorite" onward, so the order reads the same as on one row. Action names are not
  shortened (library-ui.md, [List layout](../../docs/design-docs/library-ui.md#list-layout)). Which step is needed depends on the count and the real widths of the actions;
  check it on a real device at the judged widths (640 and 768px). The five actions including "Bundle as
  versions" already do not fit one row at 768px, so this rule fixes that too.
- **Below `sm`**: the top tier is unchanged (count, Select all, clear). The bottom tier divides the width
  evenly among "Add tag", "Remove tag", "Favorite" and "Visibility" (five with "Bundle as versions" for two or
  more). Where they do not fit, "Add tag" and "Remove tag" stay on that tier and "Favorite" onward moves in
  order to the right end of the next tier; if that does not fit either, it moves to the right end of the tier
  after. The container query widths of `SelectionBar` change to fit four and five (four: the widest one's width
  × 4 + gap × 3; five likewise). No width below `sm` fits five on one tier, so two or more selected always gives
  three or more tiers.
- At 360px (iPhone SE) and 390px: count tier → "Add tag" and "Remove tag" tier → a tier with "Favorite" and
  "Visibility" at the right end. With two or more, "Bundle as versions" is at the right end of the tier below.
- Action names are not shortened (library-ui.md, [List layout](../../docs/design-docs/library-ui.md#list-layout)). "Favorite" is not reduced to an icon.

## Filter menu

The filter popover (`FilterMenu`) gains a "Favorites only" checkbox, for the owner only.

- It goes **right below** the watch state `fieldset`, above "Playable only". It has the same form as "Playable
  only" (`label` + `input[type=checkbox]`, `size-4 accent-accent`, `text-sm text-fg`), without an icon. It sits
  below watch state because both are the owner's own marks (watched, favorited), while playability is a
  property of the file. The two checkboxes are separated by `gap-2`, tighter than the `mb-4` below watch state
  (two lines of the same kind).
- Checking it fetches the list with `favorite=true` and adds `fav=1` to the URL. It counts in the button's
  badge, and the button becomes `bg-accent-soft text-link` (the current rule). "Clear filters" clears it with
  watch state and playability. The sort stays.
- It combines with search terms, tags, watch state and playability by AND (requirements 8 and 9). With no match
  the current "No videos match these conditions" stays; no favorites-specific text is added. Its hint "change
  the filters" applies.
- In a favorites-only list, a favorite video in a group that is not a favorite appears as a video card
  (requirement 9, acceptance criterion 4). It looks the same as a member shown alone in search, with no
  distinguishing mark.
- The folder screen uses the same popover, so the same checkbox appears (requirement 11). The folder screen has
  no groups and filters by video favorites only.
- Not shown to guests ("Guest degradation").

## Sort and direction

The sort kinds go from eight to **nine**. The library and folder screens share `SortControls`, so both get it
(requirement 11). The names, icons, directions and wording of the existing eight do not change.

| Kind | Name | Icon | Direction when chosen | Ascending / descending wording |
| --- | --- | --- | --- | --- |
| Date added | Date added | `CalendarArrowDown` | Descending | oldest first / newest first |
| Date modified (file) | Date modified | `CalendarClock` | Descending | oldest first / newest first |
| Date created | Date created | `FileClock` | Descending | oldest first / newest first |
| Title | Title | `ArrowDownAZ` | Ascending | — |
| Length | Length | `Timer` | Descending | shortest first / longest first |
| File size | File size | `HardDrive` | Descending | smallest first / largest first |
| Recently played | Recently played | `History` | Descending | least recently played / most recently played |
| **Date favorited** | **Date favorited** | **`CalendarHeart`** | Descending | oldest first / newest first |
| Random | Random | `Shuffle` | — | — |

- **Position**: right after "Recently played", before "Random". The two owner-only kinds (played, favorited)
  sit together, and the guest menu has the **seven** kinds without them, in the same order.
- **Icon**: `CalendarHeart`. The calendar family (added, modified) says it is a date, and the heart says it is
  the same thing as the mark on cards and the video page. It is not plain `Heart`, so it is not mistaken for
  the favorites-only filter inside the sort menu.
- Choosing it gives `favoritedDesc` (the last favorited first; acceptance criterion 8), and the direction toggle
  gives `favoritedAsc`. In either direction non-favorite items gather at the end (Edge Case, R-4). The
  accessible name and tooltip of the direction button keep the current form "Descending (newest first). Press
  for ascending". A group item is placed by the time the group was favorited.
- The round trip through device preferences (`viewPreferences`) and the URL (`sort=favoritedDesc`,
  `favoritedAsc`) is the same as for other kinds.
- In the combined "View and sort" panel below `md`, the two-column radios have five rows (nine kinds) for the
  owner and four rows (seven kinds) for guests. The menu order flows row-first into `grid-cols-2` without
  rearranging. For the owner, row 4 is "Recently played" and "Date favorited", and row 5 is "Random" (left only).
  For guests, row 4 is "Random" (left only). "Date favorited" fits one line in the two-column width and is not
  cut by `truncate`.
- The menu button shows only the kind name ("Date favorited"). Its width is about that of "Date modified", and
  it does not move other toolbar parts.

## Guest degradation

When rendering for a guest, the following are **not shown**. They are neither `disabled` nor `aria-disabled`
(as in 016).

| Place | Not shown | Instead |
| --- | --- | --- |
| Cards and rows (library, folder screen, search results) | The favorite mark and toggle | The response has no `favorite`. Like the public mark, it has no meaning for a guest |
| Video page | The toggle at the right end of the facts row | The response has neither `location` nor `favorite`, so the whole right-hand group is absent (as today) |
| Selection bar | — | Guests have no selection bar |
| Toolbar | "Favorites only" in the filter, "Date favorited" in the sort | Guests have seven sorts. When the URL still has `fav=1`, `sort=favoritedAsc` or `favoritedDesc`, the request uses the normalised defaults, like `watch` and `played*`, and the URL is corrected (added to the table in [guest-api.md, Conditions guests cannot use](../016-single-account-auth/contracts/guest-api.md#conditions-guests-cannot-use)). A sort stored on the device as `favorited*` is normalised the same way, and the stored value is not rewritten |

## Responsive behaviour

Only Tailwind's default breakpoints are used, switched in CSS (library-ui.md, [Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception)). The judged widths are 360px,
640px, 768px and 1280px.

| Width | Card | Selection bar | Video page | Toolbar |
| --- | --- | --- | --- | --- |
| 360px (below `sm`) | Full-width single-column grid. The heart is at the thumbnail's top right, `top-1.5 right-1.5`, `size-7`, level with the checkbox (top left). On `hover:none` devices the white outline heart with its shadow shows on every card and does not overlap the duration or public mark | Three tiers (four with two or more). "Favorite" and "Visibility" sit at the right end, with no text cut | The actions at the right end of the facts row (four) wrap to the end of the row; no horizontal scroll | "Favorites only" is in the popover. The sort is 2 columns × 5 rows in the "View and sort" panel |
| 640px (`sm`) | Two columns | With one selected, the count and four actions on the first row, "Select all" and clear at the right end of the second. With two or more (five actions), the count, "Add tag" and "Remove tag" on the first row; "Favorite", "Visibility", "Bundle as versions", divider, "Select all" and clear at the right end of the second. Nothing overflows the screen | Same as above | Same as above |
| 768px (`md`) | Two to three columns. The mark size does not change | One row with one selected. With two or more ("Bundle as versions" present), "Select all" and clear at the right end of the second row, and if the first row still does not fit, "Favorite" onward on the second row too (Layout step 2). Nothing overflows the screen | The action group fits on one line in the left column | "Date favorited" fits on one line in the sort menu button, which stays joined to the direction button |
| 1280px (`lg`, two columns) | Four to five columns. At any zoom level the mark stays in the corner | Always one row | Same as above | Same as above |

## Review criteria

Judge on a real device (library-ui.md, [Layout verified by people, not machines](../../docs/design-docs/library-ui.md#layout-verified-by-people-not-machines)). "It exists" alone does not pass (Q-4).

1. **Visual hierarchy (cards)**: In the owner's library, a favorite card has only one filled heart in its
   corner, which does not catch the eye before the thumbnail and title. A card that is not a favorite looks no
   different from before this feature (when not pointed at). Scanning the screen is enough to count which cards
   are favorites (`UI品質`: `視覚的階層`). The filled heart is pink (`favorite`), not mistaken for the cyan of the
   selection checkbox or the focus outline, and not in the same family as the red (`danger`) of the "can't play"
   warning. The card corner has no square surface, only the heart. On a bright thumbnail (artwork near white),
   the white outline heart shown on hover keeps its outline through the shadow.
2. **Information density and spacing rhythm (cards)**: The card height, the spacing of the title and tag rows
   (`gap-1`, `pt-1`, `pb-3`), and the positions of the duration and public marks are unchanged. Adding the heart
   has not shortened the title by a line or squeezed the tag row. List view rows are the same height, and the
   title column is narrower by one column (`w-8`).
3. **Action priority (cards)**: Pressing the heart does not open the video page, and pressing it in selection
   mode does not change the selection. Tab reaches checkbox → card (open) → heart → tags. On a card that is not
   pointed at, reaching the card by keyboard reveals the heart. On a touch device (360px) the heart is visible
   without a pointer and one tap favorites. Favoriting involves no confirmation and no selection screen.
4. **State and action in the same place (requirement 7)**: On cards, rows and the video page, the heart that
   was pressed becomes filled and returns to an outline on the next press. A video favorited on the video page
   is filled on the library card after going back (acceptance criterion 1). Favoriting on a group card leaves
   the hearts of its member video cards (folder screen, search results) as outlines (acceptance criterion 3).
5. **Video page hierarchy**: The eye moves player → title → tags → visibility toggle → facts row, and the heart
   is first in the group with "Open file" and "Copy path", at the same size and the same muted weight. When on,
   it has only the small `bg-accent-soft` surface and the pink fill, and is less prominent than the title or the
   visibility toggle. The fill colour looks the same as the card heart. The failure line is the line right
   below the facts row, in the same place as other failures (open, thumbnail).
6. **Selection bar**: Selecting two videos and one group and choosing "Favorite" → "Add to favorites" fills the
   hearts on the three cards, leaves the group's member cards unchanged, and shows the toast "Added 3 items to
   favorites" (acceptance criterion 7). "Favorite" sits between "Remove tag" and "Visibility" and, at `sm` and
   up, reads as part of the group before the "Select all" divider. At 640px with two or more selected nothing
   overflows the screen, and "Favorite" onward on the second row reads as a continuation of the first row's
   actions. At 360px there are three tiers (four with two or more), and no tier cuts its text.
7. **Filter**: Checking "Favorites only" shows only favorite videos and favorite groups, and the favorite
   members of a group that is not a favorite appear as video cards (acceptance criteria 4 and 5). It counts in
   the filter button's badge, and combined with a tag filter only items matching both appear (acceptance
   criterion 9). "Clear filters" clears it and `fav=1` leaves the URL. The folder screen has the same checkbox
   (requirement 11).
8. **Sort**: The menu has "Date favorited" right after "Recently played", with an icon that looks different
   from the other eight. Choosing it puts the last favorited first and gathers non-favorite items at the end
   (acceptance criterion 8). Changing the direction keeps the end at the end. Reloading the page gives the same
   order with `sort=favoritedDesc`. In the panel below `md`, "Date favorited" is on the right of row 4.
9. **Guests**: Signing out and opening the same library, folder screen and video page shows no heart anywhere,
   no "Favorites only" in the filter, and seven sorts. Opening as a guest a URL the owner had at
   `?fav=1&sort=favoritedDesc` shows the default list and corrects the URL (acceptance criterion 10).
10. **Examples that do not meet the requirement** (`UI品質`): a confirmation dialog or menu appears when
    favoriting; cards gain a "favorite" text row or band; off hearts are always visible on every card and the
    grid looks like a scoreboard; the filled heart is red and looks like the same family as the "can't play"
    warning; the filled heart is cyan and looks like the same family as the selection or focus marks; a dark
    square surface remains in the card corner and looks like a second duration mark; the fill colour differs
    between cards, rows and the video page; the video page heart sits next to the title or the visibility
    toggle and catches the eye before playback; the favorites list is a separate screen from the library; an
    outline heart or a disabled checkbox remains on the guest screen.

## Colour

One colour token is added: `--color-favorite` (the colour of the filled heart; "Mark"). Its value is
`#ff6f9c`, set by the maintainer; the implementation PR puts it in `index.css`, which is then the source of the
value. One shadow token is added: `--drop-shadow-mark` (the shadow of marks on thumbnails; translucent black,
outside `pairs`).

The filled heart is `favorite` (on `accent-soft` on the video page, on `surface` in rows, on the thumbnail on
cards, and on `navbar` on a card without a thumbnail). The outline heart is `fg` (on the card thumbnail, with
shadow) and `fg-muted` (rows and the video page). `fg-muted` on `surface` is already in `pairs`. The
implementation PR adds the following four pairs to `pairs`, all 4.5 or more. The mark is not text, but
`favorite` has this mark as its only use and the fill colour itself carries the meaning "favorite", so it is
tested at the text threshold.

| Pair | Where |
| --- | --- |
| `favorite` on `surface` | List view rows |
| `favorite` on `accent-soft` | The `active` surface on the video page |
| `favorite` on `navbar` | The surface of a card without a thumbnail (not yet fetched, unreadable) |
| `favorite` on `bg` | The list surface (a pair on the same basis as the other colours in `pairs`) |

The thumbnail artwork itself cannot be tested, so on cards the shadow (`drop-shadow-mark`) keeps the mark
readable ("Card"). There is one dark palette only (library-ui.md, [Dark scheme only, without a light/dark switch](../../docs/design-docs/library-ui.md#dark-scheme-only-without-a-lightdark-switch)), and testing uses that palette.
`favorite` is a role name, so if a light palette is ever defined, it needs one more `favorite` value in that
palette and the same four pairs tested; the screens do not change.

## Accessibility

The parent Issue does not ask for screen reader or contrast design, so only names and states are decided (the
same scope as 029, 030 and 033).

- The toggle is a `button` with `aria-pressed`, and its accessible name follows the "Words" table (cards
  include the title or group name; the video page says "Favorite"). While sending it is `aria-disabled`.
- The selection bar's menu items are distinguished by their text. Toasts use the current toast mechanism
  (`role="status"`).
- The filter checkbox is named "Favorites only", and the sort radio "Date favorited". The direction button's
  accessible name keeps its current form.
