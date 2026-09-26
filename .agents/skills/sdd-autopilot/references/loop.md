# Autopilot loop

Read [../SKILL.md](../SKILL.md) first. Each iteration: select the next stage
(§1, §2), run it (§3–§6), and start the next iteration from §1 again.

Everything the loop needs to decide is re-derived from GitHub and the
repository. Anything you remember from earlier in the session is only a
shortcut: losing it may repeat an idempotent step (a worker that finds its work
already done), never pick a wrong one.

## 1. Derive the state

The stage selection is the one-stage workflow's:
[Selecting the stage](../../issue-handoff/references/README.md#selecting-the-stage).
Apply it to the parent Issue on every iteration, with two constraints that keep
the orchestrator's reads small:

- Collect only the facts the rules need, each as a field-limited read: the
  base of a merged PR that `Refs #<parent>`, and the integration PR (the
  parent's `closed_by_pull_requests`) once `integrate` has opened it;
  `specs/*/plan.md` and `ui-design.md` on `origin/<feature>` (`git fetch`, then `git diff --name-only` and
  `git cat-file -e`); the parent's labels; its native sub-issues (number,
  state, `state_reason`); and the open PRs into the feature branch (number,
  head ref, head SHA).
- Do not read the parent Issue body, PR bodies, or child Issue bodies. What a
  rule needs from a body — whether an open PR belongs to this feature, a
  child's prerequisites, whether a merged PR already `Refs` a child — is
  answered by the worker that handles it (§3, §4).

## 2. What autopilot changes in the selection

| Rule in the selection | Autopilot does instead |
| --- | --- |
| 1, and 5 when every remaining child has an open PR: an open PR waits on human merge | Drive the open PRs into the feature branch to merge (§4), stage PRs first and then in sub-issue order, skipping one already returned `FOREIGN` in this session |
| 2 `plan`, 3 `design`, 4 `plan-to-issues`, 5 `implement` | Run that stage through workers (§3). For `implement`, take the first child in sub-issue order that is not closed and not already returned `BLOCKED` for a prerequisite in this session; if every such child is blocked, stop |
| 6 `integrate` | Integration refresh, which opens the integration PR, and the finish line (§6) |
| "stop and ask" (ambiguous feature, not a specification) | Stop and report |

If a stage stops before its PR exists, selection picks the stage again. Anything
the selection does not cover is a stop: report the facts from §1 and what you
expected.

## 3. Run a stage

Stages `plan`, `design` and implementation each produce one PR to the feature
branch. `plan-to-issues` produces no PR.

1. Start a fresh stage worker with the brief for that stage from
   [briefs.md](briefs.md). It creates its own sub-branch, does the stage's
   work and checks, commits, pushes, opens the PR, and returns `DONE` with
   the PR number.
   - `plan-to-issues` returns `DONE`. Go to §1.
   - An implementation worker that finds a merged PR into the feature branch
     already referencing its child returns `DONE` with that PR and no branch.
     Close the child (§5) and go to §1.
2. Go to §4 with the new PR.

## 4. Drive a feature PR to merge

Handle each PR with one review-fix pass:

1. On the head first handled for this PR, wait for every check to complete and
   for a review by someone other than the PR author (a GitHub review whose
   `commit_id` is that head SHA), or for 20 minutes to pass without a review.
   A check still pending an hour after the head was pushed is a stop.
2. Start a fresh review fixer with the feature-PR brief. It handles every
   failing check, every unresolved review thread, and a conflict with the
   base, and either pushes (`FIXED`, new head) or changes nothing (`CLEAN`).
   A fixer never returns `CLEAN` while a check on that head is not passing.
   It runs the checks its change needs before pushing a fix.
   It first checks that the PR belongs to this feature: its `Refs` names the
   parent or one of the parent's native children (for the integration PR, it
   is the feature branch's PR to `main` that `Closes` the parent). Otherwise
   it changes nothing and returns `FOREIGN`. It also returns the PR's `KIND`, which §5 uses.
3. For `FIXED` or `CLEAN`, confirm only that GitHub reports the current PR
   conflict-free and mergeable, then merge with a merge commit and go to §5.
   After `FIXED`, do not wait for checks or reviews on the new head or run
   another fixer. If GitHub branch protection prevents the merge, stop and
   report it; do not bypass the protection. `BLOCKED`: stop. `FOREIGN`: leave
   the PR alone, never merge it, and name it in the final report; go to §1.

After a restart, if GitHub state does not establish whether the one fixer pass
already happened, stop and report that ambiguity instead of repeating it.

**Limits.** Stop when three integration-fix PRs have merged since the
integration PR was opened, when its head has been refreshed from `main` twice
(§6 step 2), or when a fixer returns `BLOCKED` because a finding repeats one
it can see was already fixed and resolved on the same PR. All of them are read
from GitHub, not remembered: the integration-fix count is the number of PRs
merged into the feature branch that `Refs #<parent>` after the integration
PR's `created_at`, which a PR search with a `merged:>` date returns as a total
without bodies, less those that change `plan.md` or `ui-design.md` (a Plan or
Design revision the maintainer ran, read from the PR's file list); the refresh count is the number of merge commits from `main`
on the feature branch after that time. Neither count is reset by a refresh.
Hitting a limit means the fixes are not converging. Report the integration PR
as it stands and stop; the maintainer decides what is left.

## 5. Bookkeeping after a merge

The parent Issue body is never edited: progress is read from GitHub and the
feature branch, not recorded
([README](../../issue-handoff/references/README.md#inputs-and-sources-of-truth)).
The only follow-up is closing a child: when the merged PR's `KIND` is
`implement`, close the child named by its `REFS` as `completed`. Those lines
come from the fixer that handled the PR. A child whose close was missed still
counts as done for the selection, because a merged PR `Refs` it; the
implementation worker reports it
as `DONE` if it is picked again, and you close it then.

## 6. Integration refresh and the finish line

1. Start a fresh stage worker with the integrate brief (on `fable` when
   `git merge-tree --write-tree origin/<feature> origin/main` reports a
   conflict). It runs
   [integrate.md](../../issue-handoff/references/integrate.md): merges the
   latest `origin/main` into the feature branch directly, runs the checks,
   pushes, opens the integration PR on the first refresh, and writes its
   body — also listing the out-of-scope items the feature PRs deferred.
2. Before the integration PR's review pass: if it conflicts with `main`, or a
   required check on its head fails because of a change on `main`, go back to
   step 1. `main` having moved on without either is not a reason: every
   refresh moves the head and gets the whole feature reviewed again. Conflicts
   with `main` are never a review fixer's job here.
3. Drive the integration PR like §4, with two differences. The review fixer
   uses the integration brief, and only blocking findings
   ([integrate.md](../../issue-handoff/references/integrate.md#review-of-the-integration-pr))
   are fixed: it answers and resolves the rest, and lists the real defects
   among them in the integration PR body. Its fixes go to a new sub-branch and
   a PR to the feature branch, never directly onto the feature branch. It
   returns that PR as `FIXED`; drive that PR with §4 until merged, then go
   to step 4. Do not repeat checks, review, or fixer work on the updated
   integration PR head.
4. The **finish line**: after the initial review pass and any fix PR merge,
   confirm only that GitHub reports the integration PR conflict-free and
   mergeable. If branch protection blocks it, stop and report the blocker
   without rerunning checks or review. Do not merge it. Report the integration
   PR link to the maintainer, and stop.

## 7. Waiting

Waiting runs no worker. Subscribe to the PR's activity and schedule a check-in
roughly 15 minutes out when the host offers both, and end the turn; otherwise
poll at an interval of a few minutes. On every wake, start again from §1 — an
event tells you something changed, not what to do.
