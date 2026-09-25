# Worker briefs

Fill the `<…>` slots with numbers, branch names and paths only. Do not paste
Issue text, artifact content, diffs, or findings into a brief: the worker reads
them itself, in its own context. Add nothing else unless a previous worker's
`BLOCKER` line is exactly what this one must resolve.

## Return block

Anything longer than the return block goes into a file, not into the reply.
Findings files live under `$(git rev-parse --git-common-dir)/sdd-autopilot/`,
which every worker in the checkout can read and nothing commits. Name one per
branch and round, for example `review-<branch>-1.md`.

The one accepted exception is a reviewer whose sandbox cannot write that file
(Codex's read-only `self_reviewer`). It returns the findings in its reply;
pass that reply to the stage worker's phase 2 unchanged, and do not read,
summarise, or act on it yourself.

Every worker ends its reply with this block and nothing after it. Keep each
line to one line.

```text
STATUS: READY | DONE | CLEAN | FINDINGS | FIXED | FOREIGN | BLOCKED
BRANCH: <branch or ->
BASE: <base branch or ->
HEAD: <commit SHA or ->
PR: <#number or ->
REFS: <#Issue the PR references, or ->
KIND: <plan | design | implement | integration-fix | integrate | ->
FEATURE_DIR: <specs/NNN-name or ->
CHECKS: <commands run and their result>
DEFERRED: <out-of-scope item for the PR body or the maintainer, or none>
BLOCKER: <what stops this, and whose decision it is, or none>
```

## Stage worker: `plan`, `design`, implementation

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: <plan | design | implement>
Parent Issue: #<parent>   Child Issue: #<child or ->
Feature branch: <feature or "none yet"> Feature directory: <dir or ->
Procedure: .agents/skills/issue-handoff/references/README.md and
  .agents/skills/issue-handoff/references/<plan|design|implement>.md.
The orchestrator selected this stage; do not re-select it.

Phase 1 now: create your sub-branch from origin/<feature> (plan: create the
feature branch from main locally first), do the stage's work and its checks,
and commit. Push nothing, not even a new feature branch, and open no PR.
Return STATUS: READY.
Implement only: if a PR merged into <feature> already Refs this child, change
nothing and return DONE with that PR. If a dependency named in the child is
not merged into <feature> yet, change nothing and return BLOCKED naming it.
```

Continuation, sent to the same worker after self-review:

```text
Phase 2. Self-review returned STATUS: <CLEAN | FINDINGS>; the findings are in
<findings file>.
Fix the findings that are defects within this stage, re-run your checks,
commit, push (plan: the new feature branch first), and open the PR to
<feature> as the stage reference says. A
finding that needs an approved artifact changed: do not push; return BLOCKED.
Put out-of-scope findings in the PR body and in DEFERRED. Return STATUS: DONE.
```

When the original worker is gone but you still have its `BRANCH` in this
session, start a fresh one with the stage brief plus
`Phase 1 is already committed on <branch>; go straight to phase 2.` and the
continuation text. After a restart you will not have it; nothing was pushed, so
§1 selects the same stage again and a fresh worker redoes it.

## Stage worker: `plan-to-issues`

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: plan-to-issues   Parent Issue: #<parent>
Feature branch: <feature>   Feature directory: <dir>
Procedure: .agents/skills/issue-handoff/references/README.md and
  .agents/skills/issue-handoff/references/plan-to-issues.md.
Create the missing native sub-issues in Implementation Work order. Return STATUS: DONE, or BLOCKED with the
question the plan does not settle.
```

## Stage worker: integration refresh

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: integrate   Parent Issue: #<parent>   Integration PR: #<pr>
Feature branch: <feature>   Feature directory: <dir>
Procedure: .agents/skills/issue-handoff/references/integrate.md, in one phase.
Regenerate generated files with `task generate` when resolving conflicts,
never by hand. In the integration PR body, also list the out-of-scope items
the merged feature PRs' bodies deferred. Return STATUS: DONE, or BLOCKED when a
conflict needs a product decision.
```

## Self reviewer

```text
Self-review for an autopilot stage. Repository: <owner/repo>.
Diff: origin/<base>...<branch> (committed locally; read it with git).
Parent Issue: #<parent>   Child Issue: #<child or ->
Feature directory: <dir>
Procedure: .agents/skills/self-review/references/README.md.
Write the numbered findings (file:line, source cited, defect or deferred) to
<findings file>, and reply with the return block only: STATUS: CLEAN when
there is no defect, FINDINGS otherwise.
```

## Review fixer: feature PR

```text
Autopilot review fixer. Repository: <owner/repo>. PR: #<pr> into <base>.
Head: <sha>. Parent Issue: #<parent>.
First: if the PR's Refs names neither #<parent> nor one of #<parent>'s native
sub-issues, change nothing and return FOREIGN.
Handle this one round:
- every check on the head that did not pass (failure, cancelled, timed_out,
  action_required, stale): read its log, find the root cause, fix it
- every unresolved review thread: verify the finding against the code and the
  sources of truth; fix it when it is a real defect, otherwise reply why not
- a conflict with <base>: merge origin/<base> into the head branch
Check out the PR's head branch, run the checks the change needs, commit in
the repository's commit-message style, push to the same branch, then reply
once on each thread you handled and resolve it.
Return FIXED with the new head, or CLEAN if nothing needed changing. Never
return CLEAN while a check on the head has not passed; when you find no cause
to fix (a cancelled run, for example), return BLOCKED. Return BLOCKED when a fix
needs an approved artifact or a requester decision changed, or when an
unresolved finding repeats one this PR already fixed and resolved.
Put the Issue the PR references in REFS, and in KIND what the PR is: `plan`
(it adds or revises <feature-dir>/plan.md), `design` (ui-design.md),
`implement` (it Refs a child), or `integration-fix` (it Refs the parent and
fixes a review of the integration PR).
```

## Review fixer: integration PR

Same as the feature-PR brief, with these two changes.

Replace the `First:` line, because the integration PR carries `Closes`, not
`Refs`:

```text
First: if the PR is not from <feature> to main with Closes #<parent>, change
nothing and return FOREIGN.
```

Replace the push-and-resolve instruction:

```text
This is the integration PR. Ignore any conflict with main; the orchestrator
refreshes the feature branch itself. Do not push to <feature>.
A finding you verified is not a defect: reply why, and resolve it now.
A real defect or a check on the head that did not pass: create a sub-branch
from origin/<feature>, commit the fixes there, push it, and open a PR to
<feature> titled for the review round, with Refs #<parent>. Reply on each
thread it fixes naming that PR and leave it unresolved; resolve it only once
the fix it names is merged into <feature>, which a later round sees.
Return FIXED with the new PR number. Return CLEAN only when every check on the
head passed and no thread needed a fix; a check that did not pass with no
cause to fix is BLOCKED, as in the feature-PR brief.
```
