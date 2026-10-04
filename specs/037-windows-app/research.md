# Research: Windows app

Parent Issue: #653. Inherited decisions: the tech stack, the boundaries and the
dependency direction follow [ARCHITECTURE.md](../../ARCHITECTURE.md) and
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
and this feature does not change them (one Go binary, `CGO_ENABLED=0`,
`modernc.org/sqlite`, the embedded SPA, `ffmpeg`/`ffprobe` as child processes).
The hardware encoder candidates for each OS follow
[docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md),
and the steps for running the binary directly and the conditions for "Open in
default app" follow [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md).
This file records only the decisions this feature adds.

## R-1: The desktop build compiles `cmd/mdm` as a Windows GUI under the `desktop` build tag and runs the server in the same process

**Decision**: `cmd/mdm` gains a set of `//go:build windows && desktop` files,
and `go build -tags desktop -ldflags "-H=windowsgui"` produces `VVMDM.exe`. The
desktop `main` builds its configuration itself instead of from environment
variables (R-4, R-5, R-9), calls the startup and shutdown steps split out of
the current `run()` ([plan.md structure](plan.md#source-code)) in the same
process, and runs the same shutdown steps when the window closes. Builds
without the tag (Docker, `task build`, `mdm account`) stay as they are.

**Rationale**: `cmd/mdm` is the only composition root (ARCHITECTURE.md
"Intended dependency direction"), and only `cmd/mdm` may read
`internal/eventbus`. A second entry point in the same package keeps a single
composition and leaves the depguard rules unchanged. In the same process, the
close action leads directly into the current staged shutdown (stop HTTP →
cancel the workers → wait for the scan to end → close the DB), and the close
confirmation reads "is an import in progress" straight from the DB (R-7).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A new `cmd/vvmdm`, with the server composition moved to `internal/server` | Rejected: moves 3,500 lines of composition and rewrites the rules "composition happens only in `cmd/mdm`" and "only `cmd/mdm` reads `eventbus`". |
| A launcher exe that starts the current `mdm.exe` as a child process | Rejected: on Windows a GUI parent has no way to send the equivalent of SIGTERM to a console child (`CTRL_BREAK_EVENT` needs a shared console), so a graceful shutdown cannot be guaranteed. The close confirmation would also have to ask "is an import in progress" over HTTP through owner authentication. |

## R-2: The window embeds WebView2 with `pkg/edge` from `github.com/wailsapp/go-webview2` in VVMDM's own Win32 window

**Decision**: The desktop build creates its own Win32 window with its own
window procedure and embeds `edge.Chromium` from `github.com/wailsapp/go-webview2`
(a tagged release that builds with `CGO_ENABLED=0`) in it. The window procedure
handles `WM_CLOSE` (R-7), `WM_QUERYENDSESSION`/`WM_ENDSESSION` (R-8) and
bringing the window to the front for a second launch (R-6).
`ContainsFullScreenElementChanged` switches video full screen to window full
screen (borderless, filling the screen), and `ProcessFailed` reloads the page
when the renderer process crashes. The app uses the WebView2 Runtime already
installed on Windows 10/11; when it is missing, a check before startup shows a
dialog with where to get it (R-10).

**Rationale**: Requirement 6, "choose to close or keep running", needs the app
to receive `WM_CLOSE` itself. For a video app, a `<video>` full screen confined
to the window frame is unusable. No CGO means the current `CGO_ENABLED=0`
cross build and `build-windows-check` work unchanged.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| The high-level API of `github.com/jchv/go-webview2` | Rejected: its fixed window procedure destroys the window on `WM_CLOSE` with no room for a confirmation, it does not handle the full-screen event, it calls `log.Fatal` on an initialization failure (a GUI app exits silently), and it has no tagged release. |
| Wails (v2/v3) | Rejected: it serves assets from its own origin such as `wails://`, which fails the same-origin check (`acceptsSameOrigin`) and the `Host` check of "Open in default app", and it needs its own CLI and project layout. |
| Starting Edge with `--app=` | Rejected: no room for a close confirmation, and closing is not reliably detected (requirements 5 and 6). |
| Electron or Tauri | Rejected: adds a Node or Rust toolchain and a runtime of tens to a hundred MB. |

## R-3: The window opens `http://localhost` on the listening port

**Decision**: WebView2 opens `http://localhost:<port>/`. The SPA, the API and
video delivery go through the current HTTP server unchanged.

**Rationale**: "Open in default app" (acceptance criterion 4) requires a
loopback client and a `Host` such as `localhost` (`loopbackRequest` in
`internal/httpapi/open.go`). The same-origin check also passes with `http://`
and a matching `Host`. Over HTTP the session cookie is `vv_session` without
`Secure`, and works as it is.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Intercept requests with WebView2's `WebResourceRequested` and pass them straight to the handler | Rejected: the HTTP server is needed anyway for LAN connections (requirement 9) and for checking from a browser (acceptance criterion 5), so there would be two paths. |

## R-4: A fixed default port `47880`, changed with `--port`; an unavailable port ends startup with the reason

**Decision**: The default port is `47880`. `VVMDM.exe --port <number>` changes
it (given as an argument in a shortcut). When listening fails, a dialog shows
the port number, that another program may be using it, and how to change it
with `--port`, and the app exits
([contracts/windows-app.md, Startup arguments and failure messages](contracts/windows-app.md#startup-arguments-and-failure-messages)).

**Rationale**: The edge case "the port VVMDM tries to use is taken by another
program" assumes a fixed port and a stated reason when it is unavailable.
Because the origin (`http://localhost:<port>`) does not change between starts,
WebView2's `localStorage` (display preferences) and the address opened on LAN
devices (requirement 9) keep working next time. `8080` is avoided because it
often collides with development servers and the Docker default.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Pick a free port on each start | Rejected: the origin changes, so `localStorage` is lost every time and the address remembered on LAN devices stops working. |
| Try the next number automatically when the port is taken | Rejected: the address given to LAN devices changes silently. |
| Change the port on the Settings screen | Rejected: the value must be read before listening, and when the port is taken and startup fails, the screen itself is unreachable. |

## R-5: Data lives in `%LOCALAPPDATA%\VVMDM`

**Decision**: The data location that corresponds to `MDM_DATA_DIR` is
`%LOCALAPPDATA%\VVMDM\data` (`mdm.db` and `thumbnails\`), WebView2's user data
is `%LOCALAPPDATA%\VVMDM\webview2`, and logs are `%LOCALAPPDATA%\VVMDM\logs`
([contracts/windows-app.md, Data locations](contracts/windows-app.md#data-locations)). The
`MDM_*` environment variables are not read.

**Rationale**: This meets requirement 4 (per user, writable, nothing to
specify) and requirement 10 (data survives replacing the zip). Generated media
(thumbnails, seek thumbnail sprites, previews) is large, so it goes in Local
rather than Roaming, which travels with a roaming profile. WebView2's default
user data location is next to the exe, and the app crashes when extracted to a
location it cannot write, so the location is set explicitly (edge case).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Next to the exe (portable) | Rejected: easily lost when the zip is replaced (requirement 10), and not writable in places such as `Program Files`. |
| `%APPDATA%` (Roaming) | Rejected: generated media would sync at every sign-in. |

## R-6: A second launch is detected by a per-user named mutex across sessions, and the existing window comes to the front

**Decision**: First thing at startup, before the DB opens, the app takes the
named mutex `Global\VVMDM-<hash of the user's SID and the data location>` with a
DACL that only the creating user can open. When it cannot take the mutex and
an existing window is in the same session (found by its unique window class
name), the app restores that window to its normal size, brings it to the front
and exits. When there is no window in the same session (the same user is
running VVMDM in another session), it shows a dialog saying VVMDM is already
running in another sign-in and exits. The process holds the mutex until it
ends. When the previous process is still shutting down and its window is
already gone, the app waits up to 30 seconds for the mutex before starting.

**Rationale**: Requirement 7. The same user uses the same DB in
`%LOCALAPPDATA%\VVMDM` in every session, so with `Local\` (per session), a
different `--port` in another session (Remote Desktop, for example) would let
two processes open the same DB. Putting the SID in the `Global\` name and
limiting the DACL to the creating user keeps other users free to start their
own. Detecting by a port collision cannot tell this apart from another program
using the port (R-4). Starting again while the previous process is shutting
down (the staged shutdown in R-7 takes up to tens of seconds) must not end
with nothing shown.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A `Local\` mutex | Rejected: as above, two processes in different sessions can open the same DB. |
| A lock file | Rejected: it remains after the process is killed and wrongly blocks the next start. |

## R-7: The close confirmation appears while a scan runs or import jobs are unfinished, and closing hides the window before the shutdown

**Decision**: On `WM_CLOSE` the app reads `Busy(ctx)`, added to `app.Scans`
(a scan is running, or at least one job is `queued` or `running`). When it is
false, the window closes without a confirmation. When it is true, a standard
Windows confirmation dialog (TaskDialog: "Import is in progress. If you close
VVMDM now, the import is interrupted. It continues from where it left off the
next time you start VVMDM.", with the buttons "Close" and "Keep running")
appears, and "Keep running" does nothing. On close the app hides the window,
runs the current shutdown steps to the end, closes the DB and exits. Live
transcoding (an `ffmpeg` per request) does not count.

**Rationale**: Requirement 6, "continues from where it stopped on the next
start", holds for the scan (R-9) and for import jobs
(`RequeueRunningJobs`). Live transcoding is tied to a request from a viewer and
is not resumed, and counting it would ask for confirmation on every close
during playback. Hiding the window first makes the response to the close action
visible at once, and acceptance criterion 5 (no response after closing) is met
by the process exiting.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| An HTML confirmation (`beforeunload`) | Rejected: closing the window does not reach WebView2. |
| A custom confirmation dialog inside the SPA | Rejected: needs a round trip from the window procedure to the SPA and back, and the window cannot close before the SPA loads or when the renderer process is broken. |

## R-8: Sign-out and shutdown show no confirmation, give Windows a reason to wait, and run the same shutdown steps

**Decision**: The app always answers `WM_QUERYENDSESSION` with "may end" and
registers "VVMDM is stopping" with `ShutdownBlockReasonCreate`. On
`WM_ENDSESSION` (the end is confirmed) it runs the current shutdown steps
synchronously and returns from the procedure when they finish. No
confirmation appears.

**Rationale**: The edge case "sign-out or shutdown". SQLite runs in WAL mode,
so a forced exit does not corrupt the DB, but a graceful stop records the scan
cancellation and the job states. When Windows ends the process before the stop
finishes, R-9 and `RequeueRunningJobs` continue the work on the next start.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Refuse the end in `WM_QUERYENDSESSION` and show the close confirmation | Rejected: Windows moves on to the force-close screen without waiting for the confirmation, so the confirmation means nothing. |

## R-9: An interrupted last scan restarts automatically at startup, for every way of starting

**Decision**: At startup, after `RecoverInterrupted` and after the workers
start, the app starts one new scan when the latest scan is `failed` with the
reason `interrupted`. `interrupted` marks both a scan cut short by a stop
request (the cancellation of the `Scans` lifetime) and a scan left `running`
and closed at startup (`internal/app/scans.go`, `internal/store/scans.go`).
Users have no action that cancels a scan, so `interrupted` always comes from
the process stopping. The behaviour is the same for Docker and for running the
binary directly, not only for the desktop build.

**Rationale**: Today an interrupted scan only closes as `failed`; jobs
continue (`RequeueRunningJobs`), but the file-finding stage does not.
Acceptance criterion 6 ("after choosing Close and restarting, the scan
continues from where it stopped") and the shutdown edge case need it. A scan
passes over files whose size and mtime have not changed without doing
anything (`internal/scanner/scanner.go`), so starting over gives the same
result as continuing. A scan stopped by a container restart is better
continued for the same reason, and there is no reason to vary the behaviour by
how VVMDM was started.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Restart only in the desktop build | Rejected: adds only a branch, and Docker users would still have to restart an interrupted scan by hand. |
| Record the scan's position (how far through the folders it got) and resume from there | Rejected: starting over skips unchanged files, so the gain does not justify a mechanism for recording and resuming. |
| Restart only when `RecoverInterrupted` closed a scan | Rejected: a gracefully stopped scan is already closed as `interrupted` at stop time, so choosing "Close" in the close confirmation (acceptance criterion 6) would not continue it. |

## R-10: Every startup failure shows its reason in a standard Windows dialog and is logged under `logs`

**Decision**: Before showing the window, the app checks the following in
order, and on a failure shows a dialog with the reason and what to do, then
exits (wording in
[contracts/windows-app.md, Startup arguments and failure messages](contracts/windows-app.md#startup-arguments-and-failure-messages)):

1. It is not running directly from inside the zip (the exe is not under a
   temporary folder, and `ffmpeg\ffmpeg.exe` and `ffmpeg\ffprobe.exe` are next
   to the exe).
2. The data location is writable.
3. The WebView2 Runtime is installed.
4. The DB opens and migrates.
5. The port can be listened on.

The server finishes listening before the window appears, so the window never
stays blank. Logs stay JSON and go to `logs\vvmdm.log`; each start moves the
previous run's log to `vvmdm.1.log`. The dialog also shows where the log is.

**Rationale**: The edge cases "the window does not stay blank, and the reason
it cannot start is clear" and "it does not exit silently, and what to do is
clear". A GUI exe has no standard output, so without a log file the cause
cannot be traced. When Explorer runs an exe inside a zip, it extracts only that
exe to a temporary folder, so the missing bundled `ffmpeg` next to it detects
the case.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Show a failure page in WebView2 | Rejected: cannot be shown when the WebView2 Runtime is missing or fails to initialize. |

## R-11: `ffmpeg` comes from the `ffmpeg\` folder next to the exe, added first to `PATH`, and child processes run without a console window inside a job object

**Decision**:

- At startup the desktop build adds `<exe folder>\ffmpeg` to the front of its
  own `PATH`. The command names in `internal/media` (`ffmpeg`/`ffprobe`) and
  the `exec.LookPath` check stay as they are.
- Every `ffmpeg`/`ffprobe` that `internal/media` starts gets
  `CREATE_NO_WINDOW` on Windows (one function in a `_windows.go` file sets it
  on the `exec.Cmd`; on other OSes it does nothing).
- First thing at startup, the desktop build puts itself in a job object with
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, so child processes join the same job.

**Rationale**: Requirements 2 and 3. A console child started from a GUI
subsystem parent flashes a console window for each child (requirement 3). When
the process is killed, a live transcoding `ffmpeg` does not stay behind and
keep running (requirement 5, "does not stay resident"). Adding the folder to
the front of `PATH` makes the bundled version win even when the user has
another `ffmpeg` installed, and the command names in the 6 places in
`internal/media` do not have to become configurable.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Pass the `ffmpeg`/`ffprobe` paths as configuration to each place in `internal/media` | Rejected: the desktop build is the only caller, and it would change 6 constants and their tests. |
| Set `CREATE_NO_WINDOW` only in the desktop build | Rejected: running the binary directly without a window, from Task Scheduler for example, has the same problem, and setting it causes no harm anywhere. |

## R-12: The bundled `ffmpeg` is Gyan.dev's Windows "essentials" build, pinned by version and SHA-256

**Decision**: The build script pins the version and SHA-256 of
`ffmpeg-<version>-essentials_build.zip` from the GitHub Releases of
`GyanD/codexffmpeg`, downloads it, and puts only `ffmpeg.exe`, `ffprobe.exe`,
FFmpeg's LICENSE and the source location in the zip
([contracts/windows-app.md, Zip contents](contracts/windows-app.md#zip-contents)). On a
Windows runner the build checks that `ffmpeg -hide_banner -encoders` lists
`h264_nvenc` and `h264_qsv`, and fails otherwise. The supported Windows
hardware encoders stay NVENC and QSV from
[hardware-encoding.md](../../docs/design-docs/hardware-encoding.md); AMF is not
added.

**Rationale**: Requirements 2 and 3, acceptance criterion 3. Gyan.dev's
Releases keep a stable URL per version, so a pinned version can be reproduced
later. essentials includes NVENC and QSV (oneVPL) and is smaller than full.
"Supported GPU" in acceptance criterion 3 means NVENC and QSV, the Windows
candidates in the current design; adding AMF is a separate change that adds an
encoder option.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| BtbN/FFmpeg-Builds | Rejected: it keeps no fixed Release per version (old dated automatic builds disappear), so a pinned version cannot be fetched later. |
| Have users install `ffmpeg` | Rejected: contradicts requirement 2. |

## R-13: GitHub Actions builds the distribution and attaches it to a GitHub Release on a `v*` tag

**Decision**: A new workflow `.github/workflows/windows-app.yml` runs on a push
of a `v*` tag and on manual dispatch. A Linux job builds the SPA, builds
`VVMDM.exe` with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` and `-tags desktop
-ldflags "-H=windowsgui -X main.version=<version>"`, and zips it with the
`ffmpeg` from R-12. A Windows job checks the encoders of the bundled `ffmpeg`
(R-12) and, for a tag, attaches the zip to the GitHub Release. A manual run
keeps it as a workflow artifact. Locally, `task build-windows-app` builds the
same zip. The exe's icon and manifest (DPI awareness, Common Controls v6) are
embedded as a `.syso` generated with `github.com/tc-hib/go-winres`. The only
target is `windows/amd64`.

**Revised later**: every push to `main` also builds the zip and replaces the
zip of a single `nightly` prerelease, whose tag moves to the commit, so the
newest build can be downloaded without cutting a tag. A `v*` tag still marks a
version boundary for users. The current rule is in
[docs/design-docs/windows-app.md](../../docs/design-docs/windows-app.md#distribution).

**Rationale**: Requirement 1 (distribute a zip). The current CI only publishes
a Docker image on a push to `main`, and there is no place for a distribution
users can download. Without the manifest, WebView2 renders blurry on high-DPI
screens.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Also build `windows/arm64` | Rejected: there is no arm64 `ffmpeg` build that can be pinned, and Windows on ARM runs x64 exes under emulation. |

## R-14: LAN access is stored in the settings table, switching it reopens the listener, and the default is loopback only

**Decision**:

| Aspect | Decision |
| --- | --- |
| Storage | The key `desktop.lan_access` in the existing `settings` table (value `true`/`false`; false when the row is missing). No migration is added. |
| Listening | `127.0.0.1:<port>` when false, `0.0.0.0:<port>` when true. At startup the value is read after the DB opens and before listening. A switch request reopens the listener on the new address: the current listener closes and the same `http.Server` calls `Serve` on a new one, while established connections continue. When reopening fails, the listener goes back to the old address, the stored value stays unchanged, and the response is `409`. |
| Display | While allowed, the response holds `http://<address>:<port>/` for each non-loopback IPv4 address that is up. |
| Route | Owner-only `GET`/`PUT /api/settings/network`, `404` outside the desktop build. When saving fails after the listener reopened, the listener goes back to the old address and the response is `500`, so the listener and the stored value never disagree ([contracts/network-settings-api.md](contracts/network-settings-api.md)). |
| Forwarding headers | The desktop build runs as `MDM_TRUSTED_PROXIES=none` and does not read forwarding headers. |

**Rationale**: Requirements 8 and 9, acceptance criterion 7. Listening on
loopback only means that, while LAN access is not allowed, a TCP connection
from the LAN is not even possible, and the first start shows no Windows
Firewall permission dialog. The dialog appears when the user turns access on,
so the reason for it is clear. The settings table is where the current video
encoding choice is stored, so no new storage mechanism is needed. Nothing sits
in front of the desktop build as a reverse proxy, so the default "trust
forwarding headers from private addresses" would only let a LAN device fake its
client address to escape the sign-in attempt limit.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Always listen on `0.0.0.0` and drop non-loopback connections at once while access is not allowed | Rejected: the first start shows the firewall dialog, asking "allow on the network?" when the user has not allowed anything. |
| Add a `0.0.0.0` listener next to the `127.0.0.1` one when access is allowed | Rejected: on Windows, listening on the wildcard and a specific address on the same port at once depends on socket options and is not reliable. |
| Keep the setting in a file in the data location | Rejected: two storage mechanisms. The DB is open before listening, so the table is enough. |

## R-15: The LAN access switch sits in an owner-only section of Settings and cannot be changed before account setup

**Decision**: The Settings screen gains a "Network" section, hidden when
`GET /api/settings/network` returns `404` (not the desktop build). The section
shows the switch, the addresses to open while access is allowed, and a caution
for turning it on (devices on the same LAN can then open VVMDM; when Windows
Firewall asks, allow it on "private networks"; the connection is HTTP, so
passwords travel unencrypted). The Settings screen and the API are owner-only,
so the switch cannot be changed before an account exists (`setupRequired`).

**Rationale**: Requirement 9 (access can be allowed from inside the app, and
the address is visible while allowed). Making the switch owner-only makes the
edge case "trying to allow LAN connections before account setup" impossible.
The risk of another LAN device creating the account first disappears because
an account always exists by the time access is allowed.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Put it in the window's (native) menu | Rejected: it could be pressed before account setup and would need a warning, and it would add a second settings screen outside the SPA. |
| Put the switch with a warning on the account setup screen | Rejected: there is no reason to open VVMDM to the LAN before the account exists. |
