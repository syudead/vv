# Research: Show each matching video when a library search matches only some members of a group

Parent Issue: #523.

Inherited decisions: [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
and [ARCHITECTURE.md](../../ARCHITECTURE.md) (a single Go binary, SQLite, code generated from
`api/openapi.yaml` as the source of truth, reads in `LibraryStore`). The current way items are built is
in [specs/017-folder-groups/data-model.md §5–§7](../017-folder-groups/data-model.md#5-library-items).
This file records only the decisions this feature adds.

## R-1: The decision is made in the SQL stage that builds items

**Decision**: Whether all members or only some of a group matched is decided inside
`libraryItemsCTE` in `internal/store` (the `with` clause that builds `items`), and `ListLibrary` and
`LibraryIDs` use the same clause. The rules are in
[contracts/library-api.md §1](contracts/library-api.md#1-how-get-apilibrary-builds-items).

**Rationale**: `total`, sorting, the keyset cursor and "Select all" all work on items (items 4–7 of
017 §5). None of them can be applied before items are fixed, so the decision has to come before the
stage that builds items, and that stage is already in SQL. If `LibraryIDs` uses the same clause,
requirement 8 (match "Select all" to the list's items) is met without a second implementation.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Fetch a page, then split in Go | Rejected: `total` and the cursor count a group as one item, so they disagree with the number of items after splitting, and items are duplicated or skipped across pages. |
| Split on the screen (`useVideos`) | Rejected: guests are not told how many private members a group has (017 §7), so the screen does not know "all members" and cannot decide. `total` and `GET /api/library/ids` would also stop matching. |

## R-2: Only the search query and tags decide; playability still applies to items as before

**Decision**: A "matched member" is the scope and search expression (`chosen`) combined with the tag
AND. Playability (`playable`) is not part of this decision; like watch status, it is applied to the
resulting items. A group item remains if any one member is playable (the same result as today), and a
video item is judged by that video.

**Rationale**: Requirement 6 rules out using watch status and playability for this decision. The
current implementation puts playability in the per-member condition, but for a group where "all
members matched", "some member satisfies (query ∧ tags ∧ playable)" and "some member is playable"
are the same set, so applying it to items instead does not change today's list (requirement 4).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Make playability a deciding factor too | Rejected: merely turning on "playable only" would split a group that was one card without filters. Violates requirements 4 and 6. |
| Require every member of a group item to be playable | Rejected: a group containing one unplayable member, which shows today, would disappear. Violates "as before" in requirement 6. |

## R-3: 017's artifacts stay unchanged; the current rule lives in this feature's contract and ARCHITECTURE.md

**Decision**: `data-model.md` and `contracts/library-api.md` in `specs/017-folder-groups/` do not
change. The replacing rule is written in [contracts/library-api.md](contracts/library-api.md), and the
implementation unit points the `GET /api/library` paragraph of ARCHITECTURE.md, the description in
`api/openapi.yaml` and the references in code comments there.

**Rationale**: A finished feature's artifacts are history; the current design is read from the
implementation, the API schema, the tests and ARCHITECTURE.md (`specs/README.md`). 017 likewise did not
rewrite 013's `list-api.md`; it added a delta document and referenced it from ARCHITECTURE.md. Using
the same form keeps a record of which feature changed the rule.

**Alternatives considered**: Rewrite 017 §5. Rejected: if a later feature revises an approved
artifact, it is no longer clear which point in time the document's decisions belong to. ARCHITECTURE.md
points at the current rule, so a rewrite is not needed either.
