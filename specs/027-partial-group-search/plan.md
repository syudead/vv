# Implementation Plan: Show each matching video when a library search matches only some members of a group

**Branch**: `feature/027-partial-group-search` | **Parent Issue**: #523

**Input**: The parent Issue. It is this feature's specification.

## Summary

Change how `GET /api/library` builds its items. When a search query or tag filter is present and only
some members of a group match, the group card is not shown; each matched member becomes its own video
item. Only when every member matches does the group card show, as before. `GET /api/library/ids`
follows the same items.

The decision is made inside the SQL that builds items in `internal/store` (`libraryItemsCTE`;
[research.md R-1](research.md#r-1-the-decision-is-made-in-the-sql-stage-that-builds-items)). Only the
search query and tags decide; playability and watch status still apply to items as before
([R-2](research.md#r-2-only-the-search-query-and-tags-decide-playability-still-applies-to-items-as-before)).
The response schema does not change, and the rule delta is in
[contracts/library-api.md](contracts/library-api.md)
([R-3](research.md#r-3-017s-artifacts-stay-unchanged-the-current-rule-lives-in-this-features-contract-and-architecturemd)).
The screen code does not change: a member shown on its own is a `kind: video` item, and the library
screen chooses how to render by item kind alone (`web/src/api/libraryItems.ts`,
`web/src/library/LibraryPage.tsx`).

## Technical Context

**Canonical definitions**:

| Area | Source |
| --- | --- |
| Boundaries, dependency direction, the role of `LibraryStore`, the current library list rules | [ARCHITECTURE.md](../../ARCHITECTURE.md) (the `GET /api/library` paragraph in "Intended topology" and the `LibraryStore` entry) |
| How items are built today and what each viewer sees | [specs/017-folder-groups/data-model.md §5–§7](../017-folder-groups/data-model.md#5-library-items), [specs/017-folder-groups/contracts/library-api.md](../017-folder-groups/contracts/library-api.md) |
| Implementation | [internal/store/library_items.go](../../internal/store/library_items.go) (`libraryItemsCTE`, `ListLibrary`, `LibraryIDs`), [internal/store/listing.go](../../internal/store/listing.go) (`chosenLocationsCTE`, `listSpec`), [internal/httpapi/library.go](../../internal/httpapi/library.go) |
| API source of truth and generated code | [api/openapi.yaml](../../api/openapi.yaml) (`listLibrary`, `listLibraryIds`), `task generate` ([Taskfile.yml](../../Taskfile.yml)) |
| How the screen holds the list and renders items | [web/src/api/libraryItems.ts](../../web/src/api/libraryItems.ts), [web/src/api/useVideos.ts](../../web/src/api/useVideos.ts), [web/src/library/LibraryPage.tsx](../../web/src/library/LibraryPage.tsx) |
| Browser tests | [web/e2e/guest.e2e.ts](../../web/e2e/guest.e2e.ts) (`task test-e2e`; CI runs it only on pushes to `main`) |

**Feature-specific context**:

- No dependency, migration, table or response schema is added or changed. What changes is how
  `GET /api/library` and `GET /api/library/ids` build their items (SQL), and the documents that
  describe it.
- The owner scenario in `web/e2e/guest.e2e.ts` assumes that a search for `ゲスト` shows `非公開だけ`
  (of 7 videos, only D matches) as a group card (`ownerGroups`). Under the new rule D shows as a video
  card, so the implementation unit fixes this premise. e2e runs only on pushes to `main`, so finding
  out after integration would be too late.

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended topology", depguard in `.golangci.yml`): pass.
  Only the read in `internal/store` (`libraryItemsCTE`) changes; the types and interfaces of
  `internal/domain` and `internal/httpapi` do not.
- **API source of truth and generated code** (AGENTS.md, ARCHITECTURE.md): pass. The description in
  `api/openapi.yaml` changes, and the generated code is rebuilt with `task generate`. The schema does
  not change.
- **Viewer-specific conditions go through `visibleLocationCondition`** (the `LibraryStore` entry in
  ARCHITECTURE.md, 017 §7): pass. "All members" is counted over the current `gm` (members the viewer
  may see), and for guests it is decided over public members only
  ([contracts/library-api.md §1](contracts/library-api.md#1-how-get-apilibrary-builds-items)).
- **Constraints are checked by tests** (core-beliefs.md): pass. Store tests fix the rule, and e2e
  checks how it looks on screen.
- **Documents change in the same PR as the code** (core-beliefs.md, AGENTS.md): pass. The
  implementation unit updates the ARCHITECTURE.md paragraph and the `api/openapi.yaml` description
  (R-3).

The verdicts are the same after Phase 1. There is no violation for Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/027-partial-group-search/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-3
└── contracts/
    └── library-api.md    # Delta in how GET /api/library and /ids build items (replaces items 1–2 of 017 §5)
```

No `data-model.md` (no entity or table changes). No `quickstart.md` (store tests and `task test-e2e`
check the acceptance criteria, and there are no feature-specific manual steps).

### Source Code

**Affected boundaries**:

- `internal/store` (`libraryItemsCTE` in `library_items.go`, and its tests)
- `internal/httpapi` (tests only; the handlers do not change)
- `api/openapi.yaml` (descriptions) and the generated code, `ARCHITECTURE.md`
- `web/e2e` (fixing scenarios that assume the old rule, and adding scenarios for the acceptance
  criteria)

**New paths**: None

**Structure decision**: Follows the existing layout ([ARCHITECTURE.md](../../ARCHITECTURE.md)).

## Implementation Work

### Show each matched video, instead of the group, when a library search matches only some members

**Scope**: make `libraryItemsCTE` (`internal/store/library_items.go`) follow the rules in
[contracts/library-api.md §1–§2](contracts/library-api.md#1-how-get-apilibrary-builds-items), effective
for both `ListLibrary` and `LibraryIDs`. Replace the store and httpapi tests that fix the old rule
(`TestListLibraryFiltersPerMember`, `TestLibraryIDsIncludeAllMembersOfMatchedGroups` and others) with
the new rule. The `listLibrary` and `listLibraryIds` descriptions in `api/openapi.yaml` and
`task generate`, the `GET /api/library` paragraph of ARCHITECTURE.md, and the references to 017 §5 in
code comments ([§4](contracts/library-api.md#4-documents-and-tests-to-update)). Fix the owner's `ゲスト`
search scenario in `web/e2e/guest.e2e.ts` so that `公開あり` shows as a group card and `非公開だけ` as
D's video card (keep the step that selects a group by searching for a term that matches every
member).

**Dependencies**: None

**Acceptance**: `task check` passes. Store tests on the fixture `show` (ep1, ep2 and ep10):

| Case | Expected | Acceptance criteria |
| --- | --- | --- |
| Search `ep10` | One video item, ep10; `total` is 1; no group item | 1, 5 |
| Search the folder name `show` | One group item with 3 members | 2 |
| Filter by a tag attached by hand to only 2 of them | Those 2 as video items | 3 |
| Filter by a folder-derived tag | The group item | 3 |
| No search query and no tags | The same items as today | 4 |
| `WatchUnwatched` while only some members match | Only the unwatched matched members | 6 |
| `LibraryIDs` for search `ep10` / search `show` | Only ep10's id / all 3 | 7 |
| Guest (public: ep1 and ep10), search `ep10` / search `show` | A video item / a group of 2 | 8 |

In httpapi tests, the item `kind` for `GET /api/library?query=ep10` is `video`, and
`GET /api/library/ids?query=ep10` returns one id in `ids`. Run `task test-e2e` locally, confirm that the
owner's `ゲスト` search shows the `公開あり, group of 5 videos` card next to the video card for
`ゲスト非公開D`, and write the result in the PR body.

### Check playback, "Select all" and the guest view of members shown on their own with e2e

**Scope**: add scenarios to `web/e2e` for acceptance criteria 1, 2, 7 and 8, using the existing guest
fixture (`公開あり`, 5 videos of which 4 are public). The screen code does not change (a member shown
on its own is a `kind: video` item, the same as a video card).

**Dependencies**: Show each matched video, instead of the group, when a library search matches only
some members

**Acceptance**: `task test-e2e` passes and checks the following.

| Scenario | Expected | Acceptance criterion |
| --- | --- | --- |
| The owner searches for a term that matches only one video's title | One `article[data-video-id]` and no `article[data-group-root]`; clicking it opens that video's `/videos/{id}` | 1 |
| Search for the folder name | One group card; the members are not shown on their own | 2 |
| With only some members matched, "Select all" and attach a tag | The tag appears only in `GET /api/videos/{id}` of the matched videos | 7 |
| A guest searches for a term that matches only one public member | That one shows as a video card | 8 |
