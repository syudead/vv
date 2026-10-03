# Research: Hardware encoding for live transcoding

Inherited decisions: the tech stack, the current live transcoding design (reusing
probe data, switching between copy and encode, keyframe interval), and the design
of the Settings page and API errors follow their sources of truth
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md),
[docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md),
[internal/media/transcode.go](../../internal/media/transcode.go),
[specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)).
This file records only the decisions this feature adds. Which ffmpeg encoders
exist and what options they take was confirmed with `-encoders` and
`-h encoder=…` on the development container's ffmpeg 6.1.1 (the Ubuntu 24.04
package, built with `--enable-libvpl` and with NVENC and VAAPI enabled).

## R-1: The bundled image stays on Alpine with software encoding only

**Decision**: The `Dockerfile` runtime stage stays on the same Alpine as before
#491 (`e5acc24`) and installs only `ffmpeg`, `ca-certificates` and `tzdata`. No GPU
driver or runtime library is added, and neither `compose.yaml` nor
`compose.hosting.yaml` gains a setting that passes a GPU through. Inside the
container, the startup check
([R-2](#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-in-parallel))
reports every hardware encoder as unavailable, and live transcoding runs in
software. Hardware encoding is used with VVMDM installed directly on the host
([R-9](#r-9-hardware-encoding-runs-on-a-direct-host-install-documented-with-prerequisites-and-steps)).

**Rationale**: Requirement 14 and the out-of-scope list of the parent Issue rule
out hardware encoding inside the bundled Docker image (GPU passthrough to the
container). The check and the software fallback do not depend on the OS or
environment, so without changing the image, the container just shows the
encoders as unavailable. Keeping the image also keeps the runtime base, the image
size, `HEALTHCHECK` and the dependency update process (Renovate) as they are.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Switch the runtime stage to Debian, install the Intel/AMD drivers, and pass the NVIDIA driver libraries through the NVIDIA Container Toolkit (the NVIDIA libraries link against glibc and cannot load on musl-based Alpine) | Rejected: out of scope in the parent Issue. |
| Install the `jellyfin-ffmpeg` deb | Rejected: same reason; it also brings in a third-party apt repository and key. |
| Publish a hardware-capable variant as a second image under a separate tag | Rejected: same reason. |

## R-2: The startup check runs a short real encode per encoder in parallel

**Decision**: `EncoderCheck` in `internal/media` runs, for each target encoder,
`ffmpeg -f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8 <R-7 encode arguments> -f null -`
and treats exit code 0 as "available". The targets depend on the OS:

| OS | Targets |
| --- | --- |
| linux | NVENC, Quick Sync, VAAPI |
| windows | NVENC, Quick Sync |
| darwin | VideoToolbox |
| Any other combination | Unavailable with `unsupported_os` |

| Situation | Result |
| --- | --- |
| The encoder name is missing from `ffmpeg -encoders` (read once first, within the same time limit) | `encoder_missing`, without running the encode |
| `-encoders` exceeds the time limit | Every target becomes `timed_out` and the result is returned (nothing stays "checking") |
| The real encode fails | `check_failed`; the tail of stderr is logged |
| The time limit (`encoderCheckTimeout`, 10 seconds per encoder) is exceeded | `timed_out` |
| Exit code 0 | `available` |

The checks run in parallel and do not hold up the HTTP listener ("checking" in
[R-3](#r-3-a-pure-domain-function-decides-the-effective-encoder-and-app-holds-it-in-memory)).
The check runs once at startup; there is no action on the screen to run it again
(out of scope in the parent Issue).

**Rationale**: Requirement 5 of the parent Issue asks to "actually try a short
encode". The presence of a name in `-encoders` cannot tell when an encoder is in
the build but the device or driver is missing (inside a Docker container, no
permission on `/dev/dri`, no NVIDIA libraries); only a real encode checks all of
that at once. The `lavfi` synthetic input exists in every build and needs no input
file. The checks run in parallel so that one hung encoder costs at most 10 seconds
overall (20 seconds worst case including reading `-encoders`). `-encoders` also
has a limit because if it hangs, the later checks never start and the screen keeps
reloading while "checking".

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Only the presence in `-encoders` | Rejected: see above. |
| Judge by the presence of device files (`/dev/dri/renderD128`, `/dev/nvidia*`) | Rejected: misses driver mismatches and session limits. |
| Run the checks in sequence | Rejected: 30 seconds worst case. |
| Delay startup until the check finishes | Rejected: contradicts the Edge Case "the startup check is slow or hangs". |

## R-3: A pure domain function decides the effective encoder and app holds it in memory

**Decision**: `internal/domain` holds the encoder value (`VideoEncoder`:
`software`, `nvenc`, `qsv`, `vaapi`, `videotoolbox`; the choice `EncoderChoice`
adds `auto`), each encoder's check result (`EncoderAvailability`: `checking` /
`available` / `unavailable` with a reason), and the pure function
`ResolveVideoEncoder(choice, availability)` that decides the encoder actually
used.

| Choice | Effective encoder |
| --- | --- |
| `software` | software |
| `auto` | The first available of `nvenc`, `qsv`, `vaapi`, `videotoolbox`, in that order. If none, software (not a fallback; requirement 7) |
| A hardware encoder | That encoder when available. Otherwise software, with `fallbackReason` `selected_unavailable` (checked and unavailable) or `checking` (check in progress) |
| A stored string it does not know | Treated as `software` (Edge Case "the stored value is unknown") |

`TranscodeSettings` in `internal/app` holds the stored value and the check results
in memory. `Current()` returns the current state (choice, effective encoder,
reason, whether checking, each encoder's result), and `Select(choice)` saves. The
transcode route puts the effective encoder from `Current()` into
`LiveTranscodeRequest` on every request, so a change applies from the next
request, and a transcode already streaming continues with the encoder it started
with (requirement 4). When the startup check ends and on every `Select`, app logs
the choice, the effective encoder and the reason (requirement 9).

**Rationale**: "Which one to use" is a rule decided only by its inputs (the
choice and the check results), so following ARCHITECTURE.md's layering it lives in
`internal/domain` and is tested without SQLite or ffmpeg. Check results are
rebuilt on every startup, so they are not stored. Each request reads the
in-memory state rather than the stored value, so no SQLite read is added to the
start of a transcode.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Write the effective encoder to SQLite | Rejected: it changes on every startup and mixes with requirement 8, which keeps the stored value unchanged. |
| The route reads the stored value every time | Rejected: adds a SQLite read to the start of a transcode. |
| Make the `auto` order a setting | Rejected: per-encoder fine-tuning is out of scope. |

## R-4: Storage is a generic key-value `settings` table

**Decision**: Add the table
`settings(key text primary key, value text not null, updated_at integer not null)`
and store the choice string under the key `transcode.video_encoder`
([data-model.md](data-model.md)). Reads and writes are `SettingsStore` methods;
no row means "not chosen", which is `software`. Interpreting the value (an
unknown value falls back to `software`) is done by `domain.ParseEncoderChoice`;
the store returns the string as it is.

**Rationale**: The stored value is a single string, and the requirement to keep an
unknown value stored as it is (Edge Case) is simpler with a string than with a
typed column. Future settings of the same kind ("values the owner picks on the
Settings page") can go here without a new table and migration. In
ARCHITECTURE.md's classification, it is user and settings data like
`media_folders`, and a scan does not restore it.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A one-row `transcode_settings` table | Rejected: every new item needs a column and a migration. |
| A dedicated table like `media_folders` | Rejected: there is one value, so no reason for a table of its own. |
| An environment variable | Rejected: excluded by requirement 1. |

## R-5: No change notification; the screen syncs on display and on the save response

**Decision**: A change of encoder adds no domain event and no `/api/events` event
type. The screen calls `GET` when it shows the section and replaces the display
with the `PUT` response (the whole state after saving). Only while "checking", it
calls `GET` again every few seconds until the check ends.

**Rationale**: The parent Issue's Edge Case "the encoder is changed from several
tabs or devices at once" accepts "the last save wins, and the display is correct
the next time it is shown or when the result of a change is received". Adding an
event type, a subscription and a screen subscription for one owner-only setting
costs more. "Checking" lasts only a few to a dozen-odd seconds after startup, so
reloading during that time is enough.

**Alternatives considered**: Add `domain.TranscodeSettingsChanged` and deliver it
over SSE (rejected, see above). Also deliver the end of the check over SSE
(rejected for the same reason).

## R-6: In-request fallback tries hardware then software at the encode step

**Decision**: The "encode" step of the fallback ladder in `LiveTranscoder.Start`
(live-transcode-seek.md `コピーの経路と差の上限`) becomes two steps. When the
effective encoder is hardware, the transcode starts with that encoder first; if it
ends without producing initial data (`errNoInitialData`), it restarts with
`libx264` on the same probe data.

| Situation | Behaviour |
| --- | --- |
| Deadline | One `StartupDeadline`, as before |
| Deadline exceeded or cancelled | No fallback |
| Fallback happened | `LiveTranscode` carries the encoder used and the hardware error; the route logs it with `Warn` (Edge Cases "hardware becomes unavailable after startup" and "concurrent session limit") |
| The request can copy (`videoCanCopy` and not `Normalize`) | Copies, as before, without looking at the encoder setting (requirement 12) |
| Failure after initial data | No fallback, as before |

**Rationale**: Requirement 11 of the parent Issue, "fall back to software within
the same request when it fails before producing initial data", is exactly the
mechanism already used for "copy → encode → probe on the spot" (when a process
ends without data, go to the next step). Hardware initialization failures (session
limit, no device, unsupported input) return an exit code at once, so software
still has time to retry without a separate deadline. A separate deadline would
push the total fallback time past the current limit (Edge Case "upper limit on
startup wait").

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A separate short limit (such as 3 seconds) for the hardware attempt | Rejected: for 4K input, software decoding alone can take more than 3 seconds for the first 2 seconds of output, cutting off attempts that would succeed. A hung encoder already fails on the deadline in the existing ladder. |
| Give the software retry a new deadline | Rejected: the total could reach 12 seconds. |
| Remember the failed encoder and skip it in later requests | Rejected: session limits are temporary and the next request often succeeds. Requirement 8 also accepts that the display keeps the startup result. |

## R-7: Encode arguments swap only the per-encoder codec options; common arguments keep the output guarantees

**Decision**: `videoEncodeArgs` branches on the encoder. The common part (filters:
scale, pad, setsar, fps; `-force_key_frames expr:gte(t,n_forced*2)`), audio and
`-movflags` do not change. `software` keeps today's arguments
(`-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23`)
unchanged to the character (acceptance criterion 1). For hardware, H.264 High,
Level 5.1, 4:2:0 8-bit, constant quality, and forced keyframes as IDR are given in
each encoder's spelling:

| Encoder | Arguments |
| --- | --- |
| `nvenc` | `-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23 -b:v 0 -forced-idr 1` |
| `qsv` | `-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -global_quality 23 -look_ahead 0 -forced_idr 1` |
| `vaapi` | `-vaapi_device /dev/dri/renderD128`, `format=nv12,hwupload` at the end of the filter chain, `-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode CQP -qp 23` |
| `videotoolbox` | `-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -q:v 60 -realtime 1` (on models where `-q:v` has no effect, the encoder's default bitrate applies) |

10-bit and unusual pixel-format inputs are reduced to 8-bit 4:2:0 by the
`format` / `-pix_fmt` option before reaching the encoder (Edge Case "input the
hardware encoder cannot handle"). Quick Sync and VAAPI take `nv12`, but the output
bitstream is 4:2:0 8-bit, the same as `yuv420p` from the browser's view. Input
above the resolution limit is scaled down first with the same `scale` as today.
The numbers (quality, preset) in this table are a starting point; the
implementation PR may tune them during the check for acceptance criterion 7
([quickstart.md](quickstart.md)). The output guarantees (profile, level, pixel
format, keyframe interval) do not change.

**Rationale**: Requirement 10 asks for the same output guarantees as software, and
the filters and keyframe options (already written independently of the encoder)
keep them. The only differences are each encoder's spelling (how the level is
written, `nv12` input, VAAPI's hwupload, the option that makes forced keyframes
IDR). Keyframes must be IDR because `frag_keyframe` uses keyframes as the marks
where fragments are cut; a non-IDR I-frame does not cut a fragment, which delays
the initial data.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Also use hardware decoding (`-hwaccel`) | Rejected: out of scope in the parent Issue. |
| Insert keyframes with `-g 60` alone | Rejected: frame count and time drift after the fps filter drops frames; #371 chose a time base. |
| Pass software frames to VAAPI as they are | Rejected: `h264_vaapi` accepts only hardware frames. |

## R-8: The settings API is GET and PUT on `/api/settings/transcoding`, returning the whole state

**Decision**: Add `GET /api/settings/transcoding` and
`PUT /api/settings/transcoding` (body `{ "videoEncoder": <choice> }`). Both return
the same `TranscodingSettings` (choice, effective encoder, fallback reason,
whether checking, each encoder's result and reason)
([contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)).
Owner only (not added to `accessRoutes`). A `PUT` that chooses an unavailable
encoder returns 409 `conflict` with reason `encoder_unavailable`. Reasons are
machine-readable codes, and the SPA's English catalog holds the wording (the 023
error-api approach).

**Rationale**: The screen needs three things: the stored value, the encoder in use
now, and whether each encoder is available. When the `PUT` response returns the
whole state, the R-5 display lines up without another `GET` after saving. The
route sits under `/api/settings/` so that future values of the same kind ("values
the owner picks on the Settings page") line up in the same place (`media-folders`
predates this namespace).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| `/api/transcoding-settings` | Rejected: see above. |
| `PATCH` | Rejected: there is one item, so partial update has no meaning. |
| A separate route for the encoder list | Rejected: the screen always shows both, so it adds a round trip. |

## R-9: Hardware encoding runs on a direct host install, documented with prerequisites and steps

**Decision**: Add a "Hardware encoding" section to `docs/how-to/running-vv.md`
that covers:

- Hardware encoding works when VVMDM is installed and run directly on the host
  (Windows, Linux, macOS). The bundled Docker image encodes in software only, and
  inside the container the Settings page shows every hardware encoder as
  unavailable
  ([R-1](#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only)).
- Prerequisites per encoder: the supported OS (the R-2 targets), the driver, the
  device (VAAPI's `/dev/dri/renderD128` and the group with permission on it), and
  an ffmpeg on PATH that includes that encoder (checked with
  `ffmpeg -hide_banner -encoders`).
- For the direct install, point to the existing procedure that builds and runs the
  single binary with `task build` (the toolchain in
  [docs/how-to/development.md](../../docs/how-to/development.md)), and to
  "Runtime settings" in the same document for runtime settings such as
  `MDM_DATA_DIR`.
- Turning it on in Settings, and how the startup check and the software fallback
  look.

No override example that passes a GPU to the container (`devices`, `group_add`,
NVIDIA Container Toolkit) is written. Nothing is added to `compose.yaml` or
`compose.hosting.yaml`. The Settings description points to this section (R-11).

**Rationale**: Requirement 14 and acceptance criterion 11 of the parent Issue ask
that this document makes clear that hardware encoding needs a direct install and
that Docker is software only, and that hardware encoding can be turned on in a
direct install. Whether a hardware encoder is present depends on the ffmpeg build
and the host driver, which VVMDM does not provide, so the way for users to check
(`-encoders`) is written next to the prerequisites.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Write an override example that passes the GPU to the container | Rejected: out of scope in the parent Issue; the image has no driver, so passing the GPU does not make it usable. |
| Add a distribution for direct installs (prebuilt binaries, an installer) | Rejected: not in the requirements and beyond this feature's scope. |

## R-10: Checks use tests with a substituted ffmpeg; on-hardware checks live in the quickstart

**Decision**: CI has no hardware encoder, so fallback, checking, saving, the API
and the screen are tested with a helper process that substitutes
`commandContext` (the existing pattern in `internal/media/transcode_test.go`) and
a fake checker. The on-hardware checks for acceptance criteria 2, 3, 5, 6, 7 and
11 follow [quickstart.md](quickstart.md) on a host with a GPU, and the results go
in the implementation PR body. That the software arguments do not change, and the
common output guarantees (rotation, aspect ratio, 4K downscaling, keyframe
interval), are confirmed by the existing ffmpeg tests passing unchanged.

**Rationale**: Do not make something that can only be confirmed on hardware look
as if CI confirmed it. Reuse the existing test patterns.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A self-hosted runner with a GPU | Rejected: cannot be provided within this feature. |
| Skip the on-hardware check | Rejected: requirement 10's guarantees depend on how each encoder's options are spelled, so they must be checked with ffprobe on hardware. |

## R-11: The Settings description links to the published documentation site section

**Decision**: The description of the Video conversion section has an external
link to the "Hardware encoding" section of `docs/how-to/running-vv.md` on the
GitHub Pages documentation site
(`https://syudead.github.io/vv/docs/how-to/running-vv#hardware-encoding`), opened
in a new tab. The URL is held once, as a constant in `web/src/settings`.

**Rationale**: The SPA does not serve the repository's Markdown, so a relative
link does not work. As described in
[docs/how-to/docs-site.md](../../docs/how-to/docs-site.md), `docs/` is published to
this site on every merge to `main` (with `cleanUrls`, so no `.md`), and the README
also points users there. Heading anchors are generated by the same rule as on
GitHub.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A GitHub blob URL | Rejected: it shows the `main` source, not the place published for users. |
| Bundle the documents into the SPA and serve them | Rejected: it introduces document serving, which is out of proportion to requirement 14's "the user can find the relevant part". |
