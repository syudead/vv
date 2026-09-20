---
name: "sdd-design"
description: "Create the UI and interaction design artifact for an Issue-driven SDD feature."
---

# UI design

Read and execute `.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/design.md`. This skill adds no branch naming, state,
trigger, or retry protocol. Produce only the Design-stage artifact and PR,
then stop.

`ui-design.md` holds what this feature's screens do that the repository's UI
documents do not already settle. The design system, the settled layout, the
token values and the existing screens decide everything they cover; the
artifact links them instead of restating them, and names tokens instead of
writing values. `design.md` lists those sources and the rule for when something
is a question for the requester rather than a choice to make here.
