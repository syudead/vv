# Implementation Plan: Hardware encoding for live transcoding

**Branch**: `feature/025-hardware-encoding` | **Parent Issue**: #370

**Input**: The parent Issue. It is this feature's specification.

## Summary

Live transcoding (`GET /api/videos/{id}/transcode.mp4`) encodes video with the
server's hardware encoders (NVENC, Quick Sync, VAAPI, VideoToolbox) as well as
`libx264`. The owner picks the encoder (software, one of the hardware encoders, or
automatic) in a new Settings section, Video conversion. The choice is stored in
SQLite and applies from the next transcode request without a restart.

| Part | Approach |
| --- | --- |
| Deciding the encoder | A pure function in `internal/domain` decides the effective encoder from the stored choice and the startup check results, and `TranscodeSettings` in `internal/app` holds it in memory ([research.md R-3](research.md#r-3-a-pure-domain-function-decides-the-effective-encoder-and-app-holds-it-in-memory), [R-4](research.md#r-4-storage-is-a-generic-key-value-settings-table)). |
| Startup check | Short real encodes per encoder run in parallel without holding up the HTTP listener ([R-2](research.md#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-in-parallel)). |
| Transcoding | `internal/media` builds the codec arguments per encoder; common filters and keyframe options keep the output guarantees. When hardware fails before producing initial data, the same request switches to software ([R-6](research.md#r-6-in-request-fallback-tries-hardware-then-software-at-the-encode-step), [R-7](research.md#r-7-encode-arguments-swap-only-the-per-encoder-codec-options-common-arguments-keep-the-output-guarantees)). |
| API and screen | `GET`/`PUT /api/settings/transcoding` ([contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)) and the Settings section. The look and interaction criteria are the parent Issue's `UI品質` section as written (no `ui` label, so no design stage). |
| Bundled image and documents | The bundled Docker image stays on Alpine with software encoding only, as before #491, and inside the container the startup check reports every hardware encoder as unavailable. Hardware encoding is used with VVMDM installed directly on the host (Windows, Linux, macOS), and the documents describe the prerequisites and steps ([R-1](research.md#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only), [R-9](research.md#r-9-hardware-encoding-runs-on-a-direct-host-install-documented-with-prerequisites-and-steps)). |

## Technical Context

**Canonical definitions**:

- Boundaries and dependency direction, the settings data classification, the
  authentication boundary: [ARCHITECTURE.md](../../ARCHITECTURE.md) ("Intended
  dependency direction", "Rebuildable and user data", the authentication
  paragraph)
- Today's live transcoding:
  [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md),
  [docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md),
  [internal/media/transcode.go](../../internal/media/transcode.go) (the fallback
  ladder in `Start`, `buildTranscodeArgs`, `videoEncodeArgs`),
  [internal/httpapi/transcode.go](../../internal/httpapi/transcode.go),
  [internal/domain/live_transcode.go](../../internal/domain/live_transcode.go)
- Startup order and wiring: [cmd/mdm/main.go](../../cmd/mdm/main.go),
  [internal/media/preflight.go](../../internal/media/preflight.go)
- How settings work today:
  [internal/store/media_folders.go](../../internal/store/media_folders.go)
  (`SettingsStore`), [internal/app/media_folders.go](../../internal/app/media_folders.go),
  [internal/httpapi/media_folders.go](../../internal/httpapi/media_folders.go),
  [web/src/settings/SettingsPage.tsx](../../web/src/settings/SettingsPage.tsx),
  [web/src/settings/ScanStatusSection.tsx](../../web/src/settings/ScanStatusSection.tsx)
- API source of truth and error shape: [api/openapi.yaml](../../api/openapi.yaml),
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md);
  screen text: [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- Bundled image and operations documents: [Dockerfile](../../Dockerfile),
  [compose.yaml](../../compose.yaml),
  [compose.hosting.yaml](../../compose.hosting.yaml),
  [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md),
  [docs/how-to/hosting-vv.md](../../docs/how-to/hosting-vv.md),
  [.github/workflows/ci.yml](../../.github/workflows/ci.yml) (publishes
  `linux/amd64,linux/arm64`)
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`,
  `task check-docs`, `task generate`)

**Feature-specific context**:

- No Go or npm dependency is added. The `Dockerfile` and compose files keep their
  content from before #491 (`e5acc24`) (R-1). The prerequisites for hardware
  encoding (drivers, an ffmpeg that includes the hardware encoders) are provided
  by the user on the host where VVMDM is installed directly (R-9).
- SQLite gains one table ([data-model.md](data-model.md)). The migration takes the
  next number in `internal/store/migrations`.
- CI has no hardware encoder. On-hardware checks run with VVMDM installed directly
  on a host with a GPU, following [quickstart.md](quickstart.md); automated tests
  substitute ffmpeg (R-10).
- Requirement 10 of the parent Issue includes the keyframe interval of 2 seconds
  or less in output time from #371 (merged to `main`). The `-force_key_frames`
  option is common to every encoder, so it keeps holding (R-7).
- The copy path decided by #371 (`videoCanCopy`, `CopySeekAllowance`) does not
  change. The encoder setting applies only to requests that encode
  (requirement 12).

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"):
  pass.

  | Package | Role in this feature |
  | --- | --- |
  | `internal/domain` | Encoder values, interpreting the choice, the rule that decides the effective encoder, the check targets per OS. Pure functions and enums only; `os/exec` and `runtime` values come in as arguments. |
  | `internal/media` | Per-encoder arguments, the real encode for checking, the in-request fallback. Holds neither the store nor a logger, and returns the fact of a fallback as a value (R-6). |
  | `internal/app` | Holds the stored value and check results, runs the check, writes logs. Receives the store and the checker through interfaces it declares. |
  | `internal/store` | Only reads and writes the `settings` table. |
  | `internal/httpapi` | Only interprets requests and converts to `gen` types. Asks app for the encoder decision. |
  | `cmd/mdm` | Wiring, and starting the startup check goroutine. |

  Sibling packages do not import each other (the depguard rules do not change).
- **API source of truth** (ARCHITECTURE.md): pass. Change `api/openapi.yaml` and
  run `task generate`; generated files are not hand-edited (AGENTS.md).
- **Authentication boundary** (the authentication paragraph of ARCHITECTURE.md,
  requirement 13): pass. The new routes are not added to `accessRoutes`, so they
  are owner-only, and `openapi_routes_test.go` checks that they match `security`.
  The Settings page is already owner-only, so guests do not see the section.
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): pass.
  `settings` is settings data and is added to the list.
- **Domain events** (the events paragraph of ARCHITECTURE.md): not applicable. A
  change of encoder publishes no event (R-5).
- **Server output is English** (gosmopolitan in `.golangci.yml`, the 023
  approach): pass. Logs, `message` and reason codes are English, and the catalog
  holds the screen text.
- **Documents change in the same PR** (core-beliefs.md, AGENTS.md): pass. Each unit
  updates ARCHITECTURE.md, the design documents and the how-to guides.

The verdicts are the same after Phase 1. Complexity Tracking has no violation to
list.

## Project Structure

### Documentation (this feature)

```text
specs/025-hardware-encoding/
├── plan.md                              # This file
│                                        # No spec.md — the parent Issue is the specification
├── research.md                          # Runtime environment, check, decision rule, storage, fallback, arguments, API, documents, testing, link to documents
├── data-model.md                        # settings table and in-memory values
├── quickstart.md                        # On-hardware check on a host with a GPU
└── contracts/
    └── transcoding-settings-api.md      # GET/PUT /api/settings/transcoding
```

There is no `ui-design.md` (no `ui` label). The screen criteria are in the parent
Issue's `UI品質` section.

### Source Code

**Affected boundaries**:

| Boundary | What changes |
| --- | --- |
| `internal/domain` | `VideoEncoder`, `EncoderChoice`, `EncoderAvailability`, `TranscodeEncoding`; `ParseEncoderChoice`, `ResolveVideoEncoder`, `HardwareEncoderCandidates`; `LiveTranscodeRequest.VideoEncoder`; the encoder used and the fact of a fallback on `LiveTranscode` |
| `internal/store` | Migration; `SettingsStore` reads and writes |
| `internal/app` | `TranscodeSettings` |
| `internal/media` | Per-encoder arguments, `EncoderCheck`, the `Start` ladder |
| `api/openapi.yaml`, `internal/httpapi` (new routes, `transcode.go`, `requiresJSONBody`), `cmd/mdm/main.go` | API and wiring |
| `web/src/api`, `web/src/settings`, `web/src/i18n` | Screen |
| `docs/how-to/running-vv.md`, `docs/how-to/hosting-vv.md`, `ARCHITECTURE.md`, `docs/design-docs/` | Documentation. `Dockerfile`, `compose.yaml` and `compose.hosting.yaml` keep their content from before #491 (`e5acc24`) and are not part of this feature's diff. |

**New paths**:

- `internal/store/migrations/000NN_settings.sql` (the next number at
  implementation time)
- `internal/domain/video_encoder.go`
- `internal/app/transcode_settings.go`
- `internal/media/encoder_check.go`
- `internal/httpapi/transcoding_settings.go`
- `web/src/settings/TranscodingSection.tsx`
- `docs/design-docs/hardware-encoding.md` (the current design of encoder
  selection, checking, fallback and arguments; listed in
  `docs/design-docs/index.md`)

**Structure decision**: Follows the existing layout
([ARCHITECTURE.md](../../ARCHITECTURE.md)). The encoder decision and the check
results live in `TranscodeSettings` in `internal/app`, because the values span
memory (check results) and SQLite (the choice), and, like `MediaFolders`, it is a
use case behind a Settings page action. Having httpapi read the store directly and
decide was rejected, because the decision rule and the check results would move
into the HTTP layer. Having media read the store was rejected because it goes
against the dependency direction.

## Implementation Work

### Store the live transcoding video encoder and decide the effective encoder from startup check results

**Scope**: Encoder values and decision rule, storage, app state.

- `internal/domain`: the values of [data-model.md, Key `transcode.video_encoder`](data-model.md#key-transcodevideo_encoder) and [In-memory values (not stored)](data-model.md#in-memory-values-not-stored), and
  `ParseEncoderChoice`, `ResolveVideoEncoder` and `HardwareEncoderCandidates`
  ([R-3](research.md#r-3-a-pure-domain-function-decides-the-effective-encoder-and-app-holds-it-in-memory)).
- `internal/store`: the migration of [data-model.md, Migration](data-model.md#migration), and
  `TranscodeEncoderChoice` and `SaveTranscodeEncoderChoice` on `SettingsStore`
  ([R-4](research.md#r-4-storage-is-a-generic-key-value-settings-table)).
- `internal/app`: `TranscodeSettings` (loading the stored value, running the
  checks in parallel through the checker interface with a per-encoder time limit,
  `Current`, `Select`, logging at startup and on change;
  [R-2](research.md#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-in-parallel)
  and R-3). `Select` returns `domain.ErrEncoderUnavailable` for an unavailable
  encoder.
- Documentation: add `settings` to "Rebuildable and user data" in ARCHITECTURE.md,
  and `TranscodeSettings` and settings reads and writes to the descriptions of
  `internal/app` and `SettingsStore`.

**Dependencies**: None.

**Acceptance**: The following checks exist, and `task check` and
`task check-docs` pass.

- `internal/domain` tests: no choice and an unknown value become `software`.
  `auto` picks an available encoder in the order `nvenc`, `qsv`, `vaapi`,
  `videotoolbox`, and when none is available gives `software` with no
  `fallbackReason`. An unavailable hardware encoder gives `software` with
  `selected_unavailable`, or `checking` while the check runs. The check targets
  per OS (linux, windows, darwin, other).
- `internal/store` tests: no row means not chosen. A saved value reads back.
  Saving twice keeps the later value.
- `internal/app` tests (fake checker): while checking, `Current()` is `checking`
  with `software`. When the check ends, the results apply, and the log shows the
  choice, the effective encoder and the reason. When one check exceeds the time
  limit, the other results still appear and that encoder is `timed_out`. `Select`
  rejects an unavailable encoder and leaves the stored value unchanged.

### Transcode with ffmpeg hardware encoders, with a short check encode and fallback to software

**Scope**: Changes in `internal/media`.

- `videoEncodeArgs` branches on the encoder. The `software` arguments do not
  change
  ([R-7](research.md#r-7-encode-arguments-swap-only-the-per-encoder-codec-options-common-arguments-keep-the-output-guarantees)).
- Accept `LiveTranscodeRequest.VideoEncoder`, and make the encode step of the
  `Start` ladder two steps, "hardware → software". `LiveTranscode` carries the
  encoder used and, after a fallback, the hardware error
  ([R-6](research.md#r-6-in-request-fallback-tries-hardware-then-software-at-the-encode-step)).
- `EncoderCheck`: reading `-encoders` with a time limit, and a short real encode
  per encoder
  ([R-2](research.md#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-in-parallel)).
  It satisfies the checker interface from the previous unit.

**Dependencies**: `Store the live transcoding video encoder and decide the effective encoder from startup check results`
(uses `domain.VideoEncoder`, `EncoderAvailability` and the checker interface).

**Acceptance**: The following checks exist, and `task check` passes.

- Argument tests: the `software` arguments match the current test expectations
  character for character. Each hardware encoder has `-c:v h264_<encoder>`, High
  and Level 5.1, conversion to 4:2:0 8-bit, the `-force_key_frames` option, VAAPI's
  `hwupload`, and the MOV dual input as before. A request that can copy gets
  `-c:v copy` whatever the encoder.
- Helper process tests: when hardware ends without producing initial data, the
  same request starts with `libx264`, and `LiveTranscode` carries the fact of the
  fallback. A deadline or cancellation does not fall back. A failure after the
  initial data does not fall back.
- `EncoderCheck` tests: an encoder missing from `-encoders` is `encoder_missing`
  without running. When `-encoders` hangs past the time limit, every target
  becomes `timed_out` and the result is returned. A failed real encode is
  `check_failed`, a hanging helper is `timed_out` at the time limit, and success is
  `available`.
- The existing ffmpeg tests (rotation, aspect ratio, 4K, keyframe interval, MOV,
  cleanup on disconnect) pass unchanged.

### Connect the settings API and the live transcode route so encoder changes apply without a restart

**Scope**: API, route, wiring, design document.

- Add the types and routes of
  [contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)
  to `api/openapi.yaml` and run `task generate`. Add `encoder_unavailable` to the
  error reasons.
- `internal/httpapi`: `getTranscodingSettings`, `updateTranscodingSettings`,
  `requiresJSONBody`. `transcode.go` puts the effective encoder from
  `TranscodeSettings.Current()` on every request, and when a fallback happens,
  logs `Warn` (video, encoder, tail of the ffmpeg error).
- `cmd/mdm/main.go`: create `TranscodeSettings`, load the stored value, start the
  check goroutine without holding up the listener, and stop it on shutdown.
- Documentation: write `docs/design-docs/hardware-encoding.md`, and update
  `docs/design-docs/index.md` and ARCHITECTURE.md (the live transcoding
  paragraph, the settings API).

**Dependencies**:

- `Store the live transcoding video encoder and decide the effective encoder from startup check results`
- `Transcode with ffmpeg hardware encoders, with a short check encode and fallback to software`

**Acceptance**: The following checks exist, `task check` and `task check-docs`
pass, and `task generate` produces no diff.

- `internal/httpapi` tests: `GET` returns the contract shape. `PUT` saves
  `software`, `auto` and an available encoder and reflects them in the response.
  An unavailable encoder returns 409 `encoder_unavailable`, and a value outside the
  enum returns 400. A guest gets 401 / 403, and `openapi_routes_test.go` passes.
- `internal/httpapi/transcode_test.go` (helper process): after the encoder
  changes, requests start with `-c:v h264_<encoder>`. A request started before the
  change continues as it was. A request with an injected hardware failure returns
  200 with a body, and the `Warn` log appears (acceptance criterion 8).
- `cmd/mdm` tests: `/api/health` responds before the check ends. The startup log
  has the encoder line.

### Add the Video conversion section to Settings for choosing an encoder and showing which encoders are available

**Scope**: The Settings section. The criteria are the parent Issue's `UI品質`
section and requirements 1, 6 and 8.

- `web/src/api/client.ts`: `getTranscodingSettings`, `updateTranscodingSettings`.
- `web/src/settings/TranscodingSection.tsx`:
  - Content: heading; description (with an external link to the "Hardware
    encoding" section of `docs/how-to/running-vv.md` on the published
    documentation site;
    [R-11](research.md#r-11-the-settings-description-links-to-the-published-documentation-site-section));
    the encoder in use now; the choices (radio buttons, one per line, with encoder
    name and state).
  - An unavailable encoder cannot be selected and shows its reason.
  - While checking, show "Checking…" and reload every few seconds until the check
    ends
    ([R-5](research.md#r-5-no-change-notification-the-screen-syncs-on-display-and-on-the-save-response)).
  - Selecting saves at once. While saving, show "Saving…" and prevent a second
    change. On failure, revert the selection and show the reason inside the
    section.
  - The `fallbackReason` warning appears in `text-warning` above the choices.
  - Place it in `SettingsPage` with the same spacing as `ScanStatusSection` and
    Media folders (`mt-8`, a divider under the heading).
- `web/src/i18n/en.ts`: the section text, the encoder names, the reasons
  (`unsupported_os`, `encoder_missing`, `check_failed`, `timed_out`, `checking`),
  and the text for reason `encoder_unavailable`.

**Dependencies**: `Connect the settings API and the live transcode route so encoder changes apply without a restart`.

**Acceptance**: This unit changes a screen, so look and interaction are checked at
360px, 768px and 1280px. The following checks exist, and `task check` passes.

- Unit tests in `web/src/settings` confirm:
  - With a not-chosen response, Software is selected.
  - An unavailable encoder's radio is disabled and has its reason text.
  - Selecting sends `PUT`; during "Saving…" the other choices cannot be operated;
    the response changes the encoder in use now.
  - A failed `PUT` reverts the selection and shows the reason inside the section
    with `role="alert"` (acceptance criterion 10).
  - `fallbackReason: selected_unavailable` shows the warning above the choices.
  - The description link's `href` is the R-11 URL and opens in a new tab
    (`target="_blank"`, `rel="noreferrer"`).
  - `checking: true` shows the checking state, which a response with `false`
    replaces.
  - The pseudo-locale check (`expectCatalogTextOnly`) passes, and the guest screen
    has no section (existing tests confirm `/settings` is owner-only).

### Keep the Docker image on software encoding and document using hardware encoding with a direct install

**Scope**: Settle the scope of the bundled image and write the user documents.

- `Dockerfile`, `compose.yaml`, `compose.hosting.yaml`: restore the content from
  before #491 (as of `e5acc24`). The runtime stage is Alpine and installs only
  `ffmpeg`; no GPU driver and no GPU passthrough setting are added
  ([R-1](research.md#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only)).
  Remove everything #491 added on the feature branch: the Debian runtime stage,
  the GPU drivers and the compose changes.
- `docs/how-to/running-vv.md`: a "Hardware encoding" section covering:
  - hardware encoding needs a direct install on the host
  - the bundled Docker image is software only, and inside the container every
    hardware encoder shows as unavailable
  - the prerequisites per encoder (driver, device, OS, an ffmpeg that includes
    the hardware encoder)
  - a pointer to the direct-install procedure
  - turning it on in Settings

  No override example that passes a GPU to the container
  ([R-9](research.md#r-9-hardware-encoding-runs-on-a-direct-host-install-documented-with-prerequisites-and-steps)).
  `docs/how-to/hosting-vv.md` points to the section.
- Run [quickstart.md](quickstart.md) with VVMDM installed directly on a host with
  a GPU, and record the results in the PR body.

**Dependencies**: `Add the Video conversion section to Settings for choosing an encoder and showing which encoders are available`.

**Acceptance**: `task check-docs` passes. The `Dockerfile` and compose files have
no diff against `e5acc24` (before #491), and the CI `Docker image` build
(`linux/amd64,linux/arm64`) passes. Started from the bundled image, the Settings
page shows every hardware encoder as unavailable, and live transcoding runs in
software. Reading the document, a user can meet the prerequisites on a host with a
direct install and turn on hardware encoding in Settings (acceptance
criterion 11). The PR body has the result of each quickstart step.
