# Research: Scrub along the bottom of a library card's thumbnail to skim the video

Inherited decisions: [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
and [ARCHITECTURE.md](../../ARCHITECTURE.md) (Web layer). This file records only the decisions this feature adds.
The parent Issue is #616.

## R-1: Shared home for frame selection and cropping

**Decision**: the pure function `seekSpriteCell` in `web/src/player/seekPreview.ts`, which picks a frame from a
position, and the calculation that lays a sheet over the frame box and shifts it by column and row (`showCell`'s
`background-size` / `background-position` and `offsetPercent`) move to `web/src/lib/seekSprite.ts`. Both the
player's seek bar and the card band use them from there. The rule that turns the band's horizontal position into
a position (milliseconds) goes in the same file and shares `seekPreviewTarget`'s position calculation (rounded to
`ceil(durationMs) - 1` at the right end). `attachSeekPreview` itself stays: it is a player-only attachment that
builds the DOM directly.

**Rationale**: requirement 5 says "the player and the library never show different frames for the same
position". Calling the same function is more reliable than writing the rule in two places and keeping them in
step, and it is tested in one place. `web/src/lib/` is the home for locale-independent calculation, with a
precedent of reading only types from `api/client` (ARCHITECTURE.md "Web layer"). `videoList/` and `player/` do not
import each other, so placing it in either would create a new dependency direction.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Attach `attachSeekPreview` to cards too | Rejected: it builds its own tooltip (`.vv-seek-preview`) with its time display and captures the pointer on `pointerdown`, which does not fit a band that shows the frame on the thumbnail itself. It also does not mesh with React state (swapping the time display, pausing the loop) |
| Copy the rule into `web/src/ui/` | Rejected: the same formula in two places invites changing the handling of `intervalMs` in only one |

## R-2: Loop playback and the band

**Decision**: on entering the band, the action depends on the loop's state:

| Loop state | On entering the band | On leaving the band into the card |
| --- | --- | --- |
| During the 400ms wait (timer running) | Clear the timer | Count 400ms again from there |
| Playing (`playing`) or loading (`attempting`, before `playing`) | `pause()` the `video` element, keeping `src` and the element | Resume with `play()` |

Leaving the band to outside the card, another card's preview starting, a change of `previewResetEpoch`, or
entering selection mode releases the preview, as the current `releasePreview` does.

This "suspend" and "resume" are added to both `useCardPreview` (`web/src/videoList/cardPreview.tsx`) and
`useHoverPreview` (`web/src/player/useHoverPreview.ts`) as `suspendPreview()` / `resumePreview()`, and the band
hook (R-3) only calls them on entering and leaving the band. The band hook does not know how the loop works.

**Rationale**: requirement 3 asks that "leaving the band while staying on the card returns to loop playback" and
"the wait does not progress while in the band and counts again from where the pointer left the band". Stopping
with `pause()` meets the Edge Case "if the band is entered during loop playback, keep showing the frame at the
moment it stopped" with the `video` element's last frame as it is. Releasing and restarting from 400ms would
return slowly and also lose the picture to show while waiting for the fetch.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Release the loop on entering the band and restart from 400ms on leaving | Rejected: does not meet requirement 3's "returns to loop playback" (returning takes 400ms or more and reloads) |
| Merge `useHoverPreview` into `useCardPreview` before adding the band | Rejected: the two hooks grew separately in 010 and 012 and differ in coordination (`activePreviewId`). Merging is a separate change beyond this feature's scope (requirement 8); adding the same two operations to both is smaller |

## R-3: The band hook, and rules for fetching the layout and sheets

**Decision**: `web/src/ui/ScrubPreview.tsx` holds the hook `useScrubPreview`, the frame layer `ScrubFrame`, and the
band `ScrubBand`. There is one hook per card, holding the following:

| Concern | Rule |
| --- | --- |
| When the band is active | `video.seekThumbnailUrl` exists, `durationMs` is positive, `unplayableText(video) === null`, not in selection mode, and `pointerType === "mouse"`. It does not depend on `previewState` (Edge Cases: the band works whenever there is a sprite, even with no loop) |
| Fetching | When the pointer enters the band on that card **for the first time**, call `fetchSeekThumbnailSprite`; when the layout arrives, fetch the sheet holding the current position's frame with `fetchSeekThumbnailSheet` and hold it as an object URL. For the old multi-sheet format (up to 6 sheets), fetch only the sheet for the pointed position when it is needed. Held sheets and the layout are not fetched again (acceptance criterion 5) |
| Aborting | When the pointer leaves the card, abort fetches in progress with `AbortController`. While the pointer has left the band but stays on the card, fetching continues; when it finishes, no frame is shown (it is used on the next entry into the band) |
| Failure (`409`, `404`, network) | Set the state to "unavailable" and show nothing but the thumbnail. Do not fetch again until the pointer leaves the card and comes back. The player's 5-second retry is not used |
| Display | Show the frame for the latest pointer position at the moment fetching finishes. Draw nothing while waiting (`ScrubFrame` renders no element until both the layout and the sheet are present) |
| Leaving the band | Clear the position and the display. Keep the layout and sheets |

