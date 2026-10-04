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
is a question for the requester rather than a choice to make here. A screen is
composed from the components and page patterns of the
[design system](../../../docs/design-docs/design-system.md); when it needs one
the design system lacks, the design adds it there first.

Start `ui-design.md` from [`assets/ui-design-template.md`](assets/ui-design-template.md)
and follow [writing-quality.md](../../../docs/design-docs/writing-quality.md):
English, tables for words, states and widths, and a diagram for multi-step
interactions. Once it is final, hand it to the `doc-translator` subagent and
commit its `translations/ja/` output in the same pull request.
