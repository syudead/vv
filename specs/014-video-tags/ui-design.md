# UI Design: video tags and the tag filter

**Feature**: [parent Issue #193](https://github.com/syudead/vv/issues/193)

This document does not re-decide what other documents own:

| Topic | Source |
| --- | --- |
| Visual rules, shell, list density | [Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md) and `@theme` in [`web/src/index.css`](../../web/src/index.css) |
| Playback page structure | [012's ui-design.md](../012-video-detail-ia/ui-design.md) |
| Toolbar, no-match state, "Clear filters" | [013's ui-design.md](../013-library-search/ui-design.md) |
| URL shape and history | [contracts/list-url.md](contracts/list-url.md) |
| Error `code` values and video counts | [contracts/tags-api.md](contracts/tags-api.md) |

It defines only what the card tag row, active tags, playback page tags, selection bar tag actions
and the tag management page **add to or change in** existing screens. No new color, radius or
shadow tokens are added.

## Screen boundary

- **Grid cards in the library (`/`)**: a tag row sits below the title. Rows in the library list
  view have no tag row. Folder page cards have the same tag row; pressing a tag opens the library
  filtered by it (`/?tag=<id>`).
- **Top of the library content**: only while a tag filter is set, an "active tags" row sits above
  the summary line. Nothing is added to the toolbar (inside the top bar).
- **Playback page (`/videos/:id`)**: the video's tags sit below the title.
- **Selection bar**: has actions to add or remove a tag in bulk. Placement follows
  [library-ui.md §6](../../docs/design-docs/library-ui.md#6-list-layout).
- **Tag management page (`/tags`)**: inside `AppShell`. The sidebar has a "Tags" entry
  (lucide `Tags`) right after "Folders".

## Tag chip

Tags appear in three forms with different roles. All share one visual skeleton (`h-5` or `h-6`,
`rounded-sm`, `px-1.5` to `px-2`, `text-xs`, one line with truncation). Color and marks tell the
roles apart.

| Form | Place | Surface and text | Mark | On press |
| --- | --- | --- | --- | --- |
| Video tag | Card, playback page | `bg-elevated`, `text-fg-muted` (`text-fg` on the playback page) | None | Filter by that tag |
| Active tag | Above the list | `bg-accent-soft`, `text-link` (the accent of the current `Chip`) | lucide `Tag` first, `X` last | Remove that filter |
| Suggestion / summary | Suggestion list, selection bar | Row form ("Combobox" below) | Text when on only some | Add or remove |

- A long name gets `min-w-0 truncate` on the chip text, which truncates the end, and the full
  name in `title`. The chip's maximum width is the width of its row (`max-w-full`), so even a
  100-character name does not overflow horizontally at 360px.
- Only active tags use the accent color. Video tags stay neutral, so color also shows which tags
  filter the list now. Color is not the only cue: active tags also have the `Tag` icon and the
  label at the start of the row ("Active tag filters" below) (UI quality "Accessibility").

## Library card

### Tag row

- One line directly below the title `h3`, where the current meta line sits. The gap to the title
  stays the current `gap-1`, and the bottom padding stays the current `pb-3` (UI quality
  "Spacing rhythm").
- The row is `flex flex-nowrap gap-1 overflow-hidden`. The gap between tags is a constant
  `gap-1`. The order is the API order (natural order of names).
- Both the row and the card `article` clip their content, so the outer outline of the global
  `:focus-visible` would be cut outside the chip. Only card chips (and "+N") draw the outline
  inside the chip (`focus-visible:outline-offset-[-2px]`). Color and width stay global.
- Chips are `h-5` and `text-xs`, smaller than the title (`text-sm font-medium`), and
  `text-fg-muted`, so they do not draw the eye before the title. On a watched card the title is
  also `text-fg-muted`, but size and the chip surface still tell them apart.
- A video with no tags has no row, and no "no tags" text. An empty row would signal "no
  classification" with blank space, which says nothing. Cards in each grid row keep equal height
  as today (the default `stretch` of `flex-wrap`).

### Overflow

- Tags that do not fit in one line are replaced by a "+N" chip at the end of the row; only the
  chips that fit are shown (UI quality "Information density"). "+N" has the video tag form, with
  `tabular-nums` text.
- "+N" is a button that opens `ui/Popover` with the hidden tags as the same chips in a column.
  Pressing a chip filters by that tag, as in the card row, and closes the popover. Its
  accessible name is "Show N more tags".
- The number that fits is measured after render, from the row width and the chip widths; CSS
  alone cannot produce a count. library-ui.md §4 avoids viewport-width branching in JavaScript,
  which is different from measuring whether content fits.
- The count is decided before paint (layout effect), so the row does not jump for one frame. It
  is measured again when the card size setting or the viewport width changes.
- The list does not use virtual scrolling (library-ui.md §3), so every loaded card's row is
  observed. One `ResizeObserver` on the list watches all rows, instead of one per card. Card
  width is fixed per zoom level, so re-measuring runs only when the zoom, the viewport width or
  the tags change.

| | |
| --- | --- |
| **Decision** | Show a "+N" chip for the tags that do not fit. |
| **Why** | It tells the user that more tags exist and how many. |
| **Rejected** | Cutting the row with a fade: the user cannot tell a cut name from the existence of further tags. |

### Card structure and pressing

- Today the whole card is one link to the playback page. The tag row sits **outside** the link,
  below the title inside the same `article` (no interactive element nested in a link). It still
  looks like one card, with no dividing line.
- The card's hover lift, shadow and hover preview work the same over the tag row.
- Each tag chip is one button. Pressing it adds the tag to the current conditions
  ([list-url.md §2](contracts/list-url.md#2-pressing-and-removing-a-tag)). It does not open the
  playback page. Pressing a tag that is already active changes nothing. That chip looks like the
  others (the active mark appears only above the list).
- Pressing a 17th tag while 16 are active does not add it; a toast says "Filter by at most 16
  tags."
- **During selection** (one or more items selected), tag chips are not rendered as buttons. Text
  and surface stay; pressing toggles the card's selection (Edge case "Pressing a tag during
  selection"). They leave the Tab order too, because announcing "Filter by" with the name would
  contradict what the press does. "+N" also becomes non-interactive.
- Keyboard order inside a card: check box → link (title) → tags → "+N".

## Active tag filters

- One row at the top of the library content, above the summary line. It appears only while a tag
  filter is set and takes no space otherwise.
- Like the summary line, the row is centered (`flex flex-wrap justify-center items-center
  gap-1.5`). It starts with the label "Filtering by tags" in `text-xs text-fg-muted`, followed by
  the active tags in natural order of names. It wraps when it does not fit.
- Each chip is one button with the `Tag` icon, the name and the `X` icon. Pressing anywhere on it
  removes only that tag (UI quality "reachable in one click"). On hover, an inner
  `ring-1 ring-inset ring-border-strong` edge appears (the same as video tag chips).
- After a removal, focus moves to the next chip, else the previous chip, else (for the last one)
  the search box. The removed chip disappears and would otherwise leave focus nowhere (the same
  handling as 013's no-match state).
- Hierarchy: the row is in the content below the toolbar and smaller than toolbar controls
  (`h-6`, `text-xs`). Only this row, with its accent surface, draws the eye before the grid, so
  the current condition reads at a glance.
- While the tag list has not loaded, each chip holds a `Skeleton` (`w-12`) instead of the name,
  one per `id` in the URL. The names replace them once loaded.
- The filter menu's "Clear filters" (013 "Filter menu") also appears while a tag filter is set.
  Pressing it also removes the tag filters. The filter button's count does not include tags
  ([list-url.md §2](contracts/list-url.md#2-pressing-and-removing-a-tag)).
- The summary line keeps its current format and does not include tag names. Its count is the
  `total` with `tag` applied.
- Per 013's current contract (PR #262), the no-match state (013 "No-match state") has no chips or
  notes for search terms, filters or search scope, and no "Clear filters" button. Tag filters are
  handled the same way: nothing is added inside the no-match state.
- Active tags are visible only in this row above the no-match state. They are removed here, or
  with "Clear filters" in the filter menu (which removes everything, tags included).
- Changing the search term, watch status, playability or tags clears the selection. Videos
  selected under different conditions must not stay selected while hidden.

| | |
| --- | --- |
| **Decision** | The whole active-tag chip is the remove target, and hover shows an inner ring. |
| **Why** | The chip has only one action, so there is no reason for a small target. |
| **Rejected** | A small target on × only. Changing the text color on hover: `accent-hover` and `link` have the same value, so nothing would change. |

## Video page tags

### Placement

- Directly below the title (`h1`). Tags classify the video and are read together with the
  title. Cards also show them directly below the title, so the order matches when the page is
  opened from the list.
- The title and the tags form one group (`flex flex-col gap-2`), with the current `gap-5`
  between that group and the divider. A `gap-5` between title and tags would make the tags look
  like part of the attributes.
- 012's "no chips or buttons below the title" (Visual review criteria "Information density") is
  changed by this feature for the tag list only. A summary line, buttons or other chips below the
  title are still not placed.
- Hierarchy comes from small chips (`h-6`, `text-xs`) and a neutral surface. They do not draw
  the eye before the title (`text-xl` to `text-2xl`, `font-semibold`) (UI quality "Visual
  hierarchy").
- No label (such as "TAGS"). The chip shape identifies tags, and a label line directly below the
  title would crowd it. The group's heading is a visually hidden `h2` "Tags".
- The list is `flex flex-wrap items-center gap-1.5`. After the attached tag chips, the "Add tag"
  input sits on the same line. The line wraps.

### Chip

- `h-6`, `text-xs`, `bg-elevated`, `text-fg`. It has two parts, the name (a link) and the ×
  (a button), with a `bg-border-strong` vertical line (`w-px h-3.5`) between them.
  - Name part: a link (`Link`) to `/?tag=<id>` that opens the library list filtered by that one
    tag. It is navigation, so it is a link, not a button, and can open in a new tab (as the
    management page name does). Accessible name: "Filter by {name}".
  - × part: a `size-6` square with lucide `X` (`size-3`). Pressing it removes the tag from this
    video. Accessible name: "Remove {name} from this video". The name differs from the active
    tag's "Remove the filter for {name}" (UI quality "Accessibility").
- Right after a removal, focus moves to the next chip's ×, else the previous chip's ×, else (for
  the last one) the "Add tag" input.
- Chips are added or removed after the server responds. While waiting, that × is `disabled` and
  the input is in the "submitting" state below.

### Add input

- The control is the "Combobox" below. The input is `h-6`, `w-40`, `bg-field`, `border-border`,
  `rounded-sm`, `text-xs`, with lucide `Plus` (`size-3`, `text-fg-muted`) on the left and the
  placeholder "Add tag". Focus shows `border-accent` (as the search box does).
- On confirm, the input clears and keeps focus, so the next tag can be added right away. The new
  chip takes its place in name order.
- While the input has focus, the playback page keys (Space, ←→, F, M, 0, Esc) do not act
  (`isEditable` in the current `keyboard.ts`). Esc closes the suggestion list if it is open, and
  otherwise clears the input. Esc in an empty input does nothing. Only Esc pressed outside the
  input closes the page.

### States

| State | Appearance |
| --- | --- |
| Details loading | No tag list (inside the skeleton, like the title) |
| No tags | Only the "Add tag" input below the title |
| Could not add or remove | One `text-xs text-danger` line (`role="alert"`) right below the list: "Couldn't add the tag: {reason}" or "Couldn't remove the tag: {reason}". The input text stays, and Enter retries. The line clears on the next action |
| Tag no longer exists (`tag_not_found`) | Toast: `The tag "{name}" no longer exists, so the tags were reloaded`. The tag list and this video are refetched |

## Combobox

The playback page's "Add tag", the selection bar's "Add tag" and "Remove tag", and the merge
target picker on the management page share one control (`web/src/ui/Combobox.tsx`). It is an
ARIA 1.2 combobox (input plus listbox, `aria-activedescendant`).

- The suggestion list opens below the input (above it in the selection bar) with the same surface
  as `ui/Popover` (`bg-elevated`, `shadow-elevated`, `rounded-md`). Width `w-64`; over 8 rows it
  scrolls. It opens on focus and shows all suggestions when the input is empty.
- Suggestion rows are `h-8`, `px-2.5`, `text-sm`. The name is `text-fg`, truncated to one line.
  Secondary information sits on the right in `text-xs text-fg-muted tabular-nums` (the video
  count, as on the management page; "Some" below for remove suggestions in the selection bar).
  The active row is `bg-hover-wash` (as in menu items).
- Filtering: keep tags whose name or synonym contains the input, case-insensitively (as in the
  management page search). Prefix matches first, then natural order of names. A tag matched by a
  synonym shows `Synonym: アニメ` below the name in `text-xs text-fg-muted`. The name shown is
  always the original name (Requirement 7).
- "Add tag" does not suggest tags already on the video.
- When no name or synonym matches the input exactly, the list ends with a `Create "{input}"` row
  (lucide `Plus`). With `Anime` present, typing `anime` shows this row (Requirement 4). With an
  exact match it is not shown.
- Keys: ↓ and ↑ move between rows and stop at the ends. Enter confirms the active row. Tab closes
  the list and moves on without confirming.
- Enter with no active row confirms the typed spelling (trimmed): the matching tag if a name or
  synonym matches, otherwise a new tag. In forms without a create row (the selection bar's
  "Remove tag", the management page merge target), it confirms only when the spelling exactly
  matches a suggestion's name or synonym, and otherwise does nothing.
- Esc closes only the suggestion list when it is open. In a combobox inside a popover or dialog,
  Esc with the list closed closes only that popover or dialog. In both cases Esc stops there and
  does not reach outer handlers (clearing the selection, closing the playback page). Esc in the
  playback page input follows "Add input" above.

Name validation (Requirement 4):

- The name is checked on every input. An invalid name shows one reason line below the input in
  `text-xs text-danger`, and Enter is not accepted. The reasons are "Line breaks and tabs aren't
  allowed" and "Use 100 characters or fewer (currently 101)". The reason line is referenced by the
  input's `aria-describedby`.
- Empty and whitespace-only input shows no reason while typing (an empty input right after a
  confirm or on open is normal). Only Enter in that state shows "Enter a name".
- Length counts the code points of the trimmed name, matching the server
  ([data-model.md §2](data-model.md#2-name-rules)). It does not use JavaScript `length` (UTF-16
  units). The input has no `maxLength`: it would drop the 101st character silently, with no
  reason shown.

Line breaks cannot be found in the input value. A one-line `input` strips LF and CR before they
reach the value, so pasting `旅行\n2024` gives the value `旅行2024`, which would be confirmed as a
different name. The original string is therefore checked at paste time (`clipboardData` of
`paste`) and drop time (`insertFromDrop` of `beforeinput`):

- If it contains a line break, the input is not taken in (`preventDefault`). The value stays, and
  the reason "Line breaks and tabs aren't allowed" appears.
- The reason stays until the input changes. Until then, neither Enter nor the confirm button
  ("Create", "Add" and so on) is accepted. The name from before the paste is still in the input,
  so without this block the old name would be confirmed instead of the pasted one.
- When the input changes, the reason clears, and the new value goes through the validation above
  before a confirm is accepted.
- Tabs and other control characters stay in the value, so the per-input validation catches them.
- This handling is the same in every input that takes a tag name ("Combobox", and create, rename
  and add synonym on the management page).
- While submitting, the input has `aria-busy="true"` and does not accept Enter. Visually, a
  `LoaderCircle` (`size-3`, `animate-spin`) appears at the right end of the input.

| | |
| --- | --- |
| **Decision** | A paste or drop with a line break is not taken in, and the reason is shown. |
| **Why** | The user sees why the name was refused, and no other name is entered silently. |
| **Rejected** | Taking in only the part without line breaks: a name different from the pasted one would enter the input silently. |

## Selection bar

| | |
| --- | --- |
| **Decision** | The two tag actions are separate buttons, not one menu. |
| **Why** | One menu would add one more step, at narrow widths only. |
| **Rejected** | One menu for both actions. |

### Add

- Pressing it opens `ui/Popover` above the bar with a "Combobox" inside. The current
  `PopoverContent` takes no direction, so it gains a `side` prop to open upward. Focus moves to
  the input.
- The suggestions are all tags, each with its video count on the right. On confirm, the tag is
  added to every selected video, the popover closes, and a toast says
  `Added "{name}" to N videos`. Focus returns to "Add tag".
- The selection stays after adding, so another tag can be added or removed next.

### Remove

- Pressing it opens the same kind of popover and fetches the tag summary of the selected videos.
  Until it arrives, the popover shows "Loading…" (`text-xs text-fg-muted`, `role="status"`).
- The content is the "Combobox" form without a create row, with only the summary's tags as
  suggestions. On the right of each row, a tag on every video shows "3 videos". A tag on only
  some shows lucide `CircleDashed` (`size-3`) and "Some: 1 / 3" (Requirement 2).
- The color does not change. The "Some" text and the icon mark the difference, and the row's
  accessible name includes "only some videos, 1 of 3".
- On confirm, the tag is removed from every selected video and a toast says
  `Removed "{name}" from N videos`.
- If none of the selected videos has a tag, the popover shows "The selected videos have no tags
  that can be removed".
- If the summary fails, the popover shows one `text-xs text-danger` line (`role="alert"`)
  "Couldn't load the tags" and a `Button` (ghost, `sm`) "Retry". The selection stays.

### Failure

- On failure, the popover stays open and one `text-xs text-danger` line (`role="alert"`) appears
  below the input: "Couldn't add the tag: {reason}" or "Couldn't remove the tag: {reason}". The
  selection and the input text stay, and Enter retries (Edge case "Partial failure of a bulk
  operation").
- On `tag_not_found`, a toast says `The tag "{name}" no longer exists, so the tags were
  reloaded`, and the popover's suggestions are rebuilt from the refetched tag list. The selection
  stays.

## Tag management page

### Layout

- The same content width and padding as the settings page (`mx-auto max-w-4xl px-4 py-6
  sm:px-6 sm:py-8`). From the top: `h1` "Tags", the action row, the count line, the tag list.
  Nothing goes in the top bar.
- The action row has a search input on the left and a `Button` (secondary) "New tag" (lucide
  `Plus`) on the right. The search input looks like the library `SearchBox`, with placeholder
  and accessible name "Search tags", and `flex-1 sm:max-w-sm`.
- The search input is the first Tab stop in the content (UI quality "on the management page, the
  search input is reachable by keyboard alone"). The `/` key moves to it, as in the library
  search box.
- The count line is `text-xs text-fg-muted tabular-nums`, `role="status"`,
  `aria-live="polite"`: "12 tags", and "3 of 12 tags" while searching.

### Rows

- The list is rows with `divide-y divide-border` (as for media folders on the settings page),
  with no frame or card. Each row is `py-2`, dense enough to scan many tags (UI quality
  "Information density").
- A row has, from left: the name column (`flex-1 min-w-0`), the video count, the actions.
  - Name: `text-sm font-medium text-fg`, one line with truncation, full name in `title`. A link
    to `/?tag=<id>` (Requirement 6 "Navigation"). `text-link` on hover. Accessible name: "Open
    the library filtered by {name}".
  - Synonyms: one `text-xs text-fg-muted` line below the name, such as
    `Synonyms: anime · アニメ`. No line when there are none. One line with truncation, full text
    in `title`.
  - Count: `text-sm text-fg-muted tabular-nums`, right-aligned, `w-16`: "12 videos". Zero shows
    "0 videos".
  - Actions: `IconButton` (ghost, `sm`) "Rename" (`Pencil`) and "Synonyms" (lucide `Tags`), then
    a "More actions" (lucide `Ellipsis`) menu. The menu holds "Merge into another tag…"
    (`Merge`), a divider, and "Delete…" (`Trash2`, `text-danger`).
- Hierarchy: the name is primary; the count and synonyms are secondary (UI quality "Visual
  hierarchy").

| | |
| --- | --- |
| **Decision** | Delete and merge sit in the menu, not directly on the row. |
| **Why** | Both cannot be undone (UI quality "make delete and merge less prominent than create and rename"). This page has four row actions, so the row stays light. |
| **Rejected** | Showing delete on the row, as media folders on the settings page do: that page has only two row actions. |

### Create and rename

- "New tag" inserts an input row at the top of the list and moves focus to it. The input uses
  the same validation and reason display as "Combobox", without a suggestion list. Enter
  creates; Esc or "Cancel" closes. On the right of the row: `Button` primary `sm` "Create" and
  ghost `sm` "Cancel". After creation, the row moves to its name-order position and focus moves
  to its name.
- "Rename" replaces the row's name with the same input, with the current name selected and
  focused. Enter confirms; Esc reverts. After confirming, focus returns to "Rename".
- On a clash with an existing name or synonym (`tag_name_taken`), the server `message` appears
  below the input in `text-xs text-danger` (such as `"anime" is already a synonym of the tag
  "Anime".`), and the input stays.

### Synonyms

- "Synonyms" opens a `ModalFrame` dialog `Synonyms of "Anime"`.
  - At the top, the current synonyms are listed as `Chip` with an added ×. The dialog surface is
    `bg-elevated`, the same color as a neutral `Chip`, so the chip surface would vanish; only
    these chips use `bg-bg` (`fg` on `bg` is in `pairs`).
  - The × accessible name is `Remove the synonym "アニメ"`. Removal is immediate, without
    confirmation: it only frees the name, and the videos' tags do not change.
  - Below: an "Add synonym" input (same validation as "Combobox", no suggestions) and a `Button`
    (secondary) "Add".
  - On a clash with another tag's synonym, the `message` (which tag's synonym it is) appears
    below the input (Edge case "Synonym name clash").
- When the name to add is an existing tag's name (Requirement 6 "Synonym registration"), the same
  dialog switches to a confirmation.
  - Text: `"anime" is a tag on 10 videos. Merging it into "Anime" adds "Anime" to those videos,
    and "anime" and its synonyms become synonyms of "Anime". "anime" leaves the tag list.` For a
    tag without synonyms, "and its synonyms" is omitted (Edge case "Merge and synonyms"). The
    count is `videoCount` from `GET /api/tags` (Acceptance criterion 17).
  - Actions: `Button` secondary "Back" (initial focus) and danger "Merge". "Back" returns to the
    input without changes and keeps the input text.
  - On `tag_merge_required`, the tag list is refetched and the confirmation is shown again with
    the new count.

### Merge and delete

- "Merge into another tag…" opens a `ModalFrame` dialog `Merge "X"`. The source is the row's
  tag. The target is picked with "Combobox" (all tags except the source, no create row).
- After a pick, a confirmation appears below: `The 5 videos tagged "X" get the tag "Y". "X" and
  its synonyms become synonyms of "Y", and "X" leaves the tag list. This can't be undone.`
  Actions: secondary "Cancel" and danger "Merge". "Merge" is `disabled` until a target is
  picked.
- "Delete…" opens a dialog `Delete "X"`. Text: "This tag will be removed from 12 videos. This
  can't be undone." (Acceptance criterion 11). With 0 videos: "This tag isn't on any videos."
  Actions: secondary "Cancel" (initial focus) and danger "Delete".
- While running, both buttons are `disabled` and the danger button shows `LoaderCircle` (as in
  the folder change confirmation on the settings page).
- When done, the dialog closes and a toast says `Merged "X" into "Y"` or `Deleted "X"`. Focus
  moves to the target row's name after a merge. After a delete it moves to the next row's
  "Rename", else the previous row's, else "New tag" when no rows remain.

### States

| State | Appearance |
| --- | --- |
| Loading | Count line "Loading…"; 6 rows of `Skeleton` (`h-10`) in place of the list |
| Load failed | `EmptyState` (danger, `AlertCircle`) "Couldn't load the tags", `Button` "Retry" |
| No tags | `EmptyState` (lucide `Tags`) "No tags yet", description "Add tags from a video's playback page or the library's selection bar. You can also create them here first.", `Button` primary "New tag". The search input stays visible and is `disabled` |
| No search match | `EmptyState` (`SearchX`) `No tags match "{input}"`, `Button` "Show all tags". Pressing it clears the input and moves focus to it. The heading and icon set it apart from "No tags" |
| Action failed | One `text-sm text-danger` line (`role="alert"`) in the dialog or below the input. The dialog and input stay open |
| Tag no longer exists (`tag_not_found`) | The dialog closes; toast "This tag no longer exists, so the list was reloaded". The list is refetched |

## Stale tags in other screens

- When the "Select all" response has `missingTagIds`, no selection is built. The deleted tags
  are removed from the filter and the user is told. The user presses "Select all" again on the
  corrected list ([tags-api.md §5](contracts/tags-api.md#5-list-filter-and-select-all)).
- When the library URL's `tag` includes a tag that no longer exists (`missingTagIds`, or the
  comparison on restore from the snapshot), a toast says "Removed deleted tags from the filter",
  and the chip leaves the active tags row ([list-url.md §1](contracts/list-url.md#1-parameters)).
  The deleted tag's name may be unknown from the tag list, so the text does not include it.
- Card tags and playback page tags may keep old names until the next list or video fetch. When a
  press then returns `tag_not_found` or `missingTagIds`, the screen reports and corrects it as
  above.

## Interaction states

The added controls (chips, "+N", suggestion rows, selection bar buttons, management row actions)
inherit hover, focus-visible, active and disabled from their base components (`Button` ghost and
secondary, `IconButton`, `ui/Menu` items, `Chip`). No custom state colors are added.

- Video tag chips switch the text to `text-fg` on hover and show an inner
  `ring-1 ring-inset ring-border-strong` edge. The surface color does not change, because there is
  no opaque surface token lighter than `elevated`.
- Focus uses the global `:focus-visible` (outer outline in the `link` color).
- When a tag inside a card has focus, the card's outer outline (`has-[a:focus-visible]`) is not
  shown, only the chip's. This keeps the user from confusing focus on the link with focus on a
  tag.

## Accessibility

- Accessible names:
  - Video tag (card, playback page, inside "+N"): "Filter by {name}"
  - Active tag: "Remove the filter for {name}"
  - × on the playback page: "Remove {name} from this video"
  - "+N": "Show N more tags"
  - Name link on the management page: "Open the library filtered by {name}"
  - Partial suggestion in the selection bar: "{name}, only some videos, M of N"
- Groups: card tags are a `ul` with `aria-label="Tags"`. Active tags are a `ul` with
  `aria-label="Tags in the filter"`. The playback page uses a `ul` under the hidden `h2` "Tags".
- Validation reasons use the input's `aria-describedby`, action failures `role="alert"`, and
  count changes `role="status"` (`polite`).

Color pairs used for text:

| Pair | Where | In `pairs` of `tokens.test.ts` |
| --- | --- | --- |
| `fg`, `fg-muted` on `elevated` | Chips, suggestions | Yes |
| `fg` on `bg` | Synonym chips in the dialog | Yes |
| `danger` on `bg` | Reason and failure lines in the playback page and management page content | Yes |
| `danger` on `elevated` | Reason and failure lines in the selection bar popover and in management page dialogs (`ModalFrame` is `bg-elevated`) | No (computed about 5.9, meets 4.5). The selection bar's use of this pair is added to `pairs` |
| `link` on `accent-soft` | Active tags | Out of scope: the same pair as the current `Chip` accent, and `accent-soft` is translucent (`tokens.test.ts` reads only 6-digit hex values) |

No new tokens are added.
