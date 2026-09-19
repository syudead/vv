# Contract: `sdd-stage.sh`

## Invocation

```bash
bash scripts/issue-handoff/sdd-stage.sh --feature specs/NNN-name [--ui] [--root PATH]
```

## Output

Successful output is newline-delimited, human-readable `key=value` text:

```text
feature=specs/NNN-name
workflow=standard
next=tasks
reason=missing tasks.md
parent_issue=123
```

`next` is one of `specify`, `plan`, `design`, `tasks`, or `taskstoissues`.
The command writes no files and performs no network request.

`taskstoissues` is an artifact-readiness result, not proof that GitHub
sub-issues are absent. Pending/completed reconciliation is distinguished by
the parent Issue's `Next` line and its native sub-issues.

## Errors

Exit 2 for an invalid or non-normalized feature path, missing directory,
invalid parent line, or dirty artifact directory. Existing downstream content
is not compared with earlier artifacts; revisions are coordinated through the
parent Issue summary and reviewed stage PRs.
