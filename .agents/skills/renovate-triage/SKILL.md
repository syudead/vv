---
name: renovate-triage
description: Clear the Renovate PRs that Renovate does not merge on its own (failed checks, conflicts, major updates) and watch what this routine merged. Use when the daily Renovate routine fires, or when the maintainer asks to process the open Renovate PRs now.
---

# Renovate triage

Renovate merges minor, patch, pin, digest and lockfile updates itself once the
required checks pass ([dependency-updates.md](../../../docs/how-to/dependency-updates.md)).
This skill handles the rest without a person: a Renovate PR with a failed
check, a conflict, or a major update. The maintainer has delegated the merge of
these PRs into `main`, major updates included, under the gates below. Nothing
else is delegated: this skill merges only Renovate PRs, and never reverts a
merge.

The skill is an orchestrator. It derives the state from GitHub, hands each PR
that needs work to one `renovate-pr-fixer` worker (`renovate_pr_fixer` on
Codex), merges what passes the gates, and reports. It does not read diffs or
job logs itself; that is the worker's job.

## Labels and markers

GitHub is the state. Do not write a state file or a tracking Issue.

| Name | On | Meaning |
| --- | --- | --- |
| `needs-human` label | Renovate PR | The routine gave up on it. Skip it until a person removes the label. On a merged PR: the update broke `main` |
| `<!-- renovate-triage -->` | Comment | Written by this routine. The first line of every comment it posts |
| `<!-- renovate-triage:attempt -->` | Comment | Posted by a worker with each push to a Renovate branch |
| `<!-- renovate-triage:merged -->` | Comment | Posted on a PR right before the routine merges it; marks the PRs the post-merge watch checks |
| `<!-- renovate-triage:broke-main -->` | Comment | Posted on a merged PR whose update broke `main` |

Create `needs-human` when it does not exist.

## Procedure

### Preflight

Check that the session can read and write PRs, Issues and labels, push to
`renovate/*` branches, and merge into `main`. Stop and report when one is
missing; change nothing.

### Watch what earlier runs merged

List the Renovate PRs (author `app/renovate`) merged in the last 7 days that
carry a `renovate-triage:merged` comment. For each, read the CI workflow run of
the push to `main` at its merge commit:

- **Green, or still running**: nothing to do.
- **`Browser E2E` or `Docker image` failed**, and the same job passed on the
  first parent of the merge commit: the update broke `main`. Unless the PR
  already has a `renovate-triage:broke-main` comment, post one naming the
  failed job and linking its run, label the PR `needs-human`, and report it.
  Do not revert the merge and do not try to fix `main`; a person decides.
- **Failed on the first parent too**: `main` was already red; not this
  update's failure. Nothing to do.

While any merged PR from the last 7 days has both `needs-human` and a
`renovate-triage:broke-main` comment, `main` is treated as broken: this run
still lists, fixes and reports PRs, but merges none. Merges resume once a
person removes the label.

### List the open Renovate PRs

List the open PRs by `app/renovate`. Request only number, title, labels, head
branch, head commit, mergeable state, the check runs of the head commit, and
which `renovate-triage` markers its comments carry.
Order them: `security` label first, then the rest by PR number.

Skip a PR that:

- has the `needs-human` label
- has a check still queued or running on its head commit (the next run sees it)
- is not a major update, has all required checks green, no conflict, and no
  `renovate-triage:attempt` comment (Renovate merges it itself)

Send a PR that is not a major update, has a `renovate-triage:attempt`
comment, and has all required checks green and no conflict straight to the
[merge gates](#merge-gates). Renovate does not merge a branch someone else has
committed to.

Label `needs-human`, with one comment saying why, a PR that already has three
`renovate-triage:attempt` comments and still fails a check: three fixes did
not get it green.

### Hand each remaining PR to a worker

Run one `renovate-pr-fixer` per PR, one at a time. Concurrent workers on
lockfiles conflict with each other. Use this brief:

```text
Renovate PR: syudead/vv#<number>
Head branch: <branch>
Reason: <failed checks: names | conflict | major update>
Follow .agents/skills/renovate-triage/references/worker.md.
```

Act on the worker's `STATUS` line
([references/worker.md, Return](references/worker.md#return)):

| STATUS | Next |
| --- | --- |
| `PUSHED` | CI runs on the new commit. Leave the PR to the next run, which applies the gates |
| `READY` | Apply the merge gates now |
| `BLOCKED` | Label `needs-human`. The worker has already commented why |

### Merge gates

Merge a Renovate PR only when `main` is not treated as broken
([Watch what earlier runs merged](#watch-what-earlier-runs-merged)) and all of
these hold on its current head commit:

- GitHub reports it mergeable, with no conflict.
- Every check run on the head commit has concluded `success`, `skipped` or
  `neutral`. `Checks`, `Browser E2E (*)` and `Docker image` run on Renovate PRs
  (see the comments in `.github/workflows/ci.yml`), and none of them may be
  `skipped` when the PR changes code.
- For a major update, the PR has the worker's `renovate-triage` comment
  ([references/worker.md, Major updates](references/worker.md#major-updates))
  and its verdict is `safe to merge`.
- It has no unresolved review thread. The routine does not request reviews,
  but answer and resolve one a person left, or label `needs-human` when the
  answer needs a decision.

Then post `<!-- renovate-triage:merged -->` with one line on what was merged,
and merge with a merge commit, as the repository does for other PRs.

### Report

When the run did anything, add one comment to the Renovate "Dependency
Dashboard" Issue: one line per PR merged, fixed, labelled `needs-human` or
found to have broken `main`, each with the PR link. When the run did nothing, post nothing.

## Never

- Merge a PR that is not by `app/renovate`, or revert a merged one.
- Skip, disable, loosen or quarantine a test or a check, or re-run a failed
  check to get it green.
- Change `renovate.json` to stop an update. That is the maintainer's call;
  label `needs-human` instead.
- Change the excluded dependencies in
  [dependency-updates.md](../../../docs/how-to/dependency-updates.md#excluded-dependencies).
- Push to `main` directly or rewrite a Renovate branch's history.
