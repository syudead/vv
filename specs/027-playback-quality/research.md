# Research: Quality selection for slow networks, and a warning when playback stalls

Inherited decisions: the tech stack, the current live transcode design (reuse of probe data,
switching between copy and encode, keyframe interval, per-method encoder arguments), how playback
errors are classified and reloaded, and where screen text lives follow the canonical documents
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md),
[docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md),
[docs/design-docs/library-ui.md "Video page layout"](../../docs/design-docs/library-ui.md#7-video-page-layout),
[docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)). This file records only the decisions this
feature adds. The ffmpeg arguments were checked with `-h encoder=…` on ffmpeg 6.1.1 in the development
container.

## R-1: Quality is a `quality` parameter on the live transcode request; no new transcode path

**Decision**: Add an optional `quality` (`1080p`, `720p`, `480p`, `360p`) to
`GET /api/videos/{id}/transcode.mp4` ([contracts/transcode-quality-api.md](contracts/transcode-quality-api.md)).
Without it, behaviour is as before (original quality). A request with `quality` always encodes, even
for a video whose video stream could be copied, and starts exactly at `startMs`. The `transcode-start`
report path and `attempt` do not change (because it encodes, the report is the requested position).
The server does not remember the quality; each request decides.

**Rationale**: Requirement 3 of the parent Issue asks to "replace the current transcode only when a
quality is chosen". Making it a parameter on the same path lets seek rebuilding (`reloadAt` in
`liveOffset.ts`) and reload (`playbackRecovery`) reuse the current URL construction as is, and adds no
ledger, guest boundary or expiry machinery. Because each request decides, viewing the same video in
another tab or by another viewer at a different quality has no effect on either (Edge Case 7).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A path per quality (`/transcode-480p.mp4` and so on) | Rejected: four more paths, and more `accessRoutes` and OpenAPI `security` pairs. |
| A per-session quality setting on the server | Rejected: contradicts requirement 5 (the viewer's choice stays in the browser), and guests would need a settings endpoint. |

## R-2: Quality scales the short side of the display, and `-maxrate`/`-bufsize` caps the bitrate

**Decision**: `internal/domain` gets `TranscodeQuality` (`1080p`, `720p`, `480p`, `360p`) and a pure
function holding the per-quality values (short side, video cap in kbps, audio kbps). The values follow
the guide in requirement 3 of the parent Issue: video 5000/2500/1200/700 kbps, audio 128/128/96/64
kbps. When a quality is set, `videoEncodeArgs` in `internal/media` computes even dimensions whose short
side, in display geometry (`displayGeometry`, rotation already applied), equals the quality, emits
`scale=W:H`, and passes the existing `setsar` handling through unchanged. For a portrait video the
short side is the width, so 720p of 1080×1920 is 720×1280 (Edge Case 3).

The scale factor is the smaller of the factor to the quality's short side and the factor to the
current transcode frame (long side 3840, short side 2160 in `outputDimensions`; the frame that fits
H.264 Level 5.1's per-frame limit). Only extremely elongated videos end up with a short side below the
quality: for 1200×12000, `1080p`, `720p` and `480p` all give 384×3840, the same as the original-quality
transcode, and `360p` gives 360×3600 (the landscape 12000×1200 gives the swapped dimensions). The
per-quality bitrate cap and audio still apply, so the chosen quality is still lighter on the network.
The option rule ([R-3](#r-3-availability-of-a-quality-depends-on-the-videos-short-side-and-the-server-rejects-unavailable-qualities-with-400))
stays as in requirement 1, decided by the video's short side, and this frame does not change it.

The encoder arguments (`encoderCodecArgs`) gain a cap per method:

| Method | Arguments when a quality is set |
| --- | --- |
| `software`, `nvenc` | Keep the current constant quality (`-crf 23` / `-cq 23`) and add `-maxrate <cap>k -bufsize <cap×2>k`. Scenes with quality headroom come out lighter than the cap. |
| `qsv`, `vaapi`, `videotoolbox` | Drop the constant-quality setting (`-global_quality` / `-rc_mode CQP -qp` / `-q:v`) and use VBR: `-b:v <cap>k -maxrate <cap>k -bufsize <cap×2>k` (VAAPI adds `-rc_mode VBR`). |
| Audio | With a quality, always encoded to AAC (`-ac 2 -ar 48000`) at the quality's kbps; never copied. |
| Shared parts | Filters, `-force_key_frames` and `-movflags` do not change. |

**Rationale**: Deciding by the short side makes a quality "equally heavy" for landscape and portrait
(requirement 1, Edge Case 3). `-maxrate`/`-bufsize` are the VBV cap and keep the output's average
bitrate near the cap on every method (acceptance criterion 3). Software and NVENC can layer a cap over
constant quality, which keeps the benefit of lighter quiet scenes. On QSV, VAAPI and VideoToolbox,
"constant quality with a cap" (QVBR and similar) depends on driver support, so VBR, where the cap
reliably applies, is used. When hardware is unavailable and the transcode falls back to software,
scaling is in the shared filter and the cap is in the software arguments (Edge Case 6). Audio is made
lighter because requirement 3 says "make the audio lighter too"; copying would keep the original
256–320 kbps.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Scale by the long side | Rejected: 720p of a portrait video becomes 405×720, lighter than 720p of a landscape video. |
| Drop qualities whose scaled dimensions exceed the frame for extremely elongated videos | Rejected: narrows requirement 1's rule of offering qualities "smaller than the original's short side", and takes an option away from viewers who want it lighter through the bitrate cap alone. |
| Keep the quality's short side and exceed the frame | Rejected: 1080×10800 would have a long side over 3840, at 45,900 macroblocks per frame, over Level 5.1's 36,864. |
| Average bitrate with `-b:v` alone | Rejected: no instantaneous cap, so high-motion scenes exceed the network. |
| Leave the dimension calculation to ffmpeg's `scale=-2:480` | Rejected: ffmpeg would also have to decide orientation for portrait video, duplicating the Go calculation and its tests. |

## R-3: Availability of a quality depends on the video's short side, and the server rejects unavailable qualities with 400

**Decision**: The options are "Original" plus those of `1080p`, `720p`, `480p` and `360p` whose short
side is smaller than the short side of the video's display (`Video.width`/`height`, rotation already
applied) (requirement 1). The player builds the options with this rule; if the remembered quality is
not among them, it plays at "Original" and does not rewrite the remembered value (Edge Case 2). The
server checks the same rule and returns 400 `invalid_request` for a quality at or above the video's
short side, and for any quality on a video without dimensions.

**Rationale**: Upscaling or transcoding to the same size only makes playback heavier, and requirement
1 rules those options out. The server also rejects so that, if the player's options drift from the
canonical rule, it is noticed instead of silently starting a useless transcode. The rule is only a
dimension comparison, so a pure function in `internal/domain` and one in `web/src/player/quality.ts`
each hold it, and tests on both check the same table.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| The server silently rounds to the original dimensions | Rejected: a 360p video would be transcoded under the 480p cap while showing "480p", so the label and content disagree. |
| Put the options in the server response | Rejected: the `Video` shape grows, and a round trip is needed for a rule that depends only on dimensions. |

## R-4: The quality menu is a video.js `MenuButton` component

**Decision**: `web/src/player/qualityMenu.ts` defines a component that extends video.js `MenuButton`
and `MenuItem`, registers it with `videojs.registerComponent`, and places it before
`playbackRateMenuButton` in `controlBarChildren`. Like the playback rate's `1x`, the button text shows
the current quality briefly, and React passes the options through `player.trigger` or the component's
`setOptions`. The component reports the chosen quality with an event, and `VideoPlayer.tsx` performs
the switch ([R-5](#r-5-switching-quality-swaps-the-source-at-the-same-position-without-recreating-the-player)).
`rateMenuOpen` in `playerControls.ts` looks for `.vjs-menu.vjs-lock-showing`, so it applies to the
quality menu as is (Esc while the menu is open only closes the menu).

**Rationale**: The parent Issue's `UI品質` asks for "the same weight, size, spacing and type as the
playback rate". The playback rate is video.js `PlaybackRateMenuButton`, so a component on the same base
automatically gets the same look (the `.vjs-menu` rules in `index.css`), the same opening and closing
by pointer, press and keyboard, and the same Esc handling. Building it with React `Popover` would mean
rebuilding all of that and then keeping it aligned with the video.js components to the pixel. The
"Converting for playback" indicator is a React popover because it is an explanation opened by a press,
not a menu; that does not change this decision.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Portal `web/src/ui/Menu.tsx` into the control bar | Rejected: the reason above. |
| Mix qualities into the options of `PlaybackRateMenuButton` | Rejected: mixes the meanings of speed and quality, and no accessible name can be given. |

## R-5: Switching quality swaps the source at the same position without recreating the player

**Decision**: `PlaybackAttempt` gains `quality`. If `quality` is anything other than "Original", the
route is `transcode`; for "Original", the current rule applies (`direct` if `playable`, otherwise
`transcode`). The switch is built like the switch from direct playback to transcoding (the fallback in
`handleFailure`): read the logical position, rebuild `attempt` with the new quality and route, replace
`player.src`, and on `canplay` keep playing if it was playing and stay paused if it was paused
(requirement 4).

- A transcode source carries the position in the URL's `startMs`, but a direct source
  (`setDirectSource`; a `playable` video switched back to "Original") carries no position and loads
  from 0. So, as `finishRecovery` does on reload, when metadata arrives the player seeks to the
  logical position with `player.currentTime` and then restores the play intent (requirements 4 and 6).
- Each switch advances a generation, and stale `canplay` and metadata handlers (an old source must not
  seek the new one) and stale reports (`attempt`) are discarded.
- On consecutive changes, replacing `player.src` makes the browser abort the previous request, and the
  server's transcode stops when the request is cancelled (Edge Cases 4 and 10).
- `liveSource` carries `vvQuality`, and `reloadAt` in `liveOffset.ts` and `reload` in
  `VideoPlayer.tsx` carry it over, so seeking and reloading keep the quality (requirement 7).
- A failure to start the switched transcode is reported by type through the current error path
  (`playbackRecovery`) unchanged, and can be retried from the position where it failed (Edge Case 5).
- The chosen quality is written to `localStorage` by `web/src/preferences/playbackQuality.ts` at the
  moment it is chosen (built like the volume; requirement 5).

**Rationale**: Recreating the player (advancing `attempt.key` in `VideoPage`) returns to the poster,
hides the control bar, and also rebuilds the state inside the fullscreen element (the popover
container). The fallback already implements "replace the source keeping the position and the play
intent", and going through the same path lets the tests be shared too.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Recreate the player from `VideoPage` | Rejected: the reason above. |
| Swap on the component side using video.js `sourceset` | Rejected: the `attempt` position and intent live in `VideoPlayer.tsx`, so the switch decision would be held in two places. |

## R-6: Stalls are counted as `waiting`/`playing` pairs, excluding waits while settling

**Decision**: `web/src/player/stallMonitor.ts` holds a pure state machine.

- A data wait runs from a `waiting` received during playback to the next `playing`, and records its
  start time.
- If 3 or more data waits start within a 60-second window, or one data wait exceeds 10 seconds,
  playback is judged "interrupted by a slow connection" (requirement 9).
- After a seek (`seeking`), a source set (`loadstart`; includes the first load and quality switches)
  or a play start (`play`), the wait until the next `playing` is not counted.
- While paused (`paused`) and while reloading after a network failure (`recovering`), nothing is
  counted, and counting restarts (Edge Case 8).
- For the 10-second rule, `VideoPlayer.tsx` sets a 10-second timer when a data wait starts and judges a
  stall if the wait is still going.
- The judgement reaches `VideoPage` as `PlayerStatus.stalled`.
- An `error` that leads to a reload is not a stall; counting restarts when `recovering` begins.

**Rationale**: `waiting` also fires right after a seek and at the start of loading, so counting it
directly would count the waits the requirement says not to count. The single rule "don't count until
the next `playing`" handles the three exclusions (seek, start and quality switch) in the same way. A
pure state machine lets window and count tests be written by passing timestamps only.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Estimate speed by watching the remaining `buffered` | Rejected: turns into a network speed meter and heads towards automatic switching, which is out of scope. |
| Judge by the interval of `progress` events | Rejected: varies widely between browsers. |

## R-7: The warning is a separate layer from the status overlay container, hidden while a status layer shows

**Decision**: The warning is one small bar at the top edge of the player, separate from the status
displays (loading, failure, ended, centre controls) inside `VideoPage`'s container
(`data-overlay-layer`).

- It is `role="status"`, and everything except the close button is `pointer-events-none`, so it does
  not block the controls beneath (requirements 9 and 10).
- It does not show while the failure, ended, reconnecting or up-next status layer shows, and it
  disappears on ended, failure and moving to another video (Edge Case 9).
- It is not hidden during the data-wait loading display (`LoadingOverlay`, shown when `waiting` sets
  `loading`); the two show side by side. The stall judgement is made during a data wait (when one wait
  exceeds 10 seconds), so hiding it during loading would keep the warning invisible for as long as the
  stall lasts and show it only after playback resumes. The loading display is a small spinner in the
  centre and the warning is a bar at the top edge, so they do not overlap.
- During loading for a play start, seek, quality switch or reload,
  [R-6](#r-6-stalls-are-counted-as-waitingplaying-pairs-excluding-waits-while-settling) does not count,
  so no new warning appears then.
- `VideoPage` keeps the dismissed record per video id; it does not show again during playback of the
  same video (including retries after a failure), and the record clears when another video opens
  (requirement 10).
- The warning has no action that switches quality, and no automatic downgrade.
- The design stage's `ui-design.md` decides the text, size and how it folds in a narrow frame.

**Rationale**: The current container holds "only one layer at a time" and assumes a centred position
([library-ui.md](../../docs/design-docs/library-ui.md#7-video-page-layout)). The warning neither stops
playback nor blocks the controls, so putting it in that container would make it mutually exclusive
with the centre controls, which violates requirement 9. A separate layer leaves the container's
exclusivity rule unchanged, and CSS sets the stacking order (more subdued than the status layers).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Show it in a screen corner with `web/src/ui/Toast.tsx` | Rejected: invisible in fullscreen, and outside the player it is not tied to playback. |
| Make it one of the container's layers | Rejected: the reason above. |

## R-8: Bitrate and dimensions are checked by Go tests with ffmpeg

**Decision**: A test with ffmpeg in `internal/media/transcode_test.go` (built like
`TestTranscodeRotated4K…`) transcodes a high-motion synthetic input (`testsrc2` or similar) at `480p`
and checks that the output video's short side is 480, the average video bitrate is at most 1200 kbps ×
1.2, and the audio is around 96 kbps. The average bitrate is the sum of packet sizes per stream from
`ffprobe -show_entries packet=pts_time,size`, divided by the difference between the first and last
`pts_time` (fragmented MP4 may not report `bit_rate` in `format` or `stream`; like the existing keyframe
interval test, it reads packets). For a portrait input it also checks that the width is 480. The
hardware methods' caps are not available in CI, so they are checked on real hardware with
[quickstart.md](quickstart.md).

**Rationale**: Acceptance criteria 2 and 3 are facts about the output's content; checking the argument
strings alone does not show that the cap actually takes effect. Go tests with ffmpeg already check
rotation, 4K and keyframe interval in the same way. e2e can see dimensions through `video.videoHeight`
but cannot see bitrate.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Argument tests only | Rejected: the reason above. |
| Measure response size in e2e | Rejected: depends on how much the browser reads ahead. |
