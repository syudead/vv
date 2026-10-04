# Specification quality rules: no degraded requirements

Follow these rules when writing or changing a specification. The specification
lives in the parent Issue, and for UI, `ui-design.md` carries it forward.

The aim: **do not convert a vague but essential user requirement into only
discrete conditions that are easy to check mechanically.** Catch, at the
specification stage, a specification that would not produce what the user wants
even if implemented faithfully.

Rule numbers stay as they were when introduced. Q-1 (keep the requirement
quoted) and Q-2 (map acceptance criteria to the requirement) were retired: once
the parent Issue itself became the source of truth for the requirement, they no
longer had anything to apply to.

## Terms

| Term | Meaning |
| --- | --- |
| **Requester** | The person who asked for this feature. The one to ask questions, and the one running the process |
| **User** | The person who uses the finished product. The subject of the Issue's `要件` and `受け入れ条件`, not someone to ask questions |

"The answer changes what the user gets" means it changes the experience of the
people who will use this feature after it ships. The requester makes that call.

## Rules

### Q-3: A UI specification states criteria for five aspects

A specification that includes UI states criteria for these five aspects, not
just which parts exist:

- Visual hierarchy (what is primary and what is secondary)
- Information density (packed or spacious)
- Rhythm of spacing
- Typography
- Priority of actions (what should be touched first)

It also states who should feel what has improved, where the current state is
"not decent", which visual traits of the reference images must be kept, and
what change would fail to meet the requirement.

### Q-4: "The element exists" does not meet a UI quality criterion

Acceptance criteria that judge only the presence and order of parts are not
accepted as UI quality criteria. Write criteria so that "what good looks like"
can be judged on the aspects of Q-3. Do not force an overall judgement that
cannot be quantified ("does it look like a real product?") into numbers; state
which aspects a person judges it on (non-goal: defining everything by numbers
alone).

### Q-5: A placeholder states its value and its risk of being mistaken

When placing an element that cannot be operated, or an entry point with nothing
behind it, state for each one:

- The user value of placing it
- The risk that users mistake it for something usable, and how that is
  prevented

Do not place anything whose risk of being mistaken outweighs its value.
"Filling out the composition" is not a value.

### Q-6: Vagueness that cannot be made concrete goes back for clarification, not private interpretation

This rule is not "ask when in doubt". First find the answer yourself. Look, in
this order, at the repository's design documents and existing screens, the
specification's own text, and the form that comparable current products have
converged on; when you find the answer, write it into the specification. Turning
a matter that has an answer into a question is a defect that wastes the
requester's attention.

Ask a question only when all of the following hold:

- The answer changes what the user gets (scope, operations, how data is
  protected)
- The references above give no answer, or give two or more strong answers with
  very different outcomes
- A wrong choice is costly to undo

When a vague requirement cannot be made concrete enough, do not fill it with a
plausible interpretation. Go back to the requester with a question, and do not
settle the specification until there is an answer or an explicit agreement to
"proceed with this vagueness". Interpretation without agreement is where
degradation starts.

### Q-7: Do not promote implementation constraints to product requirements

"That approach is hard" or "standard components cannot do it" is no reason to
narrow the requirement. When you notice an implementation constraint, do not
rewrite the specification; take it back to the requester as a question (Q-6).

When you judge that the requirement cannot be met for implementation reasons,
make that judgement itself the question: "Because of this constraint it would be
B instead of A; is that acceptable?" Write it only after the answer.

The requester, not implementation difficulty, decides choices that change the
user's experience. Difficulty is information to attach when presenting the
options, not the choice itself.

## Non-goals

- Defining all UI by numbers alone
- Forbidding the model from making judgements when there is no designer
- Requiring a pixel-perfect match with the original draft
