# Live transcoding seek and probe reuse

- Status: adopted
- Scope: starting `GET /api/videos/{id}/transcode.mp4` (preparing the probe,
  starting FFmpeg, the start position for a mid-file start) and
  `GET /api/videos/{id}/transcode-start`, which tells the player the actual
  start position

## Probe reuse

### Context

The ffmpeg arguments for live transcoding (which video and audio streams to
pick, whether they can be copied, how dimensions, rotation and fps are handled,
the two MOV inputs) are decided by ffprobe facts. For videos on a network
drive, the ffprobe run is a large part of the time to first data.

### Decision

Import analysis (ffprobe) saves the values live transcoding needs,
`domain.TranscodeProbe`, in the table `video_transcode_probes`. A row holds the
values as JSON with their version, and the size and modification time
(nanoseconds) of the analysed file. Live transcoding prepares the probe in this
order:

1. The route (`internal/httpapi/transcode.go`) reads the saved value with
   `LibraryStore.TranscodeProbe` and decides with
   `domain.TranscodeProbeUsable` whether it may be used. It is used only when
   the version equals the current `domain.TranscodeProbeVersion`, the JSON
   parses, and the size and modification time match the `Stat` of the file the
   transcode actually opened. If another location has the same content, the
   comparison is still against the opened location.
2. The route puts the usable probe, or nil, in `domain.LiveTranscodeRequest`
   and passes it to `LiveTranscoder.Start` (`internal/media`). With a probe,
   media does not start ffprobe; without one, it runs ffprobe on the spot.
