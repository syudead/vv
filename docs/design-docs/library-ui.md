# Library UI: visual rules and list layout

- Status: adopted
- Scope: the visual rules that the screens in `web/` (shell, lists, video page)
  follow, and how compliance with those rules is checked

This document records the layout shared by the screens and the reasons behind
it. The values themselves (colours, radii, card widths) are in `@theme` in
`web/src/index.css`, and the checked pairs are in
`web/src/theme/tokens.test.ts`. They are not copied here, because a copy would
make two sources of truth.

## 1. Visual values in one CSS location, with contrast guaranteed by tests

The rules live in `@theme` in `web/src/index.css`. Screens and components set
colours through the utility classes Tailwind generates from it (`bg-surface`,
`text-fg-muted`, `rounded-md` and so on). A raw hex colour, `rgb()`, or a
Tailwind default palette name (`neutral-*`, `sky-*`) anywhere in `web/src/**`
fails the scan in `web/src/theme/tokens.test.ts`.

CSS is the source of truth because it leaves screens no option other than token
names. Keeping values in TypeScript and generating CSS is possible, but screens
are written with Tailwind class names anyway, so the names would be duplicated
and there would be one more generated artefact that must not be hand-edited.

Contrast is not left to the eye. The same `tokens.test.ts` extracts colours from
`@theme`, computes sRGB → relative luminance → contrast with the WCAG 2 formula,
and asserts that every text/surface pair is at least 4.5. The check needs to
**read only that file** because the values are in one place. Adding an external
checker (axe, Lighthouse) to CI was rejected: it needs a real browser and
rendering, and what it adds beyond contrast are findings that people review
anyway.

**When you add a text or surface colour, add it to the pairs in
`tokens.test.ts`.** A colour not in the pairs is not checked. The test catches a
pair listed but missing from CSS, but cannot catch the reverse.

VVMDM screens use the supplied dark colours for `navbar`, `bg` and `surface`;
cyan `accent` for primary actions, `accent-hover` for hover, and
`accent-active` for pressed and selected. Borders of shared controls use
`control-border`, and keyboard focus uses `link`. Danger, warning and success
each use their own semantic colour together with text and an icon, so they are
not confused with the cyan interaction states. Only the favorite mark uses its
own pink, `favorite`, a family separate from both the cyan of selection and
focus and the red of `danger` (specs/035-favorites/ui-design.md, Mark).
`tokens.test.ts` checks the contrast of body text against surfaces, and of the
main borders and focus.

## 2. Dark scheme only, without a light/dark switch

`html` declares `color-scheme: dark`, and one dark set of colours is defined.
There is no `prefers-color-scheme` branch and no switch.

The reason is maintenance cost. A switch doubles the sets under contrast
checking, and one of them would go stale unseen while still having to be kept
under test.

Instead, token names describe **roles** (`bg`, `surface`, `elevated`, `fg`,
`fg-muted`, `accent`, `danger`, `warning`), not colours (`neutral-850`). If a
light scheme is ever needed, the work is defining a second set of values, with
no changes to screens. Having no branch now does not prevent adding one later.

## 3. No virtual scrolling

Lists do not use a virtual scrolling library. The goal is that responses to
input start immediately, not fewer DOM elements as such.

Virtual scrolling was avoided because the list is a **wrapping grid**. The
number of cards per row changes with screen width and card width, so
virtualization would mean computing that count ourselves, and restoring the
position on returning to the list would have to be rebuilt in the virtualized
coordinate system. The list loads 60 items per page, so the DOM holds only what
the user has loaded. No dependency is added either.

**Revisit this decision only after measuring.** Even if responses turn out to be
slow, measure what is slow first instead of moving straight to virtual
scrolling.

## 4. Width breakpoints in CSS, and the sidebar exception

Layout variations by width use Tailwind's default breakpoints in CSS. Watching
width in JavaScript would bring the watcher itself, a one-frame flicker where
the first render uses the default, and a `matchMedia` stub for tests.

The sidebar's expanded, rail and drawer states are decided by
`web/src/shell/useSidebar.ts`, which reads the screen width with `matchMedia`.
It interprets the user's open/close choice per width and keeps the drawer's
open state, which CSS alone cannot express. The breakpoints (1024px, 640px) equal
Tailwind's `lg` and `sm`.

