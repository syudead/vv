---
name: "quality-check"
description: "Check a finished diff against the repository's mechanical rules (test levels and cost, complexity, duplication, generated files, translations) before the push. Use after the repository checks pass and before self-review or the push."
---

# Quality check

These are the rules a reviewer would otherwise raise one at a time and that a
command can decide. Each check names its command; run it on the whole diff
against the base branch, and report what the command printed, not an opinion.
Meaning, scope and agreement with the Issue belong to
[self-review](../self-review/SKILL.md), not here.

## Checks

| # | Check | Command | Finding when |
| --- | --- | --- | --- |
| Q-1 | Test levels | `task test-cost -- -measure` | A store, http, media, page or e2e row has no row in the level table supplied by the parent, or its reason is that the rule lives in the page or handler ([test-writing](../test-writing/SKILL.md)) |
| Q-2 | Slow tests | Same output | An added test takes over 1 s without a stated reason |
| Q-3 | Waiting and snapshots | `git diff <base> -- '*_test.go' '*.test.ts' '*.test.tsx' '*.e2e.ts' \| grep -nE '^\+.*(time\.Sleep\|toMatchSnapshot\|toMatchInlineSnapshot\|waitForTimeout)'` | Any added line matches |
| Q-4 | Go complexity | `task complexity-go`, then the rows of `reports/quality/complexity-go/gocyclo.txt` for functions the diff adds or changes | A changed function outside tests and generated code is over 25 |
| Q-5 | Web complexity | `npx eslint --rule 'complexity: [error, {max: 25, variant: modified}]' <changed files under web/src>` in `web/` | A changed function outside tests is over 25 |
| Q-6 | Duplication | `task duplication`, then the clones in `reports/quality/duplication/jscpd-report.json` with a side in a changed file | A clone the diff creates; a moved block is not one |
| Q-7 | Generated files | `git diff --name-only <base>` | A path under `internal/httpapi/gen/`, `internal/httpapi/extgen/` or `web/src/api/gen/` changed without a change to `api/openapi.yaml` or `api/external-v1.yaml`, or `task generate-check` fails |
| Q-8 | Translations | `git diff --name-only <base>` | A changed, renamed or deleted file under `docs/`, `specs/` or `ARCHITECTURE.md` has no matching change under `translations/ja/` |
| Q-9 | Design document index | `git diff --name-status <base> -- docs/design-docs/` | An added design document is not linked from `docs/design-docs/index.md` |

The thresholds and their reasons are in
[complexity-and-duplication.md](../../../docs/design-docs/complexity-and-duplication.md),
[go-testing.md](../../../docs/design-docs/go-testing.md) and
[web-testing.md](../../../docs/design-docs/web-testing.md).

## Return

One row per check: the check, the command run, `pass` or the findings with
file and line. Every finding blocks the push; the parent fixes it and runs the
check again. A finding the parent judges wrong goes to the pull request's
`残したこと` with the reason, never silently dropped.
