# Quickstart: verify hardware encoding on a host with a GPU

CI has no hardware encoders. Parent Issue #370 Acceptance criteria 2, 3, 5, 6, 7 and 11 are
therefore verified with these steps on a host with a GPU. Record the results (host OS and GPU,
encoder, result of each step) in the implementation PR body
([research.md R-10](research.md#r-10-tests-replace-ffmpeg-the-real-hardware-check-lives-in-the-quickstart)).
`task check` runs the automated tests that replace ffmpeg.

Hardware encoding works only when VVMDM is installed directly on the host. The bundled Docker
image is software encoding only
([research.md R-1](research.md#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only)).

## Prerequisites

- A host with a hardware encoder: Linux or Windows with an Intel/AMD GPU (VAAPI, Quick Sync) or an
  NVIDIA GPU (NVENC), or a Mac with VideoToolbox. The drivers are installed, and the ffmpeg on
  PATH includes that encoder (`ffmpeg -hide_banner -encoders`). Follow the "Hardware encoding"
  section of [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) (this feature writes
  it).
- Videos for Acceptance criterion 7: an MP4 the browser cannot play (HEVC or similar), an MKV, a
  rotated video, a portrait video, and one larger than 4K (the output of
  `web/e2e/media-fixtures.mjs` may be reused).
- Docker for step 11 (`task up` works).

## Steps

1. Following the section in `docs/how-to/running-vv.md`, start the single binary from
   `task build` directly on the host with `MDM_DATA_DIR` set. The startup log has a
   `live transcode video encoder` line with `effective` set to `software` and the check result of
   each encoder (Requirement 9).
2. Sign in as the owner and open "Video conversion" on the settings screen. Software is selected,
   the encoders present on the host are selectable, and missing encoders are not selectable and
   show a reason (Acceptance criteria 1 and 4).
3. Select an encoder present on the host. After the saving indicator, "In use now" shows that
   encoder. Open a video in a format the browser cannot play and play it through transcode;
   `live transcoding started` in the log shows that encoder (Acceptance criterion 2). The host
   process list shows `-c:v h264_<encoder>` (Linux and macOS: `ps -o args= -C ffmpeg` or
   `ps ax | grep ffmpeg`; Windows: the command line column in Task Manager).
4. Restart the server. The settings screen keeps the selected encoder, and transcoded playback
   uses the same encoder (Acceptance criterion 3).
5. For the 5 videos from the prerequisites, test playback from the start, seek, pause, and resume
   after reload. Orientation, aspect ratio and duration match software encoding (Acceptance
   criterion 7).
6. Check the transcode output: `ffprobe -show_streams` shows `profile=High`, `level` at 51 or
   lower and `pix_fmt=yuv420p`; `-show_frames -select_streams v` shows a keyframe interval of 2 s
   or less in output time (Requirement 10). A few seconds of output is enough:
   `curl -o out.mp4 --cookie "<session>" "http://localhost:8080/api/videos/<id>/transcode.mp4?startMs=30000"`
   (`<session>` is the session cookie).
7. Switch to automatic. "In use now" and the log show the host's encoder (Acceptance
   criterion 6).
8. Select and save a hardware encoder, make it unavailable, and restart with the same
   `MDM_DATA_DIR`. For example, put an ffmpeg without that encoder first on PATH; for VAAPI or
   Quick Sync on Linux, run as a user without permission on `/dev/dri`. The server starts, the
   settings screen shows the warning that the selected encoder is unavailable and software is
   used, and transcoded playback uses software (Acceptance criterion 5).
9. NVIDIA only: open more transcoded playback tabs at once than the GPU session limit allows. The
   extra ones play with software, and the log shows
   `hardware encoder failed; transcoding with software` (edge case "concurrent session limit").
10. Change the encoder while a transcode is streaming. The playing stream does not stop, and the
    next seek uses the new encoder (edge case "changing the encoder while a transcode is
    streaming").
11. Stop the server started directly in step 1 (both use port 8080 on the same host). Start the
    bundled Docker image on the same host with `task up`. "Video conversion" on the settings
    screen shows every hardware encoder as unavailable with a reason, and a video in a format the
    browser cannot play is transcoded with software (Acceptance criterion 11).

## Expected result

Every step above holds. If any step fails, fix that encoder's arguments
([research.md R-7](research.md#r-7-encode-arguments-replace-only-the-per-encoder-codec-options-shared-arguments-keep-the-output-contract))
or the check procedure (R-2), then run the steps again.
