# Research: seeking in live transcodes and reusing probe data

The tech stack and the current live transcode design follow their sources of truth
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md),
[internal/media/transcode.go](../../internal/media/transcode.go)). This document holds only the
decisions this feature adds.

The values in R-1, R-6 and R-7 were confirmed with ffmpeg 6.1.1 in the dev container, using `-ss 27`
on 2 fixtures: a 40-second H.264/AAC MOV at 30fps with a keyframe interval of 250 frames, and the
same file remuxed to MKV with `-c copy`.

## R-1: Where the actual start position comes from

| | |
| --- | --- |
| **Decision** | ffmpeg copying from a midpoint gets `-copyts -start_at_zero` and `-movflags frag_keyframe+empty_moov+default_base_moof+delay_moov`. With `delay_moov`, the mp4 muxer holds `moov` until it cuts the first fragment. It writes the time each track starts (movie timescale; 1000 = milliseconds in ffmpeg) as an "empty edit" at the head of that track's edit list (`elst`). The server reads the actual start position from there. Before sending, it rewrites `elst` so that the earliest track is 0 and the other tracks keep their difference. `tfdt` in `moof` already starts at 0 in the first fragment and is not rewritten. Because of `-start_at_zero`, the times are relative to the start of the video (the player's current timeline). |
| **Why** | When the mp4 muxer writes `moov` first with `empty_moov`, it shifts each track's first time to 0 and drops the information (confirmed: with `-copyts` alone, `tfdt` becomes 0). `delay_moov` is the muxer's own option for this case; the start position appears once, in `moov`, in ISO BMFF form. `moov` comes out with the first fragment, so it can be read within the current flow of waiting for the first data before starting the response. Chrome and Firefox shift the first time to 0 in progressive playback. So emitting the `-copyts` times as is does not tell the player the start position, and the timeline differs per browser. With the edit list rewritten, the output the browser sees has the same shape as today's copy output (timeline starts at 0; `elst` holds only the difference between tracks). |
| **Rejected** | Emitting with `-copyts` as is: browser-dependent, as above. Rewriting `tfdt` in each `moof`: all sent data must pass through it; rewriting `moov` once is chosen instead. `-movflags frag_discont`: `tfdt` stays 0 and, as far as tested, the start position does not appear. Reading the stderr of ffmpeg `-debug_ts` or `-f framecrc`: the line format depends on the version, and it writes out every packet of a fragment. A separate process that inspects keyframes before ffmpeg: that is exactly the waste Requirement 8 removes. |

## R-2: Keyframe distance allowed for copying

| | |
| --- | --- |
| **Decision** | `domain.CopySeekAllowance = 15 seconds`. When requested position − actual start position is at most this, copying continues. When it is larger, the same request switches to encoding and starts at the requested position. |
| **Why** | The default keyframe interval of x264/x265 is 250 frames: 8.3 seconds at 30fps and 10.4 seconds at 23.976fps. Many files of the parent Issue's example, "an MKV containing H.264", are made with this default. At 10 seconds, every video at the film frame rate would fall back to re-encoding. Camera and streaming videos use 1 to 10 seconds, and 15 seconds passes all of them through copy. Videos with keyframes only at scene cuts (tens of seconds to minutes) jump back so far that the position no longer reads as "the position I chose", so they fall back to encoding, as Requirement 3 says. The cost of switching is 1 ffmpeg startup (about 0.07 seconds) plus rereading the demuxer index, paid only by videos over the limit. |
| **Rejected** | 10 seconds: drops the 23.976fps default. No limit: violates Requirement 3. Remembering videos that exceeded the limit and encoding them directly next time: keyframe intervals vary within a video, so 1 excess does not decide it. The cost of 1 extra process is accepted. |

## R-3: Fallback order and time budget

| | |
| --- | --- |
| **Decision** | `LiveTranscoder.Start` tries these steps in order within the same request. (1) Copy with the stored probe data, when `videoCanCopy` holds and `normalize` does not; otherwise encode from the start. A copy with `startMs > 0` uses the R-1 arguments and resolves the actual start position. A copy with `startMs = 0` keeps today's copy-from-start arguments (no `-copyts`, no `delay_moov`); its actual start position is 0 and the R-2 check is skipped. (2) When the copy ends without producing its first data, or the actual start position exceeds the R-2 limit, encode with the same probe data. (3) When it fails before the first data and the probe data came from storage, run ffprobe on the spot, return the result, and retry from (1) exactly once more. The only reasons to switch are "the process ends without data" and "the limit is exceeded". Expiry of `transcodeStartupTimeout` (6 seconds) is still a failure, as today. There is 1 deadline for the whole fallback sequence. |
| **Why** | The parent Issue's Edge cases (a video whose keyframe position cannot be obtained, a video that fails with stored values) both decide "retry within the same request if before the first data". A slow copy is slow on I/O, and an encode of the same file also needs CPU, so a budget that gives up on copying early is pointless. The ffprobe retry is limited to 1 so a broken file does not repeat ffprobe and ffmpeg. |
| **Rejected** | A deadline per attempt (reason above). Switching in httpapi: starting the process and reading its output are the responsibility of media, and httpapi would need knowledge of ffmpeg arguments. |

