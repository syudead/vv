# Plan workflow

Read [README.md](README.md) first. Input is a parent Issue and a user request to
create or revise a plan.

1. Use the supplied Issue, PR, branch, and current checkout as context.
2. Read the relevant `spec.md` and any existing `plan.md` as inputs.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Run the installed Spec Kit plan procedure for the explicit feature path.
   Resolve all consequential choices and include an implementation-work section
   detailed enough to create native child Issues directly from the plan.
5. Run the plan's checks, then the [`self-review` skill](../../self-review/SKILL.md)
   over the whole diff. Checks 1 and 5 carry the weight here: a plan is
   reconciled against `spec.md` and against the contracts it supersedes.
6. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
7. After human merge, the maintainer marks Plan complete. For a `ui` Issue set
   `Next: design`; otherwise set `Next: plan-to-issues`.

