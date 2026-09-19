# Quickstart: Issue handoff SDD

## S1: Local artifact state

Verify that `.agents/skills/issue-handoff/SKILL.md` is discoverable and that
`.claude/skills` resolves to `.agents/skills`. Open the skill and confirm that
it routes to the expected stage reference without creating files or contacting
GitHub.

## S2: Parent Issue and Specify

1. Create a test parent Issue with the SDD summary and `Next: specify`.
2. Give only that Issue to one coding agent.
3. Verify that it creates an arbitrary feature branch and sub-branch, writes an
   exact parent line, and opens a Spec PR to the feature branch.
4. Merge the Spec PR, create one integration PR to `main` with
   `Closes #<parent>`, and update the parent to `Next: plan`.

Expected: no `sdd` label, Routine, branch pattern, or Issue/feature number
equality is used.

## S3: Cross-agent Plan

Give the same parent to a different agent implementation or fresh session.

Expected: it obtains the feature branch from the integration PR, creates Plan
on a new arbitrary sub-branch, opens one feature-targeting PR, and stops.

## S4: UI path

Repeat S2 with the existing `ui` label.

Expected: after Plan, the parent selects Design; after `ui-design.md` merges,
artifact work is complete.

## S5: Native sub-issues

After the final artifact merges, run Plan-to-issues to create native sub-issues
directly from the approved Plan and optional UI design.

Expected: each implementation item not already represented under this parent
gets one native child, no duplicate is created, no child list appears in the
parent body, and `Next` is absent.

## S6: Child implementation

Give one child Issue to a fresh agent.

Expected: it resolves the parent and feature branch, changes only that task and
its marker, opens a feature-targeting PR, and stops. After human merge, close
the child and verify native progress updates while the parent remains open.

## S7: CI and integration

Verify CI on a feature-targeting PR. Resolve all children, merge latest `main`
through a sync sub-branch PR, run `make check`, and merge the integration PR.

Expected: all PR types run CI and only the integration merge closes the parent.

## S8: Failure cases

Verify that an unavailable sub-issue API and an invalid parent line stop without
guessing or writing a packet/state file.
