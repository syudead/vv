# Implementation Plan: Windows app (extract the zip, run the exe, and use VVMDM in its own window)

**Branch**: `feature/037-windows-app` | **Parent Issue**: #653

**Input**: The parent Issue. It is this feature's specification.

## Summary

A Windows user extracts the zip and runs `VVMDM.exe`, and VVMDM opens in its own
window with no console window, with no `ffmpeg` to install and no environment
variables to set. Closing the window also stops the server.

| Concern | Approach |
| --- | --- |
| Shape | `cmd/mdm` is built with the `desktop` build tag as a Windows GUI exe, and the server runs in the same process ([research.md R-1](research.md#r-1-the-desktop-build-compiles-cmdmdm-as-a-windows-gui-under-the-desktop-build-tag-and-runs-the-server-in-the-same-process)). The window is a custom Win32 window that embeds WebView2 (`github.com/wailsapp/go-webview2`) and opens `http://localhost:47880/` ([R-2](research.md#r-2-the-window-embeds-webview2-with-pkgedge-from-githubcomwailsappgo-webview2-in-vvmdms-own-win32-window), [R-3](research.md#r-3-the-window-opens-httplocalhost-on-the-listening-port), [R-4](research.md#r-4-a-fixed-default-port-47880-changed-with---port-an-unavailable-port-ends-startup-with-the-reason)). |
| Data and ffmpeg | Data lives in `%LOCALAPPDATA%\VVMDM`. The bundled `ffmpeg\` is prepended to `PATH`, and child processes open no console window and run in a job object ([R-5](research.md#r-5-data-lives-in-localappdatavvmdm), [R-11](research.md#r-11-ffmpeg-comes-from-the-ffmpeg-folder-next-to-the-exe-added-first-to-path-and-child-processes-run-without-a-console-window-inside-a-job-object), [R-12](research.md#r-12-the-bundled-ffmpeg-is-gyandevs-windows-essentials-build-pinned-by-version-and-sha-256)). |
| Closing, second launch and failures | Closing during an import asks first. A second launch brings the existing window to the front. Sign-out stops gracefully without asking. Every startup failure shows its reason in a dialog ([R-6](research.md#r-6-a-second-launch-is-detected-by-a-per-user-named-mutex-across-sessions-and-the-existing-window-comes-to-the-front), [R-7](research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown), [R-8](research.md#r-8-sign-out-and-shutdown-show-no-confirmation-give-windows-a-reason-to-wait-and-run-the-same-shutdown-steps), [R-10](research.md#r-10-every-startup-failure-shows-its-reason-in-a-standard-windows-dialog-and-is-logged-under-logs)). An interrupted scan starts again on the next start, for every way of starting VVMDM ([R-9](research.md#r-9-an-interrupted-last-scan-restarts-automatically-at-startup-for-every-way-of-starting)). |
| LAN | By default VVMDM listens on loopback only. The owner-only "Network" section of Settings allows connections from the LAN, and the permission is kept in the settings table ([R-14](research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only), [R-15](research.md#r-15-the-lan-access-switch-sits-in-an-owner-only-section-of-settings-and-cannot-be-changed-before-account-setup), [contracts/network-settings-api.md](contracts/network-settings-api.md)). |
| Distribution | GitHub Actions attaches `VVMDM-<version>-windows-amd64.zip` to a GitHub Release on a `v*` tag and, as revised later, on every push to `main` ([R-13](research.md#r-13-github-actions-builds-the-distribution-and-attaches-it-to-a-github-release-on-a-v-tag), [contracts/windows-app.md](contracts/windows-app.md)). |

The Issue has no `ui` label, so there is no design stage. The only screen change
is one section of Settings, and R-15 decides its placement and content.

## Technical Context

**Canonical definitions**:

| Topic | Source |
| --- | --- |
| Boundaries, dependency direction, where assembly happens, depguard | [ARCHITECTURE.md](../../ARCHITECTURE.md#intended-dependency-direction), [.golangci.yml](../../.golangci.yml) |
| Tech stack and the distribution form (Docker) | [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) |
| Hardware encoders per OS | [docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md) |
| Starting, environment variables, direct start, exposing on the network | [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) |
| API | [api/openapi.yaml](../../api/openapi.yaml) |
| Check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task generate`) |
| Dependency updates | [docs/how-to/dependency-updates.md](../../docs/how-to/dependency-updates.md) |

**Feature-specific context**:

- New runtime form: a `windows/amd64` exe for the GUI subsystem
  (`-tags desktop -ldflags -H=windowsgui`), cross-built from Linux with
  `CGO_ENABLED=0` kept.
- New dependencies: `github.com/wailsapp/go-webview2` (`go.mod`; only the
  Windows build reads it) and `github.com/tc-hib/go-winres` (a `tool` in
  `tools/go.mod`). FFmpeg is bundled (Gyan.dev essentials, pinned by version and
  SHA-256).
- Runtime prerequisite: the WebView2 Runtime (included by default in Windows
  10/11; when it is missing, the R-10 dialog appears).
- New stored value: the key `desktop.lan_access` in the existing `settings`
  table. No migration is added. No other part of the data model changes (no
  `data-model.md`).
- Behaviour changes for every way of starting: the automatic restart of an
  interrupted scan (R-9), and `CREATE_NO_WINDOW` for `ffmpeg`/`ffprobe` (R-11,
  Windows only).

## Constitution Check

| Gate | Verdict |
| --- | --- |
| Dependencies run one way, `cmd → internal/{adapters, app} → internal/domain`, and sibling adapters do not read each other (ARCHITECTURE.md, depguard) | Pass. The new adapter `internal/desktop`, which handles Win32 and WebView2, is read only by `cmd/mdm` and reads no other `internal/*`. It is added to depguard's sibling rule. |
| `cmd/mdm` only assembles and holds no use case (ARCHITECTURE.md) | Pass. "Is an import in progress", which the close confirmation needs, lives in `app.Scans.Busy`, and `cmd/mdm` only wires it. Reopening the listener is start and stop wiring, which is part of assembly. |
| `internal/app` does not read `os/exec`, `net/http` or the DB (ARCHITECTURE.md) | Pass. `Busy` goes through a role type the app declares (whether unfinished jobs exist). |
| Generated files are not hand-edited (AGENTS.md) | Pass. `/api/settings/network` changes `api/openapi.yaml` and runs `task generate`. |
| Constraints are enforced by checks (core-beliefs.md) | Pass. A `-tags desktop` build is added to `build-windows-check`, and lint with `GOOS=windows` and the `desktop` tag is added to `task lint`, so CI also checks Windows-only files. |
| Documents are fixed in the same change as the behaviour (core-beliefs.md, AGENTS.md) | Each document section has exactly one owning unit ([Documentation ownership](#documentation-ownership)), and no two units write the same section. The new design document `docs/design-docs/windows-app.md` is added to the index. |
| Docker and direct start keep working (parent Issue requirement 11) | Pass. The untagged build's entry point, environment variables and image do not change. R-9 and R-11 are the only changes that affect every way of starting. |

A re-check after design found no violation.

## Project Structure

### Documentation (this feature)

```text
specs/037-windows-app/
├── plan.md
├── research.md                      # R-1..R-15
├── quickstart.md                    # Checks on real hardware (the parts CI cannot see)
└── contracts/
    ├── network-settings-api.md      # /api/settings/network
    └── windows-app.md               # Zip contents, startup arguments and failure messages, data location, window
```

There is no `data-model.md`. The only added stored value is one key in the
existing `settings` table, described in R-14 and the contracts.

### Source Code

**Affected boundaries**:

| Boundary | What changes |
| --- | --- |
| `cmd/mdm` | `run()` is split into a start and stop sequence that takes the configuration, the log destination, the listen address, a notification after listening starts and the stop signal as arguments; both the untagged `main` and the desktop `main` call it. It wires a listener that can be reopened (R-14) and the reading and writing of the LAN permission. |
| `internal/app` | `Scans.Busy` (R-7) and restarting the scan at startup (R-9) |
| `internal/store` | Reading and writing `desktop.lan_access`, and the query for whether unfinished jobs exist |
| `internal/media` | Child process creation moves into one function, which adds `CREATE_NO_WINDOW` on Windows |
| `internal/httpapi`, `api/openapi.yaml` | `/api/settings/network` |
| `web/src/settings` | The "Network" section |
| Build and distribution | `scripts/build`, `Taskfile.yml`, `tools/go.mod`, `.github/workflows/windows-app.yml` |

**New paths**:

| Path | Purpose |
| --- | --- |
| `internal/desktop/` | Window, WebView2, dialogs, mutex, job object, and resolving the data location; the Windows implementation is in `_windows.go` files |
| `cmd/mdm/desktop_windows.go` and other `desktop`-tagged files | Desktop assembly and wiring |
| `cmd/mdm/winres/` | Icon and manifest inputs |
| `.github/workflows/windows-app.yml` | Build and release workflow |
| `docs/design-docs/windows-app.md` | Design document |

**Structure decision**: Win32 and WebView2 handling lives in the new adapter
`internal/desktop`, and the `desktop`-tagged files in `cmd/mdm` hold only
assembly and wiring. Rejected: putting everything in `cmd/mdm`. The window
procedure and the dialog implementation would sit in the assembly package, and
depguard could not keep them apart from the other adapters.

### Documentation ownership

The unit that changes a behaviour writes its section in the same PR
(core-beliefs.md). Each section has exactly one owning unit, so no two units
write the same section. The "Windows desktop app" unit creates
`docs/design-docs/windows-app.md` with all its section headings, and later units
write only the body of their own sections.

| Document | Section | Owning unit |
| --- | --- | --- |
| ARCHITECTURE.md | Recovery at startup (scan recovery in the "In place today" paragraph of Intended topology) | Restart an interrupted scan automatically at startup |
| ARCHITECTURE.md | The distribution form in Intended topology, and `internal/desktop` in Intended dependency direction | Windows desktop app `VVMDM.exe`: open in its own window and stop the server when it closes |
| ARCHITECTURE.md | `/api/settings/network` in the API list | Store the LAN access permission and switch it with `/api/settings/network` |
| docs/design-docs/windows-app.md and the index | Creating the document, "Process and window", "Data and ffmpeg", "Startup failures" | Windows desktop app `VVMDM.exe`: open in its own window and stop the server when it closes |
| docs/design-docs/windows-app.md | "Closing, second launch and sign-out" | Confirm before closing, bring the running window to the front on a second launch, and stop gracefully on sign-out |
| docs/design-docs/windows-app.md | "Connections from the LAN" | Store the LAN access permission and switch it with `/api/settings/network` |
| docs/design-docs/windows-app.md | "Distribution" | Build the Windows zip and attach it to a GitHub Release on a tag with a workflow |
| docs/how-to/running-vv.md | Data and recovery | Restart an interrupted scan automatically at startup |
| docs/how-to/running-vv.md | The new "Windows app" section (getting it, starting it, SmartScreen, data location, updating, `--port`, LAN permission) | Build the Windows zip and attach it to a GitHub Release on a tag with a workflow |
| docs/design-docs/tech-stack-selection.md | The distribution form | Build the Windows zip and attach it to a GitHub Release on a tag with a workflow |
| docs/how-to/dependency-updates.md | How to update the FFmpeg version | Build the Windows zip and attach it to a GitHub Release on a tag with a workflow |

## Implementation Work

### Restart an interrupted scan automatically at startup

**Scope**: At startup, when the latest scan is `failed` with reason
`interrupted`, start one scan after the workers start
([research.md R-9](research.md#r-9-an-interrupted-last-scan-restarts-automatically-at-startup-for-every-way-of-starting)).
This affects every way of starting VVMDM. This unit's sections in
[Documentation ownership](#documentation-ownership).

**Dependencies**: None

**Acceptance**: `task check` passes. In the app tests, a start whose latest scan
is `interrupted` starts one scan, and no scan starts when the latest scan is
`done`, `failed` with a reason other than `interrupted`, or when no scan record
exists. In a cmd/mdm test or a local check, sending SIGTERM during a scan and
starting again makes `GET /api/scans/current` return a new `running` scan, and
when it finishes, files that had not been found before the stop are imported
too.

### Start `ffmpeg`/`ffprobe` without a console window on Windows

**Scope**: Move the six places in `internal/media` that create child processes
into one function, which adds `CREATE_NO_WINDOW` on Windows
([R-11](research.md#r-11-ffmpeg-comes-from-the-ffmpeg-folder-next-to-the-exe-added-first-to-path-and-child-processes-run-without-a-console-window-inside-a-job-object)).

**Dependencies**: None

**Acceptance**: `task check` (including `build-windows-check`) passes. That
function is the only place in `internal/media` that calls `exec.Command` or
`exec.CommandContext` directly. The media tests build with `GOOS=windows`
(`go test -c` succeeds), and a Windows test confirms that
`SysProcAttr.CreationFlags` contains `CREATE_NO_WINDOW`.

### Split `cmd/mdm` startup and shutdown so the configuration and the listener are passed in

**Scope**: Split `run()` into a sequence that takes the `Config`, the log
destination, a component that creates the listener, a notification after
listening starts, and the stop signal as arguments. The untagged `main` passes
today's environment variables, standard output, `MDM_ADDR` and SIGINT/SIGTERM.
The listener component can reopen on a different address (the listener in
[R-14](research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only)).
Also `app.Scans.Busy`
([R-7](research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown))
and the store query it needs. Behaviour does not change.

**Dependencies**: None

**Acceptance**: `task check` passes. The existing `serveUntil` tests pass. In
the listener component tests, after reopening from `127.0.0.1:<p>` to
`0.0.0.0:<p>`, a connection from the machine's own non-loopback address
succeeds; after switching back, it is refused; established connections (SSE)
survive the reopen. When reopening fails, the listener stays on the original
address. In the `Busy` tests, it is true while a scan runs or a `queued` or
`running` job exists, and false when none exists. The `task up` container
answers `/api/health` as before.

### Windows desktop app `VVMDM.exe`: open in its own window and stop the server when it closes

**Scope**: `internal/desktop` (the window, embedding WebView2 with full screen
and reloading after a renderer process failure, dialogs, the job object,
resolving the data location) and the `desktop`-tagged `main` in `cmd/mdm`:
arguments (`--port`), the R-10 pre-start checks and dialogs, the log file,
prepending `ffmpeg\` to `PATH`, setting `%LOCALAPPDATA%\VVMDM`, configuring the
server not to read forwarding headers, showing the window once listening starts,
and on close running the stop sequence (without confirmation) and exiting
([R-1](research.md#r-1-the-desktop-build-compiles-cmdmdm-as-a-windows-gui-under-the-desktop-build-tag-and-runs-the-server-in-the-same-process)
to [R-5](research.md#r-5-data-lives-in-localappdatavvmdm),
[R-10](research.md#r-10-every-startup-failure-shows-its-reason-in-a-standard-windows-dialog-and-is-logged-under-logs),
R-11,
[contracts/windows-app.md, Startup arguments and failure messages](contracts/windows-app.md#startup-arguments-and-failure-messages) to [Window](contracts/windows-app.md#window)).
Add `internal/desktop` to depguard. Add a `-tags desktop` build to
`build-windows-check`, and lint with `GOOS=windows` and the `desktop` tag to
`task lint`. This unit's sections in
[Documentation ownership](#documentation-ownership) (including creating
`docs/design-docs/windows-app.md` and its index entry).

**Dependencies**: "Split `cmd/mdm` startup and shutdown so the configuration and
the listener are passed in", "Start `ffmpeg`/`ffprobe` without a console window
on Windows"

**Acceptance**: `task check` passes, and the `-tags desktop` Windows build
succeeds. The tests for resolving the data location and parsing arguments pass.
On real Windows hardware, with `ffmpeg\` placed next to the built exe, steps 1,
2, 4 and 5 of [quickstart.md](quickstart.md) and the Edge Cases "Port",
"Running from inside the zip", "Unwritable location", "Network drive" and "Full
screen" behave as written. The unit affects the screen, so check the window's
appearance and interaction.

### Confirm before closing, bring the running window to the front on a second launch, and stop gracefully on sign-out

**Scope**: Read `Busy` on `WM_CLOSE` and show the confirmation
([R-7](research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown)),
the named mutex and bringing the existing window to the front
([R-6](research.md#r-6-a-second-launch-is-detected-by-a-per-user-named-mutex-across-sessions-and-the-existing-window-comes-to-the-front)),
and `WM_QUERYENDSESSION`/`WM_ENDSESSION`
([R-8](research.md#r-8-sign-out-and-shutdown-show-no-confirmation-give-windows-a-reason-to-wait-and-run-the-same-shutdown-steps)).
This unit's sections in [Documentation ownership](#documentation-ownership).

**Dependencies**: "Windows desktop app `VVMDM.exe`: open in its own window and
stop the server when it closes", "Restart an interrupted scan automatically at
startup"

**Acceptance**: `task check` passes. On real Windows hardware, step 6 of
[quickstart.md](quickstart.md) and the Edge Cases "Second launch" and
"Sign-out" behave as written. Check the confirmation dialog's appearance and
interaction.

### Store the LAN access permission and switch it with `/api/settings/network`

**Scope**: `api/openapi.yaml` and `task generate`, the `internal/httpapi` route
(`404` when not running as the desktop app), `desktop.lan_access` in the store,
and the `cmd/mdm` wiring that picks the listen address from the stored value at
startup and, on `PUT`, reopens the listener before saving
([contracts/network-settings-api.md](contracts/network-settings-api.md),
[R-14](research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only)).
This unit's sections in [Documentation ownership](#documentation-ownership).

**Dependencies**: "Split `cmd/mdm` startup and shutdown so the configuration and
the listener are passed in", "Windows desktop app `VVMDM.exe`: open in its own
window and stop the server when it closes"

**Acceptance**: `task check` passes. In the httpapi tests, every row of the
tables in [`GET /api/settings/network`](contracts/network-settings-api.md#get-apisettingsnetwork) and [`PUT /api/settings/network`](contracts/network-settings-api.md#put-apisettingsnetwork) of the contract returns its response: `404` when not the
desktop app, `401` for a guest, `403` from another site, `409` `listen_failed`
with the stored value unchanged when the reopen fails, and `500` with the
listener back on the original address when saving fails. In the store tests, a
missing row reads as false and a saved value reads back. In the cmd/mdm tests,
VVMDM listens on `0.0.0.0` when the stored value is true and on `127.0.0.1` when
it is false or the row is missing. `addresses` is filled only while access is
allowed, and excludes loopback.

### Allow LAN connections and show the addresses to open in the "Network" section of Settings

**Scope**: Add the section to `web/src/settings`, hidden on `404`: the switch,
the addresses while access is allowed, the caution shown when turning it on
([R-15](research.md#r-15-the-lan-access-switch-sits-in-an-owner-only-section-of-settings-and-cannot-be-changed-before-account-setup)),
and the message for `409` `listen_failed`. Strings go in `web/src/i18n/en.ts`.
This unit writes no documents (the zip unit writes the LAN permission in
running-vv.md).

**Dependencies**: "Store the LAN access permission and switch it with
`/api/settings/network`"

**Acceptance**: `task check` passes. In the component tests, the section is
absent on `404`; when `lanAccess` is false, no addresses appear and the caution
shows; the switch sends a `PUT`; when true, the returned `addresses` are listed;
on `409` an error appears and the switch reverts. On real Windows hardware, step
7 of [quickstart.md](quickstart.md) behaves as written. The screen changes, so
check the section's appearance and interaction.

### Build the Windows zip and attach it to a GitHub Release on a tag with a workflow

**Scope**: Assembling the Windows zip in `scripts/build` (pinning and fetching
FFmpeg by version and SHA-256, the icon and manifest via `go-winres`,
`README.txt`), `task build-windows-app`, `go-winres` in `tools/go.mod`, and
`.github/workflows/windows-app.yml` (a `v*` tag and manual runs, an encoder
check in a Windows job, attaching to the Release)
([R-12](research.md#r-12-the-bundled-ffmpeg-is-gyandevs-windows-essentials-build-pinned-by-version-and-sha-256),
[R-13](research.md#r-13-github-actions-builds-the-distribution-and-attaches-it-to-a-github-release-on-a-v-tag),
[contracts/windows-app.md, Zip contents](contracts/windows-app.md#zip-contents)). The
Windows app section of running-vv.md (getting it, starting it, SmartScreen,
data location, updating, `--port`, LAN permission) and this unit's other
sections in [Documentation ownership](#documentation-ownership).

**Dependencies**: "Windows desktop app `VVMDM.exe`: open in its own window and
stop the server when it closes", "Allow LAN connections and show the addresses
to open in the "Network" section of Settings" (so that running-vv.md can
describe the LAN permission)

**Acceptance**: `task check` passes. `task build-windows-app` creates
`dist/VVMDM-<version>-windows-amd64.zip`, its contents match [Zip contents](contracts/windows-app.md#zip-contents) of the contract,
and the build fails for an FFmpeg whose SHA-256 does not match. A manual run of
the workflow succeeds, the Windows job confirms `h264_nvenc` and `h264_qsv`, and
the zip remains in the artifacts. On real Windows hardware without ffmpeg, steps
1 to 3 and 8 of [quickstart.md](quickstart.md) behave as written with that zip
(step 8 is checked with this unit's zip and a zip built from a later change).
