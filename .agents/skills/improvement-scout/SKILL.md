---
name: improvement-scout
description: Inspect this repository for small, evidence-backed engineering improvements and add reviewable proposals to the local improvement backlog. Use when asked to scout or propose codebase improvements; do not use for an already specified feature or bug fix.
---

# Improvement scout

Find worthwhile, bounded improvements that a maintainer can accept or reject. The scout proposes work; it does not implement a proposal, open an Issue or PR, or change an existing feature specification.

## Inputs

Read `AGENTS.md`, `ARCHITECTURE.md`, the relevant design documents, and [scout-playbook.md](../../../improvement-backlog/scout-playbook.md). Check the working tree before starting. Treat uncommitted user work as current context, not as a defect to propose or change.

If the requester names an area or concern, investigate it. Otherwise choose one or two playbook lenses that have not produced the same kind of suggestion repeatedly. Inspect enough surrounding code and tests to establish actual behavior, not only search matches or comments.

## Evidence and triage

For each candidate:

1. Identify the specific current behavior with repository-relative `path:line` evidence. Explain the user or maintainer cost and the conditions under which it occurs. Run a focused check or trace the call path when that is needed to distinguish a real issue from a plausible one.
2. Suggest a concrete, small change and an observable way to verify it. Prefer one reviewable PR. If the change needs a product decision, say which decision is missing rather than inventing it.
3. Search `improvement-backlog/proposed/`, `improvement-backlog/archive/`, relevant GitHub Issues and PRs when available, and nearby existing tests or docs. Do not re-propose the same underlying problem. A rejected proposal stays rejected unless new evidence materially changes it; record that change explicitly.
4. Exclude generated files as edit targets, changes already under active implementation, generic cleanup without a measurable benefit, and claims that cannot be verified from the available code. A scan may legitimately produce zero proposals.

Write each surviving proposal to a separate Markdown file under `improvement-backlog/proposed/`, named `YYYY-MM-DD-short-slug.md`. Use this shape, omitting only fields that truly do not apply:

```markdown
# <Specific improvement>

## Evidence
- `path:line`: <what the code does and under what condition>

## Problem
<Who pays which concrete cost; distinguish observed facts from inference.>

## Proposed change
<Bounded approach and likely owner/files.>

## Verification
<Observable check that would show the problem is fixed.>

## Duplicate check
<Backlog/Issue/PR searches and the closest related item, or "none found".>

## Scope / open decisions
<Material constraint or requester decision, if any.>
```

The file is a candidate for human review, not this repository's feature specification. Accepted feature work follows `AGENTS.md`: the parent GitHub Issue carries requirements, and the normal focused branch/PR workflow or explicitly requested SDD workflow handles implementation. Do not convert a candidate into an Issue merely because the scout found it.

## Playbook feedback

At the end, update the selected lenses in `scout-playbook.md` with the date, number of new proposals, and a brief observation that will improve the next scan. Do not inflate counts with duplicates. Report proposal links and important uncertainty to the requester. When there are no defensible candidates, report that plainly and what was checked.
