# Plan workflow

Read [README.md](README.md) first. Input is a parent Issue with Spec complete
and `Next: plan`.

1. Resolve the feature branch from the parent's unique open integration PR.
2. Confirm that its `spec.md` maps to the parent and `plan.md` is absent.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Run the installed Spec Kit plan procedure for the explicit feature path.
   Resolve all consequential choices in the plan; do not pass undecided
   architecture or scope into Tasks.
5. Run the plan's checks, push, and open a feature-branch PR with
   `Refs #<parent>`. Stop.
6. After human merge, the maintainer marks Plan complete and sets `Next: design`
   for a `ui` Issue or `Next: tasks` otherwise.

