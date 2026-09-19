# Data Model: Issue handoff SDD

No dedicated runtime state is persisted. The following values are derived.

## Repository values

| Value | Source | Rule |
| --- | --- | --- |
| feature directory | explicit workflow input | normalized path under `specs/` |
| parent Issue | `spec.md` | exactly one `**Parent Issue**: #NNN` line |
| workflow | parent `ui` label | `standard` or `ui`; passed explicitly to local inspection |
| artifact stage | required files | first missing downstream artifact |

Artifact dependency order is `spec -> plan` or `spec -> plan -> ui-design`.
The repository does not persist or infer
which upstream revision produced a downstream artifact.

The approved Plan contains the implementation-work breakdown.
`Next: plan-to-issues` means child creation is pending; child creation and completion
are represented by native GitHub sub-issues.

## GitHub values

| Value | Source |
| --- | --- |
| feature branch | head of the unique open integration PR that targets `main` and closes the parent |
| active pre-Spec work | unique open Spec PR cross-referenced from the parent |
| child parent | native GitHub sub-issue parent relationship |
| stage relationship | normal Issue reference in a feature-targeting PR |
| shipped state | parent closed by the merged integration PR |
| implemented state | child closed after its implementation PR merges to the feature branch |

These values are fetched by the agent's native integration and are never written to a
repository state file.

## Parent SDD summary

The parent summary is a human-maintained projection of artifact state. It has
checkboxes for Spec, Plan, and optional Design, plus one `Next` value
while artifact work remains. It does not contain PR, branch, child, retry,
agent, or session records.

## Task lifecycle

- New: unchecked task and open child.
- Implemented: checked task and completed child.
- Cancelled: checked struck-through task with a reason and child closed as not
  planned.
- Revised: update the task text; an existing child changes only when explicitly
  requested.
