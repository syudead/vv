---
name: issue-handoff
description: Run one stage of this repository's Issue-driven SDD workflow, including parent/sub-issue discovery, feature-branch recovery, artifact work, implementation, and PR handoff. Use when starting or continuing SDD work from a GitHub Issue or native sub-issue.
---

# Issue handoff

Read [references/README.md](references/README.md) before acting. It is the
shared contract for branch discovery, artifact state, GitHub relationships,
and stopping conditions.

Then read exactly the stage reference required by the parent Issue's `Next`
value or by the supplied child Issue:

- `specify`: [references/specify.md](references/specify.md)
- `plan`: [references/plan.md](references/plan.md)
- `design`: [references/design.md](references/design.md)
- `tasks`: [references/tasks.md](references/tasks.md)
- `taskstoissues`: [references/taskstoissues.md](references/taskstoissues.md)
- child Issue implementation: [references/implement.md](references/implement.md)

Perform one stage, open or update one pull request, and stop. Use the native
GitHub integration available to the current agent. Never fall back to `gh`.
