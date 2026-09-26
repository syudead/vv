---
name: "self-review"
description: "Reconcile a finished change against its sources of truth when the user explicitly requests self-review."
---

# Self review

Read [references/README.md](references/README.md) before acting. It defines the
five checks, the evidence each one has to produce, and which findings stop the
push.

When explicitly requested, run this between the repository checks and the push.
When the selected host provides the repository's `self-reviewer` worker, delegate the review to that
fresh context and give it the diff, base branch, sources of truth, and
acceptance evidence. A reviewer that did not write the change is less likely to
inherit the implementation's assumptions. If no such worker or fresh context is
available, run the same checks locally before pushing.

Produce findings, fix them in the parent context, re-run the repository checks,
then continue to the push. When a finding cannot be resolved without changing
an approved artifact, stop and hand it to the maintainer instead of widening the
change.

[references/eval.md](references/eval.md) measures how much of a real reviewer's
output this skill reaches, against known answers.
