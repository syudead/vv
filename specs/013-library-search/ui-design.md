# UI Design: search, filter and sort in the list and folder pages

**Feature**: [parent Issue #195](https://github.com/syudead/vv/issues/195)

The visual rules, the shell and the list density follow
[Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md) and `@theme`
in [`web/src/index.css`](../../web/src/index.css). The folder page structure follows
[011's ui-design.md](../011-folder-browser/ui-design.md). The URL shape and history are fixed by
[contracts/list-url.md](contracts/list-url.md), and the search syntax by
[contracts/list-api.md §1](contracts/list-api.md#1-search-syntax); this document does not
redecide them. It defines only what the toolbar, the syntax help, the result count, the no-match
state and the folder page search **add to or change in** the existing pages. No new color,
radius or shadow tokens are added.

## Screen boundary

- The library (`/`), the top level of the folder page (`/folders`) and each folder put the same
  parts (the search box and its help, the filter, the sort and direction) inside `TopBarPortal`.
  The folder page borrows the parts from `web/src/library/`.
- The folder page toolbar is the library toolbar without the view switch, because the folder
  page has no list view.
- The folder page toolbar drops its current right-aligned layout and uses the library layout:
  the search box first, grouped in the center.
  - Why: right alignment was the layout when there was no search box (011 "Toolbar"). With a
    search box, the parts should look like the same parts of the same product.
- The grid, the cards, the shell and the scroll ownership do not change.

## Toolbar

The result grid is primary and the toolbar is secondary. Inside the toolbar, only the search box
looks like an input (a `bg-field` frame). Everything else is the current `Button` secondary and
`IconButton`. No row is added.

The parts are ordered from left to right as below. This order is also the Tab order.

1. **Search box** (the current `SearchBox`; the width rules `min-w-20`, `sm:min-w-40` and
   `sm:max-w-md` stay).
2. The button that opens the **syntax help**, at the **inner right edge** of the search box frame
   ("Syntax help" below).
3. The **filter** button (the current `ListFilter` button, with the active count as today).
4. The **sort** menu button (the current kind label + `ChevronDown`).
5. The **direction** toggle, or **"Shuffle"** in random order (only one of them, in the same
   place).
6. View (library only), card size, and the "View and sort" menu button (as today).

The width breakpoints stay the current ones (Tailwind default `md`, `lg`, `xl`).

| Width | Visible |
| --- | --- |
| 360 px (below `sm`) | Search box (including the help button), filter, "View and sort" |
| 768 px (`md` and up) | The above, plus sort and direction |
| 1024 px (`lg` and up) | The above, plus view (library only) |
| 1280 px (`xl` and up) | The above, plus card size. In grid view, "View and sort" disappears |

- Below `md`, the sort kind and the direction move into the "View and sort" menu (as today). The
  filter stays a separate button at every width, as in the current library. The search box stays
  at every width.
  - Rejected: moving the filter into the menu too. The active count would hide inside the menu,
    and at 360 px the toolbar would no longer show what is filtering the list.
- The toolbar shows what is filtering the list in 3 places: the search term in the search box,
  the watch status and playability in the filter button's `bg-accent-soft` and count (the
  current look), and the sort in the button label.

### Sort and direction

- The sort menu (`MenuRadioGroup` from `ui/Menu`) lists the 7 kinds in one column, with the
  labels and lucide icons below. The menu button shows only the kind label, not the direction.
- "Recently played" is for the owner only. The menu for a guest (someone not signed in) has the
  other 6 kinds
  ([016's ui-design.md "Guest degradation"](../016-single-account-auth/ui-design.md#guest-degradation)).

| Kind | Label | Icon | Direction when selected | Ascending / descending wording |
| --- | --- | --- | --- | --- |
| Date added | "Date added" | `CalendarArrowDown` | Descending | "oldest first" / "newest first" |
| Date modified | "Date modified" | `CalendarClock` | Descending | "oldest first" / "newest first" |
| Title | "Title" | `ArrowDownAZ` | Ascending | — |
| Length | "Length" | `Timer` | Descending | "shortest first" / "longest first" |
| File size | "File size" | `HardDrive` | Descending | "smallest first" / "largest first" |
| Recently played | "Recently played" | `History` | Descending | "least recently played" / "most recently played" |
| Random | "Random" | `Shuffle` | — | — |

- Selecting a kind sets that kind's "direction when selected". Date added and title keep the
  directions of the current `addedDesc` and `titleAsc`.
  - Rejected: carrying over the previous kind's direction. Moving from date added descending to
    title would give "Z→A", which is almost never wanted.
- The **direction toggle** is one button right next to the menu button.
  - It is secondary like the menu button, with no gap between the two, and the touching corners
    are not rounded (the same edge treatment as `SegmentedControl`).
  - The icon is `ArrowDownWideNarrow` for descending and `ArrowUpNarrowWide` for ascending.
  - The accessible name and tooltip take the form "Descending (newest first). Press for
    ascending". A kind without wording uses "Ascending. Press for descending".
- In **random** order, the same place holds the "Shuffle" button instead (lucide `Dices`; the
  accessible name and tooltip are "Shuffle").
  - Rejected: hiding or disabling the direction button. Hiding shifts the toolbar width, and
    disabling adds one more control that cannot be pressed (Q-5).
  - `RefreshCw` is not used for "Shuffle", because the top bar's "Refresh" uses it.
- Inside the menu below `md`, the sort kinds are 2-column radios as today. Below them, the
  direction is a `SegmentedControl` (2 options; the accessible names include the wording). In
  random order, that place holds a `Button` (ghost, `sm`) labelled "Shuffle".

### Filter menu

The current filter popover (the 2×2 radios for watch status, "Playable only") stays.

- The button at the bottom is renamed from the old label (clear the filtering) to "**Clear
  filters**" (clear the conditions). As defined in
  [list-url.md §2](contracts/list-url.md#2-meaning-on-the-folder-page), it removes the search term, the
  watch status and playability.
  - It appears when any of those 3 is active (today, only when a filter is active).
  - Pressing it closes the popover and returns focus to the filter button.
- The count on the button still counts only the watch status and playability, as today. The
  search term is visible in the search box, so it is not counted.

## Syntax help

- The part is `ui/Popover`. The opening button is a small `IconButton`-like button (`size-6`,
  lucide `CircleHelp`, `text-fg-muted`, `bg-hover-wash` and `text-fg` on hover) at the inner right
  edge of the search box frame.
  - With a search term, it sits to the right of the current "Clear search" ×.
  - The `/` key hint appears to the left of the help button, as today only when the search term
    is empty and the width is `sm` or up.
  - Rejected: a separate button outside the frame. At 360 px it takes about 36 px from the search
    box, and it looks like a control unrelated to the search box.
- It takes no space when closed. Click, Enter and Space open it. Hover or focus alone does not.
- Contents from top to bottom are listed below. The popover width is `w-80`, wider than the
  current default (`w-72`). At 360 px the current collision handling keeps it on screen.
  1. The heading "How to search" (`text-sm font-medium text-fg`).
  2. A 4-row table. The left column shows the example in `font-mono text-xs text-fg` (the same
     monospace as `player/StatusOverlays`). The right column shows the meaning in
     `text-xs text-fg-muted`.
     - `京都 2024` — "Finds videos that contain every space-separated word"
     - `"京都旅行 2024"` — "Words wrapped in `"` are searched for as one term, spaces included"
     - `京都 -2023` — "Prefix a word with `-` to leave out videos that contain it"
     - `京都 OR 奈良`, `京都 | 奈良` — "Finds videos that contain either word. Binds tighter
       than a space"
  3. Below a divider (the same `bg-border` line as `MenuSeparator`), 2 lines in
     `text-xs text-fg-muted`: "Full-width and half-width characters, upper and lower case, and
     hiragana and katakana are treated the same." and "Only the first 16 terms are used."
     ([list-api.md §1-7](contracts/list-api.md#1-search-syntax))
- Pressing an example does not put it into the search box.
  - Rejected: inserting it. A single press would replace the current search term by accident.
- Opening keeps focus on the help button (stop `onOpenAutoFocus`; the precedent is the popover
  in `web/src/shell/ScanProgressIndicator.tsx`).
  - The popover has no controls. Moving focus inside would let the Radix FocusScope trap Tab, and
    only Esc could leave.
  - Rejected: moving focus inside and handling Tab ourselves. The help is read-only secondary
    information, with no reason to take focus.
- The help button points to the popover with `aria-expanded` and `aria-controls`. While the
  popover is open, `aria-describedby` points to its contents. A screen reader reads the contents
  while focus stays on the button.
- Tab from the button to the next control (the filter button) moves focus out of the popover, so
  the popover closes.
- Esc while the popover is open closes only the popover. Focus stays on the help button, and the
  search term stays (Acceptance criterion 16). Esc with focus in the search box (clear the term
  and leave) behaves as today. The popover is not modal.

## Result status

- The search box shows the search term and the list itself shows the results, so the summary
  shows only "N videos". It does not repeat the search term, the loaded range, the total
  duration, the total size or explanatory words.
- The count has `role="status"` (`aria-live="polite"`). N is the server's `total` (all items
  after every search term and filter is applied).
- When the first page fails to load, the count is not shown; only the load-failure notice is.

## No-match state

The library and the folder page share one part.

- The heading is "No videos match these conditions".
- It shows no chips or notes for the search term, the filters or the search scope.
- It shows no "Clear filters" button. The user changes the conditions from the search box or the
  filter menu.
- On the folder page with filters only, the subfolder group stays on top, and this state takes
  the place of the video group.
- The empty state for 0 registered folders or 0 videos appears only when there are no
  conditions (as today).

## Folder screen

### Toolbar and scope

- The search box's accessible name is "Search in {folder name}" in each folder and "Search
  videos in all folders" at the top level. The library keeps "Search videos".
- The visible placeholder is "Search this folder" in each folder and "Search all folders" at the
  top level.
  - Why the wording differs from the library's "Search": it tells the user, before typing, that
    the scope here is different.
- While searching (the search term is not empty), the breadcrumb bar continues after the current
  location with "— searching inside" in `text-fg-muted` ("Folders › movies › A — searching
  inside"). At the top level, "Searching all folders" replaces "Folders".
  - This text is not truncated, even when the current location's name is.
  - The rule that collapses ancestor segments into "…" stays (below `md`, with 4 or more
    segments).
  - The breadcrumb bar is sticky, so the scope stays visible while scrolling.
- With filters only, nothing is added to the breadcrumb bar, because the scope stays as today
  (direct).
- `/` and Esc behave as in the library, because the whole search box part is shared (Acceptance
  criterion 21).

### Search results

With a search term, the content takes the same shape as the library list.

- The subfolder group and the "Videos N" heading are not shown. Below the breadcrumb bar come the
  same "N videos" as in the library and one grid. A visually hidden `h2` "Search results" replaces
  the heading.
- The grid and the video cards have the same width, the same `gap-2.5` and the same zoom as the
  library.
- Each card gets a **location line** below the title and above the current metadata line (date
  added · size).
  - One line: lucide `Folder` (`size-3`, `text-fg-subtle`, not read aloud) + `text-xs
    text-fg-muted`. It is smaller than the title (`text-sm font-medium`) and in a secondary
    color.
  - The content is the path relative to the open folder, joined with `/` ("B" for `y` in
    Acceptance criterion 17). A video directly in the open folder shows "This folder".
  - Rejected: no line for a video directly in the folder. The absence of the line could not tell
    "direct" from "no information".
  - A top-level search starts from the registered folder's display name (`movies/2024/京都`).
    Directly in a registered folder, the line is only the display name.
  - When it does not fit, the **start** is truncated and the last folder name stays
    (`…/2024/京都`), like the folder card path in 011. The `title` attribute holds the whole path
    (at the top level, starting from the registered folder's path).
  - The line is not a link. A link would give the card 2 links and double the Tab stops. To reach
    that place, the user clears the search term and follows the subfolder cards. This feature adds
    no direct way to move there from a search result (the Issue's requirements do not ask for it).
  - The card link's accessible name is "{title}, {location}", so results with the same title can
    be told apart by screen reader.
- Library search result cards get no location line.

### Filter only

With an empty search term and only watch status or playability set, the current folder page
layout stays (subfolders → "Videos N"), and only the videos directly in the folder are filtered.

- N in "Videos N" is the filtered `total`. The subfolder group does not change.
- No visible summary line is added: above the subfolders, it would be unclear what it counts. A
  visually hidden `role="status"` (`aria-live="polite"`) announces the change as "N videos
  directly in this folder".

### Top level (`/folders`)

- With an empty search term, only the registered folder cards are shown, as today. The filter,
  sort and direction buttons are **disabled** (`disabled`, the current parts' `opacity-50`), and
  the filter count is not shown. `watch` and `playable` in the URL have no effect there
  ([list-url.md §2](contracts/list-url.md#2-meaning-on-the-folder-page)).
  - Rejected: hiding them. If the buttons appeared the moment a term is typed, the centered
    toolbar would shift the search box sideways during typing.
  - Q-5: the value of the disabled buttons is that the same parts sit in the same place as on
    the other folder pages, showing that typing a search term enables filtering. The risk that
    they look broken is reduced because they sit right next to the search box, and the
    placeholder "Search all folders" tells the user that the condition is to type. Disabled
    buttons leave the Tab order.
- Inside the "View and sort" menu below `md`, the sort radios and the direction are disabled the
  same way at the top level with an empty search term. Card size stays usable.
- With a search term, the results for the whole library appear as in "Search results" above.

## States

| State | Appearance |
| --- | --- |
| Search results loading | "Loading…" in place of the count, and 12 video card skeletons in one grid. No folder card skeletons |
| The folder disappears during a search (404) | The same "This folder wasn't found" as today, not the no-match state |
| Search results or the next page fail to load | The same `LoadFailed` and "Couldn't load more" as the library |
| No match | "No-match state" above |

## Interaction states

The added controls (the help button, direction, "Shuffle", "Clear filters") inherit hover,
focus-visible, active and disabled from their base parts (`IconButton`, `Button` secondary and
ghost, the current × button). No custom state colors are created. Focus is shown by the global
`:focus-visible` (an outer outline in the `link` color). When a button inside the search box frame
has focus, that button shows its outline, and the search box frame's focus style
(`focus:border-accent`) is not shown.

## Accessibility

- Accessible names:
  - The search box has the 3 names above.
  - The help button is "How to search", and the popover is named by its heading "How to search".
  - The filter button keeps "Filter (N applied)".
  - The sort menu button keeps "Sort by: {kind}". The menu heading and the "View and sort" menu
    keep the current "Sort by" wording.
  - Direction and "Shuffle" are as above.
- Count changes: the library and the folder page while searching announce them `polite` from the
  visible count line.
- Keyboard order (Acceptance criterion 23): in the top bar, search box → (× when there is a
  search term) → help → filter → sort → direction (or "Shuffle") → view, card size, menu →
  "Refresh". Then the sidebar entries, then the result cards in the main content. Shell parts
  come before the main content as in 011 "Accessibility", and within that sequence the Tab order
  keeps this order.
- Color pairs: the additions are `fg-muted` and `fg-subtle` on `surface` (the location line), and
  `fg` and `fg-muted` on `elevated` (the help and chips). The text pairs are already in
  `tokens.test.ts`. `fg-subtle` is used only for icons, never for text. No new pair is added.
