# Live transcoding seek and probe data reuse

- Status: Adopted
- Scope: how `GET /api/videos/{id}/transcode.mp4` starts (preparing the probe data, launching
  FFmpeg, the start position when starting mid-video), and `GET /api/videos/{id}/transcode-start`,
  which tells the player the actual start position

## Probe data reuse

### Context

The ffmpeg arguments for live transcoding depend on ffprobe facts: which video and audio streams
to pick, whether they can be copied, how to handle dimensions, rotation and fps, and the two MOV
inputs. For videos on a network drive, ffprobe takes a large share of the time to the first data.

### Decision

The scan's probe (ffprobe) saves the values that live transcoding needs, `domain.TranscodeProbe`,
in the table `video_transcode_probes`. A row holds the JSON of the values, its version, and the
size and modification time (nanoseconds) of the probed file. Live transcoding prepares the probe
data in this order.

1. The route (`internal/httpapi/transcode.go`) reads the saved value with
   `LibraryStore.TranscodeProbe`, and `domain.TranscodeProbeUsable` decides whether to use it. It
   is used only when its version equals the current `domain.TranscodeProbeVersion`, the JSON
   parses, and the size and modification time match `Stat` of the file the transcode actually
   opened. Even when another location has the same content, the comparison uses the opened one.
2. The route puts the usable probe data, or nil, into `domain.LiveTranscodeRequest` and passes it
   to `LiveTranscoder.Start` (`internal/media`). With probe data, media does not launch ffprobe.
   Without it, media runs ffprobe on the spot.
