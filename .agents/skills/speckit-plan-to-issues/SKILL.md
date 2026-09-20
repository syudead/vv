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

Each child Issue is what `/speckit-implement` builds from, so check the plan's
units against P-5 in
[docs/design-docs/plan-quality.md](../../../docs/design-docs/plan-quality.md)
before creating anything. A unit that is not implementable on its own is sent
back to `/speckit-plan`; it is never patched with invented content here.

Use an available GitHub integration that supports Issue creation and native
sub-issue attachment. If a required operation is unavailable, stop before
creating anything. Do not create `tasks.md`, ordinary unparented fallback
Issues, or repository result packets.
