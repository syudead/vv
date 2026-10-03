# Playback quality

- Status: adopted
- Scope: per-quality downscaling and bitrate caps for live transcoding
  (`GET /api/videos/{id}/transcode.mp4`), choosing and switching quality on the
  playback screen, and the warning shown when playback stalls on a slow
  connection
- Background: [specs/027-playback-quality/](../../specs/027-playback-quality/plan.md)
  (parent Issue #521)

This document covers what a live transcode with a quality changes. Starting a
live transcode (preparing the probe, switching between copy and encode) is in
[live-transcode-seek.md](live-transcode-seek.md), and the video encoder choice
and per-encoder arguments are in [hardware-encoding.md](hardware-encoding.md).

## Transcoding

### Context

On a slow connection, a live transcode at the original quality (copying the
video when possible, and constant quality when encoding) exceeds the connection
speed and playback stalls. When the viewer picks a quality, the video
dimensions, the video bitrate and the audio bitrate all have to come down
together.

### Decision

A quality is a `domain.TranscodeQuality` (`1080p`, `720p`, `480p`, `360p`);
pure functions in `internal/domain/transcode_quality.go` hold the values.
`ParseTranscodeQuality` does not accept unknown strings.

| Quality | Display short side | Video cap (`-maxrate`) | `-bufsize` | Audio (AAC) |
| --- | --- | --- | --- | --- |
| `1080p` | 1080 | 5000 kbps | 10000 kbps | 128 kbps |
| `720p` | 720 | 2500 kbps | 5000 kbps | 128 kbps |
| `480p` | 480 | 1200 kbps | 2400 kbps | 96 kbps |
| `360p` | 360 | 700 kbps | 1400 kbps | 64 kbps |

A quality is available for a video only when its short side is smaller than the
video's display short side (with rotation applied), and none is available for a
video without dimensions (`TranscodeQuality.Available`). Upscaling or
transcoding to the same size only makes the stream heavier.

When `domain.LiveTranscodeRequest.Quality` is empty, the transcode arguments
are byte-for-byte the same as before qualities existed. With a quality, the
transcode in `internal/media` does the following:

- Video is always encoded, even when it could be copied. Copy is not attempted,
  so a mid-file transcode starts exactly at the requested position.
- Dimensions come from the display dimensions (`displayGeometry`) through
  `qualityDimensions`, which emits `scale=W:H`. The scale factor is the smaller
  of "the ratio to the quality's short side" and "the ratio to the current
  transcode frame (long side 3840, short side 2160)", and width and height are
  each rounded to the nearest even number. Because the short side decides, a
  portrait 1080×1920 at `480p` becomes 480×854, the same weight as landscape.
  Only extremely elongated videos whose long side exceeds the frame get a short
  side smaller than the quality (1200×12000 becomes 384×3840 at `1080p`, `720p`
  and `480p`, and 360×3600 at `360p`). The existing `setsar` handling passes
  through unchanged and keeps the display aspect ratio.
- The encoder gets the cap from the table. Software and NVENC put the cap on
  top of constant quality; QSV, VAAPI and VideoToolbox use capped VBR
  ([encoder arguments](hardware-encoding.md#encoder-arguments)). When hardware
  is unusable and the transcode falls back to software, it uses the same
  dimensions and cap.
- Audio is never copied; it is always encoded to AAC (`-ac 2 -ar 48000`) at the
  table's kbps. A copy would keep the original 256–320 kbps.
- H.264 High, Level 5.1, 4:2:0 8-bit, `-force_key_frames` (every 2 seconds of
  output time), `-movflags` and fps handling are the same as for a transcode
  without a quality.

A Go test with ffmpeg (`TestTranscodeQualityCapsBitrateWithFFmpeg`) verifies the
cap: it transcodes full-frame noise at `480p` and checks that the output short
side is 480 and the average video bitrate (the sum of packet sizes divided by
the difference between the first and last `pts_time`) is at most 1.2 times
1200 kbps. CI cannot verify the caps of hardware encoders; they are verified on
real hardware with the steps in
[quickstart.md](../../specs/027-playback-quality/quickstart.md).

### Alternatives

- **Scale by the long side**: a portrait video at `720p` would become 405×720,
  lighter than a landscape `720p`.
- **Average bitrate with `-b:v` only**: no instantaneous cap, so high-motion
  scenes exceed the connection.
- **Let ffmpeg compute dimensions with `scale=-2:480`**: ffmpeg would also need
  to know about portrait orientation, duplicating the Go calculation and its
  tests.
- **Keep the quality's short side and exceed the frame**: 1080×10800 exceeds the
  per-frame limit of H.264 Level 5.1.

## API

### Context

The viewer picks the quality and can switch it during playback. Adding routes or
making the server remember the quality would mean tracking separately which
quality goes with which seek or resume request.

### Decision

`GET /api/videos/{id}/transcode.mp4` takes an optional `quality` (`1080p`,
`720p`, `480p`, `360p`)
([contracts/transcode-quality-api.md §1](../../specs/027-playback-quality/contracts/transcode-quality-api.md#1-get-apivideosidtranscodemp4-の-quality)).
The server does not remember the quality; each request decides it from
`quality`.

- Without `quality` the quality is the original, and the transcode is unchanged.
- `internal/httpapi/transcode.go` parses the value with
  `domain.ParseTranscodeQuality` and checks `TranscodeQuality.Available` with
  the video's `Width` and `Height` (display dimensions). A value not in the
  enumeration, a quality at or above the video's short side, or a video without
  dimensions returns 400 `invalid_request` (with an English `message`) and does
  not start a transcode.
- An available quality goes into `LiveTranscodeRequest.Quality`, and the start
  log (Debug) carries `quality`. A request without a quality logs
  `quality=original`.
- `startMs` and `attempt` combine as before. A transcode with a quality encodes
  video and starts exactly at `startMs`, so `transcode-start` returns the same
  value as `startMs`.
- The response shape (fragmented MP4, `Cache-Control: no-store`) and the error
  shape are unchanged. The boundary stays a guests-too route (public videos
  only).

### Alternatives

- **A route per quality**: the start position ledger and the public boundary
  would be duplicated for each route.
- **The server remembers the quality per viewer**: guests have no viewer
  identity, and the request URL alone would no longer determine the output.
- **Silently map an unavailable quality to the original or the largest
  quality**: the selection on screen and the actual quality would disagree.

## Options and remembered quality

### Context

Quality is chosen to suit the viewer's connection, so it is decided per browser,
not per video. Once chosen, it has to stay in effect for the next video opened,
after a seek, and after a reload following a network failure. Meanwhile the
server rejects a quality at or above the video's short side with 400 (see
[API](#api)).

### Decision

- `web/src/preferences/playbackQuality.ts` keeps the chosen quality in
  `localStorage` under `vv.playback-quality.v1` as a JSON string (`"480p"`,
  `"original"`). It is built like volume (`playbackVolume.ts`): when the saved
  value is missing, broken, not in the enumeration, or storage is unavailable,
  playback uses the original quality (`"original"`). It is not sent to the
  server.
- `qualityOptions` in `web/src/player/quality.ts` builds the options from the
  video's `width` and `height`: only qualities smaller than the short side,
  largest first, and none for a video without dimensions. The rule is the same
  short-side comparison as the server's `TranscodeQuality.Available`; downscaling
  forced by the transcode frame (1200×12000 and the like) is not considered.
- For a video whose options do not include the remembered quality (`480p` on a
  360p video, for example), `effectiveQuality` decides to play the original
  quality. The remembered value is not rewritten, so the next larger video plays
  at that quality again.
- The player (`VideoPlayer.tsx`) reads the remembered quality once when it is
  created and passes it to `createPlaybackAttempt`. When `PlaybackAttempt.quality`
  is anything but the original, the route is a transcode even for a video that
  can play directly, and `sourceOffsetMs` is the resume position. With the
  original quality, as before, only videos that can play directly play
  directly.
- The transcode source keeps the quality. `transcodeUrl` and `liveSource` put
  `quality` in the URL and in the source's `vvQuality`; an unbuffered seek
  (`reloadAt` in `liveOffset.ts`) rebuilds with the source's quality, and a
  reload after a network failure (`reload` in `VideoPlayer.tsx`) rebuilds with
  the attempt's quality. Neither falls back to a request without a quality.
- The transcode indicator keeps its existing text for the original quality; for
  a chosen quality it reads `Converting to 480p` with an explanation that the
  video is downscaled and how to go back
  ([ui-design.md, Control bar: transcode indicator](../../specs/027-playback-quality/ui-design.md#control-bar-transcode-indicator)).
  Quality names are not translated.

### Alternatives

- **The server remembers the quality**: guests have no viewer identity (see
  [API](#api)).
- **Overwrite an unavailable remembered quality with the original**: opening one
  small video would lose the quality chosen for the connection.
- **Put the options in the server response**: the rule is only a dimension
  comparison, and this would add to the `Video` shape and to round trips.

## Switching

### Context

Connection speed changes during playback. If every quality change restarted
from the beginning or from a stopped state, changing quality would itself
interrupt viewing.

### Decision

- Quality is chosen from the quality menu on the control bar
  (`web/src/player/qualityMenu.ts`). A component inheriting from video.js
  `MenuButton` and `MenuItem` is registered as `QualityMenuButton` and placed
  before playback speed in `controlBarChildren`. The items are the original
  quality (`Original (1080p)`) and the qualities from `qualityOptions`; a video
  with no selectable quality shows a non-interactive note row
  (`No smaller sizes for this video`). The button label is the current quality
  (for the original quality, the video's short side; without dimensions,
  `Orig`), and its text comes from the catalog.
- The component only reports the chosen quality through the `vvqualityselect`
  event; `VideoPlayer.tsx` performs the switch. The options and the current
  quality are passed to the component through `setQualityMenu`.
- The chosen quality is written to `playbackQuality.ts` at that moment. Choosing
  the current quality only writes it and does not replace the source. When the
  remembered quality is unavailable for this video and the original is playing,
  choosing the original again replaces the remembered quality with the original.
- Switching does not recreate the player. It reads the logical position and the
  playback intent (whether playing), rebuilds the attempt with the new quality
  and route through `switchQuality` (`playbackAttempt.ts`), and replaces the
  source. A reduced quality transcodes from the switch position; the original
  quality returns to direct playback (`stream`) for a video that can play
  directly. A video that fell back to transcoding because direct playback could
  not be read stays transcoded even at the original quality.
- A direct playback source has no position and loads from 0, so when metadata
  arrives it seeks to the logical position (the same as `finishRecovery` in a
  reload). Then playback continues if it was playing and stays paused if it was
  paused. Replacing the source resets playback speed to the default, so the
  speed from before the switch is restored.
- Only one switch waits at a time, and each switch replaces it (this is the
  switch generation). After several changes in a row, only the source for the
  last quality remains, and the metadata handling of an earlier switch (the seek
  for direct playback, restoring speed and intent) does not apply to the new
  source. Video.js hands the source to the element slightly later, so every
  switch checks that arriving metadata belongs to the element's current source
  (`currentSrc`) before using it. The element discards pending events of the
  previous source when its source changes, so once the switched source has
  reached the element (`loadstart`), the switch finishes with that metadata even
  if an unbuffered transcode seek changes the URL. The browser aborts the
  previous request, and the server's transcode stops on request cancellation.
  `liveOffset.ts` tells stale transcode start-position reports apart by
  attempt and discards them.
- A choice made while a reload after a network failure is pending cancels the
  wait and loads the chosen quality. A failure of the switched source is reported
  by kind through the existing error path (`playbackRecovery.ts`), and the retry
  position is the switch position. A choice made while the failure layer is
  showing is only remembered; the retry recreates the player with the remembered
  quality.

### Alternatives

- **Recreate the player** (advance the attempt in `VideoPage`): returns to the
  poster, the control bar disappears, and state inside full screen is rebuilt.
- **Build the menu as a React popover**: the look, opening behaviour and keyboard
  handling of the playback speed menu would have to be rebuilt and kept in sync
  (research.md R-4).

## Stall warning

### Context

When the connection cannot keep up with the video's bitrate, playback stops
repeatedly waiting for data. The viewer cannot tell whether the cause is the
connection or the video. Waiting for data right after a seek, the start of
playback, or a quality switch happens even on a fast enough connection, and
counting those would show the warning too often.

### Decision

- A pure state machine (`web/src/player/stallMonitor.ts`) decides. A data wait
  runs from a `waiting` received while counting to the next `playing`, and keeps
  its start time. Playback is judged stalled when 3 or more data waits start
  within a 60-second window, or one data wait exceeds 10 seconds.
- Counting happens only after `playing`.

  | Event | Effect on counting |
  | --- | --- |
  | Seek (`seeking`) | The wait until the next `playing` is not counted |
  | Source set (`loadstart`; includes the first load, quality switches and reloads) | The wait until the next `playing` is not counted |
  | Playback start (`play`) | The wait until the next `playing` is not counted |
  | Pause (`pause`) | Discard the count so far and start again |
  | Reload after a network failure (`recovering`) | Discard the count so far and start again |

- For the 10-second rule, `VideoPlayer.tsx` sets a timer when a counted data
  wait starts. The judgement goes to `VideoPage` as `PlayerStatus.stalled` and is
  cleared at the end of playback and on failure. Playback is not stopped and
  quality is not changed.
- The warning (`web/src/player/StallWarning.tsx`) is a small banner at the
  player's top left, separate from the status overlay container
  (`data-overlay-layer`). It has `role="status"`, and everything except the ×
  is `pointer-events-none` so it does not block the controls beneath. It is not
  shown while the failure, playback-ended, up-next, reconnecting or importing
  layers are showing, and it is shown alongside the loading spinner for a data
  wait (the 10-second judgement is made during the wait, so hiding it behind the
  spinner would keep it invisible during an ongoing stall).
- `VideoPage` keeps the dismissal by video id. For the same video the warning is
  not shown again, even after a retry from a failure; moving to another video
  forgets it. The warning offers no action or suggestion to switch quality, and
  there is no automatic quality reduction.

### Alternatives

- **Estimate connection speed from remaining `buffered` or `progress`
  intervals**: measuring speed leads toward automatic quality switching, and
  `progress` intervals differ widely between browsers.
- **Make it one of the layers of the status overlay container**: the container
  shows one layer at a time, exclusive with the central controls. That conflicts
  with the requirement to neither stop playback nor block the controls.
- **Show a toast in a screen corner**: invisible in full screen, and not tied to
  the player.
