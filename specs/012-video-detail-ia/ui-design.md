# UI Design: information architecture of the video detail page

**Feature**: [parent Issue #171](https://github.com/syudead/vv/issues/171)

Visual rules follow [Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md)
and the `@theme` of [`web/src/index.css`](../../web/src/index.css). This document defines only what
the video detail page (`/videos/:id`) **adds** to them and what it **changes**.

- No new color, radius or shadow tokens.
- Components are the existing ones in `web/src/ui/` (`Button`, `IconButton`, `Skeleton`, `Tooltip`,
  `Popover`).

## Screen boundary

- The playback page stays outside `AppShell`, as today (ARCHITECTURE.md "Web layer"). It has no
  list top bar (☰, refresh). Instead it has a header band of its own ("Video header"). Closing
  the page layered over the list with × does not change.
- The page consists of the header band, the player, the title and tags, the file facts ("Video
  facts") and related videos (Requirement 1 of the parent Issue).
- The page does not draw these (Requirements 4 and 5):
  - Chips
  - A summary line below the title
  - Large action buttons near the title ("Open file" and "Copy path" stay as small icons at the
    right end of the facts row)
  - Tabs
  - A details section in a frame or card
- These existing components are replaced:
  - The old `VideoHeader` and `FileDetails`: replaced by the title and the facts row. The current
    `VideoHeader` is the header band, a different component.
  - The red band below the player and `Blocked`: replaced by the state display inside the player.

## Layout and responsive behaviour

Only Tailwind's default breakpoints are used, and the layout switches with CSS alone (library-ui.md
4).

### `lg` (1024px) and above: 2 columns

- The header band (height `h-navbar`) sits at the top of the page, with 2 columns below it.
- The column area has `px-6` on the sides and `pt-6` on top. At the bottom, the left column has
  `pb-6` and the related videos list has `pb-6`. The column gap is `gap-6`.
- The left column (`minmax(0, 1fr)`) holds, from top to bottom:
  - The player
  - The creating line (when present)
  - The title and tags
  - The file facts row and the technical details row
- The right column is `w-80` at `lg` and `w-96` at `xl` and above. It holds the related videos
  heading and the vertical list of related videos.
- The top of the right column aligns with the top of the player.
- The player is `rounded-lg`, `overflow-hidden`, `bg-navbar`. Its width is capped so that a 16:9
  video fits the viewport height (a video wider than 16:9 fits at its own ratio):
  - The cap is the existing `max-w-[calc(max(100dvh-17rem,15rem)*16/9)]`. At that height, the
    band (`3.25rem`), the column padding, the title and tags and the 2 facts rows fit in the left
    column, so it normally does not scroll. Below `lg` the whole page scrolls, so the value stays
    `12.25rem`.
  - The height term never goes below `15rem`, so the player does not vanish in a short window. The
    overflow then scrolls in the left column. The bottom padding of the left column is `pb-6`.
  - A player narrower than the column because of the cap aligns left with the title (`lg:ml-0`).
  - The height follows the video's aspect ratio (16:9 while the resolution is unknown; extreme
    ratios are clamped to 9:16 to 21:9) and grows up to the height that fits the viewport.
  - A portrait video is centered in the frame with side margins. The neighbor arrows and the
    control bar keep the same position and width as for a landscape video.
- The gap between left-column items is `gap-5` (the "spacing rhythm" requirement). No divider lines
  or frames.

### Below `lg`: stacked

- The header band stays at the top of the page (`sticky top-0`). × and the logo stay reachable while
  scrolling.
- The player sits edge to edge below the band (`rounded-none`, no side margins). × is not
  overlaid on the player.
- Below it, the body uses `px-4` (`px-6` at `sm` and above) in this order:
  - The creating line
  - The title and tags
  - The file facts row and the technical details row
  - Related videos
- No horizontal scroll at 360px or 768px (Acceptance criterion 2).

