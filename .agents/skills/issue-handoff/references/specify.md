# Specify workflow

Read [README.md](README.md) first. Input is one parent Issue and a user request
to create or revise a specification.

1. If the request names an existing Spec PR, work on that PR. Otherwise an
   existing Spec PR may be reused or a separate PR may be created.
2. For a new Spec, fetch `main`. Determine the next feature-directory number
   after inspecting `main`, every open integration PR head, and every open Spec
   PR. The directory number is independent from the Issue number.
3. Create and push an arbitrary-name long-lived feature branch from `main`,
   then create an arbitrary-name sub-branch from it.
4. Run the installed Spec Kit specify procedure on the sub-branch. Record the
   initiating Issue as exactly `**Parent Issue**: #NNN` in `spec.md`. Do not
   leave unresolved clarification markers.
5. Validate the specification against `docs/product-specs/spec-quality.md`
   and correct any issues before opening the PR.
6. Run the [`self-review` skill](../../self-review/SKILL.md) over the whole
   diff. Checks 1 and 5 carry the weight here: a new specification is
   reconciled against the contracts it supersedes in other feature
   directories, which is where a contradiction survives unnoticed.
7. Push the sub-branch and open a PR to the feature branch with `Refs #NNN`.
   Another open Spec PR does not require either PR to be closed.
8. Stop. A human decides when to merge the PR.
9. After merge, the maintainer opens the feature-to-`main` integration
   PR with `Closes #NNN`, then marks Spec complete and sets `Next: plan` in the
   parent Issue.

If execution stops after the empty feature branch is pushed but before the
Spec PR exists, that branch has no standard GitHub relationship to the Issue.
Do not guess its identity; a maintainer removes it before retrying.
