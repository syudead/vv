# Tasks-to-sub-issues workflow

Read [README.md](README.md) first. This is the only SDD workflow that writes
GitHub Issues. It does not change repository files or open a PR.

1. Resolve the parent, requested feature branch, and feature directory. Require
   a checked-in `tasks.md`; treat `Next: taskstoissues` as an optional progress
   hint.
2. Read checkbox task rows from the current `tasks.md`; do not scan retired or
   historical task files. List the parent's native sub-issues in both open and
   closed states.
3. Stop before writing anything if native sub-issue read/add operations or
   Issue write access are unavailable.
4. Immediately before creating each child, compare the proposed work with the
   parent's existing sub-issues. Skip creation when the same work is already
   represented. Otherwise create a child using the task description as its
   title and add it through GitHub's native sub-issue API. Include the task
   text, feature artifact links, dependencies, and acceptance evidence in the
   child body.
5. Do not automatically synchronize later task edits or cancellations to
   existing children. Update or close an existing child only when that Issue is
   explicitly requested.
6. Do not put child links in the parent body and do not add child `Closes`
   entries to the integration PR.
7. When reconciliation succeeds, remove `Next` from the parent's SDD summary
   and stop. Re-running is idempotent against the parent's sub-issues.

Do not commit a task packet or result file.
