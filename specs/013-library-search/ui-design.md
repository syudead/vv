# UI Design: Search, filters and sort in the list and folder screens

**Feature**: [parent Issue #195](https://github.com/syudead/vv/issues/195)

Sources: the visual rules, the shell and the list density follow
[Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md)
and `@theme` in [`web/src/index.css`](../../web/src/index.css). The folder screen
layout follows [011's ui-design.md](../011-folder-browser/ui-design.md). The URL
shape and history are fixed in [contracts/list-url.md](contracts/list-url.md) and
the query syntax in [contracts/list-api.md, Query syntax](contracts/list-api.md#query-syntax);
this document does not decide them again. It defines only what the toolbar, the
syntax help, the count, the no-match state and folder-screen search **add to or
change in** the existing screens. No new colour, radius or shadow token is added.

## Screen boundary

- The library (`/`), the top of the folder screen (`/folders`) and each folder
  place the same components (the search field and its help, the filters, and the
  sort and its direction) inside `TopBarPortal`. The folder screen borrows the
  components from `web/src/library/`. The folder screen's toolbar is the library
  toolbar without the view-mode switch, because the folder screen has no list
  view.
- The folder screen's toolbar drops its current right alignment and uses the
  library's layout, "search field first, grouped in the centre". Right alignment
  was the layout from when there was no search field (011 "Toolbar"). With the
  search field added, making it look like the same component of the same product
  wins.
- The grid, the cards, the shell and the scroll ownership do not change.

## Toolbar

The result grid is primary and the toolbar is secondary. In the toolbar only the
search field looks like an input (the `bg-field` frame); everything else stays
the current secondary `Button` and `IconButton`. No row is added.

Left to right, which is also the Tab order:

1. **Search field** (the current `SearchBox`; the width rules `min-w-20`,
   `sm:min-w-40` and `sm:max-w-md` stay)
2. The button that opens the **syntax help**, placed **inside the right end** of
   the search field frame (see "Syntax help" below)
3. The **filter** button (the current `ListFilter` button, including its count
   of active filters)
4. The **sort** menu button (the current sort name + `ChevronDown`)
5. The **direction** toggle, or **Shuffle** when the sort is random (only one of
   them, in the same place)
6. View mode (library only), card size, and the combined "view and sort" button
   (unchanged)

The per-width breakpoints stay the current ones (Tailwind's default `md`, `lg`
and `xl`).

| Width | Visible |
| --- | --- |
| 360px (below `sm`) | Search field (including the help button), filter, "view and sort" |
| 768px (`md` and up) | The above, plus sort and direction |
| 1024px (`lg` and up) | The above, plus view mode (library only) |
| 1280px (`xl` and up) | The above, plus card size. In grid view the "view and sort" button disappears |

- Below `md`, the sort and direction move into the "view and sort" group (as
  now). The filter stays a separate button at every width, as in the current
  library. Moving the filter into the group too was rejected: its count would be
  hidden in the group, and at 360px the toolbar would no longer show what is being
  filtered. The search field stays at every width.
- What is filtered is visible in three places in the toolbar: the query in the
  search field, watch state and playability in the filter button's
  `bg-accent-soft` and count (the current look), and the sort in the button's
  name.

### Sort and direction

- The sort menu (`MenuRadioGroup` from `ui/Menu`) lists the seven sorts in one
  column, with the names and lucide icons below. The menu button shows only the
  sort name, not the direction. "Recently played" is for the owner only; the
  guest's (a person not logged in) menu has the other six
  ([016's ui-design.md "Guest degradation"](../016-single-account-auth/ui-design.md#guest-degradation)).

| Sort | Name | Icon | Direction when chosen | Ascending / descending wording |
| --- | --- | --- | --- | --- |
| Date added | `追加日` | `CalendarArrowDown` | Descending | `古い順` / `新しい順` |
| Date modified | `更新日時` | `CalendarClock` | Descending | `古い順` / `新しい順` |
| Title | `題名` | `ArrowDownAZ` | Ascending | — |
| Duration | `長さ` | `Timer` | Descending | `短い順` / `長い順` |
| File size | `ファイルサイズ` | `HardDrive` | Descending | `小さい順` / `大きい順` |
| Recently played | `最近再生した順` | `History` | Descending | `前に再生した順` / `最近再生した順` |
| Random | `ランダム` | `Shuffle` | — | — |

- Choosing a sort sets its "direction when chosen". Date added and title get the
  same directions as the current `addedDesc` and `titleAsc`. Keeping the previous
  sort's direction was rejected: moving from date added (descending) to title
  would give Z→A, which is almost never wanted.
- The **direction toggle** is one button right next to the menu button. It looks
  like the menu button (secondary), with no gap between the two and no rounding
  on the touching corners (the same edge treatment as `SegmentedControl`). The
  icon is `ArrowDownWideNarrow` for descending and `ArrowUpNarrowWide` for
  ascending. The accessible name and tooltip take the form `降順（新しい順）。押すと昇順`;
  for sorts without wording, `昇順。押すと降順`.
- For **random**, the same place holds the Shuffle button (lucide `Dices`;
  accessible name and tooltip `並べ直す`). Hiding or disabling the direction button
  was rejected: hiding shifts the toolbar width, and disabling adds one more
  control that cannot be pressed (Q-5). `RefreshCw` is used by the top bar's
  refresh, so Shuffle does not use it.
- Inside the group below `md`, the sorts are listed as radios in two columns, as
  now, with the direction below as a `SegmentedControl` (two options; their
  accessible names include the wording). For random, that place holds a `Button`
  (ghost, `sm`) labelled `並べ直す`.

### Filter menu

The current filter popover (watch state as 2×2 radios, and `再生できるものだけ`)
stays.

- The button at the bottom changes from `絞り込みを解除` to **`条件を解除`**. As in
  [list-url.md, Meaning on the folder screen](contracts/list-url.md#meaning-on-the-folder-screen), it
  removes the query, watch state and playability. It is shown when any of the
  three is active (today only when a filter is active). Pressing it closes the
  popover and returns focus to the filter button.
- The button's count counts only watch state and playability, as now. The query
  is visible in the search field, so it is not counted.

## Syntax help

- The component is `ui/Popover`. The opening button is a small button like
  `IconButton` (`size-6`, lucide `CircleHelp`, `text-fg-muted`; on hover
  `bg-hover-wash` and `text-fg`), placed inside the right end of the search field
  frame. When there is a query, it sits to the right of the current "clear query"
  ×. The `/` key hint appears to the left of the help button only when the query
  is empty and the width is `sm` or more, as now. Placing it as a separate button
  outside the frame was rejected: at 360px it takes about 36px from the search
  field, and it looks like a control unrelated to the search field.
- It takes no space at rest. Click, Enter and Space open it; hover or focus alone
  does not.
- Contents, top to bottom. The popover width is `w-80`, wider than the current
  default (`w-72`); at 360px the current collision handling keeps it on screen.
  1. The heading `検索の書き方` (`text-sm font-medium text-fg`).
  2. A four-row table. The left column shows the example in
     `font-mono text-xs text-fg` (the same monospace as `player/StatusOverlays`),
     the right column the meaning in `text-xs text-fg-muted`.

     | Example | Meaning |
     | --- | --- |
     | `京都 2024` | `空白で区切った語をすべて含む` |
     | `"京都旅行 2024"` | `" で囲んだ部分を、空白ごと1つの語として探す` |
     | `京都 -2023` | `- を付けた語を含むものを除く` |
     | `京都 OR 奈良`, `京都 \| 奈良` | `どちらかを含む。空白より強く結び付く` |

  3. Below a divider (the same `bg-border` line as `MenuSeparator`), two lines in
     `text-xs text-fg-muted`:
     `全角と半角、大文字と小文字、ひらがなとカタカナは区別しません。` and
     `語は先頭から 16 個まで使います。` ([list-api.md, Query syntax, item 7](contracts/list-api.md#query-syntax)).
- Pressing an example does not put it into the search field. Doing so was
  rejected because a single press would accidentally replace the current query.
- Opening keeps focus on the help button (`onOpenAutoFocus` is prevented; the
  precedent is the popover in `web/src/shell/ScanProgressIndicator.tsx`). The
  popover contains no controls, so moving focus inside would make Radix's
  FocusScope trap Tab, and only Esc would get out. Moving focus inside and handling
  Tab ourselves was rejected: the help is read-only secondary information and has
  no reason to take focus.
- The help button points at the popover with `aria-expanded` and
  `aria-controls`, and, while open, at its contents with `aria-describedby`. A
  screen reader reads the contents while focus stays on the button.
- Tab from the button to the next control (the filter button) moves focus out of
  the popover, which closes it. When the popover is open, Esc closes only the
  popover; focus stays on the help button and the query is not cleared
  (acceptance criterion 16). Esc in the search field (clears the query and
  leaves) stays as now. The popover is not modal.

## Result status

- The query is visible in the search field and the results in the list itself,
  so the summary shows only the count (`N件`). It does not repeat the query, the
  loaded range, the total duration, the total size or explanatory words.
- The count has `role="status"` (`aria-live="polite"`), and N is the server's
  `total` (the count of all items with the query and every filter applied).
- When the first page cannot be fetched, no count is shown, only the fetch
  failure message.

## No-match state

One component for the library and the folder screen.

- The heading is `条件に一致する動画はありません`.
- No chips or notes for the query, the filters or the search scope are shown.
- No `条件を解除` button is shown. Conditions are changed from the search field or
  the filter menu.
- On the folder screen with filters only, the child folder group stays at the
  top, and this state appears where the video group would be.
- The empty state for zero registered folders or zero videos appears only when
  there are no conditions at all (as now).

## Folder screen

### Toolbar and scope

- The search field's accessible name is `〈フォルダ名〉の中を検索` in each folder
  and `すべてのフォルダの動画を検索` at the top. The library keeps `動画を検索`. The
  visible placeholder is `このフォルダ内を検索` in each folder and
  `すべてのフォルダを検索` at the top. The wording differs from the library's `検索`
  so that the user knows, before typing, that the scope is different.
- While searching (the query is not empty), the breadcrumb band shows `内を検索中`
  in `text-fg-muted` right after the current location (`フォルダ › movies › A 内を検索中`).
  At the top, `すべてのフォルダを検索中` replaces `フォルダ`. This text is not
  truncated even when the current location's name is truncated to fit. The rule
  that collapses ancestor segments into `…` (four or more segments below `md`)
  stays. The breadcrumb band is sticky, so the scope stays visible while
  scrolling.
- With filters only, nothing is added to the breadcrumb band, because the scope
  stays the current one (direct children).
- `/` and Esc behave as in the library, because the whole search field component
  is the same (acceptance criterion 21).

### Search results

With a query, the content has the same shape as the library list.

- The child folder group and the `動画 N` heading are not shown. Below the
  breadcrumb band come the same count (`N件`) as the library and one grid. A
  visually hidden `h2` `検索結果` replaces the heading.
- The grid and the video cards have the same width, the same `gap-2.5` and the
  same zoom as the library.
- Each card gets a **location line** below the title and above the current
  metadata line (date added · size).
  - lucide `Folder` (`size-3`, `text-fg-subtle`, not read aloud) + one line of
    `text-xs text-fg-muted`. It is smaller than the title (`text-sm font-medium`)
    and in a secondary colour.
  - The content is the path relative to the open folder, joined with `/` (for `y`
    in acceptance criterion 17, `B`). A video directly in the open folder shows
    `このフォルダ`. Omitting the line for direct children was rejected: the absence
    of the line could not be told apart from "no information".
  - In a top-level search, the path starts with the registered folder's display
    name (`movies/2024/京都`). Directly in the registered folder, only the display
    name is shown.
  - When it does not fit, the **start** is truncated and the last folder name
    stays (`…/2024/京都`), as with the path on 011's folder cards. The `title`
    attribute holds the whole path (at the top, from the registered folder's
    path).
  - The line is not a link: a card would have two links and twice the Tab stops.
    The user reaches that place by clearing the query and following the child
    folder cards. This feature adds no direct way to move there from a search
    result (it is not in the Issue's requirements).
  - The card link's accessible name is `〈題名〉、〈置き場所〉`, so that results with
    the same title can be told apart by screen reader.
- Cards in library search results do not get the location line.

### Filter only

With an empty query and only watch state or playability, the current folder
screen layout (child folders → `動画 N`) stays, and only the direct videos are
filtered.

- N in `動画 N` is the `total` after filtering. The child folder group does not
  change.
- No visible summary line is added: shown above the child folders, it would be
  unclear what it counts. The change in count is announced through a visually
  hidden `role="status"` (`aria-live="polite"`) as `直下の動画 N 件`.

### Top level (`/folders`)

- With an empty query, only the registered folder cards are shown, as now. The
  filter, sort and direction buttons are then **disabled** (`disabled`, the
  current components' `opacity-50`), and the filter count is not shown, because
  `watch` and `playable` have no effect even if they stay in the URL
  ([list-url.md, Meaning on the folder screen](contracts/list-url.md#meaning-on-the-folder-screen)).
  - Hiding them was rejected: if the buttons appeared the moment a query was
    typed, the search field in the centred toolbar would shift sideways while
    typing.
  - Q-5: the value of the disabled buttons is that the same components sit in the
    same place as on the other folder screens, and it is visible that entering a
    query enables filtering. The risk of them reading as broken is reduced because
    they sit right next to the search field and the placeholder
    `すべてのフォルダを検索` makes it clear that the condition is typing. Disabled
    buttons are removed from the Tab order.
- In the "view and sort" group below `md`, the sort radios and the direction are
  also disabled at the top with an empty query. Card size stays operable.
- With a query, the results for the whole library are shown, as in "Search
  results" above.

## States

| State | What the screen shows |
| --- | --- |
| Search results loading | `読み込み中…` where the count goes, and 12 video card skeletons in one grid. No folder card skeletons |
| The folder disappeared during a search (404) | The current `このフォルダは見つかりません`. Not the no-match state |
| Search results failed to load, or the next page failed | The same `LoadFailed` and `続きを取得できません` as the library |
| No match | "No-match state" above |

## Interaction states

The added controls (the help button, direction, Shuffle, `条件を解除`) inherit
hover, focus-visible, active and disabled from their base components
(`IconButton`, the secondary and ghost `Button`, the current × button). No custom
state colours are created. Focus is shown by the global `:focus-visible` (an
outer outline in the `link` colour). When a button inside the search field frame
has focus, that button's outline is shown and the search field frame's focus
style (`focus:border-accent`) is not.

## Accessibility

- Accessible names: the search field has the three variants above. The help
  button is `検索の書き方`, and the popover takes its name from the heading
  `検索の書き方`. The filter button keeps the current `絞り込み（N 件適用中）`. The
  sort menu button keeps the current `並び順: 〈種類〉`, and the menu heading and
  the "view and sort" group keep the current word `並び順`. Direction and Shuffle
  are as described above.
- Count changes: the library and the folder screen during search announce them
  `polite` from the visible count line.
- Keyboard order (acceptance criterion 23): within the top bar, search field →
  (× when there is a query) → help → filter → sort → direction (or Shuffle) →
  view mode, size, group → refresh; then the sidebar entries and the result cards
  in the body. Shell components come before the body as described in 011
  "Accessibility", and the Tab order keeps this order within that sequence.
- Colour pairs: the added pairs are `fg-muted` and `fg-subtle` on `surface` (the
  location line) and `fg` and `fg-muted` on `elevated` (the help and chips); the
  text pairs are already in `tokens.test.ts`. `fg-subtle` is used only for icons,
  never for text. No new pair is added.
