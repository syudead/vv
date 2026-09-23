# UI Contract: Library Hover Video Preview

## Scope

この contract はグリッド表示の `VideoCard` が利用者へ見せる hover preview の状態遷移を定義する。要求の正本は [GitHub Issue #134](https://github.com/syudead/vv/issues/134)、実装境界は [plan.md](../plan.md#source-code) に従う。

## Eligible Card

A grid card is eligible for preview only when all conditions are true:

- `video.playable === true`
- `video.probeState === "done"`
- Existing `Video` data needed to build the direct stream URL is present
- The active input can hover with a fine pointer
- The card is mounted in grid view

If any condition is false, hover leaves the current thumbnail, disabled, pending, failed, watched, and progress displays unchanged. A missing or failed thumbnail does not by itself make a playable card ineligible; the card returns to its existing placeholder after preview stops.

## State Transitions

- `thumbnail`: default state. The thumbnail or existing placeholder is visible with existing quality, duration, watched, progress, selection, and unplayable overlays.
- `pending-preview`: pointer has entered an eligible card and the delay timer is running. The card remains visually equivalent to `thumbnail`.
- `previewing`: delay elapsed and the muted video has started or is attempting to start. The video is clipped to the thumbnail area, uses the same aspect ratio, and never pushes title, metadata, toolbar, selection bar, or adjacent cards.
- `fallback-thumbnail`: `play()` is rejected, the media element errors, the card becomes ineligible, or the pointer leaves. The video element is removed or reset and the thumbnail state is restored.

The implementation may keep `pending-preview` internal if no visible state differs.

## Required Behaviors

- Preview start is delayed so fast pointer passes across cards do not start playback.
- Only the card currently under the pointer may preview. Leaving a card stops its preview before or as the next card starts.
- Pointer leave, navigation to `/videos/:id`, grid/list switch, filtering, sorting, reload, infinite-scroll unmount, and component unmount stop playback and release the video element.
- The preview video is always muted and inline. The UI provides no volume, unmute, seek, playback controls, or progress-save action.
- Starting, playing, pausing, ending, or failing the preview does not call `PUT /api/videos/{id}/progress`.
- Selection controls keep their existing z-order and click behavior. In selection mode, clicking the card toggles selection instead of navigating, as it does today.
- Keyboard focus does not start preview. Focus indicators and Enter navigation continue to work.
- Touch-only environments do not start preview from touch contact.
- `prefers-reduced-motion: reduce` suppresses decorative scale/fade motion while preserving the final preview/thumbnail states.

## Observable Evidence

- Unit tests cover delay/cancel, ineligible cards, cleanup on unmount, keyboard/touch non-start, progress API non-use, and selection-mode interaction.
- Manual or browser-backed verification covers 360px, 768px, and 1280px widths, plus reduced motion.
