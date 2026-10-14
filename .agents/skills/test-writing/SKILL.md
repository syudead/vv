---
name: "test-writing"
description: "Write or move the tests for a finished code change at the level the repository's test policy sets, with the reason for every slow test. Use after the implementation of an Issue or child Issue, before the repository checks."
---

# Test writing

Tests are written from the behaviour the source of truth asks for, at the
lowest level that fails when that behaviour breaks. Every test above that level
lengthens the CI run of every later pull request: a Web page test costs about
35 logic tests, and a Go store test about 20
([go-testing.md](../../../docs/design-docs/go-testing.md),
[web-testing.md](../../../docs/design-docs/web-testing.md)).

## Steps

1. Read `docs/design-docs/go-testing.md` for Go and
   `docs/design-docs/web-testing.md` for the Web client, whichever the change
   touches. Read them now, not from memory.
2. List the behaviours from the source of truth: the Issue's `受け入れ条件`
   and `要件`, the bug report's steps, or the Plan's lines. Do not list them
   from the code; a list from the code tests what was written, not what was
   asked.
3. Fill the level table, one row per behaviour, before writing any test:

   | Behaviour | Level | Why the level below would not fail | Test file |
   | --- | --- | --- | --- |

   Walk each behaviour through the "Choosing the level" diagram of the policy.
   The third column is required for a Go store, HTTP or media test and for a
   Web page or E2E test. "The rule is computed in the page" or "in the
   handler" is not a reason: move the rule to a pure module or function in the
   same change and test it there, keeping one case at the higher level that
   shows the rule is used. When moving it is outside the write scope you were
   given, stop and return that as a blocker.
4. Write the tests.
   - Add a case to an existing test that already sets up the same fixture
     before adding a new top-level store or HTTP test; read-only cases share
     one database as subtests.
   - A media clip is the shortest and smallest that shows the behaviour.
   - Wait on a channel, a callback or a fake clock, never `time.Sleep`.
   - No snapshot tests.
   - When the change touches a page, move the existing tests of the rules it
     touches to the lower level, in the same change (web-testing.md, Existing
     tests).
5. Prove each new test can fail. For a bug fix, run the test before the fix
   and see it fail. Otherwise revert the implementing lines, run the test, see
   it fail, and restore them. A test that passes either way is deleted or
   rewritten.
6. Run `task test-cost -- -base <base> -measure`, where `<base>` is the
   branch the pull request targets: `origin/main`, or `origin/<feature branch>`
   for an SDD child, so tests already merged into the feature branch are not
   listed again. Every row it prints at store, http, media, page or e2e level,
   including a Go test marked `(cases added)`, must be in the level table with
   its reason, and a test over 1 s states why it needs that long or is cut
   down.
7. Run the focused tests of the changed packages and files.

## Return

- The level table.
- The output of `task test-cost -- -base <base> -measure`.
- The focused test commands and their results, including the failing run of
  step 5.
- Any blocker: a behaviour that cannot be tested at the level the policy sets
  without a change outside the write scope.

The parent agent copies the level table into the pull request body under
`テスト`.
