# UI Design: folder page for browsing videos through the folder hierarchy

**Feature**: [parent Issue #145](https://github.com/syudead/vv/issues/145)

The visual rules, the shell and the list structure follow
[Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md) and `@theme`
in [`web/src/index.css`](../../web/src/index.css). This document defines only what the folder
page **adds** to them (folder card, breadcrumb, folder toolbar, states). No new color, radius or
shadow tokens are added.

## Screen boundary

- `/folders` (top level) and `/folders/{rootId}/{segment}…` (each folder) sit inside the same
  `AppShell` as the library. They use the top bar, the sidebar and `window` scrolling as they are.
- The sidebar entry "Folders" sits right after "Library", with the lucide `Folder` icon (thicker
  strokes when selected, like the existing entries). It is selected on every URL under
  `/folders` ("Library" stays selected only on `/`, as before).
- Video cards reuse `web/src/library/VideoCard.tsx` as is. The folder page has no selection
  (parent Issue Out of scope), so it uses the card in the form that **does not draw the
  selection checkbox**. The look and behavior of the library card do not change.
- There is no table (list) view. The watch status filter and the search box were out of scope for
  #145, and were added in #228
  ([specs/013-library-search/ui-design.md "Folder screen"](../013-library-search/ui-design.md#folder-screen)).

## Page structure and hierarchy

The page stacks the parts below from top to bottom. The content of 3 and 4 is primary; 1 and 2
are secondary.

1. **Folder toolbar** (in the top bar): sort order and zoom only.
2. **Breadcrumb bar** (sticky right below the top bar): the current location and the way back up
   the hierarchy.
3. **Subfolder group**: the heading "Folders N" + a grid of folder cards.
4. **Video group**: the heading "Videos N" + a grid of video cards and loading of more pages.

With 0 subfolders, 3 is omitted together with its heading. With 0 direct videos, 4 is omitted the
same way. No heading shows "0". The top level (`/folders`) has only 3, with the heading "Media
folders N".

The page has a visually hidden `h1` holding the current folder name ("Folders" at the top level).
Moving to another folder (except returning from the player page) moves focus to the `h1` and
scrolls to the top of the page. This lets screen reader users know that they moved and where
they are.

## Spacing and density

- The page padding and the spacing between elements match `LibraryPage` (horizontal `px-3` /
  `sm:px-4`, vertical `gap-3` between elements, `pb-24` at the bottom).
- The grid has the same structure as the library: centered wrapping, `gap-2.5` between cards,
  card width `min(var(--card), 100%)`, and `--card` from the zoom's `--spacing-card-0..3`.
  **Folder cards and video cards have the same width**, so the same zoom gives the same column
  count.
- The space between groups (between 3 and 4) is about `mt-6`, clearly wider than the `gap-2.5`
  between cards.
- Headings align with the page's left edge, not the grid's left edge. The gap between a heading
  and its grid is `gap-2`.

## Folder card

A folder card uses the same box as a video card (`rounded-lg`, `bg-surface`, `shadow-card`,
`-translate-y-0.5` and `shadow-card-hover` on hover) and changes **only the top half's artwork**.
A video card's top half is a flat thumbnail that reaches the edges. A folder card draws a **tabbed
folder outline** inside the box and slides the thumbnails into it. The different outline lets the
user tell the two kinds apart at a glance.

The top half (`aspect-video`) consists of these layers, from back to front:

1. **Back panel**: a `rounded-md` `bg-elevated` surface inset inside the box. A tab of the same
   color sits on its top left (about 2/5 of the panel width, `h-3` high, only the top corners
   rounded).
2. **Inserted thumbnails**: 1 to 4 items from `previews`, stacked with small horizontal offsets
   like inserted sheets of paper.
   - Each image is about 70% of the panel width, 16:9, `rounded-sm`, with a `border-border-strong`
     edge and a `shadow-card` shadow.
   - They spread evenly left and right by count. Odd-numbered ones counted from the left sit
     slightly higher, and each is tilted by a few degrees either way (1 item: centered, not
     tilted).
   - The stack is centered vertically on the panel, and nothing covers it.
   - Images use `object-contain` (portrait videos are shown whole, not cropped). The alt text is
     empty, so they are not read aloud.
3. **Bring-forward preview (hover)**: moving the mouse horizontally over the outline brings one
   image forward, chosen by pointer position (the width split evenly by count).
   - That image untilts, scales to 1.12, and comes to the front at the panel center (shadow
     `shadow-card-hover`).
   - When the pointer leaves, it returns to its position and tilt.
   - The motion takes 200 ms. With reduced motion, it only switches.
   - Non-mouse pointers do not bring an image forward (a tap opens the folder).
4. **Preview playback**: when the forward image has a `previewUrl` (the list preview is
   generated and can be served), it plays after 400 ms.
   - It plays the same list preview video as the video card's hover preview, inside that one
     frame, muted and looping. Until it starts, the thumbnail stays.
   - Moving to another image switches to that image's video.
   - When the pointer leaves, playback stops and the load is released.
   - An image without a `previewUrl` (not generated, failed, or the file is gone) or one that
     fails to load only comes forward as a thumbnail.

With 0 `previews`, only the back panel and the tab are drawn. No empty frame, broken image or "no
image" text appears (Acceptance criterion 6).

The bottom half uses the same padding as a video card (`px-3 pt-2 pb-3`, `gap-1` between lines)
and holds:

- **Folder name**: `text-sm font-medium text-fg` like a video title, truncated at 2 lines
  (`line-clamp-2 break-all`). The `title` attribute holds the whole name.
- **Counts**: "N videos · M folders" in `text-xs text-fg-muted tabular-nums`. Shown even when 0
  (Acceptance criteria 4 and 6 check this text).
- **Path** (top-level cards only): `rootPath` on one line in `text-xs text-fg-muted`, with the
  **start** truncated when it does not fit (`…/a/movies`). Registered folders with the same name
  differ at the end of the path (Acceptance criterion 2). The `title` attribute holds the whole
  path.

The whole card is one link. Its accessible name is "{name}, N videos, M folders" (at the top
level, followed by ", {path}"). When the link has keyboard focus, an outer outline in the same
`link` color and thickness as the global `:focus-visible` appears outside the (rounded) box. The
box is `overflow-hidden`, so the link's own outline would be clipped. Decorative motion (the hover
lift and the shadow transition) stops under `motion-reduce:`, and the final shadow state stays.

## Breadcrumb bar

- It is fixed right below the top bar with `sticky` (`top-navbar`) and stays while scrolling
  (parent Issue "control priority").
  - The surface matches the top bar: `bg-bg/90` and `backdrop-blur-md` (the grid below shows
    through faintly), with `border-border` on the bottom edge.
  - The height is one line, lower than the top bar (about `h-10`). The horizontal padding matches
    the page.
- It is an `ol` inside a `nav` (`aria-label="Breadcrumbs"`). The segments are "Folders ›
  registered folder name › … › current folder". The separator is lucide `ChevronRight`
  (`text-fg-subtle`, `aria-hidden`).
- The text is `text-xs`. Ancestor segments are `text-fg-muted` links, with `text-fg` and
  `bg-hover-wash` on hover. The current location is `text-fg font-medium` plain text, not a link,
  with `aria-current="page"`.
- Each segment is one line, and a long name is truncated at the end (ancestors up to a set
  maximum width; the current location uses the remaining width). The `title` attribute holds the
  whole name. The bar itself does not wrap.
- Below `md` with 4 or more segments, the **start is truncated** and only "… › parent › current
  location" is shown (parent Issue "width-based layout").
  - "…" is plain text, not a control, and is not read aloud.
  - Hidden segments are reachable by following "parent" up, and the top level is always reachable
    from the sidebar's "Folders".
  - This switch uses only CSS width breakpoints (library-ui.md 4).
- At the top level (`/folders`) there is only the "Folders" segment, and it is the current
  location.

## Toolbar

> **#228 replaced this section
> ([specs/013-library-search/ui-design.md "Folder screen" and "Toolbar"](../013-library-search/ui-design.md#folder-screen)).**
> The folder page toolbar now uses the centered layout with the search box first, and has the
> search box, the filter, and sorting (7 kinds, direction, and shuffle for random order). The
> "right-aligned because there is no search box" text below describes the state before #228 and
> no longer matches the implementation.

Inside the top bar (`TopBarPortal`), the toolbar holds **only the sort order and the zoom**, with
the same parts and the same width breakpoints as the library toolbar. There is no search box, so
the controls are right-aligned next to "Refresh" at the right end of the top bar. This avoids
controls floating alone in the center, and keeps them quieter than the content.

- `md` and up: the sort menu button ("Date added", "Title").
- `xl` and up: the zoom slider.
- Narrower: one "View and sort" button opens both the sort order and the zoom.
- The top level (`/folders`) shows no videos, so it has no sort order, only the zoom.

Changing the sort order reloads only the video group. The subfolder group is neither re-rendered
nor re-sorted (Acceptance criterion 8). The sort order and the zoom read and write the same saved
values as the library (Requirement 13).

## States

| State | Appearance |
| --- | --- |
| Top level loading | The "Media folders" heading and 6 folder card skeletons (`Skeleton` in the outline shape) |
| 0 registered folders | The library's empty state part with "No media folders yet", and the action "Open Settings" (a link to `/settings`) |
| Folder loading | The breadcrumb first shows the segments known from the URL, with a skeleton only for the registered folder segment that needs a name. The content shows 3 folder card skeletons and 6 video card skeletons |
| Registered folder is empty (before a scan) | The empty state part with "No videos in this folder yet" and "No videos or subfolders were found. Put files there, then scan.", and the existing action "Scan" |
| Folder not found (404) | The empty state part (`FolderX` icon) with "This folder wasn't found" and "It was removed from the media folders, or its videos are gone.", and the action "Go to all folders" (a link to `/folders`). No empty grid. The registered folder name is unknown, so the breadcrumb is only "Folders › the name of the segment being opened" ("Folders › …" for a URL that cannot be parsed) |
| Load failure (other than 404) | The same `LoadFailed` (retry) as the library |
| Loading more, or failing to | The same as the library (6 skeletons; on failure, "Couldn't load more" and retry below the grid) |
| Unplayable video, or thumbnail not generated | The existing video card display, unchanged |

## Accessibility

- A folder card is a link. Its accessible name has the form above. Preview images have empty alt
  text and are not read.
- The breadcrumb is `nav` > `ol`, with `aria-current="page"` on the current location. The
  separators and "…" are not read.
- The headings "Folders N" and "Videos N" are `h2`. The count is read as part of the heading.
- Keyboard: the Tab order is the DOM order, and the main content follows the shell's existing
  order. At every width, the main content order is "breadcrumb → subfolder cards → video cards".
  The folder toolbar is inside the top bar (`TopBarPortal`), so it comes before the main content,
  as in the library. The shell part depends on the sidebar state:
  - Expanded or rail (640 px and up): ☰ → (from `sm`) logo → folder toolbar (sort order → zoom;
    one "View and sort" button when narrow) → "Refresh" → sidebar entries → main content.
  - Closed drawer (the initial state below 640 px): the sidebar is `inert` and outside the Tab
    order. ☰ → folder toolbar → "Refresh" → main content.
  - Open drawer (opened with ☰): "Refresh" → "Close menu" → sidebar entries → main content.

  Enter opens a folder or plays a video. The user goes back through the breadcrumb (Shift+Tab) or
  the browser's Back.
- The added text and surface pairs are the existing `fg` and `fg-muted` on `bg`, `surface` and
  `elevated`, already in the pairs in `tokens.test.ts`. No new pair is added.
