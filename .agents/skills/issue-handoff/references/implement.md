# Implement workflow

Read [README.md](README.md) first. Input is one native child Issue.

1. Read the supplied child Issue and its native parent relationship. Use any
   supplied PR or branch and the current checkout as additional context.
2. If the request names an existing implementation PR, update that PR's head.
   Otherwise, an existing PR for the child does not block a separate PR.
3. Create an arbitrary-name sub-branch from the current feature branch.
4. Implement only the work named by the child Issue and its necessary tests.
   When the selected host provides the project-scoped `subissue-implementer`
   worker, delegate this step to it. Give it the child Issue body, the relevant
   approved artifact paths, the acceptance evidence, and an explicit write
   scope. The worker owns only the implementation, focused tests, and focused
   checks: it does not create branches, change approved artifacts, run
   self-review, push, or open a PR. Wait for it to finish, inspect its changes,
   and keep ownership of every later step in this workflow. If that worker is
   unavailable, use an equivalent bounded worker when the host supports one, or
   perform this step locally.
5. Run focused checks and the repository checks required by the change. For UI
   work, check the result against the review criteria in `ui-design.md`.
6. Run the [`self-review` skill](../../self-review/SKILL.md) over the whole diff
   against the feature branch. When the selected host provides the
   project-scoped `self-reviewer` worker, delegate the review to that fresh
   context and give it the diff, sources of truth, acceptance evidence, and
   self-review procedure. Fix what it finds, re-run the checks from step 5, and
   carry anything it defers into the PR body.
7. Push and open a feature-branch PR with `Refs #<child>`, checks, and remaining
   risks in the body. Stop.
8. After human merge, the maintainer closes the child as completed. This means
   implemented on the feature branch; the parent closes only when the
   integration PR reaches `main`. Under
   [`sdd-autopilot`](../../sdd-autopilot/SKILL.md) its orchestrator merges and
   closes the child.

If an approved artifact changes, use the selected branch's current artifacts as
implementation inputs.

