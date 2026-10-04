# Quickstart: Check the design system end to end

These steps prove acceptance criteria 1 to 4 and 6 of parent Issue #757, which
`task check` and `task test-e2e` do not show on their own: that the
confirmations exist, that the registry answers through shadcn, that an agent
finds it from `AGENTS.md`, that each kind of violation fails, and that the
five screens keep their operations. Criterion 5 is `task check` and
`task test-e2e` passing on the final feature branch.

## Prerequisites

- A checkout of the feature branch after the last implementation unit, with
  `npm --prefix web ci` run ([docs/how-to/development.md](../../docs/how-to/development.md)).
- For step 6, a library with at least one folder of videos, tags and a
  duplicate bundle, as for `task dev`.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| 1. Open the foundations, components and page patterns PRs into the feature branch | Each has an approving review by the maintainer and `/design-system` screenshots at 1440 px and 390 px | 1 |
| 2. In `web/`, run `npx shadcn view ./registry/r/vv.json`, then `npx shadcn view ./registry/r/button.json` | The first lists every item; the second prints the Button source and a `docs` line naming its section in `components.md` | 2 |
| 3. In a Claude Code session in the repository, call the shadcn MCP tool `view_items_in_registries` with `./registry/r/vv-rules.json` | The three rule files come back | 2 |
| 4. Start a fresh agent session and ask it, without naming files, where to look before building a screen | It names `docs/design-docs/design-system.md` from `AGENTS.md` and, from there, the registry | 3 |
| 5. In any screen file outside `web/src/ui`, add in turn `<button>`, `className="h-[3px]"`, `className="p-7"` and `className="text-[#fff]"`, running `task lint-web` and `task test-web` after each and reverting | Each run fails with the message in [contracts/registry.md](contracts/registry.md#check-rules) | 4 |
| 6. With `task dev`, walk the operation list in the body of each screen's migration PR (the library's is in the page patterns PR) | Every listed operation, recorded there as working on `main`, still works | 6 |
