# Implementation Plan: hardware encoding for live transcode

**Branch**: `feature/025-hardware-encoding` | **Parent Issue**: #370

**Input**: The parent Issue. It is this feature's specification.

## Summary

Live transcode (`GET /api/videos/{id}/transcode.mp4`) encodes video with `libx264` only. This
feature adds the server's hardware encoders (NVENC, Quick Sync, VAAPI, VideoToolbox). The owner
selects the encoder (software, one hardware encoder, or automatic) in a new "Video conversion"
section of the settings screen. SQLite stores the choice, and it applies from the next transcode
request without a restart.

| Area | Approach |
| --- | --- |
| Encoder decision | A pure function in `internal/domain` decides the encoder actually used from the saved choice and the startup check results. `TranscodeSettings` in `internal/app` holds it in memory ([research.md R-3](research.md#r-3-a-pure-domain-function-decides-the-encoder-actually-used-and-app-holds-it-in-memory), [R-4](research.md#r-4-storage-is-a-generic-settings-table-key-value)). |
| Startup check | A short real encode per encoder runs concurrently and does not delay HTTP listening ([R-2](research.md#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-concurrently)). |
| Transcode | `internal/media` builds the per-encoder codec arguments and keeps the output contract with the shared filter and keyframe arguments. When hardware fails before its first data, the same request switches to software ([R-6](research.md#r-6-switching-inside-a-request-tries-hardware-then-software-at-the-encode-step), [R-7](research.md#r-7-encode-arguments-replace-only-the-per-encoder-codec-options-shared-arguments-keep-the-output-contract)). |
| API and screen | `GET`/`PUT /api/settings/transcoding` ([contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)) and a section on the settings screen. The parent Issue's "UI quality" is the look-and-behavior specification as is (no `ui` label, so no design stage). |
| Bundled image and docs | The bundled Docker image stays the same Alpine image as before #491, software encoding only. Inside the container, the startup check reports every hardware encoder as unavailable. Hardware encoding is for VVMDM installed directly on the host (Windows, Linux, macOS); the docs describe its prerequisites and steps ([R-1](research.md#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only), [R-9](research.md#r-9-hardware-encoding-uses-a-direct-install-on-the-host-and-the-docs-describe-prerequisites-and-steps)). |

## Technical Context

**Canonical definitions**:

- Boundaries and dependency direction, the settings data category, the authentication boundary:
  [ARCHITECTURE.md](../../ARCHITECTURE.md) ("Intended dependency direction", "Rebuildable and
  user data", the authentication paragraph)
- Current live transcode:
  [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md),
  [docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md),
  [internal/media/transcode.go](../../internal/media/transcode.go) (the `Start` fallback ladder,
  `buildTranscodeArgs`, `videoEncodeArgs`),
  [internal/httpapi/transcode.go](../../internal/httpapi/transcode.go),
  [internal/domain/live_transcode.go](../../internal/domain/live_transcode.go)
- Startup order and wiring: [cmd/mdm/main.go](../../cmd/mdm/main.go),
  [internal/media/preflight.go](../../internal/media/preflight.go)
- Current settings structure: [internal/store/media_folders.go](../../internal/store/media_folders.go)
  (`SettingsStore`), [internal/app/media_folders.go](../../internal/app/media_folders.go),
  [internal/httpapi/media_folders.go](../../internal/httpapi/media_folders.go),
  [web/src/settings/SettingsPage.tsx](../../web/src/settings/SettingsPage.tsx),
  [web/src/settings/ScanStatusSection.tsx](../../web/src/settings/ScanStatusSection.tsx)
- API source of truth and error shape: [api/openapi.yaml](../../api/openapi.yaml),
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md);
  screen text: [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- Bundled image and operations docs: [Dockerfile](../../Dockerfile), [compose.yaml](../../compose.yaml),
  [compose.hosting.yaml](../../compose.hosting.yaml),
  [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md),
  [docs/how-to/hosting-vv.md](../../docs/how-to/hosting-vv.md),
  [.github/workflows/ci.yml](../../.github/workflows/ci.yml) (publishes `linux/amd64,linux/arm64`)
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`,
  `task generate`)

**Feature-specific context**:

- No Go or npm dependencies are added. `Dockerfile` and the compose files stay as they were before
  #491 (`e5acc24`) (R-1). The user provides the hardware encoding prerequisites (drivers, an
  ffmpeg that includes the hardware encoders) on the directly installed host (R-9).
- SQLite gets one new table ([data-model.md](data-model.md)). The migration uses the next number
  in `internal/store/migrations`.
- CI has no hardware encoders. Real-hardware checks run on a host with a GPU and a direct install,
  following [quickstart.md](quickstart.md). Automated tests replace ffmpeg (R-10).
- Parent Issue Requirement 10 includes the keyframe interval of "2 s or less in output time" from
  #371 (merged into `main`). The `-force_key_frames` argument is shared by all encoders, so the
  interval holds unchanged (R-7).
- The copy path that #371 defined (`videoCanCopy`, `CopySeekAllowance`) does not change. The
  encoder setting affects only requests that encode (Requirement 12).

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"): pass.
  - `internal/domain`: encoder values, interpretation of the choice, the rule that decides the
    encoder actually used, and the check targets per OS. Pure functions and enums only; `os/exec`
    is not used, and `runtime` values arrive as arguments.
  - `internal/media`: per-encoder arguments, the real check encode, and switching inside a
    request. It holds neither a store nor a logger, and returns the switch as a value (R-6).
  - `internal/app`: holds the saved value and the check results, runs the check, and writes logs.
    It receives the store and the checker through interfaces it declares.
  - `internal/store`: reads and writes the `settings` table only.
  - `internal/httpapi`: interprets requests and converts to `gen` types only. It asks app for the
    encoder decision.
  - `cmd/mdm`: wiring, and starting the startup check goroutine. Sibling packages do not import
    each other (the depguard rules do not change).
- **API source of truth** (ARCHITECTURE.md): pass. Change `api/openapi.yaml`, run `task generate`,
  and never hand-edit generated files (AGENTS.md).
- **Authentication boundary** (ARCHITECTURE.md authentication paragraph, Requirement 13): pass.
  The new paths are not added to `accessRoutes`, so they are owner only; `openapi_routes_test.go`
  checks that they match `security`. The settings screen is already an owner-only route, so a
  guest never sees the section.
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): pass. `settings` is
  settings data and is added to that list.
- **Domain events** (ARCHITECTURE.md events paragraph): not applicable. An encoder change emits no
  event (R-5).
- **Server output is English** (gosmopolitan in `.golangci.yml`, the 023 policy): pass. Logs,
  `message` and reason codes are English; the catalog owns screen text.
- **Docs change in the same PR** (core-beliefs.md, AGENTS.md): pass. Each unit updates
  ARCHITECTURE.md, design docs and how-tos.

The verdict is the same after Phase 1. Complexity Tracking lists no violations.

## Project Structure

### Documentation (this feature)

```text
specs/025-hardware-encoding/
├── plan.md                              # This file
│                                        # No spec.md — the parent Issue is the specification
├── research.md                          # Runtime environment, check, decision rule, storage, switching, arguments, API, docs, tests, docs link
├── data-model.md                        # settings table and in-memory values
├── quickstart.md                        # Real-hardware check on a host with a GPU
└── contracts/
    └── transcoding-settings-api.md      # GET/PUT /api/settings/transcoding
```

There is no `ui-design.md` (no `ui` label). The parent Issue's "UI quality" holds the screen
criteria.

### Source Code

**Affected boundaries**:

- `internal/domain`: `VideoEncoder`, `EncoderChoice`, `EncoderAvailability`, `TranscodeEncoding`,
  `ParseEncoderChoice`, `ResolveVideoEncoder`, `HardwareEncoderCandidates`,
  `LiveTranscodeRequest.VideoEncoder`, and the encoder used plus the switch fact on `LiveTranscode`
- `internal/store`: migration, `SettingsStore` reads and writes
- `internal/app`: `TranscodeSettings`
- `internal/media`: per-encoder arguments, `EncoderCheck`, the `Start` ladder
- `api/openapi.yaml`, `internal/httpapi` (new paths, `transcode.go`, `requiresJSONBody`),
  `cmd/mdm/main.go`
- `web/src/api`, `web/src/settings`, `web/src/i18n`
- `docs/how-to/running-vv.md`, `docs/how-to/hosting-vv.md`, `ARCHITECTURE.md`, `docs/design-docs/`
  (`Dockerfile`, `compose.yaml` and `compose.hosting.yaml` keep their pre-#491 content (`e5acc24`)
  and are not part of this feature's diff)

**New paths**:

- `internal/store/migrations/000NN_settings.sql` (the next number at implementation time)
- `internal/domain/video_encoder.go`
- `internal/app/transcode_settings.go`
- `internal/media/encoder_check.go`
- `internal/httpapi/transcoding_settings.go`
- `web/src/settings/TranscodingSection.tsx`
- `docs/design-docs/hardware-encoding.md` (current design of the encoder decision, check,
  switching and arguments; listed in `docs/design-docs/index.md`)

**Structure decision**: follow the existing layout ([ARCHITECTURE.md](../../ARCHITECTURE.md)).

| | |
| --- | --- |
| **Decision** | `TranscodeSettings` in `internal/app` decides the encoder and holds the check results. |
| **Why** | The values span memory (check results) and SQLite (the choice), and this is a settings-screen use case like `MediaFolders`. |
| **Rejected** | httpapi reads the store directly and decides: the decision rule and the check results would move into the HTTP layer. media reads the store: it violates the dependency direction. |

## Implementation Work

### Save the live transcode video encoder and decide the encoder actually used from the startup check results

**Scope**: encoder values and the decision rule, storage, app state.
- `internal/domain`: the values in [data-model.md](data-model.md) §2 and §3, and
  `ParseEncoderChoice`, `ResolveVideoEncoder`, `HardwareEncoderCandidates`
  ([R-3](research.md#r-3-a-pure-domain-function-decides-the-encoder-actually-used-and-app-holds-it-in-memory)).
- `internal/store`: the migration in [data-model.md](data-model.md) §1, and
  `TranscodeEncoderChoice` and `SaveTranscodeEncoderChoice` on `SettingsStore`
  ([R-4](research.md#r-4-storage-is-a-generic-settings-table-key-value)).
- `internal/app`: `TranscodeSettings`: loading the saved value, running the checks concurrently
  through a checker interface with a per-encoder time limit, `Current`, `Select`, and logging at
  startup and on change
  ([R-2](research.md#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-concurrently), R-3).
  `Select` returns `domain.ErrEncoderUnavailable` for an unavailable encoder.
- Docs: add `settings` to ARCHITECTURE.md "Rebuildable and user data", and add
  `TranscodeSettings` and settings reads and writes to the `internal/app` and `SettingsStore`
  descriptions.

**Dependencies**: None.

**Acceptance**: the following checks exist, and `task check` and `task check-docs` pass.
- `internal/domain` tests: no selection and an unknown value become `software`. `auto` picks the
  first available encoder in the order `nvenc`, `qsv`, `vaapi`, `videotoolbox`; with none, it is
  `software` with no `fallbackReason`. An unavailable hardware encoder gives `software` and
  `selected_unavailable`; while checking, `checking`. Check targets per OS (linux, windows,
  darwin, other).
- `internal/store` tests: no row means no selection. A saved value reads back. After two saves,
  the later value remains.
- `internal/app` tests (fake checker): while checking, `Current()` is `checking` with `software`.
  When the check ends, the results apply, and the log shows the choice, the encoder actually used
  and the reason. When one check exceeds its time limit, the other results still arrive and that
  encoder is `timed_out`. `Select` rejects an unavailable encoder and keeps the saved value.

### Transcode with ffmpeg hardware encoders, with a short check encode and a switch to software

**Scope**: changes in `internal/media`.
- `videoEncodeArgs` branches by encoder. The `software` arguments do not change
  ([R-7](research.md#r-7-encode-arguments-replace-only-the-per-encoder-codec-options-shared-arguments-keep-the-output-contract)).
- Take `LiveTranscodeRequest.VideoEncoder`, and split the encode step of the `Start` ladder into
  two steps, "hardware → software". Put the encoder used, and the hardware error when it switched,
  on `LiveTranscode`
  ([R-6](research.md#r-6-switching-inside-a-request-tries-hardware-then-software-at-the-encode-step)).
- `EncoderCheck`: reads `-encoders` with a time limit and runs a short real encode per encoder
  ([R-2](research.md#r-2-the-startup-check-runs-a-short-real-encode-per-encoder-concurrently)).
  It satisfies the checker interface from the previous unit.

**Dependencies**: `Save the live transcode video encoder and decide the encoder actually used from
the startup check results` (uses `domain.VideoEncoder`, `EncoderAvailability` and the checker
interface).

**Acceptance**: the following checks exist, and `task check` passes.
- Argument tests: the `software` arguments match the current test expectations character for
  character. Each hardware encoder has `-c:v h264_<encoder>`, High and Level 5.1, conversion to
  4:2:0 8-bit, the `-force_key_frames` argument, `hwupload` for VAAPI, and the MOV two-input form
  as before. A copyable request gets `-c:v copy` whatever the encoder.
- Helper process tests: when hardware ends without first data, the same request starts with
  `libx264`, and `LiveTranscode` records the switch. A deadline expiry or a cancellation does not
  switch. A failure after the first data does not switch.
- `EncoderCheck` tests: an encoder missing from `-encoders` is not run and is `encoder_missing`.
  When `-encoders` hangs past the time limit, every check target becomes `timed_out` and the
  result returns. A failed real encode is `check_failed`, a hanging helper is `timed_out` at the
  time limit, and success is `available`.
- The existing ffmpeg tests (rotation, aspect ratio, 4K, keyframe interval, MOV, cleanup on
  disconnect) pass unchanged.

### Connect the settings API and the live transcode path, and apply encoder changes without a restart

**Scope**: API, paths, wiring, design doc.
- Add the types and paths from
  [contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md) to
  `api/openapi.yaml`, and run `task generate`. Add `encoder_unavailable` to the error reasons.
- `internal/httpapi`: `getTranscodingSettings`, `updateTranscodingSettings`, `requiresJSONBody`.
  For each request, `transcode.go` puts the encoder actually used from
  `TranscodeSettings.Current()` on the request, and logs a `Warn` (video, encoder, tail of the
  ffmpeg error) when a switch happens.
- `cmd/mdm/main.go`: create `TranscodeSettings`, load the saved value, start the check goroutine
  without delaying listening, and stop it on shutdown.
- Docs: write `docs/design-docs/hardware-encoding.md`, and update `docs/design-docs/index.md` and
  ARCHITECTURE.md (the live transcode paragraph, the settings API).

**Dependencies**:
- `Save the live transcode video encoder and decide the encoder actually used from the startup check results`
- `Transcode with ffmpeg hardware encoders, with a short check encode and a switch to software`

**Acceptance**: the following checks exist, `task check` and `task check-docs` pass, and
`task generate` produces no diff.
- `internal/httpapi` tests: `GET` returns the contract shape. `PUT` saves `software`, `auto` and an
  available encoder, and the response reflects it. An unavailable encoder gives 409
  `encoder_unavailable`; a value outside the enum gives 400. A guest gets 401 / 403, and
  `openapi_routes_test.go` passes.
- `internal/httpapi/transcode_test.go` (helper process): requests after an encoder change start
  with `-c:v h264_<encoder>`. A request that started before the change continues as is. A request
  with an injected hardware failure returns 200 with a body, and a `Warn` log appears (Acceptance
  criterion 8).
- `cmd/mdm` tests: `/api/health` responds before the check ends. The startup log has the encoder
  line.

### Add the "Video conversion" section to the settings screen for selecting the encoder and showing available encoders

**Scope**: the settings screen section. The criteria are the parent Issue's "UI quality" and
Requirements 1, 6 and 8.
- `web/src/api/client.ts`: `getTranscodingSettings`, `updateTranscodingSettings`.
- `web/src/settings/TranscodingSection.tsx`: heading; description with an external link to the
  "Hardware encoding" section of `docs/how-to/running-vv.md` on the public docs site
  ([R-11](research.md#r-11-the-settings-screen-description-links-to-the-section-on-the-public-docs-site)); "In use now"; choices
  (radio, one per line, encoder name and state). An unavailable encoder cannot be selected and
  shows its reason.
- While checking, the section shows "Checking…" and reloads every few seconds until the check ends
  ([R-5](research.md#r-5-no-change-notification-the-screen-syncs-on-display-and-from-the-save-response)). Selecting saves
  immediately; while saving, it shows "Saving…" and blocks a second change. On failure, the
  selection reverts and the reason appears inside the section. The `fallbackReason` warning uses
  `text-warning` above the choices.
- `SettingsPage` places the section with the same spacing as `ScanStatusSection` and media folders
  (`mt-8`, divider under the heading).
- `web/src/i18n/en.ts`: section text, encoder names, reasons (`unsupported_os`,
  `encoder_missing`, `check_failed`, `timed_out`, `checking`), and the text for reason
  `encoder_unavailable`.

**Dependencies**: `Connect the settings API and the live transcode path, and apply encoder changes
without a restart`.

**Acceptance**: this unit changes the screen, so check the look and behavior at 360 px, 768 px
and 1280 px widths. The following checks exist, and `task check` passes.
- Unit tests in `web/src/settings` confirm:
  - A response with no selection shows software selected.
  - An unavailable encoder's radio is disabled and has its reason text.
  - Selecting sends `PUT`; while "Saving…" shows, the other choices are not operable; the
    response changes "In use now".
  - A failed `PUT` reverts the selection and shows the reason with `role="alert"` inside the
    section (Acceptance criterion 10).
  - `fallbackReason: selected_unavailable` shows the warning above the choices.
  - The description link's `href` is the R-11 URL and opens in a new tab (`target="_blank"`,
    `rel="noreferrer"`).
  - `checking: true` shows the checking state, and a response with `false` replaces it.
  - The pseudo-locale check (`expectCatalogTextOnly`) passes, and the guest screen has no section
    (existing tests confirm `/settings` is owner only).

### Keep the Docker image on software encoding and document using hardware encoding with a direct install

**Scope**: fixing the bundled image's scope and the user docs.
- `Dockerfile`, `compose.yaml`, `compose.hosting.yaml`: restore the content from before #491 (at
  `e5acc24`). The runtime stage is Alpine and installs only `ffmpeg`; no GPU drivers and no GPU
  passthrough settings
  ([R-1](research.md#r-1-the-bundled-image-stays-on-alpine-with-software-encoding-only)). Remove
  everything #491 added on the feature branch: the Debian runtime stage, the GPU drivers and the
  compose changes.
- `docs/how-to/running-vv.md`: a "Hardware encoding" section covering:
  - hardware encoding requires a direct install on the host;
  - the bundled Docker image is software only, and inside the container every hardware encoder
    shows as unavailable;
  - per-encoder prerequisites (drivers, devices, OS, an ffmpeg that includes the hardware encoder);
  - a pointer to the direct install steps, and enabling it on the settings screen.
- No override example for passing a GPU into the container
  ([R-9](research.md#r-9-hardware-encoding-uses-a-direct-install-on-the-host-and-the-docs-describe-prerequisites-and-steps)).
  `docs/how-to/hosting-vv.md` links to the section.
- Run [quickstart.md](quickstart.md) on a host with a GPU and a direct install, and record the
  results in the PR body.

**Dependencies**: `Add the "Video conversion" section to the settings screen for selecting the
encoder and showing available encoders`.

**Acceptance**:
- `task check-docs` passes.
- `Dockerfile` and the compose files have no diff against `e5acc24` (before #491), and the CI
  `Docker image` build (`linux/amd64,linux/arm64`) passes.
- Started from the bundled image, the settings screen shows every hardware encoder as
  unavailable, and live transcode runs with software.
- A reader of the docs can meet the prerequisites on a directly installed host and enable hardware
  encoding from the settings screen (Acceptance criterion 11).
- The PR body has the result of each quickstart step.
