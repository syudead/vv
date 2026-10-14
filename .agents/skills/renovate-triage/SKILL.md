---
name: renovate-triage
description: Take every open Renovate PR to merged or needs-human within one run (failed checks, conflicts, major updates included) and watch what this routine merged. Use when the daily Renovate routine fires, or when the maintainer asks to process the open Renovate PRs now.
---

# Renovate triage

Renovate merges minor, patch, pin, digest and lockfile updates itself once the
required checks pass ([dependency-updates.md](../../../docs/how-to/dependency-updates.md)),
but only on its own schedule and only while it alone has committed to the
branch. This skill takes every open Renovate PR the rest of the way without a
person: green ones, ones with a failed check or a conflict, and major updates.
The maintainer has delegated the merge of these PRs into `main`, major updates
included, under the gates below. Nothing else is delegated: this skill merges
only Renovate PRs, and never reverts a merge.

The skill is an orchestrator. It derives the state from GitHub, hands each PR
that needs work to one `renovate-pr-fixer` worker (`renovate_pr_fixer` on
Codex), waits for CI, merges what passes the gates, and reports. Every open
Renovate PR ends the run merged or labelled `needs-human`; nothing is left for
the next run. It does not read diffs or job logs itself; that is the worker's
job.

## Labels and markers

GitHub is the state. Do not write a state file or a tracking Issue.

| Name | On | Meaning |
| --- | --- | --- |
| `needs-human` label | Renovate PR | The routine gave up on it. Skip it until a person removes the label. On a merged PR: the update broke `main` |
| `<!-- renovate-triage -->` | Comment | Written by this routine. The first line of every comment it posts |
| `<!-- renovate-triage:attempt -->` | Comment | Posted by a worker with each push to a Renovate branch; the record of what each attempt changed |
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
still fixes PRs and waits for their CI, but merges none: a PR that passes the
gates stays open, and the report says merges are paused. This is the one case
where a run leaves PRs open. Merges resume once a person removes the label.

### List the open Renovate PRs

List the open PRs by `app/renovate`. Request only number, title, labels, head
branch, head commit, mergeable state, the check runs of the head commit, and
which `renovate-triage` markers its comments carry.
Order them: `security` label first, then the rest by PR number. Leave out the
PRs labelled `needs-human`.

### Drive each PR to an end state

A run does not leave work for the next run. Take the listed PRs one at a time,
in order, and keep working on each until it reaches one of two end states:

- **Merged** by this routine, through the [merge gates](#merge-gates).
- **`needs-human`**, with a `renovate-triage` comment saying why.

One at a time, because each merge moves `main` and concurrent workers on
lockfiles conflict with each other. For the current PR, repeat:

1. **Checks queued or running on the head commit**: wait for them
   ([Waiting for CI](#waiting-for-ci)). Never skip a PR because its CI is
   still running.
2. **Every check green and no conflict**: if `main` has moved since those
   checks started (this run merged another PR, or anyone pushed to `main`),
   the checks tested a stale merge. Update the branch with `main` (GitHub's
   update-branch, merge method; it counts as no attempt) and go back to step 1.
   Otherwise apply the merge gates. A major update without the worker's review
   comment goes to step 3 first.
3. **Anything else** (a failed check, a conflict, a major update not yet
   reviewed): hand it to one `renovate-pr-fixer` with this brief, then act on
   its `STATUS`:

   ```text
   Renovate PR: syudead/vv#<number>
   Head branch: <branch>
   Reason: <failed checks: names | conflict | major update>
   Attempt: <n> of 3
   Follow .agents/skills/renovate-triage/references/worker.md.
   ```

   | STATUS | Next |
   | --- | --- |
   | `PUSHED` | Back to step 1: wait for CI on the new commit, then judge it |
   | `READY` | Apply the merge gates |
   | `BLOCKED` | Label `needs-human`. The worker has already commented why |

A PR whose checks still fail after the third worker push in this run gets
`needs-human` with a comment listing the three attempts. Earlier runs'
attempts do not count toward the three.

### Waiting for CI

The checks of a Renovate PR, Browser E2E included, take about 15 minutes.
Wait inside the run: subscribe to the PR's activity and end the turn until a
check-suite event for the head commit arrives, or, on a host without such
events, check the head commit's check runs every 5 minutes from a background
wait (never a foreground `sleep`). Count 60 minutes from when the run started
waiting on that head commit, whether its checks are queued or running. When
they have not all finished by then, stop waiting on that PR, label it
`needs-human` with the unfinished checks' names, and move on.

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

At the end of the run, when it did anything, add one comment to the Renovate "Dependency
Dashboard" Issue: one line per PR merged, labelled `needs-human` or
found to have broken `main`, each with the PR link and, for a fixed PR, what
the fix changed. When the run did nothing, post nothing.

## Never

- Merge a PR that is not by `app/renovate`, or revert a merged one.
- Skip, disable, loosen or quarantine a test or a check, or re-run a failed
  check to get it green.
- Change `renovate.json` to stop an update. That is the maintainer's call;
  label `needs-human` instead.
- Change the excluded dependencies in
  [dependency-updates.md](../../../docs/how-to/dependency-updates.md#excluded-dependencies).
- Push to `main` directly or rewrite a Renovate branch's history.
