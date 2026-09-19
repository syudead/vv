# Data Model: Issue handoff SDD

No dedicated runtime state is persisted. The following values are derived.

## Repository values

| Value | Source | Rule |
| --- | --- | --- |
| feature directory | explicit workflow input | normalized path under `specs/` |
| parent Issue | `spec.md` | exactly one `**Parent Issue**: #NNN` line |
| workflow | parent `ui` label | `standard` or `ui`; passed explicitly to local inspection |
| artifact stage | files and Git history | first missing or stale downstream artifact |
| task ID | `tasks.md` | `T` plus at least three digits; immutable after child creation |

Artifact dependency order is `spec -> plan -> tasks` or
`spec -> plan -> ui-design -> tasks`. A downstream artifact is current only
when the latest commit touching its direct input is an ancestor of the latest
commit touching the downstream file.

The repository can prove only that Tasks are ready for reconciliation. Whether
Tasks-to-sub-issues has run is GitHub state: `Next: taskstoissues` is pending;
no `Next` and reconciled native children is complete.

## GitHub values

| Value | Source |
| --- | --- |
| feature branch | head of the unique open integration PR that targets `main` and closes the parent |
| active pre-Spec work | unique open Spec PR cross-referenced from the parent |
| child parent | native GitHub sub-issue parent relationship |
| stage relationship | normal Issue reference in a feature-targeting PR |
| shipped state | parent closed by the merged integration PR |
| implemented state | child closed after its implementation PR merges to the feature branch |

These values are fetched by an agent adapter and are never written to a
repository state file.

## Parent SDD summary

The parent summary is a human-maintained projection of artifact state. It has
checkboxes for Spec, Plan, optional Design, and Tasks, plus one `Next` value
while artifact work remains. It does not contain PR, branch, child, retry,
agent, or session records.

## Task lifecycle

- New: unchecked task and open child.
- Implemented: checked task and completed child.
- Cancelled: checked struck-through task with a reason and child closed as not
  planned.
- Revised materially: old task cancelled and a new monotonically larger ID
  created.
