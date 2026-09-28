# Hosting VVMDM with the published image

Use this on a Docker host that does not have the VVMDM source, such as a NAS or a
home server. The host pulls the published image instead of building it. For
development from a clone, keep using `task up` ([Running VVMDM](running-vv.md)).

## Image

The image is published to `ghcr.io/syudead/vv` for `linux/amd64` and
`linux/arm64` whenever a change is merged into `main`:

| Tag              | Points to                                          |
| ---------------- | -------------------------------------------------- |
| `latest`         | The latest build of `main`                         |
| `sha-<12 chars>` | The build of that commit; use it to pin a version |

The CI `Docker image` job publishes it, and the package is public, so hosts
pull it without logging in.

## Start

Take [`compose.hosting.yaml`](../../compose.hosting.yaml) and change the lines
marked `変える` to match the host:

- the host port on the left of `8080:8080` (QNAP and some other NAS use 8080
  for their own management page, so pick another port such as `18080`);
- the host folder with the videos on the left of `:/media:ro`; mount a folder
  high enough that every video folder you want is below it, then choose the
  folders below `/media` in VVMDM's Settings;
- the host folder for the database and thumbnails on the left of `:/data`.

Create the data folder first. Then either paste the file into the NAS's
container manager as a new application (QNAP Container Station, Synology
Container Manager and similar), or save it as `compose.yaml` on the host and run
`docker compose up -d` in its folder. Nothing is built on the host.

Open `http://<host>:<port>/api/health` to check that VVMDM is up. Then open VVMDM in a
browser, finish the account setup right away
([Account setup](running-vv.md#account-setup)), add folders below `/media` in
Settings and start a scan. `MDM_LOG_LEVEL` works as in
[Runtime settings](running-vv.md#runtime-settings). A reverse proxy on the NAS
or the home network works without further settings; see
[Network exposure](running-vv.md#network-exposure) before making VVMDM reachable
from the internet.

To convert videos with the host's GPU, add the lines in
[Hardware encoding](running-vv.md#hardware-encoding) to the `mdm` service of
`compose.hosting.yaml` and choose the encoder in Settings.

## Update

First pull `ghcr.io/syudead/vv:latest` again, then recreate the container.
Recreating alone reuses the image already on the host, so do both, in the
container manager or with:

```bash
docker compose pull
docker compose up -d
```

The data folder survives a container update. Migrations run when the new version
starts. To stay on a version or go back to one, replace `latest` in `image:`
with its `sha-` tag. A database already migrated by a newer version may not
open with an older one,
so back up before updating.

## Back up and restore

Stop the container so that the SQLite database is not written during the copy,
copy the data folder (for example with the NAS's file manager or backup tool),
and start the container again.

The data folder includes settings and user data that a scan cannot recover;
[Data and recovery](running-vv.md#data-and-recovery) explains the distinction.
To restore, stop the container, replace the contents of the data folder with
the copy, and start it again. If the data folder was lost without a backup,
finish the new account setup and register the media folders again before
starting a scan.
