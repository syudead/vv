---
name: "sdd-plan-to-issues"
description: "Create native GitHub sub-issues directly from an approved plan's implementation-work section."
---

# Plan to native sub-issues

Read and execute `.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/plan-to-issues.md`. They are the
complete contract for this repository.

Each child Issue is what `/sdd-implement` builds from, so write the body up
for the implementer rather than transcribing the plan: expand the unit using the
artifacts the plan points at, and link the plan, the feature directory, and the
branch. Every statement must trace to the spec, the plan, or an artifact
([P-5](../../../docs/design-docs/plan-quality.md)). Anything none of them
settles is a question for the user, not something to invent here.

Use an available GitHub integration that supports Issue creation and native
sub-issue attachment. If a required operation is unavailable, stop before
creating anything. Do not create `tasks.md`, ordinary unparented fallback
Issues, or repository result packets.
