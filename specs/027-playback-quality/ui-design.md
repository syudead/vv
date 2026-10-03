# UI design: Quality menu, and a warning when a slow connection interrupts playback

**Feature**: [parent Issue #521](https://github.com/syudead/vv/issues/521) ·
[plan.md](plan.md) · [research.md R-4](research.md#r-4-the-quality-menu-is-a-videojs-menubutton-component) ·
[R-5](research.md#r-5-switching-quality-swaps-the-source-at-the-same-position-without-recreating-the-player) ·
[R-7](research.md#r-7-the-warning-is-a-separate-layer-from-the-status-overlay-container-hidden-while-a-status-layer-shows)

Sources (this document does not re-decide what they define):

| Topic | Source |
| --- | --- |
| Colours, interaction states, width breakpoints, playback screen layout | [Library UI](../../docs/design-docs/library-ui.md) ("8. Video page layout") |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css). Referred to by name; values are not copied. |
| Contrast pairs under test | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| Control bar order, the "Converting for playback" indicator, the layer container | [specs/012-video-detail-ia/ui-design.md "Player"](../012-video-detail-ia/ui-design.md#player) and the current [`web/src/player/VideoPlayer.tsx`](../../web/src/player/VideoPlayer.tsx), [`VideoPage.tsx`](../../web/src/player/VideoPage.tsx), [`StatusOverlays.tsx`](../../web/src/player/StatusOverlays.tsx) |
| Where screen text lives and its format | [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The English in this document is a proposal showing intent; after implementation the catalogue [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth. |

This feature adds exactly three things to the screen, all inside the player on the playback screen
(`/videos/:id`). No other screen changes, and nothing outside the player (title, information, related
videos) changes. Owners and guests see the same (requirement 8 of the parent Issue). No new colour,
radius or shadow token is added, and nothing is added to `pairs` in `tokens.test.ts` ("Colour" below).

1. The **quality menu** in the control bar (requirements 1–8)
2. **Quality-aware text** for the "Converting for playback" indicator (requirement 6)
3. The **stall warning** at the top edge of the player (requirements 9 and 10)

## Words

| Place | Text (proposed) |
| --- | --- |
| Quality menu button accessible name and tooltip | Quality |
| Button text (original quality) | The short side of the video's display (`1080p`, `2160p` and so on). Orig if the short side is unknown |
| Button text (chosen quality) | `720p`, `480p`, `360p`, `1080p` |
| Menu item (original quality) | Original (1080p). Original if the short side is unknown |
| Menu items (available qualities) | 720p / 480p / 360p / 1080p |
| Note line when no quality can be chosen | No smaller sizes for this video |
| "Converting for playback" indicator (original quality; unchanged) | Converting for playback |
| "Converting for playback" indicator (chosen quality) | Converting to 480p |
| Popover explanation (chosen quality) | Playing a 480p version converted while it plays, so it needs less bandwidth. Choose “Original” in the quality menu to go back. Seeking takes a few seconds. |
| Warning text | Slow connection is interrupting playback |
| Accessible name of the button that dismisses the warning | Dismiss |

- Quality names are the short-side number plus `p`, like `1080p`, and are not translated (the same
  treatment as `H.264` in the technical information row).
- The warning text only informs. It does not suggest lowering the quality, point to the menu, or say
  that it will switch automatically (requirement 10; out of scope: "suggesting a quality from the
  warning").

## Control bar: quality menu

### Placement and weight

The control bar keeps the order from 012's "Control bar" and gains quality. From left: back to start,
play, volume, current time / duration, right-aligned gap, "Converting for playback", **quality**,
playback rate, PiP, fullscreen.

- Quality sits immediately left of the playback rate and is a secondary control like the playback rate
  (`UI品質`: visual hierarchy). It is kept less prominent than play, seek and volume through its place
  (in the secondary group on the right) and by giving the button the same box and text colour as the
  playback rate. No accent colour.
- The button is a video.js `MenuButton` (R-4), with the same box as the playback rate button (the
  `.vjs-control` width of `4em`, `3em` in a narrow frame by the `index.css` rules). The hover,
  focus-visible and pressed looks come from the `.vjs-button` rules in `index.css` as they are.
- Sitting right of "Converting for playback" and left of the playback rate, it lets the eye run from
  "how it is playing now (the transcode indicator)" to "the control that changes that (quality)".
  "Converting for playback" is an indicator and quality is a control; the two neighbours' different
  roles are told apart by the existing contrast: one is `fg-muted` text with the `Info` icon, the other
  a button with `fg` text only.

### Button label

- The button text shows the current quality briefly (`UI品質`: show the current quality small enough
  to know it without opening). For a chosen quality it is `480p` and so on; for the original quality it
  is the short side of the video's display (`1080p` for a 1080p video, `2160p` for 4K). For a video
  whose short side is unknown, `Orig`.
- The original quality is shown by its short-side number because that is the fact of "the current
  quality". `1080p` is not an option for a 1080p video (requirement 1), so `1080p` is never read as a
  scaled-down quality, and opening the menu shows "Original (1080p)" selected. Showing `360p` on a video
  of 360p or less also tells why there is no quality to choose (Edge Case 1).
- Text is `text-xs`, `text-fg`, `tabular-nums`. The playback rate's `1x` is drawn at video.js's
  `1.5em`, but quality must fit 5 characters (`1080p`) in a `4em` box, so it uses `text-xs`, the same
  as "Converting for playback". This is the text the Issue means by "show small". The box size,
  spacing and text colour match the playback rate; only the text size differs.
- In a narrow frame (`@container (max-width: 39.99rem)` in `index.css`, a `3em` box) the text becomes
  `text-[0.625rem]` (10px) and `tracking-tight` so that `1080p` fits. The text is not folded into an
  icon: the number is the only clue to the quality, and replacing it with an icon would hide it until
  the menu opens.
- No icon (such as a gear) inside the button. Text only, the same form as the playback rate.

### Menu

- The open menu looks the same as the playback rate menu (video.js `.vjs-menu` rules; `index.css`
  does not override them). Only the width is video.js's popup default of `10em`, so that "Original
  (1080p)" fits on one line (the playback rate's `4em` is not used).
- Items from top: **Original (1080p)** → `720p` → `480p` → `360p` (original first, then descending).
  Only options smaller than the video's display short side are listed (requirement 1, R-3).
- The current quality's item gets video.js's `vjs-selected` mark (as the playback rate does). When the
  remembered quality is unavailable for this video and it plays at "Original" (Edge Case 2), "Original"
  carries the mark. The remembered value is not rewritten, but the menu shows the fact of the current
  playback.
- For a video with no quality to choose (360p or less, or an unknown short side), the single item
  "Original (360p)" carries the mark, and below it is the line "No smaller sizes for this video" in the
  same format as `vjs-menu-title` (`fg-muted`, not pressable, not focusable) (Edge Case 1).
  - Q-5: this line cannot be pressed. Its value is telling someone who opened the menu "this video has
    no smaller quality" rather than "it is broken or loading". The risk of mistaking it for a pressable
    item is kept down because, unlike items, it has no mark and no hover surface, and its colour is
    `fg-muted`.
  - The button itself stays. Hiding it would make the button appear and disappear per video and shift
    the control bar order.
- Choosing an item closes the menu, and the button text changes to the new quality **immediately**.
  While the switch loads, the existing centred loading mark (`LoadingOverlay`, no `backdrop`) shows.
  The playback position and whether it is playing or paused do not change (requirement 4, R-5).
- Switching back to "Original" makes "Converting for playback" disappear for a video that can play
  directly (requirements 2 and 6).

### Interaction

| Input | Behaviour |
| --- | --- |
| Mouse | Opens on hover (video.js default, as for the playback rate) and on press. Click an item to choose it. |
| Touch | The button opens and closes; an item chooses. |
| Keyboard | Tab to the button, Enter/Space to open, ↑↓ to move between items, Enter to choose, Esc to close (video.js default). Esc while the menu is open closes only the menu, not the screen (`rateMenuOpen` in `playerControls.ts` applies to the quality menu too; R-4). |
| Fullscreen | Same place and form. The menu opens upwards inside the player. |

- The button has no keyboard shortcut (no `aria-keyshortcuts`).

## Control bar: transcode indicator

The look of "Converting for playback" (`Info` `size-3.5`, `text-xs text-fg-muted`, a popover opened by
press, folding the text to leave only the icon in a narrow frame with `@max-[22.5rem]:sr-only`) stays
as in 012. Only the text changes (requirement 6).

| Situation | Text |
| --- | --- |
| Transcoding at original quality | Unchanged: "Converting for playback" and the current explanation. |
| Transcoding at a chosen quality | "Converting to 480p". The popover explanation is the quality one in "Words" above (it says the video is scaled down, and that the way back is "Original" in the quality menu). |

- When folded to the icon in a narrow frame, the quality is still readable from the neighbouring
  quality button's text. There are not two kinds of icon.

## Stall warning

When playback is judged to be interrupted by a slow connection (requirement 9, R-6), one small bar
appears at the top edge of the player, in a layer separate from the status display container
(`data-overlay-layer`) (R-7).

### Form

- Placement: the **top left** of the player frame. `absolute left-2 top-2` (`left-3 top-3` from `sm`
  up). Not in the centre: the centre belongs to the loading mark, the touch play control and the
  playback failure surface, and the warning must be more subdued than those (`UI品質`: action
  priority).
- Shape: `flex items-center gap-2 rounded-md bg-navbar px-3 py-1.5 text-xs text-fg shadow-elevated`.
  On the left lucide `WifiLow` (`size-4`, `text-warning`), then the text, and on the right a close ×
  (lucide `X`, `size-3.5`, `text-fg-muted`, `text-fg` on hover, a `size-6` hit area).
  - Text and icon are one step smaller than the loading mark (`text-sm font-medium`, a `size-5`
    `accent` ring). Loading is "what is happening now"; the warning is "a note on why", and the latter
    must not dominate.
  - The icon family matches "Reconnecting" (`WifiOff`), so it reads at a glance as a network matter.
    The colour is `warning`, signalling that it is neither a failure (`danger`) nor a control
    (`accent`).
- Width: the content width (`max-w-[calc(100%-1rem)]`). At 360px it fits on one line; in a language
  where it does not, it wraps to two lines and does not reach the centre mark.
- It enters with `animate-fade-in` (`motion-reduce:animate-none`). It leaves without animation.
- `role="status"`. The text is announced once, when it appears.

### Behaviour

- The bar's container is `pointer-events-none`, and only the × is `pointer-events-auto`. Clicks and
  taps on the progress bar or the video beneath the bar go through outside the × (requirement 9: "does
  not block the controls").
- It **stays visible** while the control bar is hidden (playing with no interaction). Its purpose is
  to inform, not to be operated. Same top-left place in fullscreen.
- Playback does not stop. There is no button, link or automatic switch that changes quality
  (requirement 10).
- After dismissing with ×, it does not show again during playback of that video (including after a
  retry from a failure). After moving to another video, it shows again when the conditions are met
  (requirement 10, Edge Case 9).
- Stacking order: above the video, below the status display container (`z-10`) and the control bar
  (`z-20`).

  | Other element | Relation |
  | --- | --- |
  | Playback failure, ended, up next, reconnecting, importing | The bar does not show while any of these shows. It disappears on ended, failure and moving to another video (Edge Case 9). |
  | Data-wait loading (small centred ring) | Shown side by side. The ring is in the centre and the bar at the top left, so they do not overlap (R-7). |
  | Touch centre play control (`size-15`, centred) | Shown side by side. At 360px width and 16:9 (about 203px high), the bar (about 28px high) and the centre control (top edge about 70px) are apart. |
  | Previous/next video handles (left and right edges, vertically centred, `min-h-18`) | No overlap. At 360px and 16:9 the handles' top edge is about 40px and the bar's bottom edge about 36px. A portrait video frame leaves even more room. |

## Responsive behaviour

Width breakpoints are only Tailwind's defaults and the frame width in `index.css` (`@container`),
switched in CSS (library-ui.md 4). The widths judged are 360px, 768px, 1280px, a portrait (9:16) video
frame at 1280px, and fullscreen.

| Width / frame | Quality button | Menu | "Converting for playback" | Warning |
| --- | --- | --- | --- | --- |
| 1280px, landscape | `4em`, `text-xs` | `10em`, opens upwards | Icon and text | Top left `left-3 top-3`, one line |
| 1280px, portrait (a 9:16 frame sized by the screen height; under 40rem) | `3em`, 10px | Same. Fits inside the frame | Icon only if the frame is 22.5rem or narrower | Top left, one or two lines |
| 768px | `4em`, `text-xs` | Same | Icon and text | Top left, one line |
| 360px (frame about 22.5rem) | `3em`, 10px | Same. Fits within the width up to PiP and fullscreen | Icon only (`@max-[22.5rem]`) | Top left `left-2 top-2`, one line |
| Fullscreen | Same as 1280px | Same | Same | Top left |

- At no width does a horizontal scroll appear or do control bar parts overlap. Adding quality keeps
  the control bar on one line, and the right-hand group (transcode indicator, quality, rate, PiP,
  fullscreen) does not cut into current time / duration (`UI品質`: information density).

## Review criteria

Judge on real devices (library-ui.md 5). "It is there" alone does not satisfy a criterion (Q-4).

- **Visual hierarchy**: looking at the control bar, play, seek and volume catch the eye first, and
  quality looks **the same weight** as the playback rate. The quality button text is no more prominent
  than the playback rate's `1x`. The quality button has no accent colour. The warning is smaller than
  the loading mark and the failure surface, and at the top left of the video it does not hide the
  video's subject.
- **Information density**: at 360px and in a portrait frame, the right-hand group of the control bar
  does not overlap, and current time / duration is not cut off. With "Converting for playback" folded
  to its icon, the quality is readable from the button's number.
- **Spacing rhythm**: the quality button's box, spacing and hover surface match the playback rate, so
  side by side it does not look like a part added later (`UI品質`). The open menu's item height and
  spacing match the playback rate menu.
- **Typography**: `1080p` fits in the quality button without being cut off or spilling into the
  neighbour (at both 4em and 3em). The digits are `tabular-nums`, so switching between `720p` and
  `480p` does not shift the text. The warning text is `text-xs` on one line (up to two at 360px).
- **Action priority**: while the warning shows, seeking on the progress bar, the centre play control
  and everything on the control bar can be pressed as usual. Pressing anything other than the warning's
  × gets no reaction from the warning and passes through. The warning has no control that changes
  quality, and the quality button text does not change when it appears (acceptance criterion 11).
- **State visibility**: right after choosing a quality, the button text becomes the new quality, the
  loading ring appears in the centre, and the position and playing/paused state are kept (acceptance
  criteria 2 and 4). Switching back to "Original" removes "Converting for playback" for a video that
  can play directly (acceptance criterion 6). Opening the menu on a video of 360p or less shows the mark
  on "Original (360p)" and the non-pressable note line beneath it (Edge Case 1).
- **Warning appearance**: when the network is throttled and the conditions are met, the bar appears at
  the top left and playback does not stop. × removes it, and it does not appear again for the same
  video. It appears again when the conditions are met on another video. There is no bar while the
  playback failure, ended or reconnecting surface shows (acceptance criteria 9 and 10).
- **Examples that do not meet the requirement** (`UI品質`): quality exists only in the settings
  screen. The warning is in the centre, dims the video, stops playback, or overlaps the control bar.
  The warning has an action such as "switch to 480p", or switches automatically. The quality button is
  only a gear icon and the current quality cannot be read.

## Colour

No new pair is used. The bar's text is `fg` on `navbar`, the menu uses the same video.js defaults as
the playback rate, and the quality button text is the control bar's `fg`. `warning` is used only for
the `WifiLow` icon, not for text, so it is not added to `pairs` (library-ui.md 1; the same treatment
of icons as 012 "Accessibility").

## Accessibility

The parent Issue does not ask for screen reader or contrast design, so only names are decided (the
same scope as 012).

- Quality button accessible name and `title`: "Quality", built from the catalogue as
  `playerDictionary` does.
- Menu item names are the same as their displayed text ("Original (1080p)", "480p").
- The warning is `role="status"`, and the × has the accessible name "Dismiss".
