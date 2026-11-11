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
STATUS: DONE | CLEAN | FIXED | FOREIGN | DROPPED | BLOCKED
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
BLOCKED_ON: <requester | artifact | prerequisite | other | ->
```

`BLOCKED_ON` tells the orchestrator what a `BLOCKED` needs, without it reading
the `BLOCKER` text:

- `artifact`: the Plan, `ui-design.md`, another artifact in the feature
  directory, or a child Issue written from them contradicts the code, another
  artifact or itself, or leaves undecided something this stage needs, and the
  parent Issue as written still decides the answer. The orchestrator can have the Plan revised
  ([loop.md, Revise the Plan](loop.md#revise-the-plan)). Name the artifact,
  the section and the fact it contradicts in `BLOCKER`.
- `requester`: the answer changes what the user gets (Q-6, Q-7 in
  [spec-quality.md](../../../../docs/product-specs/spec-quality.md)), or the
  parent Issue itself has to change.
- `prerequisite`: a child this one depends on is not merged yet.
- `other`: anything else, such as a check that did not pass with no cause to
  fix.

## Stage worker: `plan`, `design`, implementation

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: <plan | design | implement>
Parent Issue: #<parent>   Child Issue: #<child or ->
Feature branch: <feature or "none yet"> Feature directory: <dir or ->
Checkout: <worktree path the orchestrator created, or "own"> — work only there
Procedure: .agents/skills/issue-handoff/references/README.md and
  .agents/skills/issue-handoff/references/<plan|design|implement>.md.
The orchestrator selected this stage; do not re-select it.

Create your sub-branch from origin/<feature> (plan: create the feature branch
from main first), do the stage's work and checks, commit, push (plan: the new
feature branch first), and open the PR to <feature> as the stage reference says.
Put out-of-scope findings in the PR body and in DEFERRED. Return STATUS: DONE.
If the work needs an approved artifact changed, return BLOCKED without pushing,
with BLOCKED_ON: artifact when the parent Issue still decides the answer and
BLOCKED_ON: requester when it does not.
Implement only: if a PR merged into <feature> already Refs this child, change
nothing and return DONE with that PR. If a dependency named in the child (its
`Depends on:` line) is not merged into <feature> yet, change nothing and
return BLOCKED naming it. Other children may be implemented in parallel on
their own branches; stay inside this child's scope.
```

## Stage worker: `plan-to-issues`

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: plan-to-issues   Parent Issue: #<parent>
Feature branch: <feature>   Feature directory: <dir>
Revisions: <#PR of each merged Plan revision not reconciled yet, or ->
Procedure: .agents/skills/issue-handoff/references/README.md and
  .agents/skills/issue-handoff/references/plan-to-issues.md.
Create the missing native sub-issues in Implementation Work order, each
dependency before the unit that needs it, and start each body with its
`Depends on:` line. When Revisions names a PR, also reconcile the existing
children with the revised Plan as the "After a Plan revision" section says,
then comment `<!-- sdd-children-reconciled -->` on each PR it names, last.
Return STATUS: DONE, or BLOCKED with the question the plan does not settle
(BLOCKED_ON: artifact when the Plan should have decided it, requester when
the parent Issue should).
```

## Stage worker: Plan revision

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: plan (revision)   Parent Issue: #<parent>
Feature branch: <feature>   Feature directory: <dir>
Blocked work: <#child, #PR, or "plan-to-issues" / "design">
Blocker: <the blocked worker's BLOCKER line, verbatim; one line per blocker>
Procedure: .agents/skills/issue-handoff/references/README.md and the
  "Revising a merged Plan" section of
  .agents/skills/issue-handoff/references/plan.md.
The orchestrator selected this revision; do not re-select the stage.

Verify each blocker against the code on origin/<feature>, the artifacts and
the parent Issue. Revise on a sub-branch from origin/<feature> and open one PR
to <feature> with Refs #<parent>, with `<!-- sdd-plan-revision -->` as the
last line of its body. Do not create, edit or close child Issues; a later
plan-to-issues run reconciles them. Do not edit the parent Issue.
Return STATUS: DONE with the revision PR and KIND: plan. Return BLOCKED,
changing nothing, when a blocker is neither a contradiction nor a decision
the Plan left open (BLOCKED_ON: other, saying why) or when resolving it needs the requirement or an acceptance
criterion changed (BLOCKED_ON: requester).
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
review fixer does. Do not run the pre-merge sweep either; it has its own worker.
Return STATUS: DONE with the integration PR in PR, or BLOCKED when a conflict
needs a product decision.
```

