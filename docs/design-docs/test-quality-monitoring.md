# Test and CI quality monitoring

CI sends coverage and test results to Codecov, and each push to `main` keeps
the complexity of every function and the duplicated code as an artifact; the
measuring lives in [`ci.yml`](../../.github/workflows/ci.yml),
[`quality-metrics.yml`](../../.github/workflows/quality-metrics.yml) and the
`quality-metrics` task in [`Taskfile.yml`](../../Taskfile.yml).

The diagram shows where each measure goes and who reads it.

```mermaid
flowchart LR
  push[Push to main] --> ci[CI test groups]
  push --> qm[Quality metrics]
  pr[Pull request] --> ci
  ci -->|coverage, JUnit results| codecov[(Codecov)]
  qm -->|tool output| art[(Run artifact, 90 days)]
  codecov -->|changed-line coverage| prc[PR comment]
  prc --> author[PR author]
```

## Measures and where they are kept

Each measure has one home, and each is the output of an existing tool.

| Measure | Tool | Measured | Kept in |
| --- | --- | --- | --- |
| Line coverage, by `go` and `web` flag and by file | Go `-coverprofile`, Vitest v8 | Every push to `main` and every pull request | [Codecov](https://app.codecov.io/gh/syudead/vv) |
| Duration and outcome of every test; flaky tests | JUnit from gotestsum, Vitest and Playwright | Same; E2E on pushes to `main` only | Codecov Test Analytics |
| Cyclomatic complexity of every Go function | gocyclo | Every push to `main` | `go-complexity.txt` in the `quality-metrics` artifact |
| Cyclomatic complexity of every Web function | The ESLint `complexity` rule with a maximum of 0 | Same | `ts-complexity.json` in the same artifact |
| Duplicated code, by location | jscpd | Same | `jscpd/jscpd-report.json` in the same artifact |

The artifact keeps each tool's output as written, with the file and line of
every function and clone; nothing is summarised, compared with an earlier push
or kept beyond the 90 days of the artifact. CI durations are read on the Actions
pages of the CI runs.

## Coverage informs a pull request and does not block it

Codecov comments the coverage of the changed lines on a pull request, and its
status is informational ([`.github/codecov.yml`](../../.github/codecov.yml)).
A required coverage number invites tests written for the number, which
[Web test levels](web-testing.md) rules out by choosing the lowest level that
fails when a behaviour breaks.

## Measures run with no write token

`Quality metrics` runs npm dependencies and repository code, so it holds a
read-only token. A same-repository dependency update would otherwise run its
packages' install scripts with a token that can push to the repository.

| Rejected | Why |
| --- | --- |
| A history of summarised measures on a `quality-metrics` branch, compared with the previous push | A count or a rounded mean names neither the function nor the pull request, and the history had no reader |
| An Issue opened when a measure crosses a threshold | The Issue gave the maintainer nothing to decide on; rounding the mean also raised false regressions |
| A complexity counter of the repository's own | gocyclo counts the same way and already ships with golangci-lint |
| Collecting CI durations from the GitHub API | The Actions pages already show the duration of every run and job |
| Failing CI on a coverage drop or a complexity rise | A required number invites code written for the number |
| qlty as the single tool | It does not measure test durations or flaky tests, and its lint layer repeats golangci-lint and ESLint |
