# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]

**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

<!--
  WHAT THIS DOCUMENT IS: the decisions for building this feature — the deltas
  from the existing sources of truth, the structural choices, and the breakdown
  into implementation units. The requirement itself lives in spec.md; the
  research behind a decision lives in research.md.

  Quality rules for this document: docs/design-docs/plan-quality.md (P-1..P-7).
  The two that bite most often: write a decision or nothing at all (P-1), and
  leave a section out rather than filling it with plausible prose (P-6).

  HOW MUCH OF THIS TEMPLATE TO USE: as much as the change earns (P-7). Only
  `## Summary` and `## Implementation Work` are always present. Every other
  section appears when it carries a decision and is deleted when it does not —
  a one-package change with no new dependency, no structural choice and no gate
  to weigh is a plan of two sections, and that is a complete plan, not a
  shortcut. A change that spans several boundaries or picks between real
  alternatives earns every section, at length.

  PRINCIPLE FOR EVERY SECTION BELOW: do not restate what an existing source of
  truth already defines. In an established repository, link to the canonical
  document (architecture notes, design docs, dependency manifests, API schemas,
  build/test entry points) and write only what this feature adds, changes, or
  leaves open. Duplicated descriptions go stale and bury the feature-specific
  decisions.

  In a new repository, or wherever no canonical source exists yet, record the
  information here as usual — the plan is then the first source of truth, and a
  later change should move it to a durable location and link back.
-->

## Summary

[Extract from feature spec: primary requirement + technical approach from research]

## Technical Context

<!--
  CONDITIONAL — delete this section when this feature inherits everything
  unchanged and has no open question.

  ACTION REQUIRED: Link the canonical definitions instead of copying them, then
  list only what this feature changes or still has to decide. Typical links:
  architecture and dependency direction, dependency manifests, interface
  schemas, and the command entry point used for checks.

  Write out an item below ONLY when one of these holds:
  - this feature changes it (a new dependency, a new storage location, a new
    runtime target)
  - it is a feature-specific constraint the canonical documents do not cover
    (a performance budget, a compatibility window, a scale assumption)
  - it is unresolved — mark it `NEEDS CLARIFICATION` and resolve it in
    research.md

  Do not restate an unchanged language version, dependency list, test runner,
  platform, or project type. If nothing in a category changes, omit it.
-->

**Canonical definitions**: [links to the existing sources this feature inherits]

**Feature-specific context**: [deltas, constraints, and open questions only, or
"None beyond the canonical definitions"]

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

<!--
  CONDITIONAL — keep the gates this change could plausibly violate. When none
  applies, one line saying which rules you checked and that none is at stake is
  the whole section.
-->

[Gates taken from the repository's governance — its architecture notes, its
stated judgement criteria, and its agent guide. Name the rules you checked and
the verdict for each. Where a project keeps a Spec Kit constitution file with
ratified principles, those are the gates instead; an unfilled slot inside such a
file is simply not a gate, and a file that is absent, empty, or nothing but
placeholders carries none.]

## Project Structure

### Documentation (this feature)

List the artifacts this feature actually has. Each one is created only when it
carries feature-specific content (P-2); an artifact with nothing to say is left
out, not filled with invented material.

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — decisions this feature adds [omit if none]
├── data-model.md        # Phase 1 output — entity deltas [omit if none]
├── quickstart.md        # Phase 1 output — validation steps [omit if none]
└── contracts/           # Phase 1 output — interface deltas [omit if none]
```

The `## Implementation Work` section below is the input to `/speckit-plan-to-issues`;
this workflow has no separate tasks stage and produces no `tasks.md`.

### Source Code

<!--
  CONDITIONAL — delete when the change sits inside one existing boundary, adds
  no path, and makes no structural choice.

  ACTION REQUIRED: Name the ownership boundaries this feature touches, the paths
  it adds, and any structural decision worth defending. Do NOT reproduce the
  repository tree — link to the architecture notes for the overall layout.

  Only when the repository has no established layout yet (a new project), lay
  out the directory structure you are choosing here, with real paths rather than
  placeholder options.
-->

**Affected boundaries**: [existing directories or packages this feature changes,
and what each one owns in it]

**New paths**: [paths this feature adds, or "None"]

**Structure decision**: [structural choices and why, or "Follows the existing
layout" with a link]

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |

## Implementation Work

<!--
  ACTION REQUIRED: One `###` subsection per independently reviewable
  implementation unit. `/speckit-plan-to-issues` turns each one into a native
  child Issue — the heading becomes the Issue title, and the three fields below
  are its approved input for writing that Issue's body — and
  `/speckit-implement` then builds one unit per PR working from that Issue.

  What this section settles is the breakdown: what the units are, what each
  covers, what has to land first, and what counts as done. The prose an
  implementer reads is written later, when the Issue is created from these
  units and the artifacts — so keep it short here, and do not draft the Issue.
  - Write the heading as a title that still means something outside this plan;
    it becomes the Issue title verbatim.
  - Name a dependency by the other unit's heading; Issue numbers do not exist
    yet when this is written.
  - Point to the artifact section that specifies the detail (a contract, an
    entity) instead of repeating it here.
  - Between them the units must cover the whole feature, and must not overlap.
  - For a unit that changes a screen, say so in its acceptance: the
    implementation PR owes screenshots and a visual/accessibility review.

  Do not add persistent task IDs and do not create a separate tasks.md.
-->

### [Child Issue title]

**Scope**: [what changes in this unit, and the artifact section that specifies it]

**Dependencies**: [the headings of the units that must land first, or "None"]

**Acceptance**: [observable evidence that this unit is done — a check that
passes, a response that is returned, something visible on screen. "Implemented
correctly" is not evidence (P-5)]
