# Implement workflow

Read [README.md](README.md) first. Input is one native child Issue.

1. Get its parent through the native sub-issue relationship. Resolve the
   explicitly requested feature branch or PR from the parent's relationships
   and read the feature directory's `**Parent Issue**` field.
2. If the request names an existing implementation PR, update that PR's head.
   Otherwise, an existing PR for the child does not block a separate PR.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Implement only the work named by the child Issue and its necessary tests.
5. Run focused checks and the repository checks required by the change. For UI
   work, follow `docs/how-to/ui-change-screenshots.md` and `ui-design.md`, and
   include screenshots plus visual, interaction, and accessibility review.
6. Push and open a feature-branch PR with `Refs #<child>`, checks, and remaining
   risks in the body. Stop.
7. After human merge, the maintainer closes the child as completed. This means
   implemented on the feature branch; the parent closes only when the
   integration PR reaches `main`.

If an approved artifact changes, use the selected branch's current artifacts as
implementation inputs. The parent SDD summary may be updated to communicate the
revision, but it does not block a user-requested implementation.

