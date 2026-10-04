# Quickstart: Checking the Windows app by hand

These steps cover the window, the firewall, the GPU and connections from
another device, which CI (`task check`, `.github/workflows/windows-app.yml`)
cannot check; CI covers the build, the encoders of the bundled `ffmpeg` and
the Go tests. Development setup and commands are in
[docs/how-to/development.md](../../docs/how-to/development.md). Step numbers
match the acceptance criterion numbers of parent Issue #653.

## Prerequisites

- A zip: from `task build-windows-app` (`dist/VVMDM-<version>-windows-amd64.zip`)
  or the artifact of a manual run of the workflow.
- A Windows 10/11 x64 PC without ffmpeg, Go or Node installed (`where ffmpeg`
  returns nothing). Acceptance criterion 3 needs an NVIDIA or Intel GPU.
- A smartphone or a second PC on the same LAN.
- For the check against a previous version (acceptance criterion 8), two zips:
  the first zip of this feature and the one after it.

## Steps

1. **Extract All** on the zip and run `VVMDM.exe` (for SmartScreen, **More
   info → Run anyway**). No console window appears, and a window titled
   `VVMDM` shows the account setup screen. `%LOCALAPPDATA%\VVMDM\data\mdm.db`
   exists.
2. Create the account, add a media folder in Settings that contains a format
   the browser cannot play (for example, HEVC in `.mkv`), and scan. Thumbnails
   and metadata appear, and the video plays through live transcode. No console
   window appears during the import, not even for a moment.
3. In **Video conversion** in Settings, the method that matches the GPU (NVENC
   or QSV) is listed as selectable.
4. On a video's page, **Open file** opens the video in the Windows default app.
5. Close the window. In a browser on the same PC, `http://localhost:47880/`
   does not respond. Task Manager shows no `VVMDM.exe` and no `ffmpeg.exe`.
6. Start a scan of a large folder and try to close the window partway. A
   confirmation appears; **Keep running** lets the scan continue. Close again
   and choose **Close**, then start VVMDM again: **Scan status** in Settings
   shows the scan running, and when it ends every file is imported. Closing
   after the import has finished shows no confirmation.
7. On a LAN device, `http://<PC address>:47880/` does not connect. Turn on
   **Allow connections from the local network** in **Network** in Settings:
   Windows Firewall asks for permission (allow on private networks), and the
   LAN device opens one of the addresses the section lists. After VVMDM
   restarts, the setting stays on and the address still opens. After it is
   turned off, the LAN device cannot connect again.
8. With the previous version's zip, create the account, add tags and play a
   video (stopping partway). Close VVMDM, replace the contents of the extracted
   folder with the contents of the new version's zip, and start it. Sign-in
   with the same account works, and the tags and the playback position remain.

## Edge cases

| Case | What to do | Expected result |
| --- | --- | --- |
| Port | Hold the port (for example with `python -m http.server 47880`) and start VVMDM. | A dialog names the port and points to `--port`, then VVMDM exits. A shortcut with `--port 47881` starts it. |
| Second launch | Run `VVMDM.exe` again while it is running. | No new window appears; the existing window, even when minimized, comes to the front. |
| Second launch from another session | As the same user, start VVMDM with `--port 47881` from another Remote Desktop session. | A dialog says VVMDM is running under another sign-in, then VVMDM exits. |
| Sign-out | Sign out of Windows during a scan, sign in again and start VVMDM. | The scan starts over and finishes. The DB opens (VVMDM starts). |
| Running from inside the zip | Open the zip in File Explorer and double-click the `VVMDM.exe` inside. | A dialog asks for the zip to be extracted. |
| Read-only location | Extract to a read-only shared folder and run it. | VVMDM starts as usual (data goes to `%LOCALAPPDATA%`). |
| Network drive | Add a folder on `\\NAS\video` or on a mapped drive letter. | The folder can be added and its videos play. |
| Full screen | Press the video's full-screen button, then `Esc`. | The video fills the screen, and `Esc` returns to the original window. |