3. Media returns from `Start` when FFmpeg emits its first data. If a transcode
   started from a saved value ends without emitting data (for a video whose
   video stream is copied, if the switch to encoding also fails; see
   [copy path and gap limit](#copy-path-and-gap-limit)), media runs ffprobe on
   the spot within the same request and retries once. It does not retry after
   a failure of an FFmpeg started from an on-the-spot probe, a request
   cancellation, or expiry of `transcodeStartupTimeout` (6 seconds). There is
   one deadline covering probing and the retry. After `Start` returns, the route
   does not reapply the deadline; only request cancellation stops it.
4. When media probed on the spot, it returns the result in
   `LiveTranscode.Probed`, and the route saves it with
   `IngestStore.SaveTranscodeProbe` together with the stamp of the opened file.
   Saving happens after the first data is read, alongside delivery, so waiting
   for the write does not consume the startup deadline. Saving has its own
   deadline independent of delivery (`transcodeProbeSaveTimeout`, 10 seconds)
   and completes within it even if the request is cancelled. The next transcode
   uses the saved value. If the file's stamp after probing differs from when it
   was opened, media uses the result for transcoding but does not return it for
   saving. An ffprobe cut off partway ends in an error and is not saved.

Saving is a single upsert statement, so when import and transcoding write at
the same time, the later row remains. The table is an index: if it is lost,
the next transcode or reanalysis fills it. No startup job backfills existing
videos; an imported video without a row is filled by its first transcode.

### Trade-offs

- Whether a saved value fits is judged by size and modification time only;
  contents are not compared. A file rewritten with the same size and
  modification time is transcoded with the stale probe. If that transcode fails
  before its first data, the retry's probe replaces the saved value.
- Media now waits for the first data, and the route only reads the output after
  that. The switching decision sits with process start and output reading, and
  httpapi carries no knowledge of ffmpeg arguments.
- Media does not call the store. The route decides whether a saved value may be
  used and saves it, so unit tests without ffmpeg need no SQLite.

### Alternatives

- **Media reads the store**: an adapter would depend on an adapter, against the
  dependency direction in ARCHITECTURE.md.
- **Store ffprobe's raw JSON**: interpretation at request time would depend on
  the ffprobe version at save time, and each video would hold a string of tens
  of KB. The parsed values are stored with a version instead.
- **Add typed columns to `videos`**: rows read by listings would grow with
  columns they do not use, and each new field would need a migration.

### Validation

- `internal/media/transcode_test.go`: with `commandContext` replaced, checks
  that ffprobe does not start when a saved value exists; that without one it
  starts once and returns its result; that a failure with a saved value retries
  exactly once; and that expiry and cancellation neither retry nor return a
  result.
- `internal/httpapi/transcode_test.go`: with the real storage layer, checks that
  an imported video is not probed on the first request or after a seek; that a
  video without a row is probed only on the first request and the result is
  saved; and that a different size or modification time triggers probing and
  replaces the saved value.

## Copy path and gap limit

### Context

A video whose video stream can be copied as is (`videoCanCopy` is true, for
example an MKV containing H.264) avoids re-encoding even from a mid-file
position. A copy starts at the preceding keyframe rather than the requested
position, so the server has to know the time it actually started
([research.md R-1](../../specs/018-live-transcode-seek/research.md#r-1-source-of-the-actual-start-position)).

### Decision

`LiveTranscoder.Start` tries the following in order for one probe:

1. If the video stream can be copied and the request is not `Normalize` (a
   switch from direct playback), copy the video.
   - From the start (`startMs = 0`) the arguments are unchanged and the actual
     start position is 0.
   - From a mid-file position, add `-copyts -start_at_zero` and
     `-movflags …+delay_moov` to the input-side `-noaccurate_seek -ss`. The mp4
     muxer holds `moov` until it cuts the first fragment, and writes, at the
     head of each track's edit list (`elst`), the time that track starts as an
     empty edit. `internal/media/fmp4.go` reads the output up to `moov`, takes
     the start time of the earliest track as the actual start position, rewrites
     `elst` so that this track is at 0 and the others are delayed by their
     difference, and then sends it. `moof`/`mdat` are not touched.
   - If "requested position − actual start position" exceeds
     `domain.CopySeekAllowance` (15 seconds), stop this process and go to 2. A
     seek before the first keyframe yields an actual start position later than
     the requested one, which is used as is.
2. If the copy ended without its first data (up to `moov` for a mid-file start),
   or the gap exceeded the limit, encode the video with the same probe, starting
   at the requested position. The actual start position is the requested
   position.
3. If that also ends without data and the probe was a saved value, run ffprobe
   on the spot and retry once more from 1 ([probe reuse](#probe-reuse)).

There is one deadline (`transcodeStartupTimeout`, 6 seconds) for probing and all
switching; expiry and cancellation do not switch. `Start` returns the actual
start position in `LiveTranscode.StartMs`, and the route records it in the
ledger of the [start position report](#start-position-report).

Audio is copied when the request is not `Normalize` and `audioCanCopy` holds,
and is encoded to AAC otherwise.

| Video | Audio start |
| --- | --- |
| Copied, mid-file start | Encoded audio also starts, through `-noaccurate_seek`, at the same keyframe time as video |
| Encoded, mid-file start | Audio is encoded too, as before |

The input-side `-ss` discards data before the requested position for encoded
streams, but for copied streams keeps the segment from the keyframe the demuxer
reached. Copying audio would therefore make only the audio start before the
requested position (more than ten seconds earlier in videos with sparse
keyframes).

The two MOV inputs keep the same `-ss` on both, and audio also starts at the
video keyframe time ([mov-live-transcoding.md](mov-live-transcoding.md)). The
difference between tracks stays in the rewritten `elst`, so audio and video do
not drift.

### Why the gap limit is 15 seconds

The default keyframe interval of x264/x265 is 250 frames: about 8.3 seconds at
30 fps and about 10.4 seconds at 23.976 fps. Many of the "MKVs containing
H.264" that the parent Issue gives as an example are made with this default,
and a 10-second limit would send every film-frame-rate video to re-encoding.
Camera and streaming videos have intervals of 1 to 10 seconds, and 15 seconds
lets all of them through as copies. For videos with keyframes only at scene
changes (tens of seconds to minutes), the jump back no longer reads as the
position the user chose, so they are re-encoded from the requested position.
Switching costs one FFmpeg start plus reading one keyframe interval, and only
videos over the limit pay it
([research.md R-2](../../specs/018-live-transcode-seek/research.md#r-2-allowed-gap-to-the-keyframe-when-copying)).

### Trade-offs

- The first data of a copy is emitted after one keyframe interval of the
  original is read (the same as a copy from the start). For videos over the
  limit, the switch to encoding happens after that interval is read.
- In videos whose audio starts slightly before the video keyframe, the actual
  start position is the audio start time. The displayed time uses the same
  reference as the `elst` rewrite, so it matches the picture.
- The requested position is displayed until the actual start position reaches
  the player ([start position report](#start-position-report)).

### Alternatives

- **Output with the `-copyts` timestamps**: Chrome and Firefox shift the first
  time to 0 in progressive playback, so the player cannot learn the start
  position this way.
- **Find keyframe positions before FFmpeg**: adds an external process per
  request and brings back the wait that probe reuse removed.
- **Rewrite `tfdt` in each `moof`**: requires passing all sent data through; a
  single `moov` rewrite is chosen instead.

### Validation

- `internal/media/fmp4_test.go`: reading and rewriting `elst` (keeping the
  difference between tracks, timescale, a broken `moov`). Starts ffmpeg and
  checks, for an H.264 MKV and a MOV (two inputs) copied from a mid-file
  position, that the first video packet of the output equals the preceding
  keyframe, the time axis starts at 0, and output time plus the actual start
  position gives the original video's time for both video and audio. A video
  whose keyframe interval exceeds the limit switches to encoding and starts at
  the requested position.
- `internal/media/transcode_test.go`: arguments (mid-file copy, copy from the
  start, switching to encoding, switching from direct playback) and the switch
  order (copy → encode → on-the-spot probe).

## Start position report

### Context

A transcode copied from a mid-file position shows the picture from the
preceding keyframe, not the requested position. If the player displayed the
requested position as the current time, the displayed time and the saved
playback position would run ahead of the picture by up to the gap limit.
Playback through `<video src>` exposes neither response headers nor body
structure to JavaScript, so the transcode response itself cannot carry the
start position
([research.md R-4](../../specs/018-live-transcode-seek/research.md#r-4-path-that-reports-the-actual-start-position-to-the-player)).

### Decision

The player adds a per-request identifier `attempt` to the transcode URL, and
calls a separate route, `GET /api/videos/{id}/transcode-start`, with the same
`attempt`
([contracts/transcode-start-api.md](../../specs/018-live-transcode-seek/contracts/transcode-start-api.md)).

- **Ledger** (`internal/httpapi/transcode_start.go`): the key is the pair of
  video ID and `attempt`, so the route of another video cannot look up the same
  `attempt`. A transcode request with `attempt` enters the ledger once the video
  is found and the `attempt` format is checked, and records `StartMs` when
  `Start` returns with first data, before the body starts. A later record for
  the same `attempt` overwrites the earlier value. If the request ends before
  recording (409, 500, cancellation), the entry is marked failed. An entry is
  kept for `transcodeStartRetention` (60 seconds) after the transcode request
  ends, then removed. Requests without `attempt` are not recorded.
- **Report route**: looks the video up with the same `lookupServedVideo` as
  `transcodeVideo`, so a guest asking for a non-public video gets the same 404
  as for a video that does not exist (the boundary is "guests too"). If the
  `attempt` is not there yet, it waits until it appears; if it is there but
  unsettled, until it settles; at most `transcodeStartupTimeout` (6 seconds).
  The wait exists because the browser may send the video request or the report
  request first. Once settled it returns `{ "startMs": … }` with
  `Cache-Control: no-store`; on failure or after the limit it returns 404. An
  entry created only by a report request is removed once no waiter remains.
- **Player** (`web/src/player/liveOffset.ts`): `liveSource` creates an `attempt`
  (16 random bytes in hex; `crypto.randomUUID` is unavailable over plain http
  on a LAN) only when `startMs > 0`. The mediator calls `getTranscodeStart` right
  after setting the source (both the player's `src` and a rebuild for an
  unbuffered seek), and until the answer arrives returns the requested position
  as the current time.

  | Report result | Player behaviour |
  | --- | --- |
  | 200 | Replace the offset with `startMs`, tell `VideoPlayer` through `vvOffsetChanged`, and align the saved playback position to the same value |
  | 404 or error | Keep the requested position (the same display as a transcode without a report) |
  | A stale `attempt` arriving after the source was replaced | Discard |
  | Arriving while waiting for a rebuild for an unbuffered seek | Replace only the offset and do not call `vvOffsetChanged`, so the saved position stays at the chosen position; notify if the rebuild is cancelled by a seek within the buffer |

- **Subtitles**: sidecar subtitles read this offset. The mediator calls
  `vvOffsetSettled` each time the offset is settled (a report's 200, 404 or
  error, or a source without `attempt`) and `vvOffsetPending` when it starts
  waiting for a report, and the player reattaches the subtitle track at that
  offset
  ([live transcoding time alignment in sidecar-subtitles.md](sidecar-subtitles.md#live-transcoding-time-alignment)).
  How the offset is decided does not change.

### Trade-offs

- The ledger is in process memory and is lost on server restart. After a
  restart the transcode requests are recreated too, and are looked up again
  with new `attempt` values.
- The current time stays at the requested position until the report arrives.
  The ledger is written before the first data, so the report arrives before the
  picture starts moving.
- A report request adds one round trip per transcode. A transcode from the start
  (`startMs = 0`) always starts at 0, so it sends no report request.

### Alternatives

- **Deliver over the `/api/events` SSE**: needed for guest playback too, and the
  playback screen would have to handle whether it is subscribed and the order of
  arrival.
- **A route that returns the start position first starts ffmpeg, and the video
  request attaches to it**: the process lifetime spans two requests, and
  processes that never get attached need cleanup.
- **Use MSE, `fetch`, and read the response headers**: rebuilds the delivery
  mechanism and is out of the parent Issue's scope.

### Validation

- `internal/httpapi/transcode_start_test.go`: a report after the transcode; a
  report arriving before the transcode waits; waiting while unsettled; a failed
  transcode returns 404 without waiting for the limit; an `attempt` that does
  not appear before the limit and another video's route return 404; a malformed
  `attempt` returns 400 on both routes; overwriting with a later value; removal
  after the retention time.
- `internal/httpapi/guest_test.go`, `openapi_routes_test.go`: a guest can look
  up the start position only for public videos, and a non-public video returns
  the same 404 as a video that does not exist. The boundary matches `security`
  in `openapi.yaml`.
- `web/src/player/liveOffset.test.ts`: the requested position until the report
  arrives, then the actual start position; the requested position stays on 404;
  a rebuild for an unbuffered seek aligns with a new `attempt`; reports for a
  stale `attempt` are discarded; a report arriving while waiting for a rebuild
  does not overwrite the saved chosen position.
- `web/e2e/playback.e2e.ts`: in an H.264 MKV with keyframes only at 0, 8 and 16
  seconds, a seek near 14 seconds reports 8 seconds, the displayed and saved
  positions are in the 8-second range, and after a reload playback resumes at
  the same scene (same colour) with an 8-second display.
