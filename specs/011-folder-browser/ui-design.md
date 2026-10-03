# UI Design: Folder screen for browsing videos by folder hierarchy

**Feature**: [parent Issue #145](https://github.com/syudead/vv/issues/145)

Sources: the visual rules, the shell and the list layout follow
[Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md)
and `@theme` in [`web/src/index.css`](../../web/src/index.css). This document
defines only what the folder screen **adds** to them (folder cards, the
breadcrumb, the folder toolbar and the states). No new colour, radius or shadow
token is added.

## Screen boundary

- `/folders` (the top) and `/folders/{rootId}/{segment}…` (each folder) sit
  inside the same `AppShell` as the library. The top bar, the sidebar and `window`
  scrolling are used as they are.
- The sidebar's `フォルダ` entry comes right after `ライブラリ`, with the lucide
  `Folder` icon (when selected, the stroke gets heavier, like the existing
  entries). It is selected for every URL under `/folders` (`ライブラリ` stays
  selected only on `/`, as before).
- Video cards reuse `web/src/library/VideoCard.tsx` as is. The folder screen has
  no selection (out of scope for the parent Issue), so the card is used in a form
  that **does not draw the selection checkbox**. The look and behaviour of library
  cards do not change.
- There is no table (list) view. The watch-state filter and the search field were
  out of scope for #145, and were added by #228
  ([specs/013-library-search/ui-design.md "Folder screen"](../013-library-search/ui-design.md#folder-screen)).

## Page structure and hierarchy

Top to bottom, in this order. The content of 3 and 4 is primary; 1 and 2 are
secondary.

1. **Folder toolbar** (inside the top bar): sort and zoom only.
2. **Breadcrumb band** (sticky right below the top bar): the current location and
   the way back up the hierarchy.
3. **Child folder group**: the heading `フォルダ N` + a grid of folder cards.
4. **Video group**: the heading `動画 N` + a grid of video cards and loading more.

When there are no child folders, 3 is omitted with its heading; when there are no
direct videos, 4 is omitted with its heading. No heading reads "0". The top
(`/folders`) has only 3, with the heading `メディアフォルダ N`.

The page has a visually hidden `h1` holding the current folder name (`フォルダ` at
the top). When the user moves to another folder (except when returning from the
player), focus moves to the `h1` and the page scrolls to the top, so that
screen-reader users learn that they moved and where they are.

## Spacing and density

- The outer page padding and the spacing between elements are the same as
  `LibraryPage` (horizontal `px-3` / `sm:px-4`, vertical `gap-3` between
  elements, `pb-24` at the bottom).
- The grid has the same structure as the library (centred wrapping, `gap-2.5`
  between cards, card width `min(var(--card), 100%)`, where `--card` is the zoom's
  `--spacing-card-0..3`). **Folder cards and video cards have the same width**, so
  at the same zoom they give the same number of columns.
- The space between groups (between 3 and 4) is about `mt-6`, clearly wider than
  the `gap-2.5` between cards.
- Headings align with the left edge of the page, not the left edge of the grid,
  with `gap-2` between heading and grid.

## Folder card

A folder card uses the same box as a video card (`rounded-lg`, `bg-surface`,
`shadow-card`; on hover `-translate-y-0.5` and `shadow-card-hover`) and changes
**only the picture in the upper half**. A video card's upper half is a flat
thumbnail that runs to the edges; a folder card draws **a folder outline with a
tab** inside the box and slides thumbnails into it. The different outline lets
the user tell the two kinds apart at a glance.

The upper half (`aspect-video`) has these layers, back to front:

1. **Back panel**: a `rounded-md` `bg-elevated` surface inset from the box. At
   the top left it has a tab in the same colour (about 2/5 of the panel width,
   height `h-3`, only the top corners rounded).
2. **Inserted thumbnails**: 1 to 4 `previews`, stacked with small horizontal
   offsets like papers slid into a folder. Each image is about 70% of the panel
   width, 16:9, `rounded-sm`, with a `border-border-strong` edge and a
   `shadow-card` shadow. They spread evenly left and right by count; counting
   from the left, the odd-numbered ones sit slightly higher, and each is tilted by
   a few degrees either way (one image is centred and not tilted). The stack is
   centred vertically on the panel and nothing covers it. Images use
   `object-contain` (portrait videos are shown whole, not cropped). Their
   alternative text is empty and they are not read aloud.
3. **Bring-to-front preview (hover)**: when the mouse moves horizontally over the
   outline, the one image for the pointer position (the width divided equally by
   the count) straightens, scales to 1.12 and comes to the front at the centre of
   the panel (shadow `shadow-card-hover`). When the pointer leaves, it returns to
   its position and tilt. The motion takes 200ms; with reduced motion it only
   switches. Pointers other than a mouse do not bring images forward (a tap opens
   the folder).
4. **Preview playback**: when the image in front has a `previewUrl` (the list
   preview is generated and can be served), 400ms after it comes forward, the same
   list preview video as the video card's hover preview plays muted and looping
   inside that one frame. Until it starts playing, the thumbnail stays. Moving to
   another image switches to that image's video. When the pointer leaves, playback
   stops and the load is released. An image with no `previewUrl` (not generated,
   failed, or the file is gone), or one that failed to load, only comes forward as
   the thumbnail.

With zero `previews`, only the back panel and the tab are drawn. No empty frame,
broken image or "no image" text appears (acceptance criterion 6).

The lower half has the same padding as a video card (`px-3 pt-2 pb-3`, `gap-1`
between lines) and holds:

- **Folder name**: the same `text-sm font-medium text-fg` as a video title,
  truncated at two lines (`line-clamp-2 break-all`). The `title` attribute holds
  the whole name.
- **Counts**: `動画 N 本 · フォルダ M 件` in `text-xs text-fg-muted tabular-nums`.
  Shown even when 0 (acceptance criteria 4 and 6 check this wording).
- **Path** (top-level cards only): `rootPath` in one line of
  `text-xs text-fg-muted`; when it does not fit, the **start** is truncated
  (`…/a/movies`), because the end of the path is what tells registered folders
  with the same name apart (acceptance criterion 2). The `title` attribute holds
  the whole path.

The whole card is one link, with the accessible name
`{名前}、動画 N 本、フォルダ M 件` (at the top, followed by `、{パス}`). Focus is shown
outside the box (with its rounded corners) as an outer outline in the same `link`
colour and thickness as the global `:focus-visible`, only when the link has
keyboard focus. The box is `overflow-hidden`, so the link's own outline would be
clipped and invisible. Decorative movement (the hover lift and shadow transition)
stops under `motion-reduce:`, and the final shadow state stays.

## Breadcrumb bar

- It is fixed right below the top bar with `sticky` (`top-navbar`) and stays
  while scrolling (parent Issue `操作の優先順位`). The surface is the same as the top
  bar, `bg-bg/90` with `backdrop-blur-md` (the grid scrolling underneath shows
  through faintly), with a `border-border` bottom edge. Its height is one line,
  lower than the top bar (about `h-10`). The horizontal padding matches the page.
- An `ol` inside `nav` (`aria-label="パンくず"`). The segments are
  `フォルダ › registered folder name › … › current folder`. The separator is lucide
  `ChevronRight` (`text-fg-subtle`, `aria-hidden`).
- Text is `text-xs`. Ancestor segments are `text-fg-muted` links, with `text-fg`
  and `bg-hover-wash` on hover. The current location is `text-fg font-medium`
  text, not a link, with `aria-current="page"`.
- Each segment is one line, and long names are truncated at the end (ancestors up
  to a set maximum width; the current location uses the remaining width). The
  `title` attribute holds the whole name. The band itself never wraps.
- Below `md`, with four or more segments, the **start** is collapsed and only
  `… › parent › current` is shown (parent Issue `幅による出し分け`). `…` is not
  operable and not read aloud. Hidden segments are reached by following the
  parent step by step, and the top is always reachable from the sidebar's `フォルダ`.
  This switch uses only CSS width breakpoints (library-ui.md 4).
- At the top (`/folders`) there is only the one segment `フォルダ`, which is the
  current location.

## Toolbar

> **This section was replaced by #228 ([specs/013-library-search/ui-design.md "Folder screen" and "Toolbar"](../013-library-search/ui-design.md#folder-screen)).**
> The folder screen's toolbar now uses the centred layout with the search field
> first, and has the search field, the filter and the sort (seven sorts,
> direction, and Shuffle for random). The text below about right alignment
> "because there is no search field" describes the state before #228 and no longer
> matches the implementation.

Inside the top bar (`TopBarPortal`), **only sort and zoom** are placed, with the
same components and the same width breakpoints as the library toolbar. There is
no search field, so the controls are right-aligned and grouped next to the top
bar's refresh at the right end (to avoid controls floating alone in the centre,
and to keep them quieter than the content).

| Width | Controls |
| --- | --- |
| `md` and up | The sort menu button (`追加日`, `題名`) |
| `xl` and up | The zoom slider |
| Narrower | One "view and sort" button (`表示と並び順`) opens sort and zoom together |
| The top (`/folders`) | Zoom only; it shows no videos, so there is no sort |

Changing the sort reloads only the video group; the child folder group is neither
redrawn nor re-sorted (acceptance criterion 8). Sort and zoom read and write the
same saved values as the library (requirement 13).

## States

| State | What the screen shows |
| --- | --- |
| Top level loading | The `メディアフォルダ` heading and 6 folder card skeletons (a `Skeleton` shaped like the outline) |
| Zero registered folders | The same component as the library's empty state, with `メディアフォルダが登録されていません`; the action is `設定を開く` (a link to `/settings`) |
| Folder loading | The breadcrumb first shows the segments known from the URL, with a skeleton only for the registered folder segment, whose name is needed. The content is 3 folder card skeletons and 6 video card skeletons |
| Registered folder is empty (before scanning) | The empty-state component with `このフォルダにはまだ動画がありません` and `取り込むと、ここに並びます。`; the action is the existing `取り込む` |
| Folder not found (404) | The empty-state component (`FolderX` icon) with `このフォルダは見つかりません` and `登録が外れたか、中の動画が無くなりました。`; the action is `フォルダの一覧へ` (a link to `/folders`). No empty grid. The registered folder's name is unknown, so the breadcrumb is only `フォルダ › name of the segment being opened` (`フォルダ › …` for a URL that cannot be interpreted) |
| Load failure (other than 404) | The same `LoadFailed` (retry) as the library |
| Loading more, or loading more failed | Same as the library (6 skeletons; on failure, `続きを取得できません` and retry below the grid) |
| Unplayable videos, or videos without a thumbnail | The video card's existing display |

## Accessibility

- A folder card is a link. Its accessible name has the form above, and the
  preview images have empty alternative text and are not read.
- The breadcrumb is `nav` > `ol`, with `aria-current="page"` on the current
  location. The separators and `…` are not read.
- The headings `フォルダ N` and `動画 N` are `h2`. The count is read as part of the
  heading.
- Keyboard: Tab order is DOM order, with the body after the shell's existing
  order. Within the body, the order at every width is breadcrumb → child folder
  cards → video cards. The folder toolbar is inside the top bar (`TopBarPortal`),
  so, as in the library, it comes before the body. The shell part depends on the
  sidebar state:

  | Sidebar state | Tab order |
  | --- | --- |
  | Expanded or rail (640px and up) | ☰ → logo (`sm` and up) → folder toolbar (sort → zoom, or the single `表示と並び順` button at narrow widths) → refresh → sidebar entries → body |
  | Closed drawer (initial state below 640px) | The sidebar is `inert` and not in the Tab order. ☰ → folder toolbar → refresh → body |
  | Open drawer (opened with ☰) | Refresh → `メニューを閉じる` → sidebar entries → body |

  Enter opens a folder or plays a video. Going back uses the breadcrumb
  (Shift+Tab) or the browser's Back.
- The added text and surface pairs are the existing `fg` and `fg-muted` on `bg`,
  `surface` and `elevated`, already in the pairs in `tokens.test.ts`. No new pair
  is added.
