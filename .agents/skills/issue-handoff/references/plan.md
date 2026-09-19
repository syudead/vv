# Plan workflow

Read [README.md](README.md) first. Input is a parent Issue and a user request to
create or revise a plan.

1. Resolve the requested feature branch or PR from the parent relationship.
2. Read the selected branch's `spec.md` and any existing `plan.md` as inputs.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Run the installed Spec Kit plan procedure for the explicit feature path.
   Resolve all consequential choices in the plan; do not pass undecided
   architecture or scope into Tasks.
5. Run the plan's checks, push, and open a feature-branch PR with
   `Refs #<parent>`. Stop.
6. After human merge, the maintainer marks Plan complete and sets `Next: design`
   for a `ui` Issue or `Next: tasks` otherwise.

