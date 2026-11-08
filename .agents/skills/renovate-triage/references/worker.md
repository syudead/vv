# Fix one Renovate PR

A `renovate-pr-fixer` worker follows this page for the one Renovate PR in its
brief. It gets the PR to a state the orchestrator can merge, or says why it
cannot and when trying again makes sense. It never merges or closes the PR.

If the PR is not by `app/renovate`, change nothing and return `DEFERRED` with
the reason and `REVISIT: a person decides`.

When the PR already has a `renovate-triage` review or blocker comment from an
earlier run, start from it: check whether the blocker it names still holds
before redoing the analysis.

## Set up

Fetch and check out the head branch (`git fetch origin <branch>` and
`git checkout -B <branch> origin/<branch>`). Read
[dependency-updates.md](../../../../docs/how-to/dependency-updates.md) first:
it says which dependencies need an extra check on update and which ones the
routine must not touch.

Read the PR body for the updated packages, their old and new versions, and the
release notes Renovate links.

Then merge `origin/main` into the branch (`git merge origin/main`; never
rebase), so local checks run against today's `main` and not the one the branch
was cut from. A Renovate branch is often far behind, and checking it as it is
fails on changes from `main` the PR has nothing to do with. Do not wait for
Renovate to rebase; that only happens on its own schedule. When the merge of
`main` turns out to be the only change and it makes the failed checks pass
locally, push it: `main` already had the fix, and CI must run on the new head.
Do not push it only when the head's checks were already all green (a major
update review with nothing to change).

## Conflicts

Resolve a conflict from that merge in a lockfile or `go.sum` by taking
`main`'s side and regenerating it:

| File | Regenerate with |
| --- | --- |
| `go.sum` | `go mod tidy` |
| `web/package-lock.json` | `npx --yes npm@11 install --package-lock-only` in `web/` |
| `tools/package-lock.json` | `npx --yes npm@11 install --package-lock-only` in `tools/` |
| `docs-site/package-lock.json` | `npx --yes npm@11 install --package-lock-only` in `docs-site/` |

npm 11 matches the constraint in `renovate.json`. Never edit a lockfile by
hand. Resolve a conflict in the code by keeping both sides' behaviour.

## Failed checks

Read the failed job logs of the head commit. Reproduce the failure locally with
the task the job runs (the job names map to the parts of `task check`, and
`Browser E2E (<group>)` to `task test-e2e -- <files>` from `ci.yml`). Then find
the root cause:

- **The new version changed an API or a behaviour the code relies on**: adapt
  the code, with a focused test where the behaviour is testable. Follow the
  new version's migration notes, not a workaround.
- **A test relied on an implementation detail of the old version**: change the
  test to assert the behaviour, not the detail. Do not loosen what it checks.
- **The new version has a bug that breaks this repository and has no
  workaround in this repository's code**: return `DEFERRED` with the upstream
  issue link as the revisit condition, or `the next version` when no issue
  exists. Do not pin the old version.
- **The failure is not the update's** (it fails the same way on `main`): return
  `DEFERRED` saying so, with `REVISIT: the next version`.

Never skip, disable, loosen or quarantine a test or a check, and never re-run a
check to get past it.

## Major updates

Read the breaking changes between the old and the new major version: the
release notes, the changelog and the migration guide the project publishes.
For every breaking change, search the repository for what it touches (imports,
called APIs, configuration keys, CLI flags, plugin options) and decide whether
it affects this repository.

Apply the changes that do. When a design document under `docs/` describes the
behaviour the update changes, update that document in the same commit.

Then post one comment on the PR:

```markdown
<!-- renovate-triage -->
### Major update review: <package> <old> → <new>

| Breaking change | Affects vv? | Handling |
| --- | --- | --- |
| <change, linked to the release note> | yes / no: <where> | <what this PR changed, or why nothing> |

Verdict: safe to merge | blocked: <reason>
```

Write `safe to merge` only when every row that affects the repository is
handled in the PR, and the behaviour it touches is covered by a test or by the
Browser E2E run on the PR. Otherwise write `blocked`, give the reason and the
revisit condition, and return `DEFERRED`.

The revisit condition is the most specific checkable fact that would unblock
the update: an upstream issue or PR to be closed (link it), or a package
version to be published that meets a stated requirement (for example "an
`openapi-typescript` release whose `peerDependencies` allow `typescript` 7").
Use `a person decides` only when no upstream change would unblock it, such as
a rewrite the new major version demands or a `renovate.json` change.

Update the comment instead of adding a second one when the worker runs again
on the same PR.

## Before pushing

Run the checks the AGENTS.md pre-push rule asks for: use `task check` for
shared dependency changes, scope other fixes to affected packages/files,
and run `task check-docs` for Markdown, `docs/` or `specs/` changes.
Fix formatting in the affected files. Run the `Browser E2E`
files that failed, if any; CI runs the whole suite on the PR.

Commit with a Conventional Commit message (`fix(deps): ...` for a code fix,
`build(deps): ...` for a lockfile or merge fix). Push once, at the end of
the worker's run, with `git push origin <branch>`, never with force. The
orchestrator waits for CI and runs a new worker when it fails again. Then post
one comment:

```markdown
<!-- renovate-triage:attempt -->
<one line: what failed or conflicted, the root cause, and what the commit changes>
```

## Return

End with exactly this block:

```text
STATUS: PUSHED | READY | DEFERRED
PR: <number>
COMMIT: <pushed commit, or ->
REASON: <one line>
REVISIT: <for DEFERRED: the revisit condition; otherwise ->
```

- `PUSHED`: a commit was pushed. CI has to run again.
- `READY`: nothing needed a change. The checks are green, and for a major
  update the review comment says `safe to merge`.
- `DEFERRED`: the routine cannot get this PR merged until something outside
  the repository changes. Post the reason as a `<!-- renovate-triage -->`
  comment on the PR first, with what you found and a proposed fix when you
  have one. The orchestrator closes the PR.
