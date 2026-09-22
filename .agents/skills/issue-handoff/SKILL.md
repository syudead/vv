---
name: issue-handoff
description: Run one stage of this repository's Issue-driven SDD workflow using an explicitly supplied GitHub Issue, PR, or branch as context. Use for artifact work, Plan-to-issues, implementation, and PR handoff.
---

# Issue handoff

Read [references/README.md](references/README.md) before acting. It is the
shared contract for artifact work, GitHub context, PR behavior, and stopping
conditions.

Then read the stage reference for the workflow requested by the user. The
parent Issue's `Next` value is a hint, not an execution gate:

- `plan`: [references/plan.md](references/plan.md)
- `design`: [references/design.md](references/design.md)
- `plan-to-issues`: [references/plan-to-issues.md](references/plan-to-issues.md)
- child Issue implementation: [references/implement.md](references/implement.md)

Writing or revising the requirement itself is not a stage here. The parent
Issue is the specification, and the
[`issue-spec` skill](../issue-spec/SKILL.md) writes it.

Perform one stage, open or update one pull request, and stop.
