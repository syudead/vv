---
name: issue-spec
description: Write or revise the parent GitHub Issue that carries a feature's requirements and acceptance criteria. Use at the start of an Issue-driven SDD feature, and whenever the requirement itself changes, before plan. Decides first whether the request needs a discussion with the requester, settles the requirement, then files.
---

# Issue spec

The parent Issue is this repository's specification. Nothing downstream
re-derives the requirement from anywhere else, so what is not in the Issue is
not in the feature.

## Output

Create or update the parent Issue. Do not create a branch, change repository
files, or open a pull request. Stop when the Issue is written, and reply with
its link and the 概要 line.

## Steps

1. **Investigate before asking.** Read the request, then answer what you can
   yourself in Q-6's order: the repository's design documents and specs, the
   existing screens and code, the request itself, and the form comparable
   current products have converged on. Search open and closed Issues for a
   duplicate or a related feature; when one exists, say so and ask whether to
   revise it instead.
2. **Decide whether to discuss first (壁打ち).** Discuss when any of these
   holds after step 1:
   - what the user gets, or the problem being solved, is not stated;
   - the boundary between this feature and 対象外 is open;
   - a question remains that meets Q-6 (it changes what the user gets, the
     sources above do not settle it, and getting it wrong is costly);
   - the request conflicts with existing behaviour and does not say which
     wins.

   Otherwise go straight to step 4. Tell the requester in one line which way
   you went and why.
3. **Discuss until the requirement is settled.** Do not file the Issue
   mid-discussion. Each round:
   - state your current understanding in two or three lines — what the user
     gets, and what is in and out;
   - ask at most three questions, each with concrete options and your
     recommendation first (use the question tool when one is available);
     never ask what step 1 can answer;
   - fold the answers in and check the four conditions again.

   End when no condition holds, or when the requester explicitly agrees to
   proceed with a stated ambiguity — then record it in the Issue as such.
4. **Write and file.** Write the body below and create (or edit) the Issue
   directly; do not show a draft first. Corrections are made by editing the
   Issue afterwards.

## Body

```markdown
## 概要
## 背景
## 要件
## UI品質   <- `ui` ラベルの Issue だけ
## 受け入れ条件
## Edge Cases
## 対象外
```

- **概要** is one to three lines: who can now do what, and what changes for
  them. A reader who stops here knows the feature.
- **背景** is the current state and why it falls short, in a few lines. Link
  the spec or design doc of the existing behaviour instead of retelling it.

Leave out a section this feature has nothing for. Do not invent content to
fill a heading. Do not add a workflow-progress section such as `## SDD`;
progress is read from the feature branch and the native sub-issues.

Write 要件 and 受け入れ条件 as numbered lists. Downstream stages cite an item as
`要件 3` or `受け入れ条件 5`, and a plan, a child Issue, a checklist, or a review
has nothing to point at otherwise. The numbers are handles, not a traceability
matrix: do not add a table mapping one list to the other.

## Keep it short

The Issue says what the user gets; plan and design decide how. A long Issue
buries the requirement and fixes choices that belong downstream.

- **One item, one behaviour, one or two sentences.** Split an item that needs
  more; merge items that say the same thing from two sides.
- **No implementation detail** unless the requester stated it: no type, field,
  file, endpoint, component or CSS names, no pixel sizes or timings. Write
  "サムネイル下端の細い帯", not "高さ 1/5 の帯"; plan and design pick the
  numbers.
- **受け入れ条件 does not restate 要件.** Write only what someone checks, and
  leave out a criterion that would repeat its 要件 word for word.
- **Edge Cases are one line each.** Group cases with the same outcome into one
  line.
- **UI品質 is one line per viewpoint.** The design stage elaborates; the Issue
  only fixes the judgement.
- **Prefer a picture to a paragraph.** When prose would describe states, a
  flow, or before/after behaviour, use a table or a Mermaid diagram (`stateDiagram-v2`,
  `flowchart`) instead. Add one only when it replaces text, not alongside it.

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
  rather than recording the change somewhere downstream. Run step 2 on the
  change itself.
- **Accessibility is not a requirement here.** Do not write screen-reader,
  ARIA, reading-name, contrast-ratio or other accessibility requirements,
  acceptance criteria or edge cases unless the requester asks for them.
- **Labels are the requester's call.** Create the Issue with no labels, and
  add only the ones the requester names. Do not copy labels from other Issues.
  `ui` is the only label with workflow meaning — it decides whether the
  `design` stage runs — so when the Issue changes the UI and the requester has
  not said, ask instead of setting it (during the discussion when there is
  one). The `sdd` label is retired; do not add it.

## Preflight

Needs Issue read and write access. Stop before writing when it is missing.
