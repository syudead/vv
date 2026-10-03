# Windows desktop app (VVMDM.exe)

- Status: adopted
- Scope: the form that works on Windows after extracting a zip and running
  `VVMDM.exe` (a dedicated window, the server in the same process, the data
  location, the bundled `ffmpeg`, how startup failures are shown, closing,
  connections from the LAN, and distribution)
- Background: [specs/037-windows-app/](../../specs/037-windows-app/plan.md)
  (parent Issue #653)

The Windows app runs the VVMDM server and its UI in one GUI process built from
`cmd/mdm` with the `desktop` build tag; the Windows-specific parts live in
`internal/desktop`. The Docker and direct-run forms (`MDM_*`, `mdm account`,
the image) are documented in [docs/how-to/running-vv.md](../how-to/running-vv.md),
and this document does not cover them. The zip contents, the meaning of the
startup arguments and dialog messages, and the list of data locations are in
[contracts/windows-app.md](../../specs/037-windows-app/contracts/windows-app.md).

## Process and window

### Context

`cmd/mdm` is the only composition root. It runs the HTTP server, scanning and
the import workers in one process, and on a stop request it shuts them down in
stages (stop HTTP → cancel the workers → wait for the scan to end → close the
DB). Windows users need a form that shows no console window, runs in a
dedicated window rather than a browser, and stops the server when the window
is closed.

### Decision

- `cmd/mdm` is built as a Windows GUI exe with the `desktop` build tag
  (`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop -ldflags "-H=windowsgui" ./cmd/mdm`).
  The entry point is `cmd/mdm/desktop_windows.go`
  (`//go:build windows && desktop`), and the untagged entry point
  (`cmd/mdm/main_server.go`, environment variables and `mdm account`) is not
  compiled in. Both entry points call the same start and stop procedure
  `run`, so there is only one composition.
- The desktop entry point holds only composition and wiring. The adapter
  `internal/desktop` owns the Win32 window, the WebView2 embedding, the
  dialogs, the job object and resolving the data location. Only `cmd/mdm`
  imports `internal/desktop`, and `internal/desktop` imports no other
  `internal/*` package (the depguard sibling rule).
- Startup order: join the job object → parse the arguments → prepare the data
  location and the log → run the pre-start checks
  ([Startup failures](#startup-failures)) → start `run` in a separate
  goroutine, and show the window once the listener is open. The window appears
  only after the listener exists, so it never stays blank.
- The listener is on the loopback address `127.0.0.1:<port>`. The port
  defaults to `47880` and can be changed with `--port <1-65535>`; any other
  argument ends with the "unusable argument" dialog. No reverse proxy sits in
  front of the desktop app, so it does not read forwarding headers (equivalent
  to `MDM_TRUSTED_PROXIES=none`).
- The window is a custom Win32 window (title `VVMDM`, class name
  `VVMDMWindow`) that embeds `pkg/edge` (`edge.Chromium`) from
  `github.com/wailsapp/go-webview2` and opens `http://localhost:<port>/`. The
  SPA, the API and video delivery go through the HTTP server unchanged, so the
  loopback and `Host` checks for "open in default app", the same-origin check
  and the `vv_session` cookie work the same as in a direct run.
  - The initial size is 1280×800 (scaled up for the screen DPI). If that is
    larger than the primary screen's work area, the window is shrunk to fit
    and centred.
  - In video full screen (`ContainsFullScreenElementChanged`) the window
    becomes borderless (`WS_POPUP`) and covers the whole of its monitor; on
    exit it returns to its previous style, position and size
    (`WINDOWPLACEMENT`).
  - When the renderer process crashes or stops responding (`ProcessFailed`),
    the current page is reloaded. When the browser process ends, WebView2 is
    unusable, so the app shows a dialog for an unrecoverable error and exits
    through the stop procedure.
- On closing the window (`WM_CLOSE`), the app first hides the window, then
  issues the stop request; once `run` finishes the stop procedure, it destroys
  the window and ends the process. When `run` returns with an error, such as
  losing the listener, the app destroys the window along the same path and
  shows the reason in a dialog.

### Alternatives considered

- A new `cmd/vvmdm`, with composition moved to `internal/server`. This would
  create two composition roots or move 3,500 lines of composition. Rejected
  (research.md R-1).
- A launcher exe that starts the current `mdm.exe` as a child process. A GUI
  parent has no way to send a graceful stop to a console child. Rejected
  (R-1).
- The high-level API of `github.com/jchv/go-webview2`, Wails, Edge `--app=`,
  Electron or Tauri. Each one either cannot receive the close action itself,
  changes the origin of the assets, or adds a runtime. Rejected (R-2).

## Data and ffmpeg

### Context

Users set no environment variables and do not install `ffmpeg`. The extracted
zip is replaced by each new version, and may be placed in `Program Files` or a
read-only shared folder.

### Decision

- Data lives in the per-user `%LOCALAPPDATA%\VVMDM` (read as the known folder
  `FOLDERID_LocalAppData`). The app creates `data\` (the same contents as
  `MDM_DATA_DIR`: `mdm.db` and `thumbnails\`), `webview2\` (the WebView2 user
  data) and `logs\`. It reads no `MDM_*` environment variables. Nothing is
  written next to the exe, so replacing the zip does not delete data, and the
  app works when extracted to a location it cannot write.
- The log stays in the existing JSON format in `logs\vvmdm.log`; each start
  moves the previous log to `vvmdm.1.log` (the one before that is deleted).
- At startup the app prepends `<exe folder>\ffmpeg` to `PATH`. The command
  names in `internal/media` and the pre-start `exec.LookPath` check stay
  unchanged, and the bundled version is used even when the user has installed
  another `ffmpeg`.
- On Windows, the `ffmpeg`/`ffprobe` processes that `internal/media` starts get
  `CREATE_NO_WINDOW` (in every way of running the app).
- First thing at startup, the process puts itself into a job object with
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`. Child processes join the same job, so a
  forcibly terminated process leaves no live-transcoding `ffmpeg` behind. If
  joining the job fails, startup continues and the failure is logged.

### Alternatives considered

- Keeping data next to the exe (portable) or in `%APPDATA%` (Roaming). The
  data would be easy to lose when the zip is replaced, or large generated files
  would sync on every sign-in. Rejected (R-5).
- Passing the `ffmpeg`/`ffprobe` paths as settings to each place in
  `internal/media`. The desktop app is the only caller, and the change would
  spread. Rejected (R-11).

## Startup failures

### Context

A GUI exe has no standard output or standard error. If startup fails and the
app exits silently, the user sees nothing happen.

### Decision

Before showing the window, the app checks the following in order and shows
only the first failure in a standard Windows dialog (`MessageBox`, title
`VVMDM`, OK only, text in English), then exits. Every message includes the log
location (except the second, where the log cannot be written). The `Message*`
functions in `internal/desktop` build the messages.

1. `VVMDM.exe` is not under the temporary folder (as a long path), and
   `ffmpeg\ffmpeg.exe` and `ffmpeg\ffprobe.exe` are next to it. Opening the exe
   inside the zip in Explorer extracts only the exe to the temporary folder, so
   this detects that case. On failure, the message tells the user to use
   **Extract All** on the zip and run the extracted `VVMDM.exe`.
2. The folders under `%LOCALAPPDATA%\VVMDM` can be created and written. On
   failure, the message names the folder that could not be written.
3. The WebView2 Runtime is installed. If not, the message says it is required
   and gives the download URL.
4. The DB opens and migrates.
5. The app can listen on the port. On failure, the message gives the port
   number, says another program may be using it, and explains how to change it
   with `--port`.

Checks 4 and 5 happen inside `run`, so `run` attaches the failing stage
(`startupStage`: DB, listen, or other) to the error it returns, and the entry
point chooses the message from it. The error text itself is unchanged, so the
output of the untagged entry point does not change. A failure that matches
none of these (for example, `ffmpeg` not found on `PATH`) and a failure after
the window is shown (an unrecoverable WebView2 error, a lost listener) show a
summary and the log location.

### Alternatives considered

- Showing a failure page in WebView2. It cannot be shown when the WebView2
  Runtime is missing or fails to initialise. Rejected (R-10).

## Closing, second launch and sign-out

### Context

Users may close the app without knowing that closing interrupts an import
(parent Issue requirement 6). If the same user starts a second instance, two
processes open the DB in the same `%LOCALAPPDATA%\VVMDM` (requirement 7). On
sign-out or shutdown, Windows ends the process without waiting for a
confirmation.

### Decision

- **Confirmation before closing**: on `WM_CLOSE` the app reads
  `app.Scans.Busy` (a scan is running, or at least one job is `queued` or
  `running`). After `run` prepares scanning, it hands this query to the entry
  point through `runOptions.OnBusyProbe`. If it is false, the window closes
  without asking. If it is true or cannot be read, the app shows a standard
  Windows TaskDialog (title `VVMDM`, "Import is in progress", saying that
  closing interrupts it and that it resumes on the next start; buttons `Close`
  and `Keep running`, default `Keep running`). `Keep running`, Esc and closing
  the dialog do nothing. `Close` first hides the window, then issues the stop
  request, runs the stop procedure to the end, closes the DB and exits. Live
  transcoding is not something that resumes, so it is not part of the check.
  TaskDialog exists only in Common Controls v6 (selected by the exe
  manifest), so a build without the manifest asks with a Yes/No `MessageBox`
  with the same text (default No).
- **Second launch**: first thing at startup, before opening the log and the
  DB, the app takes the named mutex
  `Global\VVMDM-<user SID>-<hash of the data location>` with a DACL that only
  the creating user can open (`D:P(A;;GA;;;<SID>)`) and holds it until the
  process ends. Before that, it creates and holds a `Local\` (per-session)
  mutex with the same name. Because of this order, the owner of `Global\`
  always holds `Local\`.
  - If `Global\` cannot be taken, the app closes its own `Local\` and checks
    again. If `Local\` still exists (the owner is in the same session), the app
    takes `Local\` again; if a `VVMDMWindow` window is visible, it restores it
    from minimised, brings it to the front, and exits without a dialog. If no
    window is visible (the previous process has hidden its window and is
    stopping, or has not shown its window yet), the app waits up to 30 seconds
    for the window to appear or the mutex to be released. If the mutex is
    released, startup continues; if nothing is settled within 30 seconds, the
    app shows that VVMDM is starting and exits.
  - If `Local\` does not exist (the same user is running VVMDM in another
    session), the app shows a dialog saying so without waiting, and exits.
- **Sign-out and shutdown**: the app always answers `WM_QUERYENDSESSION` with
  "OK to end" and registers "VVMDM is stopping" with
  `ShutdownBlockReasonCreate`. On `WM_ENDSESSION` (the end is confirmed), it
  runs the same stop procedure synchronously without asking (up to 30
  seconds) and returns from the handler once it finishes. If the end is
  cancelled, the reason is removed. If the process is ended before it stops,
  the scan restart and `RequeueRunningJobs` on the next start continue the
  work.

### Alternatives considered

- HTML `beforeunload` or a confirmation inside the SPA. The window close
  action either does not reach WebView2, or the window cannot be closed before
  the SPA loads or when the renderer process fails. Rejected (R-7).
- Detecting a second instance with only a `Local\` mutex, a lock file, or a
  port conflict. These allow the same DB to be opened across sessions, are
  left behind by a forced termination, or cannot be told apart from a conflict
  with another program. Rejected (R-6).
- Refusing the end in `WM_QUERYENDSESSION` and showing a confirmation. Windows
  moves on without waiting for the confirmation. Rejected (R-8).

## Connections from the LAN

### Context

By default the desktop app opens only on its own PC, but when the owner allows
it, phones and other PCs on the same LAN should be able to use it too (parent
Issue requirements 8 and 9, acceptance criterion 7). While it is not allowed,
no TCP connection from the LAN may be possible, and the first start must not
show the Windows Firewall permission dialog.

### Decision

- **Storage**: the key `desktop.lan_access` (value `true`/`false`) in the
  existing `settings` table. If there is no row or the value is not `true`, the
  setting is false. No migration is added (`SettingsStore.LANAccess` and
  `SaveLANAccess` in `internal/store`).
- **At startup**: when `runOptions.Desktop` is true, `run` reads the stored
  value after opening and migrating the DB and before starting the jobs and
  the listener. If it is true, the app listens on `0.0.0.0:<port>`; if false,
  on `127.0.0.1:<port>` (`startNetworkSettings` in `cmd/mdm`, with the address
  from `domain.LANListenAddr`). If the value cannot be read, startup fails at
  the DB stage.
- **Route**: owner-only `GET`/`PUT /api/settings/network`
  (`NetworkSettings`: `lanAccess`, `port`, `addresses`). Authentication and
  the same-origin check sit at the same boundary as
  `/api/settings/transcoding`. A run that is not the desktop app (the untagged
  `main`) does not pass `httpapi.Options.NetworkSettings`, so the route
  returns `404` `not_found`, which the SPA reads as "do not show the Network
  section"
  ([contracts/network-settings-api.md](../../specs/037-windows-app/contracts/network-settings-api.md)).
- **Switching** (`NetworkSettings.SetLANAccess` in `internal/app`): under a
  lock that serialises switches, the app does nothing when the value equals
  the current one. Otherwise the reopenable listener in `cmd/mdm` reopens on
  the new address, and on success the value is saved.
  - If the new address cannot be opened, the listener component returns to
    the previous address, the stored value stays unchanged, and
    `domain.ErrListenFailed` is returned. The route answers `409` `conflict`
    with reason `listen_failed`.
  - If the reopen succeeds but saving fails, the listener reopens on the
    previous address and the route answers `500` `internal`.
  - Saving does not inherit the request's cancellation
    (`context.WithoutCancel`). Even if the request is cancelled after the
    reopen, the listener and the stored value do not diverge.
  - Reopening closes the current listener and calls `Serve` on the same
    `http.Server` with the new listener, so established connections (this
    request, the `/api/events` SSE, video being delivered) stay open. After
    access is turned off, new connections from the LAN are refused at the TCP
    level.
- **`addresses`**: only while access is allowed, one `http://<address>:<port>/`
  per non-loopback IPv4 address of each network interface that is up
  (`lanAddresses` in `cmd/mdm`). When access is off, the list is empty. If the
  interfaces cannot be read, access stays in effect, the list is empty, and
  the failure is logged.

### Alternatives considered

- Always listening on `0.0.0.0` and dropping non-loopback connections at once
  while access is not allowed. The firewall dialog would appear on the first
  start. Rejected (R-14).
- Adding a `0.0.0.0` listener alongside `127.0.0.1` when access is allowed.
  Listening on a wildcard and a specific address on the same port at the same
  time depends on Windows socket options. Rejected (R-14).
- Keeping the setting in a file in the data location. This would create a
  second storage mechanism. Rejected (R-14).

## Distribution

### Context

Users need a place to get the distribution from (parent Issue requirement 1).
The current CI only publishes the Docker image on a push to `main`. For the
zip alone to work, it bundles `ffmpeg` (requirement 2) and is updated by
replacement (requirement 10). Keeping the WebView2 rendering sharp on high-DPI
screens and the close-confirmation TaskDialog (Common Controls v6) both need
an exe manifest.

### Decision

- **zip**: `VVMDM-<version>-windows-amd64.zip`. Its root holds one folder with
  the same name, containing `VVMDM.exe`, `README.txt` (extract and run, how to
  get past SmartScreen, where the data is, how to update) and `ffmpeg\`
  (`ffmpeg.exe`, `ffprobe.exe`, `LICENSE.txt`, and a `README.txt` with the
  version and where to get the source). `<version>` is the tag without the
  leading `v`, or `sha-<12 characters>` of the commit when there is no tag. The
  READMEs use CRLF line endings for Windows Notepad. The only target is
  `windows/amd64`.
- **Build**: `task build-windows-app` (`scripts/build -windows-app`) builds the
  SPA, uses `go-winres` to produce `cmd/mdm/rsrc_windows_amd64.syso` from
  `cmd/mdm/winres/` (the icon, and a manifest for per-monitor v2 DPI awareness
  and Common Controls v6), builds `VVMDM.exe` with
  `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` and
  `-tags desktop -ldflags "-H=windowsgui -X main.version=<version>"`, and
  writes the zip to `dist/`. The `.syso` goes into every `cmd/mdm` build for
  the same `GOOS`/`GOARCH`, so it exists only during the build, is deleted
  afterwards, and is not under version control. When the version is numeric
  (up to `a.b.c.d`), it also goes into the exe's version information.
  `tools/go.mod` pins the `go-winres` version.
- **FFmpeg**: the build fetches `ffmpeg-<version>-essentials_build.zip` from
  the GitHub Releases of `GyanD/codexffmpeg`, at the version and SHA-256
  pinned in `scripts/build/windows_app.go`. The download is kept in
  `dist/cache/` and its SHA-256 is checked on every use; on a mismatch the file
  is deleted and the build fails, showing the expected and actual values. Only
  `bin\ffmpeg.exe`, `bin\ffprobe.exe` and `LICENSE` are taken from the zip. The
  Windows hardware encoders stay NVENC and QSV as in
  [hardware-encoding.md](hardware-encoding.md); AMF is not added.
- **Workflow** `.github/workflows/windows-app.yml`: runs on a push of a `v*`
  tag and on manual dispatch.
  - A Linux job builds the zip with `task build-windows-app` and keeps it as
    the workflow artifact `windows-app`.
  - A Windows job extracts the zip and checks the layout of its contents and
    that the bundled `ffmpeg -hide_banner -encoders` lists `h264_nvenc` and
    `h264_qsv`; if not, it fails. The runner has no GPU, so the check stops at
    the encoders being compiled in; actual GPU transcoding is checked on real
    hardware ([quickstart.md](../../specs/037-windows-app/quickstart.md)).
  - Only for a tag, after both jobs pass, the zip is attached to the GitHub
    Release. If the Release does not exist it is created; if it exists the zip
    is replaced.
- The exe is not code-signed. How to get past SmartScreen on the first run is
  written in the zip's `README.txt` and in
  [running-vv.md](../how-to/running-vv.md#windows-app).

### Alternatives considered

- Building the zip on every push to `main`. There would be no user-facing
  version boundary. Rejected (R-13).
- Also building `windows/arm64`. No pinnable arm64 build of `ffmpeg` exists,
  and the x64 exe runs under emulation. Rejected (R-13).
- Bundling `ffmpeg` from BtbN/FFmpeg-Builds. It keeps no fixed Release per
  version, so a pinned version cannot be fetched later. Rejected (R-12).
- Putting the `.syso` under version control. The untagged Windows build
  (`mdm.exe`) would also get the icon and the manifest, and the version
  information would stay stale. Rejected.
