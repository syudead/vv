# UI design workflow

Read [README.md](README.md) first. This workflow is used when the user requests
UI design work for a parent Issue.

1. Resolve the feature branch and explicit feature directory from the parent.
2. Read the available spec, plan, and existing UI design as inputs.
3. Create an arbitrary-name sub-branch from the feature branch.
4. Create `<feature-dir>/ui-design.md` from the approved spec, plan, existing
   product UI, and supplied references. Define screen boundaries, visual
   hierarchy, responsive behavior, content and system states, interactions,
   accessibility, and observable review criteria. Do not implement code.
5. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
6. After human merge, the maintainer marks Design complete, removes `Next`, and
   creates implementation children directly from the approved plan and design
   as needed.

