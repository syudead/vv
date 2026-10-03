# UI Design: Thumbnail preview while seeking

**Feature**: [parent Issue #117](https://github.com/syudead/vv/issues/117)

Sources: the existing player screen, the Video.js control structure and the role
tokens follow [Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md)
and [`web/src/index.css`](../../web/src/index.css). This document defines only
the display and interaction the seek preview adds. The page layout of the list
screen and the player screen does not change.

## Screen boundary and hierarchy

The preview is a transient helper that belongs to the progress bar inside
`VideoPlayer`. It does not carry state out to the page body, the video
information or the list cards. The video surface stays the largest area on the
screen; the preview overlays the lower part of the video surface only while the
pointer is over the progress bar or while a pointer or touch drag is in
progress.

The preview has exactly three elements:

1. The still image for the target time (in the video's aspect ratio; 16:9 when
   the ratio is unknown)
2. The target time as `h:mm:ss` or `m:ss`
3. The existing mouse display on the progress bar, which shows which position
   the preview belongs to

No title, loading text, spinner, button or explanation outside the frame is
added. So that the preview is not mistaken for the frame currently playing, it
is tied horizontally to the target position on the progress bar and never
floats independently in the middle of the player.

## Geometry and responsive behaviour

- The whole preview sits above the progress bar, centred horizontally on the
  target position. At the left and right edges it stops moving once it reaches
  the inside of the player. The image and the time always move together; only
  the mouse display shows the actual target position.
- The image keeps the video's aspect ratio, with a usual maximum width of
  240px. For portrait videos the width shrinks so that the height does not
  exceed 4/3 of the height of a 16:9 image of the same width.

  | Viewport | Base image width |
  | --- | --- |
  | 360px | 160px |
  | Between 360px and 768px | Scales continuously |
  | 768px and wider | 240px |

  When the source image is fitted to the frame, it is shown whole and never
  cropped.
- The bottom edge of the image is raised far enough that it does not cover the
  progress bar or the controls next to it. During touch it is not placed
  directly under the finger; the image and the time stay readable above the
  finger and the control bar.
- When the player is narrower than 16:9, or the target is at the start or the
  end, the whole preview, including the time and the shadow, stays inside the
  left and right edges of the player. The reference is the player rectangle,
  not the page viewport.
- Moving the target position has no position transition. Only the appearance
  may use the existing short fade; under `prefers-reduced-motion` the preview
  appears immediately, as the existing rules require.

## Visual treatment

The image is separated from the video by the `elevated` surface, a
`border-strong` border, `rounded-md` and `shadow-elevated`. No new colour or
radius token is added. The time is overlaid in one line at the bottom centre of
the image, as `fg` text on the `navbar` or `overlay` surface. The digits inherit
the existing tabular-numeral setting and are not styled more prominently than
the time in the existing control bar.

No separate card or section sits between the image and the time. The inner
padding of the outer frame is only what the time needs to be legible, so that
the image, the time and the target position read as one helper display.

## States and interaction

| State | Observable behaviour |
| --- | --- |
| hidden | The pointer is outside the progress control and no drag is in progress. The preview DOM is not shown. |
| loading | The target time updates immediately. The first time, an empty surface in the video's aspect ratio is shown; when switching, the previous image stays until the new image has finished decoding, so no black surface appears in between. |
| ready | Only the image that matches the current bucket is shown; the time keeps following the pointer position immediately. |
| unavailable | The image area is hidden and only the target time is shown above the progress position. No error text or retry control appears. |

Pointer hover, pointer drag and touch drag use the same display. The preview
returns to hidden immediately when a drag ends, when the pointer leaves while
not dragging, when the video changes and when the screen is left. Leaving the
progress bar during a drag keeps the preview shown; pointer capture or an
equivalent mechanism keeps it following, clamped inside the player, until the
drag ends. Fetching, showing or failing to fetch the image never changes
playback, pause, mute, the source or the actual seek position.

Within the same 5-second bucket the same image stays, and only the time follows
the actual pointer position. When the position moves to another bucket, the
fetch starts immediately, and the shown image is replaced in one step only after
the image for the current position has fully decoded. A stale response that
completes out of order is not shown. When a fetch fails, the held image is
hidden and only the time is shown.

## Accessibility

- The preview has `pointer-events: none` and receives no click, tap, hover or
  drag.
- No link, button or tab stop is added. The existing focus-visible styling and
  keyboard operation of the progress control stay as they are.
- The still image is decorative and has empty alternative text; the preview
  container and the time are hidden from assistive technology. Continuous
  pointer movement does not produce announcements.
- Keyboard operation does not require the preview, and the existing Video.js
  time, seek and focus display do not change.
- Colour is never the only cue for the target position; the mouse display
  position and the time text are used together.
