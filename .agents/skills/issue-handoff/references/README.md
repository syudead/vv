# Issue handoff workflows

This skill is the shared contract for explicitly requested, one-stage-at-a-time
SDD feature work. Claude, Codex, and other Agent Skills-compatible tools use the
same files. Other changes follow the repository's normal branch-to-`main`
workflow.

## Directory ownership

| Path | Owner | Purpose |
| --- | --- | --- |
| `.agents/skills/` | This repository | Shared Agent Skills and handoff procedures |
| `.codex/agents/` | This repository | Project-scoped Codex workers used inside a handoff run |
| `.claude/agents/` | This repository | Project-scoped Claude workers used inside a handoff run |

The Plan template lives with the skill that fills it, at
`.agents/skills/sdd-plan/assets/plan-template.md`. There is no `.specify/`
directory; do not reinstate one.

Project-scoped workers may perform a bounded part of a run when the selected
host supports them. The repository provides matching Codex and Claude workers:
`subissue-implementer` for child-Issue implementation and focused checks.
`sdd-stage-worker` and
`pr-review-fixer` exist for `sdd-autopilot` only, and there they push and open
PRs themselves. The other workers do not own the handoff, persist its state,
or start another stage; the parent agent remains responsible for the workflow,
fixes, full validation, push, and pull request.
Other Agent Skills-compatible hosts should use an equivalent bounded worker
when one is available, or perform that part locally.

## Inputs and sources of truth

