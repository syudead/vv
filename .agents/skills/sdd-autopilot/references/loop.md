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

Phase 1 of a stage pushes nothing (§3), so a restart before its PR exists
leaves no branch behind and the selection simply picks the stage again. Anything
the selection does not cover is a stop: report the facts from §1 and what you
expected.

## 3. Run a stage

Stages `plan`, `design` and implementation each produce one PR to the feature
branch. `plan-to-issues` produces no PR and has no self-review.

1. Start a fresh stage worker with the brief for that stage from
   [briefs.md](briefs.md). It creates its own sub-branch, does the stage's
   work and checks, commits, and returns `READY` with the branch and base.
   - `plan-to-issues` returns `DONE`. Go to §1.
   - An implementation worker that finds a merged PR into the feature branch
     already referencing its child returns `DONE` with that PR and no branch.
     Close the child (§5) and go to §1.
2. Start a fresh self reviewer with the self-review brief, pointing at the
   worker's branch and base. It writes its findings to a file and returns
   `CLEAN` or `FINDINGS`; do not open the file.
3. Continue the **same** stage worker with the phase 2 brief naming that file.
   It fixes what is in scope, re-runs its checks, pushes, opens the PR, and
   returns `DONE` with the PR number. If a finding needs an approved artifact
   changed, it returns `BLOCKED` without pushing: stop.
4. If the worker can no longer be continued (a restarted session), start a
   fresh stage worker in continuation mode with the branch name instead.
5. Go to §4 with the new PR.

## 4. Drive a feature PR to merge

A PR is **mergeable here** when all of these hold on its current head SHA:

- every check run has completed with `success`, `skipped` or `neutral`.
  `cancelled`, `timed_out`, `action_required`, `stale` and `failure` are not
  passing, whatever caused them
- an automated code-review account has reviewed this head (a GitHub review
  whose `commit_id` is the head SHA and whose author's GitHub actor type is
  `Bot`, regardless of its login), or 20 minutes have passed since the head
  was pushed with no bot review
- a review fixer has run on this head after the bot review or 20-minute wait
  and returned `CLEAN`
- GitHub reports it mergeable with no conflict

Loop:

1. Wait for the checks and the bot review on the head (§7). A check still
   pending an hour after the head was pushed is a stop.
2. Start a fresh review fixer with the feature-PR brief (on `fable` once
   three distinct head SHAs on this PR have received a bot review, the same
   count the round limit reads; see the model table in
   [../SKILL.md](../SKILL.md#which-model-runs-what)). It handles every
   failing check, every unresolved review thread, and a conflict with the
   base, and either pushes (`FIXED`, new head) or changes nothing (`CLEAN`).
   A fixer never returns `CLEAN` while a check on the head is not passing.
   It first checks that the PR belongs to this feature: its `Refs` names the
   parent or one of the parent's native children (for the integration PR, it
   is the feature branch's PR to `main` that `Closes` the parent). Otherwise
   it changes nothing and returns `FOREIGN`. It also returns the PR's `KIND`, which §5 uses.
3. `FIXED`: back to step 1 on the new head. `CLEAN`: merge with a merge
   commit, then §5. `BLOCKED`: stop. `FOREIGN`: leave the PR alone, never
   merge it, and name it in the final report; go to §1.

After a restart you do not know whether a fixer already ran on the head; run
one. It finds nothing new and returns `CLEAN`.

**Round limit.** Stop when six distinct head SHAs on a feature PR have received
a bot review (multiple bots or reviews on one head count once),
when three integration-fix PRs have merged since the integration PR was
opened, when the integration PR's head has been refreshed from `main` twice
(§6 step 2), or when a fixer returns `BLOCKED` because a finding repeats one
it can see was already fixed and resolved on the same PR. All of them are read
from GitHub, not remembered: the integration-fix count is the number of PRs
merged into the feature branch that `Refs #<parent>` after the integration
PR's `created_at`, which a PR search with a `merged:>` date returns as a total
without bodies, less those that change `plan.md` or `ui-design.md` (a Plan or
Design revision the maintainer ran, read from the PR's file list); the refresh count is the number of merge commits from `main`
on the feature branch after that time. Neither count is reset by a refresh.
Hitting a limit means the fixes are not converging, and another round spends
context without changing that. Report the integration PR as it stands and
stop; the maintainer decides what is left.

Never merge with a check that has not passed, and never skip, disable, or
re-run a test to get past one.

## 5. Bookkeeping after a merge

The parent Issue body is never edited: progress is read from GitHub and the
feature branch, not recorded
([README](../../issue-handoff/references/README.md#inputs-and-sources-of-truth)).
The only follow-up is closing a child: when the merged PR's `KIND` is
`implement`, close the child named by its `REFS` as `completed`. Those lines
come from the fixer that returned `CLEAN` for the merged head, so a restart
loses nothing. A child whose close was missed still counts as done for the
selection, because a merged PR `Refs` it; the implementation worker reports it
as `DONE` if it is picked again, and you close it then.

## 6. Integration refresh and the finish line

1. Start a fresh stage worker with the integrate brief (on `fable` when
   `git merge-tree --write-tree origin/<feature> origin/main` reports a
   conflict). It runs
   [integrate.md](../../issue-handoff/references/integrate.md): merges the
   latest `origin/main` into the feature branch directly, runs the checks,
   pushes, opens the integration PR on the first refresh, and writes its
   body — also listing the out-of-scope items the feature PRs deferred.
2. Before each round on the integration PR: if it conflicts with `main`, or a
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
   returns that PR as `FIXED`; drive that PR with §4 until merged, then return
   to step 2.
4. The **finish line**: the integration PR meets every §4 gate on its head, and
   step 2 finds nothing to do. Do not merge it. Report the integration PR
   link to the maintainer, and stop.

## 7. Waiting

Waiting runs no worker. Subscribe to the PR's activity and schedule a check-in
roughly 15 minutes out when the host offers both, and end the turn; otherwise
poll at an interval of a few minutes. On every wake, start again from §1 — an
event tells you something changed, not what to do.
