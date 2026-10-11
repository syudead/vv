# Development

## Set up the toolchain

Go, Node.js and Task versions are pinned in `mise.toml`. `ffmpeg`, Docker, Git
and bash are system dependencies, checked by `task doctor`.

```bash
mise trust
mise install
mise exec --command "task setup"
mise exec --command "task doctor"
```

`task setup` is the only task that installs npm dependencies. On Windows, stop
a running development server before repeating it, because native dependency
files may be locked.

## Run locally

```bash
mise exec --command "task dev"
```

Open <http://localhost:5173>. The command starts the Go server with automatic
restart and the Vite development server.

| Setting | Effect |
| --- | --- |
| `MDM_ADDR` | Go listen address and the default Vite API proxy target |
| `MDM_API_TARGET` | Vite API proxy target, when it differs from `MDM_ADDR` |
| `MDM_DATA_DIR` | Data folder; a Go binary started directly on Windows needs an absolute path with the drive letter (`task dev` supplies one) |

## Validate changes

```bash
mise exec --command "task check"
```

`task check` runs formatting checks, static analysis, unit tests,
generated-file checks, migration checks and the Windows build check. Before
every push, run it for a code or configuration change and `task check-docs` for
a change to Markdown, `docs/` or `specs/`; a skipped run tends to come back as a
formatting or lint fix from CI. `task fmt` rewrites Go and Web sources into the
checked format.

Lint findings are fixed in the code, never silenced: `task check` fails on any
`//nolint` comment in Go sources (`scripts/nolintguard`) and on any
`eslint-disable*` or other inline ESLint configuration comment in `web/`
(`linterOptions.noInlineConfig`).

Browser tests are a separate command:

```bash
mise exec --command "task test-e2e"
```

`task help` lists every command; `Taskfile.yml` is the supported entry point
for developer commands.

CI picks its jobs from the changed paths and reports through the required
`Checks` and `Browser E2E` jobs.

```mermaid
flowchart LR
  change[Changed paths] --> code{Code or config?}
  code -->|no| docs[task check-docs]
  code -->|yes| groups[task check in parallel groups]
  docs --> checks[Checks job]
  groups --> checks
  code -->|yes| push{Push to main?}
  push -->|yes| more[E2E in four groups and Docker image build]
```

"No" means only Markdown, `docs/` or `specs/` changed. For a code change, CI
runs the subtasks of `task check` as parallel `Checks (<group>)` jobs: Go lint
with the generated-file, migration and Windows build checks, Go tests, Web
lint with the SPA build check (`task build-check-web`), and the Vitest suite
split into three shards (`task test-web-report -- --shard=1/3`). `task test-web`
runs the build check and the whole suite. A check added to `task check` also
goes into one of these groups in `.github/workflows/ci.yml`. Only the Go groups
restore the Go build cache, and each saves its own copy on a push to `main`.
Vitest shards by file, so a test file that grows to dominate a shard is split by
topic, as `web/src/tags/TagsPage*.test.tsx` are around the shared
`web/src/testing/tagsPage.tsx`. A push to `main` is a
merged pull request, so browser E2E and the Docker image build never run on a
pull request; `Browser E2E` passes there without running.

The Go and Vitest test groups run the same tests as `task test-go` and
`task test-web-unit` through `task test-go-report` and `task test-web-report`,
which also write coverage and JUnit test results under `reports/`. CI uploads
them to [Codecov](https://app.codecov.io/gh/syudead/vv), together with the E2E
results of a push to `main`, even when a test fails. Codecov reports flaky and
slow tests from the test results, and comments the coverage of the changed lines
on a pull request without blocking it
([`.github/codecov.yml`](../../.github/codecov.yml)). The upload authenticates
with GitHub OIDC, so the repository needs no Codecov token, but it must be
activated once on codecov.io by signing in with GitHub and installing the
Codecov GitHub App. A failed upload does not fail the check.

After each successful CI run on a push to `main`, the `Quality metrics` workflow
([`.github/workflows/quality-metrics.yml`](../../.github/workflows/quality-metrics.yml))
runs `task quality-metrics`, which measures the cyclomatic complexity of every
Go function (`go/ast`) and Web function (the ESLint `complexity` rule), the
duplicated lines that jscpd finds, and the p50 and p90 durations of the last 20
successful CI runs on `main` and of each of their jobs. It appends the
measurement to `history.jsonl` on the `quality-metrics` branch, writes a
Markdown table with the change since the previous push to the run's summary,
and comments on the Issue labelled `quality-regression` when a measure got worse
([Test and CI quality monitoring](../design-docs/test-quality-monitoring.md)).
`task quality-metrics -- -ci-runs 0` measures the code alone on a local
machine; the CI durations need `gh`. A pull request that changes the workflow,
`scripts/qualitymetrics/` or a tool it uses (`Taskfile.yml`, `mise.toml`, the
npm manifests of `tools/` and `web/`, `web/eslint.config.js`) runs it without
saving the history.

When the OpenAPI contract changes, edit `api/openapi.yaml` (or
`api/external-v1.yaml` for the external API) and run `task generate`. Never edit
`internal/httpapi/gen/`, `internal/httpapi/extgen/` or `web/src/api/gen/`
directly.
