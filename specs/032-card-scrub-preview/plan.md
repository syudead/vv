# Implementation Plan: Scrub along the bottom of a library card's thumbnail to skim the video

**Branch**: `feature/032-card-scrub-preview` | **Parent Issue**: #616

**Input**: The parent Issue. It is this feature's specification.

## Summary

Video cards in the library (the grid view of the library, the folder screen and search results) and the
related-video thumbnails on the player screen get a transparent band covering the bottom fifth. While the mouse
pointer is in the band, its horizontal position maps to the video's length, the frame at that position is shown
on the thumbnail surface from the seek-thumbnail sprite, and the duration display and watch-position bar switch
to the scrub position. The server and API do not change.

- Frame selection (frame by `intervalMs`, sheet, column and row) and the cropping calculation move from the
  player's seek bar to `web/src/lib/seekSprite.ts` and are shared by both
  ([research.md R-1](research.md#r-1-shared-home-for-frame-selection-and-cropping)).
- The band's state (when it is active, fetching on the first entry into the band, aborting when the pointer leaves
  the card, not retrying after a failure until the pointer comes back, releasing when the card leaves the
  viewport) is held by a hook in `web/src/ui/ScrubPreview.tsx`, one per card, which does not know how loop
  playback works
  ([R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets),
  [R-4](research.md#r-4-releasing-held-sheets)).
- Entering the band stops loop playback with `pause()`; leaving the band into the card resumes it with `play()`.
  The 400ms wait stops while in the band and counts again from where the pointer left. The two existing hooks
  (`useCardPreview`, `useHoverPreview`) gain `suspendPreview` / `resumePreview` for this
  ([R-2](research.md#r-2-loop-playback-and-the-band)).
- The frame is shown in the same frame and with the same fit as the thumbnail, and the band sits inside the
  `Link` in front of the duration display ([R-5](research.md#r-5-how-frames-fit-and-the-bands-shape)). The time
  display and the bar reuse the existing elements, and the watch-position `progressbar` is not rewritten
  ([R-6](research.md#r-6-swapping-the-time-display-and-the-bar)).
- The look (the scrub-position bar's colour, the time separator, the switch under reduced motion) is decided in
  `ui-design.md` by the design stage for the `ui` label.

## Technical Context

**Canonical definitions**:

- Web boundaries and dependency direction, the rule that only `web/src/api/` talks to the server, tokens in
  `index.css`, the rule against fixed strings outside `web/src/i18n/`: [ARCHITECTURE.md](../../ARCHITECTURE.md)
  "Web layer"
- Library hover preview (400ms, one at a time, release triggers, priority versus selection mode, assistive
  technology handling): [specs/010-hover-video-preview/ui-design.md](../010-hover-video-preview/ui-design.md);
  implemented in [web/src/videoList/cardPreview.tsx](../../web/src/videoList/cardPreview.tsx)
  (`useCardPreview`, `CardMedia`) and
  [web/src/videoList/usePreviewCoordination.ts](../../web/src/videoList/usePreviewCoordination.ts); related videos
  in [web/src/player/useHoverPreview.ts](../../web/src/player/useHoverPreview.ts) and
  [web/src/player/RelatedVideos.tsx](../../web/src/player/RelatedVideos.tsx) (`VideoThumbnail`)
- The sprite layout and sheet contract (`SeekThumbnailSprite`, frame selection by `intervalMs`, empty cells in the
  last sheet, `private, no-cache` and `ETag`): [api/openapi.yaml](../../api/openapi.yaml)
  (`getVideoSeekThumbnail`, `getVideoSeekThumbnailSheet`),
  [specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md);
  current generation (81 frames, 9×9, 160px long side; the old format has up to 600 frames in 6 sheets):
  [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)
- Player fetching and cropping: [web/src/player/seekPreview.ts](../../web/src/player/seekPreview.ts)
  (`seekSpriteCell`, `seekPreviewTarget`, `showCell`); fetch functions `fetchSeekThumbnailSprite` and
  `fetchSeekThumbnailSheet` ([web/src/api/client.ts](../../web/src/api/client.ts))
- Card dimensions and overlays, the blurred background for portrait videos:
  [web/src/videoList/VideoCard.tsx](../../web/src/videoList/VideoCard.tsx),
  [web/src/ui/ThumbnailBackdrop.tsx](../../web/src/ui/ThumbnailBackdrop.tsx); rules in
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task test-e2e`). The
  e2e fixtures with a real sprite and the hover preview:
  [web/e2e/playback.e2e.ts](../../web/e2e/playback.e2e.ts) (`waitForSeekThumbnails`),
  [web/e2e/hover-preview.e2e.ts](../../web/e2e/hover-preview.e2e.ts)

**Feature-specific context**:

- No dependency is added. The server, API and generated files do not change (parent Issue `対象外`).
- No new dependency direction. Shared calculation goes in `web/src/lib/`, and the band's hook and components in
  `web/src/ui/`. This feature is the first time `ui/` imports fetch functions from `api/client`
  ([R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets)).
- The band reacts only to `pointerType === "mouse"` (requirement 10). Touch, pen and keyboard keep their current
  behaviour.
- In scope: `VideoCard` (grid view) and the thumbnails of `MemberItem` and `RelatedItem` in `RelatedVideos`.
  `VideoRow` (list view), `GroupCard`, `CurrentMember` and the "next video" prompt do not change (requirement 8,
  out of scope).

## Constitution Check

- **Web boundaries and dependency direction** (ARCHITECTURE.md "Web layer"): pass. `videoList/` and `player/` do
  not import each other; shared code goes in `lib/` (pure calculation) and `ui/` (hook and components). Only the
  existing fetch functions in `api/client.ts` talk to the server.
- **Tokens from `index.css`, no raw colours**: pass. The band is transparent, the frame is the sheet image, and the
  bar uses existing tokens. Colours the design stage chooses also come from tokens.
- **No fixed strings outside `web/src/i18n/`**: pass. The time "position / length" is added to `en.ts` as
  `t.list.card.scrubTime` ([R-6](research.md#r-6-swapping-the-time-display-and-the-bar)).
- **Width branches and reduced motion are handled in CSS** (library-ui.md, [Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception)): pass. The band's height is a CSS
  percentage, and JavaScript only reads the band's rectangle to compute the position. Switching frames has no
  transition.
- **Documentation changes in the same PR as the change** (core-beliefs.md): pass. The first unit adds one sentence
  about `lib/seekSprite.ts` and `ui/ScrubPreview.tsx` to ARCHITECTURE.md "Web layer"; the design stage adds the
  link to `ui-design.md` in `docs/design-docs/index.md`.

The verdict is the same after Phase 1. There is no violation for Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/032-card-scrub-preview/
├── plan.md          # This file
│                    # No spec.md — the parent Issue is the specification
├── research.md      # R-1 to R-6: shared home, relation to the loop, fetch rules, release, fit, display swap
└── ui-design.md     # Added by the design stage (ui label)
```

No `data-model.md`: no entity or column is added, and the screen state lives only inside the hook.
No `contracts/`: the API does not change, and the existing layout and sheet contract is used as is.
No `quickstart.md`: the acceptance criteria are verified with the real sprite in `task test-e2e` (the fixture in
`playback.e2e.ts`); a person checks only the screen composition, which each unit's Acceptance states.

### Source Code

**Affected boundaries**:

| Path | Change |
| --- | --- |
| `web/src/lib/` | Pure calculation for sprite position → frame and frame → crop (`seekSprite.ts`, new) |
| `web/src/ui/` | The band hook, the frame layer and the band component (`ScrubPreview.tsx`, new) |
| `web/src/player/` | `seekPreview.ts` uses `lib/seekSprite.ts` (behaviour unchanged). Suspend and resume in `useHoverPreview.ts`; the band in `VideoThumbnail` of `RelatedVideos.tsx` |
| `web/src/videoList/` | Suspend and resume in `cardPreview.tsx`; the band and display swap in `VideoCard.tsx` |
| `web/src/i18n/en.ts` | `t.list.card.scrubTime` |
| `web/e2e/` | e2e for the band (one new file) |
| `ARCHITECTURE.md` | One sentence on the shared homes in "Web layer" |

**New paths**: `web/src/lib/seekSprite.ts`, `web/src/ui/ScrubPreview.tsx`, `web/e2e/card-scrub.e2e.ts`.

**Structure decision**: shared calculation and components go in `lib/` and `ui/`, creating no dependency between
`videoList/` and `player/` ([R-1](research.md#r-1-shared-home-for-frame-selection-and-cropping),
[R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets)).

## Implementation Work

### Share seek-sprite frame selection and cropping, and build the card band hook

**Scope**: move `seekSpriteCell`, `seekPreviewTarget`'s position calculation and `showCell`'s sheet layout to
`web/src/lib/seekSprite.ts`, and have `web/src/player/seekPreview.ts` call them
([R-1](research.md#r-1-shared-home-for-frame-selection-and-cropping); the player's behaviour does not change).
Add to `web/src/ui/ScrubPreview.tsx`:

- `useScrubPreview`: when the band is active, the position from the band's rectangle and `clientX`, fetching the
  layout and sheet on the first entry into the band, aborting when the pointer leaves the card, not retrying after
  a failure until the pointer comes back, clearing the display outside the band, and releasing when the card
  leaves the viewport and on unmount.
- `ScrubFrame`: a layer that shows one frame in the same frame as the thumbnail, only when both the layout and the
  sheet are present.
- `ScrubBand`: the transparent band over the bottom fifth; ignores pointers other than `mouse`.

([R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets),
[R-4](research.md#r-4-releasing-held-sheets), [R-5](research.md#r-5-how-frames-fit-and-the-bands-shape)).
Add `suspendPreview` / `resumePreview` to `useCardPreview` and `useHoverPreview`
([R-2](research.md#r-2-loop-playback-and-the-band)). Nothing is attached to the screens yet. Add one sentence on the
two homes to ARCHITECTURE.md "Web layer".

**Dependencies**: None.

**Acceptance**: `task check` and `task check-docs` pass, and these tests pass:

- `web/src/lib/seekSprite.test.ts`: the right end of the band gives position `ceil(durationMs) - 1` and never
  exceeds frame `frameCount - 1`; with several sheets, the pointed position gives that sheet's number. The player's
  `seekPreview.test.ts` passes with the same results as now.
- `web/src/ui/ScrubPreview.test.tsx`: no fetch happens before entering the band; the first entry fetches the
  layout, then that frame's sheet, once each; entering and leaving the band does not refetch; a position in another
  sheet fetches only that next sheet; leaving the band before the fetch finishes shows no frame, and leaving the
  card aborts it; leaving the card during the layout or sheet fetch and coming back into the band fetches again
  and shows the frame; after a failure there is no fetch until the pointer comes back; leaving the viewport
  releases the object URL and the next entry fetches again; `touch` and `pen` do nothing.
- The `cardPreview.test` and `useHoverPreview` tests: suspending during the wait clears the timer and resuming
  counts 400ms again; suspending during playback calls `pause()` and resuming calls `play()`; suspending while
  loading (`attempting`, before `playing`) keeps `src` and the element, playback does not start until resume, and
  resuming calls `play()`.

### Add the scrub band to library video cards, swapping loop playback and the time display

**Scope**: in `VideoCard.tsx` and `cardPreview.tsx`, place `ScrubBand` and `ScrubFrame` on the thumbnail surface
inside the `Link`, and call `suspendPreview` / `resumePreview` on entering and leaving the band. While in the band,
the duration display shows `t.list.card.scrubTime` and a scrub-position bar replaces the watch-position bar
([R-6](research.md#r-6-swapping-the-time-display-and-the-bar); the look is in `ui-design.md`). Changes of
`previewResetEpoch`, `activePreviewId` and `selectionMode`, and `releasePreview`, also end scrubbing. No band on
videos without `seekThumbnailUrl`, videos showing the full-surface warning, in selection mode, on `VideoRow` or on
`GroupCard`. Add `web/e2e/card-scrub.e2e.ts`, checking the library and the folder screen (including search
results) with the same real sprite as `playback.e2e.ts`.

**Dependencies**: Share seek-sprite frame selection and cropping, and build the card band hook; the design stage's
`ui-design.md`.

**Acceptance**: `task check` and `task check-docs` pass. `VideoCard.test.tsx` checks that the time display and bar
are swapped only while in the band and return on leaving, that a click on the band navigates as usual, and that
there is no band in selection mode or on videos with a warning. `card-scrub.e2e.ts` in `task test-e2e` checks
acceptance criteria 1 to 7 and 9 in the library, the folder screen and search results within a folder:

- From the band's left end to its right end, `background-position` changes monotonically from first to last.
- At the same fraction, the sheet and `background-position` equal those of the player screen's seek-bar tooltip.
- Passing over sends no request to `seek-thumbnail`; entering the band fetches the layout and the sheet once each,
  and entering and leaving does not add more.
- Entering the band during the loop makes the video `paused`, and leaving returns it to playing.
- A click on the band opens the player screen without changing the start position.
- Entering and leaving the band and waiting for a fetch do not change the card's `getBoundingClientRect` or
  `scrollY`.

This unit changes a screen, so the PR keeps images at 360px, 768px and 1280px of four states (frame, time and bar
while in the band; a portrait video's frame; waiting for a fetch; fetch failed), confirming there is no decoration
marking the band and the card's dimensions do not change.

### Add the scrub band to related-video thumbnails on the player screen

**Scope**: add `ScrubBand` and `ScrubFrame` to `VideoThumbnail` in `RelatedVideos.tsx`; `MemberItem` and
`RelatedItem` hold `useScrubPreview` and call `useHoverPreview`'s `suspendPreview` / `resumePreview`. The length
display at the bottom right and the progress bar at the bottom edge are swapped by the same rule as cards
([R-6](research.md#r-6-swapping-the-time-display-and-the-bar); the look is in `ui-design.md`). `CurrentMember` and
the "next video" prompt do not change. Add band checks for related videos to `playback.e2e.ts`.

**Dependencies**: Share seek-sprite frame selection and cropping, and build the card band hook; the design stage's
`ui-design.md`.

**Acceptance**: `task check` and `task check-docs` pass. The `RelatedVideos` tests check that the length display
and bar are swapped only while in the band, and that `CurrentMember` has no band. `playback.e2e.ts` in
`task test-e2e` checks that, at the same fraction of a related video's band, the sheet and `background-position`
equal those of the player's seek-bar tooltip (acceptance criterion 3), that passing over sends no request, and
that a click on the band moves to that video. This unit changes a screen, so the PR keeps images at 768px and
1280px of a related video while in the band, confirming that row height and column width do not change.
