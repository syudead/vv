# Implementation Plan: Quality selection for slow networks, and a warning when playback stalls

**Branch**: `feature/027-playback-quality` | **Parent Issue**: #521

**Input**: The parent Issue. It is this feature's specification.

## Summary

The playback screen's control bar gets a quality choice. Choosing anything other than "Original"
plays the video through live transcoding (`GET /api/videos/{id}/transcode.mp4`), even when it could
play directly, with the video's short side scaled to that quality and the bitrate capped. The browser
remembers the chosen quality, and it survives switching during playback, seeking and reloading. When
playback is judged to be interrupted by a slow connection, a warning that only informs appears over
the player.

- **Transcoding**: the request takes `quality` as a parameter, a pure function in `internal/domain`
  holds each quality's short side and caps, and `internal/media` builds the scaling and per-method cap
  arguments
  ([research.md R-1](research.md#r-1-quality-is-a-quality-parameter-on-the-live-transcode-request-no-new-transcode-path),
  [R-2](research.md#r-2-quality-scales-the-short-side-of-the-display-and--maxrate-bufsize-caps-the-bitrate),
  [contracts/transcode-quality-api.md](contracts/transcode-quality-api.md)).
- **Options and rejection**: only qualities smaller than the video's short side are offered, and the
  server returns 400 by the same rule
  ([R-3](research.md#r-3-availability-of-a-quality-depends-on-the-videos-short-side-and-the-server-rejects-unavailable-qualities-with-400)).
- **Player**: the quality menu is a video.js `MenuButton` component, and switching replaces the source
  at the same position without recreating the player
  ([R-4](research.md#r-4-the-quality-menu-is-a-videojs-menubutton-component),
  [R-5](research.md#r-5-switching-quality-swaps-the-source-at-the-same-position-without-recreating-the-player)).
- **Warning**: a pure state machine that counts `waiting`/`playing` pairs, and a layer separate from
  the status displays
  ([R-6](research.md#r-6-stalls-are-counted-as-waitingplaying-pairs-excluding-waits-while-settling),
  [R-7](research.md#r-7-the-warning-is-a-separate-layer-from-the-status-overlay-container-hidden-while-a-status-layer-shows)).
- **Screen design**: the parent Issue has the `ui` label, so the design stage's `ui-design.md` decides
  the look and interaction of the menu, the indicator and the warning. This plan decides only structure
  and contracts.

## Technical Context

**Canonical definitions**:

| Area | Source |
| --- | --- |
| Boundaries, dependency direction, the authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| Current live transcoding | [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md), [docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md), [docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md), [internal/media/transcode.go](../../internal/media/transcode.go) (`buildTranscodeArgs`, `videoEncodeArgs`, `encoderCodecArgs`, `outputDimensions`), [internal/httpapi/transcode.go](../../internal/httpapi/transcode.go), [internal/domain/live_transcode.go](../../internal/domain/live_transcode.go) |
| Current player | [docs/design-docs/library-ui.md "Video page layout"](../../docs/design-docs/library-ui.md#video-page-layout), [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx) (`controlBarChildren`, `TranscodeIndicator`, the `handleFailure` fallback, `reload`), [web/src/player/playbackAttempt.ts](../../web/src/player/playbackAttempt.ts), [web/src/player/liveOffset.ts](../../web/src/player/liveOffset.ts), [web/src/player/playbackRecovery.ts](../../web/src/player/playbackRecovery.ts), [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx) (the layer container), [web/src/preferences/playbackVolume.ts](../../web/src/preferences/playbackVolume.ts) (the model for remembering a preference) |
| API source of truth and error shape | [api/openapi.yaml](../../api/openapi.yaml), [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md) |
| Screen text | [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md) |
| Check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`, `task test-e2e`) |

**Feature-specific context**:

- No Go or npm dependency is added. No SQLite table or setting is added either (only the browser's
  `localStorage` holds the quality; requirement 5 and the out-of-scope list of the parent Issue).
- Reloading after a lost connection and reporting failures landed in `main` with #520. This feature
  does not change that path, and makes quality switches and reloads both go through the same
  `liveSource`.
- The "Original" transcode keeps today's arguments character for character (requirement 2). The
  existing argument tests guard that.
- CI has no hardware encoder. The per-method caps are checked for software by Go tests with ffmpeg,
  and for hardware on real machines with [quickstart.md](quickstart.md)
  ([R-8](research.md#r-8-bitrate-and-dimensions-are-checked-by-go-tests-with-ffmpeg)).
- Playback on a throttled network cannot be automated, so the warning is checked by unit tests of the
  state machine and the manual check in [quickstart.md](quickstart.md).

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"): pass.

  | Package | What this feature puts there |
  | --- | --- |
  | `internal/domain` | The quality value, each quality's short side and caps, and the rule for whether a quality is available for a video's dimensions. Pure functions only. |
  | `internal/media` | The scaling and cap arguments. The meaning of a quality comes from domain. |
  | `internal/httpapi` | Parsing `quality` and deciding the 400 only. Carried on `LiveTranscodeRequest.Quality`. |
  | `web/src/player`, `web/src/preferences` | The option rule, remembering, switching, and the stall judgement. Independent of server state. |

- **API source of truth** (ARCHITECTURE.md): pass. `quality` is added to `api/openapi.yaml` and
  `task generate` is run; the generated code is not hand-edited (AGENTS.md).
- **Authentication boundary** (the authentication paragraph of ARCHITECTURE.md, requirement 8): pass.
  No path is added, and `GET /api/videos/{id}/transcode.mp4` stays "guests too". The match in
  `openapi_routes_test.go` does not change.
- **Screen text lives in the catalogue** (`docs/design-docs/i18n.md`, gosmopolitan in `.golangci.yml`):
  pass. Text for the menu, the indicator and the warning goes in `web/src/i18n/en.ts`, and the
  server's `message` is English.
- **Design documents describe the current state** (the policy in `docs/design-docs/index.md`,
  core-beliefs.md): pass. Each unit updates `docs/design-docs/playback-quality.md` (new),
  `library-ui.md` and ARCHITECTURE.md in the same PR.
- **Domain events** (the events paragraph of ARCHITECTURE.md): not applicable. Neither quality nor the
  warning emits events.
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): not applicable. Nothing is
  stored on the server.

The verdicts are the same after Phase 1. There is no violation for Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/027-playback-quality/
├── plan.md                              # This file
│                                        # No spec.md — the parent Issue is the specification
├── research.md                          # Request parameter, scaling and caps, rejection rule, menu component, switching, stall judgement and layer, checks
├── quickstart.md                        # Playback on a throttled network, and real-hardware checks of bitrate and hardware methods
├── ui-design.md                         # Created by the design stage (`ui` label)
└── contracts/
    └── transcode-quality-api.md         # quality on transcodeVideo, and the per-quality guarantees
```

No `data-model.md`: no entity or stored value is added, and the contract holds the table of
per-quality values.

### Source Code

**Affected boundaries**:

- `internal/domain`: `TranscodeQuality` and the per-quality values, the availability rule,
  `LiveTranscodeRequest.Quality`
- `internal/media`: the quality branch in `buildTranscodeArgs`, `videoEncodeArgs`, `encoderCodecArgs`
  and `outputDimensions` (no copy, scaling, caps, audio)
- `api/openapi.yaml`, `internal/httpapi/transcode.go` (parsing `quality` and the 400)
- `web/src/api/client.ts` (`quality` in `transcodeUrl`), `web/src/preferences`, `web/src/player`
  (`playbackAttempt.ts`, `liveOffset.ts`, `VideoPlayer.tsx`, `VideoPage.tsx`, `StatusOverlays.tsx`,
  `playerControls.ts`), `web/src/i18n/en.ts`, `web/e2e/playback.e2e.ts`
- `docs/design-docs/` (the new design document, `library-ui.md`, `index.md`), `ARCHITECTURE.md`

**New paths**:

- `internal/domain/transcode_quality.go`
- `web/src/preferences/playbackQuality.ts`
- `web/src/player/quality.ts` (pure functions for the options and the effective quality)
- `web/src/player/qualityMenu.ts` (the video.js component)
- `web/src/player/stallMonitor.ts` (the stall judgement state machine)
- `web/src/player/StallWarning.tsx`
- `docs/design-docs/playback-quality.md` (the current design of quality transcoding, selection,
  switching and the warning; listed in `docs/design-docs/index.md`)

**Structure decision**: follows the existing layout ([ARCHITECTURE.md](../../ARCHITECTURE.md)). The
meaning of a quality (short side, caps, availability) lives in `internal/domain`, and `internal/media`
holds only the mapping to arguments. Having httpapi know the short sides and caps was rejected because
the rejection rule and the source of the arguments would split into two places. On the player side
the same rule is confined to pure functions in `web/src/player/quality.ts` and is not brought into
`VideoPlayer.tsx`.

## Implementation Work

### Add quality to live transcoding: scale the video's short side to the chosen quality and cap the bitrate

**Scope**: the changes in `internal/domain` and `internal/media`, and the design document.

- `internal/domain/transcode_quality.go`: `TranscodeQuality` (`1080p`, `720p`, `480p`, `360p`),
  `ParseTranscodeQuality`, each quality's short side, video cap in kbps and audio kbps, and a pure
  function deciding whether a quality is available for a video's display dimensions
  ([contracts/transcode-quality-api.md, Per-quality transcode guarantees](contracts/transcode-quality-api.md#per-quality-transcode-guarantees),
  [research.md R-2](research.md#r-2-quality-scales-the-short-side-of-the-display-and--maxrate-bufsize-caps-the-bitrate),
  [R-3](research.md#r-3-availability-of-a-quality-depends-on-the-videos-short-side-and-the-server-rejects-unavailable-qualities-with-400)).
  `LiveTranscodeRequest.Quality`.
- `internal/media/transcode.go`: with a quality, encode the video instead of copying it, compute from
  the display dimensions even dimensions whose short side equals the quality and emit `scale`, add the
  per-method cap (software and NVENC: `-maxrate`/`-bufsize` on top of constant quality; QSV, VAAPI and
  VideoToolbox: VBR with `-b:v`/`-maxrate`/`-bufsize`), and encode audio to AAC at the quality's kbps.
  Without a quality the arguments do not change.
- Write the transcoding section of `docs/design-docs/playback-quality.md`, list it in
  `docs/design-docs/index.md`, and add the caps used with a quality to "Per-method arguments" in
  `hardware-encoding.md`.

**Dependencies**: None.

**Acceptance**: the following checks exist, and `task check` and `task check-docs` pass.

- `internal/domain` tests: the short sides and kbps of the four qualities match the contract table.
  An unknown string cannot be parsed. For 1920×1080 only `720p`, `480p` and `360p` are available and
  `1080p` is not. The same for 1080×1920 (portrait). None is available for 640×360.
- Argument tests: the arguments of a request without a quality are character for character the same
  as the current test expectations. At `480p`, 1920×1080 gives `scale=854:480` and 1080×1920 gives
  `scale=480:854` (checking the even-rounding rule); the extremely elongated 1200×12000 gives
  `scale=384:3840` and 12000×1200 gives `scale=3840:384` (within the current transcode frame; R-2). A
  video that could be copied still gets `-c:v libx264`, `-maxrate 1200k -bufsize 2400k` and
  `-c:a aac … -b:a 96k`. Each hardware method has the R-2 cap arguments. `-force_key_frames` and
  `-movflags` do not change.
- Tests with ffmpeg (R-8): transcoding a high-motion synthetic input at `480p` gives an output short
  side of 480 and an average video bitrate of at most 1.2 × 1200 kbps; a portrait input gives a width
  of 480. The existing tests with ffmpeg (rotation, aspect ratio, 4K, keyframe interval, MOV) still
  pass.

### Add `quality` to the live transcode API and accept only qualities smaller than the video's short side

**Scope**: the contract and the route.

- Add `quality` to `transcodeVideo` in `api/openapi.yaml`
  ([contracts/transcode-quality-api.md, `quality` on `GET /api/videos/{id}/transcode.mp4`](contracts/transcode-quality-api.md#quality-on-get-apivideosidtranscodemp4))
  and run `task generate`.
- `internal/httpapi/transcode.go`: parse `quality`; if it is not available for the video's
  `Width`/`Height`, return 400 `invalid_request`, otherwise carry it on
  `LiveTranscodeRequest.Quality`. Add the quality to the start log.
- The live transcoding paragraph of ARCHITECTURE.md, and the API section of
  `docs/design-docs/playback-quality.md`.

**Dependencies**: `Add quality to live transcoding: scale the video's short side to the chosen quality and cap the bitrate`.

**Acceptance**: the following checks exist, `task check` and `task check-docs` pass, and
`task generate` produces no diff.

- `internal/httpapi/transcode_test.go` (helper process): a `quality=480p` request starts with
  `Quality` set, and the arguments contain `scale` and `-maxrate 1200k`. A request without `quality`
  starts with copy as before. A quality at or above the video's short side, and a video without
  dimensions, return 400 and start no transcode. A value not in the enum returns 400. A `quality`
  request with `startMs` and `attempt` gets `startMs` back from `transcode-start`.
- `internal/httpapi/guest_test.go`: a guest can transcode a public video with `quality` (acceptance
  criterion 8). `openapi_routes_test.go` passes.

### Start live transcoding at the remembered quality, and keep it across seeks and reloads

**Scope**: how the player chooses its route, and carrying the quality over. No menu yet.

- `web/src/preferences/playbackQuality.ts`: reading and writing `localStorage` (built like
  `playbackVolume.ts`; a broken value or unavailable storage means "Original").
- `web/src/player/quality.ts`: the rule that builds options from the video's `width`/`height`, and the
  rule that falls back to "Original" when the remembered quality is unavailable (R-3, Edge Cases 1 and
  2).
- `web/src/player/playbackAttempt.ts`: add `quality`; anything other than "Original" makes the route
  `transcode`. Pass `quality` through `transcodeUrl` in `web/src/api/client.ts` and `liveSource` in
  `liveOffset.ts`, and have `reloadAt` and `reload` in `VideoPlayer.tsx` carry it over (R-5,
  requirement 7).
- `VideoPlayer.tsx`: on creation, read the remembered quality and pass it to `createPlaybackAttempt`.
  Pass the quality to `TranscodeIndicator` so that, for anything other than "Original", its text (and
  the popover explanation) says it is transcoding to the chosen quality (requirement 6; the look is in
  `ui-design.md`). Add the text to `web/src/i18n/en.ts`.
- The options and remembering sections of `docs/design-docs/playback-quality.md`.

**Dependencies**: `Add quality to the live transcode API and accept only qualities smaller than the video's short side`.

**Acceptance**: this unit changes the quality indicator, so the look is reviewed against
`ui-design.md`. The following checks exist and `task check` passes.

- `playbackQuality.test.ts`: save and read back; a broken value and unavailable storage give
  "Original".
- `quality.test.ts`: the options for 1920×1080 are `720p`, `480p` and `360p`; the same for 1080×1920;
  empty for 640×360; empty without dimensions. 1200×12000 shows all four (requirement 1 decides by the
  short side alone). A remembered `480p` becomes "Original" for a 640×360 video, and the stored value
  does not change.
- `playbackAttempt.test.ts`: at `480p` the route of a `playable` video becomes `transcode` and
  `sourceOffsetMs` is the position. "Original" behaves as before.
- `liveOffset.test.ts`: the URL of a `480p` source rebuilt by an unbuffered seek contains
  `quality=480p`.
- `VideoPlayer.test.tsx`: opening a `playable` video with `480p` remembered gives a first `src` of
  `transcode.mp4?…quality=480p`, and the indicator says it is a 480p transcode. The reload URL after a
  network failure also contains `quality=480p`.
- `web/e2e/playback.e2e.ts`: with `480p` in `localStorage`, opening a 1080p video that could play
  directly starts with transcoding and `videoHeight` 480, still 480 after a seek (acceptance criteria
  5 and 7).

### Switch quality during playback from the quality menu in the control bar

**Scope**: the menu component and switching. The look, order and text are in `ui-design.md`.

- `web/src/player/qualityMenu.ts`: register a component extending video.js `MenuButton`/`MenuItem`
  and place it before the playback rate in `controlBarChildren` (R-4). Options come from the
  `quality.ts` rule; when the only option is "Original", the menu makes it clear that no quality can be
  chosen (Edge Case 1). The button shows the current quality. The accessible name and tooltip are
  built from the catalogue, as `playerDictionary` does.
- `VideoPlayer.tsx`: on selection, write to `playbackQuality.ts`, rebuild `attempt` with the new
  quality and route at the logical position, replace the source, and keep playing if playing or stay
  paused if paused (R-5, requirement 4). Switching back to "Original" returns a `playable` video to
  direct playback (requirement 6). On consecutive changes only the last quality remains (Edge Case 4).
  Confirm with a test that `rateMenuOpen` in `playerControls.ts` also applies to the quality menu, and
  rename it to match what it does.
- The control bar entry in "Video page layout" of `docs/design-docs/library-ui.md`, and the
  switching section of `playback-quality.md`.

**Dependencies**: `Start live transcoding at the remembered quality, and keep it across seeks and reloads`.

**Acceptance**: this unit changes the screen, so the look and interaction (mouse, touch, keyboard,
fullscreen) are reviewed against `ui-design.md` at 360px, 768px and 1280px widths and in a portrait
video frame. The following checks exist and `task check` passes.

- `qualityMenu.test.ts` (real video.js): for a 1080p video the items are "Original", "720p", "480p"
  and "360p", without `1080p` (acceptance criterion 1). For a video of 360p or less it is clear that
  no quality can be chosen. Choosing an item reports the selection, and `rateMenuOpen` (after the
  rename) tells open from closed.
- `VideoPlayer.test.tsx`: choosing `480p` while playing continues playback from the same position with
  a `quality=480p` source; choosing it while paused stays paused at the same position (acceptance
  criteria 2 and 4). Switching back to "Original" returns a `playable` video to the `stream` source,
  and when metadata arrives it seeks to the switch position and continues (it does not restart from 0;
  acceptance criteria 4 and 6). Metadata from an old source does not seek the new source. Changing
  twice in a row leaves only the last quality's source. An error in the switched transcode is reported
  through the current path, and the retry position is the switch position.
- `web/e2e/playback.e2e.ts`: choosing `480p` from the control bar while playing makes `videoHeight`
  480 and the current time does not go back. The same as a guest (acceptance criterion 8). The
  pseudo-locale check (`expectCatalogTextOnly`) passes.

### Show an inform-only warning over the player when a slow connection interrupts playback

**Scope**: the stall judgement and the warning. The look, placement and text are in `ui-design.md`.

- `web/src/player/stallMonitor.ts`: the R-6 state machine (3 times in a 60-second window, or once for
  10 seconds; nothing counted right after a seek, a source set or a play start, nor while paused or
  reloading).
- `VideoPlayer.tsx`: pass `waiting`, `playing`, `seeking`, `loadstart`, `play`, `pause` and
  `recovering` to the state machine, set the 10-second timer, and report the judgement through
  `PlayerStatus.stalled`.
- `web/src/player/StallWarning.tsx` and `VideoPage.tsx`: show it in a layer separate from the status
  displays; do not show it while the failure, ended, reconnecting or up-next layer shows (show it
  alongside the data-wait loading display); keep the dismissed record per video id (R-7, requirements
  9 and 10, Edge Case 9). No action that switches quality. Add the text to `web/src/i18n/en.ts`.
- Add the warning layer to "Video page layout" in `docs/design-docs/library-ui.md`, and the
  warning section of `playback-quality.md`.

**Dependencies**: None.

**Acceptance**: this unit changes the screen, so the look and interaction are reviewed against
`ui-design.md` at 360px, 768px and 1280px widths and in fullscreen, and the results of steps 9 and 10
of [quickstart.md](quickstart.md) are recorded in the PR body. The following checks exist and
`task check` passes.

- `stallMonitor.test.ts`: 3 times within 60 seconds triggers the judgement, 2 times does not. One wait
  61 seconds ago is not counted. One wait over 10 seconds triggers it. The first wait right after a
  seek, a source set or a play start is not counted. Waits while paused and while reloading are not
  counted, and counting restarts.
- `VideoPlayer.test.tsx`: 3 `waiting` events within 60 seconds during playback deliver
  `stalled: true` to `onStatus`, and playback does not stop. A single `waiting` that lasts 10 seconds
  without a `playing` delivers `stalled: true` during the wait. A `waiting` right after a seek is not
  counted.
- `VideoPage.test.tsx`: `stalled` shows the warning, with `role="status"` and no action that switches
  quality. Dismissing it removes it, and it does not show again for the same video. Moving to another
  video clears the dismissed record. The warning does not show while the failure, ended or
  reconnecting layer shows. When data-wait loading (`loading`) and `stalled` occur together, both the
  loading display and the warning are visible (R-7). The control bar and the centre controls can be
  pressed (acceptance criteria 9–11).
