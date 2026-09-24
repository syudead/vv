# Development

## Set up the toolchain

Go, Node.js, and Task versions are pinned in `mise.toml`. `ffmpeg`, Docker, Git,
and bash are system dependencies and are checked by `task doctor`.

```bash
mise trust
mise install
mise exec --command "task setup"
mise exec --command "task doctor"
```

`task setup` is the only task that installs npm dependencies. Stop a running
development server before repeating it on Windows because native dependency
files may be locked.

## Run locally

```bash
mise exec --command "task dev"
```

Open <http://localhost:5173>. The command starts the Go server with automatic
restart and the Vite development server. `MDM_ADDR` changes the Go listen
address and the default Vite API proxy target; set `MDM_API_TARGET` only when
the proxy should use a different target.

On Windows, a directly started Go binary requires an absolute `MDM_DATA_DIR`
including the drive letter. `task dev` supplies an absolute development path
automatically.

## Validate changes

```bash
mise exec --command "task check"
```

`task check` runs formatting checks, static analysis, unit tests, generated-file
checks, migration checks, and the Windows build check. Browser tests are a
separate command:

```bash
mise exec --command "task test-e2e"
```

Use `task help` for the full command list. `Taskfile.yml` is the supported entry
point for developer commands.

CI always checks the changed paths, then reports the result through the required
`Checks` job. Pull requests and pushes that change only Markdown, `docs/`,
or `specs/` run `task check-docs` for links and repository artifact rules,
while skipping `task check`, browser E2E, and the Docker image build. Code and
configuration changes run `task check`; browser E2E and the Docker build run
only on a push to `main`, that is, after a pull request is merged.

When the OpenAPI contract changes, edit `api/openapi.yaml` and run
`task generate`. Never edit `internal/httpapi/gen/` or `web/src/api/gen/`
directly.
