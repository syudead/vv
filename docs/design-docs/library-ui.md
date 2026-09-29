# Library UI: visual rules and list layout

- Status: Adopted
- Scope: the visual rules that the `web/` screens (shell, lists, video page) follow, and how the
  repository verifies that the rules hold

This document records the layout shared by the screens and the reasons behind it. The values
themselves (colors, radii, card widths) are in `@theme` of `web/src/index.css`, and the pairs that
are checked are in `web/src/theme/tokens.test.ts`. Copying them here would create a second source
of truth, so this document does not copy them.

## 1. Visual values live in one CSS file, and tests guarantee contrast

| | |
| --- | --- |
| **Decision** | The rules live in `@theme` of `web/src/index.css`. Screens and components set colors with the utility classes that Tailwind generates from it (`bg-surface`, `text-fg-muted`, `rounded-md` and so on). A raw hex color, `rgb()` or a default Tailwind palette name (`neutral-*`, `sky-*`) in `web/src/**` fails the scan in `web/src/theme/tokens.test.ts`. |
| **Why** | With CSS as the source of truth, the screens have no option other than token names. |
| **Rejected** | Keeping the values in TypeScript and generating CSS: the screens are still written with Tailwind class names, so every name exists twice, and one more generated artifact that must not be hand-edited is added. |

Contrast is not left to the human eye. The same `tokens.test.ts` reads the colors from `@theme`,
computes sRGB, then relative luminance, then contrast with the WCAG 2 formula, and asserts that
every text and surface pair is 4.5 or higher. The check needs to **read only that file** because
the values are in one place.

| | |
| --- | --- |
| **Decision** | Contrast is checked by `tokens.test.ts`, not by an external tool. |
| **Why** | The values are in one file, so reading that file is enough. |
| **Rejected** | Running an external checker (axe, Lighthouse) in CI: it needs a real browser and rendering, and what it adds beyond contrast is findings that fall within what a human reviews. |

**When you add a text color or a surface color, add it to the pairs in `tokens.test.ts`.** A color
that is not in a pair is not checked. The test also fails on a pair that is listed but missing from
the CSS, but it cannot catch the reverse.

The VVMDM screens use these tokens.

| Role | Token |
| --- | --- |
| The provided dark colors | `navbar`, `bg`, `surface` |
| Primary action (cyan) | `accent` |
| Hover | `accent-hover` |
| Pressed and selected | `accent-active` |
| Border of shared controls | `control-border` |
| Keyboard focus | `link` |

Danger, warning and success use their own semantic colors plus text and icons, so they are not
confused with the cyan interaction states. `tokens.test.ts` checks the contrast of normal text
against surfaces, and of the main borders and the focus indicator.

## 2. Only the dark color scheme exists; there is no light/dark switch

`html` declares `color-scheme: dark`, and one dark color set is defined. There is no
`prefers-color-scheme` branch and no toggle.

| | |
| --- | --- |
| **Decision** | Implement one dark color set only. Tokens are named by **role** (`bg`, `surface`, `elevated`, `fg`, `fg-muted`, `accent`, `danger`, `warning`), not by color (`neutral-850`). |
| **Why** | Maintenance cost. A switch doubles the contrast pairs to check. One set goes stale because nobody looks at it, yet it has to be kept as a test target. |
| **Rejected** | A light/dark switch now. With role names, a future light scheme only means "define one more set of values", and the screens need no rewrite. Having no branch now and adding one later are compatible. |

## 3. The list does not use virtual scrolling

The list uses no virtual scrolling library. The goal is that "the response to an action starts at
once", not fewer DOM elements as such.

| | |
| --- | --- |
| **Decision** | No virtual scrolling. |
| **Why** | The list is a **wrapping grid**. Cards per row depend on the viewport width and the card width, so virtualization means owning that calculation. Restoring the position when returning to the list would also have to be rebuilt in the virtualized coordinate system. The list loads 60 items per page, so the DOM holds only what the user has loaded. No dependency is added. |
| **Rejected** | A virtual scrolling library, for the reasons above. |

**Measure before revisiting this decision.** Even if the response turns out to be slow, do not jump
to virtual scrolling; first measure what is slow.

## 4. Width breakpoints are in CSS, with the sidebar as the exception

| | |
| --- | --- |
| **Decision** | Layout changes by width use the default Tailwind breakpoints in CSS. |
| **Why** | Watching the width in JavaScript brings the watcher itself, a one-frame flash of the default layout on first render, and a fake `matchMedia` for tests. |
| **Rejected** | Width detection in JavaScript, except for the sidebar below. |

`web/src/shell/useSidebar.ts` reads the viewport width with `matchMedia` to choose between the
expanded sidebar, the rail and the drawer. It interprets the user's open or closed choice per width
and holds the drawer's open state, which CSS alone cannot express. The breakpoints (1024px, 640px)
equal Tailwind's `lg` and `sm`.

