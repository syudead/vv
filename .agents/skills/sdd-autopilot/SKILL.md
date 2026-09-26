---
name: sdd-autopilot
description: Drive one Issue-driven SDD feature from its parent Issue through every stage, implementation PR and review round, stopping just before the integration PR is merged. Use only when the maintainer explicitly asks for the feature to run unattended up to the integration merge.
---

# SDD autopilot

The maintainer supplies one parent Issue and asks for the feature to run on its
own until the integration PR is ready to merge. This skill is the orchestrator
for that request. It does not do stage work itself: every stage, review and fix
runs in a fresh worker context, and the orchestrator only decides what runs
next, merges what is ready, and keeps the parent Issue's bookkeeping current.

It reuses the one-stage procedures unchanged, including how the next stage is
selected. Read
[../issue-handoff/references/README.md](../issue-handoff/references/README.md)
once for the stage selection and the branch and PR contract; the stage
references are read by the workers, not by you.

## What the maintainer delegates

Starting this skill is the maintainer's explicit hand-off of the steps the
one-stage workflow leaves to them, **for feature-branch PRs only**:

- starting the next stage as soon as the previous one merges, instead of
  waiting to be handed the Issue URL again
- merging stage and implementation PRs into the feature branch once they pass
  the gates in [references/loop.md](references/loop.md)
- closing a child Issue as completed after its PR merges

The integration PR is never merged here. The run ends when it is green,
reviewed and mergeable, and reports that to the maintainer.

## Keeping the orchestrator's context small

The orchestrator runs for hours across a dozen PRs. It stays useful only if its
own context holds decisions, not material. These rules are the point of the
skill; follow them even when reading something yourself looks quicker.

1. **GitHub is the state.** At the start of every iteration, re-derive the next
   action from the parent Issue, its sub-issues and the feature branch's PRs
   ([references/loop.md](references/loop.md) §1). Never rely on remembering
   what happened earlier in the conversation, so compaction or a restarted
   session loses nothing. Do not write a state file, ledger, or tracking
   comment.
2. **Read summaries, not material.** Request only the fields
   [references/loop.md](references/loop.md) §1 lists (`fields`,
   `minimal_output`, `perPage`). Do not open diffs, CI job logs, review
   comment bodies, child Issue bodies, or artifact files. When a decision
   needs one of those, it is a worker's job.
3. **Brief with pointers, not content.** A worker brief names the Issue, PR,
   branch, base and feature directory, and the worker reads them itself. Do not
   paste Issue text, findings, or diffs into a brief. The templates are in
   [references/briefs.md](references/briefs.md).
4. **Fixed, short returns.** Every worker ends with the return block from
   [references/briefs.md](references/briefs.md). Act on its `STATUS` line and
   keep only the numbers you need (PR, commit, blocker). Do not restate a
   worker's report to the maintainer.
5. **One worker per unit of work.** A new stage, a new review round, and the
   integration refresh each get a fresh worker. Continue an existing worker
   only to hand it the self-review result for the change it just made.

## Workers

| Role | Claude | Codex | Does |
| --- | --- | --- | --- |
| Stage worker | `sdd-stage-worker` | `sdd_stage_worker` | One `issue-handoff` stage on its own sub-branch, up to commit; then fixes, push, PR |
| Self reviewer | `self-reviewer` | `self_reviewer` | Fresh-context review of the committed stage diff |
| Review fixer | `pr-review-fixer` | `pr_review_fixer` | One round of CI failures and review findings on one PR |

Workers cannot start workers on every host, so the orchestrator starts the
self reviewer itself between the stage worker's two phases. The stage worker
and the review fixer push and call GitHub themselves. When the host has no
workers, or its workers have no network access, this skill does not apply: run
the stages one at a time with `issue-handoff`.

## Which model runs what

On Claude, pass the model on each worker call; the agent definitions leave it
unset so that one-stage runs keep inheriting the session's model. Fable costs
about 2.5 times Opus per token, so it goes only where a better answer changes
everything downstream or a miss is expensive to find later.

| Work | Model | Why |
| --- | --- | --- |
| `plan` and `design` stage workers | `fable` | One run per feature, and every child Issue, implementation and review is built on its decisions |
| Self reviewer | `fable` | Its value is finding what the author missed; each finding it reaches here saves a review round on the PR |
| Review fixer on a PR with three distinct head SHAs reviewed by a bot | `fable` | Findings that keep coming back need the root cause, not another local patch |
| Integrate worker when merging `main` conflicts | `fable` | Keeping both sides' behaviour is a judgement across two changes |
| Implementation, `plan-to-issues`, other review-fixer rounds, conflict-free integrate | inherit | Bounded by an approved artifact or a child Issue; high volume |

The orchestrator itself stays on the session's model. It only reads short
facts and return blocks, so a more capable model buys it nothing. Codex keeps
the models in `.codex/agents/`.

## Procedure

Follow [references/loop.md](references/loop.md). In short:

1. Preflight: Issue read/write, native sub-issues, push, PR create and merge.
   Stop before any mutation when one is missing.
2. Loop: select the next stage with the one-stage workflow's rules, run it
   through workers, apply the merge gates, close merged children. Repeat.
3. Stop on a blocker, or when the integration PR meets the finish line.

Report to the maintainer in one short line per merged PR, and at the end with
the integration PR link and anything a worker deferred.

## Stop and ask instead of deciding

Autopilot does not widen what a single stage may decide. Stop the loop and
report to the maintainer when:

- a worker returns `BLOCKED` for a question that belongs to the requester
  (Q-6, Q-7 in
  [docs/product-specs/spec-quality.md](../../../docs/product-specs/spec-quality.md))
  or to an approved artifact
- a review finding can only be fixed by changing an approved artifact
- a PR does not converge (see the round limit in
  [references/loop.md](references/loop.md) §4)
- a required GitHub capability is missing, or the stage selection would stop
  and ask

Revising an approved artifact is outside this skill. The stage selection
never re-runs a stage whose artifact exists, so it does not notice that, for
example, `ui-design.md` predates a revised Plan. The maintainer names and
reruns the affected stages with `issue-handoff` first, and starts
this skill again afterwards.

Leave everything already merged in place. A later run of this skill on the same
parent picks up from GitHub where this one stopped.