**Rationale**: this turns requirement 7 and the Edge Cases "passing over without entering the band does not
fetch", "leaving the card aborts the fetch" and "leaving the card and coming back tries fetching again" directly
into state. The player's retry (5 seconds) is for an interaction that stays on the same seek bar; it does not fit
the library's interaction of leaving a card and coming back. `web/src/ui/` is the only place that both
`videoList/` (`CardMedia`) and `player/` (`VideoThumbnail`) already import (`ThumbnailBackdrop`), so putting the
hook there creates no new dependency direction. `ui/` calling the fetch functions of `api/client` follows "only
`web/src/api/` talks to the server" (ARCHITECTURE.md).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Put it in `web/src/videoList/` and import it from `player/` | Rejected: a new direction in which the player screen depends on a component shared by the library and folder screens |
| Use the `sheets[n]` URL directly as `background-image` instead of an object URL | Rejected: sheets are served with `private, no-cache` and an `ETag`, so every entry into the band sends a revalidation request and breaks acceptance criterion 5 |
| Put the layout in the list response and fetch ahead | Rejected: the server and API are out of scope |

## R-4: Releasing held sheets

**Decision**: the object URLs the hook holds are released on card unmount and also when the card leaves the
viewport (`IntersectionObserver`, `rootMargin` of one viewport). On release the layout is discarded too, and it is
fetched again on the next entry into the band.

**Rationale**: the Edge Cases say "held sheets are released when the card leaves the screen". The library keeps
adding cards with infinite scroll and does not unmount them, so releasing only on unmount would keep hundreds of
sheets (decoded bitmaps) alive.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Release only on unmount | Rejected: as above, the count grows with every scroll |
| A page-wide LRU limit (for example 20 sheets) | Rejected: can discard sheets of cards still on screen, conflicting with acceptance criterion 5 (no refetch on the same card) |

## R-5: How frames fit and the band's shape

**Decision**: `ScrubFrame` is one element laid over the thumbnail surface: a centred box with
`aspect-ratio: frameWidth / frameHeight`, `max-width: 100%` and `max-height: 100%`, over which the sheet is laid
at `columns × 100% / rows × 100%` and shifted to the R-1 position. This gives the same frame and the same fit as an
`object-contain` `<img>`, and for portrait videos the existing `ThumbnailBackdrop` (blurred thumbnail) stays
behind as it is. Switching frames has no transition (also with reduced motion).

The band `ScrubBand` is a transparent element at the bottom of the thumbnail surface (one fifth of the surface's
height), inside the `Link`, in front of the duration display and the watch-position bar (z-order), and behind the
selection check. It sits outside the media layer that scales on hover (`group-hover:scale-[1.03]`), so its width is
not affected by the scaling. The band only receives pointer enter, leave and move; clicks reach the `Link`
(requirement 9).

**Rationale**: requirement 6 asks for "the same frame and the same fit as the thumbnail", and `UI品質` says "add no
border, label or icon that signals the band". The duration display sits at the bottom right of the surface
(`bottom-2`) within the band's height, so unless the band is in front, the pointer over the display counts as
leaving the band and the frame disappears.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Draw frames on a `<canvas>` | Rejected: would own sheet decoding and drawing, and scaling blurs differently from `<img>`. A CSS background is enough |
| Crop `<img src=sheet>` with `object-fit: none` and `object-position` | Rejected: `object-position` cannot easily express offsets of whole multiples of the frame size as percentages, and the calculation would differ from the player's |

## R-6: Swapping the time display and the bar

**Decision**: while the pointer is in the band, the card's duration display becomes "scrub position / video
length", and the watch-position bar becomes a scrub-position bar. The string comes from
`t.list.card.scrubTime(position, duration)`, added to `web/src/i18n/en.ts`. The bar does not reuse the existing
`role="progressbar"` element; a separate `aria-hidden` element is shown in the same place only while in the band.
The colour, separator and end appearance are decided by the design stage's `ui-design.md`.

**Rationale**: requirement 4 and `UI品質` (reuse the existing display and add no new text styling). The
watch-position `progressbar` tells assistive technology "the fraction watched"; rewriting its value with the
pointer position would make the announcement meaningless (010 ui-design.md "Accessibility": temporary
pointer-only visual information does not interrupt the screen reader). A fixed string (` / `) cannot live outside
`web/src/i18n/` (the ESLint rule in ARCHITECTURE.md "Web layer").

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Overwrite the existing `progressbar`'s `aria-valuenow` with the scrub position | Rejected: as above, it changes what is announced |
| Show the time in a tooltip above the band (`.vv-seek-preview-time`, as in the player) | Rejected: `UI品質` says "the time reuses the existing duration display" |
