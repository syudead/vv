# UI design workflow

Read [README.md](README.md) first. This workflow is used when the user requests
UI design work for a parent Issue.

`ui-design.md` records what this feature's screens do that the repository's UI
documents do not already settle. It is not a restatement of the design system,
and it is not a place to re-choose things that are already chosen.

## Sources that already decide things

Read these before writing, and link them rather than repeating them:

- [docs/design-docs/library-ui-design-system.md](../../../../docs/design-docs/library-ui-design-system.md)
  — the visual rules and why they are what they are
- [docs/design-docs/modern-library-ui.md](../../../../docs/design-docs/modern-library-ui.md)
  — the layout and information hierarchy the product has settled on
- `web/src/index.css` (`@theme`) — the token values themselves. Never write a
  colour, radius, or size into `ui-design.md`; name the token
- `specs/004-library-ui/contracts/design-tokens.md` — the pairs the contrast
  test checks. A new token is a decision, and it is added here in the same
  change that introduces it
- the existing screens in `web/src` — what the product already does

Where those settle a question, follow them and say so. Where the form is one
comparable current products have converged on, follow that. A question goes to
the requester only when the answer changes what the user gets, none of the
above settles it (or two credible designs lead somewhere materially different),
and getting it wrong would be expensive to undo — the rule is Q-6 in
[docs/product-specs/spec-quality.md](../../../../docs/product-specs/spec-quality.md).
A technical constraint is never a reason to narrow the design (Q-7).

## Steps

1. Use the supplied Issue, PR, branch, and current checkout as context.
2. Read the available spec, plan, and existing UI design as inputs, plus the
   sources above.
3. Create an arbitrary-name sub-branch from the feature branch.
4. Create `<feature-dir>/ui-design.md`. Write only what this feature adds or
   changes: screen boundaries, visual hierarchy, responsive behaviour, content
   and system states, interactions, accessibility, and observable review
   criteria. Cover each of these when the feature touches it, and leave out the
   ones it does not — the artifact is as long as the change earns. Do not
   implement code.
5. Write the review criteria so they can be judged by looking. Q-3 names the
   viewpoints a UI specification argues in — visual hierarchy, information
   density, spacing rhythm, typography, and the priority of actions — and Q-4
   rules out "the element is present" as a criterion. An element with no
   behaviour behind it needs its value and its misrecognition risk stated
   (Q-5). Name the widths, the keyboard path, and the assistive-technology
   check the implementation PR will have to show, so the screenshots asked for
   in [docs/how-to/ui-change-screenshots.md](../../../../docs/how-to/ui-change-screenshots.md)
   have something to be judged against.
6. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
7. After human merge, the maintainer marks Design complete and sets
   `Next: plan-to-issues`.
