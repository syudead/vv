# UI design: Bundle versions of the same video

**Feature**: [parent Issue #572](https://github.com/syudead/vv/issues/572) ·
[plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) ·
[research.md R-8](research.md#r-8-bundles-are-addressed-by-video-id-new-routes-for-listing-versions-bundling-the-representative-removal-and-candidates) ·
[R-9](research.md#r-9-a-bundle-change-publishes-domainvideobundlechanged-mapped-to-the-screens-video-notification) ·
[R-11](research.md#r-11-when-the-bundles-playback-position-is-at-or-past-that-versions-duration-the-screen-plays-from-the-beginning)

Sources: the visual rules come from these documents and are not decided again here.

- Colours, interaction states, width breakpoints, library and player layout:
  [Library UI](../../docs/design-docs/library-ui.md) (the selection bar in §6, library layout, and §8, player
  screen layout)
- Role tokens: `@theme` in [`web/src/index.css`](../../web/src/index.css). Refer to them by name; do not copy
  values
- Pairs checked for contrast: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- Player screen columns, the two "Video facts" lines and their right-hand actions, the single failure line, the
  Esc exceptions: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) and the current
  [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx)
- Precedent for adding an image and an action to a facts-line item (the thumbnail item):
  [specs/029-video-overrides/ui-design.md "Thumbnail fact"](../029-video-overrides/ui-design.md#thumbnail-fact)
- Selection bar popover and failure line, management dialogs (`ModalFrame`) and row density:
  [specs/014-video-tags/ui-design.md "Selection bar" and "Tag management page"](../014-video-tags/ui-design.md#selection-bar)
- Selection bar menu and limits, what is hidden from guests:
  [specs/016-single-account-auth/ui-design.md "Selection bar" and "Guest degradation"](../016-single-account-auth/ui-design.md#selection-bar)
- Menu on the player's secondary line, row density in the related-videos column, `aria-current` on the current
  row: [specs/017-folder-groups/ui-design.md "Group line" and "Member list"](../017-folder-groups/ui-design.md#group-line)
- List rows on the settings screen (how locations are shortened, link rows):
  [specs/024-import-progress/ui-design.md "Issue List"](../024-import-progress/ui-design.md#issue-list)
- Where screen text lives and its format: [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md).
  The English in this document shows intent; after implementation the catalog
  [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth

This feature adds three things to the screens, none of which adds a colour, radius or shadow token:

1. On the player screen (`/videos/:id`), a **versions item** in the facts line and, when opened, the **versions
   list** (requirements 6, 7 and 12; acceptance criteria 7, 8, 9 and 12)
2. **"Bundle as versions"** in the library selection bar, with a **dialog** to pick the representative
   (requirement 11; acceptance criterion 6)
3. An owner-only **candidate screen** (`/duplicates`) with an entry in the sidebar (requirements 9 and 10;
   acceptance criteria 3, 4 and 5)

Library cards, list-view rows, folder cards, group cards and related-video rows do not change.
Non-representative versions appear in no list (requirement 5), and the representative's item looks the same as
any video's item today (`UI品質` "visual hierarchy" and "changes that do not meet the requirement"). Bundling is
not marked on cards.

## Why this shape

The versions entry point is an **item in the facts line**, not in the player's control bar, next to the title,
or in the related-videos column:

- The parent Issue calls it "one piece of the detail screen's information" and "secondary information alongside
  the file's details", and `UI品質` asks for "enough to see in one line that other versions exist and how many".
  An item in the facts line sits in the same tier as length, size and date added (`text-sm text-fg-muted`) as
  "3 versions", which meets this as it stands. 029's thumbnail item is the precedent for putting a state and an
  action in the same line.
- The control bar must fit in one line at 360px, and there is no room after quality, subtitles and speed
  (029 "Why here"). Inside the player it would carry the same weight as playback controls.
- The related-videos column is "what to watch next"; another file of the same video is neither related nor next.

Versions are listed in a **popover opened on press (`ui/Popover`)**, not an always-open panel or a tab, because
of `UI品質` "information density" (differences are listed only once the entry point is opened) and "changes that
do not meet the requirement" (an always-open large panel or tab). The comparable current products (version
switching on a media server's item screen) have also converged on a list opened on press.

Bundling from the selection bar is a **dialog (`ModalFrame`)**, not a popover or menu, because picking the
representative decides "whose tags, playback position and visibility become the bundle's", a choice that takes
effort to undo. As with tag merging (014 "Merge and delete"), the dialog shows the list to choose from and a
one-sentence result together. "Same video" on the candidate screen uses the same dialog, so the representative
is not chosen two different ways.

The candidate list is an **owner-only sidebar entry `/duplicates`**, not a section of the settings screen or a
library filter:

- Checking candidates and bundling them is "tidying the library", like organising tags, not an ingest setting.
  The settings screen holds media folders, conversion and API tokens, and is already long.
- A library filter shows one video per item and cannot "put two side by side and see the differences".
- Current photo and video management products (duplicate review screens) have converged on a dedicated sidebar
  entry.
- The sidebar shows no count badge. There is no route that returns only a count, and the shell would read every
  candidate each time it opens. The count is visible in the screen's heading line.

## Words

| Place | Text (proposal) |
| --- | --- |
| Facts-line item (count) | 3 versions |
| Item's accessible name and `title` | 3 versions of this video. Show versions |
| Versions list's accessible name | Versions |
| Representative marker in the list | Representative |
| Current-video marker in the list (visually hidden) | Now playing |
| Accessible name of each row's link | Play {title}, {resolution} {container} {size} |
| Row menu trigger | More actions for {title} |
| Menu items | Make representative / Remove from versions |
| List failure line | Couldn't change the versions: {reason} |
| Toast on removal | Removed "{title}" from the versions |
| Selection bar action | Bundle as versions |
| Dialog title | Bundle as versions |
| Dialog description | These {N} videos become versions of one video. Pick the one to show in the library. The library keeps that video's tags, position and visibility; the others' are set aside and come back if you remove them. |
| Dialog list's accessible name | Representative |
| Dialog row note (video already bundled) | Already {N} versions — all of them join |
| Dialog primary action | Bundle |
| Dialog failure line | Couldn't bundle: {reason} |
| Toast on bundling | Bundled {N} videos as versions of "{title}" |
| Sidebar entry and `document.title` | Duplicates |
| Candidate screen `h1` | Possible duplicates |
| Count line | 12 pairs / 1 pair / Showing 200 of 340 pairs |
| Pair heading line | Same length · similar frames |
| "Same video" | Same video… |
| "Different videos" | Different videos |
| Toast on dismissal | Marked as different videos. They won't be suggested again |
| Toast when the pair is gone | This pair is no longer a candidate |
| Empty state | No possible duplicates / When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself. |
| Load failure | Couldn't load the candidates / Retry |
| `too_few_videos` | Select at least two videos |
| `too_many_videos` | Too many videos selected (same form as the current limit message) |
| Reason the selection bar action is disabled (when `maxBundleSelection` is exceeded, not `maxVideoTagsSelection`) | Bundle up to 20 videos at a time |
| `representative_not_selected` | Pick which video to show in the library |
| `not_bundled` | This video isn't bundled with others |

- "Version" has one meaning (another file of the same video) on the video page, in the selection bar and in the
  dialog. Only the candidate screen uses "duplicates": what it lists are two not-yet-bundled videos that "might
  be duplicates", which is what users call them. Once decided they become "versions" too.
- The four reasons are added to the `reason` table in `web/src/i18n/errors.ts`
  ([contracts/screen-api.md §2 to §4](contracts/screen-api.md#2-post-apivideo-bundles)). `video_not_found` keeps
  its current text.

## Video page

### Versions fact

When `Video.versions` is present and `count` is 2 or more, one item is placed in the file facts line (the first
line of `VideoFacts`) **after** length, size and date added (for owners, before the thumbnail item). It is shown
to owners and guests (requirement 12).

- From left to right: lucide `Layers` (`size-4`, `text-fg-subtle`, `aria-hidden`) → "3 versions"
  (`tabular-nums`) → `ChevronDown` (`size-3.5`). `gap-1.5` inside the item; the gap to other items stays the
  current `gap-x-4` (`gap-x-5` from `sm`).
- The whole item is the `PopoverTrigger` `button`. Its text is `text-sm text-fg-muted` like the other items,
  `text-fg` on hover. It has no surface or border and no accent colour (`UI品質` "typography" and "visual
  hierarchy"). `-mx-1 px-1 rounded-sm` keeps the hover `bg-hover-wash` tight around the text. The accessible
  name is "3 versions of this video. Show versions", and `title` is the same.
- When `count` is 1 (every other version's file is gone), the item is not shown. There is nothing to switch to,
  and "1 version" means nothing. The bundle and its values remain, and the item returns when the files return
  (Edge Case "even if the representative's file disappears from disk").
- The other facts-line items, the right-hand actions, the technical-details line and the line spacing (`gap-3`)
  do not change (`UI品質` "spacing rhythm"). The added item wraps like the other items.

### Versions list

Pressing the item opens a `ui/Popover` below it (`side="bottom"`, `align="start"`). Its content, from top to
bottom:

- Width `w-[min(28rem,calc(100vw-2rem))]`. The surface is the current `PopoverContent` (`bg-elevated`,
  `shadow-elevated`).
- The list is a `ul` (accessible name "Versions") of rows with `divide-y divide-border`. Rows follow the response
  order (representative first, then natural title order;
  [contracts §1](contracts/screen-api.md#1-get-apivideosidversions)). Rows are `py-2`; beyond 6 rows,
  `max-h-80 overflow-y-auto` scrolls only the inside.
- Each row is `grid grid-cols-[1fr_auto] gap-x-2`, with two lines of text on the left and owner-only actions on
  the right.
  - Line 1: the title (`text-sm text-fg`, truncated to one line, full text in `title`). The representative's row
    adds "Representative" in `text-xs text-fg-muted` to the right of the title (`shrink-0`). It is marked by
    text, not by colour or weight.
  - Line 2: the differences (`text-xs text-fg-muted tabular-nums`, truncated to one line), in the order
    resolution → container → video codec → size → location, separated by "·" in `text-fg-subtle`. Unknown values
    (before probing) are omitted item by item. The location is "media folder display name / relative path" built
    from `folder`, keeping the end (the side nearest the file) when shortened (the same shortening as 024 "Issue
    List"). For owners the row's `title` holds the absolute path (`location.path`); for guests, only the relative
    location (016 "Guest degradation").
  - The left two lines together are a **`Link` to playback** (`/videos/{id}`) with the accessible name "Play
    {title}, {differences}". Hover gives the row `bg-hover-wash`. `state.from` passes the current screen's
    `backTo` unchanged, so the back destination does not change. When chosen during playback
    (`status.playing || status.ended`), `autoplay` is added so playback continues on the new page (as with the
    previous/next controls).
  - **The current video's row** is not a link: `aria-current="true"`, surface `bg-active-wash`, and
    `border-l-2 border-accent` on the left edge (the same as the current member in 017 "Member list"). A visually
    hidden "Now playing" precedes the title. It cannot be pressed, so its hover surface does not change.
  - **Owner actions** (right column): an `IconButton` (ghost, `sm`, lucide `Ellipsis`, `text-fg-muted`, `text-fg`
    on hover) opens a `ui/Menu` (accessible name "More actions for {title}"). Items from the top: "Make
    representative" (lucide `Star`; not shown on the representative's row), a separator, "Remove from versions"
    (lucide `Unlink`). Playback is the row itself, changing the representative is the first menu item, and
    removal is last below the separator; this order expresses `UI品質` "action priority" (play → representative →
    remove). Removal is not `text-danger`: it can be undone (bundle again) and is not a deletion.
  - **Guests** have no right column; rows are links only (requirement 12).
- Below the list there is a place for a single failure line (see "Failure" below).
- On open, focus goes to the first row link other than the current video (or to the first action if there is
  none). Tab moves in row order: row link → that row's `Ellipsis` → next row.
- While open, Esc only closes the popover, not the screen, and focus returns to the item. This `role="dialog"`
  popover is added to 012 "Interaction details" Esc exceptions (speed menu, tooltips, `role="menu"`). When a
  row's menu is open, Esc closes only the menu (the `ui/Menu` default).
- The content is fetched with `GET /api/videos/{id}/versions` each time it opens. Until it arrives, `count`
  `Skeleton`s (`h-10`) fill the rows. If the fetch fails, one `text-xs text-danger` line (`role="alert"`)
  "Couldn't load the versions" and a ghost `sm` "Retry" are shown inside. When the video is refetched on a
  `video` notification (`useVideoDetail`), an open popover refetches the list too; a closed one fetches on next
  open.

### Make representative

- Pressing "Make representative" in the menu sends `POST /api/videos/{id}/make-representative`. While sending,
  that row's `Ellipsis` becomes `LoaderCircle` (`animate-spin`, `aria-disabled`); other rows' actions stay
  pressable.
- On `200`, the list is replaced with the response's `VideoVersions`. The "Representative" marker moves to that
  row and the order changes (representative first). The popover stays open and no toast is shown; the marker
  moving is the result. The video is refetched too (`versions.representativeId` changes).
- The library item's title and thumbnail becoming the new representative's (requirement 7) happens when the
  library refetches on the `video` notification (`useItemRefresh`). This screen does not announce it. The library
  cache is not discarded; on return, the list has been updated on top of the cache by the notifications that
  arrived.

### Unbundle

- Pressing "Remove from versions" in the menu sends `POST /api/videos/{id}/unbundle`. There is no confirmation
  dialog: bundling again restores it, and the removed video only returns to its values from before bundling
  (requirement 3), so nothing is lost. While sending it looks the same as changing the representative.
- On `200`:

  | Removed row | Result |
  | --- | --- |
  | **Another row** | That row leaves the list and the item's count decreases. Toast "Removed "{title}" from the versions". If one version remains, the bundle has been dissolved (contract §4): the popover closes, the video is refetched, and the item disappears |
  | **The current video** | The popover closes and the video is replaced with the response's `Video` (`versions` gone; `tags`, `progress` and `public` are its own values). The item disappears and the tag list and visibility toggle redraw with its own values. Same toast |

- When the popover has closed, focus moves to the element nearest where the item was (the action before the
  technical-details line, or the first right-hand action); while it stays open, focus moves to the next row's
  link (or the previous row if there is none).

### Failure

- When changing the representative or removing fails, one `text-xs text-danger` line (`role="alert"`, preceded
  by `AlertCircle` `size-4`) "Couldn't change the versions: {reason}" appears below the list. The reason comes
  from `errorText` ("Words" above). The popover and list stay open. The line disappears on the next action or
  when the popover closes.
- `404 video_not_found` and `400 not_bundled` (changed first in another tab): show the line, then refetch the
  list and the video.
- The failure line below the facts line (012 and 029's place) is not used. What happens in the popover is
  reported in the popover.

### Resume position

- When the bundle's `progress.positionMs` is at or past that version's `durationMs`, playback starts from 0
  (R-11; Edge Case "versions differ in length"). The decision is added to `pageDecisions.resumePosition`. No
  "playing from the beginning" message appears in the player; the control bar shows the position.
- Playing a non-representative version saves the position through `PUT /api/videos/{id}/progress` as now, and the
  bundle's position advances (requirement 2, acceptance criterion 7). The screen never chooses a key (R-2).

### Group line and versions

A video that is both a group member and a bundle member has both the group-name line above the title and the
versions item in the facts line. They are separate facts (a grouping by location, and other files of the same
video) and are not merged. The "play next" sequence shows only representatives (requirement 5, data-model.md
§4).

## Selection bar

### Bundle action

- "Bundle as versions" (lucide `Layers` + text, ghost `sm` `Button`) goes right after "Visibility", before the
  vertical rule. In the lower row below `sm`, it is the fourth after the two tag actions and "Visibility", and
  follows the current rule (move to the right end when three do not fit): where four fit in one line they share
  the width equally; otherwise "Visibility" and "Bundle as versions" move to the right end of the next row. The
  container query width of `SelectionBar` is adjusted for four.
- **Not shown with fewer than 2 selected** (parent Issue requirement 11; the plan's acceptance "not shown for a
  single selection"). Making it `disabled` with a reason was rejected: most selections (one video) would always
  show a greyed action. It appears when the second video is selected.
- **When the selection exceeds the bundle limit `maxBundleSelection` (20)**, it is `disabled` with the reason
  "Bundle up to 20 videos at a time", as with the tag actions' limit (library-ui.md §6). The tag actions' limit
  (`maxVideoTagsSelection`, 20,000) is not used here. The dialog sends `GET /api/videos/{id}` for each selected
  video and lists one representative row per video, so opening it with thousands selected through "Select all"
  would fire that many requests at once and freeze the screen. Reading and choosing a representative from the
  rows also works only up to a few dozen. The limit is screen-side only; the server limit in contract §2
  (`too_many_videos`) does not change.
- Pressing it opens the "Bundle dialog" below. It is a dialog rather than a popover for the reason in "Why this
  shape".

### Bundle dialog

A `ModalFrame` dialog "Bundle as versions". Content from top to bottom:

- One paragraph of description (`text-sm text-fg-muted`, "Dialog description" in "Words"), so the user reads
  what will happen before choosing.
- The representative list (`radiogroup`, accessible name "Representative"): rows with `divide-y divide-border`,
  not wrapped in a border or card. Each row is a `label` with an `input[type=radio]` on the left (`size-4`,
  `accent-accent`; the same size and colour treatment as `Checkbox`) and two or three lines of text on the right.
  - Line 1: the title (`text-sm text-fg`, truncated to one line, full text in `title`).
  - Line 2: the differences (same format and order as line 2 of "Versions list").
  - Line 3 (only when present): in `text-xs text-fg-muted`, the names of hand-added tags joined by "·" (`tags`
    minus those that come only from folder names). It shows, before choosing, which tags become the bundle's and
    which are set aside. A video already in a bundle (has `versions`) starts this line with "Already 3 versions —
    all of them join" (contract §2).
  - Rows are `py-2`. Beyond 6 rows, `max-h-80 overflow-y-auto` scrolls only the inside. The rest of the dialog's
    height follows the `ModalFrame` rules.
  - Row order: from the selection bar, the selection order (`selectedIds`); from the candidate screen, the pair
    order (ascending id).
- The list content is fetched with `GET /api/videos/{id}` per selected id (at most `maxBundleSelection`, 20),
  because from the selection bar some ids have no list item loaded. From the candidate screen the pair's two
  `Video`s are passed as they are and nothing is fetched. Until loaded, `Skeleton`s (`h-12`) fill the rows. If any
  one fails (including 404), one `text-sm text-danger` line (`role="alert"`) "Couldn't load the selected videos"
  and a ghost `sm` "Retry" replace the list, and "Bundle" is `disabled`.
- The action row (`border-t border-border`, right-aligned, `gap-2`): secondary "Cancel" (initial focus) and
  primary "Bundle". **"Bundle" is `disabled` until a representative is chosen.** Preselecting the first row was
  rejected: one press without reading would decide whose values remain (same treatment as 014 "Merge and
  delete").
- Pressing "Bundle" sends `POST /api/video-bundles` (`videoIds` are the selected ids, `representativeId` the
  chosen representative). While sending, both buttons are `disabled` and "Bundle" shows `LoaderCircle` (as in the
  settings screen's folder-deletion confirmation).
- On `200`, the dialog closes and the toast "Bundled {N} videos as versions of "{title}"" appears (N is the
  response's `items.length`; the title is the representative's).
  - From the selection bar: the selection is cleared and the library list is refetched (non-representative items
    leave the list; acceptance criterion 6). The grid position is kept during the refetch. Focus moves to where
    "Select all" was (the bar is gone, so the first card of the grid).
  - From the candidate screen: see "Deciding" below.
- On failure the dialog stays open and one `text-sm text-danger` line (`role="alert"`) "Couldn't bundle:
  {reason}" appears above the action row. The chosen representative is kept. On `404 video_not_found` (a selected
  video has gone), the line is shown and the list is refetched.
- Esc and "Cancel" close without sending; focus returns to the trigger ("Bundle as versions", or "Same video…" on
  the candidate screen).

### Card

Library cards and list-view rows do not change. After bundling, the representative's item looks the same as
before bundling (`UI品質` "a list item looks the same as a video's item today").

## Duplicates page

### Entry

- "Duplicates" (lucide `Layers`, `/duplicates`, owner only; `ownerOnly` in `navEntries`) is added right after
  "Tags" in the sidebar's upper section. Expanded, rail and drawer views behave as for the other entries. No count
  badge ("Why this shape").
- The route has the same shape as `/tags` (inside `AppShell`, loaded only when used). A guest opening this URL is
  handled as for `/tags` (the gate handles the 401 from owner-only responses; 016 "Gate").
- `document.title` is "Duplicates".

### Layout

- The same body width and padding as the tag management screen (`mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-8`).
  From the top: `h1` "Possible duplicates" (`text-xl font-semibold`), the count line, the list of pairs. Nothing
  goes in the top bar. No search or filter (at most 200 pairs, and the number drops as decisions are made).
- The count line is `text-xs text-fg-muted tabular-nums`, `role="status"`, `aria-live="polite"`: "12 pairs".
  When `total` exceeds 200: "Showing 200 of 340 pairs".
- The pair list is a `ul` of rows with `divide-y divide-border`, not wrapped in a border or card. A pair is `py-4`
  (wider than a tag row because it holds two videos).

### Pair

A pair (`li`) has two tiers from the top:

1. **Heading line** (`flex items-center justify-between gap-3`): on the left, "Same length · similar frames" in
   `text-xs text-fg-muted` (says in one line why it is a candidate). The `distance` number is not shown: only the
   fact that it is within the threshold means anything, and it is not a value users can compare. On the right,
   two actions (`gap-2`): secondary `sm` `Button` "Different videos" and primary `sm` "Same video…" (lucide
   `Layers`). The DOM order is the same, so the first Tab stop in a pair is "Different videos", then "Same
   video…". The decision comes first and the videos' contents after (`UI品質` "put the "same video" and
   "different videos" decisions first in the candidate list"). "Same video…" is primary because the main decision
   of someone on this screen is "same", and "different" records an exception.
2. **The two videos** (`mt-3 grid gap-3 sm:grid-cols-2`): the pair's `videos` in ascending id order, left and right
   (top and bottom below `sm`). Each one is as follows (same density as a related-video row, 017 "Member list"):
   - `flex gap-3`. A thumbnail on the left (`w-40 shrink-0`, `aspect-video`, `rounded-md`, `bg-surface`; an
     `ImageOff` box when missing; a length badge at the bottom right; the same component as related-video rows).
   - Three lines on the right: the title (`text-sm font-medium text-fg`, truncated to two lines, full text in
     `title`), the differences (same format as line 2 of "Versions list", but the location moves to the next
     line), and the location (`text-xs text-fg-muted`, truncated to one line keeping the end, absolute path in
     `title`).
   - The whole video is a `Link` to `/videos/{id}` (`state.from` is `/duplicates`). Hover gives `bg-hover-wash`
     (outline from the row's `-m-1.5 p-1.5`). The accessible name is "{title} {length}" (as in related-video
     rows). The user can open the player screen to compare and come back; the back destination is the candidate
     screen, and undecided pairs are still there.
   - The two thumbnails are the same size and neither is emphasised. Differences are read from the numbers on
     line 2. Marking the one with the larger resolution or size was rejected: if the machine points at "the better
     one", it steers the choice of representative.
- `gap-3` separates the heading line from the two videos; `divide-y` and `py-4` separate pairs. There are no
  lines or borders inside a pair.

### Deciding

| Action | Behaviour |
| --- | --- |
| **"Same video…"** | Opens the "Bundle dialog" with the pair's two videos (the response's `Video`s as they are; not refetched). On `200` from "Bundle", the dialog closes, the pair leaves the list, the count decreases, and a toast appears (same text as the "Bundle dialog"). Refetching the list is left to the `scan` notification. Focus moves to the next pair's "Different videos", or the previous pair's, or the `h1` when no pair is left |
| **"Different videos"** | Sends `POST /api/version-candidates/dismiss` with no confirmation dialog. While sending, the pair's two buttons are `disabled` and the pressed one shows `LoaderCircle`. On `204` the pair leaves the list, the count decreases, and the toast "Marked as different videos. They won't be suggested again" appears. Focus moves as for "Same video…" |
| **Pair gone** (`404 video_not_found`; one side disappeared in a scan while the user was checking) | Toast "This pair is no longer a candidate" and the list is refetched (Edge Case) |
| Other failures | Toast "Couldn't bundle: {reason}" (the dialog's line when inside the dialog) or the `errorText` text. The pair stays |

"Different videos" cannot be undone (requirement 10), but its only result is "no longer offered as a
candidate", and bundling by hand (selection bar) is still possible, so no confirmation dialog is inserted. The
toast says it will not come back.

### Refresh

- `GET /api/version-candidates` is read on open. It is refetched on the `scan` notification the owner's shell
  receives (sent on success or failure of a `fingerprint` job; contract §6). During a refetch the list looks
  unchanged (no skeleton). When the response arrives it replaces the list and the scroll position is kept. New
  pairs are added at the top and pairs whose side has gone disappear (copes with candidates being added and
  removed during ingest).
- If the pair in an open dialog disappears on refetch, the dialog does not close. The 404 from "Bundle" reports
  it.

### States

| State | What the screen shows |
| --- | --- |
| Loading | The count line says "Loading…"; three `Skeleton`s (`h-24`) fill the list |
| Load failed | `EmptyState` (danger, `AlertCircle`) "Couldn't load the candidates", `Button` "Retry" |
| No candidates | `EmptyState` (lucide `Layers`) "No possible duplicates", description "When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself." No button; the ingest entry point is "Refresh library" in the top bar |
| Decided down to 0 pairs | Switches to the same empty state. The count line does not stay as "0 pairs"; the empty state's heading replaces the count |
| Action failed | See "Deciding" above |

## Responsive behaviour

Breakpoints are Tailwind's defaults only, applied in CSS (library-ui.md §4). The widths judged are 360px, 768px
and 1280px (the player screen has two columns from `lg`).

| Width | Versions item and list | Selection bar | Dialog | Candidate screen |
| --- | --- | --- | --- | --- |
| 1280px | Length, size, date added and "3 versions" on one line. The popover is `28rem`, aligned to the item's left edge | "Bundle as versions" is the fourth after the two tag actions and "Visibility", in one line | Width `max-w-lg`; the list grows to 6 rows, then scrolls inside | Two videos side by side, thumbnails `w-40`. Two buttons on the right of the heading line |
| 768px | Same | Same (one line from `sm`) | Same | Same |
| 360px | The item wraps like the other items. The popover is the screen width minus `2rem`, and line 2 of rows is truncated. No horizontal scroll | Four do not fit in the lower row, so "Visibility" and "Bundle as versions" move to the right end of the next row (current rule) | Full screen width (`ModalFrame` default). Line 2 of rows is truncated | The two videos stack vertically; each keeps its thumbnail and three lines side by side. The heading line's buttons wrap and stay right-aligned |

- Line 2 of a popover row is truncated to one line at every width; the full text is in the row's `title`.
- The candidate screen's location line keeps the end (nearest the file name) at every width.

## Review criteria

Judged by looking at the real screen (library-ui.md §5). "It exists" alone does not satisfy a criterion (Q-4).

1. **Visual hierarchy (player screen)**: on a bundled video's page, the eye goes player → title → tags, and "3
   versions" has the same weight as length, size and date added and is not read before them. Side by side with
   an unbundled video's page, the positions and spacing of the title, tags and the two facts lines are the same;
   the only difference is one more item (`UI品質` "visual hierarchy" and "spacing rhythm").
2. **Visual hierarchy (library)**: a bundled representative's card and an unbundled video's card cannot be told
   apart. No marker, number or band is added to cards (acceptance criterion 6; `UI品質` "changes that do not meet
   the requirement"). The same holds for list-view rows.
3. **Information density (player screen)**: before the popover opens, the only versions information is the "3
   versions" item; no version's resolution, size or location appears anywhere. When opened, line 2 of each row
   gives the differences between the two (resolution, codec, size, location) as numbers, with no explanatory text
   or headings between rows (`UI品質` "information density").
4. **Typography**: the item's text and the popover's row titles are `text-sm`, like the facts line. Line 2 of rows
   is `text-xs` in the secondary colour, like the technical-details line. The popover has no bold, large heading
   or accent-coloured text (`UI品質` "typography"). "Representative" is a text marker; the representative is not
   shown by colour or weight.
5. **Action priority (player screen)**: after opening the popover, one press on a row plays another version's
   file (acceptance criterion 7), and choosing during playback continues playing on the new page. Changing the
   representative takes two presses (`Ellipsis` → first item) and removal two (`Ellipsis` → below the separator);
   neither is on the row's surface. Playback is still the first thing touched, and the screen does not change
   unless the popover is opened.
6. **Results of changing the representative and removing**: changing the representative to B moves
   "Representative" in the popover to B's row; back in the library, the single item's title and thumbnail are B's
   and the tags stay X (acceptance criterion 8). Removing B returns B's page tags to Y, B appears in the library as
   a separate item, and A's bundle tags stay X (acceptance criterion 9). Both results are visible in the popover
   without leaving the page.
7. **Resume position**: when the bundle's playback position is at or past that version's duration, the version
   starts from 0. The control bar's position is 0 and no message or layer appears.
8. **Selection bar**: with one video selected the bar has no "Bundle as versions"; it appears when a second is
   selected. In the dialog, "Bundle" cannot be pressed until a representative is chosen; choosing and pressing
   leaves only the representative in the list and clears the selection (acceptance criterion 6). With 21 or more
   selected, "Bundle as versions" cannot be pressed, its reason is readable, and the dialog does not open. At
   360px the lower row does not break and there is no horizontal scroll.
9. **Dialog**: order is description paragraph → rows → actions; rows have no border, and line 3 of each row shows
   which tags remain before choosing. A row for an already bundled video shows "Already N versions — all of them
   join".
10. **Candidate screen (hierarchy and density)**: looking at a pair, the eye goes first to the two buttons at the
    top right, then the two thumbnails and titles, and last to the difference numbers and locations. The two
    thumbnails are the same size and neither is emphasised. There are no lines, borders or cards inside a pair,
    and pairs are separated by one line. At 360px the two videos stack with no overflow or overlap.
11. **Candidate screen (deciding)**: "Same video…" → choose a representative → "Bundle" removes the pair and the
    library shows one item (acceptance criteria 3 and 6). "Different videos" removes the pair, and it does not come
    back after a rescan (acceptance criterion 5). When new pairs are added at the top during ingest, the pair being
    read does not move.
12. **State display**: while sending, the pressed button or `Ellipsis` shows a spinner, and pressing again does
    not send twice. A failure is a red line in place (below the popover list, above the dialog's actions), and
    what is open stays open. Only the candidate screen's failures are toasts (because one line can say whether the
    row is gone or should be removed).
13. **Keyboard**: on the player screen Tab reaches the item; Enter opens it with focus on the first other version's
    row; Esc closes it and returns to the item; a second Esc closes the screen. On the candidate screen Tab moves
    per pair: "Different videos" → "Same video…" → left video → right video.
14. **Guest**: logged out, a bundled video has the "3 versions" item and popover, and pressing a row plays B
    (acceptance criterion 12). Rows have no `Ellipsis`. The sidebar has no "Duplicates", and there is no selection
    bar at all.
15. **Examples that do not meet the requirement** (`UI品質`): non-representative versions appear in a list; the
    versions list appears on the player screen as an always-open panel or tab; "3 versions" is next to the title or
    inside the player and is seen before the tags; a bundled card has a marker or count and looks different from
    other cards; on the candidate screen one of the two is emphasised first as "recommended".

## Colour

No new pair is used. Popover and dialog text is `fg` and `fg-muted` on `elevated`; the failure line is `danger` on
`elevated` (the pair added in 014 "Accessibility"); candidate screen text is `fg` and `fg-muted` on `bg`; toasts
are unchanged. `fg-subtle` is used only for markers (`Layers`, "·"), never for text. `bg-hover-wash`,
`bg-active-wash` and `border-accent` are not text and are outside `pairs`. No new token is added.

## Accessibility

The parent Issue does not ask for screen-reader or contrast design, so only names are decided (the same scope as
012 and 029).

- The item's accessible name: "3 versions of this video. Show versions". The popover is `role="dialog"` with the
  accessible name "Versions".
- Row links: "Play {title}, {differences}". The current video's row has `aria-current="true"` and a hidden "Now
  playing". The "Representative" marker is visible text and is not hidden.
- Row menu trigger: "More actions for {title}".
- Dialog list: `radiogroup`, accessible name "Representative". Each `label` includes the title and the
  differences.
- Candidate screen: each pair is an `li`; the heading line's "Same length · similar frames" is not used as the
  pair's name (it is the same for every pair). The two links' accessible names are "{title} {length}". Button
  accessible names append "for {title A} and {title B}" to the visible text ("Same video, for A and B").
- Failure lines are `role="alert"`. Spinners are `aria-hidden`; elements being sent are `aria-disabled` or
  `disabled`.
