# UI Design: Information architecture of the video detail screen

**Feature**: [parent Issue #171](https://github.com/syudead/vv/issues/171)

Sources: the visual rules follow
[Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md)
and `@theme` in [`web/src/index.css`](../../web/src/index.css). This document
defines only what the video detail screen (`/videos/:id`) **adds** to them and
**changes**.

- No new colour, radius or shadow token is added.
- Components are the existing ones in `web/src/ui/` (`Button`, `IconButton`,
  `Skeleton`, `Tooltip`, `Popover`).

## Screen boundary

- The player screen stays outside `AppShell`, as before (ARCHITECTURE.md "Web
  layer"). It has no list top bar (☰, refresh); instead it has its own heading
  band ("Video header"). The experience of a screen layered over the list and
  closed with × does not change.
- The screen consists of the heading band, the player, the title and tags, the
  file information ("Video facts") and related videos (parent Issue
  requirement 1).
- The following are not drawn (requirements 4 and 5):
  - chips
  - a summary line below the title
  - large action buttons near the title (`ファイルを開く` and `パスをコピー` stay as
    small icons at the right end of the information row)
  - tabs
  - a details area enclosed in a frame or card
- The following existing components are replaced:
  - The old `VideoHeader` and `FileDetails`: replaced by the title and the
    information rows (today's `VideoHeader` is the heading band, a different
    thing).
  - The red band below the player and `Blocked`: replaced by the state display
    inside the player.

## Layout and responsive behaviour

Only Tailwind's default breakpoints are used, and the switching is CSS only
(library-ui.md 4).

### `lg` (1024px) and up: two columns

- The heading band (height `h-navbar`) sits at the top of the page, with two
  columns below it.
- The column area has `px-6` left and right and `pt-6` at the top. At the bottom,
  the left column has `pb-6` and the related video list has `pb-6`. The gap
  between columns is `gap-6`.
- The left column (`minmax(0, 1fr)`) holds, top to bottom:
  - the player
  - the creating line (when present)
  - the title and tags
  - the file information row and the technical information row
- The right column is `w-80` at `lg` and `w-96` at `xl` and up. It holds the
  related videos heading and the vertical list of related videos.
- The top of the right column aligns with the top of the player.
- The player is `rounded-lg`, `overflow-hidden`, `bg-navbar`. Its width is
  limited so that a 16:9 video fits the screen height (a video wider than 16:9
  fits at its own ratio), with the existing
  `max-w-[calc(max(100dvh-17rem,15rem)*16/9)]`. That height leaves room in the
  left column for the band (`3.25rem`), the column's top and bottom padding, the
  title and tags and the two information rows, so the left column normally does
  not scroll (below `lg` the whole page scrolls, so it stays `12.25rem`). So that
  the player does not vanish in a short window, the height term never goes below
  `15rem` (the overflow then scrolls inside the left column). The left column's
  bottom padding is `pb-6`. A player that becomes narrower than the column at
  this limit aligns left, flush with the title (`lg:ml-0`). Its height follows
  the video's aspect ratio (16:9 while the resolution is unknown; extreme ratios
  are clamped to 9:16–21:9) and grows up to the height that fits the screen. A
  portrait video is shown centred in the frame with side margins, and the
  previous/next handles and the control bar keep the same position and width as
  for landscape videos.
- Elements in the left column are spaced with `gap-5` (requirement `余白のリズム`).
  No dividers or frames are used.

### Below `lg`: stacked

- The heading band stays at the top of the page (`sticky top-0`). × and the logo
  stay reachable while scrolling.
- The player sits below the band, edge to edge (`rounded-none`, no side padding).
  No × is overlaid on the player.
- Below it is the body with `px-4` (`px-6` at `sm` and up), in this order:
  - the creating line
  - the title and tags
  - the file information row and the technical information row
  - related videos
- Neither 360px nor 768px shows horizontal scrolling (acceptance criterion 2).

## Hierarchy and typography

From strongest to weakest: player > title > related videos > file information >
technical information (requirement `視覚的階層`).

| Element | Style |
| --- | --- |
| Title | `h1`. `text-xl` (`text-2xl` at `sm` and up), `font-semibold`, `text-fg`, `leading-snug`. Wraps even for file names without breaks (`[overflow-wrap:anywhere]`). The largest text on the screen |
| File information | `text-sm`, `text-fg`, `tabular-nums`. A `size-4` `text-fg-subtle` icon before each value. No visible text labels |
| Technical information | `text-xs`, `uppercase`, `tracking-wider`, `text-fg-muted`, `tabular-nums`. English, so `lang="en"`. Codecs and formats in capitals (`H.264`, `AAC`, `MKV`) |
| Breadcrumb | `text-sm`. Intermediate segments `text-fg-muted`; the last segment (the folder holding the video) `font-medium`, `text-fg` |
| Related videos heading | `h2`. `text-sm`, `font-semibold`, `text-fg` |
| Related video title | `text-sm`, `font-medium`, `text-fg`, truncated at two lines (`line-clamp-2`) |

## Video facts

Below the title and tags, two rows are placed (`gap-3`), with no divider, frame
or label.

- **File information row**: a `ul` (accessible name `ファイルの情報`) in this order:
  - Duration (lucide `Clock`). While the duration is unknown (for example before
    probing), the item is omitted.
  - Size (`HardDrive`).
  - Date added (`CalendarPlus`). Date only (`2026/09/20`).
  - Each item is an icon and value pair (`gap-1.5`); items are spaced with
    `gap-x-4` (`gap-x-5` at `sm` and up). They wrap when the width runs out.
  - Icons are `aria-hidden`; instead, visually hidden `長さ`, `サイズ` and `追加日`
    precede the values. `title` holds the same name.
  - The last-watched time is not shown (the requester's decision).
- **Actions**: two `IconButton`s (`size="sm"`) at the right end of the
  information row (`text-fg-muted`, `text-fg` on hover).
  - `ファイルを開く` (`ExternalLink`): present only when the file can be opened
    (`location.openable`). Pressing it sends `POST /api/videos/{id}/open`.
  - `パスをコピー` (`Copy`): writes the absolute path of the location to the
    clipboard and reports `パスをコピーしました` in a toast (on failure,
    `パスをコピーできませんでした`). `navigator.clipboard` exists only on secure
    connections (HTTPS, localhost), so when the page is opened by LAN address or
    the write is refused, it falls back to selecting a hidden input and copying.
  - Without a location (`location`), neither is present.
- **Technical information row**: a `ul` (accessible name `技術情報`) listing only
  the values, in the order resolution, container, video, audio. Items are
  separated by a vertical `border-l border-border-strong` line and `px-2.5`. Every
  item gets the line and padding, and the whole list is shifted left by that
  width and clipped by the outer element (`overflow-hidden`), so that no line or
  padding is left at the start of a wrapped row. Unknown values (no audio, for
  example) are omitted with their item.

  | Situation | Technical information row |
  | --- | --- |
  | Before probing | `技術情報を読み取り中` (`text-fg-muted`) |
  | Probe failed | `技術情報を読み取れませんでした` (`text-warning`) |

- When opening fails (409 `file_missing` and others), one line appears right
  below the information row (above the technical information row).
  - `text-sm text-danger`, with `AlertCircle` (`size-4`) first, `role="alert"`.
  - The text is `開けませんでした: ファイルが見つかりません` for `file_missing`, and
    `開けませんでした` otherwise.
  - It disappears on the next open action or when moving to another video.
  - No band, toast or dialog is used (requirement
    `要求を満たしたことにならない変更`).

## Related videos

- The heading is the `h2` `関連動画`. No × here (it is in the heading band).
- With zero related videos, neither the `h2` nor the list is shown (Edge Case).
- Each item is one link, and the whole row is clickable.
  - Thumbnail on the left (`w-40`, 16:9, `rounded-md`, `overflow-hidden`,
    `bg-surface`), title on the right (`gap-3`).
  - Without a thumbnail, the same fallback as `VideoCard` is shown.
  - The duration sits at the bottom right of the thumbnail (`bg-overlay`,
    `text-xs`, `text-fg`, `tabular-nums`, `rounded-sm`, `px-1`).
  - Only partly watched videos show an `accent` progress bar at the bottom edge
    of the thumbnail (3px high, ratio `watchedRatio`) (requirement 15).
  - No date added, `次に再生` or reason text is shown (requirement 15).
- Items are spaced with `gap-3`. On hover the row gets `bg-hover-wash` (with
  `p-1.5 -m-1.5` and `rounded-lg` outside the row). Keyboard focus is shown by the
  global `:focus-visible` outline.
- Hovering with the mouse plays the list preview under the same rules as list
  cards (the hover preview in library-ui.md): after 400ms it plays muted and
  looping, and stops on leave. It does not play for touch, pen, or videos whose
  preview is not ready.
- At `lg` and up the page is held to the screen height (`h-dvh`; the page itself
  does not scroll), and the left and right columns each scroll inside. The right
  column keeps its heading row at the top and only the related video list moves.
  This prevents the page and a column from scrolling at the same time. Below `lg`
  the whole page scrolls as one.
- Scrollbars always shown in each column would be noisy, so (in
  `web/src/index.css`) both can always be scrolled, and:
  - The left column uses `scrollbar-none` and draws no scrollbar. The player
    height limit normally keeps it from overflowing. When a wrapped title or the
    creating line makes it overflow slightly, a mouse resting over the video does
    not keep a bar visible.
  - The related video list uses `scrollbar-on-hover`: thin, normally
    transparent, and shown only while the mouse is over the list or it has focus.
- States:

  | State | What the screen shows |
  | --- | --- |
  | Loading | Six skeleton rows (a thumbnail and a two-line `Skeleton`) |
  | Load failed | Below the heading, `関連動画を取得できませんでした` in `text-sm text-fg-muted` and a `ghost` `再試行` |

## Video header

- The band at the top of the page has the same height as the list top bar
  (`h-navbar`), `border-b border-border`, `bg-bg/90` and `backdrop-blur-md`. It
  stays at the top with `sticky top-0` at every width.
- Left to right:
  - The logo (`●` and `vv`): a link to `/` (home), accessible name `ホーム`.
  - The breadcrumb (`nav`, accessible name `フォルダ`): the segments from the
    registered folder's display name to the folder holding the video, separated
    by `ChevronRight` (`size-3.5`, `text-fg-subtle`).
    - Every segment links to its folder screen (`/folders/{rootId}/…`). The
      current screen is the video, so the last segment is a link too.
    - The values come from `folder` (`rootId`, `path`, `rootName`) in
      `GET /api/videos/{id}`. Without `folder` or `rootName` (for example right
      after the registration was removed), the whole breadcrumb is omitted.
    - Below `md`, only the last segment stays and the intermediate segments
      collapse to `…`. The switching is CSS only (library-ui.md 4).
    - The video title is not shown in the band (it would duplicate the `h1` right
      below).
  - × at the right end (lucide `X`, `size-5`): an `IconButton` with the
    accessible name `閉じる`.
- × appears only in this band; not over the player and not in the related videos
  heading.
- Toasts on the player screen (the `パスをコピー` result, scan notifications)
  temporarily overlay the centre of the band and may hide the breadcrumb. They
  leave `4rem` on each side, so they never cover ×.
- × returns to the list in `state.from`. Esc does the same.

## Player

### Control bar

The control bar keeps the look of the existing `.vv-video-player` rules
(`index.css`); only the arrangement changes (requirement 6).

- Left to right:
  - Back to start (lucide `RotateCcw`; sets the position to 0 without changing
    play or pause; key 0)
  - Play/pause
  - Volume (mute)
  - Current time / duration
  - Right-aligned spacer
  - `変換して再生中` (only while playing through transcoding)
  - Playback speed
  - Picture-in-picture
  - Fullscreen
- There is no seek-by-seconds (10 seconds back or forward), neither as a button
  nor as a key (←/→).
- Previous and next videos are not in the control bar; they sit at the left and
  right edges of the player ("Neighbor arrows" below).
- The progress bar stays above the control bar as before, and so does the seek
  thumbnail.
- The remaining time is not shown (requirement 5).
- Volume and mute are saved in the browser and carried to the next video. When
  the saved value is broken or browser storage is unavailable, playback uses
  100% volume, unmuted.
- The speed options are 0.5, 0.75, 1, 1.25, 1.5 and 2.
- Buttons with keyboard shortcuts get `aria-keyshortcuts`, and their hover
  description includes the key (such as `一時停止（Space）`).
- `変換して再生中` is shown as follows:
  - lucide `Info` (`size-3.5`) and text in `text-xs text-fg-muted`.
  - It is the trigger (`button`) of `web/src/ui/Popover.tsx`. Click, tap and Enter
    open a small bubble with the reason
    `ブラウザがそのまま再生できない形式のため、変換しながら再生しています。シークに数秒かかります。`
    - It is not a tooltip that opens only on hover, because touch and keyboard
      users could not reach the reason (requirement 10).
  - It looks quieter than the other control bar buttons (no frame, no surface,
    text colour `fg-muted`).

### Neighbor arrows

- Handles for moving to the previous and next video sit flush against the left
  and right edges of the player. The left one (lucide `ChevronLeft`) goes to the
  previous video in natural order in the same folder; the right one
  (`ChevronRight`) to the next.
- They are narrow so as not to hide the video, and tall so they are easy to hit.

  | Width | Handle size |
  | --- | --- |
  | `sm` and up | Width `w-8`, height 50% of the player (minimum `min-h-24`) |
  | Below `sm` | Width `w-7`, height 55% of the player (minimum `min-h-18`) |

  - They are centred vertically on the video area excluding the control bar
    (about 3rem).
  - The surface is `bg-overlay`, `bg-bg` on hover. The side touching the edge has
    no rounding; only the inner side is `rounded-lg`. No dark gradient at the
    edges.
- They are visible at the same times as the control bar, and also while playback
  has ended. While hidden they cannot be pressed, and they appear when they get
  keyboard focus.
- A side is not shown when the related videos response has no `prevId` or
  `nextId` for it (the start or end of the folder, or while related videos load
  or after they failed).
- On hover, a tooltip inside the handle shows `前の動画: {題名}` / `次の動画: {題名}`,
  and the accessible name is the same. When the destination is not in the related
  video list and its title is unknown, only `前の動画` / `次の動画`.
- Moving while playing or after the video ended starts playback at the
  destination. Moving while paused does not. The return target (`state.from`) is
  carried over.

### Touch controls

- Only on devices with a coarse pointer (`pointer: coarse`), one round
  play/pause button (`size-15`) sits at the centre of the player
  (requirement 8). No seek-by-seconds buttons.
  - The surface is `bg-overlay` and the icon `text-fg`.
- It is visible at the same times as the control bar. On these devices video.js's
  big play button is not shown.
- Not shown on mouse devices. The switching uses only the CSS `pointer` media
  condition.

### Overlay layer

Layers over the player are stacked inside one container. From the top:

1. State display
2. Playback ended
3. Centre touch control

Only one shows at a time. While the state display or the ended layer is shown,
the centre control is not.

Every state display stays inside the player area (requirement 10).

- Only the creating line (right below the player) is outside the area.
- At widths where the content does not fit the video's aspect ratio (such as the
  stage display at 360px), the area grows to fit the content. The video's aspect
  ratio is kept as a minimum.
- The playback-failed and ended layers sit **below** the video.js control bar in
  stacking order. Only the video is dimmed; the control bar stays visible and
  operable.
- Text in a layer is never placed directly on `bg-overlay`. `bg-overlay` is
  translucent, and over a bright last frame `fg-muted` or `accent` text lacks
  contrast. Text blocks sit on an opaque `bg-navbar`, `rounded-lg`, `p-5`
  surface.

| State | What the screen shows |
| --- | --- |
| Loading | The thumbnail in the background when there is one, and `LoaderCircle` (`size-7`, `text-accent`, spinning) at the centre. A visually hidden `読み込み中` is read through `role="status"`. The control bar stays hidden until loading finishes |
| Playing through transcoding | See "Control bar" above. No layer |
| Playback failed | `bg-overlay` over the video. A central `bg-navbar` surface (`max-w-md`) stacks `AlertCircle` (`size-8`, `text-danger`), the `h2` `再生できませんでした` (`text-lg font-semibold`), the reason (`text-sm text-fg-muted`) and a `secondary` `{m:ss} からもう一度試す` (`RotateCcw`). `role="alert"` |
| Scanning (before probing) | "Processing stages" below |
| Probe failed | At the centre of a `bg-navbar` surface (`max-w-lg`): `AlertTriangle` (`size-8`, `text-warning`), the `h2` `この動画を読み取れませんでした`, the explanation `ファイルが壊れているか、途中までしか書き込まれていない可能性があります。` (`text-sm text-fg-muted`), the raw reason and the actions, stacked. The raw reason is in `bg-field`, `border border-border`, `rounded-md`, `font-mono text-xs text-fg`, `break-all`, and scrolls inside beyond three lines. The action is a `secondary` `もう一度読み取る` (`RefreshCw`), plus a `secondary` `ファイルを開く` (`FolderOpen`) only when the file can be opened. While sending, the button is disabled and its icon becomes a spinning `LoaderCircle`. On 202 or 409 `probe_not_failed`, the video is refetched and the screen moves to the stage display (409 means a retry already started from another tab or a double press). On any other failure, `読み取りを始められませんでした` in `text-sm text-danger` appears below the button, and the button becomes pressable again. `role="alert"` |
| Video gone (404) | At the centre of a `bg-navbar` surface: `AlertCircle` (`text-fg-muted`), the `h2` `この動画は開けません` and `ライブラリから外れたか、ファイルが無くなりました。`. `role="alert"`, so it is read when a refetch or playback switches to this display. Going back is left to ×; no button is added |

### Processing stages

How the surface looks while scanning (`probeState=pending`) (requirement 12).

- The surface is `bg-navbar`, with a left-aligned block (`max-w-sm`) at the
  centre.
- The block contains, top to bottom:
  - the `h2` `再生の準備をしています` (`text-lg font-semibold`)
  - the explanation `動画の情報を読み取っています。終わるとこの画面のまま再生できるようになります。`
    (`text-sm text-fg-muted`)
  - the `ol` of stages
  - the note `再生できるのは『動画情報の読み取り』が終わってからです。残りは再生中に作られます。`
    (`text-xs text-fg-muted`)
- The five stages, in a fixed order:

  | Stage | Shown as | Derived from |
  | --- | --- | --- |
  | File detection | `ファイルの検出` | Always `完了`: the video row exists, so detection has finished |
  | Probe | `動画情報の読み取り` | `probeState` |
  | Thumbnail | `サムネイル` | `thumbnailState` |
  | Seek preview | `シーク用プレビュー` | `seekThumbnailState` |
  | List preview | `一覧用プレビュー` | `previewState` |

- Each row shows an icon, the name (`text-sm`) and the state text at the right
  end (`text-xs`).

| Row state | Icon | Name | State text |
| --- | --- | --- | --- |
| Done | `CircleCheck` (`text-success`) | `text-fg` | `完了` in `text-fg-muted` |
| Processing | `LoaderCircle` (`text-accent`, spinning) | `text-fg font-medium` | `処理中` in `text-accent` |
| Waiting | `Circle` (`text-fg-muted`) | `text-fg-muted` | `待機中` in `text-fg-muted` |
| Could not be created | `AlertCircle` (`text-warning`) | `text-fg-muted` | `作成できませんでした` in `text-fg-muted` |

- Rows are spaced with `gap-3`, and the icon and name with `gap-2.5`.
- Below `sm`, the explanation and the note are omitted, and rows tighten to
  `gap-1.5`.
- The block is `role="status"` (`aria-live="polite"`), so it is read only when a
  stage advances. A refetch every 2 seconds that does not change the content does
  not make it read again.

### Creating line

Only when probing has finished and something is still being created, one line
appears right below the player (requirement 11).

- "Being created" means that the thumbnail, the seek preview or the list preview
  is `pending`.
- Style: `flex items-center gap-2`, `text-xs text-fg-muted`, with `LoaderCircle`
  (`size-3.5`, `text-accent`, spinning) first.
- The text is `{作成中のもの}を作成中 · 再生はできます`, where the items being created
  are the stage names above joined with `と` (for example
  `シーク用プレビューと一覧用プレビューを作成中 · 再生はできます`). No counts or
  percentages.
- `role="status"`. When nothing is `pending` any more, the whole line disappears
  (also when only `failed` remains).

### Ended

When playback reaches the end, this layer appears (requirement 14).

- `bg-overlay` over the video. The control bar stays shown above it (it can also
  be used to watch again).
- **When there is a next video**, a central `bg-navbar` surface (`max-w-lg`)
  stacks:
  - the label `次の動画` (`text-xs font-semibold text-accent`)
  - a link to the next video: the thumbnail (`w-56`, 16:9, `rounded-md`, with the
    duration tag) and the title (`text-base font-semibold`, truncated at two
    lines), side by side
  - actions: a `primary` `次を再生` (`Play`) and a `secondary` `もう一度見る`
    (`RotateCcw`)
  - Below `sm`, the thumbnail is omitted and only the title and the actions are
    shown.
- **When there is no next video**, only a `secondary` `もう一度見る` sits at the
  centre, with no label.
- Nothing plays automatically, and there is no countdown.
- When the layer appears, a visually hidden `再生が終わりました` is read through
  `role="status"`.
  - Only when focus was inside the player does focus move to `次を再生` (or
    `もう一度見る` when there is none).
  - Focus of someone outside the player (in related videos, for example) is not
    taken.

## Interaction details

- **Keyboard**: there is no ←/→ seek-by-seconds.
  - Esc while the speed menu is open only closes the menu.
  - Esc in fullscreen only leaves fullscreen.
- **Tab order**: DOM order, at every width: heading band (logo → breadcrumb → ×)
  → player (centre control → actions in the state or ended layer → control bar)
  → tags → information row actions → each related video.
- **Reduced motion** (`prefers-reduced-motion: reduce`): spinning
  (`LoaderCircle`) and the `animate-fade-in` of layers appearing and disappearing
  stop. How states look does not change (library-ui.md 4).

## Accessibility

- Accessible names:
  - × `閉じる`
  - the centre control `再生` / `一時停止`
  - the control bar's `最初に戻る`
  - the edge handles `前の動画: {題名}` and `次の動画: {題名}`
  - the logo `ホーム`, and the breadcrumb `nav` `フォルダ`
  - the information row's `ファイルを開く` and `パスをコピー`
  - the `ul`s for file and technical information, `ファイルの情報` and `技術情報`
- Each file information value is preceded by a visually hidden name (such as
  `長さ`). The technical information row has `lang="en"`.
- How things are announced:

  | Role | Elements |
  | --- | --- |
  | Status (`status`) | The stage display, the creating line, loading, playback ended |
  | Alert (`alert`) | Playback failed, probe failed, video gone (404), the open-failed line |

- The title is the `h1`, and `document.title` stays `{題名} - vv` as before.
- Colour and contrast pairs:
  - The only new pair is `fg-muted` on `navbar` (the explanations in the stage
    display, probe failure and playback failure). The implementation PR adds it
    to `pairs` in `tokens.test.ts`.
  - All text in layers sits on the opaque `bg-navbar` ("Overlay layer"). Text on
    translucent `bg-overlay` cannot be checked through `pairs`, so no text goes
    there.
  - The others (`fg` and `fg-muted` on `bg`, `warning` on `bg`, and `fg`,
    `accent` and `success` on `navbar`) are already included.
  - Colours used only for icons (such as `danger` and `warning` on `navbar`) are
    not text, so they are not added as pairs.
