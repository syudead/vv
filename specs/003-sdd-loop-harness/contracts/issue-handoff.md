# Contract: Issue handoff

## Input

One explicit GitHub parent Issue or native sub-issue.

## Preconditions

- Required GitHub read/push/PR capabilities are available.
- Tasks-to-sub-issues additionally has Issue write and native sub-issue access.
- Parent SDD summary agrees with merged feature artifacts.
- The requested stage has no other open PR, except the PR being revised.

## Output

- Specify, Plan, Design, Tasks, or Implement: one PR to the feature branch,
  then stop.
- Tasks-to-sub-issues: reconciled native children and no `Next` line, then stop.
- Integration: one existing feature-to-`main` PR reviewed and merged by a human.

## Failure

Missing capability, ambiguous GitHub relationship, invalid parent mapping,
dirty artifacts, stale parent summary, or duplicate child ID stops before a
new mutation. No fallback branch naming, Issue-number conversion, `gh`, or
state packet is permitted.

