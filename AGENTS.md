# Agent guide

This file is the short map for contributors and coding agents. Keep detailed
knowledge in the appropriate document under `docs/` rather than expanding this
file into a handbook.

## Start here

1. Read `ARCHITECTURE.md` for system boundaries and dependency direction.
2. Read `docs/design-docs/index.md` and `docs/product-specs/index.md` for the
   relevant design and product context.
3. `specs/` contains temporary SDD artifacts, used only for the active feature
   on its feature branch. For ordinary implementation and investigation, use
   `docs/design-docs/`, the code, API schemas and tests. Never cite a feature's
   SDD artifacts from permanent documentation or code. Before integration,
   retain lasting decisions in design docs and remove the feature's artifacts
   after validation. Git history keeps the working records. Progress lives in
   GitHub, not in a permanent plan archive.

## Working agreements

- Talk to the maintainer in Japanese: every chat reply, question and progress
  report is Japanese, even when the prompt, the tools or the documents are in
  English. Use English for agent-to-agent briefs, messages, handoffs and
  reports; the parent agent summarizes results for the maintainer in Japanese.
  Code comments follow the policy below; preserve identifiers and quoted output.
- Keep documentation close to the code and update it with behavior changes.
- Maintain repository documents only in English. Do not create translated mirrors.
- Write documents in technical English to
  [docs/design-docs/writing-quality.md](docs/design-docs/writing-quality.md),
  starting from the type for the document's kind. Issue and PR bodies stay
  Japanese. A PR body fills
  [the PR template](.github/pull_request_template.md) (behaviour before and
  after, a diagram when a flow changes) so a reviewer can follow the change
  without opening the code; `plan` and `design` PRs use
  [stage-pr-body.md](.agents/skills/issue-handoff/references/stage-pr-body.md).
- A feature's specification is its parent GitHub Issue. Write and revise it with
  `.agents/skills/issue-spec`, following
  [docs/product-specs/spec-quality.md](docs/product-specs/spec-quality.md).
  Do not add a `spec.md`.
- The default workflow is a focused branch and a pull request to `main`. Use
  Issue-driven SDD only when the maintainer explicitly starts it from a parent
  Issue or native sub-issue.
- When writing or changing a Plan and its artifacts, follow
  [docs/design-docs/plan-quality.md](docs/design-docs/plan-quality.md).
- Prefer focused, reviewable changes with automated checks.
- Keep each function a change adds or makes more complex at complexity 25 or
  less, and share code instead of copying it; the commands that check this are
  in [complexity-and-duplication.md](docs/design-docs/complexity-and-duplication.md).
