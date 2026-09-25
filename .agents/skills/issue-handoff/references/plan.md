# Plan workflow

Read [README.md](README.md) first. Input is a parent Issue and a user request to
create or revise a plan. This is the first stage of a feature, so it is the
stage that creates the feature branch.

1. Use the supplied Issue, PR, branch, and current checkout as context. The
   parent Issue is the specification; read it as the plan's input.
2. For a new feature, fetch `main` and determine the next feature-directory
   number after inspecting `main`, every open integration PR head, and every
   open Plan PR. The directory number is independent from the Issue number.
   Then create and push an arbitrary-name long-lived feature branch from `main`.
   For a revision, use the existing feature branch.
3. Create an arbitrary-name sub-branch from the current feature branch. Read any
   existing `plan.md` as an input.
4. Run the [`sdd-plan` skill](../../sdd-plan/SKILL.md) for the explicit feature path.
   Resolve all consequential choices and include an implementation-work section
   detailed enough to create native child Issues directly from the plan. Do not
   create a `spec.md`.
5. Run the plan's checks, then the [`self-review` skill](../../self-review/SKILL.md)
   over the whole diff. Checks 1 and 5 carry the weight here: a plan is
   reconciled against the parent Issue and against the contracts it supersedes.
6. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
7. After human merge, the next run opens the feature-to-`main` integration PR
   with `Closes #NNN` and continues with `design` for a `ui` Issue or
   `plan-to-issues` otherwise. The parent Issue body is not edited.

If execution stops after the empty feature branch is pushed but before the Plan
PR exists, that branch has no standard GitHub relationship to the Issue. Do not
guess its identity; a maintainer removes it before retrying.
