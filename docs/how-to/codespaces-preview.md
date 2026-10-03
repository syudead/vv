# Check a PR in Codespaces

Opening a PR's branch in GitHub Codespaces starts VVMDM with sample videos
already scanned, so you can try the change in the browser. Each environment is
disposable and belongs to one PR.

## Steps

1. On the PR page, choose **Code → Codespaces → Create codespace on `<branch
   name>`**.
2. Wait for the first build. It installs ffmpeg and the Go and Node dependencies,
   which takes a few minutes.
3. When the build finishes, `task preview` runs in the terminal and VVMDM on
   port 8080 opens in a new browser tab. If it does not open, open it from the
   globe icon of `VVMDM` in the **Ports** tab.

Every time you reopen the same Codespace, `task preview` runs again. If the
previous instance is still running, it only prints the URL and exits.

When new commits land on the PR, run `git pull` in the terminal, stop the
preview with Ctrl+C, and start it again with `mise exec -- task preview`.

## What the preview contains

- `ffmpeg` generates the sample videos into `.local/preview/media/`. Their
  formats vary, so you can also check how a video the browser cannot play is
  shown.
- On every start, the preview scans that folder (registering it first if it is
  not registered) and opens 8080 only after the scan finishes. The library is
  complete even if the previous scan stopped partway or files were added. If
  the scan itself fails, the preview stops with an error and does not open 8080.
- The database and thumbnails are stored in `.local/preview/data/`. Deleting
  `.local/preview/` makes the next start begin from scratch.
- To try your own videos, upload the files by dragging them into the Explorer,
  then add that folder on the Settings screen.

`task preview` also works locally as is (it needs ffmpeg).

## Visibility

VVMDM has account authentication. Codespaces forwarded ports are **Private** by
default (only your GitHub account can open them); keep that setting. **Do not
change it to Public.**

`task preview` runs VVMDM itself on `127.0.0.1:18080` and listens on 8080 with a
relay only. Codespaces terminates the browser's HTTPS and passes requests inside
over HTTP, so without the relay VVMDM's same-origin check rejects every write
operation with 403. The relay rewrites Origin to the value VVMDM expects only
for requests the browser marked as same-origin. It refuses every other write
(requests from another site, or requests without an Origin) without passing it
to VVMDM. It does not rewrite Host.

Every request through the relay reaches VVMDM from the loopback address, so the
loopback-only "open file" feature is disabled in the preview (`DISPLAY` is not
passed).

## Cost

Personal accounts have a monthly free Codespaces quota. Delete a Codespace you
are done with at <https://github.com/codespaces>; a stopped Codespace still uses
storage quota. Check the free quota and spending limits in GitHub under
**Settings → Billing and licensing**.
