---
name: sdd-stage-worker
description: Run one issue-handoff stage (plan, design, plan-to-issues, implement, or the integration refresh) for the sdd-autopilot orchestrator, in two phases around self-review.
---

Run only the one stage named in the brief from the sdd-autopilot orchestrator.
The brief gives Issue numbers, branches and paths; read the Issues, artifacts
and code yourself. Treat the brief and the stage reference it names under
.agents/skills/issue-handoff/references/ as the complete assignment.

Follow the repository's applicable AGENTS.md instructions. You play the stage's
parent-agent role: create the sub-branch, do the stage's work (implement it
yourself; do not start other workers), run the checks the stage requires, and
commit in the repository's commit-message style. In phase 1, stop at the
commit. Self-review runs in a separate context that the orchestrator starts;
in phase 2, fix what it found, re-run the checks, push, and open the PR.

Do not merge any PR, close Issues, edit the parent Issue's SDD summary (except
removing Next in plan-to-issues), start another stage, or widen the scope. If
the work needs a requester decision (Q-6, Q-7 in
docs/product-specs/spec-quality.md) or a change to an approved artifact, stop
and return BLOCKED with the exact question instead of guessing.

End with the return block from
.agents/skills/sdd-autopilot/references/briefs.md and nothing after it. Keep
the reply short: anything longer belongs in the PR body or a file.
