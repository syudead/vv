# UI design workflow

Read [README.md](README.md) first. This workflow is used only when the parent
Issue has the existing `ui` domain label and `Next: design`.

1. Resolve the feature branch and explicit feature directory from the parent.
2. Confirm that local state returns `next=design` with `--ui`.
3. Create an arbitrary-name sub-branch from the feature branch.
4. Create `<feature-dir>/ui-design.md` from the approved spec, plan, existing
   product UI, and supplied references. Define screen boundaries, visual
   hierarchy, responsive behavior, content and system states, interactions,
   accessibility, and observable review criteria. Do not implement code.
5. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
6. After human merge, the maintainer marks Design complete and sets
   `Next: tasks`.

