# Implementation Plan: Issue handoff SDD

**Date**: 2026-09-19 | **Spec**: [spec.md](./spec.md)

## Summary

Replace the Claude Routine controller with agent-neutral Markdown workflows.
Keep the long-lived feature branch and sub-branch PR topology. Use native
GitHub relationships as optional work context. Human review and explicit
invocation are the only stage transitions.

## Components

| Component | Responsibility |
| --- | --- |
| `.agents/skills/issue-handoff/` | Canonical, agent-neutral stage procedures |
| `.claude/skills` | Symlink to the shared Agent Skills directory |
| GitHub native relationships | Optional parent/sub-issue and Issue/PR context |
| `.github/workflows/ci.yml` | Validation on every PR; never starts an agent |

## Dependency direction

```text
shared Agent Skill -> Spec Kit artifact procedures
                   -> native GitHub integration
```

The local path has no dependency on GitHub, an agent, Issue text, or branch
names. GitHub authentication and API details do not enter repository scripts.

## Artifact state

For Spec Kit work, the skill uses an explicit `specs/NNN-name` directory and
inspects required artifacts in order. It selects:

1. `specify` when `spec.md` is absent.
2. `plan` when `plan.md` is absent.
3. `design` for UI work when `ui-design.md` is absent.
4. `plan-to-issues` after the final artifact is approved.

The skill does not infer whether downstream content incorporates a later
upstream revision. A maintainer resets the parent summary when artifacts are
revised, and review verifies the regenerated content.

## GitHub operations

The common workflows describe required queries and mutations but do not name a
transport. Environments without the capability required by the selected
workflow stop before mutation.

The approved Plan contains the implementation-work breakdown. `plan-to-issues`
creates native child Issues directly from it. Immediately before creating a child, the
workflow checks the parent's native sub-issues and skips work already
represented. Existing children are changed only when explicitly requested.

## Migration order

1. Enable CI for every PR through a small `main`-targeting bootstrap change.
2. Add canonical workflows, local state, and tests.
3. Point contributor documentation and every discovery path to the shared skill.
4. Replace old design/spec/reference material.
5. Remove `/sdd-next`, its scripts/tests, usage hook, and old Make target.
6. Verify locally, then verify native GitHub behavior with a dedicated Issue.
7. Disable and delete the external Claude Routine and remove the `sdd` label
   after legacy PRs no longer need it.

## Verification

- Agent Skill validation
- `task check`
- Search for executable references to `sdd-next`, old branch naming, and the
  automation label.
- GitHub live test covering supplied Issue/PR context, sub-issue parent lookup,
  child completion, parent close, and CI on a
  feature-targeting PR.

## Risks and decisions

- The workflow does not attempt to recover an unspecified branch or feature
  directory. Missing information needed for a mutation is requested directly.
- Explicit human invocation and merge replace automatic throughput, retry,
  and reconciliation. They are deliberate removals, not deferred automation.
- Child close means implemented on the feature branch, not shipped. Parent
  close is the shipped/integrated signal.
