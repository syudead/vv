# Autopilot loop

Read [../SKILL.md](../SKILL.md) first. Each iteration: select the next stage
([Derive the state](#derive-the-state), [What autopilot changes in the selection](#what-autopilot-changes-in-the-selection)), run it (from [Run a stage](#run-a-stage) to [Integration refresh and the finish line](#integration-refresh-and-the-finish-line)), and start the next iteration from [Derive the state](#derive-the-state) again.

Everything the loop needs to decide is re-derived from GitHub and the
repository. Anything you remember from earlier in the session is only a
shortcut: losing it may repeat an idempotent step (a worker that finds its work
already done), never pick a wrong one.

## Derive the state

The stage selection is the one-stage workflow's:
[Selecting the stage](../../issue-handoff/references/README.md#selecting-the-stage).
Apply it to the parent Issue on every iteration, with two constraints that keep
the orchestrator's reads small:

- Collect only the facts the rules need, each as a field-limited read: the
  base of a merged PR that `Refs #<parent>`, and the integration PR (the
  parent's `closed_by_pull_requests`) once `integrate` has opened it, and
  whether it has a comment containing `<!-- sdd-sweep:` (keep the yes or
  no, not the comment);
  `specs/*/plan.md` and `ui-design.md` on `origin/<feature>` (`git fetch`, then `git diff --name-only` and
  `git cat-file -e`); the parent's labels; its native sub-issues (number,
  state, `state_reason`, and the `Depends on:` first line of each open
  child's body); and the open PRs into the feature branch (number, head ref,
  head SHA).
- Do not read the parent Issue body, PR bodies, or the rest of child Issue
  bodies. What a rule needs from a body — whether an open PR belongs to this
  feature, whether a merged PR already `Refs` a child, the prerequisites of a
  child without a `Depends on:` line — is answered by the worker that handles
  it ([Run a stage](#run-a-stage), [Drive a feature PR to merge](#drive-a-feature-pr-to-merge)).

## What autopilot changes in the selection

| Rule in the selection | Autopilot does instead |
| --- | --- |
| 1, and 5 when every remaining child has an open PR: an open PR waits on human merge | Drive the open PRs into the feature branch to merge ([Drive a feature PR to merge](#drive-a-feature-pr-to-merge)), stage PRs first and then in sub-issue order, skipping one already returned `FOREIGN` in this session. Open implementation PRs do not stop new children from starting (next row) |
| 2 `plan`, 3 `design`, 4 `plan-to-issues` | Run that stage through workers ([Run a stage](#run-a-stage)) |
| 5 `implement` | Implement ready children in parallel, keeping at most three implementation PRs open. A child is ready when it is not closed, has no open PR, was not already returned `BLOCKED` for a prerequisite in this session, and every child its `Depends on:` line names is **done** in the selection's sense: closed as `completed`, or `Refs`'d by a PR merged into the feature branch (a PR search for `"Refs #<n>"`, `is:merged` and the feature branch as base, read as a count). A child without that line is ready only when no other implementation PR is open (it runs alone, as before). Take the first `3 − <open implementation PRs>` ready children in sub-issue order, start one stage worker for each at once ([Run a stage](#run-a-stage)), and drive each resulting PR with [Drive a feature PR to merge](#drive-a-feature-pr-to-merge) on its own; when one merges, go to [Derive the state](#derive-the-state), which may make more children ready. If no child is ready and no implementation PR is open, stop |
| 6 `integrate` | Integration refresh, which opens the integration PR, its review pass, the pre-merge sweep, and the finish line ([Integration refresh and the finish line](#integration-refresh-and-the-finish-line)) |
| "stop and ask" (ambiguous feature, not a specification) | Stop and report |

If a stage stops before its PR exists, selection picks the stage again. Anything
the selection does not cover is a stop: report the facts from [Derive the state](#derive-the-state) and what you
expected.

## Run a stage

Stages `plan`, `design` and implementation each produce one PR to the feature
branch. `plan-to-issues` produces no PR.

1. Check the free disk space ([Worktrees and disk space](#worktrees-and-disk-space)),
   then start a fresh stage worker with the brief for that stage from
   [briefs.md](briefs.md). Concurrent implementation workers never share a
   checkout. On Claude, start each with `isolation: "worktree"`. Where the
   host has no such isolation (Codex), create one first with
   `git worktree add <path> origin/<feature>` per child and put its path in
   the brief's `Checkout:` slot. Either way, remove the worktree as
   [Worktrees and disk space](#worktrees-and-disk-space) says. Without
   either, run implementation workers one at a time. Each worker creates its
   own sub-branch, does the stage's work and checks, commits, pushes, opens
   the PR, and returns `DONE` with the PR number.
   - `plan-to-issues` returns `DONE`. Go to [Derive the state](#derive-the-state).
   - An implementation worker that finds a merged PR into the feature branch
     already referencing its child returns `DONE` with that PR and no branch.
     Close the child ([Bookkeeping after a merge](#bookkeeping-after-a-merge)) and go to [Derive the state](#derive-the-state).
2. Go to [Drive a feature PR to merge](#drive-a-feature-pr-to-merge) with the new PR.

## Drive a feature PR to merge

Handle each PR with one review-fix pass:

1. Request the automated review once per PR: only when the PR has no review
   whose body contains `<!-- pr-review:` and carries none of `ai-review`,
   `ai-reviewing` and `ai-reviewed`, add the label `ai-review`. Every wake
   re-enters this step, so the condition is what keeps the PR from being
   requested twice. A routine outside the repository answers the request: it
   swaps the label for `ai-reviewing`, posts one review of the head whose body
   ends with `<!-- pr-review:<head SHA> -->`, and swaps the label for
   `ai-reviewed`. Never add `ai-reviewing` or `ai-reviewed` yourself.
2. On the head first handled for this PR, wait for every check to complete and
   for that marked review. A review by anyone else does not end the wait. Stop
   and report when the PR carries `ai-reviewed` without a marked review (the
   routine could not review it), when 45 minutes pass after the request
   without that review, or when a check is still pending an hour after the
   head was pushed.
3. Start a fresh review fixer with the feature-PR brief. It handles every
   failing check, every unresolved review thread, and a conflict with the
   base, and either pushes (`FIXED`, new head) or changes nothing (`CLEAN`).
   A fixer never returns `CLEAN` while a check on that head is not passing.
   It runs the checks its change needs before pushing a fix.
   It first checks that the PR belongs to this feature: its `Refs` names the
   parent or one of the parent's native children (for the integration PR, it
   is the feature branch's PR to `main` that `Closes` the parent). Otherwise
   it changes nothing and returns `FOREIGN`. It also returns the PR's `KIND`, which [Bookkeeping after a merge](#bookkeeping-after-a-merge) uses.
4. For `FIXED` or `CLEAN`, confirm only that GitHub reports the current PR
   conflict-free and mergeable, then merge with a merge commit and go to [Bookkeeping after a merge](#bookkeeping-after-a-merge).
   After `FIXED`, do not request another review, wait for checks or reviews on
   the new head, or run another fixer. The one exception: when a parallel
   PR merged into the same base after this PR's checks ran, the combined tree
   was never checked, with or without a textual conflict. Start one fresh
   review fixer that only merges `origin/<base>` into this PR (resolving
   generated files with `task generate`) and pushes, wait for every check on
   its new head to pass, then merge. Do not request another review for it. If
   GitHub branch protection prevents the merge, stop and report it; do not bypass the protection. `BLOCKED`:
   stop. `FOREIGN`: leave the PR alone, never merge it, and name it in the
   final report; go to [Derive the state](#derive-the-state).

After a restart, if GitHub state does not establish whether the one fixer pass
already happened, stop and report that ambiguity instead of repeating it.

**Limits.** The sweep PR ([Integration refresh and the finish line](#integration-refresh-and-the-finish-line) step 4) counts as an integration-fix PR. Stop
when three integration-fix PRs have merged since the
integration PR was opened, when its head has been refreshed from `main` twice
([Integration refresh and the finish line](#integration-refresh-and-the-finish-line) step 2), or when a fixer returns `BLOCKED` because a finding repeats one
it can see was already fixed and resolved on the same PR. All of them are read
from GitHub, not remembered: the integration-fix count is the number of PRs
merged into the feature branch that `Refs #<parent>` after the integration
PR's `created_at`, which a PR search with a `merged:>` date returns as a total
without bodies, less those that change `plan.md` or `ui-design.md` (a Plan or
Design revision the maintainer ran, read from the PR's file list); the refresh count is the number of merge commits from `main`
on the feature branch after that time. Neither count is reset by a refresh.
Hitting a limit means the fixes are not converging. Report the integration PR
as it stands and stop; the maintainer decides what is left.

## Bookkeeping after a merge

The parent Issue body is never edited: progress is read from GitHub and the
feature branch, not recorded
([README](../../issue-handoff/references/README.md#inputs-and-sources-of-truth)).
The only follow-up is closing a child: when the merged PR's `KIND` is
`implement`, close the child named by its `REFS` as `completed`. Those lines
come from the fixer that handled the PR. A child whose close was missed still
counts as done for the selection, because a merged PR `Refs` it; the
implementation worker reports it
as `DONE` if it is picked again, and you close it then.

## Integration refresh and the finish line

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
3. Drive the integration PR like [Drive a feature PR to merge](#drive-a-feature-pr-to-merge), with two differences. The review fixer
   uses the integration brief, and only blocking findings
   ([integrate.md](../../issue-handoff/references/integrate.md#review-of-the-integration-pr))
   are fixed: it answers and resolves the rest, and lists the real defects
   among them in the integration PR body. Its fixes go to a new sub-branch and
   a PR to the feature branch, never directly onto the feature branch. It
   returns that PR as `FIXED`; drive that PR with [Drive a feature PR to merge](#drive-a-feature-pr-to-merge) until merged, then go
   to step 4. Do not repeat checks, review, or fixer work on the updated
   integration PR head.
4. The **pre-merge sweep**, once per integration PR: when it has no
   sweep comment ([Derive the state](#derive-the-state)), remove every finished worker's
   worktree and check that the free space meets the e2e threshold
   ([Worktrees and disk space](#worktrees-and-disk-space)), then start a fresh stage worker with the sweep
   brief from [briefs.md](briefs.md). It runs the
   [pre-merge sweep](../../issue-handoff/references/integrate.md#pre-merge-sweep):
   every check on the feature head, the acceptance-criterion walk, and the
   pick of deferred defects worth fixing now, all fixed in at most one PR to
   the feature branch. It returns `DONE` with that PR, or with `PR: -` when
   nothing needed fixing, after posting the sweep comment. Drive the sweep PR
   with [Drive a feature PR to merge](#drive-a-feature-pr-to-merge) until merged, then go to step 5. Do not review, sweep or fix the updated
   integration PR head again.
5. The **finish line**: after the review pass, the sweep, and the merge of any
   PR either opened, confirm only that GitHub reports the integration PR
   conflict-free and mergeable. If branch protection blocks it, stop and
   report the blocker without rerunning checks or review. Do not merge it.
   Report the integration PR link to the maintainer, and stop.

## Worktrees and disk space

Every worker runs in its own worktree, and most of them install the
dependencies there (`npm ci` in `web/`, `tools/` and `docs-site/`) and build
the Go binaries and the e2e media. One worktree grows to several GB, and a
feature starts dozens of workers. Claude's `isolation: "worktree"` removes a
worktree only when the worker changed nothing, so a worker that pushed, or
that stopped with a merge or checkout half done, leaves its worktree and its
`worktree-agent-*` branch behind. The orchestrator removes them.

**Remove a worker's worktree once you have acted on its return**: for `DONE`,
`FIXED` or `CLEAN` with a PR, after that PR merges (or is left as `FOREIGN`);
for any other return, at once. No later worker reuses it: every worker starts
fresh from `origin`. The Agent result names the worktree path and branch; for
Codex they are the ones you created. First confirm that nothing the worker
made is lost:

```sh
git -C <path> fetch origin
git -C <path> branch -r --contains HEAD   # must list a remote branch
```

When `HEAD` is on a remote branch, uncommitted changes in the worktree are an
abandoned step, not the worker's result (workers return only what they
pushed). Remove it with `git worktree remove --force <path>` and
`git branch -D <branch>`, then `git worktree prune`. When `HEAD` is on no
remote branch, keep the worktree and name its path in the final report.

At the start of every iteration, apply the same rule to every worktree under
`.claude/worktrees/` (`git worktree list`) that does not belong to a worker
still running in this session. After a restart none is running, so this
also clears what an earlier session left.

**Check the free space** on the repository's file system
(`df -Pk <repository root>`, the `Available` column) before starting a
worker. Below 5 GB, remove the finished workers' worktrees first; if it is
still below 5 GB, stop and report. The pre-merge sweep runs `task test-e2e`,
which builds the binaries, generates the e2e media and starts Chromium, so it
needs 10 GB: below that after the cleanup, stop and report instead of
starting it. Its worker checks again before running e2e
([integrate.md](../../issue-handoff/references/integrate.md#pre-merge-sweep)).

The download caches are already shared: npm's cache (`~/.npm`), Go's module
cache and build cache (`go env GOMODCACHE GOCACHE`) live in the user's home,
not in the worktree, so a second `npm ci` or `go build` does not download or
recompile what an earlier worker did. What each worktree holds on its own is
`node_modules/`, `web/dist/`, the e2e build output and the test results.
They are not shared on purpose: a worktree on another commit may need other
dependency versions, and a shared `node_modules/` would let one worker's
`npm ci` break another's run. Removing finished worktrees is what keeps the
disk in bounds.

## Waiting

Waiting runs no worker. Subscribe to the PR's activity and schedule a check-in
roughly 15 minutes out when the host offers both, and end the turn; otherwise
poll at an interval of a few minutes. On every wake, start again from [Derive the state](#derive-the-state) — an
event tells you something changed, not what to do.
