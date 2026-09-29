# UI Design: thumbnail preview while seeking

**Feature**: [parent Issue #117](https://github.com/syudead/vv/issues/117)

The existing playback page, the Video.js control structure and the role tokens follow
[Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md) and
[`web/src/index.css`](../../web/src/index.css). This document defines only the display and
interaction the seek preview adds. The page structure of the list and playback pages does not
change.

## Screen boundary and hierarchy

The preview is a transient secondary display that belongs to the progress bar inside
`VideoPlayer`. Its state does not leak into the page body, the video information or the list
cards. The video surface stays the largest display area. The preview overlays the lower part of the
video surface only while the pointer is over the progress bar, or while a pointer or touch drag is
in progress.

The display consists of these three elements only:

1. A still image for the target time (in the video's aspect ratio; 16:9 when unknown)
2. The target time as `h:mm:ss` or `m:ss`
3. The existing mouse display on the progress bar, which shows the matching position

No title, loading text, spinner, button or explanation outside the frame is added. So that the
preview is not mistaken for the video currently playing, it is tied horizontally to the target
position on the progress bar and does not float independently in the center of the player.

## Geometry and responsive behaviour

- The whole preview sits above the progress bar, centered horizontally on the target position. At
  the left and right edges it stops moving horizontally where it still fits inside the player. The
  image and the time always move together; only the mouse display shows the actual target position
- The image keeps the video's aspect ratio, with a maximum width of 240 px. For portrait video the
  width shrinks so that the height does not exceed 4/3 of the height of a 16:9 image of the same
  width
- The base image width is 160 px at a 360 px viewport and 240 px at 768 px and above, scaling
  continuously in between. When fitting the source image to the frame, the whole image is shown,
  never cropped
- The bottom edge of the image sits far enough above the progress bar not to cover it or adjacent
  controls. During touch it is not placed directly under the finger; the image and time stay
  visible above the finger and the control bar
- When the player is narrower than 16:9, or the target is at the start or end, the whole preview,
  including the time and the shadow, stays inside the player's left and right edges. The reference
  is the player rectangle, not the page viewport
- Moving the target position has no position transition. Only the appearance may use the existing
  short fade, and with `prefers-reduced-motion` it appears instantly, per the existing rules

## Visual treatment

The image is separated from the video by an `elevated` surface, a `border-strong` border,
`rounded-md` and `shadow-elevated`. No new color or radius token is added. The time overlays the
bottom center of the image on one line, as `fg` text on a `navbar` or `overlay` surface. Digits
inherit the existing tabular numeral setting, and the time is not styled more strongly than the
existing control bar time.

No separate card or section sits between the image and the time. The inner padding of the outer
frame is only what the time needs to stay legible, and the image, the time and the target position
stay close enough to read as one secondary display.

## States and interaction

| State | Observable behaviour |
| --- | --- |
| hidden | The pointer is outside the progress control and no drag is in progress. No preview DOM is shown |
| loading | The target time updates instantly. The first time, an empty surface in the video's aspect ratio is shown. On a switch, the previous image stays until the new image finishes decoding, so no black surface appears in between |
| ready | Only the image that matches the current bucket is shown, and the time keeps following the pointer position instantly |
| unavailable | The image area is hidden and only the target time is shown above the progress position. No error text or retry action appears |

Pointer hover, pointer drag and touch drag use the same display. Drag end, pointer leave outside a
drag, a video switch and leaving the page return to hidden immediately. During a drag the display
stays even outside the progress bar; pointer capture or an equivalent keeps the position inside the
player and follows it until the drag ends. Fetching, showing or failing an image never changes
playback, pause, mute, the source or the actual seek position.

Within the same 5 s bucket the same image stays, and only the time follows the actual pointer
position. On a move to another bucket, the fetch starts immediately; the image for the current
position is fully decoded and then replaces the displayed image in one step. Stale responses that
complete out of order are not shown. On a fetch failure the retained image is hidden and only the
time is shown.

## Accessibility

- The preview has `pointer-events: none` and receives no click, tap, hover detection or drag
- No link, button or tab stop is added. The focus-visible style and keyboard operation of the
  existing progress control stay
- The still image is decorative with empty alternative text, and the preview container and the
  time are hidden from assistive technology. Continuous pointer movement does not become
  announcements
- The preview is not required during keyboard operation, and the existing Video.js time, seeking
  and focus display do not change
- Color is not the only cue for the target position; the mouse display position and the time
  string are used together
