# Clear the vulnerability audit

An `audit-fixer` worker follows this page for the brief it gets from the
renovate-triage routine. It opens or updates the one pull request that makes
`task audit` pass on `main`, or says what blocks it. It never merges.

## Set up

Fetch `origin/main`. With `PR: -` in the brief, create `audit/<UTC date>` from
it; otherwise check out the PR's head branch and merge `origin/main` into it
(never rebase). Read
[dependency-updates.md](../../../../docs/how-to/dependency-updates.md#vulnerability-audit)
first.

Collect the findings:

- **npm**: run `task audit-npm`. It prints one line per advisory with its
  directory, package, vulnerable range and GHSA ID, and one line per entry of
  `scripts/audit/accepted.json` that no lockfile has any more.
- **Go**: run `task audit-go` and `task audit-go-windows`. When the
  vulnerability database is unreachable from the session, read the failed
  steps of the `Audit` run named in the brief instead.

## Fix each finding

Use the first step that applies. Regenerate a lockfile with
`npx --yes npm@11 install --package-lock-only` in its directory; never edit
one by hand.

| Finding | Fix |
| --- | --- |
| npm advisory, and a patched version satisfies the range its parents ask for | `npx --yes npm@11 update <package> --package-lock-only` in that directory |
| npm advisory, and a parent pins the vulnerable range | Add `"<package>@<vulnerable range>": "<patched range>"` to `overrides` in that `package.json` |
| npm advisory without a patched version, in a package only development tooling loads (`npm ls --omit=dev <package>` in the directory prints no path) | Add it to `scripts/audit/accepted.json` with the reason and a `revisit` condition a later run can check, such as `a <package> release newer than <version> is published` |
| npm advisory without a patched version, in a package the SPA ships | None: return `BLOCKED` |
| Accepted advisory that no lockfile has any more | Remove the entry |
| Go standard library | Raise the Go patch version to the newest patch of the same minor in `go.mod`, `tools/go.mod` and `mise.toml`. Never raise the minor or major version. When the `golang` image pinned in `Dockerfile` is older than that patch, return `BLOCKED` with `REVISIT: the container images PR is merged` |
| Go module | `go get <module>@<fixed version>` and `go mod tidy`, in the module that requires it |

When a fix changes an API the code relies on, adapt the code with a focused
test, as for a Renovate update
([worker.md, Failed checks](worker.md#failed-checks)).

## Weekly upkeep

With `Upkeep: yes` in the brief, also undo what is no longer needed:

- For each `overrides` entry in `web/package.json`, `tools/package.json` and
  `docs-site/package.json`: remove it, regenerate the lockfile, and run
  `task audit-npm`. Keep the removal when the audit passes; otherwise restore
  the entry and the lockfile. When a document names the removed entry
  (`docs/how-to/docs-site.md` names the Vite one), update it in the same
  commit.
- For each entry of `scripts/audit/accepted.json`: check its `revisit`
  condition. When it holds, remove the entry and fix the advisory by the table
  above.

## Before pushing

With nothing to change, return `CLEAN` without a branch or a PR. Otherwise run
`task check`, `task audit-npm` and `task check-docs`. Commit with a
Conventional Commit message (`fix(deps): ...`) that names the advisories. Push
once, never with force.

With `PR: -`, open a pull request to `main` and label it `security`. Write the
body in Japanese on
[the PR template](../../../../.github/pull_request_template.md): one row per
finding with its advisory link and the fix. On a later attempt, post one
`<!-- renovate-triage:attempt -->` comment instead, saying what failed and
what the commit changes.

## Never

- Change `scripts/audit/`, `Taskfile.yml`, `.github/workflows/audit.yml` or
  `renovate.json`, or accept an advisory that has a patched version.
- Skip, disable, loosen or quarantine a test or a check.
- Touch a pull request other than the one this page opens.

## Return

End with exactly this block:

```text
STATUS: PUSHED | CLEAN | BLOCKED
PR: <number, or ->
COMMIT: <pushed commit, or ->
REASON: <one line per finding: the advisory and what fixed it, or what blocks it>
REVISIT: <for BLOCKED: the condition a later run can check, or `a person decides`; otherwise ->
```

- `PUSHED`: a commit was pushed. CI has to run.
- `CLEAN`: `task audit` passes on `main` and upkeep changed nothing.
- `BLOCKED`: a finding has no fix this page allows. Push the fixes for the
  other findings first and name the blocked one in `REASON`.
