# Research: Live transcode seeking and probe data reuse

Inherited decisions: the tech stack and the current live transcode follow
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md)
and [internal/media/transcode.go](../../internal/media/transcode.go). This file
records only the decisions this feature adds. The values in R-1, R-6 and R-7
were confirmed with ffmpeg 6.1.1 in the development container, using `-ss 27`
on a 40-second H.264/AAC MOV with a 250-frame keyframe interval at 30fps, and on
a fixture made from it as MKV with `-c copy`.

## R-1: Source of the actual start position

**Decision**: An ffmpeg that copies from a mid-file position gets
`-copyts -start_at_zero` and
`-movflags frag_keyframe+empty_moov+default_base_moof+delay_moov`. With
`delay_moov`, the mp4 muxer holds `moov` back until it cuts the first fragment,
and writes, as an "empty edit" at the head of each track's edit list (`elst`),
the time that track starts (in the movie timescale; ffmpeg uses 1000, so
milliseconds). The server reads the actual start position from there, rewrites
`elst` so the earliest track is 0 and the other tracks keep only their
difference from it, and then sends it. The `tfdt` of `moof` already starts at 0
in the first fragment and is not rewritten. Because of `-start_at_zero`, the
times are relative to the start of the video (the player's current timeline).

**Rationale**: When the mp4 muxer writes `moov` first with `empty_moov`, it
shifts each track's first time to 0 and discards the information (confirmed:
with `-copyts` alone, `tfdt` becomes 0). `delay_moov` is the option the muxer
itself provides for that case; the start position then appears once, in `moov`,
in ISO BMFF form. `moov` comes out together with the first fragment, so it can
be read within the current flow of "wait for the first data, then start the
response". Chrome and Firefox shift the first time to 0 in progressive
playback, so emitting the `-copyts` times as they are would not tell the player
the start position, and the timeline would differ by browser. With the edit
list rewritten, the output the browser sees has the same shape as today's copy
output (timeline starting at 0, `elst` carrying only the difference between
tracks).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Emit with `-copyts` as is | Rejected: browser-dependent, as above |
| Rewrite `tfdt` in every `moof` | Rejected: every byte sent would pass through the rewrite; rewriting `moov` once is preferred |
| `-movflags frag_discont` | Rejected: `tfdt` stays 0, and as far as was confirmed the start position does not appear |
| Read ffmpeg's stderr from `-debug_ts` or `-f framecrc` | Rejected: the line format depends on the version, and it writes out every packet of one fragment |
| A separate process that finds the keyframe before ffmpeg | Rejected: it is exactly the waste requirement 8 removes |

## R-2: Allowed gap to the keyframe when copying

**Decision**: `domain.CopySeekAllowance = 15 seconds`. When requested position
minus actual start position is at most this, the transcode continues as a copy;
when it exceeds it, the same request switches to encoding and starts from the
requested position.

**Rationale**: The default keyframe interval of x264/x265 is 250 frames: 8.3
seconds at 30fps and 10.4 seconds at 23.976fps. Many of the "MKV with H.264
inside" files the parent Issue gives as an example were made with that default,
and with 10 seconds every video at film frame rate would fall back to
re-encoding. Camera and streaming videos use 1–10 seconds, and 15 seconds lets
all of them through as copies. A video with keyframes only at scene changes
(tens of seconds to minutes) jumps back so far that the result no longer reads
as "the position I chose", so it falls back to encoding as requirement 3 says.
The cost of switching is one ffmpeg start (about 0.07 seconds) plus the demuxer
re-reading its index, and only videos above the limit pay it.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| 10 seconds | Rejected: drops the 23.976fps default |
| No limit | Rejected: violates requirement 3 |
| Remember videos that exceeded the limit and encode them directly next time | Rejected: the keyframe interval varies within a video, so one excess does not decide it; the cost of one process is accepted |

## R-3: Fallback order and time budget

**Decision**: `LiveTranscoder.Start` tries, within one request:

1. Copy, using the stored probe data (when `videoCanCopy` holds and not
   `normalize`; otherwise encode from the start). A copy with `startMs > 0` is
   emitted with the R-1 arguments and resolves the actual start position. A
   copy with `startMs = 0` keeps today's from-the-start copy arguments (neither
   `-copyts` nor `delay_moov`); its actual start position is 0 and the R-2
   check is skipped.
2. Encode with the same probe data, when the copy ended without producing its
   first data or the actual start position exceeds the R-2 limit.
3. When an attempt failed before producing its first data and the probe data
   was the stored value, run ffprobe on the spot, return the result, and retry
   from step 1 once more.

