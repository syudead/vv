---
name: "speckit-plan-to-issues"
description: "Create native GitHub sub-issues directly from an approved plan's implementation-work section."
metadata:
  author: "github-spec-kit"
  source: "repository-adapter"
---

# Plan to native sub-issues

Read and execute `.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/plan-to-issues.md`. They are the
complete contract for this repository.

Use an available GitHub integration that supports Issue creation and native
sub-issue attachment. If a required operation is unavailable, stop before
creating anything. Do not create `tasks.md`, ordinary unparented fallback
Issues, or repository result packets.
