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

Read the [testing policy](../design-docs/testing.md) when writing or changing
Go or Web tests; it defines their scope, synchronization and fixture choices.

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
(`linterOptions.noInlineConfig`). A change keeps the functions it adds or makes
more complex at complexity 25 or less and shares code instead of copying it;
the Codex review raises a breach
([Complexity and duplication](../design-docs/complexity-and-duplication.md)).

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
  code -->|yes| push{Push to main or Renovate PR?}
  push -->|yes| more[E2E in four groups and Docker image build]
  code -->|yes| manual{Manual run?}
  manual -->|yes| e2e[E2E in four groups]
```

"No" means only Markdown, `docs/` or `specs/` changed. For a code change, CI
runs the subtasks of `task check` as parallel `Checks (<group>)` jobs: Go lint
with the generated-file, migration, e2e group and Windows build checks, Go tests, Web
lint with the SPA build check (`task build-check-web`), and the Vitest suite
split into three shards (`task test-web-report -- --shard=1/3`). `task test-web`
runs the build check and the whole suite. A check added to `task check` also
goes into one of these groups in `.github/workflows/ci.yml`, and a new
`web/e2e/*.e2e.ts` file goes into one of the four E2E groups there;
`task e2e-groups-check` fails when a file is in no group or in two. Only the Go groups
restore the Go build cache, and each saves its own copy on a push to `main`.
Vitest shards by file, so a test file that grows to dominate a shard is split by
topic, as `web/src/tags/TagsPage*.test.tsx` are around the shared
`web/src/testing/tagsPage.tsx`. A push to `main` is a
merged pull request, so browser E2E and the Docker image build do not run on a
pull request; `Browser E2E` passes there without running. Renovate PRs (head
branch `renovate/*`) are the exception: they run both, without publishing the
image, because the Renovate routine merges them without a person
([dependency-updates.md](dependency-updates.md)).

To run browser E2E on a branch before it merges, start the CI workflow by hand
(`workflow_dispatch`) from the Actions tab or with
`gh workflow run ci.yml --ref <branch>`. A manual run treats the branch as a
code change and runs every `Checks` group and the four E2E groups; it does not
build the Docker image.

The Go and Vitest test groups run the same tests as `task test-go` and
`task test-web-unit` through `task test-go-report` and `task test-web-report`,
which also write coverage and JUnit test results under `reports/`. CI uploads
them to [Codecov](https://app.codecov.io/gh/syudead/vv), together with the E2E
results of a push to `main`, even when a test fails. On each pull request that
changes code, Codecov comments the coverage of the changed lines, names failed
and flaky tests and sets an informational patch status per `go` and `web` flag,
without blocking the merge
([`.github/codecov.yml`](../../.github/codecov.yml)); coverage by flag and by
component is on the Flags and Components pages of Codecov. A newer push cancels a
running CI run on a pull request but not on `main`, so every commit on `main`
reaches Codecov. The upload authenticates
with GitHub OIDC, so the repository needs no Codecov token, but it must be
activated once on codecov.io by signing in with GitHub and installing the
Codecov GitHub App. A failed upload does not fail the check.

Beside the check groups, the `Quality` jobs run `task complexity-go`,
`task complexity-web` and `task duplication` in parallel on every pull request
and push to `main`. Each writes one tool's output under `reports/quality/`: the
cyclomatic complexity of every Go function from gocyclo (pinned as a tool in
`tools/go.mod`), that of every Web function from the ESLint `complexity` rule,
and the duplicated code that jscpd finds. CI keeps each as an artifact of the
same name for 90 days
([Test and CI quality monitoring](../design-docs/test-quality-monitoring.md));
a failed measure does not fail the run. `task quality-metrics` runs all three on
a local machine.

On the first day of each month the `Mutation testing` workflow
([`.github/workflows/mutation-testing.yml`](../../.github/workflows/mutation-testing.yml))
measures whether the Go tests catch injected bugs. It runs
`task mutation-go`, which runs gremlins (pinned as a tool in `tools/go.mod`) on
`./internal/domain/...` and writes `reports/mutation/go/gremlins.json`, the
place and outcome of every mutant. The workflow keeps that report as it is, as
an artifact for 90 days; mutants that the tests miss do not fail the run.
`MUTATION_GO_DIR=./internal/other task mutation-go` measures another package
tree, and arguments after `--` go to
`gremlins unleash`. A pull request that changes the workflow runs it too. Web
mutation testing with StrykerJS waits for a release of
`@stryker-mutator/vitest-runner` that fixes
[stryker-js#6210](https://github.com/stryker-mutator/stryker-js/issues/6210):
with Vitest 5 the runner filters out every test, so every mutant survives.

When the OpenAPI contract changes, edit `api/openapi.yaml` (or
`api/external-v1.yaml` for the external API) and run `task generate`. Never edit
`internal/httpapi/gen/`, `internal/httpapi/extgen/` or `web/src/api/gen/`
directly.
