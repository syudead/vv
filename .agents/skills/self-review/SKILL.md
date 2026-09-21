---
name: "self-review"
description: "Reconcile a finished change against its sources of truth before pushing. Use after implementation or artifact work is complete and the repository checks pass, and before opening or updating a pull request."
---

# Self review

Read [references/README.md](references/README.md) before acting. It defines the
five checks, the evidence each one has to produce, and which findings stop the
push.

This runs inside the session that made the change, between the repository
checks and the push. Its purpose is not to find defects an external reviewer
would otherwise miss — a reviewer already finds them on the pull request. Its
purpose is to find them **before** the push, while the context that produced
the change is still loaded. A finding answered after a push costs a whole
session; the same finding answered here costs one pass.

Produce findings, fix them, re-run the repository checks, then continue to the
push. When a finding cannot be resolved without changing an approved artifact,
stop and hand it to the maintainer instead of widening the change.

[references/eval.md](references/eval.md) measures how much of a real reviewer's
output this skill reaches, against known answers.