**Reduced motion (`prefers-reduced-motion`) is handled in CSS.** Decorative
transitions stop through the `motion-reduce:` variant, while the final colour
state and the target marker always apply. Only motion stops, so the result of
an action remains recognizable.

## 5. Layout verified by people, not machines

Machines check contrast, the absence of raw colours, and component behaviour
(selection, filtering, sending the playback position and so on). **The layout
itself and its variations by screen width are checked by people on real
devices.**

jsdom does not apply CSS, so `position: fixed`, media queries and wrapping are
not resolved. Faking them would lead to the worst state: tests pass while the
screen is broken. Visual regression tests (comparison with reference images)
add dependencies and give false positives from glyph differences between
environments, so every later UI change would also have to update reference
images.

Only **what cannot be checked by machine** goes to people.

## 6. List layout

Lists use a dense layout suited to a management screen: a top navigation bar, a
filter band and boxed cards, in VVMDM's colours and typefaces.

- **Shell**: the top `TopBar` (`web/src/shell/TopBar.tsx`) holds ☰, the logo and
  the import button `Refresh library`, and each screen inserts its own toolbar
  between them (`TopBarPortal`). Navigation and the entry to settings are in
  `Sidebar`. The lower part of the sidebar shows `Settings` and `Sign out` to
  the owner and only `Sign in` to guests (people not signed in). For guests the
  upper part also shrinks to `Library` and `Folders`, and the top bar shows
  neither the refresh button nor import progress
  (specs/016-single-account-auth/ui-design.md, Shell entries, Guest
  degradation).
- **Sidebar**: three states: expanded, rail (icons only) and drawer (narrow
  widths). Collapsing it widens the space available to the grid (card width is
  set by the zoom level).
- **Toolbar**: search, filters, view (library only), zoom and sort. At narrow
  widths, view, zoom and sort move into the `View and sort` group. At phone width
  (below `sm`) cards fit only one per row at any zoom, so the grid is a single
  full-width column and zoom is not shown. The count appears in the row above the
  grid for the library and search results, and in the section heading for the
  direct contents of a folder page.
  There are 9 sort orders: date added, date modified (file), date created (file),
  title, duration, file size, recently played, date favorited and random. Guests
  get 7, without the owner-only recently played and date favorited. Date created
  was added in 033 and placed next to the other two date orders
  (specs/033-video-dates/ui-design.md, Sort and direction). Date favorited was
  added in 035 right after recently played and sorts newest first when chosen. In
  the `View and sort` group below `md`, the orders flow row-first into two radio
  columns: 5 rows for the owner, 4 for guests
  (specs/035-favorites/ui-design.md, Sort and direction).
  The filter popover lists watch status, `Favorites only` (URL `fav=1`) and
  playability, in that order; watch status and favorites only are shown to the
  owner only. `Clear filters` clears favorites only together with the other
  filters, and keeps the sort. If a guest's URL still has `fav=1` or
  `sort=favorited*`, they are reset to the defaults before the request and the
  URL is corrected (specs/035-favorites/ui-design.md, Filter menu, Guest
  degradation).
- **Card** (`web/src/videoList/VideoCard.tsx`): draws the thumbnail, the
  duration at its bottom right, playback progress along the bottom edge, and the
  title. In the library grid and on folder pages, a row with the video's tags is
  added under the title; videos without tags get no row
  (specs/014-video-tags/ui-design.md, Library card). In the library, pressing a
  tag filters by it; on a folder page it goes to the library filtered by that tag
  (`/?tag=<id>`). A tag that comes only from a folder name keeps the same size but
  has no fill, a dashed border and a Folder marker to tell it apart from a tag
  added by hand, and the video page shows no × for it. `Remove tag` in the
  selection bar also offers only tags added by hand
  (specs/017-folder-groups/ui-design.md, Folder-derived tag chip). Search results
  on a folder page add a location row under the title. Only one hover preview
  plays at a time on any screen, and changing the zoom returns to the card that
  was at the top of the screen. The grid (`web/src/videoList/Grid.tsx`) is shared
  by the library and folder pages.
- **Folder card** (`web/src/folders/FolderCard.tsx`): the same width and border as
  a video card, told apart by the folder artwork and a Folder icon in the title.
  The name shows up to two lines, and the full name is available through
  `title`. The path of a media folder is shown to the owner only, favouring its
  end, with the full path in `title`. An empty registered root says that files
  have to be placed there and then imported.
