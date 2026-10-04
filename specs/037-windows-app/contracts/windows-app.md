# Contract: Windows app zip, startup and data locations

The shape of the Windows app as users and the distribution steps see it. The
reasons for the decisions are in the sections of [research.md](../research.md).
The source of truth for direct start and Docker (`MDM_*`, `mdm account`, the
image) is [docs/how-to/running-vv.md](../../../docs/how-to/running-vv.md), and
this feature does not change them.

## Zip contents

The name is `VVMDM-<version>-windows-amd64.zip`. `<version>` is the tag
(without `v`), or `sha-<12 characters>` for a push to `main` or a manual run.

```text
VVMDM-<version>-windows-amd64/
├── VVMDM.exe            # -tags desktop, -H=windowsgui, icon and manifest embedded
├── ffmpeg/
│   ├── ffmpeg.exe       # Gyan.dev essentials (research.md R-12)
│   ├── ffprobe.exe
│   ├── LICENSE.txt      # The LICENSE shipped with the FFmpeg build
│   └── README.txt       # The FFmpeg version and where to get its source
└── README.txt           # Extract and run VVMDM.exe, getting past SmartScreen, the data location, how to update
```

## Startup arguments and failure messages

| Argument | Meaning |
| --- | --- |
| (none) | Starts on port `47880` |
| `--port <1-65535>` | Starts on that port |
| Anything else | Shows an "unusable argument" dialog and exits |

Startup checks run from the top. Only the first one that fails is shown, in a
standard Windows dialog (title `VVMDM`, OK only), and VVMDM exits. Every
message includes the log location (`%LOCALAPPDATA%\VVMDM\logs\vvmdm.log`),
except check 2, when the log cannot be written. The messages are in English and
say the following.

| # | Check | What the failure message says |
| --- | --- | --- |
| 1 | `VVMDM.exe` is not under a temporary folder, and `ffmpeg\ffmpeg.exe` and `ffmpeg\ffprobe.exe` are next to it | Extract the zip with **Extract All**, then run `VVMDM.exe` in the extracted folder |
| 2 | `%LOCALAPPDATA%\VVMDM` is writable | The path of the folder that could not be written |
| 3 | The WebView2 Runtime is installed | The WebView2 Runtime is required, and the URL to get it |
| 4 | The DB opens and migrates | The DB could not be opened (a summary of the error) |
| 5 | The port can be listened on | The port number, that another program may be using it, and that `--port` changes it |

A second launch
([research.md R-6](../research.md#r-6-a-second-launch-is-detected-by-a-per-user-named-mutex-across-sessions-and-the-existing-window-comes-to-the-front))
in the same session only brings the existing window to the front and shows no
dialog. When VVMDM is running in another session of the same user, a dialog
says so and the new launch exits. Both cases are detected before the DB opens.

## Data locations

| Path | Contents |
| --- | --- |
| `%LOCALAPPDATA%\VVMDM\data\mdm.db` (with `-wal`/`-shm`) | The same contents as `MDM_DATA_DIR` for Docker and direct start |
| `%LOCALAPPDATA%\VVMDM\data\thumbnails\` | Generated files |
| `%LOCALAPPDATA%\VVMDM\webview2\` | WebView2 user data (cookies, `localStorage`) |
| `%LOCALAPPDATA%\VVMDM\logs\vvmdm.log`, `vvmdm.1.log` | JSON logs of this run and the previous run |

These stay when the zip is replaced with a new version (requirement 10). The
new version applies the current migrations at startup.

## Window

- Title `VVMDM`, window class name `VVMDMWindow` (used to detect a second
  launch), initial size 1280×800 (fitted to the screen when the screen is
  smaller); it opens `http://localhost:<port>/`.
- Video full screen makes the window borderless and fills the screen. Leaving
  full screen restores the original position and size.
- The close confirmation
  ([research.md R-7](../research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown))
  is a standard Windows dialog with two buttons, **Close** and **Keep
  running** (the default is **Keep running**).
