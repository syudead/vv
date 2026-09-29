# UI Design: Library Hover Preview

## Design Basis

This artifact turns the `UI品質とアクセシビリティ` ("UI quality and accessibility") section of
[GitHub Issue #134](https://github.com/syudead/vv/issues/134) into a form that the implementation
and screenshots can be judged against. The existing rules for card width, color, type, overlays,
focus and selection come from [library-ui.md](../../docs/design-docs/library-ui.md) and the role
tokens in `web/src/index.css`; this document does not change them.

The user can tell apart videos with similar thumbnails and titles without opening them again,
while the list position and interaction stay the same. The preview plays on the same surface as
the still image and does not change the card's information density.

## Screen Boundary

- The target is the 16:9 thumbnail surface of `VideoCard` in the grid. On both the library and
  folder screens (including search results), at most 1 preview plays at a time. List rows, the
  toolbar, the selection bar and the player do not change.
- The preview video is an absolute layer with the same inset, crop and aspect ratio as the
  thumbnail. It takes no part in the size calculation of the card, grid or metadata.
- The meaning and position of the title, duration, progress, selection checkbox and focus ring
  stay the same.
- A card that shows the existing full-surface warning (probe pending/failed, or missing
  duration/codec) has no preview job, so it is never preview-eligible. When the original is
  browser-incompatible but the probe metadata is complete and the preview is done, the current
  `unplayableText` returns no warning and does not cover the preview.
- No new text, badge, spinner, toolbar, or audio/seek control appears. Generating, not generated,
  failed and playback error states all show the current thumbnail/placeholder.

## Interaction

1. When a pointer with `pointerType === "mouse"` enters an eligible card and stays inside the same
   card for 400 ms, that card becomes the active preview and starts fetching. 400 ms matches the
   existing tooltip hover delay, so moving across the list does not create media requests.
2. The thumbnail/placeholder stays visible until the first `playing` event. No blank surface or
   loading indicator appears while waiting for metadata or initial buffering.
3. After playback starts, the muted inline preview shows on the same surface and loops at the end
   of the clip. On `waiting`/`stalled` after playback, the current video frame stays, and playback
   resumes on `playing`. Only an error returns to the thumbnail. The moving content itself marks
   the preview; a still frame adds no new state indicator.
4. Pointer leave, activation of another card, click navigation, filter/sort/page/list append,
   grid/list toggle, zoom/viewport resize and unmount immediately stop the timer and playback,
   release the media resources and return to the thumbnail/placeholder.
5. A `play()` rejection, a network/media error or an asset 404 keeps the thumbnail/placeholder and
   adds no error overlay or toast. Once the pointer leaves and enters again, a new attempt is
   allowed.
6. In selection mode, the existing checkbox and card click selection take priority. A pointer on
   the checkbox does not start the preview timer, and when selection mode starts during a preview,
   the checkbox stays on top.

`LibraryPage` coordinates only the active card ID and the reset epoch. The card owns the timer,
the video element, the playing/error state and resource cleanup. At most 1 active preview is
visible at a time.

## Visual Hierarchy

- The thumbnail or preview is the card's primary visual information; the title and
  duration/progress support identification. The preview stays inside the surface and adds no
  frame or label stronger than the title.
- The public mark, duration, progress bar and selection checkbox sit in front of the preview
  layer. During a preview, the duration background switches to opaque `navbar` and the progress
  track to opaque `fg-subtle`, so they stay distinguishable regardless of frame brightness. The
  normal thumbnail state looks the same as before.
- During a preview, the card hover shadow and slight lift stay as they are; no feature-specific
  glow, color change or enlargement is added. The thumbnail and the video sit in the same
  media-layer wrapper, and the existing hover scale applies to the wrapper, so the crop/scale does
  not jump on the switch to `playing`.

## Information Density

- The number of text items and controls a card shows does not grow. The number of grid cards and
  the wrap positions are the same in the normal state, during the delay, during a preview and
  after fallback.
- The list shows no extra reason why a preview is unavailable. Only the existing thumbnail
  placeholder and state display are used; the job inspection path owns detailed failure reasons.

## Spacing Rhythm

- `aspect-video`, the card width token, the grid gap, the metadata padding and the card radius
  stay as they are.
- The video uses `h-full w-full object-contain`, the same framing as the thumbnail (portrait
  videos are shown whole without cropping; for portrait and near-square videos, a blurred copy of
  the same thumbnail fills the side margins). Start, stop and buffering do not move the surface
  height, card height, adjacent cards, toolbar, selection bar or scroll position.

## Typography

- The font family, size, weight, line height, line clamp and tabular numbers of the title,
  metadata, duration and state display do not change.
- The preview state adds no visible text or icon label, so no new typography token is needed.
  During a preview, the existing overlay background becomes opaque to keep it readable.

## Responsive And Motion

- Eligibility does not branch on viewport width. On touch-first devices a mouse event can still
  start a preview; touch, pen and focus do not.
- Layout follows the existing CSS breakpoints and the card width token. JavaScript resize handling
  exists only to reset playback and does not decide width or placement.
- Under `prefers-reduced-motion: reduce`, existing CSS rules effectively stop the card lift, the
  thumbnail scale and opacity transitions. Video playback, which results from the user explicitly
  resting the mouse, stays available. The swap between the thumbnail and the first playing frame,
  and the return on stop, happen immediately without fade or scale.

## Accessibility

- Keyboard focus alone does not start a preview. Tab reaches the card link and checkbox, and the
  focus-visible outline, Enter navigation, Space/click selection and Esc selection clear work as
  before.
- The preview video is decorative supplementary content and adds no duplicate title or control to
  the accessibility tree. The card link's accessible name stays the title.
- The video is always muted, `playsInline`, and has no controls. It has no path to enable audio
  and no progress updates.
- Screen readers get no live announcement of preview start or stop. The preview is temporary
  pointer-only visual information and must not interrupt keyboard announcements.

## System States

| State | Visible result |
| --- | --- |
| eligible, idle | Current thumbnail/placeholder and overlays |
| 400 ms delay | Same as idle. No request, spinner or label |
| initial loading/buffering | Keeps the thumbnail/placeholder |
| playing | Only the thumbnail surface content is replaced by the looping preview. Existing overlays stay in front |
| waiting/stalled after playing | Keeps the current preview frame and waits to resume |
| pending/failed/missing preview | Stays idle. No source stream fallback |
| play/media error | Returns to idle immediately and allows a new hover |
| focus/touch/pen, direct checkbox interaction | Existing interaction only. No preview start |
