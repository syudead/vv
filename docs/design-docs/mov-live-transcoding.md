# Separate track inputs for MOV live transcoding

- Status: adopted
- Scope: live transcoding to request-scoped fragmented MP4

For a MOV with audio, live transcoding opens the same file as two FFmpeg
inputs, one for video and one for audio, so that neither demuxer seeks back and
forth between tracks. The code is in `transcodeArgs` in
`internal/media/transcode.go`.

## Context

Video files live on network drives as well as local disks. The MOV demuxer
returns video and audio packets in time order by seeking frequently between
tracks, and on a network drive this can make the initial output and ongoing
transcoding very slow.

FFmpeg's MOV demuxer has `interleaved_read`, but disabling it makes reads in
file-position order able to run one track far ahead of the other. In a MOV
whose video and audio are stored far apart, the muxer waits for the missing
track, and either the initial output misses its deadline or playback stops
partway. Disabling interleaving for every MOV is therefore not used.

## Decision

FFmpeg gets the same path as two inputs only when the format name in the live
transcoding probe (the value saved at import, or the result of `ffprobe` at
request time; see [probe reuse](live-transcode-seek.md#probe-reuse)) contains
`mov` and there is an audio stream to select.

| Input | Flag | Maps |
| --- | --- | --- |
| 0 | `-an` (audio disabled) | The selected video stream only |
| 1 | `-vn` (video disabled) | The selected audio stream only |

- When resuming at a seek position, both inputs get the same `-ss`. The MOV
  demuxer seeks to a keyframe of the default stream (video), and the `-vn`
  input also aligns audio to that time. When video is copied from a mid-file
  position, audio also starts at the video keyframe time, and the difference
  between tracks is kept in the edit list of the output `moov`
  ([copy path and gap limit](live-transcode-seek.md#copy-path-and-gap-limit)).
  When video is encoded, both inputs start at the requested position.
- `interleaved_read` is not set; the MOV demuxer keeps its default behaviour.

Each demuxer follows only one track, which avoids seeks back and forth between
video and audio. Non-MOV files and MOVs without audio keep a single input. With
several video or audio streams, the existing rule still applies: only the first
non-attached video and the first audio chosen by the probe are output.

## Trade-offs

- FFmpeg opens each MOV with audio with two demuxers. File handles, demux work
  and container metadata reads increase over a single input.
- Depending on track layout and the OS cache, the two inputs can read the same
  region, increasing total reads. This method guarantees fewer seeks between
  tracks, not less I/O for every MOV.
- The load scales with the number of concurrent live transcoding requests. The
  expected use is a single user with 1 to 2 concurrent viewing sessions, so no
  shared cache or transcoding worker is added at this point.
- Both inputs are in one FFmpeg process and stop together when the request
  context is cancelled. Output is request-scoped and is not persisted to a
  local file or the database.

This added load is accepted as the price of getting usable initial output for
MOVs on a network drive while avoiding playback stops that depend on track
layout. If the number of concurrent transcodes grows, re-measure the file
handle limit, network bandwidth and server CPU, and revisit this decision.

## Alternatives

### `interleaved_read=0`

Fewer seeks between tracks, but non-interleaved reads can run one track far
ahead. Older FFmpeg builds also lack the option. Rejected because it harms
playback correctness and compatibility with the host's FFmpeg.

### Copying to a local temporary file

Avoids network seeks while keeping the default interleaving, but the whole video
has to be copied before playback starts, which adds initial wait for long videos
and local storage management. It also does not fit the current request-scoped
design that keeps nothing persistent. Rejected.

### Two inputs for every format

The observed problem is the MOV demuxer's seeking between tracks; there is no
basis for spreading file handle and demux load to other formats. Limited to MOV
files with both video and audio.

## Validation

- Generate a MOV with a normal video/audio layout and confirm that the MP4 from
  the two-input transcode contains both streams.
- Transcode a real MOV from a network drive and check the initial output, the
  video and audio start times, and the duration.
- Cancel the transcoder after the initial data arrives and confirm that the
  two-input FFmpeg process exits promptly.
- Confirm in browser E2E that playback starts for the format matrix, which
  includes MOV.

Automated checks are in `internal/media/transcode_test.go` and
`web/e2e/playback.e2e.ts`.
