# Hardware encoding for live transcoding

- Status: Adopted
- Scope: for live transcoding (`GET /api/videos/{id}/transcode.mp4`), the choice of video encoder,
  the startup check, the fallback inside a request, the arguments per encoder, and the settings
  API (`GET`/`PUT /api/settings/transcoding`)
- History: [specs/025-hardware-encoding/](../../specs/025-hardware-encoding/plan.md) (parent
  Issue #370)

[live-transcode-seek.md](live-transcode-seek.md) covers how live transcoding starts (preparing the
probe data, switching between copy and encode). [mov-live-transcoding.md](mov-live-transcoding.md)
covers the two MOV inputs. This document covers only which encoder the video encode step uses.

## Encoders and resolving the encoder in use

### Context

Live transcoding encoded video with `libx264`. When the server has a GPU, NVENC, Quick Sync,
VAAPI or VideoToolbox lowers the CPU load. An encoder present in the ffmpeg build is still unusable
without its device or driver: inside the bundled Docker image, without permission on `/dev/dri`,
or without the NVIDIA libraries.

### Decision

The owner's choice (`domain.EncoderChoice`) is one of `software`, `nvenc`, `qsv`, `vaapi`,
`videotoolbox` and `auto`. `SettingsStore` saves it as a string under the key
`transcode.video_encoder` in the SQLite `settings` table. A missing row or an unknown string reads
as `software`, and the saved value is not rewritten (`domain.ParseEncoderChoice`).

The pure function `domain.ResolveVideoEncoder` picks the encoder in use (`domain.VideoEncoder`)
from the choice and the startup check results.

| Choice | Encoder in use | Reason |
| --- | --- | --- |
| `software` | software | none |
| `auto` | The first usable one in the order `nvenc`, `qsv`, `vaapi`, `videotoolbox`; software when none is usable | none (software here is not a fallback) |
| A hardware encoder, usable | That encoder | none |
| A hardware encoder, check still running | software | `checking` |
| A hardware encoder, checked and unusable | software | `selected_unavailable` |

`TranscodeSettings` in `internal/app` holds the choice and the check results in memory. The check
results are rebuilt on every start, so they are not saved.

- `Current()` returns the current state: the choice, the encoder in use, the reason, whether the
  check is running, and the result per encoder.
- `Select()` saves the choice and updates memory. For a hardware encoder that is not usable
  (checked and `available`), it returns `domain.ErrEncoderUnavailable` and leaves the saved value
  unchanged. It always accepts `software` and `auto`.
- When the check finishes, and on every `Select()`, it logs the choice, the encoder in use and the
  reason at `Info`.

The live transcoding route (`internal/httpapi/transcode.go`) puts the encoder in use from
`Current()` into `domain.LiveTranscodeRequest.VideoEncoder` on every request. A change applies from
the next request that starts. A transcode already streaming keeps the encoder it started with. No
restart is needed. A request whose video can be copied copies it, whatever the encoder.

### Trade-offs

- Each request reads the in-memory state, so starting a transcode adds no SQLite read.
- An encoder change is not pushed to the UI (no domain event, no `/api/events` kind). Another tab
  or device shows the correct state the next time it displays the section, or from the response to
  its own save.

## Startup check

### Decision

`cmd/mdm` reads the saved value, builds `TranscodeSettings`, logs the encoder line (choice, encoder
in use, whether checking), and then starts the check in a background goroutine. The HTTP listener
does not wait for the check. Until the check finishes, the encoder in use is software even when a
hardware encoder is chosen, and the settings API returns `checking: true`. A stop request cancels
the check and waits for it to end before stopping the scan and the workers.

The OS decides what is checked (`domain.HardwareEncoderCandidates`).

| OS | Encoders checked |
| --- | --- |
| linux | NVENC, Quick Sync, VAAPI |
| windows | NVENC, Quick Sync |
| darwin | VideoToolbox |

Any other OS and encoder pair is `unsupported_os`.

`EncoderCheck` in `internal/media` checks one encoder.

1. Read `ffmpeg -encoders` once and share the output among the parallel checks. An encoder whose
   name is missing gets `encoder_missing` without running anything. A read that exceeds the time
   limit gets `timed_out`.
2. Encode `-f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8` to `-f null -` with the same
   encode arguments as live transcoding. Exit code 0 gives `available`, a failure gives
   `check_failed`, and exceeding the time limit gives `timed_out`.

The checks per encoder run in parallel, and `TranscodeSettings` applies a time limit (10 s) to
each. A check that never returns becomes `timed_out` at the limit, so none stays "checking". The
tail of a failed check's stderr is logged at `Warn` and is not exposed through the API. The check
runs only once, at startup.

### Trade-offs

- The presence of a name in `-encoders` does not prove the device or driver exists, so the check
  encodes a short clip. The synthetic input (`lavfi`) exists in every build and needs no input
  file.
- Because the checks run in parallel, one hanging encoder costs at most about 20 s including the
  `-encoders` read. Requests during that time transcode with software.

## Fallback inside a request

### Decision

In the fallback ladder of `LiveTranscoder.Start`
([live-transcode-seek.md](live-transcode-seek.md#copy-path-and-offset-limit)), the encode step has two
rungs: "the request's encoder, then software". When the hardware FFmpeg ends without producing its
first data, it restarts with `libx264` on the same probe data. The deadline stays the single
`StartupDeadline`. Deadline expiry and cancellation do not fall back. A failure after the first
data does not fall back either.

The fallback is returned as `LiveTranscode.HardwareFailure` (an error that includes the tail of
FFmpeg's stderr), and the route logs it at `Warn` (video, encoder, error). The response is 200 with
the software output, and its shape and headers do not change. The failed encoder is not
remembered, so the next request again tries the configured encoder first.

### Trade-offs

- Hardware initialization failures (session limit, missing device, unsupported input) return an
  exit code at once, so the software retry has time left without a separate deadline.
- A session limit is often temporary, so a failure does not change the settings state (what the UI
  shows).

## Arguments per encoder

### Decision

The shared part of `videoEncodeArgs` (the scale, pad, setsar and fps filters, and
`-force_key_frames expr:gte(t,n_forced*2)`), the audio arguments and `-movflags` do not depend on
the encoder. Only the encoder arguments (`encoderCodecArgs`) change per encoder. Every encoder
produces H.264 High, Level 5.1, 4:2:0 8-bit, at constant quality, and makes forced keyframes IDR.

| Encoder | Encoder arguments |
| --- | --- |
| `software` | `-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23` |
| `nvenc` | `-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23 -b:v 0 -forced-idr 1` |
| `qsv` | `-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -global_quality 23 -look_ahead 0 -forced_idr 1` |
| `vaapi` | `-vaapi_device /dev/dri/renderD128`, `format=nv12,hwupload` at the end of the filter chain, `-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode CQP -qp 23` |
| `videotoolbox` | `-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -q:v 60 -realtime 1` |

Keyframes are IDR because `frag_keyframe` cuts fragments at keyframes. A non-IDR I-frame does not
cut a fragment, which delays the first data. Every encoder decodes in software.

### Alternatives

- **Also use hardware decoding (`-hwaccel`)**: support differs widely by input format, so it is
  out of scope for this feature.
- **Insert keyframes with `-g 60`**: after the fps filter drops frames, the frame count and the
  timestamps diverge. The time-based `-force_key_frames` is used for every encoder instead.

## Settings API

`GET /api/settings/transcoding` and `PUT /api/settings/transcoding` (body
`{"videoEncoder": …}`) both return the same `TranscodingSettings` with `Cache-Control: no-store`:
the choice, the encoder in use, `fallbackReason`, `checking`, and four check results with reasons
in the order `nvenc`, `qsv`, `vaapi`, `videotoolbox`. The routes are owner-only.
`internal/httpapi/transcoding_settings.go` only parses the request and maps to the contract shape;
`TranscodeSettings` resolves and rejects encoders.

| Case | Response |
| --- | --- |
| A value outside the enum, or a body that is not JSON | 400 `invalid_request` |
| A hardware encoder that is not usable (including while checking) | 409 `conflict`, reason `encoder_unavailable`; the saved value is unchanged |
| The save fails | 500 `internal` |

The contract source of truth is [api/openapi.yaml](../../api/openapi.yaml)
([specs/025-hardware-encoding/contracts/transcoding-settings-api.md](../../specs/025-hardware-encoding/contracts/transcoding-settings-api.md)).
