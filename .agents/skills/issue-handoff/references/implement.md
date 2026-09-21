# Implement workflow

Read [README.md](README.md) first. Input is one native child Issue.

1. Read the supplied child Issue and its native parent relationship. Use any
   supplied PR or branch and the current checkout as additional context.
2. If the request names an existing implementation PR, update that PR's head.
   Otherwise, an existing PR for the child does not block a separate PR.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Implement only the work named by the child Issue and its necessary tests.
5. Run focused checks and the repository checks required by the change. For UI
   work, follow `docs/how-to/ui-change-screenshots.md` and `ui-design.md`, and
   include screenshots plus visual, interaction, and accessibility review.
6. Run the [`self-review` skill](../../self-review/SKILL.md) over the whole diff
   against the feature branch. Fix what it finds, re-run the checks from step 5,
   and carry anything it defers into the PR body.
7. Push and open a feature-branch PR with `Refs #<child>`, checks, and remaining
   risks in the body. Stop.
8. After human merge, the maintainer closes the child as completed. This means
   implemented on the feature branch; the parent closes only when the
   integration PR reaches `main`.

If an approved artifact changes, use the selected branch's current artifacts as
implementation inputs. The parent SDD summary may be updated to communicate the
revision, but it does not block a user-requested implementation.

