# UI Design: Library Hover Preview

## Design Basis

This artifact turns `UI品質とアクセシビリティ` of [GitHub Issue #134](https://github.com/syudead/vv/issues/134) into criteria that can be judged from the implementation and from screenshots. The existing rules for card width, colour, type, overlays, focus and selection are owned by [library-ui.md](../../docs/design-docs/library-ui.md) and the role tokens in `web/src/index.css`; this document does not change them.

The user can tell apart videos with similar thumbnails and titles without reopening them, while keeping their position and actions in the list. The preview plays on the same surface as the still image and does not change the information density of the card.

## Screen Boundary

- The target is the 16:9 thumbnail surface of the grid `VideoCard`. On both the library screen and the folder screen (including search results), at most one preview plays at a time. List rows, the toolbar, the selection bar and the player do not change.
- The preview video is an absolute layer with the same inset, crop and aspect ratio as the thumbnail, and does not take part in the size calculation of the card, the grid or the metadata.
- The meaning and position of the title, duration, progress, selection checkbox and focus ring stay the same. A card that shows the existing full-surface warning because the probe is pending or failed, or because the duration or codec is missing, cannot have a preview job at all, so that warning and preview eligibility are mutually exclusive. When the original is browser-incompatible but the probe metadata is complete and the preview is done, the current `unplayableText` returns no warning and does not cover the preview.
- No new text, badge, spinner, toolbar, or audio or seek control appears. Generating, not generated, failed and playback error are all shown as the current thumbnail or placeholder.

## Interaction

1. When a pointer with `pointerType === "mouse"` enters an eligible card and stays inside the same card for 400ms, that card becomes the active preview and fetching starts. 400ms is the same as the existing tooltip hover delay, so that moving across the list does not cause media requests.
2. The thumbnail or placeholder stays shown until the first `playing` event. No blank surface or loading indicator is shown while waiting for metadata or the first buffering.
3. After playback starts, the preview is shown muted and inline on the same surface, and loops at the end of the clip. On `waiting` or `stalled` after playback started, the current video frame stays and playback resumes on `playing`. Only an error returns to the thumbnail. The moving content itself marks the preview; a still frame does not add a new state indicator.
4. Pointer leave, activation of another card, click navigation, a filter, sort, page or list-append change, switching between grid and list, zoom or viewport resize, and unmount immediately stop the timer and playback, release the media resource and return to the thumbnail or placeholder.
5. On a `play()` rejection, a network or media error, or an asset 404, the thumbnail or placeholder stays, and no error overlay or toast is added. Once the pointer leaves and enters again, a new attempt is allowed.
6. In selection mode, the existing checkbox and card-click selection take precedence. A pointer over the checkbox does not start the preview timer, and when selection mode starts during a preview, the checkbox stays on top.

`LibraryPage` coordinates only the active card ID and the reset epoch. The card owns the timer, the video element, the playing and error state and resource cleanup. At most one active preview is visible at a time.

## Visual Hierarchy

- The thumbnail or the preview is the card's primary visual information; the title and the duration and progress support telling videos apart. The preview never extends outside the surface, and no outline or label stronger than the title is added.
- The public mark, the duration, the progress bar and the selection checkbox sit in front of the preview layer. During the preview, the duration background switches to opaque `navbar` and the progress track to opaque `fg-subtle`, so they stay distinguishable regardless of how light or dark the frame is. The normal thumbnail state looks the same as before.
- During the preview the card hover shadow and slight lift stay as they are now; no feature-specific glow, colour change or enlargement is added. The thumbnail and the video sit inside the same media-layer wrapper, and the existing hover scale is applied to the wrapper, so the crop and scale do not jump when the surface switches to `playing`.

## Information Density

- The number of text items and controls a card shows does not increase. The number of cards in the grid and the wrap positions are the same at rest, during the delay, during the preview and after fallback.
- Why a preview is unavailable is not shown in the list. Only the existing thumbnail placeholder and state display are used; the detailed failure reason belongs to the job inspection path.

## Spacing Rhythm

- `aspect-video`, the card width token, the grid gap, the metadata padding and the card radius are used unchanged.
- The video uses `h-full w-full object-contain`, the same framing as the thumbnail (portrait videos are shown whole, not cropped; for portrait and near-square videos, a blurred copy of the same thumbnail fills the left and right margins). Starting, stopping and buffering do not move the surface height, the card height, neighbouring cards, the toolbar, the selection bar or the scroll position.

## Typography

- The font family, size, weight, line height, line clamp and tabular numbers of the title, metadata, duration and state display do not change.
- The preview state is not added as visible text or an icon label, so no new typography token is needed. During the preview, the existing overlay backgrounds become opaque to keep them readable.

## Responsive And Motion

- Eligibility does not branch on viewport width. On touch-first devices a mouse event can still start a preview; touch, pen and focus never start one.
- Layout follows the existing CSS breakpoints and the card width token. JavaScript resize handling is used only to reset playback, never to decide width or placement.
- Under `prefers-reduced-motion: reduce`, the existing CSS rule effectively stops the card lift, the thumbnail scale and opacity transitions. Video playback, which results from the user deliberately resting the mouse, stays available, but the switch from the thumbnail to the first playing frame and the return on stop happen immediately, without fade or scale.

## Accessibility

- Keyboard focus alone does not start a preview. Tab reaches the card link and the checkbox, and focus-visible outlines, Enter navigation, Space or click selection, and Esc to clear the selection work as before.
- The preview video is decorative supporting content and adds no duplicate title or control to the accessibility tree. The accessible name of the card link stays the title.
- The video is always muted, `playsInline` and without controls. It has no path to enable audio and no progress update.
- Screen readers get no live announcement when the preview starts or stops. It is transient, pointer-only visual information and does not interrupt keyboard announcements.

## System States

| State | Visible result |
| --- | --- |
| eligible, idle | The current thumbnail or placeholder and overlays |
| 400ms delay | Same as idle. No request, spinner or label |
| initial loading/buffering | The thumbnail or placeholder stays |
| playing | Only the content of the thumbnail surface is replaced by the looping preview. Existing overlays stay in front |
| waiting/stalled after playing | The current preview frame stays until playback resumes |
| pending/failed/missing preview | Stays idle. No fallback to the source stream |
| play/media error | Returns to idle immediately; hovering again is allowed |
| focus/touch/pen, direct use of the checkbox | Existing interaction only. No preview starts |
