# Running vv

## Start the container

vv requires Task and Docker. From the repository root:

```bash
task up
```

Open <http://localhost:8080>. The health endpoint is available at
`http://localhost:8080/api/health`. Stop the application with `task down`.

The container mounts `./media` read-only at `/media` by default. Set another
host directory before starting vv when needed:

```bash
MDM_MEDIA_HOST_DIR=/path/to/videos task up
```

The container publishes port 8080 on the host by default. Choose another host
port when 8080 is already in use (for example by a NAS management UI):

```bash
MDM_HOST_PORT=18080 task up
```

The application inside the container keeps listening on 8080, so only the host
side of the mapping changes.

Add the mounted folder in Settings and start a scan. Scans are manual: adding
files does not trigger one automatically. Source videos are read-only and are
never modified, moved, deleted, or converted.

During and after a scan:

- the library and player remain available while indexing continues;
- moved or renamed files retain their identity and playback position;
- browser-incompatible files remain visible with an explanation before
  playback is attempted; and
- titles can be searched from the first character.

## Runtime settings

| Variable        | Default | Purpose                                            |
| --------------- | ------- | -------------------------------------------------- |
| `MDM_ADDR`      | `:8080` | Server listen address                              |
| `MDM_DATA_DIR`  | `/data` | Absolute path for the database and generated media |
| `MDM_LOG_LEVEL` | `info`  | `debug`, `info`, `warn`, or `error`                |

Docker Compose sets these values for the container. Media folders themselves
are managed in the application rather than with a configuration file.
Invalid environment values are reported together when the application starts.

## Data and recovery

The Docker setup stores application data in the `vv_data` volume. The SQLite
database is `MDM_DATA_DIR/mdm.db`; generated thumbnails live below
`MDM_DATA_DIR/thumbnails/`.

Most stored data is a rebuildable index and can be recreated by scanning the
media folders again. Playback positions in `playback_progress` and tags
(`tags`, `tag_names`, `video_tags`) are user data and cannot be reconstructed.
Removing the `vv_data` volume deletes both, so back it up before resetting the
application.

## Network exposure

vv does not provide authentication yet. Run it only on a trusted network and
do not expose it directly to the internet.
