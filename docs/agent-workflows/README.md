# Issue handoff workflows

This directory is the agent-neutral contract for one-stage-at-a-time SDD work.
Agent-specific skills may point here, but must not redefine branch discovery,
stage ordering, or Issue/PR ownership.

## Directory ownership

| Path | Owner | Purpose |
| --- | --- | --- |
| `docs/agent-workflows/` | This repository | Agent-neutral handoff and PR procedures |
| `scripts/issue-handoff/` | This repository | Local, stateless inspection and Spec Kit invocation |
| `tests/issue-handoff/` | This repository | Handoff contract tests |
| `.claude/skills/` | Claude adapter | Thin entry points to the agent-neutral procedures |
| `.specify/` | Spec Kit | Installed templates, scripts, metadata, and bundled workflows |

Do not put repository handoff policy or tests under `.specify/`, and do not
patch installed Spec Kit scripts to implement this workflow. The external
wrapper isolates Spec Kit's optional machine-local selection file instead.
The bundled `.specify/workflows/speckit/workflow.yml` belongs to Spec Kit. This
handoff does not invoke it, and it is not a GitHub event trigger.

`.specify/init-options.json`, `.specify/integration.json`, and integration
manifests record how Spec Kit was installed and updated. They do not select the
agent for an Issue handoff run and are not runtime state.

## Inputs and sources of truth

- Every run starts from one explicitly supplied parent Issue or sub-issue.
- The parent Issue contains the requirement and an `## SDD` summary only.
- Merged files on the feature branch are the source of truth for artifact state.
- `**Parent Issue**: #NNN` in `spec.md` is the only repository mapping between
  a feature directory and its parent Issue.
- Generated downstream artifacts record their exact upstream path and commit in
  an `SDD input` marker. A downstream artifact is current only while that marker
  matches the latest commit that changed its upstream artifact.
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
`Next` after `taskstoissues` succeeds.

## GitHub preflight

Use the agent's native GitHub integration. This repository intentionally has
no command that authenticates to GitHub. The Claude and Codex adapters use
GitHub MCP and never fall back to `gh`.

Before changing the checkout, verify that the integration can read Issues and
PRs and can push and create PRs. `taskstoissues` additionally requires Issue
write access and native sub-issue operations. Stop before changing files when
a required capability is missing.

Resolve the feature branch without naming conventions:

1. When Spec is complete, find the one open PR that targets `main` and closes
   the parent Issue. Its head is the feature branch. Zero or multiple matches
   are an error.
2. For a child Issue, get its native parent first, then apply step 1.
3. Before Spec is merged, inspect open PRs that reference the parent and add a
   `spec.md` whose `**Parent Issue**` matches it. Zero means start `specify`,
   one means continue that PR only, and multiple means stop.
4. If an open PR already exists for the requested stage or child Issue,
   update that PR's head for review fixes; do not open another PR.

After checkout, run this for an existing feature directory:

```bash
bash scripts/issue-handoff/sdd-stage.sh --feature specs/NNN-name [--ui]
```

The command is local and read-only. For Spec through Tasks, its result must
agree with the parent SDD summary. On disagreement, stop and report the
difference; a maintainer updates the Issue before the run is retried. A brand
new Specify has no directory yet and does not run this command.

Historical feature directories without an exact `**Parent Issue**: #NNN` line
are not auto-migrated and are not valid handoff inputs. They remain historical
artifacts. Continuing one requires a maintainer to choose a parent Issue and
add the exact mapping through a reviewed artifact PR; never infer it from a
directory number, branch name, or old PR.

Every adapter supplies `SPECIFY_FEATURE_DIRECTORY` explicitly. Plan, Tasks,
and Implement invoke Spec Kit through
`scripts/issue-handoff/run-speckit.sh`, which restores any pre-existing local
`.specify/feature.json` after the command. The handoff therefore neither reads
nor changes persistent session selection while `.specify/` remains owned by
the installed Spec Kit distribution. Wrapper runs are serialized with a
repository-local lock so concurrent agents cannot restore or remove each
other's selection state.

`next=taskstoissues` means only that repository artifacts are ready for GitHub
reconciliation. The command cannot tell whether sub-issues already exist. At
that boundary, `Next: taskstoissues` means reconciliation is pending, while no
`Next` plus reconciled native sub-issues means implementation may proceed.

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