## Stage worker: pre-merge sweep

```text
Autopilot stage worker. Repository: <owner/repo>.
Stage: pre-merge sweep   Parent Issue: #<parent>   Integration PR: #<pr>
Feature branch: <feature>   Feature directory: <dir>
Procedure: the "Pre-merge sweep" section of
  .agents/skills/issue-handoff/references/integrate.md, once.
Run every check it lists on origin/<feature> and walk the parent's acceptance
criteria. Run e2e as a manual CI run on origin/<feature>, as the section
says, not locally; when the run cannot be started, return BLOCKED naming it
instead of skipping e2e. Collect the backlog (the integration PR body's remaining risks, the
merged feature PRs' deferred items, unresolved threads on #<pr>), verify each,
and pick only what the section's criteria say is worth fixing now, at most
five. Resolve the threads on #<pr> whose fix PR has merged, as the section
says. Fix the check failures and the picked items on one sub-branch from
origin/<feature> and open one PR to <feature> with Refs #<parent>. Do not push
to <feature>, and do not touch a conflict with main.
Post the sweep comment on #<pr> as the section says, last.
Return STATUS: DONE with the sweep PR in PR (or PR: - when nothing needed
fixing), KIND: integration-fix, and the items left in DEFERRED. Return BLOCKED
when a failing check or an unmet acceptance criterion needs a requester
decision or an approved artifact changed.
```

## Review fixer: feature PR

```text
Autopilot review fixer. Repository: <owner/repo>. PR: #<pr> into <base>.
Head: <sha>. Parent Issue: #<parent>.
First: if the PR's Refs names neither #<parent> nor one of #<parent>'s native
sub-issues, change nothing and return FOREIGN. If the child it Refs is closed
as not planned, change nothing and return DROPPED with the child in REFS.
<only after a Plan revision: "After a Plan revision: first merge origin/<base>
into the head branch even without a conflict, then handle the round against
the revised artifacts.">
Handle this one round:
- every check on the head that did not pass (failure, cancelled, timed_out,
  action_required, stale): read its log, find the root cause, fix it
- every unresolved review thread: verify the finding against the code and the
  sources of truth; fix it when it is a real defect, otherwise reply why not
- a conflict with <base>: merge origin/<base> into the head branch
For tests added or changed by a fix, apply .agents/skills/write-tests/SKILL.md
before editing and report its evidence in the PR validation section.
Check out the PR's head branch, run the checks the change needs,
including the pre-push check from AGENTS.md, commit in
the repository's commit-message style, push to the same branch, then reply
once on each thread you handled and resolve it.
Return FIXED with the new head, or CLEAN if nothing needed changing. Never
return CLEAN while a check on the head has not passed; when you find no cause
to fix (a cancelled run, for example), return BLOCKED. Return BLOCKED when a fix
needs an approved artifact changed (BLOCKED_ON: artifact) or a requester
decision changed (BLOCKED_ON: requester), or when an
unresolved finding repeats one this PR already fixed and resolved.
An artifact in <feature-dir> that this PR itself adds or changes is not
approved yet: fix it there.
Put the Issue the PR references in REFS, and in KIND what the PR is: `plan`
(it adds or revises <feature-dir>/plan.md), `design` (ui-design.md),
`implement` (it Refs a child), or `integration-fix` (it Refs the parent and
fixes a review of the integration PR, or is the pre-merge sweep's PR).
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
thread it fixes naming that PR and leave it unresolved; the pre-merge sweep
resolves it after the fix PR merges. This workflow does not review the
integration PR again after the fix PR merges.
Return FIXED with the new PR number. Return CLEAN only when every check on the
head passed and no blocking thread needed a fix; a check that did not pass
with no cause to fix is BLOCKED, as in the feature-PR brief.
```
