# Integrate workflow

Read [README.md](README.md) first. Selected when every native child of the
parent is done.

1. Check out the feature branch and merge the latest `main` into it directly.
   Do not rebase or force-push the long-lived feature branch. Resolve conflicts
   by keeping both sides' behaviour; ask when that is not possible.
2. Run `task check`. For a `ui` feature, also run `task test-e2e` and review
   the screens against `ui-design.md`.
3. Push the feature branch. This updates the existing integration PR; do not
   open a separate PR for it.
4. Update the integration PR body with the checks you ran and any remaining
   risk. Stop. A human merges the integration PR, and GitHub closes the parent.

If the feature branch already contains the latest `main` and the checks already
passed on its head, report that the integration PR waits on human merge and
stop.
