# Specify workflow

Read [README.md](README.md) first. Input is one parent Issue with `Next:
specify`.

1. If the Issue already has one open Spec PR, work only on review changes in
   that PR. Multiple matching PRs are an error.
2. For a new Spec, fetch `main`. Determine the next feature-directory number
   after inspecting `main`, every open integration PR head, and every open Spec
   PR. The directory number is independent from the Issue number.
3. Create and push an arbitrary-name long-lived feature branch from `main`,
   then create an arbitrary-name sub-branch from it.
4. Run the installed Spec Kit specify procedure on the sub-branch. Record the
   initiating Issue as exactly `**Parent Issue**: #NNN` in `spec.md`. Do not
   leave unresolved clarification markers.
5. Run the specification checklist described in
   `docs/how-to/spec-quality-review.md`. Leave its independent-review item
   open; the author cannot approve it.
6. Push the sub-branch and open a PR to the feature branch with `Refs #NNN`.
   Recheck open Spec PRs for the chosen directory prefix. If another run won
   the same prefix, keep the oldest PR, close the later one, refetch, and start
   numbering again.
7. Stop. An independent reviewer applies `docs/how-to/spec-quality-review.md`
   to the open PR. Only after approval does a human merge it.
8. After human merge, the maintainer opens the feature-to-`main` integration
   PR with `Closes #NNN`, then marks Spec complete and sets `Next: plan` in the
   parent Issue.

If execution stops after the empty feature branch is pushed but before the
Spec PR exists, that branch has no standard GitHub relationship to the Issue.
Do not guess its identity; a maintainer removes it before retrying.
