# Issue handoff workflows

This skill is the shared contract for one-stage-at-a-time SDD work. Claude,
Codex, and other Agent Skills-compatible tools use the same files.

## Directory ownership

| Path | Owner | Purpose |
| --- | --- | --- |
| `.agents/skills/` | This repository | Shared Agent Skills and handoff procedures |
| `.codex/agents/` | This repository | Optional project-scoped Codex workers used inside a handoff run |
| `.specify/templates/` | This repository | Artifact templates, edited directly |

Spec Kit is not upgraded any more, so nothing under `.specify/` is treated as
vendored. `.specify/templates/plan-template.md` is the only part this workflow
still uses, and it is edited in place like any other file here.

The rest of Spec Kit's scaffolding (`.specify/scripts/`, `.specify/workflows/`,
the integration manifests) was removed because this workflow never invoked it.
Do not reinstate it.

Project-scoped workers under `.codex/agents/` may perform a bounded part of a
run when Codex is the selected host. They do not own the handoff, persist its
state, or start another stage; the parent agent remains responsible for the
workflow and its pull request. Other Agent Skills-compatible hosts may use an
equivalent bounded worker or perform that part locally.

## Inputs and sources of truth

- An Issue-driven run starts from an explicitly supplied Issue. An explicitly
  supplied PR or branch may provide additional context.
- The parent Issue is the specification. It carries the requirement, the
  acceptance criteria, and an `## SDD` summary. The
  [`issue-spec` skill](../../issue-spec/SKILL.md) writes and revises it; no
  stage here restates it into a repository file.
- Files on the selected branch describe the available artifacts; their presence
  does not gate a user-requested workflow.
- Read standard GitHub Issue, PR, and native sub-issue relationships when they
  are relevant. Do not copy PR, branch, or child-Issue lists into the parent
  body or create a second relationship registry in repository files.

The parent summary has this form:

```markdown
## SDD

- [ ] Plan
- [ ] Design
- Next: `plan`
```

Omit `Design` unless the parent has the existing `ui` domain label. Remove
`Next` after `plan-to-issues` succeeds. The checklist and `Next` communicate
progress to humans; they do not authorize or block a requested workflow.

## GitHub preflight

Verify only the capabilities needed by the requested workflow. Every workflow
needs Issue and PR read access. Plan, Design, and Implement need repository
push and PR creation access. `plan-to-issues` needs Issue write
access and native sub-issue operations but does not require push or PR creation.
Stop before mutation when a required capability is missing.

Use the supplied Issue, PR, branch, and current checkout directly. Read their
standard GitHub relationships as ordinary context; do not run a repository-
specific traversal to recover a branch or feature directory, compare multiple
records for consistency, or require a unique candidate. When review fixes are
requested for a PR, update that PR's head. Ask the user only when information
that is actually required for the requested mutation is unavailable.

After checkout, read `plan.md` and optional `ui-design.md` when they are
relevant and available. Their metadata and the parent checklist are useful
context, not identity checks or execution gates.

Do not add a `spec.md` to a feature directory; the requirement lives in the
parent Issue. A feature directory holds `plan.md` and, for a `ui` Issue,
`ui-design.md`.

Name the feature directory explicitly; never select work from a branch name or
a prior conversation. Use a separate checkout or worktree for each concurrent
run.

The skill deliberately does not infer whether an existing downstream
artifact incorporates a later upstream revision. When an approved artifact is
revised, the maintainer resets the parent SDD summary and reruns the affected
stages through reviewed PRs.

## Branch and PR contract

- The long-lived feature branch starts from `main`, and `plan` is the stage that
  creates it.
- Every stage and implementation runs on an arbitrary-name sub-branch created
  from the current feature branch.
- Stage and implementation PRs target the feature branch. The integration PR
  targets `main` and remains open after the Plan PR is merged.
- Stage PRs use `Refs #<parent>`. Implementation PRs use `Refs #<child>`.
  Only the integration PR uses `Closes #<parent>`.
- Do not derive hidden identity rules from branch names, Issue numbers,
  feature-directory numbers, labels, JSON packets, or session IDs.
- A run performs one workflow, opens or updates one PR, and stops. PR merges do
  not start another agent.

Humans merge every PR. After a stage PR merge, the maintainer updates the
parent SDD summary. After an implementation PR merge, the maintainer closes
that child Issue as completed. The integration PR is merged only after all
children are resolved, latest `main` has been merged through a reviewed
sub-branch, and the full checks pass.
