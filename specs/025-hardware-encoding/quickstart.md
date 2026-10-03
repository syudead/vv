# Quickstart: Check hardware encoding on a host with a GPU

CI has no hardware encoder, so acceptance criteria 2, 3, 5, 6, 7 and 11 of parent
Issue #370 are checked with these steps on a host with a GPU, and the results
(host OS and GPU, encoder, result of each step) go in the implementation PR body
([research.md R-10](research.md#r-10-checks-use-tests-with-a-substituted-ffmpeg-on-hardware-checks-live-in-the-quickstart)).
`task check` runs the automated tests that substitute ffmpeg.

Hardware encoding works only when VVMDM is installed directly on the host. The
bundled Docker image encodes in software only
([research.md R-1](research.md#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only)).

## Prerequisites

- A host with a hardware encoder: Linux or Windows with an Intel/AMD GPU (VAAPI,
  Quick Sync) or an NVIDIA GPU (NVENC), or a Mac with VideoToolbox. The driver is
  installed, and the ffmpeg on PATH includes that encoder
  (`ffmpeg -hide_banner -encoders`). Follow the "Hardware encoding" section of
  [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) (written by this
  feature).
- Videos for acceptance criterion 7: an MP4 the browser cannot play (HEVC or
  similar), an MKV, a rotated video, a portrait video, and one above 4K (the
  output of `web/e2e/media-fixtures.mjs` may be reused).
- Docker (`task up` works), for step 11.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| 1. As described in the `docs/how-to/running-vv.md` section, start the single binary from `task build` directly on the host with `MDM_DATA_DIR` set. | The startup log has a `live transcode video encoder` line with `effective` `software` and each encoder's check result. | Requirement 9 |
| 2. Log in as the owner and open Video conversion in Settings. | Software is selected; the encoders the host has can be selected; the others cannot and show a reason. | 1, 4 |
| 3. Select an encoder the host has. Open a video in a format the browser cannot play and play it through transcoding. Check the host's process list (on Linux and macOS `ps -o args= -C ffmpeg` or `ps ax \| grep ffmpeg`; on Windows the command line column in Task Manager). | After the saving indicator, the encoder in use now is that encoder. The `live transcoding started` log shows it. `-c:v h264_<encoder>` appears in the process list. | 2 |
| 4. Restart the server. | Settings still shows the chosen encoder, and transcoded playback uses it. | 3 |
| 5. Play each of the five prerequisite videos from the start, seek, pause, and resume after a reload. | Orientation, aspect ratio and duration match software encoding. | 7 |
| 6. Check the transcoded output. Fetching a few seconds with `curl -o out.mp4 --cookie "<session>" "http://localhost:8080/api/videos/<id>/transcode.mp4?startMs=30000"` is enough. | `ffprobe -show_streams` shows `profile=High`, `level` 51 or lower, `pix_fmt=yuv420p`; `-show_frames -select_streams v` shows keyframes at most 2 seconds apart in output time. | Requirement 10 |
| 7. Switch to automatic. | The encoder in use now and the log show the host's encoder. | 6 |
| 8. Select and save a hardware encoder, then restart with the same `MDM_DATA_DIR` while that encoder is unavailable (for example, put an ffmpeg without that encoder first on PATH; for VAAPI or Quick Sync on Linux, run as a user without permission on `/dev/dri`). | The server starts; Settings warns that the chosen encoder is unavailable and software is converting; transcoded playback runs in software. | 5 |
| 9. For NVIDIA: open more transcoding tabs at once than the GPU's session limit. | The tabs over the limit play through software, and the log shows `hardware encoder failed; transcoding with software`. | Edge Case "concurrent session limit" |
| 10. Change the encoder while a transcode is streaming. | The playback does not stop, and the new encoder applies from the next seek. | Edge Case "the encoder changes while a transcode is streaming" |
| 11. Stop the server started in step 1 (it uses port 8080 on the same host), then start the bundled Docker image on the same host with `task up`. | Video conversion in Settings shows every hardware encoder as unavailable with reasons, and a video in a format the browser cannot play is transcoded in software. | 11 |

## Expected result

Every step above holds. If any step fails, fix that encoder's arguments
([research.md R-7](research.md#r-7-encode-arguments-swap-only-the-per-encoder-codec-options-common-arguments-keep-the-output-guarantees))
or the check procedure (R-2) and run the steps again.
