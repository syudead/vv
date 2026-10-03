# Handle dependency updates (Renovate)

The [Renovate](https://docs.renovatebot.com/) GitHub App opens dependency update
PRs according to `renovate.json`. The intent behind the configuration lives in
this document, not in `renovate.json` itself.

## What Renovate does

- Early every Monday morning (JST), it opens PRs in 6 groups: Go modules, web
  npm, tools npm, GitHub Actions, mise tools, and container images.
  Vulnerability fix PRs are opened without waiting for Monday.
- Renovate merges minor, patch, lockfile maintenance and digest updates itself
  once the PR's required checks (Checks) pass. Browser E2E and Docker image run
  on the push to `main` after the merge.
- Major updates stay open as PRs. A person reads the breaking changes and
  merges them.
- GitHub Actions are pinned to commit hashes, with the tag name in a comment
  (the `config:best-practices` default).
- Base images in `Dockerfile` are pinned as `tag@sha256:digest`. When the
  content behind the same tag changes, Renovate opens a digest update PR.

## Excluded dependencies

| Dependency | Treatment | Reason |
| --- | --- | --- |
| `golang` / `node` images in `Dockerfile`; `go` / `node` in `mise.toml` | Version changes (major / minor / patch) are excluded; digest updates of the `Dockerfile` images stay in scope | The runtime version must match the `go` line in `go.mod`, so a person changes them together when raising `go.mod` |
| `task` and `jq` in `mise.toml`; `alpine` in `Dockerfile` | In scope, in the mise tools and container images groups respectively | — |
| Dependabot security updates | Disabled in the repository settings | Renovate's `vulnerabilityAlerts` does the same job |

The mise manager names `go` as packageName `golang/go` and `node` as `nodejs`,
so the exclusion uses `matchDepNames`, which both managers share, instead of
`matchPackageNames`.

## Renovate PRs that need a person

- A PR whose automerge stopped has a CI failure, a conflict, or is a major
  update. Renovate's Dependency Dashboard Issue lists them.
- To pause an update, close the PR (the same version is not opened again) or set
  `enabled: false` in `packageRules` in `renovate.json`.

## Changing the configuration

Validate with `npx --package renovate renovate-config-validator` before pushing.
