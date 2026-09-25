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
- SDD work starts from an explicitly supplied parent Issue or native sub-issue
  and runs `plan → design → plan-to-issues → implement`, with `design` only for
  `ui` Issues. Use `.agents/skills/issue-handoff`; each run performs one stage
  and opens or updates one PR. Stage and implementation PRs target the
  long-lived feature branch, and only its integration PR targets `main`.
- The empty feature-branch push during `plan` is the sole temporary exception
  to having a review target. Open the integration PR immediately after the Plan
  PR is merged.
- Refresh an integration PR by merging the latest `main` into its feature
  branch and rerunning the required checks. Updating an existing PR branch does
  not get a separate PR.
