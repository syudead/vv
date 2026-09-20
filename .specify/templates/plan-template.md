# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]

**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

<!--
  WHAT THIS DOCUMENT IS: the decisions for building this feature — the deltas
  from the existing sources of truth, the structural choices, and the breakdown
  into implementation units. The requirement itself lives in spec.md; the
  research behind a decision lives in research.md.

  Quality rules for this document: docs/design-docs/plan-quality.md (P-1..P-6).
  The two that bite most often: write a decision or nothing at all (P-1), and
  leave a section out rather than filling it with plausible prose (P-6).

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

[Gates determined based on constitution file. If the project has no constitution
file, check against the repository's agent guide and architecture notes and say
which rules were checked.]

## Project Structure

### Documentation (this feature)

List the artifacts this feature actually has. Each one is created only when it
carries feature-specific content (P-2); an artifact with nothing to say is left
out, not filled with invented material.

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — decisions this feature adds
├── data-model.md        # Phase 1 output — entity deltas [omit if none]
├── quickstart.md        # Phase 1 output — validation steps [omit if none]
└── contracts/           # Phase 1 output — interface deltas [omit if none]
```

The `## Implementation Work` section below is the input to `/speckit-plan-to-issues`;
this workflow has no separate tasks stage and produces no `tasks.md`.

### Source Code

<!--
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
  implementation unit; each becomes one child Issue and one implementation PR.
  State scope, dependencies, and observable acceptance evidence. Do not add
  persistent task IDs and do not create a separate tasks.md.
-->

### [Child Issue title]

**Scope**: [what changes in this unit]

**Dependencies**: [other units this one needs, or "None"]

**Acceptance**: [observable evidence that this unit is done — a check that
passes, a response that is returned, something visible on screen. "Implemented
correctly" is not evidence (P-5)]
