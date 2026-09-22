---
name: "speckit-plan"
description: "Execute the implementation planning workflow using the plan template to generate design artifacts."
metadata:
  author: "github-spec-kit"
  source: "templates/commands/plan.md"
---

## Repository issue handoff

When this command is invoked for a GitHub parent Issue, read and follow
`.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/plan.md` first. Those
files own feature-branch discovery, the one-stage boundary, and PR behavior.
Name the feature directory explicitly; do not infer it from a branch name.

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## What a plan is

A plan records the **decisions** for building one feature: the deltas from the
repository's existing sources of truth, the structural choices, and the
breakdown into implementation units. It is not a survey of the system and not a
record of what was investigated.

Before writing any artifact, read
[docs/design-docs/plan-quality.md](../../../docs/design-docs/plan-quality.md).
It defines P-1..P-8, the rules every artifact this command produces must
satisfy. The three that decide whether the output is usable:

- **P-1** — write a decision, or write nothing. Investigation belongs in
  `research.md`; a section with no decision is dropped.
- **P-2** — create an artifact only when it has feature-specific content. Never
  invent concepts to fill a template slot.
- **P-6** — a template item that does not apply is left out, not filled with
  plausible prose.
- **P-7** — the plan is as long as the change earns. `## Summary` and
  `## Implementation Work` are always there; every other section appears only
  when it carries a decision.

## Pre-Execution Checks

Dispatch the `hooks.before_plan` hooks exactly as
[references/extension-hooks.md](references/extension-hooks.md) describes, then
continue to the Outline. If `.specify/extensions.yml` does not exist, skip
silently.

## Outline

1. **Setup**: The feature directory is given to you. The plan is `<feature-dir>/plan.md`, and the other artifacts sit beside it. Copy [`.specify/templates/plan-template.md`](../../../.specify/templates/plan-template.md) to `<feature-dir>/plan.md` when no plan exists yet. Run no script for this.

2. **Load context**: Read the parent Issue — it is the specification. Then read this repository's governance, which is where the gates come from — [ARCHITECTURE.md](../../../ARCHITECTURE.md) for boundaries and dependency direction, [docs/design-docs/core-beliefs.md](../../../docs/design-docs/core-beliefs.md) for the judgement criteria, and [AGENTS.md](../../../AGENTS.md) for the working agreements. Those documents are the source of truth, and the plan names which of their rules it checked.

3. **Locate the canonical definitions**: Before writing anything, find the
   existing sources of truth for this repository — architecture notes, design
   docs, dependency manifests, interface schemas, and the entry point used for
   checks. Every artifact below links to them instead of restating them. When no
   canonical source exists for something (a new repository, or a gap), record it
   in the plan and say so.

4. **Decide the shape of the plan**: Before filling anything, judge what this
   change actually involves — one boundary or several, a new dependency or
   none, a real choice between alternatives or a single obvious way. Keep the
   sections that will hold a decision and delete the rest from the copied
   template. A small change legitimately ends with `## Summary` and
   `## Implementation Work` alone (P-7).

5. **Execute plan workflow**: Follow the structure in IMPL_PLAN template to:
   - Fill Technical Context with links to the canonical definitions plus only
     the feature-specific deltas, constraints, and unknowns (mark unknowns as
     "NEEDS CLARIFICATION")
   - Fill Constitution Check section from that governance
   - Evaluate gates (ERROR if violations unjustified)
   - Fill Project Structure with the affected ownership boundaries, new paths,
     and structural decisions — never a repository-wide tree
   - Phase 0: Resolve every NEEDS CLARIFICATION; write research.md if this
     feature adds decisions of its own
   - Phase 1: Write whichever of data-model.md, contracts/, and quickstart.md
     carry feature-specific content
   - Re-evaluate Constitution Check post-design

   Phase 0 and Phase 1 create an artifact only when it has something of its own
   to say (P-2). Name the ones you are not creating, and why, in `plan.md`.

6. **Reconcile the artifacts on disk**: This command also revises existing
   plans, and a previous run's files stay where they are. When an
   artifact no longer carries feature-specific content, delete it in this same
   change so the directory matches the list in `plan.md` — a stale file stays an
   input to later stages. If it still holds something worth keeping, move that
   into the canonical document first and link to it, then delete. Git keeps the
   history, so deleting loses nothing.

## Mandatory Post-Execution Hooks

**You MUST complete this section before reporting completion to the user.**

Dispatch the `hooks.after_plan` hooks exactly as
[references/extension-hooks.md](references/extension-hooks.md) describes. If
`.specify/extensions.yml` does not exist, or no hooks are registered under that
key, skip to the Completion Report.

## Completion Report

Command ends after Phase 1 design. Report branch, IMPL_PLAN path, and generated artifacts.

## Phases

### Phase 0: Outline & Research

1. **Extract unknowns from Technical Context** above:
   - For each NEEDS CLARIFICATION → research task
   - For each dependency **this feature introduces or changes** → best practices task
   - For each integration → patterns task
   - Skip research on choices the repository has already settled; link to the
     document that settles them instead

