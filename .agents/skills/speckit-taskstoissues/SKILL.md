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

Use GitHub MCP for Issue and sub-issue operations. Never invoke `gh`, search
the whole repository for matching task titles, create ordinary unparented
Issues as a fallback, or write a result packet to the repository. If GitHub
MCP cannot list, create, update, close, and attach native sub-issues, stop
before creating anything and report the missing capability.

The operation is complete only when every current task ID has exactly one
child under the supplied parent, cancelled tasks are closed as not planned,
and the parent's `Next` line has been removed. Do not edit the integration PR
or add child links to the parent body.
