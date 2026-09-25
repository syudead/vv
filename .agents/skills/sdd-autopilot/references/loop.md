# Autopilot loop

Read [../SKILL.md](../SKILL.md) first. Each iteration: derive the state (§1),
take the first matching row of the decision table (§2), run it (§3–§6), and
start the next iteration from §1 again.

Everything the loop needs to decide is re-derived from GitHub and the
repository. Anything you remember from earlier in the session is only a
shortcut: losing it may repeat an idempotent step (a worker that finds its work
already done), never pick a wrong one.

## 1. Derive the state

Collect only these facts. Each is a small, field-limited read. This traversal
is the autopilot's own; the one-stage workflow does not do it.

| Fact | Where it comes from |
| --- | --- |
| Feature branch | The supplied branch; otherwise the head of the open PR to `main` that closes the parent (`closed_by_pull_requests` on the parent); otherwise the base of an open or merged PR that `Refs #<parent>` and does not target `main` |
| Integration PR | The open PR from the feature branch to `main` |
| Plan merged | `git diff --name-only origin/main...origin/<feature> -- 'specs/*/plan.md'` after `git fetch` names one file; its directory is the feature directory |
| `ui` label | Labels on the parent |
| Design merged | `<feature-dir>/ui-design.md` exists on `origin/<feature>` (`git cat-file -e`) |
| Children | The parent's native sub-issues: number, state, `state_reason` only |
| Open feature PRs | Open PRs whose base is the feature branch: number, head ref, head SHA |

Do not read the parent Issue body, PR bodies, or child Issue bodies to derive
the state. The `## SDD` summary is for humans and may lag; §5 keeps it current
and the finish line (§6) rewrites it once more.

## 2. Decision table

| # | Condition | Action |
| --- | --- | --- |
| 1 | An open feature PR exists | Drive the lowest-numbered one to merge (§4) |
| 2 | No feature branch | Stage `plan` (§3) |
| 3 | Feature branch, Plan not merged, no open feature PR | Stop: an orphaned feature branch ([plan.md](../../issue-handoff/references/plan.md)) is the maintainer's to remove |
| 4 | Plan merged, no integration PR | Open it (§5), then continue |
| 5 | `ui` label and Design not merged | Stage `design` (§3) |
| 6 | No children, or children not yet confirmed complete in this session | Stage `plan-to-issues` (§3). It creates only the missing children, so rerunning it after a restart is safe |
| 7 | An open child | Implement the lowest-numbered open child not already returned `BLOCKED` for a dependency in this session (§3). If every open child is blocked that way, stop |
| 8 | Every child closed | Integration refresh (§6) |

A child closed as `not_planned` counts as resolved. Anything the table does not
cover is a stop: report the facts from §1 and what you expected.

## 3. Run a stage

Stages `plan`, `design` and implementation each produce one PR to the feature
branch. `plan-to-issues` produces no PR and has no self-review.

1. Start a fresh stage worker with the brief for that stage from
   [briefs.md](briefs.md). It creates its own sub-branch, does the stage's
   work and checks, commits, and returns `READY` with the branch and base.
   - `plan-to-issues` returns `DONE`; the children are now confirmed. Go to §1.
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

- every check run has completed and none failed
- the review bot has reviewed this head (a review whose `commit_id` is the head
  SHA from a bot account that reviews this repository — today
  `devin-ai-integration[bot]`), or 20 minutes have passed since the head was
  pushed with no bot review
- a review fixer has run on this head after that review and returned `CLEAN`
- GitHub reports it mergeable with no conflict

Loop:

1. Wait for the checks and the bot review on the head (§7). A check still
   pending an hour after the head was pushed is a stop.
2. Start a fresh review fixer with the feature-PR brief. It handles every
   failing check, every unresolved review thread, and a conflict with the
   base, and either pushes (`FIXED`, new head) or changes nothing (`CLEAN`).
   A fixer never returns `CLEAN` while a check on the head is failing.
3. `FIXED`: back to step 1 on the new head. `CLEAN`: merge with a merge
   commit, then §5. `BLOCKED`: stop.

After a restart you do not know whether a fixer already ran on the head; run
one. It finds nothing new and returns `CLEAN`.

**Round limit.** Stop when the review bot has reviewed the PR six times, or
when a fixer returns `BLOCKED` because a finding repeats one it can see was
already fixed and resolved on the same PR. Both are read from the PR, not
remembered. Either means the fixes are not converging, and another round spends
context without changing that.

Never merge with a failing or pending check, and never skip, disable, or
re-run a test to get past one.

## 5. Bookkeeping after a merge

Do these right after the merge that makes them true. Read the parent body only
to rewrite its `## SDD` section, and do not repeat what it says.

| Merged | Do |
| --- | --- |
| Plan PR | Mark `Plan` done with the plan path, set `Next` to `design` for a `ui` parent or `plan-to-issues` otherwise, and open the integration PR (feature branch → `main`, `Closes #<parent>`, the parent's title, the repository PR template) |
| Design PR | Mark `Design` done, set `Next: plan-to-issues` |
| `plan-to-issues` finished | The worker removes `Next` itself; nothing to do |
| Implementation PR | Close the child named by the worker's or fixer's `REFS` line as `completed` |
| Integration-fix PR | Nothing; §6 continues |

If a restart lost track of which child a merged PR belonged to, the child is
still open and row 7 picks it again; its worker returns `DONE` for the merged
PR (§3 step 1), and you close it then.

## 6. Integration refresh and the finish line

1. Start a fresh stage worker with the integrate brief. It merges the latest
   `origin/main` into the feature branch directly (no PR, no rebase), resolves
   conflicts, runs `task check` and the UI checks the feature's `ui-design.md`
   names, pushes the feature branch, and rewrites the integration PR's body to
   describe the feature as it now stands, including the out-of-scope items the
   feature PRs deferred.
2. Before each round on the integration PR: if it conflicts with `main`, or
   `git rev-list --count origin/<feature>..origin/main` is not `0`, go back to
   step 1. Conflicts with `main` are never a review fixer's job here.
3. Drive the integration PR like §4, with one difference: the review fixer uses
   the integration brief, and its fixes go to a new sub-branch and a PR to the
   feature branch, never directly onto the feature branch. It returns that PR
   as `FIXED`; drive that PR with §4 until merged, then return to step 2.
4. The **finish line**: the integration PR meets every §4 gate on its head, and
   step 2 finds nothing to do. Do not merge it. Rewrite the parent's `## SDD`
   summary to its final state (every stage done, no `Next`), report the
   integration PR link to the maintainer, and stop.

## 7. Waiting

Waiting runs no worker. Subscribe to the PR's activity and schedule a check-in
roughly 15 minutes out when the host offers both, and end the turn; otherwise
poll at an interval of a few minutes. On every wake, start again from §1 — an
event tells you something changed, not what to do.
