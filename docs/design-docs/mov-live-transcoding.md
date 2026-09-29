# Split-track inputs for MOV live transcoding

- Status: Adopted
- Scope: live transcoding to request-scoped fragmented MP4

## Context

Video files live on network drives as well as local disks. The MOV demuxer seeks back and forth
between tracks to return video and audio packets in time order. On a network drive, this can make
the initial output and in-playback transcoding markedly slow.

FFmpeg's MOV demuxer has `interleaved_read`. Disabling it reads in file-position order, which can
let one track run far ahead of the other. In a MOV with video and audio stored far apart, the muxer
waits for the missing track, and the initial output exceeds its deadline or playback stops midway.
Disabling interleaving for all MOV files is therefore not adopted.

## Decision

FFmpeg receives the same path as two inputs only when the format name in the live transcoding
probe data contains `mov` and a selected audio stream exists. The probe data is the value saved at
scan time or the result of `ffprobe` at request time
([Probe data reuse](live-transcode-seek.md#probe-data-reuse)).

- Input 0 disables audio with `-an` and maps only the selected video stream.
- Input 1 disables video with `-vn` and maps only the selected audio stream.
- On a seek restart, both inputs get the same `-ss`. The MOV demuxer seeks to a keyframe of the
  default stream (video), and aligns audio to that time even in the `-vn` input.
  - When the video is copied from mid-video, audio also starts at the video keyframe time, and the
    offset between tracks stays in the edit list of the output `moov`
    ([Copy path and offset limit](live-transcode-seek.md#copy-path-and-offset-limit)).
  - When the video is encoded, both inputs start at the requested position.
- `interleaved_read` is not set, so the MOV demuxer keeps its default behavior.

Each demuxer follows only one track, which avoids seeks back and forth between video and audio.
Non-MOV files and MOV files without audio keep a single input. With several video or audio
streams, the existing rule still holds: only the first non-attached video and the first audio that
the probe data selected are output. The implementation is contained in `transcodeArgs` in
`internal/media/transcode.go`.

## Trade-offs

- FFmpeg opens the same file with two demuxers for each MOV with audio. Compared with a single
  input, this adds file handles, demux work and container metadata reads.
- Depending on track layout and the OS cache, both inputs can read the same region, which raises
  the total read volume. The approach guarantees fewer seeks back and forth between tracks, not
  less I/O for every MOV.
- The load scales with the number of concurrent live transcoding requests. The expected use is one
  user with 1 to 2 concurrent viewing sessions, so no shared cache or transcoding worker is added
  for now.
- Both inputs are in one FFmpeg process and stop together when the request context is canceled.
  The output is request-scoped and is not persisted to a local file or the database.

This added load is the accepted cost of getting a usable initial output for MOV on network drives
while avoiding playback stops that depend on track layout. To raise the number of concurrent
transcodes, re-measure the file handle limit, network bandwidth and server CPU, and revisit this
decision.

## Alternatives

### `interleaved_read=0`

It reduces seeks between tracks, but non-interleaved reading can let one track run far ahead.
Older FFmpeg builds also lack the option. It is not adopted because it harms playback correctness
and compatibility with the host FFmpeg.

### Copy to a local temporary file

This avoids network seeks while keeping the default interleaving, but the whole video has to be
copied before playback starts. That adds initial wait for long videos and local storage
management. It also conflicts with the current request-scoped design that keeps no persisted
artifacts, so it is not adopted.

### Two inputs for every format

The confirmed problem is the MOV demuxer's seeks between tracks. There is no evidence to justify
spreading file handles and demux load to other formats. The approach is limited to MOV files that
have both video and audio.

## Validation

- Generate a MOV with video and audio in the usual layout, and confirm that the MP4 after two-input
  transcoding has both streams.
- Transcode a real MOV from a network drive, and check the initial output, the start times of video
  and audio, and the duration.
- Cancel the transcoder after the initial data arrives, and confirm that the two-input FFmpeg
  process exits quickly.
- Confirm playback start in the browser E2E format matrix, which includes MOV.

Automated checks are in `internal/media/transcode_test.go` and `web/e2e/playback.e2e.ts`.
