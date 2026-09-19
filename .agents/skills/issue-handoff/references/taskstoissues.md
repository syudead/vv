# Tasks-to-sub-issues workflow

Read [README.md](README.md) first. This is the only SDD workflow that writes
GitHub Issues. It does not change repository files or open a PR.

1. Resolve the parent, requested feature branch, and feature directory. Require
   a checked-in `tasks.md`; treat `Next: taskstoissues` as an optional progress
   hint.
2. Read task-list rows from the current `tasks.md`; do not scan retired or
   historical task files. Extract each `T` followed by at least three digits.
   List the parent's native sub-issues in both open and closed states. Match an
   ID only inside this parent's sub-issues, never repository-wide.
3. Stop before writing anything if native sub-issue read/add operations or
   Issue write access are unavailable, or if one ID occurs in multiple
   sub-issues.
4. Reuse the one matching child when present. Otherwise create
   `TNNN: <description>` and add it to the parent through GitHub's native
   sub-issue API. Include the task text, feature artifact links, dependencies,
   and acceptance evidence in the child body.
5. For a non-material wording update, update the existing child's body. For a
   cancelled task, close the existing child as not planned. Materially changed
   work uses the new task ID created by the Tasks workflow.
6. Do not put child links in the parent body and do not add child `Closes`
   entries to the integration PR.
7. When reconciliation succeeds, remove `Next` from the parent's SDD summary
   and stop. Re-running is idempotent against the parent's sub-issues.

Do not commit a task packet or result file.
