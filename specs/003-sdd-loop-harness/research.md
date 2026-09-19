# Research: Issue handoff SDD

## R-001: Handoff surface

**Decision**: Use an explicitly supplied GitHub Issue and native GitHub
relationships. Do not use PR-merge events as an execution trigger.

**Why**: An Issue survives agent and session changes. GitHub already owns
Issue/PR and parent/sub-issue relationships, so copying them into a custom
packet creates a second, divergent state store.

## R-002: Work context

**Decision**: Use the explicitly supplied Issue, PR, branch, and current
checkout directly. Read standard GitHub relationships only when useful to the
requested work. Do not prescribe a repository-specific recovery traversal or
uniqueness check.

**Why**: The caller already supplies the work target. Reconstructing it through
several records adds ordering and consistency requirements without improving
the requested change.

## R-003: Local state

**Decision**: Inspect one explicit feature directory and Git ancestor
relationships. Emit human-readable text, not a persistent JSON record.

**Why**: File existence identifies missing work, while ancestry detects that a
later artifact predates a revised input. Timestamps and sessions are not stable
across clones.

## R-004: Child completion

**Decision**: Close a child after its implementation PR merges to the feature
branch. Close the parent only after integration to `main`.

**Why**: Native sub-issue progress then reflects implementation progress. If
children stayed open until final integration, the progress display would show
no completed work and active-task selection would be ambiguous.

## R-005: GitHub transport

**Decision**: Keep transport selection outside the repository workflow. No
repository command calls GitHub.

**Why**: Authentication and tool availability differ by agent. Artifact-state
logic remains portable only when it is network-free.

## R-006: Automation removed

**Decision**: Remove Routine, automation label, scheduled reconciliation,
automatic merge, hop/retry limits, and usage gating.

**Why**: They exist to operate an unattended loop. Keeping them after adopting
explicit one-stage invocation would retain the coupling the migration removes.