- **Group card** (library only, `web/src/library/GroupCard.tsx`): the library list
  reads `GET /api/library` and mixes folder groups in as items on a par with
  videos. The card has the same box and size as a video card. Its thumbnail frame
  shows the same folder artwork as a folder card
  (`web/src/videoList/FolderArt.tsx`, stacking up to 4 member thumbnails); a
  panel at its bottom right shows the count (`12 videos`) and total duration; a
  bar shows the share of members watched, only while some are in progress; then
  the group name and a row of the members' tags. Pressing it goes to the video
  page of the member to continue (`/videos/{openVideoId}`). Guests see no watch
  status, watched count or bar. Folder pages and their search results still list
  videos one by one (specs/017-folder-groups/ui-design.md, Group card). The
  actions to ungroup, group a folder's direct videos, and turn a group into a tag
  are not on the card; they are in the menu at the right end of the `Videos N`
  heading row on a folder page (owner only,
  `web/src/folders/FolderGroupingMenu.tsx`) and on the group name row of the
  video page. After an action the cached library list is discarded and reloaded
  the next time the library opens (specs/017-folder-groups/ui-design.md, Folder
  grouping menu).
- **Favorite mark** (owner only, `web/src/videoList/FavoriteToggle.tsx`): a heart
  that is both the mark and the toggle sits at the top right of the thumbnail on
  video cards (library, folder pages, search results) and on library group cards.
  It has no fill behind it and no border: a 22px heart inside a 28px hit area,
  with a dark shadow (`drop-shadow-mark`) so it reads on bright artwork. A
  favorite is filled with the pink (`favorite`) that only this mark uses; a
  non-favorite is a white outline only, and the outline heart appears, like the
  selection check, only on pointer hover or focus (always, on devices that cannot
  hover). It is outside the card's link, so pressing it neither opens the card nor
  changes the selection. The mark changes after the response: a video replaces its
  list item in place, and a group is refetched with
  `GET /api/folders/{rootId}/group`. A list filtered to favorites only does not
  drop the item on the spot. In list view it sits in the column right after the
  title, with no fill or shadow behind it, and on is the same `favorite` fill as
  on cards. Guests do not see it (specs/035-favorites/ui-design.md, Mark, Card).
- **List view** (library only): each row shows title, favorite (owner only),
  watched, duration, quality, size and date added. A group row uses the same
  columns: the first member's thumbnail, a Folder marker and the count under the
  title, `3 / 12` in the watched column, and the total duration and size; quality
  is empty (specs/017-folder-groups/ui-design.md, List view row).
