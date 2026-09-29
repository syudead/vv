# Plan quality rules: no text written only to fill a slot

These are the rules to follow when writing or changing a Plan. They record, as decision rules for
the artifacts, the policy of Issue #65 "Center the Plan template on references to existing design
and feature-specific differences".

The goal is one thing: **a Plan does not state what was not decided.** Text that exists only
because the template has a slot for it makes readers assume a decision was made. It goes stale
when the source of truth changes, and it buries the real decisions.

A Plan holds only these four things. Items 1 and 4 are always present. Items 2 and 3 appear only
when there is a decision of that kind (P-7).

1. A summary of what is built (the requirement itself is in the parent Issue)
2. References to existing sources of truth, and the feature-specific differences from them
3. Ownership boundaries of what changes, and structural decisions
4. The breakdown into implementation units (the units of child Issues and implementation PRs)

## Rules

### P-1: A Plan states decisions; research goes to research.md

Each Plan section states only what was chosen and why. What was investigated and how far it got
goes in `research.md`. A section without a decision is deleted, or ends with the single line
"Not applicable".

Text that "is in the Plan but decides nothing when read" is a leftover of research, not a
decision.

### P-2: Do not create an artifact that has nothing to say

Create `research.md`, `data-model.md`, `contracts/` and `quickstart.md` only when feature-specific
content exists. When it does not, do not create the artifact; write one line saying so in the
Plan.

Do not invent concepts to fill a slot. Before creating an artifact, confirm that the content it
would hold exists. A feature that adds no entity has no `data-model.md`, and a feature that changes
no externally visible interface has no `contracts/`. That is the correct state.

When a revision of an existing Plan leaves an artifact with no feature-specific content, delete
that file in the same change. Left in place, it remains an input to later stages. If its content is
worth keeping, move it to the source of truth and link to it first, then delete the file. Git keeps
the history.

### P-3: Link to existing sources of truth and write only the difference

Point to the source of truth with a link for the tech stack, dependency direction, common commands
and repository-wide structure. The Plan artifacts state only what this feature adds or changes,
and the feature-specific constraints that the source of truth does not cover.

When no source of truth exists (for example, in a new repository), the Plan may state it. A later
change then moves it to a permanent home, and the Plan links there.

### P-4: Give every decision its rejected alternatives and the reasons

Each decision lists, one line each, the options considered and not taken, and why. Something for
which this cannot be written is not a decision; it only passed a default through. In that case,
do not write it as a decision; drop it under P-1.

Write the reasons from the circumstances of this feature and this repository, not "because it is
generally considered good".

This rule covers technical and structural decisions. Implementation units in
`## Implementation Work` are a division of work, not decisions, so they need no alternatives
(follow P-5). When the way of dividing the work itself involved a decision, write it as a decision
under P-4.

### P-5: An implementation unit fits in one PR and has observable acceptance evidence (division of labor between the Plan and child Issue creation)

Each unit in `## Implementation Work` states its scope, dependencies and acceptance evidence.
Acceptance evidence is an observable fact: a check passes, a given response is returned, something
is visible on screen. "It is implemented correctly" is not evidence.

Split a unit that does not fit in one PR. Do not assign persistent task IDs, and do not create
`tasks.md`. The Plan owns the breakdown of implementation work, and `sdd-plan-to-issues` creates the
child Issues from it.

The Plan fixes three things per unit: scope, dependencies and acceptance evidence. It does not need
to write complete instructions for the implementer. `sdd-plan-to-issues` writes each child Issue
body from these three things and the artifacts. The Plan is where decisions are fixed; child Issue
creation expands those decisions for the implementer.

For that division of labor to work, the Plan side meets these conditions.

- Each heading is a title that makes sense outside the Plan (it becomes the child Issue title
  as is).
- A dependency points to the other unit by its heading (Issue numbers do not exist yet).
- When an artifact holds the details, the unit points to that section.
- The units together cover the whole feature and do not overlap.

When child Issue creation needs a new decision, it is not made there; it goes back to the Plan. The
Plan is a document a human approved, and child Issues are standing instructions to implementers.

A unit that changes the UI says so in its acceptance evidence. The implementation checks the
visuals and the interaction.

### P-6: End an item that does not apply with "Not applicable"

When a template item has no matching content, do not fill it with a plausible sentence. Drop the
item, or write "Not applicable" and stop.

Check that you can say where in the parent Issue or existing documents the requirement behind the
sentence you are writing comes from. If you cannot, that is evidence that the item has no content.

### P-7: The size of a Plan follows the number of decisions

Fit the shape of the Plan to the size of the change. Only two sections are always present,
`## Summary` and `## Implementation Work`. Other sections appear when they carry a decision and
are deleted when they do not.

A change contained in one package, with no change in dependencies or structure and no gate to
consider, has a Plan of two sections. That is not an abbreviated Plan; it is a complete one.
Conversely, a change that crosses several boundaries, or that chooses among real options, fills
every section to the length it needs.

Do not try to keep the number of sections or artifacts constant. Inventing decisions to fill
sections lowers quality as much as skipping decisions does.
