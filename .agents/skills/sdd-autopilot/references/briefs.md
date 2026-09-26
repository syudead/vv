# Worker briefs

Fill the `<…>` slots with numbers, branch names and paths only. Do not paste
Issue text, artifact content, diffs, or findings into a brief: the worker reads
them itself, in its own context. Add nothing else unless a previous worker's
`BLOCKER` line is exactly what this one must resolve.

## Return block

Anything longer than the return block goes into the PR body or a file, not
into the reply.

Every worker ends its reply with this block and nothing after it. Keep each
line to one line.

```text
STATUS: DONE | CLEAN | FIXED | FOREIGN | BLOCKED
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

Create your sub-branch from origin/<feature> (plan: create the feature branch
from main first), do the stage's work and checks, commit, push (plan: the new
feature branch first), and open the PR to <feature> as the stage reference says.
Put out-of-scope findings in the PR body and in DEFERRED. Return STATUS: DONE.
If the work needs an approved artifact changed, return BLOCKED without pushing.
Implement only: if a PR merged into <feature> already Refs this child, change
nothing and return DONE with that PR. If a dependency named in the child is
not merged into <feature> yet, change nothing and return BLOCKED naming it.
```

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
Stage: integrate   Parent Issue: #<parent>   Integration PR: #<pr or "none yet">
Feature branch: <feature>   Feature directory: <dir>
Procedure: .agents/skills/issue-handoff/references/integrate.md, in one phase.
Regenerate generated files with `task generate` when resolving conflicts,
never by hand. Open the integration PR if it does not exist yet. In its body,
also list the out-of-scope items the merged feature PRs' bodies deferred, and
keep the remaining risks it already lists. Do not handle its review here; the
review fixer does.
Return STATUS: DONE with the integration PR in PR, or BLOCKED when a conflict
needs a product decision.
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
Check out the PR's head branch, run the pre-push check from AGENTS.md
(`task check` for code changes), commit in
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
Triage every finding first, as the "Review of the integration PR" section of
.agents/skills/issue-handoff/references/integrate.md says. A finding that is
not blocking: reply in one line and resolve it now; when it is a real defect,
add it to the integration PR body's remaining risks instead of fixing it.
A blocking finding you verified is not a defect: reply why, and resolve it now.
A real blocking defect or a check on the head that did not pass: create a
sub-branch from origin/<feature>, commit the fixes there, push it, open a PR to
<feature> titled for the review round, with Refs #<parent>. Reply on each
thread it fixes naming that PR and leave it unresolved. List those threads in
DEFERRED for the maintainer; this workflow does not review the integration PR
again after the fix PR merges.
Return FIXED with the new PR number. Return CLEAN only when every check on the
head passed and no blocking thread needed a fix; a check that did not pass
with no cause to fix is BLOCKED, as in the feature-PR brief.
```
