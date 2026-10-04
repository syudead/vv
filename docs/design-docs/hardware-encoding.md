# Hardware encoding for live transcoding

- Status: adopted
- Scope: for live transcoding (`GET /api/videos/{id}/transcode.mp4`), choosing
  the video encoder, the startup check, switching within a request, the
  arguments for each encoder, and the settings API
  (`GET`/`PUT /api/settings/transcoding`)
- Background: [specs/025-hardware-encoding/](../../specs/025-hardware-encoding/plan.md)
  (parent Issue #370)

This document covers only which encoder the video-encoding step uses. Starting
a live transcode (preparing the probe, switching between copy and encode) is in
[live-transcode-seek.md](live-transcode-seek.md), and the two MOV inputs are in
[mov-live-transcoding.md](mov-live-transcoding.md).

## Encoders and the encoder in use

### Context

Live transcoding encoded video with `libx264`. With a GPU in the server, NVENC,
Quick Sync, VAAPI and VideoToolbox lower the CPU load. An encoder present in
the ffmpeg build is still unusable without the device or driver (inside the
bundled Docker image, without permission on `/dev/dri`, without the NVIDIA
libraries).

### Decision

The owner's choice (`domain.EncoderChoice`) is one of `software`, `nvenc`,
`qsv`, `vaapi`, `videotoolbox` and `auto`, stored as a string under the key
`transcode.video_encoder` in the SQLite `settings` table (`SettingsStore`). A
missing row or an unknown string is treated as `software`, and the saved value
is not rewritten (`domain.ParseEncoderChoice`).

The pure function `domain.ResolveVideoEncoder` decides the encoder in use
(`domain.VideoEncoder`) from the choice and the startup check results.

| Choice | Check state | Encoder in use | Reason |
| --- | --- | --- | --- |
| `software` | any | software | none |
| `auto` | any | The first usable of `nvenc`, `qsv`, `vaapi`, `videotoolbox`, in that order; software if none | none (software here is not a fallback) |
| A hardware encoder | usable | that encoder | none |
| A hardware encoder | checking | software | `checking` |
| A hardware encoder | checked, unusable | software | `selected_unavailable` |

`TranscodeSettings` in `internal/app` holds the choice and the check results in
memory. Check results are rebuilt at every start, so they are not saved.
`Current()` returns the current state (choice, encoder in use, reason, whether
checking, result per encoder), and `Select()` saves and updates memory. Unless
the encoder is a usable hardware encoder (checked and `available`), `Select()`
returns `domain.ErrEncoderUnavailable` and leaves the saved value unchanged.
`software` and `auto` are always accepted. When the check finishes and on every
`Select()`, the choice, encoder in use and reason are logged at `Info`.

The live transcoding route (`internal/httpapi/transcode.go`) puts the encoder in
use from `Current()` into `domain.LiveTranscodeRequest.VideoEncoder` for each
request. A change takes effect from the next request; transcodes already
streaming keep the encoder they started with. No restart is needed. A request
whose video can be copied is copied regardless of the encoder.

### Trade-offs

- Each request reads the in-memory state, so starting a transcode adds no
  SQLite read.
- An encoder change is not announced to screens (no domain event and no
  `/api/events` type). Other tabs and devices show the right state the next time
  they display the section, or from the response to their own save.

## Startup check

### Decision

`cmd/mdm` reads the saved value, creates `TranscodeSettings`, logs the encoder
line (choice, encoder in use, whether checking), and then starts the check in a
background goroutine. HTTP listening does not wait for the check. Until the
check finishes, the encoder in use is software even when a hardware encoder is
chosen, and the settings API returns `checking: true`. A stop signal cancels the
check, and the scan and workers are stopped after it ends.

The OS decides what is checked (`domain.HardwareEncoderCandidates`).

| OS | Candidates |
| --- | --- |
| linux | NVENC, Quick Sync, VAAPI |
| windows | NVENC, Quick Sync |
| darwin | VideoToolbox |
| any other combination | `unsupported_os` |

`EncoderCheck` in `internal/media` checks one encoder:

1. Read `ffmpeg -encoders` once and share it among the concurrent checks. An
   encoder whose name is absent is `encoder_missing` without running anything.
   A read that exceeds the time limit is `timed_out`.
2. Encode `-f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8` to
   `-f null -` with the same encoding arguments as live transcoding. Exit code 0
   is `available`, a failure is `check_failed`, and exceeding the time limit is
   `timed_out`.

The checks for each encoder run concurrently, and `TranscodeSettings` gives each
a time limit (10 seconds). A check that never returns becomes `timed_out` at the
limit and does not stay "checking". The tail of a failed check's standard error
is logged at `Warn` and not exposed through the API. The check runs once, at
startup.

### Trade-offs

- `-encoders` alone does not show whether the device or driver exists, so a
  short real encode is run. The synthetic input (`lavfi`) exists in every build
  and needs no input file.
- Because the checks run concurrently, even if one encoder hangs the worst-case
  wait is about 20 seconds including the `-encoders` read. Requests in the
  meantime transcode with software.

## Fallback within a request

### Decision

In the switching ladder of `LiveTranscoder.Start`
([live-transcode-seek.md](live-transcode-seek.md#copy-path-and-gap-limit)), the
encode step has two rungs: the request's encoder, then software. If the hardware
FFmpeg ends without emitting first data, it restarts with `libx264` using the
same probe. The deadline is still the single `StartupDeadline`; expiry and
cancellation do not switch. A failure after first data does not switch either.

The switch is returned in `LiveTranscode.HardwareFailure` (an error including
the tail of FFmpeg's standard error), and the route logs it at `Warn` (video,
encoder, error). The response is 200 with software output, with the same shape
and headers. The failed encoder is not remembered; the next request tries the
configured encoder again.

### Trade-offs

- Hardware initialization failures (session limit, missing device, unsupported
  input) return an exit code immediately, so time remains for the software retry
  without a separate deadline.
- Session limits are often temporary, so a failure is not reflected in the
  settings state (what the screen shows).

## Encoder arguments

### Decision

The common part of `videoEncodeArgs` (the scale, pad, setsar and fps filters,
`-force_key_frames expr:gte(t,n_forced*2)`), audio and `-movflags` do not depend
on the encoder. Only the encoder specification (`encoderCodecArgs`) changes per
encoder. All produce H.264 High, Level 5.1, 4:2:0 8-bit, constant quality, with
forced keyframes as IDR.

| Encoder | Encoder specification |
| --- | --- |
| `software` | `-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23` |
| `nvenc` | `-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23 -b:v 0 -forced-idr 1` |
| `qsv` | `-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -global_quality 23 -look_ahead 0 -forced_idr 1` |
| `vaapi` | `-vaapi_device /dev/dri/renderD128`, `format=nv12,hwupload` at the end of the filter, `-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode CQP -qp 23` |
| `videotoolbox` | `-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -q:v 60 -realtime 1` |

A transcode with a quality (`domain.LiveTranscodeRequest.Quality`) adds the
quality's caps to the specification above (video cap `<cap>`, `-bufsize` twice
that). The values and the target dimensions are in
[playback-quality.md](playback-quality.md).

| Encoder | Specification with a quality |
| --- | --- |
| `software`, `nvenc` | Keep constant quality (`-crf 23` / `-cq 23`) and append `-maxrate <cap>k -bufsize <cap×2>k` |
| `qsv` | Replace `-global_quality 23` with `-b:v <cap>k -maxrate <cap>k -bufsize <cap×2>k` |
| `vaapi` | Replace `-rc_mode CQP -qp 23` with `-rc_mode VBR -b:v <cap>k -maxrate <cap>k -bufsize <cap×2>k` |
| `videotoolbox` | Replace `-q:v 60` with `-b:v <cap>k -maxrate <cap>k -bufsize <cap×2>k` |

Software and NVENC can combine constant quality with a cap, so quiet scenes come
out lighter than the cap. Whether QSV, VAAPI and VideoToolbox honour a cap on
top of constant quality depends on the driver, so they use VBR, where the cap
reliably applies. When hardware ends without first data and falls back to
software, the restart uses the same quality arguments.

Keyframes are IDR because `frag_keyframe` uses keyframes to cut fragments. A
non-IDR I-frame does not cut a fragment, which delays the first data. Decoding
is done in software for every encoder.

### Alternatives

- **Also use hardware decoding (`-hwaccel`)**: support differs widely by input
  format, so it was left out of this feature's scope.
- **Insert keyframes with `-g 60`**: the frame count after the fps filter thins
  frames drifts from time. The time-based `-force_key_frames` is used for every
  encoder.

## Settings API

`GET /api/settings/transcoding` and `PUT /api/settings/transcoding` (body
`{"videoEncoder": …}`) both return the same `TranscodingSettings` (choice,
encoder in use, `fallbackReason`, `checking`, and four check results with
reasons in the order `nvenc`, `qsv`, `vaapi`, `videotoolbox`) with
`Cache-Control: no-store`. They are owner-only routes.
`internal/httpapi/transcoding_settings.go` only parses the request and converts
to the contract shape; deciding the encoder and rejecting a choice are left to
`TranscodeSettings`.

| Case | Response |
| --- | --- |
| A value not in the enumeration, or a body that is not JSON | 400 `invalid_request` |
| An unusable hardware encoder (including while checking) | 409 `conflict`, reason `encoder_unavailable`; the saved value is unchanged |
| Save failure | 500 `internal` |

The source of truth for the contract is [api/openapi.yaml](../../api/openapi.yaml)
([specs/025-hardware-encoding/contracts/transcoding-settings-api.md](../../specs/025-hardware-encoding/contracts/transcoding-settings-api.md)).
