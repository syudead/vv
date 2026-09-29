# Specification quality rules: preventing requirement degradation

These rules apply whenever a specification is written or changed. The specification lives in
the parent Issue, and `ui-design.md` carries it forward for the UI.

The rules have one aim: **never turn an ambiguous but essential user requirement into only
discrete conditions that are easy to check mechanically.** A specification that yields a
product the user did not ask for, even when implemented faithfully, is rejected at the
specification stage.

Rule numbers stay as introduced. Q-1 (keep the quoted requirement) and Q-2 (map acceptance
criteria to requirements) are retired: they lost their target when the parent Issue itself
became the source of truth for requirements.

## Terms

| Term | Meaning |
| --- | --- |
| **Requester** | The person who asked for the feature. The one to ask questions, and the one running the workflow. |
| **User** | The person who uses the finished product. The subject of the Issue's Requirements and Acceptance criteria, and not someone to ask questions. |

"The answer changes what the user gets" means the answer changes the experience of the people
who use the feature after it ships. The requester makes that call.

## Rules

### Q-3: a UI specification states criteria for five aspects

A specification that includes UI states criteria for the following five aspects, beyond which
components exist.

- Visual hierarchy (what is primary and what is secondary)
- Information density (packed or spacious)
- Spacing rhythm
- Typography
- Operation priority (what the user should touch first)

It also states:

- who should feel what has improved;
- where the current state is "not decent";
- which visual features of the reference images must be kept;
- which changes would mean the requirement is no longer met.

### Q-4: "the element exists" does not meet a UI quality criterion

An acceptance criterion that checks only whether components exist and in what order is not
accepted as a UI quality criterion.

- Write the criterion so that "what good looks like" can be judged against the Q-3 aspects.
- Do not force an overall judgment that cannot be quantified (such as "does it look like a
  real product") into numbers. State which aspects a person judges it by. Non-goal: defining
  everything by numbers alone.

### Q-5: a placeholder states its value and its misreading risk

For each non-operable element or entry point with nothing behind it, state:

- the user value of placing it;
- the risk that users think it works, and how that risk is suppressed.

Do not place an element whose misreading risk outweighs its value. "Filling the composition"
is not a value.

### Q-6: unresolvable ambiguity goes back to clarification, not to a private interpretation

This rule is not "ask when in doubt". First find the answer yourself. Search, in this order:

1. The repository's design documents and existing screens
2. The specification's own text
3. The form that comparable current products converge on

When one of them answers the question, write the answer into the specification. Turning an
answerable matter into a question is a defect: it wastes the requester's attention.

Ask a question only when all of the following hold:

- The answer changes what the user gets (scope, operations, how data is protected).
- The sources above give no answer, or give two or more plausible answers with very different
  results.
- A wrong choice is costly to undo.

When an ambiguous requirement cannot be made concrete enough, do not fill it with a plausible
interpretation. Return the question to the requester. Do not settle the specification until
an answer arrives, or an explicit agreement to "proceed with this ambiguity". An
interpretation without agreement is where degradation starts.

### Q-7: implementation constraints never become product requirements

"That approach is hard" and "standard components cannot reach it" are not reasons to narrow a
requirement.

- When you notice an implementation constraint, do not rewrite the specification. Return it
  as a question about the requirement (Q-6).
- When you judge that implementation limits prevent meeting the requirement, make that
  judgment the question: "Because of this constraint the result is B, not A. Is that
  acceptable?" Write it only after the answer.
- The requester, not implementation difficulty, decides any choice that changes the user's
  experience. Difficulty is information attached to the options, not the choice itself.

## Non-goals

- Defining all UI by numbers alone
- Forbidding the model's judgment when no designer is present
- Requiring pixel-perfect match with the original draft
