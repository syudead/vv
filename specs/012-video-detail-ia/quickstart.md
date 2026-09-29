# Quickstart: verifying the video detail page

Setup, startup and checks follow [README.md](../../README.md) and [Taskfile.yml](../../Taskfile.yml)
(`task dev`, `task check`, `task test-e2e`). This document covers only the checks specific to this
feature. What automated tests can confirm is in the acceptance evidence of each unit of work.

## 1. Open file (Requirement 18, Acceptance criteria 16 and 17)

The result depends on the runtime environment, so a person checks these 3 cases.

1. **Same PC, outside a container**
   - Steps: run `task dev` on a PC with a desktop, and open
     `http://localhost:<port>/videos/<id>` in a browser on the same PC.
   - Expected: the "Open file" icon appears at the right end of the facts row. Pressing it opens
     the video in the default application.
2. **From another PC**
   - Steps: connect to the server from case 1 using its LAN address, from another device on the
     same network.
   - Expected: the "Open file" icon does not appear (only "Copy path" appears).
     `curl -X POST http://<lan-addr>:<port>/api/videos/<id>/open` returns 403.
3. **Container**
   - Steps: start with `docker compose up` and open the page in a browser on the same PC.
   - Expected: the "Open file" icon does not appear. The copied path is the path inside the
     container (`/media/...`). `curl -X POST http://localhost:8080/api/videos/<id>/open` returns
     409 `open_unavailable`. The container has no display, so this check wins over the requester
     check (403) (the check order of "Open file" in [contracts](contracts/video-detail-api.md)).

On the same PC, opening the page by the LAN address instead of `localhost` also hides "Open file",
because the requester and `Host` are not loopback.

Caution: a reverse proxy on the same PC makes requests from other PCs look like loopback. If files
open in that setup, block this route at the reverse proxy (Structural Decisions 5 of the plan).

## 2. Stage display and automatic refresh during ingest (Requirements 11 and 12, Acceptance criteria 10 and 11)

1. With a video detail page open, put a new video file in the media folder and start a scan from
   the settings page.
2. Open that video's detail page from the list in another tab.
3. Expected:
   - Without reloading, the stage display advances in this order: "Reading video information",
     thumbnail, seek preview, list preview.
   - The video becomes playable once reading finishes.
   - The line directly below the player disappears when all generation finishes.
   - Playback is not interrupted when generation progresses after playback starts.

## 3. Probe failure and retry (Requirement 13, Acceptance criterion 12)

1. Prepare a broken video. For example, put a valid MP4 cut partway from the start
   (`head -c 100000 good.mp4 > broken.mp4`) in the media folder and scan.
2. After the probe retries reach their limit, open the video's detail page.
3. Expected: the raw failure reason appears inside the player.
4. Leave the file as it is and press "Read again".
5. Expected: the page switches to the stage display. When the retries reach the limit, it returns
   to the failure display.
   - The page does not need to be reopened.
   - Pressing the button never enqueues 2 or more probe jobs for the same video.
