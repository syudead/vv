# Agent guide

This file is the short map for contributors and coding agents. Keep detailed
knowledge in the appropriate document under `docs/` rather than expanding this
file into a handbook.

## Start here

1. Read `ARCHITECTURE.md` for system boundaries and dependency direction.
2. Read `docs/design-docs/index.md` and `docs/product-specs/index.md` for the
   relevant design and product context.
3. For substantial work, the Plan (`specs/<feature>/plan.md`) carries the goal,
   the scope, the validation strategy and the notable decisions. Progress lives
   on the parent Issue and its child Issues, not in a file.

## Working agreements

- Keep documentation close to the code and update it with behavior changes.
- A feature's specification is its parent GitHub Issue. Write and revise it with
  `.agents/skills/issue-spec`, following
  [docs/product-specs/spec-quality.md](docs/product-specs/spec-quality.md).
  Do not add a `spec.md`.
- When writing or changing a Plan and its artifacts, follow
  [docs/design-docs/plan-quality.md](docs/design-docs/plan-quality.md).
- Prefer focused, reviewable changes with automated checks.
- Do not hand-edit generated files (`internal/httpapi/gen/`, `web/src/api/gen/`);
  change `api/openapi.yaml` and run `task generate`.
- Add links to new design documents from `docs/design-docs/index.md`.
- Every push to a feature branch gets a pull request. After pushing, open a PR
  against `main` if one does not exist yet, so no pushed branch is left without
  a review target.
- When a change alters how a screen looks or behaves, attach an image of the
  result to the pull request; say "UI 変更なし" when it does not. Capturing and
  embedding one is covered in
  [docs/how-to/ui-change-screenshots.md](docs/how-to/ui-change-screenshots.md).
  Renovate の PR はこの対象外。依存更新の運用は
  [docs/how-to/dependency-updates.md](docs/how-to/dependency-updates.md)。
- SDD work starts from an explicitly supplied parent Issue or native sub-issue
  and runs `plan → design → plan-to-issues → implement`, with `design` only for
  `ui` Issues. Use `.agents/skills/issue-handoff`; each run performs one stage
  and opens or updates one PR. Stage and implementation PRs target the
  long-lived feature branch, and only its integration PR targets `main`.
- The empty feature-branch push during `plan` is the sole temporary exception
  to the rule that every pushed feature branch already has a PR to `main`.
  Open the integration PR immediately after the Plan PR is merged.
