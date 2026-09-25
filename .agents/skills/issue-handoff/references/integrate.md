# Integrate workflow

Read [README.md](README.md) first. Selected when every native child of the
parent is done.

1. Check out the feature branch and merge the latest `main` into it directly.
   Do not rebase or force-push the long-lived feature branch. Resolve conflicts
   by keeping both sides' behaviour; ask when that is not possible.
2. Run `task check`. For a `ui` feature, also run `task test-e2e` and review
   the screens against `ui-design.md`.
3. Push the feature branch. If the integration PR does not exist yet, open it
   now: feature branch → `main`, the parent's title, `Closes #<parent>`, and
   the repository PR template. Otherwise the push updates it; do not open a
   separate PR for it.
4. Write the integration PR body with the checks you ran and any remaining
   risk. Stop. A human merges the integration PR, and GitHub closes the parent.

If the feature branch already contains the latest `main` and the checks already
passed on its head, report that the integration PR waits on human merge and
stop.

## Review of the integration PR

The integration PR's diff is the whole feature. Every child already went
through its own review, so a review of the integration PR is looked at for
what only the whole diff shows, not for another pass of polish. Every push to
the feature branch gets the whole diff reviewed again, and a bot reviewer
reports a few new findings on each pass of a diff this size, so fixing every
finding never runs out.

Triage each finding before touching code:

- **Blocking**: a required check that failed, a code-scanning alert, a finding
  a human reviewer wrote, and a bot finding the bot marks as a bug or a
  high-severity security issue (Devin Review: 🔴 or 🟥). Verify it; fix a real
  one through a PR to the feature branch that `Refs #<parent>`, and answer one
  that is not a defect on its thread.
- **Not blocking**: every other bot finding (Devin Review: 🟡, 🟨, 🔍). Verify
  it and reply on its thread in one line, then resolve the thread. When it is
  a real defect, add it to the integration PR body's remaining risks for the
  maintainer; do not open a fix PR for it. The maintainer decides whether it
  is fixed before the merge, in a follow-up, or not at all.

Bring in `main` again only when the integration PR conflicts with it or a
required check fails because of it. `main` having moved on is not a reason:
each merge moves the head and gets the whole diff reviewed again. The human
merging the PR can refresh it one last time.
