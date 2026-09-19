# Agent guide

This file is the short map for contributors and coding agents. Keep detailed
knowledge in the appropriate document under `docs/` rather than expanding this
file into a handbook.

## Start here

1. Read `ARCHITECTURE.md` for system boundaries and dependency direction.
2. Read `docs/design-docs/index.md` and `docs/product-specs/index.md` for the
   relevant design and product context.
3. For substantial work, create an execution plan in
   `docs/exec-plans/active/` and move it to `docs/exec-plans/completed/` when
   the work is finished.
4. Record known compromises in `docs/exec-plans/tech-debt.md`.

## Working agreements

- Keep documentation close to the code and update it with behavior changes.
- When writing or changing a specification, follow
  [docs/product-specs/spec-quality.md](docs/product-specs/spec-quality.md).
- Prefer focused, reviewable changes with automated checks.
- Do not hand-edit files in `docs/generated/`; update their source or generator.
- Add links to new design documents and product specifications from their
  respective index files.
- Every push to a feature branch gets a pull request. After pushing, open a PR
  against `main` if one does not exist yet, so no pushed branch is left without
  a review target.
- When a change alters how a screen looks or behaves, attach an image of the
  result to the pull request; say "UI 変更なし" when it does not. Capturing and
  embedding one is covered in
  [docs/how-to/ui-change-screenshots.md](docs/how-to/ui-change-screenshots.md).
- SDD work starts from an explicitly supplied parent Issue or native sub-issue.
  Use `.agents/skills/issue-handoff`; each run performs one stage and opens or
  updates one PR. Stage and implementation PRs target the long-lived feature
  branch, and only its integration PR targets `main`.
- The empty feature-branch push during `specify` is the sole temporary exception
  to the rule that every pushed feature branch already has a PR to `main`.
  Open the integration PR immediately after the Spec PR is merged.
