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
