# Running VVMDM

## Start the container

VVMDM requires Task and Docker. From the repository root:

```bash
task up
```

To run VVMDM on a Docker host without the source, such as a NAS, use the
published image instead: see [Hosting VVMDM](hosting-vv.md).

Open <http://localhost:8080>. The health endpoint is available at
`http://localhost:8080/api/health`. Stop the application with `task down`.

The container mounts `./media` read-only at `/media` by default. Set another
host directory before starting VVMDM when needed:

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
- browser-incompatible files remain visible and play through live transcoding
  when conversion succeeds; and
- titles can be searched from the first character.

## Runtime settings

| Variable              | Default | Purpose                                                                                         |
| --------------------- | ------- | ----------------------------------------------------------------------------------------------- |
| `MDM_ADDR`            | `:8080` | Server listen address                                                                           |
| `MDM_DATA_DIR`        | `/data` | Absolute path for the database and generated media                                              |
| `MDM_LOG_LEVEL`       | `info`  | `debug`, `info`, `warn`, or `error`                                                             |
| `MDM_TRUSTED_PROXIES` | private | Reverse proxies whose forwarding headers are trusted; see [Network exposure](#network-exposure) |

Docker Compose sets these values for the container. Media folders themselves
are managed in the application rather than with a configuration file.
Invalid environment values are reported together when the application starts.

## Account setup

VVMDM has a single account. Until it is configured, the first person to reach the
server can create it, so finish the initial setup in the browser right after
installing VVMDM, before the server is reachable by anyone else. Open VVMDM and
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
| `0`       | The change was saved, existing sessions were signed out and API tokens were revoked |
| `1`       | The database could not be opened or written |
| `2`       | Not configured yet, an invalid value, a mismatched confirmation, or an unknown command |

## Hardware encoding

Videos that a browser cannot play are converted while they stream. By default
the conversion uses the CPU (software encoding). A GPU can do the video part
of that conversion instead, which lowers the CPU load.

Hardware encoding works only when VVMDM runs directly on the host (Windows,
Linux or macOS). The Docker image, both with `task up` and the published image
in [Hosting VVMDM](hosting-vv.md), uses software encoding only: it carries no
GPU drivers, and passing a GPU to the container does not enable a hardware
encoder. Inside the container, Settings shows every hardware encoder as not
available, and conversions use software.

### What each encoder needs

VVMDM does not bundle FFmpeg or GPU drivers on a direct install. It runs the
`ffmpeg` found on `PATH`, so that build must include the encoder, and the host
must have the driver and the device.

| Encoder in Settings  | Operating system | GPU and driver                                                                                                               |
| -------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| NVENC (NVIDIA)       | Linux, Windows   | An NVIDIA GPU with NVENC and the NVIDIA driver                                                                               |
| Quick Sync (Intel)   | Linux, Windows   | An Intel GPU supported by the oneVPL GPU runtime (Iris Xe, 11th-generation Core or newer), the Intel driver and that runtime |
| VAAPI (Intel/AMD)    | Linux            | An Intel GPU (Broadwell or newer) or an AMD GPU with a video encoder, a VA-API driver, and access to `/dev/dri/renderD128`   |
| VideoToolbox (macOS) | macOS            | A Mac; the driver is part of macOS                                                                                            |

On Linux, Quick Sync and VAAPI open the GPU through `/dev/dri`, so the user
that runs VVMDM needs read and write access to `/dev/dri/renderD128`, usually
through the group that owns it (often `render` or `video`):

```bash
ls -l /dev/dri/renderD128
sudo usermod -aG render "$USER"   # then sign out and in again
```

VAAPI uses `renderD128`, the first GPU. Consumer NVIDIA GPUs limit how many
NVENC sessions run at the same time; a conversion that the GPU refuses falls
back to software encoding.

Check that the `ffmpeg` on `PATH` has the encoder you want (`h264_nvenc`,
`h264_qsv`, `h264_vaapi` or `h264_videotoolbox`):

```bash
ffmpeg -hide_banner -encoders
```

Having an encoder in the list does not mean the GPU works; VVMDM tests it when
it starts (below). If an encoder is missing, install an FFmpeg build that
includes it, for example one linked from
[ffmpeg.org](https://ffmpeg.org/download.html).

### Run VVMDM directly on the host

Set up the toolchain as described in
[Development](development.md#set-up-the-toolchain), then build the single
binary from the repository root:

```bash
mise exec --command "task build"
```

This produces `bin/mdm` (the SPA is embedded in it). Start it with an absolute
data directory, and the other [Runtime settings](#runtime-settings) as needed:

```bash
MDM_DATA_DIR=/absolute/path/to/vv-data ./bin/mdm
```

On Windows, `MDM_DATA_DIR` must include the drive letter. Open
<http://localhost:8080>, finish the [Account setup](#account-setup), and add
your media folders in Settings.

### Turn it on in Settings

VVMDM tests each hardware encoder with a short encode every time it starts.
Open **Settings** as the owner and go to **Video conversion**:

- encoders that passed the test can be selected; the others show the reason,
  for example "Not supported on this server's operating system", "Not found on
  this server" or "The test encode failed";
- choose one encoder, or **Automatic** to use the first available of NVENC,
  Quick Sync, VAAPI and VideoToolbox, falling back to software;
- **In use now** shows the encoder that conversions use.

The choice is kept across restarts. If the chosen encoder is not available
after a restart (for example, the driver was removed or VVMDM now runs in the
Docker image), VVMDM converts with software and Settings says so. When the
hardware encoder cannot start a conversion, for example because the GPU is
busy, that conversion uses software. The startup log shows the result of each
test (`transcode video encoder checks finished`), and a failed test is logged
with the end of FFmpeg's error output.

## Data and recovery

The Docker setup stores application data in the `vv_data` volume. The SQLite
database is `MDM_DATA_DIR/mdm.db`; generated thumbnails live below
`MDM_DATA_DIR/thumbnails/`.

The database contains user and configuration data that scanning cannot restore.
The [data classification](../../ARCHITECTURE.md#rebuildable-and-user-data)
names every table and separates this data from the rebuildable index.

Back up the **whole** `vv_data` volume, including `mdm.db`, before resetting or
updating vv. Stop the container with `task down`, copy the volume with your
Docker volume backup tool, then restart with `task up`. Do not copy the database
files while vv is running: SQLite uses WAL mode, so a filesystem copy taken
during writes may be inconsistent. `task down` does not delete the volume;
`docker compose down -v` does.

To restore, stop vv, restore the saved volume, and start vv again. If the
database is lost without a backup, set up a new account, register the media
folders again in Settings, then start a scan to rebuild the index. Scanning
cannot recover the user and configuration data in the linked classification.

## Network exposure

On a trusted home network, VVMDM can be used over plain HTTP. To make it reachable
from the internet, put it behind a reverse proxy that serves HTTPS; never
expose VVMDM's own HTTP port to the internet. Over HTTP, the password and the
session cookie travel unencrypted.

The reverse proxy must:

- terminate HTTPS and forward to VVMDM over HTTP;
- pass the `Host` header through unchanged (VVMDM compares it with `Origin` to
  accept only same-origin changes, and does not read `X-Forwarded-Host`);
- set or append the client address in `X-Forwarded-For` and set `X-Forwarded-Proto`
  to `https`.

VVMDM trusts forwarding headers from loopback and private addresses by default
(`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `::1`,
`fc00::/7`), so a proxy on the same PC, on the home network or in the same
Docker network works without any setting. Set `MDM_TRUSTED_PROXIES` only to
narrow this, as CIDR ranges or single addresses separated by commas or spaces,
or to `none` to never read forwarding headers:

```bash
MDM_TRUSTED_PROXIES=172.18.0.0/16 task up
```

VVMDM reads the forwarding headers only on connections from those addresses.
There it takes the client address by walking `X-Forwarded-For` from the right
to the first untrusted address, and decides HTTPS from the last
`X-Forwarded-Proto` value. On every other connection it uses the connecting
address and whether the connection itself was TLS, so a client on the internet
cannot fake either.

With the default, a device on the home network can: it can claim another
address and so get around the login attempt limit. Narrow
`MDM_TRUSTED_PROXIES` to the proxy's address if you do not trust every device
on the network. The `Forwarded` header (RFC 7239) is not read. Invalid entries
are reported together with other invalid settings at startup.

The client address and HTTPS decide the login attempt limit, the address in
authentication logs, the session cookie (`__Host-vv_session` with `Secure`
over HTTPS, `vv_session` over HTTP), the same-origin check, and whether the
"open in default app" action counts as coming from the server's own PC. That
action stays refused for remote clients even when the proxy runs on the same
PC, and it also requires the connection itself to come from loopback, so a
device on the network cannot claim `127.0.0.1` in `X-Forwarded-For`.

If the proxy's address is not trusted (it has a public address, or
`MDM_TRUSTED_PROXIES` leaves it out):

- every client is seen as the proxy's address, so all of them share one login
  attempt limit, and one person's failed attempts lock everyone out for a while;
- requests are treated as HTTP, so browsers send an `https://` `Origin` that
  does not match and every change (`POST`, `PUT`, `PATCH`, `DELETE`) including
  login fails with 403.

A minimal Caddy configuration, with VVMDM and Caddy in the same Docker network:

```caddyfile
vv.example.com {
	reverse_proxy mdm:8080
}
```

Caddy obtains the certificate, passes `Host` through, and sets
`X-Forwarded-For` and `X-Forwarded-Proto` by default. Docker networks use
private addresses, so no `MDM_TRUSTED_PROXIES` setting is needed.
