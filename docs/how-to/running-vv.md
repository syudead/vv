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

Then open <http://localhost:18080> instead. The application inside the
container keeps listening on 8080, so only the host side of the mapping
changes.

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

| Variable              | Default | Purpose                                                                                         |
| --------------------- | ------- | ----------------------------------------------------------------------------------------------- |
| `MDM_ADDR`            | `:8080` | Server listen address                                                                           |
| `MDM_DATA_DIR`        | `/data` | Absolute path for the database and generated media                                              |
| `MDM_LOG_LEVEL`       | `info`  | `debug`, `info`, `warn`, or `error`                                                             |
| `MDM_TRUSTED_PROXIES` | empty   | Reverse proxies whose forwarding headers are trusted; see [Network exposure](#network-exposure) |

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

On a trusted home network, vv can be used over plain HTTP. To make it reachable
from the internet, put it behind a reverse proxy that serves HTTPS; never
expose vv's own HTTP port to the internet. Over HTTP, the password and the
session cookie travel unencrypted.

The reverse proxy must:

- terminate HTTPS and forward to vv over HTTP;
- pass the `Host` header through unchanged (vv compares it with `Origin` to
  accept only same-origin changes, and does not read `X-Forwarded-Host`);
- set or append the client address in `X-Forwarded-For` and set `X-Forwarded-Proto`
  to `https`.

Then list the proxy's address in `MDM_TRUSTED_PROXIES`, as CIDR ranges or
single addresses separated by commas or spaces:

```bash
MDM_TRUSTED_PROXIES=172.16.0.0/12 task up
```

vv reads the forwarding headers only on connections from those addresses.
There it takes the client address by walking `X-Forwarded-For` from the right
to the first untrusted address, and decides HTTPS from the last
`X-Forwarded-Proto` value. On every other connection it uses the connecting
address and whether the connection itself was TLS, so a client that reaches vv
directly cannot fake either. The `Forwarded` header (RFC 7239) is not read.
Invalid entries are reported together with other invalid settings at startup.

The client address and HTTPS decide the login attempt limit, the address in
authentication logs, the session cookie (`__Host-vv_session` with `Secure`
over HTTPS, `vv_session` over HTTP), the same-origin check, and whether the
"open in default app" action counts as coming from the server's own PC. That
action stays refused for remote clients even when the proxy runs on the same
PC.

If the proxy is missing from `MDM_TRUSTED_PROXIES`:

- every client is seen as the proxy's address, so all of them share one login
  attempt limit, and one person's failed attempts lock everyone out for a while;
- requests are treated as HTTP, so browsers send an `https://` `Origin` that
  does not match and every change (`POST`, `PUT`, `PATCH`, `DELETE`) including
  login fails with 403.

A minimal Caddy configuration, with vv and Caddy in the same Docker network:

```caddyfile
vv.example.com {
	reverse_proxy mdm:8080
}
```

Caddy obtains the certificate, passes `Host` through, and sets
`X-Forwarded-For` and `X-Forwarded-Proto` by default. Set
`MDM_TRUSTED_PROXIES` to the address range of the network Caddy connects from
(for Compose, the subnet shown by `docker network inspect`). When Caddy runs on
the host and vv also runs on the host without a container, use
`MDM_TRUSTED_PROXIES=127.0.0.1,::1`.
