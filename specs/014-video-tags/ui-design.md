# UI design: Video tags and tag filtering

**Feature**: [parent Issue #193](https://github.com/syudead/vv/issues/193)

Sources: visual rules, the shell and list density follow
[Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md)
and the `@theme` in [`web/src/index.css`](../../web/src/index.css). The video
page structure follows [012's ui-design.md](../012-video-detail-ia/ui-design.md);
the toolbar, the no-match state and `条件を解除` follow
[013's ui-design.md](../013-library-search/ui-design.md). The URL shape and
history are fixed in [contracts/list-url.md](contracts/list-url.md), and the
error `code`s and counts in [contracts/tags-api.md](contracts/tags-api.md); this
document does not revisit them. It defines only what the card tag row, the
active tag filters, the video page tags, the selection bar tag actions and the
tag management page **add to or change in** existing screens. No new color,
radius or shadow tokens are added.

## Screen boundary

| Place | What this feature adds |
| --- | --- |
| Library (`/`) grid card | A tag row below the title. The library list view rows have no tag row. Folder screen cards have the same tag row; pressing a tag goes to the library filtered by it (`/?tag=<id>`) |
| Top of the library body | Only while filtering by tag, an active tag filter row above the summary line. Nothing is added to the toolbar (inside the top bar) |
| Video page (`/videos/:id`) | The tag list below the title |
| Selection bar | Bulk add and remove tag actions, placed per [library-ui.md, List layout](../../docs/design-docs/library-ui.md#list-layout) |
| Tag management page (`/tags`) | Inside `AppShell`. The sidebar has a `タグ` entry (lucide `Tags`) right after `フォルダ` |

## Tag chip

A tag appears in three forms with different roles. All share one visual
skeleton (`h-5` or `h-6`, `rounded-sm`, `px-1.5` to `px-2`, `text-xs`, one line
with truncation) and are told apart by color and marks.

| Form | Place | Surface and text | Marks | On press |
| --- | --- | --- | --- | --- |
| Video tag | Card, video page | `bg-elevated`, `text-fg-muted` (`text-fg` on the video page) | None | Filter by the tag |
| Active tag filter | Above the list | `bg-accent-soft`, `text-link` (the current `Chip` accent) | Leading lucide `Tag`, trailing `X` | Remove that filter |
| Suggestion, summary | Suggestion list, selection bar | Row form ("Combobox" below) | Wording when only on some | Add or remove |

- A long name is truncated at the end with `min-w-0 truncate` on the chip text,
  and `title` holds the full name. The chip's maximum width is the width of its
  row (`max-w-full`), so even a 100-character name does not overflow
  horizontally at 360px.
- Only active tag filters carry the accent color. Video tags stay neutral, so
  color also tells which tags the list is filtered by. Color is not the only
  cue: active tag filters also have the `Tag` icon and the wording at the start
  of the row ("Active tag filters" below) (`UI品質` "accessibility").

## Library card

### Tag row

- One line directly below the title `h3`, where the metadata line is now. The
  gap to the title stays the current `gap-1` and the bottom padding the current
  `pb-3` (`UI品質` "spacing rhythm").
- The row is `flex flex-nowrap gap-1 overflow-hidden`. Chips are spaced evenly
  with `gap-1`, in API order (natural order of names).
- The row and the card `article` both clip their content, so the global
  `:focus-visible` outer outline would be cut off outside the chip. Card chips
  (and `+N`) alone draw the outline inside the chip
  (`focus-visible:outline-offset-[-2px]`). Color and width stay global.
- Chips are `h-5`, `text-xs`: smaller than the title (`text-sm font-medium`)
  and `text-fg-muted`, so they do not catch the eye before the title. On a
  watched card the title is also `text-fg-muted`, but size and the chip surface
  still tell them apart.
- A video with no tags has no row; there is no "no tags" text. An empty row
  would convey the absence of a category with blank space, which conveys
  nothing. Cards in each grid row stay equal in height as now (the default
  `stretch` of `flex-wrap`).
- The row sits at the bottom of the card, so tag rows line up across a grid row
  even when titles take one or two lines. To do this the card link, not the
  row, grows with `flex-1`, and the space between the title and the row belongs
  to the link area (press to open or select).

### Overflow

- Tags that do not fit on one line are replaced by a `+N` chip at the end of the
  row; only the chips that fit are shown (`UI品質` "information density"). `+N`
  has the video tag form, with `tabular-nums` text. A fade-out is rejected: it
  does not show whether a name was cut or more tags exist.
- `+N` is a button. It opens a `ui/Popover` listing the hidden tags vertically as
  the same chips. Pressing a chip filters by that tag, as in the card row, and
  closes the popover. Its accessible name is `ほかのタグ N 個を表示`.
- How many chips fit is decided after rendering by measuring the row and chip
  widths; CSS alone cannot produce the count. library-ui.md, [Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception) avoids branching
  on screen width in JavaScript, which is different from measuring whether
  content fits. To keep the row from growing and shrinking for one frame before
  measurement, the count is decided before paint (layout effect) and remeasured
  when the card size or the screen width changes. Instead of one observer per
  card, one `ResizeObserver` on the list watches the rows of the drawn cards
  (library-ui.md, [Virtual scrolling of long lists](../../docs/design-docs/library-ui.md#virtual-scrolling-of-long-lists)).
  Card width is constant per zoom level, so remeasurement runs only when the
  zoom level or screen width changes, or tags change.
- The list keeps the count per row width and per chip-width inputs (name,
  tentative mark, folder-derived mark), so a card drawn again after scrolling
  away takes the count without measuring its chips.

### Card structure and pressing

- The whole card is currently one link to the video page. The tag row sits
  **outside** the link, inside the same `article` below the title (no
  interactive element nested in the link). It still looks like one card, with
  no dividing line.
- The card's hover lift, shadow and hover preview work over the tag row as now.
- Each tag chip is a button. Pressing it adds the tag to the current conditions
  ([list-url.md, Adding and removing a tag](contracts/list-url.md#adding-and-removing-a-tag)); it does
  not go to the video page. Pressing a tag that is already an active filter
  changes nothing, and the chip looks like the others (the active mark appears
  only above the list).
- Pressing a 17th tag while 16 are active does not add it; a toast says
  `絞り込めるタグは 16 個までです`.
- **While selecting** (one or more items selected), tag chips are not drawn as
  buttons. Text and surface stay, and pressing toggles the card's selection
  (Edge Case `選択中にタグを押したとき`). They also leave the Tab order.
  Announcing "filter by" with the tag name while selecting would contradict
  what pressing does. `+N` also becomes non-interactive.
- Keyboard order within a card: checkbox → link (title) → tags → `+N`.

## Active tag filters

- One row at the top of the library body, above the summary line, shown only
  while filtering by tag. Without a tag filter it takes no space.
- The row is centered like the summary line
  (`flex flex-wrap justify-center items-center gap-1.5`). It starts with the
  wording `タグで絞り込み中` in `text-xs text-fg-muted`, followed by the active
  tags in the natural order of names, wrapping when they do not fit.
- Each chip is one button with the `Tag` icon, the name and the `X` icon;
  pressing anywhere removes only that tag (`UI品質` `ワンクリックで届き`). Making
  only the × a small target is rejected: the whole chip has one action, so
  there is no reason to shrink the target. On hover an inner
  `ring-1 ring-inset ring-border-strong` edge appears (as on video tag chips).
  Changing the text color is rejected: `accent-hover` and `link` have the same
  value, so nothing would change visibly.
- After removal, focus moves to the next chip, else the previous chip, and to
  the search box when it was the last. The removed chip disappears and would
  take the focus target with it (as with 013's no-match state).
- Hierarchy: the row is in the body below the toolbar and smaller than toolbar
  parts (`h-6`, `text-xs`). It is the only thing with an accent surface that
  catches the eye before the grid, so the current conditions read at a glance.
- Before the tag list has loaded, each chip holds a `Skeleton` (`w-12`) instead
  of the name, one per `id` in the URL, replaced by the name once loaded.
- The filter menu's `条件を解除` (013 "Filter menu") also appears while
  filtering by tag, and pressing it removes the tag filters too. Tags are not
  counted in the filter button's number
  ([list-url.md, Adding and removing a tag](contracts/list-url.md#adding-and-removing-a-tag)).
- The summary line keeps its format and does not include tag names. Its count
  is `total` including `tag`.
- The no-match state (013 "No-match state"), per 013's current contract (PR
  #262), has no chips or notes for the search term, filters or search scope,
  and no `条件を解除` button. Tag filters are treated the same: no chips or
  remove actions are added inside the no-match state. Active tags are visible
  only in this row above the no-match state, and are removed here or through
  the filter menu's `条件を解除` (which removes everything, tags included).
- Changing the search term, watch state, playability or tags clears the
  selection, so videos selected in a list with different conditions do not stay
  selected while hidden.

## Video page tags

### Placement

- Directly below the title (`h1`). Tags categorize the video and are read with
  the title. Cards also show them directly below the title, so the order is the
  same when opened from the list.
- The title and the tag list form one group (`flex flex-col gap-2`), and the
  gap between the group and the divider is the current `gap-5`. A `gap-5`
  between title and tags would make the tags look like they belong to the
  attributes.
- 012's "no chips or buttons below the title" (Visual review criteria,
  "information density") is revised by this feature for the tag list only. The
  summary line, buttons and other chips below the title stay absent.
- Hierarchy is kept by the small chips (`h-6`, `text-xs`) and the neutral
  surface; they do not catch the eye before the title (`text-xl` to `text-2xl`,
  `font-semibold`) (`UI品質` "visual hierarchy").
- No label (such as `TAGS`). The chip form shows they are tags, and a label line
  directly below the title would clutter it. The group's heading is a visually
  hidden `h2` `タグ`.
- The list is `flex flex-wrap items-center gap-1.5`. After the attached tag
  chips, the `タグを追加` input sits on the same line. The line wraps.

### Chip

- `h-6`, `text-xs`, `bg-elevated`, `text-fg`. Split into a name part (link) and
  a × part (button), with a `bg-border-strong` vertical rule (`w-px h-3.5`)
  between them.

| Part | Element | On press | Accessible name |
| --- | --- | --- | --- |
| Name | `Link` to `/?tag=<id>`. A link, not a button, because it navigates, and it can open in a new tab (as the name on the management page) | Opens the library list filtered by this one tag | `〈名〉で絞り込む` |
| × | `size-6` square, lucide `X` (`size-3`) | Removes the tag from this video | `〈名〉をこの動画から外す`, distinct by wording from the active filter's `〈名〉の絞り込みを外す` (`UI品質` "accessibility") |

- After removal, focus moves to the next chip's ×, else the previous chip's ×,
  and to the `タグを追加` input when it was the last.
- Chips are added or removed after the server responds. While waiting, that ×
  is `disabled` and the input is in the submitting state described in "Combobox".

### Add input

- The component is "Combobox" below. The input is `h-6`, `w-40`, `bg-field`,
  `border-border`, `rounded-sm`, `text-xs`, with lucide `Plus` (`size-3`,
  `text-fg-muted`) on the left and the placeholder `タグを追加`. On focus,
  `border-accent` (as the search box).
- On commit the input clears and keeps focus, so another tag can be added
  right away. The new chip takes its place in the name order.
- While the input has focus, the video page keys (Space, ←→, F, M, 0, Esc) do
  not act (the current `isEditable` in `keyboard.ts`). Esc closes the
  suggestion list when open, otherwise clears the input; Esc on an empty input
  does nothing. Only Esc pressed outside the input closes the page.

### States

| State | What the screen shows |
| --- | --- |
| Details loading | No tag list (inside the skeleton, as the title) |
| No tags | Only the `タグを追加` input below the title |
| Add or remove failed | One `text-xs text-danger` line (`role="alert"`) right below the list: `タグを付けられませんでした` or `タグを外せませんでした`. The input text stays and Enter retries. Cleared by the next action |
| Tag no longer exists (`tag_not_found`) | Toast `タグ「〈名〉」はもう無いため、一覧を取り直しました`. The tag list and this video are refetched |

## Combobox

The video page's `タグを追加`, the selection bar's `タグを付ける` and
`タグを外す`, and the merge target picker on the management page use one
component (`web/src/ui/Combobox.tsx`): an ARIA 1.2 combobox (input + listbox,
`aria-activedescendant`).

- The suggestion list appears below the input (above it in the selection bar)
  with the same surface as `ui/Popover` (`bg-elevated`, `shadow-elevated`,
  `rounded-md`). Width `w-64`; it scrolls beyond eight rows. It opens on focus
  and shows every suggestion when the input is empty.
- Only the rows in view, plus a few on each side, are drawn, so a list of
  thousands of tags opens and narrows without a pause. The active row stays
  drawn after it scrolls out of view, so `aria-activedescendant` always points
  at an element. Each row carries `aria-setsize` and `aria-posinset`, so a
  screen reader announces its position in the whole list.
- Suggestion rows are `h-8`, `px-2.5`, `text-sm`. The name is `text-fg`,
  truncated to one line, with secondary information on the right in
  `text-xs text-fg-muted tabular-nums` (the count, as on the management page;
  for remove suggestions in the selection bar, the "some" mark below). The
  active row is `bg-hover-wash` (as menu items).
- Filtering: keep tags whose name or a synonym contains the input,
  case-insensitively (as the management page search). Prefix matches first,
  then the natural order of names. A tag matched by a synonym shows
  `シノニム: アニメ` below the name in `text-xs text-fg-muted`. The name shown is
  always the original name (requirement 7).
- Tags already attached to the video are not suggested for `タグを追加`.
- When no name or synonym matches the input's spelling exactly, the list ends
  with a `「〈入力〉」を作成` row (lucide `Plus`). With `Anime` present, typing
  `anime` shows this row (requirement 4). With an exact match it does not
  appear.

Keys:

| Key | Action |
| --- | --- |
| ↓, ↑ | Move between rows; stop at the ends |
| Enter, a row active | Commit that row |
| Enter, no row active | Commit the input's spelling (with leading and trailing whitespace removed): the tag whose name or synonym matches, or else a new tag. Forms without a create row (the selection bar's `タグを外す`, the merge target) commit only when the spelling exactly matches a suggested name or synonym, and otherwise do nothing |
| Tab | Close the list and move on without committing |
| Esc | Close only the list when it is open. In a combobox inside a popover or dialog, with the list closed, close only that popover or dialog. Either way Esc stops there and does not reach the outside (clearing the selection, closing the video page). Esc in the video page input works as in "Add input" above |

- Name validation (requirement 4): checked on every input. When the name cannot
  be created, one `text-xs text-danger` line below the input gives the reason
  and Enter is not accepted. The reasons are `改行やタブは使えません` and
  `100 文字以内にしてください（今 101 文字）`. Empty or whitespace-only input
  shows no reason while typing (an empty input after a commit or on opening is
  normal); only Enter in that state shows `名前を入力してください`. Characters
  are counted as code points of the name with leading and trailing whitespace
  removed, matching the server ([data-model.md, Name rules](data-model.md#name-rules)),
  not as JavaScript `length` (UTF-16 units). The input has no `maxLength`: it
  would silently drop the 101st character with no way to give a reason. The
  input's `aria-describedby` points at the reason line.
- Newlines cannot be found in the input's value. A single-line `input` strips
  newlines (LF, CR) before they reach the value, so pasting `旅行\n2024` yields
  `旅行2024` and commits a different name. The original string is therefore
  checked at paste (`clipboardData` of `paste`) and drop (`insertFromDrop` of
  `beforeinput`). When it contains a newline the input is not taken in
  (`preventDefault`), the value stays as it was, and the reason
  `改行やタブは使えません` appears. The reason stays until the input next
  changes, and until then neither Enter nor the commit buttons (`作成`, `追加`
  and so on) are accepted: the name from before the paste is still in the
  input, and committing would commit that name instead of the one the user
  pasted. When the input changes, the reason clears and the new value goes
  through the validation above before a commit is accepted. Taking in only the
  part without newlines is rejected: a name different from the one pasted would
  silently enter the input. Tabs and other control characters remain in the
  value and are caught by the per-input validation. This handling is the same in
  every input where a tag name is typed ("Combobox", and create, rename and add
  synonym on the management page).
- While submitting, the input has `aria-busy="true"` and does not accept Enter.
  A `LoaderCircle` (`size-3`, `animate-spin`) shows at the input's right end.

## Selection bar

The two tag actions are not combined into one menu, because at narrow widths
that would add one step.

### Add

- Pressing it opens a `ui/Popover` above the bar (the current `PopoverContent`
  takes no direction, so it gains a `side` prop to open upward) containing
  "Combobox". Focus moves to the input. Suggestions are all tags, each with its
  count on the right. On commit the tag is added to every selected video, the
  popover closes, and a toast says `N 件に「〈名〉」を付けました`. Focus returns
  to `タグを付ける`.
- The selection remains after adding, so the user can go on adding or removing
  other tags.

### Remove

- Pressing it opens the same kind of popover and fetches the tag summary of the
  selected videos. Until it arrives, the popover shows `読み込み中…`
  (`text-xs text-fg-muted`, `role="status"`).
- The content is "Combobox" without the create row; suggestions are only the
  summary's tags. On the right of each row, a tag on every video shows `3 件`,
  and a tag on only some shows lucide `CircleDashed` (`size-3`) and
  `一部 1 / 3 件` (requirement 2). The color does not change: the word `一部`
  and the icon tell them apart, and the row's accessible name includes
  `一部の動画だけ、3 件中 1 件`.
- On commit the tag is removed from every selected video, and a toast says
  `N 件から「〈名〉」を外しました`.
- When none of the selected videos has a tag, the popover shows
  `選んだ動画にタグはありません`.
- When the summary cannot be fetched, the popover shows one
  `text-xs text-danger` line (`role="alert"`) `タグを取得できませんでした` and a
  ghost `sm` `Button` `再試行`. The selection remains.

### Failure

- On failure the popover stays open with one `text-xs text-danger` line
  (`role="alert"`) below the input: `付けられませんでした。もう一度お試しください`
  or `外せませんでした。もう一度お試しください`. The selection and the input
  text remain, and Enter retries (Edge Case `一括操作の途中失敗`).
- `tag_not_found` shows the toast
  `タグ「〈名〉」はもう無いため、一覧を取り直しました`, and the popover's
  suggestions are rebuilt from a refetched tag list. The selection remains.

## Tag management page

### Layout

- The same body width and padding as the settings page
  (`mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-8`). From the top: `h1` `タグ`,
  the action row, the count line and the tag list. Nothing goes in the top bar.
- The action row has a search input on the left (the same look as the library
  `SearchBox`; placeholder and accessible name `タグを検索`;
  `flex-1 sm:max-w-sm`) and a secondary `Button` `新しいタグ` (lucide `Plus`) on
  the right. The search input is the first Tab stop in the body (`UI品質`
  "on management pages the search input is reachable by keyboard alone"). The
  `/` key moves to the search input, as in the library search box.
- The count line is `text-xs text-fg-muted tabular-nums`, `role="status"`,
  `aria-live="polite"`: `12 個のタグ`, or `3 / 12 個のタグ` while searching.

### Rows

- The list is rows with `divide-y divide-border` (as the settings page media
  folders), with no frame or card around it. Each row is `py-2`, dense enough to
  survey many tags (`UI品質` "information density").
- From the left a row has the name column (`flex-1 min-w-0`), the count and the
  actions.

| Part | Form |
| --- | --- |
| Name | `text-sm font-medium text-fg`, one line truncated, full name in `title`. A link to `/?tag=<id>` (requirement 6, "navigation"); `text-link` on hover. Accessible name `〈名〉で絞り込んだライブラリを開く` |
| Synonyms | One `text-xs text-fg-muted` line below the name, `シノニム: anime · アニメ`. No line when there are none. Truncated to one line, full text in `title` |
| Count | `text-sm text-fg-muted tabular-nums`, right-aligned `w-16`, `12 本`. Zero shows as `0 本` |
| Actions | `IconButton`s (ghost, `sm`) `改名` (`Pencil`) and `シノニム` (lucide `Tags`), then an `その他の操作` menu (lucide `Ellipsis`) holding `別のタグへ統合…` (`Merge`), a divider, and `削除…` (`Trash2`, `text-danger`) |

- Delete and merge cannot be undone, so they are in the menu rather than on the
  row (`UI品質` "make delete and merge less prominent than create and rename").
  The settings page media folders show delete on the row, but those rows have
  only two actions; here there are four, so keeping the row light wins.
- Hierarchy: the name is primary; the count and synonyms are secondary
  (`UI品質` "visual hierarchy").

### Create and rename

- `新しいタグ` inserts an input row at the top of the list and moves focus to
  it. The input validates and shows reasons as "Combobox" does, without a
  suggestion list. Enter creates; Esc or `キャンセル` closes. On the row's
  right are a primary `sm` `Button` `作成` and a ghost `sm` `キャンセル`. Once
  created, the row takes its place in name order and focus moves to its name.
- `改名` replaces the row's name with the same input, with the current name
  selected and focused. Enter commits; Esc reverts. After commit, focus returns
  to `改名`.
- When the name collides with an existing name or synonym (`tag_name_taken`),
  the server's `message` (of the form `「anime」は「Anime」のシノニムです`)
  appears below the input in `text-xs text-danger`, and the input stays.

### Synonyms

- `シノニム` opens a `ModalFrame` dialog `「Anime」のシノニム`.
  - At the top, the current synonyms as `Chip`s with an added ×. The dialog
    surface is `bg-elevated`, the same color as a neutral `Chip`, which would
    make the chip surface vanish, so these chips alone use `bg-bg` (`fg` on
    `bg` is in `pairs`). The ×'s accessible name is
    `シノニム「アニメ」を解除`. Removal happens immediately without
    confirmation (it only frees the name; video tags do not change).
  - Below, a `シノニムを追加` input (validated as "Combobox", no suggestions)
    and a secondary `Button` `追加`.
  - When the name collides with another tag's synonym, the `message` (which tag
    it is a synonym of) appears below the input (Edge Case `シノニム名の衝突`).
- When the name to add is an existing tag's name (requirement 6, "adding a
  synonym"), the same dialog switches to a confirmation.
  - Wording:
    `「anime」は 10 本の動画に付いているタグです。「Anime」に統合すると、その 10 本に「Anime」が付き、「anime」とそのシノニムは「Anime」のシノニムになります。「anime」はタグの一覧から消えます。`
    For a tag without synonyms, `とそのシノニム` is omitted (Edge Case
    `統合とシノニム`). The count is `videoCount` from `GET /api/tags`
    (acceptance criterion 17).
  - Actions: secondary `Button` `戻る` (initial focus) and danger `統合する`.
    `戻る` returns to the input without changes, keeping the input text.
  - On `tag_merge_required`, the tag list is refetched and the confirmation is
    shown again with the new count.

### Merge and delete

- `別のタグへ統合…` opens a `ModalFrame` dialog `「X」を統合`. The source is the
  row's tag; the target is chosen in "Combobox" (every tag except the source,
  no create row). Once chosen, the confirmation
  `「X」が付いた 5 本の動画に「Y」が付きます。「X」とそのシノニムは「Y」のシノニムになり、「X」はタグの一覧から消えます。この操作は取り消せません。`
  appears below. Actions are secondary `キャンセル` and danger `統合する`;
  `統合する` is `disabled` until a target is chosen.
- `削除…` opens a dialog `「X」を削除` with the wording
  `12 本の動画からこのタグが外れます。この操作は取り消せません。`
  (acceptance criterion 11), or `このタグはどの動画にも付いていません。` for 0
  videos. Actions are secondary `キャンセル` (initial focus) and danger
  `削除する`.
- While running, both buttons are `disabled` and the danger button shows a
  `LoaderCircle` (as the folder change confirmation on the settings page).
- When done, the dialog closes and a toast says `統合しました` or `削除しました`.
  Focus moves, after a merge, to the target row's name; after a delete, to the
  next row's `改名`, else the previous row's, else `新しいタグ` when no rows
  remain.

### States

| State | What the screen shows |
| --- | --- |
| Loading | Count line `読み込み中…`; six `Skeleton` rows (`h-10`) in place of the list |
| Load failed | `EmptyState` (danger, `AlertCircle`) `タグを取得できません`, `Button` `再試行` |
| No tags | `EmptyState` (lucide `Tags`) `タグはまだありません`, description `動画の再生画面や、ライブラリの選択バーから付けられます。ここで先に作っておくこともできます。`, primary `Button` `新しいタグ`. The search input stays visible and is `disabled` |
| No search match | `EmptyState` (`SearchX`) `「〈入力〉」に一致するタグはありません`, `Button` `検索をクリア`, which clears the input and moves focus to it. Told apart from "No tags" by the heading and icon |
| Action failed | One `text-sm text-danger` line (`role="alert"`) in the dialog or below the input. The dialog and input stay open |
| Tag no longer exists (`tag_not_found`) | The dialog closes and a toast says `このタグはもう無いため、一覧を取り直しました`. The list is refetched |

## Stale tags in other screens

- When the Select all response has `missingTagIds`, no selection is built; the
  deleted tag is removed from the filter and reported. The user presses Select
  all again on the corrected list
  ([tags-api.md, List filter and Select all](contracts/tags-api.md#list-filter-and-select-all)).
- When the library URL's `tag` includes a tag that no longer exists
  (`missingTagIds`, or the check on restoring from a snapshot), a toast says
  `削除されたタグを絞り込みから外しました` and the chip disappears from the
  active tag filter row ([list-url.md, Parameters](contracts/list-url.md#parameters)).
  The removed tag's name may not be known from the tag list, so the wording
  does not include it.
- Tags on cards and the video page may keep old names until the list or video
  is next fetched. When pressing one yields `tag_not_found` or `missingTagIds`,
  the screen reports and corrects it as above.

## Interaction states

The added controls (chips, `+N`, suggestion rows, selection bar buttons, row
actions on the management page) inherit hover, focus-visible, active and
disabled from their base components (`Button` ghost and secondary,
`IconButton`, `ui/Menu` items, `Chip`). No custom state colors are made. A
video tag chip on hover turns its text `text-fg` and shows an inner
`ring-1 ring-inset ring-border-strong` edge. The surface color does not change,
because there is no opaque surface token lighter than `elevated`. Focus is the
global `:focus-visible` (outer outline in `link` color). When a tag inside a
card has focus, the card's outer outline (`has-[a:focus-visible]`) does not
appear, only the chip's, so it is clear whether the link or the tag has focus.

## Accessibility

- Accessible names:

| Control | Name |
| --- | --- |
| Video tag (card, video page, inside `+N`) | `〈名〉で絞り込む` |
| Active tag filter | `〈名〉の絞り込みを外す` |
| × on the video page | `〈名〉をこの動画から外す` |
| `+N` | `ほかのタグ N 個を表示` |
| Name link on the management page | `〈名〉で絞り込んだライブラリを開く` |
| Partial suggestion in the selection bar | `〈名〉、一部の動画だけ、N 件中 M 件` |

- Grouping: card tags are a `ul` with `aria-label="タグ"`, active tag filters a
  `ul` with `aria-label="絞り込み中のタグ"`, and the video page a `ul` under the
  hidden `h2` `タグ`.
- Validation reasons use the input's `aria-describedby`, action failures
  `role="alert"`, and count changes `role="status"` (`polite`).
- Color pairs: of the pairs used for text, `fg` and `fg-muted` on `elevated`
  (chips, suggestions), `fg` on `bg` (synonym chips in the dialog) and `danger`
  on `bg` (reason and failure lines in the video page and management page
  bodies) are already in `pairs` in `tokens.test.ts`. `danger` on `elevated` is
  not in `pairs` (about 5.9 by calculation, which meets 4.5). It is used for
  the reason and failure lines in the selection bar popover and inside the
  management page dialogs (`ModalFrame` is `bg-elevated`); this selection bar
  pair is added to `pairs`. `link` on `accent-soft` for active tag filters is
  the same pair as the current `Chip` accent; `accent-soft` is translucent, so
  it is outside `pairs` (`tokens.test.ts` reads only 6-digit hex values). No new
  tokens are added.
