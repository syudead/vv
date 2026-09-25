# vv

vv is a self-hosted video library for files on local storage. It indexes media,
generates thumbnails and previews, remembers playback positions, and serves the
library and player from a single container.

The application is a Go binary with an embedded React SPA and SQLite database.
It reads source videos without modifying, moving, or deleting them.

## Run

Install [Task](https://taskfile.dev/) and Docker, then run:

```bash
git clone <repository-url>
cd vv
task up
```

Open <http://localhost:8080> and create the single account on the setup screen
right away: until it exists, anyone who can reach the server can create it.
Then add a media folder in Settings and start a scan.
To use a host directory other than `./media`:

```bash
MDM_MEDIA_HOST_DIR=/path/to/videos task up
```

Stop the application with `task down`.

> [!WARNING]
> Plain HTTP sends the password and session cookie unencrypted. Use it only on a
> trusted network; to reach vv from the internet, put it behind a reverse proxy
> that serves HTTPS (see
> [Network exposure](docs/how-to/running-vv.md#network-exposure)).
> Visitors who are not signed in can browse and play only the videos you mark as
> public.

Runtime settings, storage behavior, and backup cautions are documented in
[Running vv](docs/how-to/running-vv.md).

## Develop

Tool versions are pinned in `mise.toml`. With [mise](https://mise.jdx.dev/):

```bash
mise trust
mise install
mise exec --command "task setup"
mise exec --command "task doctor"
mise exec --command "task dev"
```

The development UI is served at <http://localhost:5173>. Run
`mise exec --command "task check"` before submitting a change; `task help`
lists the remaining commands.

See [Development](docs/how-to/development.md) for the full local workflow and
[AGENTS.md](AGENTS.md) for repository working agreements.

## Documentation

- [Architecture](ARCHITECTURE.md): system boundaries and dependency direction
- [Design documents](docs/design-docs/index.md): consequential technical decisions
- [Product specifications](docs/product-specs/index.md): specification policy
- [How-to guides](docs/how-to/README.md): repeatable operational procedures

The API contract in `api/openapi.yaml` is the source of truth for generated Go
and TypeScript types. Do not hand-edit `internal/httpapi/gen/` or
`web/src/api/gen/`.
