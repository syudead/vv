---
name: "speckit-taskstoissues"
description: "Reconcile tasks.md with native GitHub sub-issues of the feature's parent Issue."
metadata:
  author: "github-spec-kit"
  source: "repository-adapter"
---

# Tasks to native sub-issues

Read and execute `.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/taskstoissues.md`. They are the complete contract for this
repository.

Use an available GitHub integration that can perform the Issue and native
sub-issue operations required by the shared workflow. If a required operation
is unavailable, stop before creating anything and report the missing
capability. Do not create ordinary unparented Issues as a fallback or write a
result packet to the repository.

The operation is complete when each task not already represented by a native
child has been created and attached, and the parent's `Next` line has been
removed. Do not edit the integration PR or add child links to the parent body.
