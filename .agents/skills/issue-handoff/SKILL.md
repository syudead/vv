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

- `specify`: [references/specify.md](references/specify.md)
- `plan`: [references/plan.md](references/plan.md)
- `design`: [references/design.md](references/design.md)
- `plan-to-issues`: [references/plan-to-issues.md](references/plan-to-issues.md)
- child Issue implementation: [references/implement.md](references/implement.md)

Perform one stage, open or update one pull request, and stop.
