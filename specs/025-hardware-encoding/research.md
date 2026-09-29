# Research: hardware encoding for live transcode

The tech stack, the current live transcode design (reuse of probe data, the switch between copy
and encode, the keyframe interval), the settings screen and the API error design follow their
sources of truth:
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md),
[docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md),
[internal/media/transcode.go](../../internal/media/transcode.go),
[specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md).
This document records only the decisions this feature adds.

The presence and options of ffmpeg encoders were verified with `-encoders` and `-h encoder=…` on
the dev container's ffmpeg 6.1.1 (the Ubuntu 24.04 package; `--enable-libvpl`, NVENC and VAAPI
enabled).

## R-1: The bundled image stays on Alpine with software encoding only

| | |
| --- | --- |
| **Decision** | The `Dockerfile` runtime stage stays the same Alpine as before #491 (`e5acc24`) and installs only `ffmpeg`, `ca-certificates` and `tzdata`. No GPU drivers or runtime libraries are added, and `compose.yaml` and `compose.hosting.yaml` get no GPU passthrough settings. Inside the container, the startup check ([R-2](#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-concurrently)) reports every hardware encoder as unavailable, and live transcode runs with software. Hardware encoding is for VVMDM installed directly on the host ([R-9](#r-9-hardware-encoding-uses-a-direct-install-on-the-host-and-the-docs-describe-prerequisites-and-steps)). |
| **Why** | Parent Issue Requirement 14 and Out of scope exclude hardware encoding inside the bundled Docker image (GPU passthrough into the container). The check and the switch to software do not depend on the OS or environment, so without an image change the container only shows "unavailable". An unchanged image also keeps the runtime base, the image size, `HEALTHCHECK` and the dependency update process (Renovate) as they are. |
| **Rejected** | A Debian runtime stage with Intel/AMD drivers and NVIDIA libraries passed in by the NVIDIA Container Toolkit (the NVIDIA libraries link against glibc and do not load on musl Alpine; out of scope in the parent Issue). Installing the `jellyfin-ffmpeg` deb (same reason; it also brings a third-party apt repository and key). A second, hardware-enabled image under another tag (same reason). |

## R-2: The startup check runs a short real encode per encoder, concurrently

