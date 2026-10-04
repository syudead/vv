# UI design: Scrub along the bottom of a library card's thumbnail to skim the video

**Feature**: [parent Issue #616](https://github.com/syudead/vv/issues/616) ·
[plan.md](plan.md) ·
[research.md R-2](research.md#r-2-loop-playback-and-the-band) ·
[R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets) ·
[R-5](research.md#r-5-how-frames-fit-and-the-bands-shape) ·
[R-6](research.md#r-6-swapping-the-time-display-and-the-bar)

Sources: the visual rules come from these documents and are not decided again here.

- Colours, interaction states, width breakpoints, library and player layout:
  [Library UI](../../docs/design-docs/library-ui.md) (cards in [List layout](../../docs/design-docs/library-ui.md#list-layout), library layout, and [Video page layout](../../docs/design-docs/library-ui.md#video-page-layout), player screen layout)
- Role tokens: `@theme` in [`web/src/index.css`](../../web/src/index.css). Refer to them by name; do not copy
  values
- Pairs checked for contrast: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- Card hover preview (400ms, one at a time, release triggers, priority versus selection mode, thumbnail and video in
  the same media layer so the hover scaling does not jump, an immediate switch under reduced motion):
  [specs/010-hover-video-preview/ui-design.md](../010-hover-video-preview/ui-design.md) and the current
  [`web/src/videoList/VideoCard.tsx`](../../web/src/videoList/VideoCard.tsx),
  [`cardPreview.tsx`](../../web/src/videoList/cardPreview.tsx) and
  [`web/src/player/RelatedVideos.tsx`](../../web/src/player/RelatedVideos.tsx) (`VideoThumbnail`)
- The player's seek-bar tooltip (frame selection, time format, fetch states):
  [specs/009-seek-thumbnail-preview/ui-design.md](../009-seek-thumbnail-preview/ui-design.md),
  [specs/021-seek-thumbnail-sprite](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)
- The fit that lays a blurred thumbnail left and right of a portrait video:
  [`web/src/ui/ThumbnailBackdrop.tsx`](../../web/src/ui/ThumbnailBackdrop.tsx)
- Where screen text lives and its format: [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md).
  The English in this document shows intent; after implementation the catalog
  [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth

This feature adds three things, all contained within a video's thumbnail surface:

1. An invisible **band** covering the bottom fifth of the thumbnail (requirements 1 and 10)
2. A **frame** shown on the thumbnail surface while the pointer is in the band (requirements 1, 5 and 6)
3. A **time display** and a **scrub-position bar** that switch only while the pointer is in the band
   (requirement 4)

In scope: `VideoCard` in the grid view of the library and the folder screen (including search results), and
`VideoThumbnail` of related videos on the player screen (members of "play next" and related videos; requirement
8). List-view rows, group cards, the row of the member being watched on the player screen, the "next video" prompt
and the player's seek bar do not change. Owners and guests see the same thing. No colour, radius, shadow or text
token is added, and nothing is added to `pairs` in `tokens.test.ts` ("Colour" below).

## Why this shape

- **The band is invisible.** No border, label, icon or marker that appears on hover (`UI品質` "visual
  hierarchy"). Current video services' lists have converged on showing the scene at a position when the pointer
  traces the spot of the progress bar at the bottom of the thumbnail, so users touch that spot without looking for
  it. The only thing that signals the band is that the time and bar start following the pointer the moment it
  enters.
- **The frame is shown on the thumbnail surface itself.** No separate floating box with a border and shadow like
  the player's tooltip (009). The card surface is the primary information (`UI品質` "visual hierarchy"), and a
  tooltip would overlap the neighbouring card and disturb how the grid reads.
- **The time reuses the duration display at the bottom right, and the bar uses the place of the watch-position
  bar** (`UI品質` "information density"). No text or line is added to the surface; only the contents of the two
  existing markers change.
- **The scrub-position bar is `fg`; the watch-position bar stays `accent`.** In
  [Library UI, Visual values in one CSS location](../../docs/design-docs/library-ui.md#visual-values-in-one-css-location-with-contrast-guaranteed-by-tests)
  accent is the colour of "primary actions, selection and the user's state", and the watch-position bar is that
  state, "how far you watched". The scrub position is the pointer's temporary position, not the user's state, so a
  different colour tells them apart. With the same accent, the bar shrinking or disappearing the moment the pointer
  leaves the band would be misread as "the watch position went back".
- **The time and bar follow from the moment the pointer enters the band, ahead of the frame.** While waiting for
  the fetch (`UI品質` "what does not meet the requirement": no spinner or blank), the time and bar show that the
  band is responding, and the frame appears from the pointer position at the moment it arrives.

## Words

| Place | Text (proposal) |
| --- | --- |
| Time display while in the band (`t.list.card.scrubTime(position, duration)`) | `1:23 / 4:56` (`m:ss` and `h:mm:ss` from `formatDuration`) |

- The separator is a space, `/`, and a space, read the same way as the current time / length in the player's
  control bar. Both position and length use `formatDuration`, so digit shapes are not mixed within one video
  (`0:05 / 1:02:30` stays as is; the position is not padded to `0:00:05` when the length exceeds an hour). This is
  the same notation as the player's tooltip.
- No screen-reader text is added while in the band. The band, the frame and the scrub-position bar are all hidden
  from assistive technology ("Accessibility" below).

## Band

### Geometry

- The band is a transparent element at the bottom of the thumbnail surface (the `aspect-video` box), the full width
  of the surface and one fifth of its height (a CSS percentage; JavaScript only reads the band's rectangle to
  compute the position). That is about 25px on a 220px card (`card-0`), 18px on a related video's `w-40` (160px),
  and about 37px on a one-column card on a 360px-wide screen.
- The band's left end maps to the start of the video (0ms) and its right end to the end (the last frame;
  `ceil(durationMs) − 1` from R-1), linearly by horizontal fraction in between. Positions beyond the band's left
  and right ends are clamped to the ends.
- The band sits **outside** the media layer that scales on hover (`group-hover:scale-[1.03]`) (R-5). Its rectangle
  is not affected by the scaling, so the card's left and right edges are exactly the start and the end.
- Stacking order from the bottom: the media layer (thumbnail, loop video, frame) → the time display and the
  watch-position bar → **the band** → the full-surface warning → the selection check. The band is in front of the
  time display and bar so that the pointer over the time display at the bottom right does not count as leaving the
  band (R-5). The band is not a pressable element; clicks reach the `Link` below (requirement 9).
- The cursor over the band stays the `Link`'s `pointer`; it does not change to `col-resize`, `ew-resize` or similar.
  The band is for looking, and the cursor keeps saying that pressing opens the video.
- No band on: videos without `seekThumbnailUrl`, videos whose `durationMs` is 0 or less, videos showing the
  full-surface warning (`unplayableText`), cards in selection mode, list-view rows, group cards, the row of the
  member being watched on the player screen, and the "next video" prompt (requirement 8, Edge Cases).

### Pointer rules

Only `pointerType === "mouse"` reacts (requirement 10). Touch and pen pass through the band and pressing opens the
video as before. Keyboard focus does nothing.

| Pointer event | Behaviour |
| --- | --- |
| Enters the band | The time display and scrub-position bar appear immediately at the pointer's horizontal position; loop playback stops if running (R-2); the 400ms wait stops if running |
| Moves within the band | Time, bar and frame update together on the same `pointermove`. The frame does not lag behind and catch up (`UI品質` "visible delay between moving the pointer and the frame catching up") |
| Leaves the band upwards and stays on the card | The time display and bar return to normal and the frame disappears. The stopped loop resumes from that scene; a wait in progress counts 400ms again (requirement 3) |
| Leaves the card | Returns as above; the loop is released by the current rule. Fetches in progress are aborted |
| Another card's preview starts while in the band, or sorting, filtering, adding a page, switching view, changing screen width, navigating or entering selection mode happens | Returns as when leaving the band (Edge Cases) |

## Frame

- The frame is one element in the same media layer as the thumbnail, **in front of** the thumbnail and the loop
  video, fitted to the same frame as the thumbnail's `object-contain` (R-5). A landscape video fills the surface; a
  portrait or near-square video is centred at the surface's height, with the current `ThumbnailBackdrop` (blurred
  thumbnail) still visible on the left and right (requirement 6). For a video with no thumbnail (showing the
  fallback), the frame appears in the same frame over the fallback, and the left and right keep that surface's
  current colour (`navbar` on cards, `surface` on related videos' `VideoThumbnail`).
- The frame is shown by scaling one 160px-long-side frame up to the frame box. Blur from scaling is accepted; no
  blur correction, sharpening, border, rounded corners or shadow is added (out of scope: "revisiting resolution or
  frame count").
- Every switch (thumbnail → frame, frame → frame, frame → thumbnail) is immediate, with no fade, scaling or position
  transition. The same under reduced motion (Edge Cases).
- While waiting for the fetch, the frame element is not rendered and the thumbnail stays visible (or, if the band
  was entered during loop playback, the frame at the moment it stopped). No spinner, blank, dimmed surface or
  "loading" text. The frame appears from the pointer position at the moment the fetch finishes (R-3).
- When the layout or sheet fetch fails, or the sprite is still being generated or failed: the thumbnail stays
  unchanged, with no error text, toast or marker. The time display and bar still switch while in the band
  regardless of the failure (requirement 4 is conditioned on being in the band, not on a frame being present; the
  band does not look broken).
- While a frame is shown, the markers on the surface (the public marker and time display, the bar, the selection
  check) stay in front in the same places and shapes. As with 010's "while previewing", they are readable regardless
  of the frame's brightness because the time display sits on a `navbar/90` surface with `backdrop-blur`. The
  surface is not changed for the frame.

## Time and bar

### Time

- While in the band, only the text of the duration display at the bottom right switches to `scrubTime`. The box
  (`rounded-sm`, `bg-navbar/90`, `px-1.5 py-0.5`, `text-[11px] font-medium tabular-nums`; `bg-overlay`, `px-1`,
  `text-xs` on related videos) and its bottom-right position (`right-2 bottom-2`; `right-1 bottom-1` on related
  videos) do not change (`UI品質` "typography"). On owner cards with the public marker, the order of marker and
  time stays the same.
- The display stays anchored at the right and grows to the left (`absolute right-*`). The card's height and the
  box's height do not change. `formatDuration` has no upper limit on the hour digits, so the display has no length
  limit either. "Review criteria" below confirms it fits, including `10:00:00 / 10:00:00` (19 characters) for a
  video over 10 hours, on both the `card-0` (220px) surface and a related video's `w-40` surface.
- A video without a duration (`durationMs` unknown) has no band, so `scrubTime` never appears without a length.

### Scrub position bar

- While in the band, a bar filled with `bg-fg` from the left edge to the pointer's horizontal position appears in
  the same place, at the same height (`h-[5px]` on cards, `h-[3px]` on related videos) and on the same track
  (`bg-navbar/90` on cards, `bg-fg-subtle/50` on related videos) as the watch-position bar. No knob, vertical line
  or number is added.
- The watch-position bar (`role="progressbar"`) is not visible while in the band. The element is kept and its value
  unchanged; only its appearance is hidden (`opacity-0`; `invisible` and `hidden` are not used because they also
  remove it from assistive technology. R-6: keep the announcement of the fraction watched, and do not rewrite the
  value with the pointer). On leaving the band, the original watch position returns in the same place.
- An unwatched video also shows the scrub-position bar while in the band, and it disappears on leaving
  (requirement 4).
- The bar's length follows the pointer immediately, with no `transition`. It also returns immediately on leaving
  the band.
- The bar and the frame point at the same horizontal position. At the band's right end the bar reaches the
  surface's right edge, and the frame is the last frame (not an empty cell) (Edge Cases).

## Colour

- The tokens used are `fg` (scrub-position bar), `navbar`, `overlay` and `fg-subtle` (the current track and time
  surface), and `accent` (watch-position bar, unchanged). No new token is added.
- Nothing is added to `pairs` in `tokens.test.ts`. The only text added is inside the time display, and `fg` on
  `navbar` is already a pair. The bar is not text; it is a line read by thickness (5px / 3px) and position, so it is
  not a contrast pair.

## Responsive behaviour

- There is no width-specific variant. The band's height is a fraction (one fifth) of the surface, so it follows the
  card width (zoom levels `card-0` to `card-3`) and the screen width.
- At 360px (one column, the card near the screen width), 768px and 1280px grids, and the related-video column at
  768px and 1280px, confirm that the time display fits within the surface, the frame matches the surface's frame,
  and card height, grid wrapping, related-video row height and column width do not change when entering and
  leaving the band ("Review criteria" below).
- A change of screen width returns the card to the state of leaving the band (the same trigger as 010's reset).

## Motion

- Every switch of frame, time and bar is immediate with no transition. Reduced motion does not change this
  ([Library UI, Width breakpoints in CSS](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception) is the rule
  that stops decorative motion, and there is no motion to stop here).
- The card's hover lift and the media layer's scaling stay as they are. Entering the band does not change the
  amount of scaling.

## Accessibility

- This section adds no new design; it only applies the rules that R-6 and the plan's Canonical definitions inherit
  from 010, and requirement 10.
- The band, frame and scrub-position bar are `aria-hidden`, and the link's accessible name (the title; the title
  and length on related videos) does not change. The swapped time display is not announced either (010:
  "temporary pointer-only visual information does not interrupt the screen reader").
- The band adds no tab stop, button or link. Keyboard users have only the current link and check actions.
- The parent Issue does not ask for assistive-technology design for the band, so no further design (announcements,
  ARIA, contrast) is written here.

## System states

| State | What the surface shows | Time / bar |
| --- | --- | --- |
| Outside the band (idle, 400ms wait, loop playing) | As now (010) | Duration / watch position (none if unwatched) |
| Just entered the band, waiting for the fetch | The thumbnail, or the stopped loop's frame. No marker or blank | `position / length` / scrub position (`fg`) |
| In the band, frame available | The frame at the pointer position. Portrait videos centred over the blurred background | Same |
| In the band, fetch failed, sprite being generated or failed | The thumbnail unchanged. No error marker | Same |
| Left the band upwards, still on the card | The thumbnail, or the loop resuming (from the stopped scene) | Duration / watch position |
| Left the card | As now (released) | Duration / watch position |
| Touch, pen, keyboard focus | As now | Duration / watch position |
| Selection mode, list-view row, group card, member being watched | No band. As now | As now |

## Review criteria

The implementation PR keeps images at 360px, 768px and 1280px (768px and 1280px for related videos), judged on the
following.

1. **Visual hierarchy**: on a card with the pointer in the band, the eye goes first to the frame, then the time at
   the bottom right, and last the bar at the bottom edge. There is no line, border, icon or colour change marking
   the band; side by side with the outside-the-band state, the only difference around the surface is "the frame has
   replaced the thumbnail". The neighbouring cards are unchanged.
2. **Information density**: the number of markers on the surface while in the band equals the number outside it
   (the public marker and time box, the bar, the check while hovered). Only the time box's content changes, from
   `3:45` to `1:20 / 3:45`, with the same box position and height. `h:mm:ss / h:mm:ss` for a video over an hour and
   `10:00:00 / 10:00:00` for a video over 10 hours fit on one line in the bottom-right box on both the `card-0`
   surface and a related video's `w-40` surface, without overlapping the public marker.
3. **Spacing rhythm**: across the four images (before entering the band, waiting for the fetch, frame shown, after
   leaving the band), the card's rectangle, grid wrapping, related-video row height and column width, and scroll
   position match. A portrait video's frame has the same height and the same centred position as that video's
   thumbnail, and the blurred background on the left and right looks the same.
4. **Typography**: the time text has the same size, weight and tabular digits as the duration outside the band,
   with one space on each side of `/`. The same holds for related videos. Text size and colour do not change only
   while in the band.
5. **Action priority**: the cursor over the band stays the link's `pointer`, and pressing opens the player screen.
   The check shown on hover is not over the band, and the band does not act over the check. Entering the band
   during loop playback stops the video and switches to the frame; leaving the band upwards resumes from the stopped
   scene (not from the start of the video).
6. **Bar**: the bar while in the band is `fg`, and the `accent` watch-position bar is not visible. At the band's
   right end the bar reaches the surface's right edge, and the frame is not a black empty cell. An unwatched video
   also has the bar while in the band. On leaving the band, a partly watched video shows its `accent` watch position
   in the same place again, and an unwatched video has no bar.
7. **Frame match**: for the same video, the frame at a given fraction of the band and the tooltip frame at the same
   fraction of the player screen's seek bar show the same scene (acceptance criterion 3).
8. **Waiting and failure**: neither the waiting image nor the failure image has a spinner, blank surface, dimmed
   surface or text; only the time and bar are laid over the thumbnail (or the stopped loop's frame).
