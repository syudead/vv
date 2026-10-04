# Quickstart: Checks for the video detail screen

These steps cover what depends on the runtime environment or on watching a scan
happen, which the automated tests do not. Setup, startup and checks follow
[README.md](../../README.md) and [Taskfile.yml](../../Taskfile.yml) (`task dev`,
`task check`, `task test-e2e`). What automated tests can confirm is in each
implementation unit's acceptance evidence.

## Open file (requirement 18, acceptance criteria 16 and 17)

The result depends on the runtime environment, so a person checks these three
cases.

| Case | Steps | Expected result |
| --- | --- | --- |
| Same PC, outside a container | Run `task dev` on a PC with a desktop, and open `http://localhost:<port>/videos/<id>` in a browser on the same PC | The `ファイルを開く` icon appears at the right end of the information row, and pressing it opens the video in the default application |
| From another PC | Connect to the same server from another device on the same network, using the LAN address | The `ファイルを開く` icon does not appear (only `パスをコピー` does). `curl -X POST http://<lan-addr>:<port>/api/videos/<id>/open` returns 403 |
| Container | Start with `docker compose up` and open it in a browser on the same PC | The `ファイルを開く` icon does not appear. The copied path is the path inside the container (`/media/...`). `curl -X POST http://localhost:8080/api/videos/<id>/open` returns 409 `open_unavailable`. A container has no display, so this is decided before the requester check (403) (the check order in "Open file" in the [contracts](contracts/video-detail-api.md)) |

Even in a browser on the same PC, opening with the LAN address instead of
`localhost` hides `ファイルを開く` (the requester and `Host` are not loopback).

Caution: a reverse proxy on the same PC makes requests from other PCs look like
loopback. If that setup lets them open files, block this endpoint at the reverse
proxy (the plan's Structural Decisions 5).

## Stage display and auto-refresh during a scan (requirements 11 and 12, acceptance criteria 10 and 11)

1. With the video detail screen open, put a new video file in a media folder and
   start a scan from the settings screen.
2. From the list in another tab, open that video's detail screen.
3. Expected result:
   - Without reopening, the stage display advances in this order: `動画情報の読み取り`
     → thumbnail → seek preview → list preview.
   - The video becomes playable once probing finishes.
   - The one-line display right below the player disappears when all generation
     finishes.
   - Generation progressing after playback starts does not interrupt playback.

## Probe failure and retry (requirement 13, acceptance criterion 12)

1. Prepare a broken video, for example a valid MP4 cut off partway
   (`head -c 100000 good.mp4 > broken.mp4`), put it in place and scan.
2. After probe retries reach the limit, open that video's detail screen.
3. Expected result: the raw failure reason appears inside the player.
4. Leave the file as it is and press `もう一度読み取る`.
5. Expected result: the screen moves to the stage display, and when retries reach
   the limit, it returns to the failure display.
   - The screen does not need reopening.
   - While pressing, no more than one probe job for the same video is enqueued.
