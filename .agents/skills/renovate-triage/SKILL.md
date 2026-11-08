---
name: renovate-triage
description: Take every open Renovate PR to merged or deferred within one run (failed checks, conflicts, major updates included), reopen deferred updates once their blocker is gone, and watch what this routine merged. Use when the daily Renovate routine fires, or when the maintainer asks to process the open Renovate PRs now.
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
Renovate PR ends the run merged or [deferred](#deferring-a-pr): closed with
the reason and the condition for trying again, so no PR sits open with
nothing left to try. It does not read diffs or job logs itself; that is the worker's job.

## Labels and markers

GitHub is the state. Do not write a state file or a tracking Issue.

| Name | On | Meaning |
| --- | --- | --- |
| `needs-human` label | Merged Renovate PR | The update broke `main`; merges are paused until `main` is green again |
| `<!-- renovate-triage -->` | Comment | Written by this routine. The first line of every comment it posts |
| `<!-- renovate-triage:attempt -->` | Comment | Posted by a worker with each push to a Renovate branch; the record of what each attempt changed |
| `<!-- renovate-triage:merged -->` | Comment | Posted on a PR right before the routine merges it; marks the PRs the post-merge watch checks |
| `<!-- renovate-triage:broke-main -->` | Comment | Posted on a merged PR whose update broke `main` |
| `<!-- renovate-triage:deferred -->` | Comment | Posted on a PR right before the routine closes it; carries the reason and the `Revisit when:` condition |
| `<!-- renovate-triage:reopened -->` | Comment | Posted on a deferred PR when the routine asked Renovate to recreate it |

Create `needs-human` when it does not exist. The routine never puts it on an
open PR. An open Renovate PR that still carries it from an earlier version of
this skill is processed like any other: remove the label first.

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

For a merged PR that has both `needs-human` and a `renovate-triage:broke-main`
comment, read the latest completed CI run of a push to `main`. When the job
named in that comment passed there, `main` was fixed: remove the label and
report it. A person may also remove it.

While any merged PR still has both, `main` is treated as broken: this run
still fixes PRs and waits for their CI, but merges none: a PR that passes the
gates stays open, and the report says merges are paused.

### Reopen deferred updates whose blocker is gone

List the Renovate PRs closed in the last 180 days whose latest
`renovate-triage` marker is `deferred`. For each one whose `Revisit when:`
condition is a checkable fact (an upstream issue or PR is closed, or a
package version that satisfies a stated requirement is published), check it.
A `Revisit when:` of `the next version` or `a person decides` needs no check:
Renovate opens the next version by itself, and a person ticks the checkbox
on the Dependency Dashboard.

When the condition holds, tick that PR's checkbox under "Ignored or Blocked"
in the Dependency Dashboard Issue body (the line with
`<!-- recreate-branch=<head branch> -->`), and post
`<!-- renovate-triage:reopened -->` on the closed PR naming what changed.
Renovate recreates the PR on its next run, and a later run of this routine
takes it like any other. When the PR is no longer listed there (a newer PR
for the same update exists), do nothing.

### List the open Renovate PRs

List the open PRs by `app/renovate`. Request only number, title, labels, head
branch, head commit, mergeable state, the check runs of the head commit, and
which `renovate-triage` markers its comments carry.
Order them: `security` label first, then the rest by PR number.

### Drive each PR to an end state

A run does not leave work for the next run. Take the listed PRs one at a time,
in order, and keep working on each until it reaches one of two end states:

- **Merged** by this routine, through the [merge gates](#merge-gates).
- **Deferred**: closed by this routine ([Deferring a PR](#deferring-a-pr)).

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
   | `DEFERRED` | [Defer it](#deferring-a-pr) with the worker's `REVISIT` |

A PR whose checks still fail after the third worker push in this run is
deferred with `Revisit when: the next version` and a comment listing the three
attempts. Earlier runs' attempts do not count toward the three.

### Deferring a PR

Defer a PR the routine cannot merge in this run and whose blocker a later run
cannot remove without something outside the repository changing: an upstream
bug or missing support, a rewrite the update demands, or a choice that is the
maintainer's (such as a `renovate.json` change). Post one comment, then close
the PR:

```markdown
<!-- renovate-triage:deferred -->
<one or two lines: why it cannot be merged, linking the worker's comment>

Revisit when: <an upstream issue or PR that is closed | a package version that is published | the next version | a person decides>
```

Closing is reversible and keeps the update visible: Renovate lists a closed
PR under "Ignored or Blocked" on the Dependency Dashboard and recreates it
when its checkbox is ticked, and it opens the next version on its own.
[Reopen deferred updates](#reopen-deferred-updates-whose-blocker-is-gone)
ticks the checkbox once a checkable condition holds. Use `a person decides`
only when no upstream change would unblock the update.

### Waiting for CI

The checks of a Renovate PR, Browser E2E included, take about 15 minutes.
Wait inside the run: subscribe to the PR's activity and end the turn until a
check-suite event for the head commit arrives, or, on a host without such
events, check the head commit's check runs every 5 minutes from a background
wait (never a foreground `sleep`). Count 60 minutes from when the run started
waiting on that head commit, whether its checks are queued or running. When
they have not all finished by then, stop waiting on that PR, leave it open and
unlabelled, name the unfinished checks in the report, and move on; the next
run takes it again. Paused merges and a review thread awaiting its author are
the only other cases where a run leaves a PR open.

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
  but answer and resolve one a person left. When the answer needs that
  person's decision, leave the PR open, reply in the thread, and list it in
  the report.

Then post `<!-- renovate-triage:merged -->` with one line on what was merged,
and merge with a merge commit, as the repository does for other PRs.

### Report

At the end of the run, when it did anything, add one comment to the Renovate
"Dependency Dashboard" Issue: one line per PR merged, deferred (with its
`Revisit when:`), reopened, left open, found to have broken `main` or cleared
of `needs-human`, each with the PR link and, for a fixed PR, what the fix
changed. When the run did nothing, post nothing.

## Never

- Merge a PR that is not by `app/renovate`, or revert a merged one.
- Skip, disable, loosen or quarantine a test or a check, or re-run a failed
  check to get it green.
- Change `renovate.json` to stop an update. That is the maintainer's call;
  defer the PR with `Revisit when: a person decides` instead.
- Leave a PR open labelled `needs-human`.
- Change the excluded dependencies in
  [dependency-updates.md](../../../docs/how-to/dependency-updates.md#excluded-dependencies).
- Push to `main` directly or rewrite a Renovate branch's history.
