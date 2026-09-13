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
- `sdd` ラベル付きの PR を `main` にマージすると `/sdd-next` が Spec Kit の次の段階を
  自動で回す。手順は `.claude/skills/sdd-next/SKILL.md`、設計は
  `docs/design-docs/sdd-loop-harness.md`。
