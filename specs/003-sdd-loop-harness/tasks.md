# Tasks: Replace the Claude loop with Issue handoff

The retired controller's T001-T035 history is preserved in
[tasks-retired-routine.md](./tasks-retired-routine.md). Those IDs are reserved
and are not inputs to Tasks-to-sub-issues. This file is the only current task
artifact for this feature.

## Repository migration

- [x] T036 Enable CI for PRs targeting any base branch.
- [x] T037 Add agent-neutral Specify, Plan, Design, Tasks, Tasks-to-sub-issues,
  and Implement workflows.
- [x] T038 Add the network-free local state inspector and fixture tests.
  Retired by T044 after state inspection moved into the shared skill.
- [x] T039 Connect Claude's Spec Kit skills to the common workflow contract and
  replace repository-wide task Issue matching with native parent sub-issues.
- [x] T040 Replace the active design, specification, data model, research,
  contracts, quickstart, and contributor map with Issue handoff behavior.
- [x] T041 Remove `/sdd-next`, its scripts/tests, usage hook, and old test target;
  update all live references and technical-debt records.
- [ ] T042 Run `make check` and repository-wide searches for retired executable
  references. Local-dev tests, Web format/lint/build/tests, and searches pass; full `make check`
  remains unavailable on this machine because `make` and Go are not installed.
- [ ] T043 Obtain independent specification approval, verify
  Issue/PR/sub-issue behavior on GitHub, then disable and delete the external
  Claude Routine and remove the `sdd` automation label. GitHub MCP repository,
  permission, Issue/PR read, and native sub-issue read checks pass; the current
  connector does not expose sub-issue mutation or repository-label deletion.
- [x] T044 Consolidate every repository skill under `.agents/skills`, expose
  the same directory to Claude through one `.claude/skills` symlink, move the
  handoff contract into the shared skill, and remove the superseded inspector,
  wrapper, and fixture tests.
