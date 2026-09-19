# Issue handoff workflows

This skill is the shared contract for one-stage-at-a-time SDD work. Claude,
Codex, and other Agent Skills-compatible tools use the same files.

## Directory ownership

| Path | Owner | Purpose |
| --- | --- | --- |
| `.agents/skills/` | This repository | Shared Agent Skills and handoff procedures |
| `.specify/` | Spec Kit | Installed templates, scripts, metadata, and bundled workflows |

Do not put repository handoff policy under `.specify/`, and do not patch
installed Spec Kit scripts to implement this workflow.
The bundled `.specify/workflows/speckit/workflow.yml` belongs to Spec Kit. This
handoff does not invoke it, and it is not a GitHub event trigger.

`.specify/init-options.json`, `.specify/integration.json`, and integration
manifests record how Spec Kit was installed and updated. They do not select the
agent for an Issue handoff run and are not runtime state.

## Inputs and sources of truth

- Every run starts from one explicitly supplied parent Issue or sub-issue.
- The parent Issue contains the requirement and an `## SDD` summary only.
- Files on the selected branch describe the available artifacts; their presence
  does not gate a user-requested workflow.
- `**Parent Issue**: #NNN` in `spec.md` is the only repository mapping between
  a feature directory and its parent Issue.
- PR `head`/`base`, Issue timeline references, the integration PR's
  `Closes #NNN`, and native GitHub sub-issues carry relationships. Do not copy
  PR, branch, or child-Issue lists into the parent body.

The parent summary has this form:

```markdown
## SDD

- [ ] Spec
- [ ] Plan
- [ ] Design
- [ ] Tasks
- Next: `specify`
```

Omit `Design` unless the parent has the existing `ui` domain label. Remove
`Next` after `taskstoissues` succeeds. The checklist and `Next` communicate
progress to humans; they do not authorize or block a requested workflow.

## GitHub preflight

Before changing the checkout, verify that the integration can read Issues and
PRs and can push and create PRs. `taskstoissues` additionally requires Issue
write access and native sub-issue operations. Stop before changing files when
a required capability is missing.

Resolve the feature branch without naming conventions:

1. When Spec is complete, inspect open PRs that target `main` and close the
   parent Issue. Use the explicitly supplied PR or branch when available. If
   more than one candidate remains and the target cannot be determined from the
   request, ask which one to use; multiple PRs are not an error.
2. For a child Issue, get its native parent first, then apply step 1.
3. Before Spec is merged, inspect open PRs that reference the parent and add a
   `spec.md` whose `**Parent Issue**` matches it. Continue an explicitly
   supplied PR for review work. Otherwise, use an unambiguous requested target
   or create another PR; existing matching PRs do not block the run.
4. When the user requests review fixes for an existing PR, update that PR's
   head. A request to run a stage may create a separate PR even when another PR
   for that stage is open.

After checkout, inspect the selected feature directory directly. Use the exact
`**Parent Issue**: #NNN` line in `spec.md` when mapping an existing directory to
its parent. Read `spec.md`, `plan.md`, optional `ui-design.md`, and `tasks.md` as
inputs when they exist. Do not compare their presence with the parent checklist
or `Next`, and do not stop merely because those descriptions differ.

Historical feature directories without an exact `**Parent Issue**: #NNN` line
are not auto-migrated and are not valid handoff inputs. They remain historical
artifacts. Continuing one requires a maintainer to choose a parent Issue and
add the exact mapping through a reviewed artifact PR; never infer it from a
directory number, branch name, or old PR.

Supply `SPECIFY_FEATURE_DIRECTORY` explicitly whenever invoking Spec Kit; never
select work from a branch name, prior conversation, or existing
`.specify/feature.json`. Use a separate checkout or worktree for each concurrent
run. Before invoking Spec Kit, preserve any pre-existing
`.specify/feature.json` without reading it. Restore it after the command, or
remove the file produced by the command when none existed before.

`Next: taskstoissues` can tell a maintainer that reconciliation is pending, but
its presence or absence does not determine whether implementation may proceed.

The skill deliberately does not infer whether an existing downstream
artifact incorporates a later upstream revision. When an approved artifact is
revised, the maintainer resets the parent SDD summary and reruns the affected
stages through reviewed PRs.

## Branch and PR contract

- The long-lived feature branch starts from `main`.
- Every stage and implementation runs on an arbitrary-name sub-branch created
  from the current feature branch.
- Stage and implementation PRs target the feature branch. The integration PR
  targets `main` and remains open after Spec is merged.
- Stage PRs use `Refs #<parent>`. Implementation PRs use `Refs #<child>`.
  Only the integration PR uses `Closes #<parent>`.
- No branch name, Issue number, feature-directory number, label, JSON packet,
  session ID, or previous conversation selects the feature or stage.
- A run performs one workflow, opens or updates one PR, and stops. PR merges do
  not start another agent.

Humans merge every PR. After a stage PR merge, the maintainer updates the
parent SDD summary. After an implementation PR merge, the maintainer closes
that child Issue as completed. The integration PR is merged only after all
children are resolved, latest `main` has been merged through a reviewed
sub-branch, and the full checks pass.