**Reduced motion (`prefers-reduced-motion`) is handled in CSS.** Decorative transitions stop with
the `motion-reduce:` variant. The final color state and the focus indicator always apply. Only the
motion stops, so the result of an action stays distinguishable.

## 5. Layout is not verified by machines

Machines check contrast, the absence of raw colors, and component behavior (selection, filtering,
sending the playback position, and so on). **A human checks the composition itself and the layout
changes by width on real devices.**

| | |
| --- | --- |
| **Decision** | Composition and responsive layout are checked by a human on real devices. |
| **Why** | jsdom applies no CSS, so `position: fixed`, media queries and wrapping are never resolved. Faking them leads to the worst state: "the tests pass but the screen is broken". |
| **Rejected** | Visual regression tests (comparison with baseline images): they add dependencies, and glyph differences between environments cause false positives, so every later UI change would also need a baseline update. |

Only **what cannot be verified** goes to a human.

## 6. List layout

The list uses a dense layout suited to a management screen. It combines a top navigation bar, a
filter strip and boxed cards, with the VVMDM colors and typefaces.

- **Shell**: `TopBar` at the top (`web/src/shell/TopBar.tsx`) holds ☰, the logo and the "Refresh"
  button for scanning. Each screen inserts its own toolbar between them (`TopBarPortal`).
  Navigation and the entry to settings are in `Sidebar`.
  - The lower part of the sidebar shows "Settings" and "Sign out" to the owner, and only "Sign in"
    to a guest (someone not signed in).
  - For a guest, the upper part has only "Library" and "Folders", and the top bar shows neither
    "Refresh" nor the scan progress (specs/016-single-account-auth/ui-design.md "Shell entries",
    "Guest degradation").
- **Sidebar**: three states: expanded, rail (icons only), and drawer (narrow widths). Collapsing it
  gives the grid more width (the card width comes from the zoom level).
- **Toolbar**: search, filters, view mode (library only), zoom level and sort order. At narrow
  widths, view mode, zoom level and sort order move into the "View and sort" group. The count
  appears on the row above the grid in the library and in search results, and in the section
  heading for the direct contents of a folder screen.
- **Card** (`web/src/videoList/VideoCard.tsx`): draws the thumbnail, the duration at its bottom
  right, the playback progress along the bottom edge, and the title.
  - In the library grid and on the folder screen, a row of the video's tags goes under the title.
    A video without tags gets no row (specs/014-video-tags/ui-design.md "Library card").
  - Pressing a tag filters by it in the library. On the folder screen it opens the library
    filtered by that tag (`/?tag=<id>`).
  - A tag that comes only from the folder name keeps the same size but has no fill, a dashed
    border and a Folder marker, which sets it apart from tags added by hand. The video page shows
    no × for it. "Remove tag" on the selection bar offers only tags added by hand
    (specs/017-folder-groups/ui-design.md "Folder-derived tag chip").
  - In folder-screen search results, a location row goes under the title.
  - On every screen, only one hover preview plays at a time. Changing the zoom level returns to the
    card that was at the top of the screen. The grid (`web/src/videoList/Grid.tsx`) is shared by
    the library and the folder screen.
- **Folder card** (`web/src/folders/FolderCard.tsx`): uses the same width and border as a video
  card, and is told apart by the folder artwork and a Folder icon next to the title.
  - The name shows up to two lines, and `title` shows the full name.
  - The path of a media folder is shown only to the owner, with the end kept visible first;
    `title` shows the omitted full path.
  - An empty registered root tells the user that files must be placed there and then scanned.
- **Group card** (library only, `web/src/library/GroupCard.tsx`): the library list reads
  `GET /api/library` and mixes folder groups in as items equal to videos.
  - The card uses the same box and size as a video card. The thumbnail frame holds the same folder
    artwork as a folder card (`web/src/videoList/FolderArt.tsx`, up to four member thumbnails
    stacked). A panel at its bottom right shows the count ("12 videos") and the total length.
  - A bar with the share of watched members appears only while watching. Then come the group name
    and a row of member tags.
  - Pressing the card opens the video page of the member to continue (`/videos/{openVideoId}`).
    Guests see no watch state, watched count or bar. The folder screen and its search results still
    show videos one by one (specs/017-folder-groups/ui-design.md "Group card").
  - Ungrouping, grouping the direct contents, and turning a group into a tag are not on the card.
    They are in the menu at the right end of the "Videos N" heading row on the folder screen (owner
    only, `web/src/folders/FolderGroupingMenu.tsx`) and in the group name row on the video page.
  - After such an action, the cached library list is discarded and reloaded the next time the
    library opens (specs/017-folder-groups/ui-design.md "Folder grouping menu").
- **List view** (library only): each row shows the title, watched, length, quality, size and date
  added. A group row uses the same columns: the first member's thumbnail, a Folder marker and the
  count under the title, "3 / 12" in the watched column, and the total length and size. Quality is
  empty (specs/017-folder-groups/ui-design.md "List view row").
