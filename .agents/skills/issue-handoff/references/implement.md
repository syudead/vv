# Implement workflow

Read [README.md](README.md) first. Input is one native child Issue.

1. Get its parent through the native sub-issue relationship. Resolve the
   feature branch from the parent's unique open integration PR and verify the
   feature directory's `**Parent Issue**` field.
2. Confirm that no open implementation PR already references this child. If
   one exists, update only that PR's head for review fixes.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Implement only the named task and its necessary tests. In the same change,
   mark only that task complete in `tasks.md`; do not alter other task markers.
5. Run focused checks and the repository checks required by the change. For UI
   work, follow `docs/how-to/ui-change-screenshots.md` and `ui-design.md`, and
   include screenshots plus visual, interaction, and accessibility review.
6. Push and open a feature-branch PR with `Refs #<child>`, checks, and remaining
   risks in the body. Stop.
7. After human merge, the maintainer closes the child as completed. This means
   implemented on the feature branch; the parent closes only when the
   integration PR reaches `main`.

If an approved artifact must change, stop new implementation. The maintainer
resets the parent SDD summary to the first affected stage, and the affected stage
workflows run again before child implementation resumes.

