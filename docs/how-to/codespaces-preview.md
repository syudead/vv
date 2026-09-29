# Checking a PR in Codespaces

Opening a PR branch in GitHub Codespaces starts VVMDM with imported sample videos, so you can
check the change in a browser. Each PR gets a disposable environment.

## Open

1. On the PR page, select **Code → Codespaces → Create codespace on `<branch name>`**.
2. The first creation installs ffmpeg and the Go and Node dependencies, so it takes a few minutes.
3. When creation finishes, `task preview` runs in the terminal and VVMDM on port 8080 opens in a
   new browser tab. If it does not open, open it from the globe icon of `VVMDM` on the **Ports**
   tab.

Every reopen of the same Codespace runs `task preview`. If the previous instance is still
running, it only prints the URL and exits. When new commits land on the PR, run `git pull` in the
terminal, stop the preview with Ctrl+C, and restart it with `mise exec -- task preview`.

## Contents

- `ffmpeg` generates the sample videos into `.local/preview/media/`. The formats vary, so you can
  also check how videos the browser cannot play are shown.
- Every start imports that folder (registering it first if it is not registered) and opens 8080
  only after the import finishes. The result is complete even if the previous import stopped
  midway or files were added. If the import itself fails, the start stops with an error and does
  not open 8080.
- The database and thumbnails live in `.local/preview/data/`. Deleting `.local/preview/` makes the
  next start begin from scratch.
- To try your own videos, drag the files into the Explorer to upload them, then add that folder on
  the Settings page.

`task preview` also runs as is on a local machine (it needs ffmpeg).

## Visibility

VVMDM has account authentication. Codespaces forwarded ports are **Private** by default (only
your GitHub account can open them); keep that setting. **Do not change it to Public.**

`task preview` runs VVMDM itself on `127.0.0.1:18080` and listens on 8080 only with a relay.
Codespaces terminates the browser's https and passes http inside, so without the relay VVMDM's
same-origin check rejects every write with 403. The relay rewrites Origin to VVMDM's value only
for requests the browser marked as same-origin. It rejects every other write (a request from
another site, or a request without Origin) without passing it to VVMDM. It does not rewrite Host.

VVMDM sees every request through the relay as coming from loopback, so the loopback-only
"Open file" feature is disabled in preview (`DISPLAY` is not passed).

## Cost

Personal accounts have a monthly free Codespaces quota. Delete a Codespace you no longer use at
<https://github.com/codespaces>. A stopped Codespace still consumes the storage quota. Check the
free quota and the spending limit in GitHub **Settings → Billing and licensing**.
