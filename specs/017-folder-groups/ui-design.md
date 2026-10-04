# UI design: Listing a folder group as one entry and playing it continuously

**Feature**: [parent Issue #326](https://github.com/syudead/vv/issues/326)

Sources: visual rules, the shell and list density follow
[Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md)
and the `@theme` in [`web/src/index.css`](../../web/src/index.css). The video
page structure and the end-of-playback layer follow
[012's ui-design.md](../012-video-detail-ia/ui-design.md); the folder screen
structure follows [011's ui-design.md](../011-folder-browser/ui-design.md) and
[013's ui-design.md "Folder screen"](../013-library-search/ui-design.md#folder-screen);
tag chips, the selection bar and the combobox follow
[014's ui-design.md](../014-video-tags/ui-design.md); guest degradation follows
[016's ui-design.md "Guest degradation"](../016-single-account-auth/ui-design.md#guest-degradation).
The responses and routes the screens use are fixed in
[contracts/library-api.md](contracts/library-api.md) and
[contracts/folder-groups-api.md](contracts/folder-groups-api.md), and how a
group's watch state and member to open are decided is in
[data-model.md, Group watch state and the member to open](data-model.md#group-watch-state-and-the-member-to-open);
this document does not revisit them.

This document defines only what the group card, the folder-derived tag
distinction, the video page's member list and autoplay notice, and the grouping
controls on the folder screen and video page **add to or change in** existing
screens. No new color, radius or shadow tokens are added, and nothing is added
to `pairs` in `tokens.test.ts` ("Accessibility" below).

## Screen boundary

| Place | What changes |
| --- | --- |
| Library (`/`) grid and list view | Entries become `LibraryItem`; group entries appear as group cards (`web/src/library/`) and group rows. Ordinary video entries keep the current `VideoCard` and `VideoRow` with no visual change. No sections, headings or tabs (requirement 15) |
| Tags on cards and the video page | Folder-derived tags differ in form from tags attached by hand (requirement 13). The active tag filter row, the suggestion list and the management page do not change |
| Selection bar | N in `N 件を選択中` and the `タグを外す` suggestions change (requirements 21 and 13). The order of controls does not change |
| Video page (`/videos/:id`) | Only when a group member is open: one line above the title ("Group line"), the member list and divider above the related videos column ("Member list"), and the end-of-playback notice ("Autoplay notice"). The page for a video outside a group is unchanged (second half of requirement 27, acceptance criterion 17) |
| Folder screen (`/folders/{rootId}/…`) | A grouping menu at the right end of the `動画 N` heading line ("Folder grouping menu"). The list still shows one video at a time (requirement 22). Nothing is added at the top level (`/folders`) or to search results |
| Guest | The group card omits watch state and the watched count; the grouping menu, the video page menu and tags are not shown ([data-model.md, Visibility per audience](data-model.md#visibility-per-audience)) |

## Group card

### Structure

The group card uses the same box as an ordinary video card (`VideoCard`)
(`rounded-lg`, `bg-surface`, `shadow-card`, the hover lift, the selection
`ring-2 ring-accent`) and sits in the same place at the same size
(requirement 16, `UI品質` "visual hierarchy" and "information density"). The
thumbnail frame draws the same folder artwork as the folder cards on the folder
screen, so a group looks like "a folder of matching content".

| Part | Form |
| --- | --- |
| **Folder artwork** | The shared `FolderArt` (`web/src/videoList/FolderArt.tsx`) used as is: on a tabbed back panel, `previews` (up to four members with generated thumbnails, in order) overlap at an angle; moving the mouse sideways brings the one at that position forward and plays its preview. With no member thumbnails, only the back panel is drawn |
| **Count and duration badge** | At the back panel's bottom right (`right-5 bottom-4`), on the same surface as a video card's duration (`bg-navbar/85`, `text-[11px]`, `tabular-nums`): `12 本` then the total duration. It sits above the forward image (`z-20`) and is not interactive (`pointer-events-none`). Without a total duration (`durationMs`) it shows only the count. The watched count is not shown as a number. The artwork already shows a folder, so there is no lucide `Folder` mark either |
| **Top left** | The selection checkbox (same position as now) |
| **Bottom progress** | Only when `watchState = inProgress`: the same `h-[5px]` bar as a video card, with the `bg-accent` width at `watchedCount / videoCount`. In progress with `watchedCount = 0` (the first member partly watched) only the bar's track (`bg-fg-subtle/50`) shows, which marks "started". `aria-valuenow` of `role="progressbar"` is the same ratio, with the accessible name `視聴済みの本数の割合`. Not shown for `unwatched` or `watched` |
| **Title** | The group name as `h3`, `text-sm font-medium`, truncated to two lines, full name in `title`. Same style as a video title (`UI品質` "typography"). When `watched`, `text-fg-muted` as for videos |
| **Tag row** | The union of the members' tags (`tags`) in the same `CardTagRow` as a video card. Tags only from folders take the "Folder-derived tag chip" form. No row without tags |
| **Public mark** | Not shown: a group entry has no `public` |

- Date added, size and the word `フォルダ` are not shown. The card adds only the
  count and duration badge, and its height matches a video card (`UI品質`
  "information density").

### Pressing and selection

- The whole card is one link to `/videos/{openVideoId}`, with `state.from` set
  to the current list URL (as a video card). It does not open a member list
  screen (requirement 23, `要求を満たしたことにならない変更`). The accessible
  name is `〈名〉、〈N〉本のグループ`, followed by `、〈M〉本を視聴済み` when
  `watchedCount` is 1 or more. For guests it stops at the count.
- Checking the selection checkbox (accessible name
  `「〈名〉」のグループを選択`) adds every member in `videoIds` to the selection;
  unchecking removes every member. The card looks selected when every member is
  selected (requirement 21). N in the selection bar's `N 件を選択中` is the
  number of videos (members are counted), not cards, because tag add and
  remove and `公開` act on that number.
- So the selection bar's Select all is not disabled by comparing the selected
  count with `total` (entries, that is cards; the current `count >= total`):
  selecting one group would push the count past `total` and disable the button
  while unselected entries remain. It is disabled only while sending, and while
  the selection equals the set of `ids` in the response to the latest Select
  all (`GET /api/library/ids`). Removing or adding even one video, or a
  condition change that clears the selection, enables it again. When every
  loaded entry is selected it stays enabled, since unloaded entries may remain.
- While selecting (one or more selected), pressing the card toggles its
  selection, as a video card.
- No ungroup or tag conversion controls on the card (`UI品質` "action
  priority"). They are in "Group line" on the video page and "Folder grouping
  menu" on the folder screen.

### List view row

A group row in list view (library only) fills the same columns as a video row
(`VideoRow`):

| Column | Group row |
| --- | --- |
| Selection | The same checkbox as a video row, with the meaning in "Pressing and selection" |
| Thumbnail | The first member's thumbnail (the first of `previews`). The bottom progress is `watchedCount / videoCount` when `inProgress` |
| Title | The group name (a link to `/videos/{openVideoId}`). Below it, one `text-xs text-fg-muted` line with lucide `Folder` (`size-3`) and `12 本` |
| Watched | `Check` for `watched`, as for videos. `3 / 12` in `tabular-nums` for `inProgress`. Empty for `unwatched` |
| Duration | The total duration (empty when unknown) |
| Quality | Empty (it differs per member) |
| Size | The total |
| Date added | `addedAt` (the latest member) |

### Refresh and removal

- A group card still in the list is refetched from
  `GET /api/folders/{rootId}/group` when members' playback positions, tags or
  `video` events change. While refetching the card looks the same (no
  skeleton), and it is replaced when the response arrives.
- When the refetch returns 404, the card is removed from the list. `total` in
  the count line may stay until the list is next read. Nothing is announced on
  removal: the main case is returning from playback to find an override change
  has turned the group back into single videos, and rereading the list makes it
  consistent.

## Folder-derived tag chip

The source distinction is added to the three tag chip forms (014 "Tag chip").
Size (`h-5`, `h-6`, `text-xs`) does not change; form and surface tell them
apart (requirement 13, `UI品質` "typography").

| Source (`VideoTag`) | Form |
| --- | --- |
| `manual` only, or both `manual` and `fromFolder` | The current video tag chip (`bg-elevated`, with a surface) |
| `fromFolder` only | No surface; a dashed frame `border border-dashed border-border-strong`. Lucide `Folder` (`size-3`, `text-fg-subtle`, `aria-hidden`) before the name |

- The dashed frame is a shape that says "borrowed from the folder structure",
  not a color-only difference (`UI品質` "accessibility"). The text color is the
  same as the current chip (`text-fg-muted` on cards, `text-fg` on the video
  page). Without a surface the text sits on the parent surface (`surface` on a
  card, `bg` on the video page, `elevated` in the `+N` popover). Every pair is
  already in `pairs`.
- The accessible name in the interactive form (card) stays `〈名〉で絞り込む`,
  followed by a visually hidden `（フォルダ名から）`.
- Pressing (filter by the tag) behaves the same whatever the source.
- **Video page**: a `fromFolder`-only chip has no × (requirement 13). Its name
  part is the same link to `/?tag=<id>` as now. A chip attached from both
  sources keeps the current form (name + vertical rule + ×); pressing × removes
  only the hand-attached part, and after the response the chip turns dashed in
  the same place (it does not disappear; acceptance criterion 9). Focus moves to
  the × of the next chip, else the previous chip's ×, else the `タグを追加`
  input (the same rule as 014; dashed chips have no × and are skipped).
- **`タグを追加` suggestions**: tags attached to the video only from folders
  are suggested. Committing one attaches it by hand too, and the chip takes the
  form with a surface.
- **Selection bar `タグを外す`**: suggestions are only summary tags with
  `manualCount >= 1`. Folder-only tags cannot be removed, so they are not
  suggested (no `disabled` rows either). "Some" and the count are decided by
  `manualCount`, and the denominator of `一部 1 / 3 件` stays the number
  selected. When none of the selected videos has a hand-attached tag, the
  wording is `選んだ動画に、外せるタグはありません` (folder-derived tags may
  exist, so it does not say `タグはありません`).
- Order stays the API order (natural order of names), not split by source.

## Video page

### Group line

Only when `Video.group` exists, one line goes **above** the title (`h1`)
(requirement 25). It is the first item in the title and tag group
(`flex flex-col gap-2`), with `gap-2` to the title.

- From the left: lucide `Folder` (`size-3.5`, `text-fg-subtle`,
  `aria-hidden`) → the group name (`text-fg-muted`, one line truncated, full
  name in `title`) → `·` in `text-fg-subtle` → `3 / 12` (`tabular-nums`). The
  whole line is `text-xs` (`text-sm` from `sm`). Smaller than the title
  (`text-xl` to `text-2xl`) and in a secondary color, it does not catch the eye
  before the title despite sitting above it (`UI品質` "visual hierarchy").
- The breadcrumb in the header band already links to the same folder, so the
  group name is not a link.
- For the **owner**, the whole line is a ghost `sm` `Button` (`-ml-2` aligns the
  text's left edge with the title) ending in `ChevronDown` (`size-3.5`) that
  opens a `ui/Menu` (requirement 29). The accessible name is
  `グループ「〈名〉」、12 本中 3 本目。まとめ方のメニュー`. Items:

| Item | Action |
| --- | --- |
| `まとめを解除` (lucide `Ungroup`) | `PUT /api/folders/{rootId}/grouping`, `mode: ungroup` |
| `グループをタグに変える` (lucide `Tag`) | `POST /api/folders/{rootId}/grouping/tag`. Not shown when `group.folder.path` is empty (the registered folder itself) (contracts/folder-groups-api.md, [Turning a group into a tag](contracts/folder-groups-api.md#turning-a-group-into-a-tag)) |

- There is no `直下をまとめる`: what the video page shows is already a group, so
  it would mean nothing.
- While the menu is open, Esc only closes the menu and does not close the page;
  focus returns to the trigger (the `ui/Menu` default). The video page receives
  Esc in the `window` capture phase, so an open `ui/Menu` (`role="menu"`) is
  added to the exclusions, like the current ones (speed menu, popovers). This is
  one more Esc exception in 012 "Interaction details".
- For **guests** the line is non-interactive text (`Video.group` is built from
  public members for guests too).
- After an action, the video (`GET /api/videos/{id}`) and related videos are
  refetched. `group` disappears, so this line, "Member list" and the
  previous/next arrows return to the ordinary video form. The library snapshot
  is discarded too, so closing the page rereads the list (Structural Decisions
  10, acceptance criteria 3, 4 and 5). A toast reports it:

| Action | Toast |
| --- | --- |
| Ungroup | `「〈名〉」のまとめを解除しました` |
| Turned into a tag, `created` true | `タグ「〈名〉」を作り、まとめを解除しました` |
| Turned into a tag, `created` false | `タグ「〈名〉」を付け、まとめを解除しました` |

- While sending, the menu trigger is `disabled`. Opening a menu is the action,
  so there is no confirmation dialog for undoing. Both actions can be reverted
  to `自動` in the folder screen menu (and the tag can be deleted on the
  management page).
- Failures show a toast and leave the screen unchanged:

| Response | Toast |
| --- | --- |
| 400 `invalid_request` (tag name rules) | `「〈名〉」はタグの名前に使えないため、タグに変えられません` (Edge Case `グループをタグに変えられない名前`) |
| 409 `conflict` | `このフォルダはもうグループではありません`; the video and related videos are refetched (another tab changed it first) |
| 404, others | `変更できませんでした` |

### Member list

The right column (below under `lg`) when `RelatedVideos.group` exists
(requirement 26).

- The column's first heading (`h2`, `text-sm font-semibold text-fg`) is
  `続けて再生`, with `3 / 12` in `text-xs text-fg-muted tabular-nums` to its
  right (`flex items-baseline justify-between`). The range that continues
  automatically is then readable at the top, before reaching the divider.
- Below it, the members in order (not cut even at thousands; Edge Case
  `大きなグループ`). The related videos response carries only a window of up to
  100 members around the current one (contracts/folder-groups-api.md §3);
  the list reads the rest with `GET /api/videos/{id}/group-members`, 100 at a
  time (issue 674, "Reading beyond the window" below). Each row has the density of the current related video rows
  (thumbnail `w-40`, duration badge, title `text-sm font-medium`, `gap-3`
  between rows, hover `bg-hover-wash`) (`UI品質` "information density"), plus:

| Member | Form |
| --- | --- |
| Every row | The order number at the start (left of the thumbnail) in `w-5 text-right text-xs text-fg-muted tabular-nums`, so the current position and the remainder read without counting |
| Watched | Title in `text-fg-muted`, with lucide `Check` (`size-3.5`, `text-success`) to its right. A visually hidden `視聴済み` is read; the icon is `aria-hidden` |
| Partly watched | The same bottom progress bar as now |
| **Currently playing** | Not a link: a row with `aria-current="true"`, surface `bg-active-wash`, and `border-l-2 border-accent` on the left (the left edge of the row's `-m-1.5 p-1.5` outline). A visually hidden `再生中` precedes the title. It is not interactive, so the hover surface does not change |

- **Divider**: one `border-t border-border` line between the member list and
  the related videos, with `pt-5` and `pb-2` around it (one step larger than the
  `gap-3` between rows; `UI品質` "spacing rhythm"). Below the line is the `h2`
  `関連動画` (the current heading), then the current related videos (which do
  not include members of the same group; contracts/folder-groups-api.md, [Groups of videos and related videos](contracts/folder-groups-api.md#groups-of-videos-and-related-videos)).
  With 0 related videos, neither the divider nor the `関連動画` heading appears.
- Scrolling at `lg` and up: as in 012, the column's first heading line (here
  `続けて再生`) stays at the top, and everything below it (member list →
  divider → `関連動画` → related videos) scrolls inside one container. When the
  page opens (including moving to another member), the container alone
  scrolls so the current member's row is visible in it (Edge Case
  `大きなグループ`, acceptance criterion `数百本`). The page and the left column
  do not move. Under `lg` the page does not move (the player would leave the
  top of the screen).
- Drawn rows: only the loaded members near the view are drawn, with four rows
  more above and below, measured against the container at `lg` and up and
  against the page under `lg` (#675). The current member's row and a focused
  row stay drawn.
- The `3 / 12` position is a button: pressing it scrolls the current member's
  row into view at any width (the container at `lg` and up, the page under
  `lg`), so a viewer who scrolled far away gets back to it.
- **Reading beyond the window**: the order numbers and `3 / 12` count in the
  whole group (`offset` and `total`), not in the window.

  | Edge | `lg` and up | Under `lg` |
  | --- | --- | --- |
  | After the last loaded member | Reads the next 100 when the end comes within about 600px of the view, while scrolling the container | The same, while scrolling the page |
  | Before the first loaded member | Reads the previous 100 when the top comes within about 600px, then shifts the container's scroll by the added height so the rows in view do not move | A ghost button `Show N earlier videos` reads the previous 100. Reading on scroll would start as soon as the page opens, because the top of the list is already in view below the player |

  A failed read keeps the loaded rows and shows `Couldn't load more of the
  group` with a `Retry` button at that edge. Moving to another member starts
  again from the new window.
- Member rows play the list preview on hover, as related video rows do.
- Loading and failure look the same as the current related videos (six skeleton
  rows; `関連動画を取得できませんでした` and `再試行`). The member list cannot be
  shown until the related videos response arrives, so the heading stays
  `関連動画` until then and changes to `続けて再生` when it arrives.
- **Guests**: `group.items` holds only public members, with no watched marks or
  progress (no `progress`). Numbers are the order among public members.

### Neighbor arrows

The previous/next arrows (012 "Neighbor arrows") follow `prevId` and `nextId`
from the related videos response, so for a member they move within the group:
no left arrow on the first member and no right arrow on the last
(requirement 28). Accessible names and tooltips stay the current
`前の動画: 〈題名〉` and `次の動画: 〈題名〉`, with titles taken from
`group.items`.

### Autoplay notice

When a member finishes playing and `nextId` exists, a notice layer replaces the
current end-of-playback layer (requirement 27). Layer order, `bg-overlay` and
keeping the control bar visible are as in 012 "Ended"; only one layer shows at a
time.

- A centered `bg-navbar`, `rounded-lg`, `p-5`, `max-w-lg` surface stacks, from
  the top (`gap-3`):

| Part | Form |
| --- | --- |
| First line | `続けて再生` in `text-xs font-semibold text-accent`, with `5 秒後` (seconds left, counting down every second) to its right in `text-xs text-fg-muted tabular-nums` |
| Link to the next member | Thumbnail (`w-56`, hidden under `sm`) and title (`text-base font-semibold`, truncated to two lines). The same form as the current `次の動画` |
| Countdown bar | A full-width `h-1 rounded-full bg-fg-subtle/50` track whose `bg-accent` remainder shrinks from the right over 5 seconds. With reduced motion the bar stays, but the smooth shrink stops and it shrinks in steps as the seconds drop (`motion-reduce:transition-none`; the state stays visible and only motion stops, [library-ui.md, Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception) and 012 "Interaction details") |
| Actions | Secondary `取り消す` (lucide `X`) on the **left** and primary `今すぐ再生` (`Play`) to its right. DOM order is the same, so Tab reaches `取り消す` first (`UI品質` "action priority") |

- The countdown is 5 seconds from when the notice appears. At 0 the next member
  opens (carrying `state.from` over) and starts playing.

| Event | Behaviour |
| --- | --- |
| **Cancel** (`取り消す`) | The notice layer goes and the current end-of-playback layer appears (`次の動画` = the next member, `次を再生`, `もう一度見る`) (requirement 27). If focus was on `取り消す`, it moves to `次を再生` |
| **Esc** | While the notice shows, Esc cancels and does not close the page. After cancelling, Esc closes the page as now. One more Esc exception in 012 "Interaction details" (speed menu, fullscreen) |
| **The next member has disappeared** (404 from the check in Structural Decisions 12) | The notice goes and the end-of-playback layer for "no next video" (only `もう一度見る`) appears. The wording gives no reason. Reopening decides with the latest group |
| **Last member** (no `nextId`) | No notice; the current layer with only `もう一度見る` (acceptance criterion 16) |
| **An automatically opened member fails to play** | It stops at the current playback failure layer (`PlaybackFailure`) and goes no further (Edge Case) |
| `最初に戻る` or play on the control bar | As with the current end of playback, the notice goes and playback resumes |

- **Announcement**: when the layer appears, a visually hidden `role="status"`
  reads `再生が終わりました。5 秒後に次の動画「〈題名〉」を再生します` **once**.
  The seconds text and the bar are `aria-hidden` so they are not reread every
  second (`UI品質` "accessibility"). Focus moves to `取り消す` only when it was
  inside the player (as 012 "Ended").
- Touch center controls are hidden while the notice shows (012 "Overlay
  layer").

## Folder grouping menu

The folder screen's `動画 N` heading line holds the grouping controls for that
folder (requirement 22, requirements 7 and 11). Child folder cards do not: they
are links for opening, and adding controls would double the Tab stops (the same
reason as 013 "Search results").

- The `Section` heading line becomes `flex items-center justify-between`, with
  the current `h2` `動画 N` on the left and a ghost `sm` `Button` trigger on the
  right. The trigger holds a lucide icon + wording + `ChevronDown`
  (`size-3.5`), changing with `FolderSummary.grouping.grouped`:

  | `grouped` | Icon | Wording |
  | --- | --- | --- |
  | true | `Group` | `ライブラリで 1 件` |
  | false | `Ungroup` | `ライブラリで 1 本ずつ` |

  Accessible names are
  `ライブラリでのまとめ方: 1 件にまとめて表示。メニューを開く` and
  `…: 1 本ずつ表示。…`. The wording states the current state, and pressing
  changes it. `grouped` is the result of the override and the automatic rule,
  so an override on a folder with one direct video still reads `1 本ずつ`,
  showing here that it has no effect.
- The trigger is `text-fg-muted` (the ghost default), as weak as the heading
  (`text-xs font-semibold text-fg-muted`), and does not catch the eye before the
  grid (`UI品質` "action priority").
- The menu (`ui/Menu`, `align="end"`) holds, from the top:
  - `MenuLabel` `ライブラリでのまとめ方`
  - `MenuRadioGroup` (the current `grouping.mode`): `自動` (`auto`),
    `まとめを解除` (`ungroup`), `直下をまとめる` (`groupDirect`). No
    descriptions to the right of items; the line below explains.
  - One `text-xs text-fg-muted` line
    `自動: 登録フォルダより下で、子フォルダが無く動画が 2 本以上のフォルダをまとめる`
    (same padding as `MenuLabel`, not interactive). It states all three
    conditions of requirement 2, so even on the registered folder's own screen
    it reads that `自動` does not group it.
  - Only when `taggable` is true, a divider and `グループをタグに変える` (lucide
    `Tag`). When false the item is absent (contracts/folder-groups-api.md, [Turning a group into a tag](contracts/folder-groups-api.md#turning-a-group-into-a-tag)).
    Keeping a disabled item is rejected: for the registered folder itself the
    reason cannot be given in one line.
- Choosing a radio sends `PUT /api/folders/{rootId}/grouping` and updates the
  trigger wording and radios from the response's `grouping`. No toast: the
  changed wording is the result. Choosing the same value again still sends
  (200).
- `グループをタグに変える` sends `POST …/grouping/tag` and reports with a toast
  (the same wording as "Group line"). The trigger updates from the response's
  `grouping` (`ungroup`).
- Both discard the library snapshot on response (Structural Decisions 10). The
  next time the library opens, cards change without a rescan (acceptance
  criteria 3, 4 and 5).
- A radio change does not reread the folder screen list: it still shows one
  video at a time and its content does not change (only `grouping` in
  `FolderSummary` is replaced from the response).
- A successful tag conversion replaces `grouping` and also rereads the folder
  screen's video list (keeping the scroll position). Once the tag is created,
  the videos inside get the folder-derived tag from the next read
  (data-model.md, [Folder-derived tags](data-model.md#folder-derived-tags)), but `tags` on the cards on screen are not in the response
  and would stay stale (this shows requirement 11's result on the spot).
- While sending, the trigger is `disabled` and its icon becomes `LoaderCircle`
  (`animate-spin`).
- Failures show a toast:

| Response | Toast |
| --- | --- |
| 400 `invalid_request` | `「〈名〉」はタグの名前に使えないため、タグに変えられません` |
| 409 | `このフォルダはもうグループではありません` (the folder list is refetched) |
| 404 | `このフォルダは見つかりません` |
| Others | `変更できませんでした` |

- A folder without a `動画` group (no direct videos) has no heading line, so no
  menu. The registered folder itself (empty `path`) shows the same menu on the
  same line (`直下をまとめる` works at the root too; requirement 7).
- **Guests** have no `grouping`, so no trigger (the heading line stays as now).
- Child folder cards get no group mark. It is not required, and adding
  information to the bottom half of the card would compete with 011's count
  line.

## Interaction states

| Control | States |
| --- | --- |
| Group card and row | Same as `VideoCard` and `VideoRow` (hover lift and shadow, outer outline from `has-[a:focus-visible]`, selection `ring-2 ring-accent`, `select-none` in `selectionMode`) |
| Dashed chip | On hover, text `text-fg` and the frame turns solid while staying `border-border-strong` (adding a `ring` to a surfaceless form would look like a double frame). Focus is the global `:focus-visible` (inside a card, the inner outline as in 014) |
| "Group line" trigger, "Folder grouping menu" trigger | Inherit ghost `Button` states. `disabled` only while sending |
| Member row | The same hover as a related video row. The current member's row is not interactive and does not change on hover |
| Notice layer actions | Inherit secondary and primary `Button` states |

## Accessibility

- Accessible names:

| Control | Name |
| --- | --- |
| Group card link | `〈名〉、〈N〉本のグループ` (for the owner with `watchedCount >= 1`, followed by `、〈M〉本を視聴済み`) |
| Group card checkbox | `「〈名〉」のグループを選択` |
| Group card progress | `視聴済みの本数の割合` |
| Dashed chip | `〈名〉で絞り込む` + hidden `（フォルダ名から）` |
| "Group line" trigger | `グループ「〈名〉」、〈N〉本中〈M〉本目。まとめ方のメニュー` |
| Member row | As the current related videos, `〈題名〉 〈長さ〉`. The current member has `aria-current="true"` and hidden `再生中`; watched members have hidden `視聴済み` |
| Notice layer | `role="status"` `再生が終わりました。5 秒後に次の動画「〈題名〉」を再生します` (once). `取り消す`, `今すぐ再生` |
| "Folder grouping menu" trigger | `ライブラリでのまとめ方: 1 件にまとめて表示。メニューを開く` or `ライブラリでのまとめ方: 1 本ずつ表示。メニューを開く` |

- Grouping: the member list is a `ul` under the `続けて再生` `h2`, and related
  videos a `ul` under the `関連動画` `h2`. The two `ul`s are not merged, so the
  divider is clear to screen readers too.
- The notice's seconds and bar are `aria-hidden`. Cancelling works both with
  the first button Tab reaches and with Esc.
- Color pairs: no new pair is used. Dashed chip text sits on the parent surface
  (`surface`, `bg`, `elevated`) in `fg` and `fg-muted`, and notice layer text on
  `navbar` in `fg`, `fg-muted` and `accent`; all are in `pairs`.
  `border-border-strong`, `bg-active-wash` and `bg-fg-subtle/50` are not text
  and are translucent, so they are outside `pairs`. No new tokens are added.
