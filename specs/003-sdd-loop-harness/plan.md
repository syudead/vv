# Implementation Plan: Issue handoff SDD

**Date**: 2026-09-19 | **Spec**: [spec.md](./spec.md)

## Summary

Replace the Claude Routine controller with agent-neutral Markdown workflows.
Keep the long-lived feature branch and sub-branch PR topology. Use native
GitHub relationships for remote discovery and a pure local script for artifact
state. Human review and explicit invocation are the only stage transitions.

## Components

| Component | Responsibility |
| --- | --- |
| `docs/agent-workflows/*.md` | Canonical, agent-neutral stage procedures |
| agent skills | Thin entry points that defer repository behavior to the canonical workflows |
| `scripts/issue-handoff/sdd-stage.sh` | Local artifact and stale-dependency inspection |
| `tests/issue-handoff/run.sh` | Network-free fixture tests |
| GitHub native relationships | Parent/sub-issue and Issue/PR discovery |
| `.github/workflows/ci.yml` | Validation on every PR; never starts an agent |

## Dependency direction

```text
agent adapter -> docs/agent-workflows -> Spec Kit artifact procedures
                                  -> native GitHub integration

local test -> sdd-stage.sh -> feature artifacts + git history
```

The local path has no dependency on GitHub, an agent, Issue text, or branch
names. GitHub authentication and API details do not enter repository scripts.

## Artifact state

The local script receives `--feature specs/NNN-name` and optional `--ui`.
It validates one parent line, requires a clean artifact directory, and emits
human-readable `key=value` lines. It selects:

1. `specify` when `spec.md` is absent.
2. `plan` when `plan.md` is absent or does not contain the latest Spec commit.
3. `design` for UI work when `ui-design.md` is absent or stale against Plan.
4. `tasks` when `tasks.md` is absent or stale against its direct input.
5. `taskstoissues` when all artifacts are current.

Git ancestor checks are used instead of file timestamps so merge topology is
preserved across machines.

## GitHub operations

The common workflows describe required queries and mutations but do not name a
transport. Claude and Codex use GitHub MCP. Another agent may use its own
official-API integration. Environments without the required capability stop
before mutation; `gh` is not a fallback in this repository.

`taskstoissues` is intentionally a GitHub-only operation. It compares task IDs
with the parent's native sub-issues, creates and attaches missing children,
updates non-material wording, and closes cancelled tasks as not planned.

## Migration order

1. Enable CI for every PR through a small `main`-targeting bootstrap change.
2. Add canonical workflows, local state, and tests.
3. Point agent adapters and contributor documentation to the workflows.
4. Replace old design/spec/reference material.
5. Remove `/sdd-next`, its scripts/tests, usage hook, and old Make target.
6. Verify locally, then verify native GitHub behavior with a dedicated Issue.
7. Disable and delete the external Claude Routine and remove the `sdd` label
   after legacy PRs no longer need it.

## Verification

- `make test-agent-workflows`
- `make check`
- Search for executable references to `sdd-next`, old branch naming, and the
  automation label.
- GitHub live test covering Spec recovery, integration-PR discovery,
  sub-issue parent lookup, child completion, parent close, and CI on a
  feature-targeting PR.

## Risks and decisions

- The gap between pushing an empty feature branch and creating its Spec PR is
  not recoverable without custom state. This is accepted; orphan cleanup is
  manual.
- Explicit human invocation and merge replace automatic throughput, retry,
  and reconciliation. They are deliberate removals, not deferred automation.
- Child close means implemented on the feature branch, not shipped. Parent
  close is the shipped/integrated signal.
