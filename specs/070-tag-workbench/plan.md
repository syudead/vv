# Implementation Plan: Tag cards and tag pages

**Branch**: `feature/070-tag-workbench` | **Parent Issue**: #1480

**Input**: The parent Issue. It is this feature's specification.

## Summary

The tags screen becomes a grid of tag cards with the kind filter beside the
title and a strip of what is left to tidy (tentative, no kind, unused), and
each tag gets a page of its own (`/tags/:id`) where its names, kind and merge
are organized. The screen API gains a read of one tag for that page and three
synonym operations (make a synonym the canonical name, move a synonym to
another tag, detach a synonym into a new tag), and a `tags` server event lets an open screen say the list changed
without moving it. The design system gains a record card, a record editor
skeleton, kind colours and three optional `ListPage` slots for the tags
screen, and the list pieces other
screens borrow from `web/src/tags/` move to shared homes. The tags screen is
the only screen this feature rebuilds ([Scope](#scope)).

The diagram shows the parts this feature adds or changes.

```mermaid
flowchart LR
  list[Tag card list] -->|open| page[Tag page]
  list -->|batch, merge| api[Screen API]
  page -->|synonym operations, merge| api
  api --> store[TagStore]
  api -->|canonical name change| peer[Paired VVMDM]
  store -->|after commit| bus[Event bus]
  bus -->|tags event| list
  bus -->|tags event| page
  ds[RecordCard, RecordEditorPage, kind colours, ListPage slots] --> list
  ds --> page
  shared[web/src/recordList, web/src/ui] --> list
  shared -->|imports only| others[People, organizations, settings, history]
```

## Scope

The feature covers the tags screen only: `/tags`, `/tags/:id`, `/tags/kinds`
and `/tags/rejected`. No other screen is a target.

| Screen or piece | In this feature |
| --- | --- |
| People, organizations, settings, history | Import paths change when the pieces they borrow leave `web/src/tags/` ([Shared list modules](#shared-list-modules)); behaviour and layout do not |
| Organizations | Keep `AdminTablePage` and the details dialog; no card list and no `/organizations/:id` page |
| Assistant review page | Unchanged; it keeps `AdminTablePage` |
| `AdminTablePage` | Stays in the design system for the organizations screen and the assistant review page |
| `ListPage` | Gains optional `filter`, `summary` and `notice` slots that only the tags screen fills; the people screen passes none and does not change |

Three lines of the parent Issue are met in part, and the maintainer decides
whether to reword them in the Issue.

| Issue line | State after this feature |
| --- | --- |
| Requirement 9 and acceptance 8 (no pattern left that copies the old tags screen) | Met for the tags screen only: it no longer uses `AdminTablePage`, and the pattern and its row in the usage table remain for the organizations screen and the assistant review page |
| Requirement 10 (move the people screen onto the new pattern) | The people screen does not change; it only stops importing from `web/src/tags/` |
| Acceptance 6 (the people screen's operations work) | Covered by the existing people tests passing unchanged apart from import paths |

## Technical Context

**Canonical definitions**: [ARCHITECTURE.md](../../ARCHITECTURE.md) for
layers, store roles, events after commit and the web layer;
[catalog-organization.md](../../docs/design-docs/catalog-organization.md) for
tags, tag kinds, search keys and folder-derived tags;
[peer-library.md, Vocabulary by name](../../docs/design-docs/peer-library.md#vocabulary-by-name)
for writes on a paired node; [library-ui.md](../../docs/design-docs/library-ui.md)
for the current tags and organizations screens, virtualization and URL
criteria; [design-system.md](../../docs/design-docs/design-system.md) and
[web/registry/rules/patterns.md](../../web/registry/rules/patterns.md) for the
page patterns; [api/openapi.yaml](../../api/openapi.yaml) for the screen API;
[testing.md](../../docs/design-docs/testing.md) and
[web-testing.md](../../docs/design-docs/web-testing.md) for test levels.

**Feature-specific context**:

| Item | Delta |
| --- | --- |
| Synonym storage | A synonym is a `tag_names` row with `canonical = 0`; names are unique across canonical names and synonyms. The new operations rewrite rows and add no table or migration |
| Scale | The tag list stays responsive with 5,000 tags: server pages of 100 and a row-virtualized card grid ([Tag list and tag page](#tag-list-and-tag-page)) |
| Bulk limit | `MaxTagBatch` (20,000) stays the limit for bulk actions and merges |
| Change notice | There is no tag event today; the screen reloads only after its own writes and on `peerItems` ([Change notice](#change-notice)) |
| Reading one tag | The screen API has no read of one tag today; `listTags` searches names by substring ([Tag list and tag page](#tag-list-and-tag-page)) |
| External API and MCP | Unchanged; the new operations are screen API only ([Synonym operations](#synonym-operations)) |

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Rule | Source | Verdict |
| --- | --- | --- |
| The domain decides, the store enforces | ARCHITECTURE.md | The rules for each synonym operation (which names may move, what becomes confirmed) are in `internal/domain`; `TagStore` re-reads the source, the name and the target in the operation's transaction |
| Store roles | ARCHITECTURE.md | The three operations are `TagStore` methods; no other role gains tag writes |
| Events after commit | ARCHITECTURE.md | `domain.TagsChanged` is published only after a committed tag or attachment write |
| Every read knows its viewer | ARCHITECTURE.md | The new operations are owner-only in `api/openapi.yaml`, so the routing test covers them; the `tags` event goes to owners only |
| Remote ids carry an offset | ARCHITECTURE.md | The synonym operations answer 404 for a remote id, as synonym edits do today; canonical-name changes reach the peer by name ([Synonym operations](#synonym-operations)) |
| Pages never call `fetch` | ARCHITECTURE.md, Web layer | `web/src/api/tags.ts` is the only caller of the new operations |
| Generated files are not hand-edited | AGENTS.md | `api/openapi.yaml` changes, then `task generate` |
| Complexity 25 and no copies | AGENTS.md | The pieces credits and settings borrow from `web/src/tags/` are moved, not copied ([Shared list modules](#shared-list-modules)) |
| Design system first | AGENTS.md, design-system.md | `RecordCard`, `RecordEditorPage`, the kind colours, the `ListPage` slots and the components that fill them are added to the registry before the tags screen uses them |

No gate is at stake after Phase 1; `## Complexity Tracking` is left out.

## Synonym operations

Three owner-only operations under `/api/tags/{id}/synonyms/` change one
synonym of a tag in one transaction each
([contracts/tag-synonym-operations.md](contracts/tag-synonym-operations.md)).
"Make this tag a synonym of another tag" is the existing merge with the other
tag as the target; the tag page offers it as merge into another tag, and the
list's selection bar as merge into one tag (requirement 6).

The table shows what each operation changes.

| Operation | Names | Explicit attachments | Tentative state |
| --- | --- | --- | --- |
| Make canonical (`promote`) | The synonym becomes the canonical name; the old canonical name becomes a synonym | Unchanged | The tag is confirmed |
| Move (`move`) | The synonym leaves the source and becomes a synonym of the target | Stay on the source | The target is confirmed; the source is unchanged |
| Detach (`detach`) | The synonym leaves the source and becomes the canonical name of a new tag | The new tag has none | The new tag is confirmed; the source is unchanged |

| Decision | Rejected | Why |
| --- | --- | --- |
| Three explicit operations, each naming the synonym and, for a move, the target id | `PATCH /api/tags/{id}` renaming to the tag's own synonym swaps the names | A rename of a canonical name is also the peer's by-name write and the external API's `rename_tag`; giving an existing call a second meaning changes them all |
| Same | Replacing a tag's whole synonym list in one request | A list written from a stale screen silently undoes changes made elsewhere; one named synonym fails cleanly when it is gone |
| A synonym that is no longer the tag's answers `409 tag_synonym_not_found`, a missing source or target `404 tag_not_found`, and nothing changes | Applying what still applies | The Issue's edge case asks for no change and a reason when the other side moved |
| Moving to the source itself answers `400 invalid_request` with reason `move_same_tag` | Treating it as a no-op | The screen never offers it, so a request that asks for it is a client error, like `merge_same_tag` |
| Explicit attachments stay with the tag id; videos whose folder name equals the moved or detached name follow the name, as folder-derived tags always do | Moving attachments with the name | `video_tags` does not record which name attached a video (the Issue's edge case for detach) |
| Every operation publishes the smart-folder notice and the `tags` event after commit, as merge does | Only the `tags` event | Moving a name changes folder-derived matches, which smart folders read |
| On a paired node, `promote` writes the peer first by canonical name with the new action `promote`, then the local node; `move` and `detach` stay local | All three stay local | Tags join across nodes by canonical name, so a local-only promote splits one shown tag into two; synonyms and creation are per node already ([peer-library.md](../../docs/design-docs/peer-library.md#vocabulary-by-name)) |
| The peer's `promote` swaps when the peer's tag holds the new name as a synonym, and otherwise makes the new name canonical; it keeps the old canonical name as a synonym either way, and answers `changed: false` when its tag of the new name already holds the old name | Plain rename on the peer | A rename to the peer's own synonym is a conflict, the two nodes may hold different synonyms, a dropped old name loses the peer's folder-derived tagging by that name, and a repeated promote after a partial one must not conflict with its own earlier write |
| No external API or MCP change | Adding the operations to `/api/v1/tags/synonyms` | The Issue is about the screen; an external contract change is its own versioned decision |

## Change notice

A new owner-only server event `tags` (an empty object) is sent after a
committed change to the tag list's rows: tag names, synonyms, kinds, tentative
state, creation, deletion, merges and video attachments, including automatic
tagging ([contracts/tags-event.md](contracts/tags-event.md)).

The diagram shows what the open tag list does with the event.

```mermaid
flowchart LR
  ev[tags event] --> busy{Own write in flight}
  busy -->|yes| wait[Check after it settles]
  busy -->|no| check[Re-read the loaded rows]
  wait --> check
  check --> same{Differs from shown rows}
  same -->|no| keep[Nothing]
  same -->|yes| notice[Show the changed notice]
  notice -->|owner refreshes| reload[Reload with the same criteria]
```

| Decision | Rejected | Why |
| --- | --- | --- |
| A dedicated `tags` event, coalesced per subscriber like `candidates` | Reusing the all-folders `smartFolder` event | That event also fires for people, favorites and watch state, and reaches guests |
| The list re-reads the cards it has loaded, with its criteria, in pages of 200 (`MaxTagPageLimit`), and compares them and the totals with what it shows, and shows a notice to refresh | Reloading on the event | The Issue's edge case forbids moving the list on its own |
| Same | Re-reading the first page only | A synonym added elsewhere to a loaded tag past the first page changes neither the first page nor the totals, so the notice would not appear |
| Same | A vocabulary revision returned by every write and carried by the event | Every write path, including scans that change folder-derived counts, would have to bump it in its transaction; comparing the loaded rows needs no new state, and 5,000 loaded rows are 25 reads after an event |
| The check waits for the list's own writes to settle | Ignoring events for a fixed time after a write | Own writes are applied to the shown rows first, so a later check matches them; a timer either hides other writes or shows a notice for its own |
| Folder-derived count changes from a scan send no `tags` event | Sending it on every video change | A scan changes thousands of videos; the existing continuation check (`totalAll`) still catches a list that changed under paging |

## Tag list and tag page

`/tags` is a `ListPage` whose body is a grid of tag cards; a card opens the
tag's page `/tags/:id`, where every operation on one tag lives. Operations on
several tags stay on the list through the selection bar. Kinds and rejected
names move to pages of their own (`/tags/kinds`, `/tags/rejected`), reached
from the list's header. The layout, states and widths are decided in
`ui-design.md`.

The diagram shows where each operation lives.

```mermaid
flowchart LR
  list[Tag list] -->|select cards| bulk[Selection bar: merge, kind, confirm, reject, delete]
  list -->|open a card| page[Tag page]
  page --> names[Names: make main, move, detach, remove, add]
  page --> kind[Kind]
  page --> merge[Merge into another tag]
  page --> acts[Confirm, rename, reject, delete]
  list -->|header| kinds[Kinds page]
  list -->|header| rejected[Rejected names page]
```

| Decision | Rejected | Why |
| --- | --- | --- |
| Tags are cards: name, video count, kind colour, synonyms; a card is one fixed height | Table rows, as today | The maintainer's review: rows read as old and plain; cards of one fixed height still show about 24 tags at 1280×820 and keep virtualization simple |
| Same | A list beside a detail pane (the first design) | Reviewed as an extension of today's screen; the pane crowds both the list and the tag's work |
| One tag's work is a page of its own, `/tags/:id`, using the whole width | A side panel over the list | The list's own operations (selection bar, filters) cover the many-tag work; one tag's names, kind and merge need the room, and the page has a URL to return to |
| Opening a card holds an in-memory snapshot of the list (loaded cards, cursor, totals, scroll position) under its criteria, one at a time, as the library's `listSnapshot` does; Back restores it when the URL's criteria match, then runs the change check of [Change notice](#change-notice) | Restoring from the URL's criteria only | The criteria give the first page only; a card opened past it could not be scrolled back to without its loaded pages |
| Same | Keeping the list mounted behind the page | The library already returns from a video this way and needs no second mounted screen |
| A write on the tag page drops the snapshot, so Back reloads the list with the same criteria from the top | Restoring the snapshot and patching the written tag into it | A merge, detach or kind change can move cards across pages; the library drops its snapshot after a write for the same reason |
| The kind filter sits beside the title as one segmented control with counts; tentative, no kind and unused show as a strip of three counts that each apply their filter | The filter popover of today | The counts are what the owner checks first ("how many are left"), and the review asked for the filters next to the header |
| Search, the filters and sort stay in the URL as today (`q`, `tentative`, `unused`, `kind`, `sort`); `tab` is dropped because kinds and rejected names are pages | Keeping `tab` | Requirement 8 covers the criteria; separate pages have their own URLs |
| The card grid keeps server pages of 100 and is virtualized by rows (`@tanstack/react-virtual`), with the column count taken from the grid's width | A wrapping grid that keeps every loaded card | The tag list froze with thousands of rows before it was virtualized ([library-ui.md](../../docs/design-docs/library-ui.md)); a fixed card height makes a row the unit |
| Selecting cards follows the library's selection (check on hover, Shift for a range, the bottom selection bar); bulk selection stops at 20,000 before running, as today | A separate selection mode | One selection model across the app; requirement 2 keeps today's limit |
| The tag page reads its tag with a new owner-only `GET /api/tags/{id}` (`getTag`) on open and on the `tags` event; `404 tag_not_found` is the gone state ([contracts/tag-synonym-operations.md](contracts/tag-synonym-operations.md)) | Taking the tag from the list's loaded cards, or from `listTags` with its name as `q` | The page opens from its URL without the list; a name search matches substrings and synonyms, and cannot tell a renamed tag from a deleted one |
| The tag page re-reads its tag on the `tags` event when no own write is in flight, and shows the gone state when the tag no longer exists | A notice to refresh, as on the list | One tag's page has no positions to keep, so updating in place moves nothing |
| A stale target (`tag_not_found`, `tag_synonym_not_found`) shows the reason, and the page or list reloads | Retrying | The Issue's edge case |

## Design-system pieces

The design system gains a record card, a record editor skeleton, kind colours,
and three optional `ListPage` slots with the components that fill them; tokens
other than the kind colours and the shadcn components do not change.
`AdminTablePage` stays ([Scope](#scope)). `ui-design.md` decides how each
piece looks.

| Piece | Item | What it is |
| --- | --- | --- |
| `RecordCard` | `record-card` | A fixed-height card for one named record: name, count, a kind colour mark, a line of secondary names, a selection check |
| `CardGrid size="record"` | `card-grid` | The grid of record cards, at least `card-record` wide |
| `RecordEditorPage` | `record-editor-page` | A new skeleton for organizing one record: a back link, the title with its figures and a row of actions, a two-column body of its parts, then a `Shelf` of the items it is on |
| Kind colours | `vv-theme` | Eight colour tokens `kind-1` … `kind-8`, each with a `-soft` fill, for the kind mark, the kind filter and the kind picker |
| `ListPage` `filter`, `summary` and `notice` slots | `list-page` | Optional slots for the list's main filter beside the title, a strip of tallies between the header and the body, and a notice over the body that does not move the list |
| `TallyStrip` | `tally-strip` | Up to three counts that each toggle a filter, for the `summary` slot |
| `FloatingNotice` | `floating-notice` | One line and one action over the body, for the `notice` slot |
| `KindMark` | `kind-mark` | A kind's colour as a card's edge (`bar`) or before its name (`dot`) |
| `KindToggleGroup` | `kind-toggle-group` | A single-choice group of kinds with their marks plus plain choices, for the `filter` slot and the kind picker |

| Decision | Rejected | Why |
| --- | --- | --- |
| A new skeleton, `RecordEditorPage` | Widening `RecordPage` | `RecordPage` is for reading one record and rules out operations on it in its actions; the activity entry depends on that |
| Same | `ProfilePage` | It puts viewing first and sends work on the subject to another page ([patterns.md, Profile page](../../web/registry/rules/patterns.md#profile-page)); here the work is the point |
| A kind takes `kind-N` from its id, N = (id − 1) mod 8 + 1, so a kind keeps its colour when others are created or deleted | A colour the owner picks | A picker is a new setting and API field; the Issue asks for none |
| Same | Its position among the kinds ordered by id | Deleting a kind with a lower id shifts every later kind to another colour |
| Same | One colour for all kinds | The review chose cards with a kind colour; telling kinds apart at a glance is the reason |
| `AdminTablePage`, its example and its rules stay; the tags screen stops using it | Removing it in this feature | The organizations screen and the assistant review page still use it, and neither is a target; the organizations screen is rebuilt under its own Issue (the parent Issue's out-of-scope list) |
| The list's filter, tallies and change notice are optional `ListPage` slots | Building them inside the tags screen | Other lists can take them up later without copying the tags screen; a screen that passes none renders as today |
| The organizations screen keeps its table and details dialog | A card list at `/organizations` and an organization page `/organizations/:id` | The maintainer limited the feature to the tags screen |
| The people screen keeps `ListPage` with face cards and only stops importing from `web/src/tags/` | Moving people onto record cards | Same; people are also browsed by face ([people.md](../../docs/design-docs/people.md)) |

## Shared list modules

The pieces other screens import from `web/src/tags/` move before the screen is
rebuilt, so the rebuild deletes the old files without breaking the people,
organizations, settings and history screens. The move changes import paths
only.

| Piece | New home |
| --- | --- |
| `usePagedRows`, `useSearchQuery`, `moreIdle` and their types, `useUrlCriteria`, the generic row helpers and types of `tagPageRows.ts` with `compareTagsForSort`, `tagListView` and its `TagListEmpty`, the virtual rows and their focus handling, `useAdminTableBand`, `AdminListToolbar`, list notices, `TagsEmpty`, the `TagFieldError` type | `web/src/recordList/` |
| `DialogError`, `RowMoreMenu` with `TagRowRefs`, `RowIconButton`, `RowNameInput`, `CreateTagRow`, `MergeTargetField`, `MergeSummary` | `web/src/ui/` as registry components |
| `TagListSort`, its default and its guard, used by preferences | `web/src/api/tags.ts` |

Any other export that a file outside `web/src/tags/` imports when the unit
starts moves with the piece it belongs to; the unit's acceptance finds one
left behind.

| Decision | Rejected | Why |
| --- | --- | --- |
| A new `web/src/recordList/` for list data hooks, beside `videoList/` | `web/src/lib/` | `lib/` holds helpers without screen state; these hooks page through the API |
| Move first, in its own unit | Moving during the rebuild | A pure move is reviewable by diff, and the rebuild unit then only deletes |

## Project Structure

### Documentation (this feature)

```text
specs/070-tag-workbench/
├── plan.md
├── quickstart.md                          # Checks a person makes: 5,000 tags, the change notice
└── contracts/
    ├── tag-synonym-operations.md          # promote, move, detach, peer promote
    └── tags-event.md                      # the tags server event
```

No `research.md`: every decision and its rejected alternatives fit in this
plan's tables. No `data-model.md`: no table or entity is added.
`ui-design.md` is written by the `design` stage.

### Source Code

**Affected boundaries**: `internal/domain` (synonym rules, `TagsChanged`),
`internal/store` (`TagStore` operations, publishing), `internal/httpapi`
(handlers, peer promote, `tags` event), `api/openapi.yaml`, `web/src/api/`,
`web/src/tags/`, `web/src/credits/`, `web/src/settings/`,
`web/src/history/`, `web/src/preferences/`, `web/src/ui/`, `web/registry*`,
`web/e2e/tags.e2e.ts`.

**New paths**: `web/src/recordList/`, `web/src/ui/RecordCard.tsx`,
`web/src/ui/patterns/record-editor-page.tsx`, the `TallyStrip`,
`FloatingNotice`, `KindMark` and `KindToggleGroup` components under
`web/src/ui/`, their example blocks under
`web/src/designSystem/blocks/`, `docs/design-docs/tag-organizing.md`.

**Structure decision**: Follows the existing layout
([ARCHITECTURE.md, Web layer](../../ARCHITECTURE.md#web-layer)), plus
`recordList/` as decided in [Shared list modules](#shared-list-modules).

## Implementation Work

The graph shows the units and what each waits for.

```mermaid
flowchart LR
  ops[Synonym operations API] --> page[Tag page]
  ev[Tags server event] --> list[Tag card list]
  move[Shared list modules] --> views[Kinds and rejected names pages]
  ds[Record card, editor page, kind colours] --> views
  views --> list
  move --> list
  ds --> list
  list --> page
  list --> bulk[Selected tag cards]
  page --> e2e[End to end]
  bulk --> e2e
  views --> e2e
```

### Read one tag and make a synonym canonical, move it or detach it through the screen API

**Scope**: `getTag`, the three operations and the peer `promote` action in
[contracts/tag-synonym-operations.md](contracts/tag-synonym-operations.md):
domain rules, `TagStore` methods, handlers, `api/openapi.yaml` and
`task generate`, and their wrappers in `web/src/api/tags.ts`. Adds
`docs/design-docs/tag-organizing.md` with the operations table and decisions
of [Synonym operations](#synonym-operations), linked from the index.

**Dependencies**: None

**Acceptance**: Store tests show, per operation, the names and tentative state
after it, attachments unchanged for promote and move, none on a detached tag,
and no change with `tag_synonym_not_found` or `tag_not_found` when the synonym
or target is gone. A video-list test filters by a promoted, moved and detached
name and gets the tag named in the table. Handler tests cover `getTag` answering
the `Tag` that `listTags` gives and `404 tag_not_found` for a missing or remote
id, `move_same_tag`, a remote id answering 404, and on a paired pair of test servers that `promote`
changes the canonical name on both and `move` only locally. `web/src/api`
tests assert each wrapper's request.

### Send a tags server event when the tag list changes

**Scope**: `domain.TagsChanged`, its publication after commit from every tag,
kind and attachment write, and the owner-only `tags` SSE event in
[contracts/tags-event.md](contracts/tags-event.md); the event type in
`web/src/api/serverEvents.ts`. Adds the event to
`docs/design-docs/tag-organizing.md` or creates the file if the synonym
operations unit has not landed.

**Dependencies**: None

**Acceptance**: A store test shows one event after a committed rename,
attachment and kind change, and none after a rolled-back write. An HTTP test
reads `event: tags` for the owner after a write and nothing for a guest.

### Move the list pieces other screens use out of web/src/tags

**Scope**: The moves in [Shared list modules](#shared-list-modules), with
their tests, and every importer in `web/src/credits/`, `web/src/settings/`,
`web/src/history/`, `web/src/preferences/` and `web/src/testing/`. Moved
`web/src/ui` pieces become registry items. No behaviour changes.

**Dependencies**: None

**Acceptance**: No file outside `web/src/tags/` imports from it except the
tags screen's own entry points: the `/tags` route in `web/src/app/App.tsx` and
`TagsPage` in its test fixture `web/src/testing/tagsPage.tsx`
(`grep -rnE 'from "(\.\./)+tags/|import\("(\.\./)+tags/' web/src --include='*.ts*' | grep -v '^web/src/tags/'`
finds only those two lines), `task check SCOPE=web` passes with the existing credits, people,
settings and history tests unchanged apart from import paths, and
`task generate-check` passes.

### Add the record card, the record editor page and kind colours to the design system

**Scope**: `RecordCard` and `CardGrid size="record"` with the `card-record`
step, the `RecordEditorPage` skeleton, the `kind-1` … `kind-8` tokens with
their `-soft` fills, the `ListPage` `filter`, `summary` and `notice` slots, and
the `TallyStrip`, `FloatingNotice`, `KindMark` and `KindToggleGroup`
components, following [Design-system pieces](#design-system-pieces) and
`ui-design.md`: components, example blocks, registry entries, the rows in
`patterns.md`, `components.md` and `foundations.md`, the contrast pairs in
`web/src/theme/tokens.test.ts`, and their showcase entries. `AdminTablePage`
and its rows are not changed, and a `ListPage` that passes none of the new
slots renders as before.

**Dependencies**: None

**Acceptance**: `task generate-check` passes with `record-card.json`,
`record-editor-page.json`, `tally-strip.json`, `floating-notice.json`,
`kind-mark.json` and `kind-toggle-group.json` built and `list-page.json`
updated; `tokens.test.ts` checks each kind colour against `card` and each
`kind-N-soft` fill with `foreground` text; component tests show a pressed
tally and a pressed kind toggle reporting their choice, and the existing
people screen tests pass unchanged; the showcase renders the examples at the
widths `ui-design.md` names, reviewed visually (card states, the editor page's
two columns and their narrow stack, the `ListPage` slots filled at `lg` and
below).

### Rebuild the tags list as kind-filtered tag cards

**Scope**: `/tags` as a `ListPage` of tag cards: the kind filter with counts
beside the title, the tidy-up strip (tentative, no kind, unused), search and
sort in the top bar, their URL, create, the row-virtualized card grid, empty
states, the change notice of [Change notice](#change-notice), the snapshot
kept for Back from a tag page, links to the kinds and rejected names pages,
and deletion of the old screen's files and
tests, including `web/src/testing/tagsPage.tsx`. Updates the tags part of
[library-ui.md](../../docs/design-docs/library-ui.md), and any sentence in
the design docs or registry rules that names the tags screen as a caller of
`AdminTablePage`.

**Dependencies**: Send a tags server event when the tag list changes; Move the
list pieces other screens use out of web/src/tags; Add the record card, the
record editor page and kind colours to the design system; Give tag kinds and
rejected names pages of their own

**Acceptance**: Component tests: criteria round-trip through the URL and are
restored on reload and Back (acceptance 5); Back from a card opened past the
first page restores the same loaded cards and scroll position without a
request for them, and after a write on the tag page reloads with the same
criteria; a tidy-up count applies its
filter; an event whose re-read differs in a loaded card past the first page
shows the notice and leaves the cards in place, one whose re-read matches
shows nothing, and one during an own write is checked after it; the two empty
states. `grep -r AdminTablePage web/src/tags` finds nothing. A visual review
at the `ui-design.md` widths.

### Act on selected tag cards

**Scope**: Card selection (check, Shift range, select all loaded and all
matching), the selection bar with merge into one tag, kind, confirm, reject
and delete, the over-limit stop, and the impact counts before reject, delete
and merge.

**Dependencies**: Rebuild the tags list as kind-filtered tag cards

**Acceptance**: Component tests assert each bulk request, the cards after it,
`notFoundIds` handled as stale, and that a selection over 20,000 is stopped
before any request. A visual review of the selection states.

### Add the tag page with its names, kind and merge

**Scope**: `/tags/:id` on `RecordEditorPage`: the title with rename, the
actions (confirm, reject, delete), the names with make main, move, detach,
remove and add, the kind picker, merge into another tag, the row of the tag's
videos with the link to the library, the gone state, in-place refresh on the
`tags` event, and stale-target handling.

**Dependencies**: Read one tag and make a synonym canonical, move it or detach
it through the screen API; Rebuild the tags list as kind-filtered tag cards

**Acceptance**: Component tests with a fake API assert each control's request
and the page after it: opening `/tags/:id` directly reads the tag with
`getTag`, and a `404 tag_not_found` shows the gone state; make main shows the new name with the old one among the
names; move removes the name; detach links to the new tag; merge returns to the
list without the tag; a `tag_synonym_not_found` or `tag_not_found` answer
shows the reason and reloads; an event re-reads the tag. A visual review of
the page's states.

### Give tag kinds and rejected names pages of their own

**Scope**: `/tags/kinds` (create, rename, delete, card-first, each kind's
colour and its link to the filtered list) and `/tags/rejected` (list,
forget). They replace the current screen's Kinds and Rejected names tabs, which
become links to them, so both stay reachable until the card list replaces that
screen and keeps the links in its header.

**Dependencies**: Move the list pieces other screens use out of web/src/tags;
Add the record card, the record editor page and kind colours to the design
system

**Acceptance**: Component tests: the tags screen links to both pages; deleting
a kind leaves its tags with no kind in the list; forgetting a rejected name removes it from the page. A visual
review of both pages.

### Cover the tag screens end to end

**Scope**: Rewrite the tag-screen tests of `web/e2e/tags.e2e.ts` for the
card list and the tag page (the card overflow test there stays), and run
[quickstart.md](quickstart.md).

**Dependencies**: Act on selected tag cards; Add the tag page with its names,
kind and merge; Give tag kinds and rejected names pages of their own

**Acceptance**: The e2e spec, run through `workflow_dispatch` of `ci.yml`,
shows that after make main on the tag page the library card shows the new
name and a search for the old name finds the same videos (acceptance 1),
after move tagging a video by the moved name attaches the target
(acceptance 2), and after detach the new tag is in the list (acceptance 3).
The quickstart's 5,000-tag check is recorded in the PR (acceptance 4).
