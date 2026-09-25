---
name: issue-handoff
description: Run one explicitly requested stage of this repository's Issue-driven SDD feature workflow using a supplied GitHub Issue, PR, or branch as context.
---

# Issue handoff

This is an explicit feature workflow. Other changes use the repository's normal
branch-to-`main` workflow.

Read [references/README.md](references/README.md) before acting. It is the
shared contract for artifact work, GitHub context, PR behavior, and stopping
conditions.

When the selected host provides the repository's bounded workers, delegate the
implementation step to the `subissue-implementer` role and the self-review step
to the `self-reviewer` role. In Codex these agents are named
`subissue_implementer` and `self_reviewer`; in Claude they are named
`subissue-implementer` and `self-reviewer`. The parent agent still owns
checkout, branching, full validation, fixes, push, and the pull request.

Then read the stage reference for the workflow requested by the user. The
parent Issue's `Next` value is a hint, not an execution gate:

- `plan`: [references/plan.md](references/plan.md)
- `design`: [references/design.md](references/design.md)
- `plan-to-issues`: [references/plan-to-issues.md](references/plan-to-issues.md)
- child Issue implementation: [references/implement.md](references/implement.md)

Writing or revising the requirement itself is not a stage here. The parent
Issue is the specification, and the
[`issue-spec` skill](../issue-spec/SKILL.md) writes it.

Perform one stage, open or update one pull request, and stop. To run every
stage up to the integration merge unattended, the maintainer uses the
[`sdd-autopilot` skill](../sdd-autopilot/SKILL.md) instead, which drives these
stages through its own workers.
