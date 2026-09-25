# Agent guide

This file is the short map for contributors and coding agents. Keep detailed
knowledge in the appropriate document under `docs/` rather than expanding this
file into a handbook.

## Start here

1. Read `ARCHITECTURE.md` for system boundaries and dependency direction.
2. Read `docs/design-docs/index.md` and `docs/product-specs/index.md` for the
   relevant design and product context.
3. For substantial feature work, the Plan (`specs/<feature>/plan.md`) carries
   the goal, the scope, the validation strategy and the notable decisions.
   Progress is read from the feature branch's artifacts, the integration PR
   and the native child Issues; it is not recorded in a file or in the parent
   Issue body.

## Working agreements

- Keep documentation close to the code and update it with behavior changes.
- A feature's specification is its parent GitHub Issue. Write and revise it with
  `.agents/skills/issue-spec`, following
  [docs/product-specs/spec-quality.md](docs/product-specs/spec-quality.md).
  Do not add a `spec.md`.
- The default workflow is a focused branch and a pull request to `main`. Use
  Issue-driven SDD only when the maintainer explicitly starts it from a parent
  Issue or native sub-issue.
- When writing or changing a Plan and its artifacts, follow
  [docs/design-docs/plan-quality.md](docs/design-docs/plan-quality.md).
- Prefer focused, reviewable changes with automated checks.
- Do not hand-edit generated files (`internal/httpapi/gen/`, `web/src/api/gen/`);
  change `api/openapi.yaml` and run `task generate`.
- Add links to new design documents from `docs/design-docs/index.md`.
- Give every pushed working branch a pull request as its review target.
- 依存更新（Renovate）の運用は
  [docs/how-to/dependency-updates.md](docs/how-to/dependency-updates.md)。
- SDD work starts when the maintainer hands over a parent Issue or native
  sub-issue URL, and runs `plan → design → plan-to-issues → implement →
  integrate`, with `design` only for `ui` Issues. Use
  `.agents/skills/issue-handoff`; it selects the next stage from GitHub and
  feature-branch state, and each run performs one stage and opens or updates
  one PR. Stage and implementation PRs target the
  long-lived feature branch, and only its integration PR targets `main`.
- To run a feature unattended up to (not including) the integration merge,
  the maintainer explicitly starts `.agents/skills/sdd-autopilot` with the
  parent Issue. It merges feature-branch PRs itself; never the integration PR.
- The long-lived feature branch is the one exception to giving every pushed
  branch its own pull request: until `integrate`, its review targets are the
  stage and implementation PRs into it. `integrate` opens its integration PR
  once every child is done.
- Refresh an integration PR by merging the latest `main` into its feature
  branch and rerunning the required checks, when it conflicts with `main` or a
  check fails because of `main`. Updating an existing PR branch does not get a
  separate PR. On the integration PR, fix only blocking review findings and
  list the rest for the maintainer
  ([integrate](.agents/skills/issue-handoff/references/integrate.md#review-of-the-integration-pr)).
