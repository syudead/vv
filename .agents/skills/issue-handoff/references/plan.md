# Plan workflow

Read [README.md](README.md) first. Input is a parent Issue and a user request to
create or revise a plan. This is the first stage of a feature, so it is the
stage that creates the feature branch.

1. Use the supplied Issue, PR, branch, and current checkout as context. The
   parent Issue is the specification; read it as the plan's input.
2. For a new feature, fetch `main` and determine the next feature-directory
   number after inspecting `main`, the `specs/` of every other remote branch
   (a feature branch has no integration PR until `integrate`), and every open
   Plan PR. The directory number is independent from the Issue number.
   Then create and push an arbitrary-name long-lived feature branch from `main`.
   For a revision, use the existing feature branch.
3. Create an arbitrary-name sub-branch from the current feature branch. Read any
   existing `plan.md` as an input.
4. Run the [`sdd-plan` skill](../../sdd-plan/SKILL.md) for the explicit feature path.
   Resolve all consequential choices and include an implementation-work section
   detailed enough to create native child Issues directly from the plan. Do not
   create a `spec.md`.
5. Run the plan's checks and reconcile it against the parent Issue and the
   contracts it supersedes. Then check what a later stage would otherwise
   find: every statement the Plan makes about existing code (a path, a
   symbol, a schema, a behaviour it builds on) holds on the feature branch
   head; every dependency names an existing unit heading and the order has no
   cycle; no two units own the same change; and the other artifacts agree
   with the Plan.
6. Push and open a feature-branch PR with `Refs #<parent>`, with the body in
   [stage-pr-body.md](stage-pr-body.md). Stop.
7. After human merge, the next run continues with `design` for a `ui` Issue
   or `plan-to-issues` otherwise. The feature-to-`main` integration PR is
   opened by `integrate`, after every child is done. The parent Issue body is
   not edited.

## Revising a merged Plan

A revision is run when the maintainer names it, or by
[`sdd-autopilot`](../../sdd-autopilot/references/loop.md#revise-the-plan) when
a later stage finds that the Plan contradicts the code, another artifact or
itself, or leaves undecided something that stage needs. Steps 3 to 6 apply, with these rules:

- Change what the reason for the revision requires, and look for the same
  wrong assumption elsewhere in the Plan and its artifacts, so the next stage
  does not find it one place at a time.
- The code merged into the feature branch is a fact. When the revision
  invalidates work that is merged, add a unit that corrects it; do not
  rewrite a unit that is done.
- Keep the heading of a unit whose work does not change, because the heading
  is the title its child Issue was created with. Drop a unit that is no
  longer needed.
- Revise `ui-design.md` and the other artifacts in the same PR when the
  change reaches them (`ui-design.md` under [design.md](design.md)), and
  delete one left without content (P-2).
- The parent Issue is the specification and does not change here. A
  contradiction that only a different requirement or acceptance criterion
  resolves goes to the requester.
- In the PR body, list the units added, changed and dropped, counting as
  changed a unit whose text stays but whose artifact detail moved, so the
  children can be reconciled
  ([plan-to-issues](plan-to-issues.md#after-a-plan-revision)).

If execution stops after the empty feature branch is pushed but before the Plan
PR exists, that branch has no standard GitHub relationship to the Issue. Do not
guess its identity; a maintainer removes it before retrying.