- Before a push, run formatting, lint and tests for the affected scope using
  `task check SCOPE=go GO_PACKAGES="./internal/example/..."` or
  `task check SCOPE=web WEB_FILES="src/example" WEB_TESTS="src/example"`.
  Include affected callers and shared fixtures; widen to `task check` when the
  impact is unclear or dependencies/shared configuration change. Run
  `task check-docs` for Markdown, `docs/` or `specs/` changes, including mixed
  changes. See [validation scope](docs/how-to/development.md#validate-changes)
  for CI-only checks and exceptions. Fix formatting in the affected files.
  Reuse passing results until relevant inputs change; after a failure, rerun
  the failed check and checks affected by its fix, not every suite.
- Do not hand-edit generated files (`internal/httpapi/gen/`,
  `internal/httpapi/extgen/`, `web/src/api/gen/`); change `api/openapi.yaml` or
  `api/external-v1.yaml` and run `task generate`.
- Add links to new design documents from `docs/design-docs/index.md`.
- Before building or changing a screen, read
  [docs/design-docs/design-system.md](docs/design-docs/design-system.md); it
  routes to the registry of components, tokens and rules every screen uses.
- Before writing, moving or optimizing Go or Web tests, read
  [docs/design-docs/testing.md](docs/design-docs/testing.md) for shared scope,
  synchronization and fixture rules; it links to the Web-specific levels.
  Apply [write-tests](.agents/skills/write-tests/SKILL.md) before selecting
  cases, including tests added during implementation or review fixes without
  an explicit test request. SDD carries that selection in acceptance evidence.
- Name the unit contract before choosing its fixture. Use the minimum cases
  that expose its failure, fake unrelated dependencies, and assert arguments,
  results and required effects. Apply the
  [unit contracts](docs/design-docs/testing.md#unit-test-contracts) and
  [fixture rules](docs/design-docs/testing.md#fixtures-and-build-cost).
- Keep load and stress work out of unit cases, and use observable synchronization
  or virtual time instead of wall-clock waits. Remove duplicate or out-of-scope
  cases; do not move them to integration tests merely to retain them
  ([load and waiting](docs/design-docs/testing.md#load-and-waiting),
  [existing tests](docs/design-docs/testing.md#changes-to-existing-tests)).
- Give every pushed working branch a pull request as its review target.
- Dependency updates (Renovate) follow
  [docs/how-to/dependency-updates.md](docs/how-to/dependency-updates.md). The
  daily Renovate routine runs `.agents/skills/renovate-triage`, which may merge
  Renovate PRs into `main`, major updates included, under that skill's gates.
- SDD work starts when the maintainer hands over a parent Issue or native
  sub-issue URL, and runs `plan → design → plan-to-issues → implement →
  integrate`, with `design` only for `ui` Issues. Use
  `.agents/skills/issue-handoff`; it selects the next stage from GitHub and
  feature-branch state, and each run performs one stage and opens or updates
  one PR. Stage and implementation PRs target the
  long-lived feature branch, and only its integration PR targets `main`.
- To run a feature unattended up to (not including) the integration merge,
  the maintainer explicitly starts `.agents/skills/sdd-autopilot` with the
  parent Issue. It merges feature-branch PRs itself; never the integration PR.
- The long-lived feature branch is the one exception to giving every pushed
  branch its own pull request: until `integrate`, its review targets are the
  stage and implementation PRs into it. `integrate` opens its integration PR
  once every child is done.
- Refresh an integration PR by merging the latest `main` into its feature
  branch and rerunning the required checks, when it conflicts with `main` or a
  check fails because of `main`. Updating an existing PR branch does not get a
  separate PR. On the integration PR, fix only blocking review findings and
  list the rest for the maintainer
  ([integrate](.agents/skills/issue-handoff/references/integrate.md#review-of-the-integration-pr)).

## Code comments

- Write comments in concise English. Explain a non-obvious reason, invariant,
  constraint or workaround; describe public contracts when callers need them.
- Delete narration of the next statement, repeated type/function names,
  section dividers and obsolete history. Prefer clear names over explanations.
- Keep a comment to one or two sentences. Put lasting design rationale in
  `docs/design-docs/` and link to it instead of copying it into code.
- When touching existing comments, translate useful Japanese text, shorten it,
  and remove redundant text. Preserve compiler/tool directives, licenses,
  generated-code markers and meaningful quoted text or examples. Regenerate
  generated files from their source instead of editing their comments by hand.

## Review guidelines

- Flag bulk fixtures, repeated runs, wall-clock waits and real dependencies
  whose necessity is not explained by the tested contract. Check extracted
  rules retain a caller wiring case and removed integration tests are not
  claimed as covered by isolated unit tests
  ([testing policy](docs/design-docs/testing.md)).
- Treat as P1 a function the change adds or makes more complex whose
  complexity goes over 25 (gocyclo for Go; ESLint `complexity` with
  `variant: "modified"` for `web/src`), and code the change copies from
  elsewhere instead of sharing it. Tests, generated code and code the change
  only moves are exempt
  ([complexity-and-duplication.md](docs/design-docs/complexity-and-duplication.md)).