| | |
| --- | --- |
| **Decision** | `EncoderCheck` in `internal/media` runs `ffmpeg -f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8 <R-7 encode arguments> -f null -` per target encoder; exit code 0 means available. The OS decides the targets (see below). The check runs concurrently and does not delay HTTP listening (the "checking" state in [R-3](#r-3-a-pure-domain-function-decides-the-encoder-actually-used-and-app-holds-it-in-memory)). It runs once at startup; the screen has no re-check action (parent Issue Out of scope). |
| **Why** | Parent Issue Requirement 5 asks for an actual short encode. `-encoders` alone cannot tell a build that includes the encoder but lacks the device or driver (inside a Docker container, no permission on `/dev/dri`, missing NVIDIA libraries); only a real encode checks all of it at once. The `lavfi` synthetic input exists in every build and needs no input file. Concurrency keeps the total wait at 10 s even when one encoder hangs (20 s worst case including reading `-encoders`). `-encoders` also has a time limit because a hang there would block the later checks and leave the screen reloading in the checking state. |
| **Rejected** | `-encoders` alone (above). Checking for device files (`/dev/dri/renderD128`, `/dev/nvidia*`): misses driver mismatches and session limits. Sequential runs: 30 s worst case. Delaying startup until the check ends: violates the edge case "the startup check is slow or hangs". |

Check targets and results:

| Item | Rule |
| --- | --- |
| Targets per OS | linux: NVENC, Quick Sync, VAAPI. windows: NVENC, Quick Sync. darwin: VideoToolbox. Any other pair is unavailable with `unsupported_os`. |
| `ffmpeg -encoders` | Read once first, within the same time limit. An encoder not named there is not run and is `encoder_missing`. |
| `-encoders` over the time limit | Every check target becomes `timed_out` and the result returns (it does not stay in "checking"). |
| Real encode fails | `check_failed`; the tail of stderr goes to the log. |
| Time limit exceeded | `timed_out`. The limit is `encoderCheckTimeout`, 10 s per encoder. |

## R-3: A pure domain function decides the encoder actually used, and app holds it in memory

| | |
| --- | --- |
| **Decision** | `internal/domain` holds the encoder values, the check results and the pure function `ResolveVideoEncoder(choice, availability)` that decides the encoder actually used (rules below). `TranscodeSettings` in `internal/app` holds the saved value and the check results in memory. `Current()` returns the current state (choice, encoder actually used, reason, whether checking, per-encoder results); `Select(choice)` saves. |
| **Why** | Which encoder to use is a rule decided only by its inputs (choice and check results), so it belongs in `internal/domain` per the ARCHITECTURE.md layering, and tests need neither SQLite nor ffmpeg. The check results are rebuilt on every start, so they are not stored. Each request reads the in-memory state, not the saved value, so starting a transcode adds no SQLite read. |
| **Rejected** | Writing the encoder actually used to SQLite: it changes on every start and would mix with Requirement 8, which keeps the saved value unchanged. The path reads the saved value every time: adds a SQLite read to transcode start. Making the `auto` order a setting: per-encoder fine-grained settings are out of scope. |

Values in `internal/domain`:

- `VideoEncoder`: `software`, `nvenc`, `qsv`, `vaapi`, `videotoolbox`. The choice `EncoderChoice`
  adds `auto` to these.
- `EncoderAvailability`: per-encoder check result, `checking` / `available` / `unavailable` with a
  reason.

Rules of `ResolveVideoEncoder`:

- `software` → software.
- `auto` → the first available encoder in the order `nvenc`, `qsv`, `vaapi`, `videotoolbox`. With
  none, software (not a fallback; Requirement 7).
- A hardware encoder → that encoder when available. Otherwise software, with `fallbackReason`
  `selected_unavailable` (checked and unavailable) or `checking` (still checking).
- An unknown saved string is treated as `software` (edge case "the saved value is unknown").

Behavior of `TranscodeSettings`:

- For each request, the transcode path puts the encoder actually used from `Current()` on
  `LiveTranscodeRequest`. A change applies from the next request, and a streaming transcode keeps
  the encoder it started with (Requirement 4).
- When the startup check ends, and on every `Select`, app logs the choice, the encoder actually
  used and the reason (Requirement 9).

## R-4: Storage is a generic `settings` table (key-value)

| | |
| --- | --- |
| **Decision** | Add the table `settings(key text primary key, value text not null, updated_at integer not null)` and save the choice string under the key `transcode.video_encoder` ([data-model.md](data-model.md)). `SettingsStore` methods read and write it; no row means "no selection" = `software`. `domain.ParseEncoderChoice` interprets the value (an unknown value falls back to `software`); the store returns the string as is. |
| **Why** | The stored value is one string, and the requirement to keep an unknown value stored (edge case) is simpler with a string than with a typed column. Future settings of the same kind (values the owner selects on the settings screen) fit without a new table or migration. In the ARCHITECTURE.md categories it is user and settings data like `media_folders`, and a scan does not restore it. |
| **Rejected** | A one-row `transcode_settings` table: every new item needs a column and a migration. A dedicated table like `media_folders`: one value gives no reason for a table. An environment variable: excluded by Requirement 1. |

## R-5: No change notification; the screen syncs on display and from the save response

| | |
| --- | --- |
| **Decision** | An encoder change adds no domain event and no `/api/events` type. The screen calls `GET` when it shows the section, and replaces the display with the `PUT` response (the full state after saving). Only while checking, it calls `GET` again every few seconds until the check ends. |
| **Why** | The parent Issue edge case "changing the encoder from several tabs or devices at once" accepts "the later save wins, and the display is correct on the next display or on receiving the change result". Adding an event type, a subscription and a screen subscription for one owner-only setting costs more. Checking lasts only a few to a dozen or so seconds after startup, so reloading during that time is enough. |
| **Rejected** | Adding `domain.TranscodeSettingsChanged` and delivering it over SSE (above). Delivering the check completion over SSE (same reason). |

## R-6: Switching inside a request tries hardware, then software, at the encode step

| | |
| --- | --- |
| **Decision** | Split the "encode" step of the `LiveTranscoder.Start` fallback ladder (live-transcode-seek.md, section "Copy path and gap limit") into two steps. When the encoder actually used is hardware, start with that encoder; when it ends without first data (`errNoInitialData`), restart with `libx264` using the same probe data. Details are below. |
| **Why** | Parent Issue Requirement 11, "switch to software inside the same request when it fails before the first data", is the mechanism the "copy → encode → on-the-spot probe" ladder already uses (move to the next step when the process ends without data). Hardware initialization failures (session limit, no device, unsupported input) return an exit code at once, so software has time left without a separate deadline. An extra deadline would push the total switch time past the current limit (edge case "limit on startup wait"). |
| **Rejected** | A separate short limit for the hardware attempt (such as 3 s): for 4K input, software decoding alone can take over 3 s for the first 2 s of output, which would cut successful attempts; the existing ladder already fails a hanging encoder at the deadline. A new deadline for the software retry: the total reaches up to 12 s. Remembering a failed encoder and skipping it in later requests: session limits are temporary and the next request often succeeds; Requirement 8 also accepts the display staying at the startup result. |

Details of the switch:

- The deadline stays one `StartupDeadline`. A deadline expiry or a cancellation does not switch.
- The switch is returned on `LiveTranscode` (the encoder used and the hardware error), and the
  path logs it with `Warn` (edge cases "hardware becomes unavailable after startup" and
  "concurrent session limit").
- A request that can copy (`videoCanCopy` and not `Normalize`) copies as before and ignores the
  encoder setting (Requirement 12).
- A failure after the first data does not switch, as before.

## R-7: Encode arguments replace only the per-encoder codec options; shared arguments keep the output contract

| | |
| --- | --- |
| **Decision** | `videoEncodeArgs` branches by encoder. The shared parts do not change: the filter (downscale, pad, setsar, fps), `-force_key_frames expr:gte(t,n_forced*2)`, audio and `-movflags`. `software` keeps the current arguments (`-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23`) character for character (Acceptance criterion 1). Hardware encoders get H.264 High, Level 5.1, 4:2:0 8-bit, constant quality and IDR on forced keyframes, in each encoder's own spelling (below). |
| **Why** | Requirement 10 asks for the same output contract as software. The filter and keyframe arguments, already written independent of the encoder, keep it. The only differences are each encoder's spelling (the level syntax, `nv12` input, VAAPI hwupload, the option that makes forced keyframes IDR). IDR matters because `frag_keyframe` cuts fragments at keyframes; a non-IDR I-frame does not cut a fragment and delays the first data. |
| **Rejected** | Also using hardware decoding (`-hwaccel`): parent Issue Out of scope. Inserting keyframes with `-g 60` only: frame count and time drift after the fps filter drops frames, and #371 chose a time-based interval. Passing software frames to VAAPI: `h264_vaapi` accepts only hw frames. |

| Encoder | Arguments |
| --- | --- |
| `nvenc` | `-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23 -b:v 0 -forced-idr 1` |
| `qsv` | `-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -global_quality 23 -look_ahead 0 -forced_idr 1` |
| `vaapi` | `-vaapi_device /dev/dri/renderD128`, `format=nv12,hwupload` at the end of the filter, `-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode CQP -qp 23` |
| `videotoolbox` | `-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -q:v 60 -realtime 1` (on models where `-q:v` has no effect, the encoder's default bitrate applies) |

- 10-bit input and unusual pixel formats are reduced to 8-bit 4:2:0 by the `format` / `-pix_fmt`
  argument before reaching the encoder (edge case "input a hardware encoder cannot handle").
- Quick Sync and VAAPI take `nv12`, but the output bitstream is 4:2:0 8-bit, the same as
  `yuv420p` from the browser's view.
- Input above the resolution limit is downscaled first by the same `scale` as today.
- The numbers (quality, preset) in the table are a starting point. The implementation PR may tune
  them during the Acceptance criterion 7 check ([quickstart.md](quickstart.md)). The output
  contract (profile, level, pixel format, keyframe interval) does not change.

## R-8: The settings API is GET and PUT on `/api/settings/transcoding`, returning the full state

| | |
| --- | --- |
| **Decision** | Add `GET /api/settings/transcoding` and `PUT /api/settings/transcoding` (body `{ "videoEncoder": <choice> }`). Both return the same `TranscodingSettings`: choice, encoder actually used, fallback reason, whether checking, and per-encoder results with reasons ([contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)). Owner only (not added to `accessRoutes`). A `PUT` that selects an unavailable encoder gets 409 `conflict` with reason `encoder_unavailable`. Reasons are machine-readable codes; the SPA's English catalog owns the text (the 023 error-api policy). |
| **Why** | The screen needs three things: the saved value, the encoder in use now, and whether each encoder is available. When the `PUT` response returns the full state, the R-5 display is consistent without a second `GET`. The path sits under `/api/settings/` so that future values of the same kind (values the owner selects on the settings screen) line up in one place (`media-folders` predates this namespace). |
| **Rejected** | `/api/transcoding-settings` (above). `PATCH`: one field, so partial update has no meaning. A separate path for the encoder list: the screen always shows both, so it adds a round trip. |

## R-9: Hardware encoding uses a direct install on the host, and the docs describe prerequisites and steps

| | |
| --- | --- |
| **Decision** | Add a "Hardware encoding" section to `docs/how-to/running-vv.md` with the content below. Do not document an override that passes a GPU into the container (`devices`, `group_add`, NVIDIA Container Toolkit). Add nothing to `compose.yaml` or `compose.hosting.yaml`. The settings screen description links to this section (R-11). |
| **Why** | Parent Issue Requirement 14 and Acceptance criterion 11 ask that this doc makes clear that hardware encoding needs a direct install and that Docker is software only, and that a reader can enable it on a direct install. The ffmpeg build and the host drivers decide whether hardware encoders exist, and VVMDM does not provide them, so the doc lists the way to check (`-encoders`) next to the prerequisites. |
| **Rejected** | Documenting an override that passes a GPU into the container: parent Issue Out of scope; the image has no drivers, so passing the GPU does not work. Adding direct-install distributions (prebuilt binaries, an installer): not required and beyond this feature's scope. |

Content of the section:

- Hardware encoding works when VVMDM is installed and run directly on the host (Windows, Linux,
  macOS). The bundled Docker image is software encoding only, and inside the container the
  settings screen shows every hardware encoder as unavailable
  ([R-1](#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only)).
- Per-encoder prerequisites: supported OS (the R-2 check targets), drivers, devices (VAAPI's
  `/dev/dri/renderD128` and the group with permission on it), and an ffmpeg on PATH that includes
  that encoder (check with `ffmpeg -hide_banner -encoders`).
- For the direct install steps, a pointer to the existing steps that build and run the single
  binary with `task build` (the toolchain in
  [docs/how-to/development.md](../../docs/how-to/development.md)). Runtime settings such as
  `MDM_DATA_DIR` point to "Runtime settings" in the same doc.
- Enabling it on the settings screen, and how the startup check and the switch to software
  appear.

## R-10: Tests replace ffmpeg; the real-hardware check lives in the quickstart

| | |
| --- | --- |
| **Decision** | CI has no hardware encoders. Switching, checking, storage, API and screen are tested with a helper process that replaces `commandContext` (the existing form in `internal/media/transcode_test.go`) and a fake checker. The real-hardware check for Acceptance criteria 2, 3, 5, 6, 7 and 11 follows [quickstart.md](quickstart.md) on a host with a GPU, and the results go in the implementation PR body. The existing ffmpeg tests pass unchanged, which confirms that the software arguments did not change and that the shared output contract (rotation, aspect ratio, 4K downscale, keyframe interval) holds. |
| **Why** | CI must not appear to pass what only real hardware can verify. The existing test forms are reused as is. |
| **Rejected** | A self-hosted runner with a GPU: cannot be provided within this feature. Skipping the real-hardware check: the Requirement 10 contract depends on the spelling of encoder options, so ffprobe on real hardware is necessary. |

## R-11: The settings screen description links to the section on the public docs site

| | |
| --- | --- |
| **Decision** | The description of the "Video conversion" section has an external link to the "Hardware encoding" section of `docs/how-to/running-vv.md` on the GitHub Pages docs site (`https://syudead.github.io/vv/docs/how-to/running-vv#hardware-encoding`), opened in a new tab. The URL lives in one constant in `web/src/settings`. |
| **Why** | The SPA does not serve the repository's Markdown, so a relative link does not work. Per [docs/how-to/docs-site.md](../../docs/how-to/docs-site.md), `docs/` is published to this site on every merge to `main` (`cleanUrls`, so no `.md`), and the README also sends users there. Heading anchors follow the same rules as GitHub. |
| **Rejected** | A GitHub blob URL: shows the `main` source, not the user-facing publication. Bundling and serving the docs in the SPA: introduces document serving, excessive for Requirement 14's "the relevant section is clear". |