## R-4: How the actual start position reaches the player

| | |
| --- | --- |
| **Decision** | The player adds `attempt` (a random value per request) to the transcode URL and at the same time calls `GET /api/videos/{id}/transcode-start?attempt=…`. The server enters `attempt` in the ledger when the transcode request starts. It records the actual start position once known (before it starts writing the response). The report route waits, up to `transcodeStartupTimeout`: until the record appears if absent, and until it is resolved if unresolved. It then returns `{ "startMs": … }`, or 404 when nothing appears within the limit. An `attempt` whose transcode request has ended is kept for 60 seconds, then deleted ([contracts/transcode-start-api.md](contracts/transcode-start-api.md)). |
| **Why** | Playback through `<video src>` exposes neither the response headers nor the body structure to JavaScript. The browser may send the video request and the report request in either order; a report route that waits for the record makes the order irrelevant. Until the report arrives, the requested position is displayed as today, so a missing report leaves the display the same as now. |
| **Rejected** | SSE on `/api/events`: adds guest subscriptions and ordering. A route that returns the start position first starts ffmpeg, and the video request attaches to it: the process lifetime spans 2 requests. Fetching the body through MSE: rebuilds delivery, out of scope. Putting the start position in the URL through a redirect: `<video>` does not expose the URL after a redirect. |

## R-5: Storage shape of the probe data

| | |
| --- | --- |
| **Decision** | A new table `video_transcode_probes` holds, in 1 row, the JSON of `domain.TranscodeProbe`, its version (`domain.TranscodeProbeVersion`), and the size and modification time of the probed file (`os.Stat` values, nanoseconds) ([data-model.md](data-model.md)). The reader treats rows with a different version and rows whose JSON cannot be parsed as absent. |
| **Why** | The stored fields are only those the ffmpeg arguments need (listed in Requirement 7); lists and search do not use them. With 1 JSON column, adding a field only bumps the version, and old rows refill naturally through the path of Requirement 10 (probe on the spot at the first transcode and save). Size and modification time are stored separately from the probe-time `os.Stat`, not taken from `video_locations` (scan time, seconds). This compares them at the same precision as the `Stat` of the file opened at request time; different units would never match. |
| **Rejected** | Typed columns in `videos`: fattens list rows and needs a migration per added field. Raw ffprobe JSON: interpretation at request time depends on the ffprobe version at save time, and it is large. |

## R-6: Aligning MOV dual-input audio to the actual start position

| | |
| --- | --- |
| **Decision** | Dual input stays as today (the same `-ss` on both inputs), with no extra alignment. |
| **Why** | The audio input drops the video stream with `-vn`, but the MOV demuxer seeks to a keyframe of the default stream (video) and aligns the other streams to that time. In the confirmed output, audio started at the video keyframe time with both dual and single input (`elst` is 25.000 seconds for video and 24.981 seconds for audio, the same as single input). The R-1 rewrite keeps the difference between tracks, so audio does not drift. |
| **Rejected** | Single input only for copy seeks: it would revert the reason for dual input (seeking between tracks on network drives) for a problem that, as far as tested, is already solved. |

## R-7: Time until the first copy data

| | |
| --- | --- |
| **Decision** | No change. Copy fragments are cut at the source keyframes ("when copying, keep the original keyframes" in Requirement 6), so the first fragment comes out after 1 keyframe interval has been read. With `delay_moov`, `moov` also comes out at that point, but playback waits for the first fragment anyway. |
| **Why** | Copying from the start (`startMs = 0`) already has this property, and the parent Issue does not count it as waste. Cutting fragments by time (`-frag_duration`) produces fragments that do not start at a keyframe, which widens what must be verified for progressive playback in browsers. |
| **Rejected** | Adding `-frag_duration 2000000` (reason above). |
