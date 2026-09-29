# PR body digest

A stage PR adds or changes documents. The reviewer should be able to judge the
stage from the PR body alone, and open the files only to check a detail. So
the body carries a digest: the artifact's key points, copied into fixed tables.

## Rules

- **Copy, do not add.** Every row comes from the artifact in the diff, the
  parent Issue, or GitHub state. A point that is not in the artifact is not in
  the digest. If the digest needs it, the artifact is incomplete; fix that.
- **Link each row to its source.** Use the full GitHub URL of the file on the
  PR's head branch, with the section anchor:
  `https://github.com/<owner>/<repo>/blob/<branch>/specs/<dir>/research.md#r-2-...`.
  Relative links do not work in a PR body.
- **Tables over prose.** One row per decision, unit, screen or risk. A cell
  holds one sentence. Leave a table out when the artifact has nothing for it;
  do not write "None" rows.
- **Short.** The digest fits on one screen for a small stage and stays under
  about 80 lines for a large one. Summarise a long list as its first rows plus
  `and N more in [file](link)`.
- **English**, following
  [writing-style.md](../../../../docs/design-docs/writing-style.md).
- **Keep it current.** When a review round changes the artifact, update the
  digest rows it affects in the same push.

## Common header

Every stage PR starts with this block. It also records why the stage was
selected ([README.md](README.md#selecting-the-stage)).

```markdown
## Summary

<One to three sentences: what this stage decides or delivers.>

Refs #<parent or child>

| Stage | Selected because | Feature |
| --- | --- | --- |
| plan | No feature branch existed | `specs/027-name/` on `feature/027-name` |
```

## `plan` stage

```markdown
## Decisions

| # | Decision | Rejected alternative | Source |
| --- | --- | --- | --- |
| R-1 | <decision heading from research.md or plan.md> | <main rejected option> | [research.md#r-1](...) |

## Implementation units

| # | Unit (child Issue title) | Depends on | Acceptance evidence |
| --- | --- | --- | --- |
| 1 | <### heading under Implementation Work> | <unit # or -> | <observable result> |

## Interface and data changes

| Surface | Change | Source |
| --- | --- | --- |
| `GET /api/...` / table `...` | <what is added or changed> | [contracts/...](...) |

## For the reviewer

- <Open question, risk, or judgement call the reviewer should weigh.>

## Checks

| Command | Result |
| --- | --- |
| `task check-docs` | passed |
```

## `design` stage

```markdown
## Screens

| Screen | Route | Change | Source |
| --- | --- | --- | --- |
| Library | `/` | <what this feature adds> | [ui-design.md#...](...) |

## Key visual decisions

| Element | Treatment | Why | Rejected |
| --- | --- | --- | --- |
| <element> | <tokens, placement> | <reason> | <alternative> |

## States

| State | What the user sees |
| --- | --- |
| Empty | <view> |

## Review criteria

| # | Viewpoint | Criterion | Width |
| --- | --- | --- | --- |
| 1 | Hierarchy | <criterion copied from ui-design.md> | 360 px |

<Wireframes or screenshots from the artifact, when it has them.>

## Checks

| Command | Result |
| --- | --- |
| `task check-docs` | passed |
```

## Implementation PR

Use the repository PR template. Under **Changes**, add the child's acceptance
evidence as a table:

```markdown
| Acceptance evidence (from #<child>) | How it was verified |
| --- | --- |
| <evidence> | <test name, command, or screenshot> |
```

## Integration PR

```markdown
## Summary

<The feature in one to three sentences, from the parent Issue.>

Closes #<parent>

## Delivered

| Child Issue | PR | What it delivered |
| --- | --- | --- |
| #<child> | #<pr> | <one sentence> |

## Checks

| Command | Result |
| --- | --- |
| `task check` | passed |
| `task test-e2e` (ui features) | passed |

## Remaining risks

| Risk | Source | Suggested handling |
| --- | --- | --- |
| <risk> | <review thread or check> | <fix before merge / follow-up / accept> |
```