- An Issue-driven run starts from a supplied Issue URL. That URL is normally
  the whole request: the run selects its own stage (see
  [Selecting the stage](#selecting-the-stage)). A stage, PR, or branch the user
  names explicitly overrides the selection.
- The parent Issue is the specification. It carries the requirement and the
  acceptance criteria, and nothing about workflow progress. The
  [`issue-spec` skill](../../issue-spec/SKILL.md) writes and revises it; no
  stage here restates it into a repository file.
- Files on the selected branch describe the available artifacts; their presence
  does not gate a user-requested workflow.
- Read standard GitHub Issue, PR, and native sub-issue relationships when they
  are relevant. Do not copy PR, branch, or child-Issue lists into the parent
  body or create a second relationship registry in repository files.

Progress is read, not recorded. `plan.md` on the feature branch means Plan is
done; `ui-design.md` means Design is done; the parent's native sub-issues mean
`plan-to-issues` ran, and their open or closed state shows implementation. Do not add a progress checklist, a `Next` marker,
or any other stage record to the parent body. An older parent may still carry an
`## SDD` section; ignore it and leave it alone unless the user asks otherwise.

The `ui` label is the only label with workflow meaning: a parent carrying it
goes through `design` between `plan` and `plan-to-issues`. The `sdd` label is
retired. Never add or remove labels on your own.

## Selecting the stage

The user supplies an Issue and nothing else; the run works out what comes next
from GitHub and the feature branch, does that one thing, and stops. Read
state fresh on every run — never from a prior conversation.

**Locate the feature** from a parent Issue:

- The feature branch is the base of the merged PR that `Refs` the parent and
  does not target `main` (the Plan PR, and every later stage PR). The
  `specs/<dir>/plan.md` on that branch names the feature directory.
- Once `integrate` has opened it, the integration PR is the open PR into
  `main` whose closing references include the parent; its head is the same
  feature branch.
- If either lookup finds more than one candidate, stop and ask. Never fall back
  to branch names or directory numbers.

An Issue that is neither a native child nor a parent written as a
specification (no `要件` or `受け入れ条件`) is not SDD work; ask before doing
anything.

**Child Issue** (it has a native parent): run `implement` for that child. If it
is already done (see below), report that and stop. If it has an open
implementation PR, follow the open-PR rule below for that PR.

**Parent Issue**: take the first rule that applies.

1. **An open stage PR** (`Refs` the parent and does not target `main`). If it has unaddressed review feedback — a
   changes-requested review with no later push, or unresolved threads — fix it
   on that PR's head. Otherwise report that it waits on human merge and stop.
2. **No feature branch yet** → `plan`.
3. **`ui` label and no `ui-design.md` on the feature branch** → `design`.
4. **No native sub-issues** → `plan-to-issues`.
5. **A child that is not done** → `implement` the first such child, in
   sub-issue order, that has no open PR and whose prerequisites (the "has to
   land first" part of its body) are done. When every remaining child has an
   open PR, apply rule 1's review check to those PRs in order, and otherwise
   report what is waiting on merge and stop. When the rest are blocked only by
   prerequisites, report that and stop.
6. **Every child done** → [integrate](integrate.md), which opens the
   integration PR if it does not exist yet.

A child is **done** when it is closed as completed or a merged PR into the
feature branch `Refs` it. Closing children stays a maintainer action (under
`sdd-autopilot`, its orchestrator's); selection does not wait for it.

Report the selected stage and the facts behind it at the start of the run and
in the PR body, so a wrong selection is visible at review. Two runs started at
the same time on the same parent can pick the same child; the second PR is
closed at review.

A revision is the exception: re-running a stage whose artifact already exists
is never selected automatically. The maintainer names that stage.

## GitHub preflight

Verify only the capabilities needed by the requested workflow. Every workflow
needs Issue and PR read access. Plan, Design, and Implement need repository
push and PR creation access. `plan-to-issues` needs Issue write
access and native sub-issue operations but does not require push or PR creation.
Stop before mutation when a required capability is missing.

Recover the feature branch and directory only through the standard GitHub
relationships in [Selecting the stage](#selecting-the-stage). When review fixes
are made for a PR, update that PR's head. Ask the user only when information
that is actually required for the selected mutation is unavailable or
ambiguous.

After checkout, read `plan.md` and optional `ui-design.md` when they are
relevant and available. Their metadata is useful context, not an identity
check or an execution gate.

Do not add a `spec.md` to a feature directory; the requirement lives in the
parent Issue. A feature directory always holds `plan.md`, adds `ui-design.md`
for a `ui` Issue, and carries `research.md`, `data-model.md`, `contracts/` or
`quickstart.md` when the Plan has that content of its own (P-2).

Never select work from a branch name or a prior conversation. Use a separate
checkout or worktree for each concurrent run.

The skill deliberately does not infer whether an existing downstream
artifact incorporates a later upstream revision. When an approved artifact is
revised, the maintainer names the affected stages and reruns them through
reviewed PRs.

## Branch and PR contract

- The long-lived feature branch starts from `main`, and `plan` is the stage that
  creates it.
- Every stage and implementation runs on an arbitrary-name sub-branch created
  from the current feature branch.
- Stage and implementation PRs target the feature branch. The integration PR
  targets `main` and is opened by `integrate`, once every child is done. Until
  then the feature branch's review targets are its stage and implementation
  PRs: an integration PR opened earlier is reviewed on every feature-branch
  merge while the feature is half-built, and those reviews report the
  unfinished parts as defects.
- Stage PRs use `Refs #<parent>`. Implementation PRs use `Refs #<child>`.
  Only the integration PR uses `Closes #<parent>`.
- Do not derive hidden identity rules from branch names, Issue numbers,
  feature-directory numbers, labels, JSON packets, or session IDs.
- A run performs one workflow, opens or updates one PR, and stops. PR merges
  do not start another agent; the maintainer starts the next run by handing
  over the Issue URL again.
- The exception is the [`sdd-autopilot` skill](../../sdd-autopilot/SKILL.md),
  which the maintainer starts explicitly for one parent Issue. It applies the
  same stage selection in a loop, runs each stage in a fresh worker context,
  and merges the feature-branch PRs itself. The integration PR is still merged
  by a human.

Humans merge every PR (under `sdd-autopilot`, only the integration PR). A stage
PR merge needs no follow-up edit to the parent Issue. After an implementation
PR merge, the maintainer may close that child Issue as completed. After every
child is done, the next run [integrates](integrate.md) and a human merges the
integration PR.