- **Selection** (library only, owner only): in the grid, the checkbox appears on pointer hover or
  focus, and always on devices that cannot hover. In the list view, checkboxes are always shown
  faintly, and all of them show while anything is selected. What a group's checkbox selects and
  how it counts is in
  [017 UI design "Pressing and selection"](../../specs/017-folder-groups/ui-design.md#pressing-and-selection).
- **Selection bar**: appears fixed at the bottom of the screen as soon as one item is selected. It
  does not move or resize the toolbar.
  - At wide widths, "N videos selected", "Add tag", "Remove tag" and "Visibility" form one group
    without dividers, followed on the same row by a divider, "Select all" and clear.
  - Below `sm`, the top row holds the count, "Select all" and clear, and the bottom row holds the
    two tag actions and "Visibility". Where the three actions do not fit, "Visibility" wraps to the
    right end of the next row.
  - Action names are not shortened, so that they stay readable on devices that cannot hover. The
    tag actions are in
    [014 UI design "Selection bar"](../../specs/014-video-tags/ui-design.md#selection-bar), and the
    visibility menu is in
    [016 UI design "Selection bar"](../../specs/016-single-account-auth/ui-design.md#selection-bar).
- **Interaction states**: normal, hover, focus-visible, active, selected and disabled are clearly
  distinct and consistent across components. Keyboard focus is an outer outline in the accent
  color. The search box shows focus on its outer frame, not on the inner `input`, to avoid a double
  outline.

## 7. Sidebar navigation

Each sidebar item opens its screen.

## 8. Video page layout

The video page (`/videos/:id`) is for watching, so it is less dense than the list. The detailed
shapes and labels are in
[specs/012-video-detail-ia/ui-design.md](../../specs/012-video-detail-ia/ui-design.md); this
section records only the layout decisions.

- **Parts**: a header strip, the player, the title, tags, the visibility toggle, file details and
  related videos. There is no shell; the page is a layer over the list and closes with × or Esc.
  - The header strip exists only on the video page. It holds the logo (to home), a breadcrumb to
    the video's folder, and ×.
  - The return target is the list open before the page. Moving through related videos or "Play
    next" keeps that first list.
  - Tags sit right under the title, and the title and tags form one unit (details in
    [specs/014-video-tags/ui-design.md "Video page tags"](../../specs/014-video-tags/ui-design.md#video-page-tags)).
    The visibility toggle (`role="switch"`) is right under that unit, above the file details.
  - Guests see no tags, visibility toggle, "Open file" or "Copy path". Under the title they see
    only the two lines of file details and technical details
    ([specs/016-single-account-auth/ui-design.md "Visibility toggle", "Guest degradation"](../../specs/016-single-account-auth/ui-design.md#visibility-toggle)).
- **Width breakpoints in CSS** (as in 4): at `lg` and wider, related videos form a column on the
  right; below that, everything stacks. × exists only once, in the strip. Below `md`, the
  breadcrumb collapses to its last level.
- **Two lines under the title, with no rules, frames or labels**: line 1 is the file details with
  icons (length, size, date added) and, at the right end, "Open file" and "Copy path". Line 2 is
  the technical details (resolution, container, codec), the smallest and most subdued. The
  location is already in the strip as a breadcrumb, so no path appears under the title.
- **States and failures appear inside the player**: loading, scan stages, read failure, playback
  failure, video gone and playback ended overlay one container above the player. The container
  decides the stacking order and shows one at a time.
  - The only state outside the player is the single "Creating" line right under it.
  - Text in the layer sits on the opaque `bg-navbar` surface, because `tokens.test.ts` cannot check
    the contrast of text on the translucent `bg-overlay`.
- **Only control-bar parts are video.js components**: restart, playback speed, current time /
  duration, and "Converting for playback". State displays live in React, because a video.js
  component cannot share state with the rest of the page.
- **Keyboard shortcuts are handled for the whole page**: video.js control-bar components stop key
  propagation, so keys are caught in the capture phase on `window`. Only keys that operate a
  focused button or slider itself are passed through.
- **Group members only add to the same page**: a secondary line above the title with the group name
  and the member's position, an "Up next" member list and divider at the top of the related-videos
  column, and, instead of the playback-ended layer, a notice for the next member (5 s, cancelable,
  Esc cancels).
  - For the owner, the group name row opens a menu with "Ungroup" and "Turn the group into a tag".
    After an action, the page reloads the video and related videos and returns to the normal video
    layout. Esc on an open menu closes only the menu.
  - The previous and next handles move within the group. Pages of videos outside any group do not
    change (details in
    [specs/017-folder-groups/ui-design.md "Video page"](../../specs/017-folder-groups/ui-design.md#video-page)).
  - The `lg` width is read with `matchMedia` only to scroll the current member's row into view on
    wide screens. On narrow widths the whole page would scroll and the player would leave the
    view, so it does not scroll.
- **Central touch controls appear on `pointer: coarse` devices**: a CSS media condition decides
  this, and `matchMedia` is not read (the same reason as in 4).
