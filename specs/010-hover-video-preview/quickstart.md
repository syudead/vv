# Quickstart: 一覧画面の hover 動画プレビュー

## Prerequisites

- Follow the repository setup and checks in [Taskfile.yml](../../Taskfile.yml).
- Prepare at least one browser-playable video (`playable=true`) with completed probe, one playable card without a ready thumbnail, plus one ineligible card such as `playable=false` or probe pending/failed.

## Automated Checks

Run the focused web tests while implementing:

```powershell
npm --prefix web run test -- LibraryPage
```

Before opening the implementation PR, run:

```powershell
task check
```

If the implementation adds browser-backed coverage for pointer media behavior, also run the targeted e2e test and note it in the PR.

## Manual Validation

1. Start the app with the normal development command:

   ```powershell
   task dev
   ```

2. Open the library in grid view. Hover a playable, ready card long enough for the delay to elapse. Expected: a muted video preview plays inside the thumbnail area; no sound controls appear.

3. Move the pointer away, then hover a second eligible card. Expected: the first card returns to its thumbnail and only the second card previews.

4. Click a previewing card. Expected: navigation to `/videos/:id` still occurs, and the preview stops during navigation.

5. Watch network calls while starting and stopping previews. Expected: no `PUT /api/videos/{id}/progress` request is sent.

6. Hover ineligible cards: unplayable, probe pending/failed, or missing playback information. Expected: the existing card state stays visible and no video preview starts.

7. Hover a playable card whose thumbnail is pending or failed. Expected: preview can start from the placeholder, and stopping preview restores that placeholder.

8. Reject the preview element's `play()` promise, then simulate a media load or playback error in a separate attempt. Expected: each failure removes or resets the preview, restores the prior thumbnail or placeholder without a large error overlay, and a later hover can try again.

9. Enable selection mode by checking a card, then interact with card checkboxes and card bodies. Expected: selection toggles stay usable and the check indicator is not hidden by preview.

10. Verify keyboard behavior by tabbing to cards/links and pressing Enter. Expected: focus rings and navigation work, and focus alone does not start preview.

11. Verify a touch-only or emulated touch environment. Expected: touching a card does not start hover preview; existing selection and navigation behavior remains available. Then emulate a touch-primary device with an attached mouse, or dispatch mouse pointer events without changing the primary-input media query. Expected: mouse hover starts preview even when `(hover: hover) and (pointer: fine)` is false.

12. Capture implementation screenshots at 360px, 768px, and 1280px while a card is previewing and compare them with the approved `ui-design.md`. Expected: its visual hierarchy, information density, spacing, typography, and action-priority criteria pass, with no overlap between the preview and adjacent cards, toolbar, selection bar, progress bar, state labels, title, or metadata.

13. Repeat with `prefers-reduced-motion: reduce`. Expected: preview can start and stop, but decorative scale/fade motion is reduced and no excessive flicker appears.