2. **Generate and dispatch research agents**:

   ```text
   For each unknown in Technical Context:
     Task: "Research {unknown} for {feature context}"
   For each technology choice:
     Task: "Find best practices for {tech} in {domain}"
   ```

3. **Consolidate findings** in `research.md`. Create the file only when this
   feature has unknowns to resolve or decisions of its own to record; when every
   relevant choice is already settled by a canonical document, skip it and say so
   in one line in `plan.md` (P-2). Use the format:
   - Decision: [what was chosen]
   - Rationale: [why chosen]
   - Alternatives considered: [what else evaluated]

   Open `research.md` with a link to the inherited technology decisions, then
   record only the decisions this feature adds. Do not re-derive the existing
   stack.

**Output**: all NEEDS CLARIFICATION resolved; research.md when this feature has
decisions of its own, otherwise a one-line note in `plan.md` saying none were
needed

### Phase 1: Design & Contracts

**Prerequisites:** Phase 0 complete — `research.md` written, or established that
this feature needs none

Each artifact below is produced **only if it has feature-specific content**
(P-2). Decide that first, for each one: if this feature adds no entity, exposes
no interface, or needs no validation steps beyond the repository's existing
checks, do not create that file — say so in one line in `plan.md` and move on.
An artifact that restates the existing model, or that invents concepts to have
something to say, is worse than an absent one.

1. **Extract entities from the parent Issue** → `data-model.md`:
   - Entity name, fields, relationships
   - Validation rules from requirements
   - State transitions if applicable
   - Write only the entities this feature adds and the fields it changes on
     existing ones; say that the rest of the model is unchanged rather than
     restating it

2. **Define interface contracts** (if project has external interfaces) → `/contracts/`:
   - Identify what interfaces the project exposes to users or other systems
   - Document the contract format appropriate for the project type
   - Examples: public APIs for libraries, command schemas for CLI tools, endpoints for web services, grammars for parsers, UI contracts for applications
   - Where a machine-readable schema is the source of truth, name it and
     describe only the endpoints, payloads, and errors this feature adds or
     changes
   - Skip if project is purely internal (build scripts, one-off tools, etc.)

3. **Create quickstart validation guide** → `quickstart.md` (skip when the
   repository's existing checks already validate this feature and there is
   nothing feature-specific to run or observe):
   - Document runnable validation scenarios that prove the feature works end-to-end
   - Link the repository's existing setup and check commands instead of
     re-documenting them; spell out only steps specific to this feature
   - Include prerequisites, setup commands, test/run commands, and expected outcomes
   - Use links or references to contracts and data model details instead of duplicating them
   - Do not include full implementation code, model/service/controller bodies, migrations, or complete test suites
   - Keep this artifact as a validation/run guide; detailed implementation work belongs in the implementation-work section of `plan.md`

4. **Add implementation work to `plan.md`**:
   - Add one `## Implementation Work` section
   - Add one `### <child Issue title>` subsection per independently reviewable implementation unit
   - For each unit, state its scope, dependencies, and observable acceptance evidence
   - Acceptance evidence is something observable — a check that passes, a
     response that is returned, something visible on screen. "Implemented
     correctly" is not evidence (P-5)
   - The heading becomes the child Issue's title verbatim, so write one that
     still means something outside this plan. The three fields are the approved
     input `/speckit-plan-to-issues` writes the Issue body from — not a draft of
     that body, so keep them short: name dependencies by the other unit's
     heading, and point to the artifact section holding the detail rather than
     copying it
   - The units together must cover the whole feature without overlapping
   - Keep units small enough for one implementation PR
   - Do not add persistent task IDs or create a separate `tasks.md`

**Output**: plan.md with implementation work, plus whichever of data-model.md,
/contracts/*, and quickstart.md carry feature-specific content

## Key rules

- Use absolute paths for filesystem operations; use project-relative paths for references in documentation
- Follow [docs/design-docs/plan-quality.md](../../../docs/design-docs/plan-quality.md)
  (P-1..P-8) for every artifact this command writes
- Link the canonical definition rather than copying it; a plan artifact holds
  what is specific to this feature, plus whatever has no canonical home yet
- Every artifact must let a reader reach the canonical sources it relies on
- Record a decision with the alternative you rejected and why (P-4). If you
  cannot name one, it is a default you passed through, not a decision — leave
  it out
- ERROR on gate failures or unresolved clarifications

## Done When

- [ ] Plan workflow executed and design artifacts generated
- [ ] Artifacts link to the canonical definitions and restate none of them
- [ ] Every artifact produced carries feature-specific content; none was created
      to fill a slot, and no template item was answered with invented prose
- [ ] The sections kept in plan.md are the ones that hold a decision; the rest
      were deleted rather than filled (P-7)
- [ ] Each decision names the alternative it rejected
- [ ] Feature-specific decisions, contract deltas, data deltas, and the
      implementation-work units are present in the artifacts
- [ ] Extension hooks dispatched or skipped according to the rules in Mandatory Post-Execution Hooks above
- [ ] Completion reported to user with branch, plan path, and generated artifacts
