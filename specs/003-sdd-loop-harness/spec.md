# Feature Specification: Issue handoff SDD

**Created**: 2026-09-13

**Revised**: 2026-09-19

**Status**: Independent review required

**Parent Issue**: #46

## Original requirements

The following statements are preserved verbatim from the user conversation on
2026-09-19. Speaker: user.

- **RQ-001**: 「claudeじゃなくて任意のコーディングエージェントにしたいんだが
  ルーティーン依存、PRトリガーも消す」
- **RQ-002**: 「feature branchとサブbranch運用を捨てるなよ
  あと未決定事項を放置しまたママ計画を書くな」
- **RQ-003**: 「変な番号の紐づきとか作るなよ　せっかく異存なくしてるのに」
- **RQ-004**: 「親issueはspec / planとかの状況は書く必要あるけど、prとか子タスクとかその情報を書く必要はあるのか？」
- **RQ-005**: 「issueとprとの紐づけは計画上どうなってる？」
- **RQ-006**:

  > - issueを切る
  > - specify でissue をspec.mdに
  > - spec.md prマージ
  > - githubトリガーか何かでissueの本文をアップデートしてplan.mdと紐づけ？
  > - ーーーここで1回切れる
  > - issueを指定してplanでplan.mdに
  > - plan.md prマージ

- **RQ-007**: 「余計な制御しなくていいだろ」
- **RQ-008**: 「そのSDDうんチャラとかラベルとか必要なの？」

## Problem

The previous SDD harness could derive a stage from repository artifacts, but
could proceed only when a Claude Routine reacted to a labelled PR merge. It
also restored context from Claude-specific session input and branch names.
Changing from PR-oriented tracking to Issues did not remove those execution
dependencies.

The repository needs a handoff that any coding agent can use from a supplied
Issue without inheriting a previous conversation, agent session, trigger, or
branch naming convention.

## User scenarios

### US1: Continue one stage with another agent (P1)

A maintainer gives a parent Issue to a coding agent. The agent follows native
Issue/PR relationships to the long-lived feature branch, reads merged
artifacts, performs exactly the requested missing stage, opens a PR to the
feature branch, and stops. A later stage can be run by a different agent.

**Independent test**: Specify and Plan complete in separate sessions using
different agent implementations and arbitrary branch names.

### US2: Implement one child task (P1)

Tasks become native sub-issues of the parent. A maintainer gives one child to
an agent, which finds the parent and feature branch, implements only that task,
and opens a feature-branch PR. The child closes after that PR is merged; the
parent remains open until the integration PR reaches `main`.

**Independent test**: Starting from only a child Issue, a fresh agent opens the
correct PR and changes only its matching task marker.

### US3: Inspect state without an agent service (P2)

A maintainer runs a local command with an explicit feature directory and can
see the next artifact stage or stale dependency. The command does not access
GitHub and does not read branch naming, session files, or agent state.

**Independent test**: Fixture repositories return the same stage on arbitrary
branch names and return an earlier stage after its input changes.

## Functional requirements

- **FR-001**: Every run starts from an explicitly supplied parent Issue or
  native sub-issue.
- **FR-002**: Standard flow is Specify, Plan, Tasks, Tasks-to-sub-issues. A
  parent with the existing `ui` label inserts Design between Plan and Tasks.
- **FR-003**: One run performs one stage, opens or updates one PR, and stops.
- **FR-004**: Stage and implementation PRs target a long-lived feature branch;
  only the integration PR targets `main`.
- **FR-005**: Branch names are arbitrary and never select an Issue, feature,
  stage, or retry.
- **FR-006**: Issue numbers and feature-directory numbers are independent.
  The only repository mapping is one exact `**Parent Issue**: #NNN` line in
  `spec.md`.
- **FR-007**: The parent body contains requirements and the Spec, Plan,
  optional Design, Tasks, and Next summary. It does not copy PR, branch,
  child-Issue, retry, agent, or session data.
- **FR-008**: GitHub timeline references, PR head/base, the parent-closing
  integration PR, and native sub-issues are the relationship sources of truth.
- **FR-009**: No Routine, merge-triggered agent, schedule, SDD automation
  label, committed packet, result JSON, or session state is required.
- **FR-010**: The local state command reads only an explicit feature directory
  and Git history and reports stale downstream artifacts.
- **FR-011**: Tasks-to-sub-issues deduplicates task IDs only within the supplied
  parent's native sub-issues and fails before writing when that API is absent.
- **FR-012**: Task IDs are immutable after child creation. New work receives a
  larger ID; cancelled work retains its ID and closes as not planned.
- **FR-013**: Implementation PR merge closes its child manually as completed.
  Integration PR merge closes only the parent through `Closes`.
- **FR-014**: All PRs run CI regardless of base branch name.
- **FR-015**: Before integration, latest `main` enters the feature branch
  through a reviewed sub-branch PR; the feature branch is not force-pushed.
- **FR-016**: Repository GitHub access is performed by each agent's native
  adapter. No shared repository command owns authentication or API transport.

## Edge cases

- Zero or multiple integration PRs after Spec completion stop the run.
- Zero open Spec PRs before Spec completion starts Specify; one resumes that
  PR; multiple stop the run.
- Parent SDD summary and merged artifacts disagree: stop until a maintainer
  updates the Issue.
- A feature branch pushed before its Spec PR cannot be recovered from standard
  GitHub relationships; a maintainer removes it and retries.
- Revised Spec, Plan, or Design pauses implementation until downstream
  artifacts and sub-issues are reconciled.

## Success criteria

- **SC-001**: Specify and Plan can be completed by different coding agents
  without sharing conversation or session state.
- **SC-002**: Every fixture for missing and stale artifacts passes without
  network access and without inspecting the current branch name.
- **SC-003**: Parent Issues contain no replicated PR or child-Issue list.
- **SC-004**: Every stage and implementation PR targets the long-lived feature
  branch from an arbitrary-name sub-branch; only the integration PR targets
  `main`.
- **SC-005**: Merging an implementation PR updates native sub-issue progress,
  while only integration to `main` closes the parent.
- **SC-006**: Removing Claude Routine and `/sdd-next` leaves `make check`
  passing and leaves no executable reference to the retired controller.

## Requirement traceability

| Success criterion | Original requirements |
| --- | --- |
| SC-001 | RQ-001, RQ-006 |
| SC-002 | RQ-001, RQ-003, RQ-007 |
| SC-003 | RQ-004 |
| SC-004 | RQ-002, RQ-005 |
| SC-005 | RQ-004, RQ-005 |
| SC-006 | RQ-001, RQ-007, RQ-008 |