3. media returns from `Start` when FFmpeg produces its first data. A transcode started from the
   saved value may end without data. For a video whose video stream is copied, this means the
   fallback to encoding also failed ([Copy path and offset limit](#copy-path-and-offset-limit)).
   media then runs ffprobe on the spot and retries once, within the same request.
   - It does not retry after a failure of FFmpeg started from on-the-spot probe data, after request
     cancellation, or after `transcodeStartupTimeout` (6 s) expires.
   - The one deadline covers probing and the retry. After `Start` returns, the route does not apply
     the same deadline again; only request cancellation stops it.
4. When media probed on the spot, it returns the result in `LiveTranscode.Probed`. The route saves
   it with `IngestStore.SaveTranscodeProbe`, together with the marks of the opened file.
   - The save runs after the first data is read, alongside streaming, so waiting for the write does
     not use the startup deadline.
   - The save has its own deadline, independent of streaming (`transcodeProbeSaveTimeout`, 10 s),
     and completes within it even if the request is canceled. The next transcode uses the saved
     value.
   - If the file marks after probing differ from those at open time, media uses the result for the
     transcode but does not return it for saving. An ffprobe cut short ends with an error, so it is
     not saved.

The save is one upsert statement. When a scan and a transcode write at the same time, the later
row wins. The table is an index; if it is lost, the next transcode or re-probe fills it. No startup
job backfills existing videos. A scanned video without a row gets one on its first transcode.

### Trade-offs

- Whether the saved value matches is judged by size and modification time only; content is not
  compared. A file rewritten with the same size and modification time is transcoded with stale
  probe data. If that transcode fails before the first data, the retry's probe replaces the saved
  value.
- media now waits for the first data, and the route only reads the output after that. The fallback
  decision stays with the side that launches the process and reads its output, and httpapi needs no
  knowledge of ffmpeg arguments.
- media does not call the store. The route decides whether a saved value is usable and saves it,
  so unit tests without ffmpeg need no SQLite.

### Alternatives

- **media reads the store**: an adapter would depend on an adapter, against the dependency
  direction (ARCHITECTURE.md).
- **Save ffprobe's raw JSON**: interpretation at request time would depend on the ffprobe version
  at save time, and each video would hold a string of tens of KB. The parsed value is saved with a
  version instead.
- **Add typed columns to `videos`**: rows grow with columns that list reads never use, and every
  new field needs a migration.

### Validation

- `internal/media/transcode_test.go`: replaces `commandContext` and checks that ffprobe is not
  launched when a saved value exists, that it is launched exactly once and its result returned when
  none exists, that a failure with the saved value retries exactly once, and that deadline expiry
  and cancellation neither retry nor return a result.
- `internal/httpapi/transcode_test.go`: with the real store, checks that a scanned video is not
  probed on the first request or after a seek, that a video without a row is probed only on the
  first request and saved, and that a different size or modification time triggers a probe that
  replaces the saved value.

## Copy path and offset limit

### Context

A video whose video stream can be copied as is (`videoCanCopy` is true, such as an MKV containing
H.264) avoids re-encoding even when starting mid-video. A copy starts from the preceding keyframe,
not the requested position, so the server needs to know the time it actually started
([research.md R-1](../../specs/018-live-transcode-seek/research.md#r-1-where-the-actual-start-position-comes-from)).

### Decision

`LiveTranscoder.Start` tries these steps for one set of probe data.

1. If the video can be copied and the request is not `Normalize` (a fallback from direct play),
   copy the video.
   - From the start (`startMs = 0`), the arguments are unchanged and the actual start position is 0.
   - Mid-video, add `-copyts -start_at_zero` and `-movflags …+delay_moov` to the input-side
     `-noaccurate_seek -ss`. The mp4 muxer holds `moov` until it cuts the first fragment, and
     writes, at the head of each track's edit list (`elst`), the time that track starts as an empty
     edit.
   - `internal/media/fmp4.go` reads the output up to `moov`. It takes the start time of the
     earliest track as the actual start position, rewrites `elst` so that track starts at 0 and the
     others are delayed by their difference, and then sends it. It does not touch `moof`/`mdat`.
   - If "requested position − actual start position" exceeds `domain.CopySeekAllowance` (15 s), it
     stops this process and goes to step 2. A seek before the first keyframe gives an actual start
     position after the requested one, and that is used as is.
2. If the copy ends without its first data (up to `moov` when mid-video), or the offset exceeds the
   limit, encode the video from the same probe data, starting at the requested position. The
   actual start position is the requested position.
3. If that also ends without data and the probe data was the saved value, run ffprobe on the spot
   and retry once more from step 1 ([Probe data reuse](#probe-data-reuse)).

One deadline (`transcodeStartupTimeout`, 6 s) covers probing and all fallbacks. Deadline expiry and
cancellation do not fall back. `Start` returns the actual start position in
`LiveTranscode.StartMs`, and the route records it in the ledger of the
[report route](#report-route).

Audio is copied when the request is not `Normalize` and `audioCanCopy` holds; otherwise it is
encoded to AAC.

- When the video is copied mid-video, encoded audio also starts, via `-noaccurate_seek`, at the
  same keyframe time as the video.
- When the video is encoded mid-video, audio is encoded too, as before.
- The input-side `-ss` discards data before the requested position for encoded streams. For copied
  streams, the demuxer keeps the segment from the keyframe it landed on. Copying audio would
  therefore start only the audio before the requested position (more than ten seconds early in
  videos with sparse keyframes).

The two MOV inputs keep the same `-ss` on both inputs, and audio also starts at the video keyframe
time ([mov-live-transcoding.md](mov-live-transcoding.md)). The offset between tracks remains in the
rewritten `elst`, so audio and video stay in sync.

### Why the offset limit is 15 seconds

- The default keyframe interval of x264/x265 is 250 frames: about 8.3 s at 30 fps and about 10.4 s
  at 23.976 fps. Many "MKV containing H.264" files, the parent Issue's example, use this default.
  With 10 s, every video at the film frame rate would fall back to encoding.
- Cameras and streaming-oriented videos have intervals of 1–10 s, and 15 s lets all of them copy.
- In videos with keyframes only at scene changes (tens of seconds to minutes), the jump back no
  longer reads as the position the user chose, so they are re-encoded and start at the requested
  position.
- The fallback costs one FFmpeg launch and reading one keyframe interval, and only videos over the
  limit pay it
  ([research.md R-2](../../specs/018-live-transcode-seek/research.md#r-2-keyframe-distance-allowed-for-copying)).

### Trade-offs

- The first data of a copy comes out after one keyframe interval of the source has been read (the
  same as copying from the start). For videos over the limit, that interval is read before the
  switch to encoding.
- When audio starts slightly before the video keyframe, the actual start position is the audio
  start time. The displayed time uses the same reference as the `elst` rewrite, so it matches what
  is on screen.
- Until the actual start position reaches the player, the player shows the requested position
  ([report route](#report-route)).

### Alternatives

- **Emit the `-copyts` timestamps as is**: Chrome and Firefox shift the first timestamp to 0 in
  progressive playback, so the player cannot learn the start position this way.
- **Look up keyframe positions before FFmpeg**: one more external process per request, which brings
  back the wait that probe data reuse removed.
- **Rewrite `tfdt` in every `moof`**: all sent data would have to pass through it. Rewriting `moov`
  once is chosen instead.

### Validation

- `internal/media/fmp4_test.go`: reading and rewriting `elst` (keeping the offset between tracks,
  timescale, broken `moov`).
  - It launches ffmpeg and copies an H.264 MKV and a MOV (two inputs) from mid-video. It checks
    that the first video packet of the output equals the preceding keyframe, that the timeline
    starts at 0, and that output time plus the actual start position gives the source time for
    both video and audio.
  - A video whose keyframe interval exceeds the limit switches to encoding and starts at the
    requested position.
- `internal/media/transcode_test.go`: the arguments (mid-video copy, copy from the start, fallback
  to encoding, fallback from direct play) and the fallback order (copy → encode → on-the-spot
  probe).

## Report route

### Context

A transcode that copies from mid-video shows content from the preceding keyframe, not from the
requested position. If the player showed the requested position as the current time, the display
and the saved playback position would run ahead of what is on screen by up to the offset limit.
Playback through `<video src>` exposes neither response headers nor body structure to JavaScript,
so the transcode response itself cannot carry the start position
([research.md R-4](../../specs/018-live-transcode-seek/research.md#r-4-how-the-actual-start-position-reaches-the-player)).

### Decision

The player adds a per-request identifier `attempt` to the transcode URL, and calls a separate
route, `GET /api/videos/{id}/transcode-start`, with the same `attempt`
([contracts/transcode-start-api.md](../../specs/018-live-transcode-seek/contracts/transcode-start-api.md)).

- **Ledger** (`internal/httpapi/transcode_start.go`): the key is the pair of video ID and
  `attempt`, so another video's route cannot look up the same `attempt`.
  - A transcode request with `attempt` enters the ledger once the video is found and the `attempt`
    format is validated. `StartMs` is recorded after `Start` returns with the first data and before
    the body is written.
  - A later record for the same `attempt` overwrites the earlier value. A request that ends before
    recording (409, 500, cancellation) is marked as failed.
  - An entry is deleted `transcodeStartRetention` (60 s) after the transcode request ends. Requests
    without `attempt` do not enter the ledger.
- **Report route**: it looks up the video with the same `lookupServedVideo` as `transcodeVideo`, so
  a guest asking for a non-public video gets the same 404 as for a missing video (the boundary
  treatment is "guests too").
  - It waits up to `transcodeStartupTimeout` (6 s): until the `attempt` appears if it is absent,
    and until it settles if it is pending. The browser may send the video request or the report
    request first.
  - Once settled, it returns `{ "startMs": … }` with `Cache-Control: no-store`. On failure or when
    the limit passes, it returns 404.
  - An entry created only by a report request is deleted when no waiter remains.
- **Player** (`web/src/player/liveOffset.ts`): `liveSource` creates `attempt` only when
  `startMs > 0` (16 random bytes in hex; `crypto.randomUUID` is unavailable over plain http on a
  LAN).
  - The broker calls `getTranscodeStart` right after setting the source (both the player's `src`
    and the rebuild for an unbuffered seek). Until the report arrives, it returns the requested
    position as the current time.
  - On 200, it replaces the offset with `startMs`, tells `VideoPlayer` through `vvOffsetChanged`,
    and aligns the saved playback position to the same value. On 404 or an error, it keeps the
    requested position (the same display as a transcode without a report).
  - It discards reports for an old `attempt` that arrive after the source was replaced.
  - A report that arrives while waiting for an unbuffered-seek rebuild replaces only the offset and
    does not call `vvOffsetChanged`. This keeps the saved position at the chosen position. It
    notifies when the rebuild is canceled by a seek within the buffer.

### Trade-offs

- The ledger lives in process memory and is lost when the server restarts. After a restart the
  transcode requests are also recreated, so a new `attempt` looks it up again.
- The current time stays at the requested position until the report arrives. The ledger record is
  written before the first data, so the report arrives before the video starts moving.
- Each transcode adds one report round trip. A transcode from the start (`startMs = 0`) always
  starts at 0, so it sends no report request.

### Alternatives

- **Deliver it over SSE on `/api/events`**: guest playback needs it too, and the video page would
  have to handle subscription state and delivery order.
- **A route that returns the start position first launches ffmpeg, and the video request attaches
  to it**: the process lifetime would span two requests, and processes that never get attached
  would need cleanup.
- **Use MSE, `fetch`, and read the response headers**: a rebuild of the delivery mechanism, out of
  scope for the parent Issue.

### Validation

- `internal/httpapi/transcode_start_test.go`: a report after the transcode, waiting for a report
  that arrives before the transcode, waiting while pending, 404 for a failed transcode without
  waiting for the limit, 404 for an `attempt` that does not appear by the limit and for another
  video's route, 400 on both routes for a malformed `attempt`, overwriting with a later value, and
  deletion after the retention time.
- `internal/httpapi/guest_test.go`, `openapi_routes_test.go`: a guest can look up the start
  position only for public videos, and a non-public video gives the same 404 as a missing one. The
  boundary treatment matches `security` in `openapi.yaml`.
- `web/src/player/liveOffset.test.ts`: the requested position until the report arrives, the actual
  start position after it, the requested position kept on 404, alignment with a new `attempt` on an
  unbuffered-seek rebuild, discarding reports for an old `attempt`, and a report that arrives while
  waiting for a rebuild not overwriting the saved chosen position.
- `web/e2e/playback.e2e.ts`: with an H.264 MKV whose keyframes are only at 0, 8 and 16 s, seeking
  to about 14 s gives a report of 8 s, and the display and the saved position are in the 8 s
  range. After a reload, playback resumes from the same scene (the same color) with a display in
  the 8 s range.