- **Selection** (library only, owner only): in the grid, the check appears on
  pointer hover or focus, and always on devices that cannot hover. In list view
  the check is always shown faintly, and during selection every check is shown.
  What a group's check selects and how it is counted is in
  [017 UI design, Pressing and selection](../../specs/017-folder-groups/ui-design.md#pressing-and-selection).
- **Selection bar**: appears fixed at the bottom of the screen as soon as one item
  is selected, without changing the toolbar's position or height. At wide widths,
  `N selected`, `Add tag`, `Remove tag`, `Favorite` and `Visibility` (with 2 or
  more videos, `Bundle as versions`) form one group without separators, followed
  by a separator, `Select all` and clear, all on one line. At `sm` and above, when
  the line does not fit the screen width, the bar does not overflow with `nowrap`;
  it spans the full screen width and first moves everything from the separator on
  to the right end of a second line. If the first line still does not fit,
  `Favorite` and the items after it also move, in the same order, to the second
  line before the separator (decided by measuring the actual width of the count
  and the actions). Below `sm`, the top line holds the count, `Select all` and
  clear, and the bottom line holds the two tag actions, `Favorite` and
  `Visibility`. Where the bottom line does not fit, `Favorite` and the items after
  it move to the right end of the next line, and to the line after that if they
  still do not fit.
  Action names are not shortened so that their meaning is readable on devices
  that cannot hover. The contents of the tag actions are in
  [014 UI design, Selection bar](../../specs/014-video-tags/ui-design.md#selection-bar),
  the visibility menu in
  [016 UI design, Selection bar](../../specs/016-single-account-auth/ui-design.md#selection-bar),
  and the `Favorite` menu and the wrapping rules in
  [035 UI design, Selection bar](../../specs/035-favorites/ui-design.md#selection-bar).
  `Favorite` sends a group chosen through a group card's check or through the
  response to `Select all` as a group, and does not send its members as videos.
  When the list is refetched with the selection kept (after an import finishes,
  for example) and that folder's members appear as video items, or when a group
  gains members so that not all of them are selected, it sends the members as
  videos instead of the group.
- **Interaction states**: normal, hover, focus-visible, active, selected and
  disabled are distinct, and consistent across components. Keyboard focus is an
  outer outline in the accent colour. The search field shows focus on its outer
  frame, not on the inner `input`, to avoid a double outline.

## 7. Sidebar navigation

Each sidebar item goes to its screen.

## 8. Video page layout

The video page (`/videos/:id`) is for watching, so unlike the lists it lowers
density. The detailed shapes and text are in
[specs/012-video-detail-ia/ui-design.md](../../specs/012-video-detail-ia/ui-design.md);
this section records only the layout decisions.

- **Components**: a header band, the player, the title, tags, the visibility
  toggle, file information and related videos. There is no shell; the page is
  layered over the list and closes with × or Esc. The header band belongs only to
  the video page and holds the logo (to home), a breadcrumb to the containing
  folder, and ×. The return target is the list from before the page opened, kept
  even after moving through related videos or `Play next`. Tags sit right below
  the title and form one unit with it (details in
  [specs/014-video-tags/ui-design.md, Video page tags](../../specs/014-video-tags/ui-design.md#video-page-tags)).
  The visibility toggle (`role="switch"`) sits directly below that unit, above
  the file information. Guests see no tags, visibility toggle, `Open file` or
  `Copy path`; below the title there are only the file information and technical
  information rows
  ([specs/016-single-account-auth/ui-design.md, Visibility toggle, Guest degradation](../../specs/016-single-account-auth/ui-design.md#visibility-toggle)).
- **Width variations in CSS** (as in section 4): at `lg` and above, related
  videos form a column on the right; below, everything stacks. × exists only once,
  in the band, and below `md` the breadcrumb collapses to its last segment.
- **Two rows under the title with no lines, frames or labels**: the first row
  holds the file information with icons (duration, size, date added) and, at its
  right end, `Open file` and `Copy path`. The second row holds technical
  information (resolution, container, codec), the smallest and most subdued. The
  location is in the band as the breadcrumb, so no path appears under the title.
- **Favorite first in the right-hand action group** (owner only): at the start of
  the action group at the right end of the information row (left of
  `Use current frame as thumbnail`) sits the favorite toggle for the current
  video (`FavoriteToggle` in its `page` form: `IconButton` `sm`,
  `aria-pressed`). It is the only control in the group with state, so it is where
  the eye lands first when moving right, and it is no more prominent than the
  title or the visibility toggle (on is only a small `bg-accent-soft` fill and the
  same pink `favorite` heart as cards and rows). Pressing it sends
  `PUT /api/favorites`; on success the video is refetched and the fill changes.
  The result is also applied to the cached list, so the card mark in the library
  has changed on return. A failure appears in a single line directly below the
  information row, like open and capture failures, with no toast. It is not on
  group rows, and guests do not see it (details in
  [specs/035-favorites/ui-design.md, Video page](../../specs/035-favorites/ui-design.md#video-page)).
- **States and failures appear inside the player**: loading, import stages, read
  failure, playback failure, video gone and playback ended stack in one container
  over the player. The container decides the stacking order and shows one at a
  time. The only state outside the player is the "being created" line directly
  below it. Text in the layers sits on an opaque `bg-navbar` surface, because the
  contrast of text on the translucent `bg-overlay` cannot be checked by
  `tokens.test.ts`.
- **The stall warning is a separate layer outside the container**: when playback
  is judged to stall on a slow connection, a small notification-only banner
  (`web/src/player/StallWarning.tsx`, `role="status"`) appears at the player's
  top left. The status container is a "one at a time, centred" layer, so a
  warning that neither stops playback nor blocks controls does not go in it. The
  banner is above the video and below the container and the control bar, and
  everything except × passes input through to the controls beneath. It is not
  shown while the playback failure, playback ended, up next, reconnecting or
  importing layers are showing, and it is shown alongside the loading spinner for
  a data wait. Once dismissed it does not reappear for the same video. It offers
  no action to change quality
  ([Playback quality, Stall warning](playback-quality.md#stall-warning); details
  in [specs/027-playback-quality/ui-design.md, Stall warning](../../specs/027-playback-quality/ui-design.md#stall-warning)).
- **Playback errors are classified by kind** (`web/src/player/playbackRecovery.ts`):

  | `video` element error | Classification |
  | --- | --- |
  | 1, 2 | Network failure |
  | 3 | Data cannot be read |
  | 4, or no code | Determined by checking the cause (not checked while the device is offline) |

  For the check, direct playback requests the first byte of the video file: no
  response is a network failure, content means a format problem, and an error
  status means the video cannot be served. For transcoding, a request would start
  a transcode, so only whether `/api/health` is reachable is checked.
  A network failure does not switch to transcoding; it reloads on the current
  route from where it broke off. The waits are 1, 2, 4, 8 and 15 seconds (about
  30 seconds in total); meanwhile a "reconnecting" display shows instead of the
  failure layer, and the wait ends early when the device comes back online
  (`online`). If a reloaded request returns no metadata within 15 seconds, the
  next reload proceeds even without an error. Viewer input during the wait is not
  passed to the broken source. Whether playback is paused is answered from the
  viewer's intent, not the element, and the play/pause button follows it. Play
  ends the wait and reloads; pause keeps playback paused after the reload; a seek
  becomes the reload position (a seek during a request moves to that position
  after metadata). After a reload, when playback actually advances 10 seconds
  (seeks and changes while paused do not count), the retry count resets. Direct
  playback switches to transcoding once, only for data-unreadable and format
  failures. Once retries are exhausted, the failure is reported with separate
  messages: the server cannot be reached, the data cannot be read (corrupt), or
  the video cannot be served (moved, gone, format).
- **Only control bar parts are video.js components**: restart, quality,
  subtitles, playback speed, current time/duration, and the transcode indicator.
  The status display is on the React side, because as a video.js component it
  could not share state with the rest of the screen.
- **Quality on the control bar is a secondary control like playback speed**: the
  right-hand group holds the transcode indicator, quality, playback speed, PiP and
  full screen, in that order. Quality is a video.js `MenuButton` component
  (`web/src/player/qualityMenu.ts`) whose box, padding, opening behaviour and Esc
  handling match playback speed, and its button label shows the current quality
  (for the original quality, the video's short side). Choosing one replaces the
  source at the same position without recreating the player
  ([Playback quality, Switching](playback-quality.md#switching); details in
  [specs/027-playback-quality/ui-design.md, Control bar: quality menu](../../specs/027-playback-quality/ui-design.md#control-bar-quality-menu)).
  Esc in an open menu (quality, playback speed) closes only the menu.
- **Subtitles sit between the video and the control bar**: sidecar subtitles
  appear in video.js's subtitle layer (`vjs-text-track-display`) and, while the
  control bar is visible, move up so that they do not overlap the control bar and
  progress bar. The subtitle button is next to playback speed and is absent for
  videos without subtitles. The menu holds only subtitle names and `Off`, with no
  subtitle appearance settings. The chosen subtitle is remembered per browser,
  and the next video with the same label starts with it on
  (`web/src/player/subtitleTracks.ts`, [sidecar-subtitles.md](sidecar-subtitles.md)).
- **Keyboard shortcuts are received for the whole screen**: video.js control bar
  components stop key propagation, so keys are received in the capture phase on
  `window`, letting through only keys that operate the focused button or slider
  itself. Space, F, M, C (subtitles), 0 and Esc are handled.
- **Group members only add to the same page**: a secondary line above the title
  with the group name and the position in the group; the `Up next` member list
  and a divider at the top of the related videos column; and, instead of the
  playback-ended layer, a notice for the next member (5 seconds, cancellable, Esc
  cancels). For the owner, the group name row opens a menu with `Ungroup` and
  `Turn the group into a tag`; after an action the video and related videos are
  refetched and the page returns to the plain video form. Esc in an open menu
  closes only the menu. The previous/next handles move within the group. The page
  for a video outside any group is unchanged (details in
  [specs/017-folder-groups/ui-design.md, Video page](../../specs/017-folder-groups/ui-design.md#video-page)).
  The `lg` width is read with `matchMedia` only to scroll the current member's
  row into view on wide screens. On narrow widths that would scroll the whole
  page and hide the player, so it does not scroll.
- **Central touch controls appear on `pointer: coarse` devices**: the variation
  uses a CSS media condition, and `matchMedia` is not read (for the same reason as
  section 4).
