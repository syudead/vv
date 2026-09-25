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

## Account setup

vv has a single account. Until it is configured, the first person to reach the
server can create it, so finish the initial setup in the browser right after
installing vv, before the server is reachable by anyone else. Open vv and
choose the username and password on the setup screen.

## Changing the username or resetting the password

After the initial setup, the username and password can be changed only from a
shell on the host. Both commands read `MDM_DATA_DIR` alone, apply migrations,
and do not need `ffmpeg`; they work whether the server is running or stopped.
Each change signs out every existing session.

With Docker Compose, while the container is running:

```bash
docker compose exec mdm mdm account set-username NEW_NAME
docker compose exec mdm mdm account set-password
```

When the container is stopped:

```bash
docker compose run --rm mdm account set-password
```

`set-password` asks for the new password twice with echo turned off. When
standard input is not a terminal, it reads the first line (without the
trailing newline) instead, which suits scripts:

```bash
docker compose exec -T mdm mdm account set-password < new-password.txt
```

The password is never accepted as an argument or an environment variable, so
it does not end up in shell history, process listings or `docker inspect`.

The username must be 1 to 128 characters without control characters or
leading and trailing spaces; the password must be 1 to 1024 bytes. The
commands never create the account: on an unconfigured server they change
nothing and ask you to use the setup screen.

| Exit code | Meaning |
| --------- | ------- |
| `0`       | The change was saved and existing sessions were signed out |
| `1`       | The database could not be opened or written |
| `2`       | Not configured yet, an invalid value, a mismatched confirmation, or an unknown command |

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
