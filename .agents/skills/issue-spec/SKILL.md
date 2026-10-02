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
     wins;
   - the feature changes the UI and the requester has not said whether the
     Issue gets the `ui` label.

   Otherwise go straight to step 4. Tell the requester in one line which way
   you went and why.
3. **Discuss until the requirement is settled.** Do not file the Issue
   mid-discussion. Each round:
   - state your current understanding in two or three lines — what the user
     gets, and what is in and out;
   - ask at most three questions, and never ask what step 1 can answer or
     what the requester has already answered in this discussion;
   - fold the answers in and check the conditions again.

   Settle the purpose before the shape:
   - **Purpose first, open-ended.** While the problem, who will use the
     feature, or the situation it is used in is unclear, ask only about
     those, as open questions without options. A motive is not in the code
     or the docs, so do not infer it, and do not offer choices of
     implementation form before it is known: the options would frame the
     answer. When an answer changes the purpose, drop or revisit what was
     decided on the old one.
   - **Then choices.** Once the purpose is settled, ask about the remaining
     forks with concrete options and your recommendation first (use the
     question tool when one is available). One decision per question; do
     not combine unrelated decisions into one set of options.
   - **Ask about consequences, not details.** Spend questions on what changes
     what the user gets and is costly to undo, not on behaviour plan or
     design can settle and later change cheaply.
   - **State costs as they are.** When a trade-off rests on a fact about a
     platform or a tool, check it and give its real size; an overstated
     hurdle steers the answer.
   - **Answer how-questions briefly.** When the requester asks whether or how
     something can be built, answer in a few lines — enough to show the
     requirement is feasible — and keep the choice of how for plan.

   End when no condition holds, or when the requester explicitly agrees to
   proceed with a stated ambiguity — then record it in the Issue as such.
   Before filing, show a short summary — what the user gets, what is out of
   scope, and every decision you made yourself rather than the requester —
   and file only after the requester agrees.
4. **Write and file.** Write the body below and create (or edit) the Issue
   directly; do not show a full draft first. Corrections are made by editing
   the Issue afterwards. In the reply, list the decisions you made yourself
   that the summary did not cover.

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
- **Labels are the requester's call, except `spec`.** Create the Issue with
  the `spec` label, which marks it as a parent (specification) Issue, and add
  only the other labels the requester names. Do not copy labels from other
  Issues. `ui` is the only label with workflow meaning — it decides whether the
  `design` stage runs — so when the Issue changes the UI and the requester has
  not said, ask before filing (step 2 makes this a discussion condition). The
  `sdd` label is retired; do not add it.

## Preflight

Needs Issue read and write access. Stop before writing when it is missing.
