# Integrate workflow

Read [README.md](README.md) first. Selected when every native child of the
parent is done. Integration has three parts, each run once in this order: the
refresh that opens the integration PR, the
[review of the integration PR](#review-of-the-integration-pr), and the
[pre-merge sweep](#pre-merge-sweep). A run performs the first part that is not
done yet and stops.

1. Check out the feature branch and merge the latest `main` into it directly.
   Do not rebase or force-push the long-lived feature branch. Resolve conflicts
   by keeping both sides' behaviour; ask when that is not possible.
2. Run `task check`. For a `ui` feature, also run `task test-e2e` and review
   the screens against `ui-design.md`.
3. Push the feature branch. If the integration PR does not exist yet, open it
   now: feature branch → `main`, the parent's title, `Closes #<parent>`, and
   the repository PR template. Otherwise the push updates it; do not open a
   separate PR for it.
4. Write the integration PR body in the repository PR template: what changes
   for the user across the whole feature, as one table and a diagram, the
   checks you ran and any remaining risk. When the body already lists remaining risks, including those recorded
   from its review, keep them. Stop. Its review and the sweep come in later
   runs; a human merges the integration PR, and GitHub closes the parent.

When the integration PR already exists, take the first that applies:

1. Its review has a thread not triaged yet (unresolved, with no reply from
   this workflow): triage every finding as
   [below](#review-of-the-integration-pr). A blocking one gets a fix PR to the
   feature branch that `Refs #<parent>`, which is this run's one PR, and the
   run stops there. When triage needs no fix PR, go on with the next rules in
   the same run.
2. A PR into the feature branch that `Refs #<parent>` is still open (a review
   fix or the sweep's PR): it waits on human merge; report that and stop.
3. It has no sweep comment (one containing `<!-- sdd-sweep:`): when it has no
   review yet, report that it waits on its review and stop; otherwise run the
   [pre-merge sweep](#pre-merge-sweep). Its PR, if any, is this run's one PR.
4. Otherwise, if the feature branch already contains the latest `main` or
   needs no refresh (see the end of the next section) and the checks passed
   on its head, report that it waits on human merge and stop.

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
  a real defect, add it to the integration PR body's remaining risks; do not
  open a fix PR for it. The [pre-merge sweep](#pre-merge-sweep) picks the
  ones worth fixing before the merge, and the maintainer decides on the rest:
  a follow-up, or nothing.

Bring in `main` again only when the integration PR conflicts with it or a
required check fails because of it. `main` having moved on is not a reason:
each merge moves the head and gets the whole diff reviewed again. The human
merging the PR can refresh it one last time.

## Pre-merge sweep

The last code change before the merge, run once after the review of the
integration PR and after its fix PR, if any, has merged. It does two things a
review cannot: it runs every check against the finished feature, and it
collects the deferred defects that are worth fixing now rather than in a
follow-up. Everything it changes goes into one PR, so the sweep converges in a
single round.

1. **Run every check on the feature branch head**, not only the ones a single
   child needed: `task check`, `task check-docs`, `task test-e2e` (for every
   feature, not only `ui`; when the host cannot run a browser, say so in the
   body instead of passing it over), and `task docs-build` when the feature
   changed `docs/` or `specs/`. Then walk every acceptance criterion of the
   parent Issue and point at the test or the code path that satisfies it. A
   failing check or an unmet criterion is a defect to fix in this sweep,
   whatever its size, unless the fix needs a requester decision or an approved
   artifact changed; then stop and ask.
2. **Collect the backlog**: the remaining risks the integration PR body lists,
   the out-of-scope items the merged feature PRs' bodies deferred, and
   unresolved threads on the integration PR. Verify each against the current
   head; drop the ones already fixed or not reproducible. A thread left
   unresolved because its fix went into a review fix PR that has merged:
   confirm the fix on the head, reply naming that PR, and resolve it, so no
   thread on the integration PR stays open for the merge.
3. **Pick only what is worth fixing before the merge.** An item is picked when
   every one of these holds:
   - it is a verified defect in what this feature added or changed, not a
     state `main` already had
   - it breaks an acceptance criterion, loses or corrupts data, is a security
     issue, crashes or hangs, leaks a resource, races, or regresses behaviour
     `main` had
   - its fix stays local: no change to an approved artifact, the OpenAPI
     contracts, a migration, or a requester decision, and it fits in the
     sweep PR alongside the others without widening it into a redesign
   - a focused test can show it, unless the defect is in code with no test
     seam (platform glue, for example); then name how you checked it

   Leave everything else: style, naming, refactoring, comments, speculative
   hazards with no reachable trigger, unmeasured performance, defects `main`
   already had, and anything larger. Pick at most five, most severe first; the
   rest stay remaining risks.
4. **Fix them in one PR.** When step 1 found a defect or step 3 picked one,
   create a sub-branch from `origin/<feature>`, fix each at its root cause with
   a focused test, run `task check` and `task check-docs` again (and
   `task test-e2e` when the fix touches the web app or the flow it tests),
   push, and open one PR to the feature branch with `Refs #<parent>`, titled
   for the pre-merge sweep. Its body lists each fix and the backlog item or
   check it answers. Nothing to fix means no PR.
5. **Comment the result on the integration PR**, in one comment: the checks
   run and their results, the acceptance-criterion walk, the items collected
   (naming the sweep PR), and every item left with a one-line reason. Start
   it with `<!-- sdd-sweep:<feature head SHA swept> -->`; that comment is how
   a later run knows the sweep happened, so post it last, after the PR
   exists. Leave the integration PR body as it is.

The sweep PR goes through the same review as any PR into the feature branch.
After it merges, the integration PR is not reviewed or swept again; it waits on
human merge.
