# Plan quality rules: no text written only to fill a slot

Follow these rules when writing or changing a Plan. They turn the policy of
Issue #65 ("Plan templates centred on references to existing design and
feature-specific differences") into decision rules for the artifacts.

The aim: **a Plan states nothing that was not decided.** Text that exists only
because the template has a slot for it makes readers believe a decision was
made, goes stale when the source of truth changes, and buries the real
decisions.

A Plan can contain only these four things. Items 1 and 4 are always present;
items 2 and 3 appear only when there is a decision of that kind (P-7).

1. A summary of what is being built (the requirement itself is in the parent
   Issue)
2. References to existing sources of truth, and the feature-specific
   differences from them
3. Ownership boundaries of what changes, and structural decisions
4. The breakdown into units of implementation work (child Issues and
   implementation PRs)

## Rules

### P-1: A Plan records decisions; research goes in research.md

Each Plan section states only what was chosen and why. What was investigated
and how far it got goes in `research.md`. A section with no decision is deleted
or ends with a single line, "Not applicable".

Text that is in the Plan but decides nothing when read is not a decision; it is
leftover research.

### P-2: Do not create an artifact with nothing to say

Create `research.md`, `data-model.md`, `contracts/` and `quickstart.md` only
when feature-specific content exists. When it does not, do not create the
artifact; say so in one line in the Plan.

Do not invent concepts to fill a slot. Before creating an artifact, confirm that
its content exists. A feature that adds no entity has no `data-model.md`, and a
feature that changes no externally visible interface has no `contracts/`. That
is the correct state.

When a revision of an existing Plan leaves an artifact without feature-specific
content, delete the file in the same change. Left in place, it remains an input
to later stages. If its content is worth keeping, move it to the source of truth
and link it first, then delete. Git keeps the history.

### P-3: Link to existing sources of truth and write only the differences

Show the tech stack, dependency direction, common commands and repository-wide
structure by linking to their sources of truth. A Plan's artifacts contain only
what this feature adds or changes, and feature-specific constraints the sources
of truth do not cover.

When no source of truth exists (a new repository, for example), the Plan may
state it. A later change then moves it to a permanent location, and the Plan
links there.

### P-4: Each decision names the rejected alternatives and why

Each decision comes with the alternatives considered and not taken, one line
each, with the reason. Anything for which this cannot be written is not a
decision; it is a default passed through unchanged. Do not record it as a
decision; drop it under P-1.

Give reasons specific to this feature and this repository, not "because it is
generally considered good".

This rule covers technical and structural decisions. The units in
`## Implementation Work` are a division of work, not decisions, so they need no
alternatives (follow P-5). When the way of dividing the work itself involved a
judgement, record it as a decision under P-4.

### P-5: A unit of implementation fits in one PR and has observable acceptance evidence (division of labour between the Plan and child Issue creation)

Each unit in `## Implementation Work` states its scope, dependencies and
acceptance evidence. Acceptance evidence is an observable fact (a check passes,
a specified response comes back, something is visible on screen). "Implemented
correctly" is not evidence.

Split a unit that is too large for one PR. Do not assign persistent task IDs and
do not create `tasks.md`. The Plan holds the breakdown of implementation work,
and `sdd-plan-to-issues` creates child Issues from it.

For each unit the Plan settles three things: scope, dependencies and acceptance
evidence. It does not need to write out the full explanation for implementers.
`sdd-plan-to-issues` drafts each child Issue body from those three things and
the artifacts. The Plan is where decisions are settled; child Issue creation is
where they are expanded for implementers.

For that division of labour to work, the Plan meets the following. Each heading
is a title that makes sense outside the Plan (it becomes the child Issue title
as is). Dependencies refer to the other unit's heading (no Issue numbers exist
yet). When an artifact holds the details, point to its section. The set of units
covers the whole feature with no overlap.

When child Issue creation finds that a new decision is needed, it does not make
it there; it goes back to the Plan. The Plan is a document people approved, and
child Issues are lasting instructions to implementers.

A unit that changes a screen says so in its acceptance evidence. The
implementation then checks the visuals and the interaction.

### P-6: An item that does not apply ends with "Not applicable"

When a template item has no matching content, do not fill it with plausible
text. Drop the item or write "Not applicable" and stop.

Check whether you can say where in the parent Issue or an existing document the
requirement behind the sentence you are writing comes from. If you cannot, that
is evidence the item has no content.

### P-7: The amount of planning follows the amount of judgement

Match the shape of the Plan to the size of the change. Only `## Summary` and
`## Implementation Work` are always present; other sections appear when they
carry a judgement and are deleted when they do not.

A Plan for a change that stays within one package, changes no dependency or
structure, and has no gate to consider ends after two sections. That is not an
abbreviated Plan; it is a complete one. Conversely, a change that crosses several
boundaries or chooses among real options fills every section to the length it
needs.

Do not try to keep the number of sections or artifacts constant. Inventing
judgements to fill slots lowers quality as much as omitting judgements does.