The only reasons to switch are "the process ended without producing data" and
"the limit was exceeded". Expiry of `transcodeStartupTimeout` (6 seconds)
remains a failure as before. There is one deadline for the whole sequence.

**Rationale**: The parent Issue's edge cases (a video whose keyframe position
cannot be obtained, a video that fails with the stored values) both settle on
"retry within the same request if no data has been produced yet". A slow copy
is slow on I/O, and an encode reading the same file needs CPU on top of that,
so a budget that gives up on the copy early buys nothing. The ffprobe retry is
limited to one so that a broken file does not loop through ffprobe and ffmpeg.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A deadline per attempt | Rejected: see above |
| Switching in httpapi | Rejected: starting the process and reading its output belong to media; httpapi would gain knowledge of ffmpeg arguments |

## R-4: Path that reports the actual start position to the player

**Decision**: The player adds `attempt` (a random value per request) to the
transcode URL and at the same time calls
`GET /api/videos/{id}/transcode-start?attempt=…`. The server enters `attempt`
in the ledger when the transcode request starts, and records the actual start
position once it is known (before starting to write the response). The report
path waits, with `transcodeStartupTimeout` as the limit, until the entry
appears if there is none, or until it is resolved if it is unresolved, then
returns `{ "startMs": … }`; if nothing appears by the limit it returns 404. An
`attempt` whose transcode request has ended is kept for 60 seconds and then
removed ([contracts/transcode-start-api.md](contracts/transcode-start-api.md)).

**Rationale**: Playback through `<video src>` exposes neither the response
headers nor the body structure to JavaScript. The browser may send the video
request and the report request in either order, so making the report side wait
until the entry appears removes the ordering concern. Until the report arrives
the requested position is displayed as before, so when it never arrives the
display stays the same as today.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| SSE on `/api/events` | Rejected: adds guest subscriptions and ordering handling |
| A path that returns the start position first starts ffmpeg, and the video request attaches to it | Rejected: the process lifetime spans two requests |
| Fetch the body ourselves with MSE | Rejected: rebuilds delivery; out of scope |
| Carry the start position in the URL through a response redirect | Rejected: `<video>` does not expose the URL after a redirect |

## R-5: Storage shape of the probe data

**Decision**: A new table `video_transcode_probes` holds, in one row, the JSON
of `domain.TranscodeProbe`, its version (`domain.TranscodeProbeVersion`), and
the size and modification time of the probed file (the `os.Stat` values, in
nanoseconds) ([data-model.md](data-model.md)). The reader treats a row with a
different version or unreadable JSON as absent.

**Rationale**: The stored fields are only those the ffmpeg arguments need
(listed in requirement 7), and they are never listed or searched. With one
JSON column, adding a field only bumps the version, and old rows fill in
naturally through the path of requirement 10 (probe on the spot during the
first transcode and save). Size and modification time are kept separately from
the `os.Stat` at probe time, not taken from `video_locations` (scan time, in
seconds), so that they compare at the same precision as the `Stat` of the file
opened at request time (different units would never match).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Typed columns on `videos` | Rejected: fattens the list rows and needs a migration for every added field |
| ffprobe's raw JSON | Rejected: interpretation at request time would depend on the ffprobe version at save time, and it is large |

## R-6: Aligning audio to the actual start position in two-input MOV

**Decision**: Two-input stays as it is (the same `-ss` on both inputs), with no
additional alignment.

**Rationale**: The audio-side input discards the video stream with `-vn`, but
the MOV demuxer seeks to a keyframe of the default stream (video) and aligns
the other streams to that time. In the confirmed output, audio started at the
video keyframe time with two inputs as well as with one (`elst` was 25.000
seconds for video and 24.981 seconds for audio, the same as with a single
input). The R-1 rewrite keeps the difference between tracks, so audio does not
drift.

**Alternatives considered**: Using a single input only for copy seeks was
rejected: the reason for two inputs is cross-track seeking on network drives,
and this would bring that back for a problem that, as far as was confirmed, is
already solved.

## R-7: Time until the first data of a copy

**Decision**: Unchanged. Copy fragments are cut at the source video's keyframes
(requirement 6: "when copying, keep the original keyframes"), so the first
fragment comes out after one keyframe interval has been read. With
`delay_moov`, `moov` comes out at the same moment, but playback waits for the
first fragment anyway.

**Rationale**: A copy from the start (`startMs = 0`) already has this property,
and the parent Issue does not count it as waste. Cutting fragments by time
(`-frag_duration`) yields fragments that do not start at a keyframe, which
widens what has to be confirmed about progressive playback in browsers.

**Alternatives considered**: Adding `-frag_duration 2000000` was rejected for
the reason above.
