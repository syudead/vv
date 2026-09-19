# Contract: Issue handoff

## Input

One explicit GitHub parent Issue or native sub-issue.

## Preconditions

- Issue and PR read capabilities are available.
- PR-producing workflows have repository push and PR creation access.
- Tasks-to-sub-issues has Issue write and native sub-issue access; it does not
  require repository push or PR creation access.
- Parent SDD summary agrees with merged feature artifacts.
- The requested stage has no other open PR, except the PR being revised.

## Output

- Specify, Plan, Design, Tasks, or Implement: one PR to the feature branch,
  then stop.
- Tasks-to-sub-issues: reconciled native children and no `Next` line, then stop.
- Integration: one existing feature-to-`main` PR reviewed and merged by a human.

## Failure

Missing capability, ambiguous GitHub relationship, or invalid parent mapping
stops before a new mutation. No fallback branch naming, Issue-number
conversion, or state packet is permitted.

