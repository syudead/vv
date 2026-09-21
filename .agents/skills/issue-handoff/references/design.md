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
- `web/src/index.css` (`@theme`) — the design-token values themselves. Never
  copy a token's value into `ui-design.md`; name the token. This is about token
  values, not about numbers in general: the viewport widths and breakpoints a
  review criterion needs are written out
- `web/src/theme/tokens.test.ts` — the `pairs` array is what actually enforces
  contrast; it is a hard-coded list, so a foreground/background combination is
  checked only once it is added there. A new colour token is a decision, and the
  change that introduces it records the token in
  `specs/004-library-ui/contracts/design-tokens.md`. Add a pair to that array
  only for a combination that carries a contrast requirement — the array is not
  a registry of tokens, and it reads six-digit hex values only, so a
  translucent or decorative token does not belong in it
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
6. Run the [`self-review` skill](../../self-review/SKILL.md) over the whole diff.
   Reconcile every stated behaviour against `spec.md` and the existing design
   system rules, which this artifact narrows more often than it contradicts.
7. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
8. After human merge, the maintainer marks Design complete and sets
   `Next: plan-to-issues`.