## Hierarchy and typography

The order of emphasis is player > title > related videos > file facts > technical details (the
"visual hierarchy" requirement).

| Element | Style |
| --- | --- |
| Title | `h1`. `text-xl` (`text-2xl` at `sm` and above), `font-semibold`, `text-fg`, `leading-snug`. Wraps even a file name with no break points (`[overflow-wrap:anywhere]`). The largest text on the page |
| File facts | `text-sm`, `text-fg`, `tabular-nums`. A `size-4` `text-fg-subtle` icon before each value. No visible text labels |
| Technical details | `text-xs`, `uppercase`, `tracking-wider`, `text-fg-muted`, `tabular-nums`. `lang="en"` because the values are English. Codecs and formats in capitals (`H.264`, `AAC`, `MKV`) |
| Breadcrumbs | `text-sm`. Intermediate levels `text-fg-muted`; the last level (the containing folder) `font-medium`, `text-fg` |
| Related videos heading | `h2`. `text-sm`, `font-semibold`, `text-fg` |
| Related video title | `text-sm`, `font-medium`, `text-fg`, truncated at 2 lines (`line-clamp-2`) |

## Video facts

Below the title and tags sit these 2 rows (`gap-3`), with no divider, frame or label.

- **File facts row**: a `ul` (accessible name "File details") in this order:
  - Length (lucide `Clock`). Omitted entirely while the length is unknown (before probing, for
    example).
  - Size (`HardDrive`).
  - Date added (`CalendarPlus`). Date only (`2026/09/20`).
  - Each item is an icon and value pair (`gap-1.5`). Items are separated by `gap-x-4` (`gap-x-5` at
    `sm` and above) and wrap when they do not fit.
  - Icons are `aria-hidden`. A visually hidden "Length", "Size" or "Added" precedes each value
    instead, and `title` carries the same name.
  - The last-watched time is not shown (the requester's decision).
- **Actions**: 2 `IconButton`s (`size="sm"`) at the right end of the facts row (`text-fg-muted`,
  `text-fg` on hover).
  - "Open file" (`ExternalLink`): present only when the file can be opened (`location.openable`).
    Pressing it sends `POST /api/videos/{id}/open`.
  - "Copy path" (`Copy`): writes the absolute path of the location to the clipboard and shows a
    toast "Copied the path" ("Couldn't copy the path" on failure).
    `navigator.clipboard` exists only on secure connections (HTTPS, localhost). On a LAN address,
    or when the write is refused, it falls back to selecting a hidden input and copying.
  - Neither is present when there is no location (`location`).
- **Technical details row**: a `ul` (accessible name "Technical details") listing only values, in
  the order resolution, container, video, audio.
  - Items are separated by a `border-l border-border-strong` vertical line and `px-2.5`.
  - Every item gets the line and padding. The whole list shifts left by that width and the outer
    element (`overflow-hidden`) clips it, so no line or padding remains at the start of a wrapped
    line.
  - Unknown values (no audio, for example) are omitted entirely.

  | Situation | Technical details row |
  | --- | --- |
  | Before probing | "Reading technical details…" (`text-fg-muted`) |
  | Probe failed | "Couldn't read the technical details" (`text-warning`) |

- When opening fails (409 `file_missing` and so on), 1 line appears directly below the facts row
  (above the technical details row).
  - `text-sm text-danger`, led by `AlertCircle` (`size-4`), `role="alert"`.
  - The text is "Couldn't open the file: The video file is missing." for `file_missing`, and
    "Couldn't open the file" otherwise.
  - It clears on the next open action or when moving to another video.
  - No band, toast or dialog (the "changes that do not meet the requirement" requirement).

## Related videos

- The heading is the `h2` "Related videos". No × here (it is in the header band).
- With 0 related videos, neither the `h2` nor the list appears (Edge case).
- Each item is 1 link, and the whole row is clickable.
  - A thumbnail on the left (`w-40`, 16:9, `rounded-md`, `overflow-hidden`, `bg-surface`) and the
    title on the right (`gap-3`).
  - Without a thumbnail, the same fallback as `VideoCard`.
  - The length sits at the bottom-right of the thumbnail (`bg-overlay`, `text-xs`, `text-fg`,
    `tabular-nums`, `rounded-sm`, `px-1`).
  - Only partly watched videos get an `accent` progress bar at the bottom edge of the thumbnail
    (3px high, ratio `watchedRatio`) (Requirement 15).
  - No date added, "Up next" or reason text (Requirement 15).
- Items are separated by `gap-3`. On hover the row gets `bg-hover-wash` (`p-1.5 -m-1.5` and
  `rounded-lg` outside the row). Keyboard focus shows the global `:focus-visible` outline.
- On hover, the list preview plays by the same rule as a list card (the hover preview in
  library-ui.md). It starts after 400ms, muted and looping, and stops when the pointer leaves.
  It does not play for touch or pen, or for videos whose preview is not ready.
- At `lg` and above, the page is fixed to the viewport height (`h-dvh`; the page itself does not
  scroll), and the left and right columns each scroll internally. The right column keeps its
  heading row on top, and only the related videos list moves. This avoids nested scrolling of the
  page and the columns. Below `lg`, the whole page scrolls as one.
- Permanent scrollbars on every column are noisy, so the page does the following
  (`web/src/index.css`). Both areas can always scroll.
  - The left column uses `scrollbar-none` and draws no scrollbar. The player height cap normally
    keeps it from overflowing. When a wrapped title or the creating line overflows slightly, a
    mouse over the video does not keep a bar visible.
  - The related videos list uses `scrollbar-on-hover`: a thin bar, normally transparent, visible
    only while the pointer is over the list or it has focus.
- States:
  - **Loading**: 6 skeleton rows (a thumbnail and 2 lines of `Skeleton`).
  - **Load failed**: below the heading, `text-sm text-fg-muted` "Couldn't load related videos" and a
    `ghost` "Retry".

## Video header

- A band at the top of the page with the same height as the list top bar (`h-navbar`),
  `border-b border-border`, `bg-bg/90`, `backdrop-blur-md`. It stays on top with `sticky top-0` at
  every width.
- From the left:
  - The logo (`●` and "vv"): a link to `/` (home). Accessible name "Home".
  - Breadcrumbs (`nav`, accessible name "Folder"): the levels from the media folder's display name to
    the folder holding the video, separated by `ChevronRight` (`size-3.5`, `text-fg-subtle`).
    - Every level links to its folder page (`/folders/{rootId}/…`). The current page is a video, so
      the last level is a link too.
    - The values come from `folder` (`rootId`, `path`, `rootName`) of `GET /api/videos/{id}`.
      Without `folder` or `rootName` (right after the folder is removed, for example), the
      breadcrumbs are not shown at all.
    - Narrower than `md`, only the last level remains and the intermediate levels collapse to
      "…". The switch uses CSS alone (library-ui.md 4).
    - The video title is not in the band (it would duplicate the `h1` just below).
  - × at the right end (lucide `X`, `size-5`): an `IconButton` with accessible name "Close".
- × exists only in this band. It is not on the player or on the related videos heading.
- Playback page toasts (the "Copy path" result, ingest notifications) briefly overlay the top center
  of the band and can hide the breadcrumbs. They leave `4rem` on each side, so they never cover ×.
- × returns to the list in `state.from`. Esc does the same.

## Player

### Control bar

The control bar keeps the existing look of `.vv-video-player` (`index.css`) and changes only the
order (Requirement 6).

- From the left:
  - Restart (lucide `RotateCcw`. Sets the position to 0 and keeps the play/pause state. Key 0)
  - Play/pause
  - Volume (mute)
  - Current time / duration
  - Flexible space
  - "Converting for playback" (only while playing through conversion)
  - Playback speed
  - Picture-in-picture
  - Fullscreen
- No skip buttons (10 seconds back/forward) and no skip keys (←/→).
- The previous and next videos are not in the control bar; they sit at the left and right edges of
  the player ("Neighbor arrows" below).
- The progress bar stays above the control bar, as today, and keeps the seek thumbnails.
- The remaining time is not shown (Requirement 5).
- The volume and mute state are saved in the browser and carried to the next video. When the saved
  value is corrupt or browser storage is unavailable, playback uses 100% volume, unmuted.
- Playback speed options are 0.5, 0.75, 1, 1.25, 1.5 and 2.
- Buttons with a keyboard shortcut get `aria-keyshortcuts` and show the key in their hover
  description ("Pause (Space)", for example).
- "Converting for playback" appears as follows:
  - lucide `Info` (`size-3.5`) and text in `text-xs text-fg-muted`.
  - It is the trigger (`button`) of `web/src/ui/Popover.tsx`. Click, tap or Enter opens a small
    bubble with the reason "The browser can't play this format directly, so it's converted while it
    plays. Seeking takes a few seconds."
    - It is not a hover-only tooltip, because touch and keyboard users could not reach the reason
      (Requirement 10).
  - It looks quieter than the other control bar buttons (no border or fill; text color
    `fg-muted`).

### Neighbor arrows

- Tabs flush with the left and right edges of the player move to the neighboring videos. The left
  one (lucide `ChevronLeft`) goes to the previous video in natural order in the same folder; the
  right one (`ChevronRight`) goes to the next.
- They are narrow so they do not hide the picture, and tall so they are easy to press.
  - `sm` and above: width `w-8`, height 50% of the player (minimum `min-h-24`).
  - Below `sm`: width `w-7`, height 55% of the player (minimum `min-h-18`).
  - Vertically centered on the picture area, excluding the control bar (about 3rem).
  - Fill `bg-overlay`, `bg-bg` on hover. No corner radius on the edge side; `rounded-lg` on the inner
    side only. No edge shading (gradient).
- They show and hide with the control bar, and also stay visible while playback has ended. While
  hidden they cannot be pressed; they appear only when keyboard focus reaches them.
- A side whose `prevId` or `nextId` is missing from the related videos response is not shown (start
  or end of the folder, related videos loading or failed).
- On hover, a tooltip inside the tab shows "Previous video: {title}" or "Next video: {title}". The
  accessible name is the same. When the target is not in the related videos list and its title is
  unknown, only "Previous video" or "Next video".
- Moving while playing or after playback ended starts playback at the target. Moving while paused
  does not. The return target (`state.from`) carries over.

### Touch controls

- Only on coarse-pointer devices (`pointer: coarse`), 1 round play/pause button (`size-15`) sits in
  the center of the player (Requirement 8). No skip buttons.
  - Fill `bg-overlay`, icon `text-fg`.
- It shows and hides with the control bar. On these devices, the video.js big play button is not
  shown.
- It is not shown on mouse devices. The switch uses only the CSS `pointer` media condition.

### Overlay layer

Layers stack inside 1 container over the player. From the top they are as follows, and only 1 is
visible at a time.

1. State display
2. Playback ended
3. Center touch control

While the state display or the ended layer is visible, the center control is hidden.

Every state display stays inside the player area (Requirement 10).

- Only the creating line (directly below the player) sits outside the area.
- When the content does not fit the video's aspect ratio (the stage display at 360px, for example),
  the area grows to the content height. The video's aspect ratio stays as the minimum.
- The playback failed and ended layers stack **below** the video.js control bar. Only the picture
  is dimmed; the control bar stays visible and usable.
- Text in a layer never sits directly on `bg-overlay`. `bg-overlay` is translucent, and over a
  bright last frame `fg-muted` or `accent` text lacks contrast. Text blocks sit on an opaque
  surface: `bg-navbar`, `rounded-lg`, `p-5`.

| State | Appearance |
| --- | --- |
| Loading | The thumbnail as background when available, with `LoaderCircle` (`size-7`, `text-accent`, spinning) in the center. A visually hidden "Loading" is read through `role="status"`. The control bar stays hidden until loading ends |
| Converting for playback | See "Control bar" above. No overlay layer |
| Playback failed | `bg-overlay` over the picture. A centered `bg-navbar` surface (`max-w-md`) stacks `AlertCircle` (`size-8`, `text-danger`), the `h2` "Couldn't play this video" (`text-lg font-semibold`), the reason (`text-sm text-fg-muted`) and a `secondary` "Try again from {m:ss}" (`RotateCcw`). `role="alert"` |
| Ingesting (before probing) | "Processing stages" below |
| Probe failed | Centered on a `bg-navbar` surface (`max-w-lg`), stacked: `AlertTriangle` (`size-8`, `text-warning`), the `h2` "Couldn't read this video", the description "The file may be damaged or only partly written." (`text-sm text-fg-muted`), the raw reason, and the actions. The raw reason uses `bg-field`, `border border-border`, `rounded-md`, `font-mono text-xs text-fg`, `break-all`, and scrolls internally beyond 3 lines. The actions are a `secondary` "Read again" (`RefreshCw`) and, only when the file can be opened, a `secondary` "Open file" (`FolderOpen`). While the request is in flight the button is disabled and its icon becomes a spinning `LoaderCircle`. On 202 or 409 `probe_not_failed`, the page refetches the video and switches to the stage display (a 409 means another tab or a repeated press already started the retry). On any other failure, `text-sm text-danger` "Couldn't start reading the video again" appears below the button, and the button becomes pressable again. `role="alert"` |
| Video gone (404) | Centered on a `bg-navbar` surface: `AlertCircle` (`text-fg-muted`), the `h2` "This video can't be opened", and "It was removed from the library, or its file is gone." `role="alert"`, so it is announced when a refetch or playback switches to it. Going back is left to ×; no button is added |

### Processing stages

The surface shown while ingesting (`probeState=pending`) (Requirement 12).

- The surface is `bg-navbar`, with a left-aligned block (`max-w-sm`) in the center.
- The block contains, from top to bottom:
  - The `h2` "Getting ready to play" (`text-lg font-semibold`)
  - The description "Reading the video's information. When that's done, you can play it right
    here." (`text-sm text-fg-muted`)
  - The `ol` of stages
  - The note "Playback starts once “Reading video information” is done. The rest is created during
    playback." (`text-xs text-fg-muted`)
- The stages are these 5, in a fixed order:
  - Finding the file
  - Reading video information
  - Thumbnail
  - Seek preview
  - List preview
- Each row has an icon, a name (`text-sm`) and a state text at the right end (`text-xs`).
- The "Finding the file" row is always "Done": the existence of the video row means detection is
  complete. The other 4 rows derive from `probeState`, `thumbnailState`, `seekThumbnailState` and
  `previewState` respectively.

| Row state | Icon | Name | State text |
| --- | --- | --- | --- |
| Done | `CircleCheck` (`text-success`) | `text-fg` | "Done" `text-fg-muted` |
| In progress | `LoaderCircle` (`text-accent`, spinning) | `text-fg font-medium` | "In progress" `text-accent` |
| Waiting | `Circle` (`text-fg-muted`) | `text-fg-muted` | "Waiting" `text-fg-muted` |
| Couldn't create | `AlertCircle` (`text-warning`) | `text-fg-muted` | "Couldn't create" `text-fg-muted` |

- Rows are separated by `gap-3`; the icon and name by `gap-2.5`.
- Below `sm`, the description and note are hidden and the row gap tightens to `gap-1.5`.
- The block is `role="status"` (`aria-live="polite"`), so it is read only when a stage advances. A
  refetch every 2 seconds that changes nothing does not trigger a reread.

### Creating line

Only when probing has finished and something is still being created, 1 line appears directly below
the player (Requirement 11).

- "Being created" means any of the thumbnail, seek preview or list preview is `pending`.
- Style: `flex items-center gap-2`, `text-xs text-fg-muted`, led by `LoaderCircle` (`size-3.5`,
  `text-accent`, spinning).
- The text is "Creating {items} · You can play the video now". The items are the stage names above,
  joined with "and" (for example, "Creating the seek preview and the list preview · You can play the
  video now"). No counts or percentages.
- `role="status"`. The whole line disappears when nothing is `pending` (even if only `failed`
  remains).

### Ended

When playback reaches the end, this layer appears (Requirement 14).

- `bg-overlay` over the picture. The control bar stays visible above it (it also serves to watch
  again).
- **When there is a next video**, a centered `bg-navbar` surface (`max-w-lg`) stacks:
  - The label "Next video" (`text-xs font-semibold text-accent`)
  - A link to the next video: the thumbnail (`w-56`, 16:9, `rounded-md`, with the length badge) and
    the title (`text-base font-semibold`, truncated at 2 lines) side by side
  - Actions: a `primary` "Play next" (`Play`) and a `secondary` "Watch again" (`RotateCcw`)
  - Below `sm`, no thumbnail; only the title and the actions.
- **When there is no next video**, only a `secondary` "Watch again" sits in the center, with no
  label.
- Nothing plays automatically, and there is no countdown.
- When the layer appears, a visually hidden "Playback finished" is read through `role="status"`.
  - Focus moves to "Play next" (or "Watch again" when absent) only when focus was inside the player.
  - Focus of someone outside the player (in related videos, for example) is not taken.

## Interaction details

- **Keyboard**: no ←/→ skipping.
  - Esc while the playback speed menu is open only closes the menu.
  - Esc in fullscreen only exits fullscreen.
- **Tab order**: DOM order, as follows.
  - At every width: header band (logo → breadcrumbs → ×) → player (center control → controls in the
    state display or ended layer → control bar) → tags → facts row actions → each related video.
- **Reduced motion** (`prefers-reduced-motion: reduce`): stops the spin (`LoaderCircle`) and the
  `animate-fade-in` of layers entering and leaving. The appearance of states does not change
  (library-ui.md 4).

## Accessibility

- Elements with an accessible name:
  - "Close" on ×
  - "Play" / "Pause" on the center control
  - "Restart" in the control bar
  - "Previous video: {title}" and "Next video: {title}" at the edges
  - "Home" on the logo, "Folder" on the breadcrumbs `nav`
  - "Open file" and "Copy path" in the facts row
  - "File details" and "Technical details" on the `ul`s of the file facts and technical details
- A visually hidden name ("Length" and so on) precedes each file facts value. The technical details
  row has `lang="en"`.
- Fixed announcement roles:
  - Read as status (`status`): the stage display, the creating line, loading, playback ended
  - Read as alert (`alert`): playback failed, probe failed, video gone (404), the open-failed line
- The title is the `h1`, and `document.title` stays "{title} - vv", as today.
- Color and contrast pairs:
  - The only new pair is `fg-muted` on `navbar` (descriptions in the stage display, probe failed
    and playback failed). The implementation PR adds it to `pairs` in `tokens.test.ts`.
  - All text in layers sits on the opaque `bg-navbar` ("Overlay layer"). Text on the translucent
    `bg-overlay` cannot be checked by `pairs`, so no text goes there.
  - The other pairs (`fg` and `fg-muted` on `bg`, `warning` on `bg`, `fg`, `accent` and `success`
    on `navbar`) are already listed.
  - Colors used only for icons (`danger` or `warning` on `navbar`, and so on) are not text, so they
    are not added as pairs.
