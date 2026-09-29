# Dependency updates (Renovate)

The [Renovate](https://docs.renovatebot.com/) GitHub App opens dependency update PRs according to
`renovate.json`. The intent of the configuration lives in this document, not in `renovate.json`
itself.

## What happens

- Every Monday early morning (JST), PRs open in 6 groups: Go modules / web npm / tools npm /
  GitHub Actions / mise tools / container images. Vulnerability fix PRs open without waiting for
  the weekday.
- Renovate merges minor, patch, lockfile maintenance and digest updates automatically once the
  PR's required checks (Checks) pass. Browser E2E and Docker image run on the push to `main` after
  the merge.
- Major update PRs stay open. A person reads the breaking changes and merges them.
- GitHub Actions are pinned to commit hashes, with the tag name in a comment (the
  `config:best-practices` default).
- `Dockerfile` base images are pinned as `tag@sha256:digest`. When the content behind the same tag
  changes, Renovate opens a digest update PR.

## Excluded

- The `golang` / `node` images in `Dockerfile` and `go` / `node` in `mise.toml`. The runtime
  versions must match the `go` line in `go.mod`, so a person changes them together when raising
  `go.mod`. The mise manager uses the packageName `golang/go` for `go` and `nodejs` for `node`, so
  the exclusion uses `matchDepNames`, common to both managers, instead of `matchPackageNames`.
- The exclusion stops only version changes (major / minor / patch). Digest updates of the
  `Dockerfile` images stay in scope.
- `task` and `jq` in `mise.toml` and `alpine` in `Dockerfile` are in scope, in the mise tools and
  container images groups respectively.
- Dependabot security updates are disabled in the repository settings. Renovate's
  `vulnerabilityAlerts` does the same job.

## Handling Renovate PRs

- A PR whose automerge stopped has a CI failure, a conflict, or a major update. Renovate's
  Dependency Dashboard Issue lists them.
- To pause an update, close the PR (the same version is not recreated), or set `enabled: false`
  in `packageRules` of `renovate.json`.

## After changing the configuration

Validate with `npx --package renovate renovate-config-validator` before pushing.
