# Hosting vv with the published image

Use this on a Docker host that does not have the vv source, such as a NAS or a
home server. The host pulls the published image instead of building it. For
development from a clone, keep using `task up` ([Running vv](running-vv.md)).

## Image

The image is published to `ghcr.io/syudead/vv` for `linux/amd64` and
`linux/arm64` whenever a change is merged into `main`:

| Tag              | Points to                                          |
| ---------------- | -------------------------------------------------- |
| `latest`         | The latest build of `main`                         |
| `sha-<12 chars>` | The build of that commit; use it to pin a version |

## Start

Copy [`compose.hosting.yaml`](../../compose.hosting.yaml) to a directory on the
host, for example `/opt/vv/compose.yaml`, and create an `.env` file beside it:

```dotenv
# Host port that serves vv (the container listens on 8080).
MDM_HOST_PORT=8080
# Host folder with the videos; mounted read-only at /media.
MDM_MEDIA_HOST_DIR=/volume1/videos
# Host folder for the database and thumbnails. Leave unset to use the vv_data volume.
MDM_DATA_HOST_DIR=/volume1/docker/vv/data
# Image tag; see the table above.
VV_IMAGE_TAG=latest
```

Then start vv from that directory:

```bash
docker compose up -d
curl -fsS http://localhost:8080/api/health
```

Nothing is built on the host. Open vv in a browser, finish the account setup
right away ([Account setup](running-vv.md#account-setup)), add `/media` in
Settings and start a scan. `MDM_LOG_LEVEL` and `MDM_TRUSTED_PROXIES` work as in
[Runtime settings](running-vv.md#runtime-settings); see
[Network exposure](running-vv.md#network-exposure) before making vv reachable
from the internet.

`MDM_DATA_HOST_DIR` must be an absolute path. When it is unset, the data lives
in the Docker volume `vv_data`.

## Update

```bash
docker compose pull
docker compose up -d
```

The container is recreated with the new image. Playback positions, tags and
the account are stored under `/data`, which is the host folder or the
`vv_data` volume, so they survive the update. Migrations run when the new
version starts. To stay on a version or go back to one, set `VV_IMAGE_TAG` to
its `sha-` tag; a database already migrated by a newer version may not open
with an older one, so back up before updating.

## Back up and restore

Stop vv so that the SQLite database is not written during the copy, then copy
`/data`:

```bash
docker compose stop
# Host folder:
tar -C /volume1/docker/vv -czf vv-data-$(date +%F).tar.gz data
# vv_data volume:
docker run --rm -v vv_data:/data -v "$PWD":/backup alpine \
  tar -C /data -czf /backup/vv-data-$(date +%F).tar.gz .
docker compose start
```

Thumbnails and the index can be rebuilt by scanning again; the account,
playback positions and tags cannot
([Data and recovery](running-vv.md#data-and-recovery)). To restore, stop vv,
replace the contents of the data folder or volume with the archive, and start
vv again.
