---
name: issue-spec
description: Write or revise the parent GitHub Issue that carries a feature's requirements and acceptance criteria. Use at the start of an Issue-driven SDD feature, and whenever the requirement itself changes, before plan.
---

# Issue spec

The parent Issue is this repository's specification. Nothing downstream
re-derives the requirement from anywhere else, so what is not in the Issue is
not in the feature.

## Output

Create or update the parent Issue. Do not create a branch, change repository
files, or open a pull request. Stop when the Issue is written.

## Body

```markdown
## 背景
## 目的
## 要件
## UI品質とアクセシビリティ   <- `ui` ラベルの Issue だけ
## 受け入れ条件
## Edge Cases
## 対象外

## SDD

- [ ] Plan
- [ ] Design                  <- `ui` ラベルの Issue だけ
- Next: `plan`
```

Leave out a section this feature has nothing for. Do not invent content to
fill a heading.

Write 要件 and 受け入れ条件 as numbered lists. Downstream stages cite an item as
`要件 3` or `受け入れ条件 5`, and a plan, a child Issue, a checklist, or a review
has nothing to point at otherwise. The numbers are handles, not a traceability
matrix: do not add a table mapping one list to the other.

## Rules

Follow [docs/product-specs/spec-quality.md](../../../docs/product-specs/spec-quality.md).
Q-3 through Q-5 govern the UI section, Q-6 governs what becomes a question, and
Q-7 keeps implementation difficulty out of the requirement. The points below are
what this stage adds.

- **受け入れ条件 is observable.** Write what someone can watch happen. A number
  belongs there only when this repository can actually measure it — a criterion
  phrased around test participants or a user study will never be run here, and
  is a defect, not a strong requirement.
- **Edge Cases carry the failures.** Boundaries, partial failure, concurrent
  use, and cleanup after the user leaves. This is the section a plan cannot
  recover on its own.
- **対象外 is a decision, not a disclaimer.** List only what a reader would
  otherwise expect to be included.
- **Name the existing behaviour this replaces.** When the feature changes
  something the product already does, say so in 要件.
- **Revising is the same stage.** When the requirement changes, edit this Issue
  rather than recording the change somewhere downstream.

## Preflight

Needs Issue read and write access. Stop before writing when it is missing.
