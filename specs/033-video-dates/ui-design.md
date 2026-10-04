# UI design: Keep each video's edit time and file creation time, and use them in display, sorting and the external API

**Feature**: [parent Issue #630](https://github.com/syudead/vv/issues/630) ·
[plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) ·
[research.md R-7](research.md#r-7-the-response-fields-are-updatedat-edit-time-in-vv-and-filecreatedat-location-creation-time-with-the-same-names-in-the-screen-and-external-apis) ·
[R-8](research.md#r-8-the-video-page-reloads-the-video-after-tag-and-visibility-changes-and-no-new-domain-event-is-added)

Sources: the visual rules come from these documents and are not decided again
here.

| Topic | Source |
| --- | --- |
| Colour, interaction states, width breakpoints, toolbar and video page layout | [Library UI](../../docs/design-docs/library-ui.md) (the toolbar in "6. List layout" and "8. Video page layout") |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css). Referenced by name; values are not copied |
| Pairs checked for contrast | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| The two "Video facts" rows (fact form, icons, accessible names, `title`, wrapping) | [specs/012-video-detail-ia/ui-design.md "Video facts"](../012-video-detail-ia/ui-design.md#video-facts) and the current [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx) |
| Precedents for adding a fact to the row (the thumbnail and versions facts) | [specs/029-video-overrides/ui-design.md "Thumbnail fact"](../029-video-overrides/ui-design.md#thumbnail-fact), [specs/030-video-versions/ui-design.md "Versions fact"](../030-video-versions/ui-design.md#versions-fact) |
| Sort menu, direction toggle, below-`md` grouping | [specs/013-library-search/ui-design.md "Sort and direction"](../013-library-search/ui-design.md#sort-and-direction) and the current [`web/src/videoList/SortControls.tsx`](../../web/src/videoList/SortControls.tsx) and [`listCriteria.ts`](../../web/src/videoList/listCriteria.ts) |
| Guest sorting (without "Recently played") | [specs/016-single-account-auth/ui-design.md "Guest degradation"](../016-single-account-auth/ui-design.md#guest-degradation) |
| Where screen text lives and its formatting | [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The English here states intent; after implementation the catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth |

This feature adds two things to the screens, and neither adds a colour, radius or
shadow token:

1. **Edit time and creation time facts** in the facts row of the video page
   (`/videos/:id`) (requirement 4, acceptance criteria 1 to 5)
2. **"Date created"** in the toolbar sort of the library and folder screens
   (requirement 5, acceptance criteria 6 and 8)

List cards, list-view rows, folder cards, group cards and related-video rows do
not change (out of scope: "showing the edit or creation time on list cards"). The
existing "Date modified" sort keeps its name, icon, direction and phrasing
(requirement 7). Owners and guests see the same things (R-7; like the date added,
these are facts about a public video).

## Words

The English terms are chosen only to avoid collisions with words already used on
these screens.

| Time | Name in the facts row | Name in the sort | Reason |
| --- | --- | --- | --- |
| Date added (unchanged) | Added | Date added | Unchanged |
| Edit time (edits in vv) | **Edited** | — | See below |
| File modification time (unchanged) | — | Date modified | Unchanged (requirement 7) |
| Creation time (file) | **Created** | **Date created** | See below |

- The edit time is **Edited**, not "Updated", to avoid collisions with words on
  the same screens. The sort's "Date modified" is the file's mtime, and in English
  modified and updated read as the same thing. The top bar's refresh (ingest)
  would also share the word with "Updated", the literal rendering of the API's
  `updatedAt`. "Edited" says that this is when the owner **edited** the display
  name, tags, visibility or thumbnail, using the same word as the title's edit
  button (lucide `Pencil`, 029). The parent Issue's `UI品質` requirement that "the
  date added, edit time and creation time can be told apart in words" is met
  because Added, Edited and Created cannot be read as meaning the same thing. The
  API name stays `updatedAt` as in R-7 and may differ from the screen's word (the
  API mirrors the parent Issue's term; the screen prioritizes distinction from
  other words on the same screen).
- The creation time is **Created** / **Date created** because file managers
  (Finder, Explorer) have converged on "Date added", "Date modified" and "Date
  created", and their meanings there (added, file modified, file created) match
  vv's. "Date created" and "Date modified" in the sort use different words, which
  meets the `UI品質` requirement that "the difference between date created and
  date modified (file) is clear from the name". A name that mentions the file,
  such as "File created", is rejected: "Date modified" does not mention the file,
  so the two would not match, and it would instead suggest that "modified" is not
  about the file.
- The direction phrasing is "oldest first / newest first", the same as for date
  added and date modified.

## Video facts

Two facts are added **after** the date added in the file facts row (the first row
of `VideoFacts`, `ul` "File details"), for owners and guests. The order is as
follows; the position and form of length, size and date added do not change (the
parent Issue says "alongside the existing length, size and date added").

1. Length (`Clock`, unchanged)
2. Size (`HardDrive`, unchanged)
3. Date added (`CalendarPlus`, unchanged)
4. **Edit time** (Edited)
5. **Creation time** (Created)
6. Versions fact (030, only when present)
7. Thumbnail fact (029, only for the owner when set)

- The fact form matches the existing `Fact`: icon (`size-4`, `text-fg-subtle`,
  `aria-hidden`) → visually hidden name → value. Text is the row's
  `text-sm text-fg-muted tabular-nums`, `gap-1.5` inside a fact, and `gap-x-4`
  between facts (`gap-x-5` at `sm` and up). No label text, divider, border or
  accent colour (the parent Issue: "in the same format as the current date
  added"; `UI品質` "visual hierarchy").
- **The value is the date only** (`formatDate`, e.g. `Sep 27, 2026`), so the three
  dates share one format. Showing date and time is rejected: on a row beside
  length and size, a time would make that fact alone longer and heavier and break
  "the same format as the date added".
- **The name and time are readable both on hover and on press.** The `title` of
  the three date facts (date added, edit time, creation time) is "name + date and
  time" (e.g. `Edited Sep 27, 2026, 3:04 PM`, `formatDateTime`). The current
  date-added `title` is the name only, so the date added changes to this form too.
  This lets the user confirm on screen that the edit time advanced after an edit
  on the same day (acceptance criterion 1). The accessible name (the hidden name)
  stays the name only, and the value is read from the text after it.
- `title` does not appear on devices that cannot hover (touch, including 360px
  widths), so each of the three date facts is a `ui/Popover` trigger (`button`).
  Press, tap or Enter shows the same "name + date and time" in a small popover
  (`side="bottom"`, `align="start"`, `w-auto px-3 py-2 text-sm`; one line, the name
  in `text-fg` and the date and time in `text-fg-muted tabular-nums`). Always
  showing the name is rejected (it adds label text and breaks "the same format as
  the current date added"). A hover-only tooltip is rejected for the same reason
  as 012's "converting for playback" (touch and keyboard users cannot reach it).
  - The trigger looks like the current facts (icon → value, `text-sm
    text-fg-muted`), and like the versions fact (030) uses `-mx-1 px-1 rounded-sm`
    with `bg-hover-wash` and `text-fg` on hover. No `ChevronDown` (the content is
    read-only; there is nothing to choose). No surface, border or accent colour.
  - The trigger's accessible name stays hidden name + value ("Edited Sep 27,
    2026"). The popover closes on Esc, an outside press, or a second press, and
    focus returns to the trigger. It contains nothing focusable.
  - Length and size are not triggers. Their value units (`12:34`, `1.2 GB`) and
    icons convey their meaning, and without a name they cannot be confused the way
    dates can.
- **Icons** differ across the three dates, so they can be told apart without
  reading the name (`UI品質` "distinguishable visually too").

  | Fact | Icon | Reason |
  | --- | --- | --- |
  | Date added | `CalendarPlus` | Unchanged |
  | Edit time | lucide `PencilLine` | The same family as the title's edit button `Pencil`, meaning "the owner changed it". A calendar-family icon (`CalendarCog` and similar) is rejected: beside the date added's `CalendarPlus` the difference would be a small glyph that cannot be read at `size-4` |
  | Creation time | lucide `FileClock` | Means "the file's own time" and, like size's `HardDrive`, shows by its picture that this is a file fact. The document outline tells it apart from length's `Clock`. `FilePlus` is rejected because its "+" overlaps with the date added's `CalendarPlus` |

- The order date added → edit time → creation time is chosen because the date
  added and edit time are a timeline inside vv (the edit time is on or after the
  date added), and placing them side by side makes "an unedited video shows the
  same date twice" (acceptance criterion 3) readable directly. The creation time
  is a file fact, so it comes after them. Putting the creation time first, in
  chronological order, is rejected: it would move the date added and change the
  existing order of three.
- Where the creation time is unavailable (mtime substitute), the fact's name, icon
  and form are the same, and the screen does not indicate the substitution. For
  the viewer the value is "the best available file creation time", and a marker
  would offer no way to correct it (out of scope: "the owner correcting the
  creation time by hand").
- When a value cannot be read (empty), the fact is shown with an empty value like
  the other dates (`formatDate`'s existing handling). The field is required, so
  this does not normally happen.
- The actions at the right end (camera, "Open file", "Copy path"), the error line,
  the technical-details row, and the row spacing (`gap-3`) do not change. The two
  extra facts wrap like the others (see "Responsive behaviour" below).

### Refresh after edits

Acceptance criterion 1 is that after an edit the video page shows the edit time
at that date.

| Edit | How the edit time updates |
| --- | --- |
| Saving the display name, setting or clearing the thumbnail | The current path that replaces the screen's video with the response's `Video` also swaps the edit time |
| Attaching or detaching a tag, toggling visibility | The video is reloaded after success (R-8). During the reload the facts row keeps its previous values and does not return to a skeleton or empty values: only the edit time changes, and clearing the whole row would hide what changed |

When the reload fails, nothing is shown in that operation's error line (the
existing form for tags and the visibility toggle), and the previous values stay.
Reopening the video shows the correct value.

## Sort and direction

The sort kinds grow from 7 to **8**. The library and folder screens share
`SortControls`, so both show it (requirement 5). The existing seven kinds keep
their names, icons, directions and phrasing (requirement 7).

| Kind | Name | Icon | Direction when chosen | Ascending / descending phrasing |
| --- | --- | --- | --- | --- |
| Date added | Date added | `CalendarArrowDown` | Descending | oldest first / newest first |
| Date modified (file) | Date modified | `CalendarClock` | Descending | oldest first / newest first |
| **Date created** | **Date created** | **`FileClock`** | Descending | oldest first / newest first |
| Title | Title | `ArrowDownAZ` | Ascending | — |
| Length | Length | `Timer` | Descending | shortest first / longest first |
| File size | File size | `HardDrive` | Descending | smallest first / largest first |
| Recently played | Recently played | `History` | Descending | least recently played / most recently played |
| Random | Random | `Shuffle` | — | — |

- **Position**: right after "Date modified". The three date kinds (added,
  modified, created) sit together and read as one group through their "Date …"
  names. It goes before Title, so the five kinds from Title on move down one. It
  also matches Finder's sort order (Added → Modified → Created) and keeps an order
  users already know.
- **Icon**: `FileClock`, the same as the creation time in the facts row. Like
  file size, whose `HardDrive` is the same in the facts row and the menu, this
  shows across screens that they are the same thing. Its picture differs from the
  calendar family of date added and date modified, so the three date kinds can be
  told apart at `size-4`.
- Choosing it gives `createdDesc`; the direction toggle gives `createdAsc`. The
  direction button's accessible name and tooltip keep the current form
  "Descending (newest first). Press for ascending".
- The device setting (`viewPreferences`) and URL (`sort=createdDesc`,
  `createdAsc`) round trips work like the other kinds. Guests get it too (it does
  not depend on owner data; R-7). The guest menu has **7 kinds**, without
  "Recently played".
- In the below-`md` "View and sort" grouping, the two-column radios take 4 rows
  for the owner (8 kinds) and 4 rows for guests (7 kinds; the last row has only
  the left cell). Like the current `CompactSortControls`, the menu order flows
  row-first into `grid-cols-2` without rearranging. For the owner, row 1 is "Date
  added" and "Date modified", row 2 "Date created" and "Title", row 3 "Length" and
  "File size", row 4 "Recently played" and "Random". "Date created" is on the left
  of row 2, so the three dates continue in reading order (left to right, top to
  bottom). For guests "Recently played" drops out and "Random" moves to the left
  of row 4. The direction `SegmentedControl` is unchanged.
- The menu button's text is only the kind's name ("Date created"), as today, with
  no direction. The button is about as wide as "Date modified" and does not move
  the other toolbar parts.

## Responsive behaviour

Only Tailwind's default breakpoints are used, switched in CSS (library-ui.md 4).
The judged widths are 360px, 768px and 1280px.

| Width | Facts row | Sort |
| --- | --- | --- |
| 360px (below `sm`) | The five facts wrap onto 2 or 3 lines with `gap-x-4`. Wrapping happens between facts, never inside one (icon and value stay together). The right-end actions go to the end of the row, as today. No horizontal scroll | Inside the "View and sort" grouping: two columns × four rows of radios, and the text "Date created" fits on one line without truncation (`truncate`) |
| 768px (`md`) | The five facts take 1 or 2 lines, `gap-x-5` | "Date created" fits on one line in the toolbar's menu button, which stays attached to the direction button |
| 1280px (the two-column `lg` layout) | The five facts (up to seven with the versions and thumbnail facts) take 1 or 2 lines within the left column's width. The player's maximum-width calculation (`max-w-[calc(…)]`, 012) does not change. Even for a video whose facts row takes two lines, the left column normally stays within the area that does not scroll (the row grows by one `text-sm` line) | Same as above |

## Review criteria

Judged by looking at a real device (library-ui.md 5). "It is there" alone does
not pass (Q-4).

1. **Visual hierarchy (video page)**: on a video page, the eye goes player →
   title → tags → visibility toggle, and Edited and Created have the same weight
   as length, size and date added and are not read before them. Compared with the
   screen before this feature, the position and spacing of the title, tags and the
   two facts rows are the same; the only difference is two more facts (`UI品質`
   "in the same format as the current date added").
2. **Distinction (video page)**: in the facts row, the three dates can be told
   apart by picture without reading the name (`CalendarPlus`, `PencilLine` and
   `FileClock` look different from one another at `size-4`). Both the hover
   `title` and the popover opened by pressing a fact on a touch device (360px)
   show the three words "Added …", "Edited …" and "Created …" with times, and a
   viewer can say which is a vv action and which is a file fact without any
   explanatory text. When two identical dates sit side by side (Added and Edited
   of an unedited video), the date formats match, and neither alone has a time or
   bold text.
3. **Information density (video page)**: the facts row gains no label text,
   divider, or annotation such as "(file)". Dates show year, month and day only;
   times appear only in the `title` and the press-to-open popover (`UI品質`
   "information density").
4. **Spacing rhythm**: the spacing between facts (`gap-x-4` / `gap-x-5`) and
   between rows (`gap-3`) is the same as before. At 360px the left edges of the
   wrapped second and third lines align with the first line, and no fact breaks in
   the middle.
5. **Typography**: date text uses the same `text-sm text-fg-muted tabular-nums`
   as length and size, and the digits of the three dates line up. Icons are
   `text-fg-subtle`, lighter than the text.
6. **Edit results**: saving the display name changes the Edited date to that day
   without leaving the screen, and the `title` time becomes the save time
   (acceptance criterion 1). The same holds after attaching or detaching a tag,
   toggling visibility, and setting or clearing the thumbnail. Meanwhile the facts
   row never disappears or empties. Playing, closing and reopening, or rescanning
   does not change Edited (acceptance criterion 2). For an unedited video, Added
   and Edited show the same date and the same `title` time (acceptance
   criterion 3).
7. **Creation time value**: where creation times are available, the date and
   time in Created's `title` match the file creation time the OS shows
   (acceptance criterion 4); where they are unavailable, they match the mtime
   (acceptance criterion 5). The fact looks the same either way.
8. **Sort menu**: opening the menu shows "Date added", "Date modified" and "Date
   created" in this order, with three different icons. From the names alone, a
   viewer can say which of "Date created" and "Date modified" is the file's
   creation and which its modification (`UI品質` "clear from the name"). The order
   and names from Title on are the same as before.
9. **Sort results**: choosing "Date created" orders the list by creation time,
   newest first, and the direction button switches to oldest first (acceptance
   criterion 6). Reloading the page restores the same order from
   `sort=createdDesc` in the URL, and both the library and folder screens offer it
   (requirement 5). The "Date modified" order is the same as before (acceptance
   criterion 8). A group card appears at the position of its members' newest
   creation time.
10. **Action priority (list)**: adding a sort kind does not move or resize the
    search, filter, zoom or direction button. Opening the menu and pressing once
    gives the creation-date order, and one press of the button to its right
    changes the direction.
11. **Guests**: logged out, the same video shows Edited and Created with the same
    values as for the owner. The sort menu has "Date created" and no "Recently
    played", for 7 kinds.
12. **What does not satisfy the requirement** (`UI品質`): date facts that always
    show a time and are longer than the date added; three date icons that are the
    same or differ only by a small calendar glyph; "Updated" in the facts row
    alongside "Date modified" in the sort, with explanatory text needed to say
    which one is the file; Edited or Created on list cards or list-view rows; a
    changed name or position for the existing "Date modified"; a new sort kind
    with an annotation such as "Date created (file)".

## Colour

No new pair is used. Date text is `fg-muted` on `bg`, like the current facts row,
and icons are `fg-subtle` (used only for glyphs, never for text). Menu items stay
as the current `ui/Menu`. No new token is added, and nothing is added to `pairs`
in `tokens.test.ts`.

## Accessibility

The parent Issue does not ask for screen-reader or contrast design, so only names
are decided here (the same scope as 012, 029 and 030).

| Element | Decision |
| --- | --- |
| Hidden names of the facts | "Edited" and "Created". The value is read from the date text after them. The `title` takes the form "Edited Sep 27, 2026, 3:04 PM", and the date added changes to "Added …" to match. The press-to-open popover shows the same text (reachable by touch and keyboard; see "Video facts") |
| Sort radio | Name "Date created". The direction button's accessible name keeps the current form "Descending (newest first). Press for ascending" |
