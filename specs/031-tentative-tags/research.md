# Research: Tentative tags and rejected names

Inherited decisions: the tech stack, boundaries and dependency direction, the
split between index and user data, the authentication boundary, and the tag
tables and name rules follow their sources of truth
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[ARCHITECTURE.md](../../ARCHITECTURE.md),
[specs/014-video-tags/data-model.md](../014-video-tags/data-model.md),
[specs/017-folder-groups/data-model.md §4](../017-folder-groups/data-model.md#4-folder-derived-tags),
[specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)).
This file records only the decisions this feature adds.

## R-1: Tentative state is one column on `tags`

**Decision**: Add `tentative integer not null default 0 check (tentative in (0, 1))`
to `tags` ([data-model.md §1](data-model.md#1-migration)). Existing rows take the
default and become confirmed tags (requirement 16, edge case "migrating existing
data"). Confirming rewrites the column to `tentative = 0` and touches neither
`tag_names` nor `video_tags` (requirement 8.1: "the videos the tag is on do not
change").

**Rationale**: Tentative or not is the state of one tag, and every read that
returns tags (the list, one tag, a video's tags, the summary) starts from `tags`
and joins outward. As a column, adding it to those reads puts it in every response
requirement 5 names. Attaching, detaching, filtering, search and counts read
`video_tags` and `tag_names`, so the new column leaves their behaviour unchanged
(requirement 4).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A separate table holding the `id` of each tentative tag | Rejected: every read needs an extra `exists`, and confirming becomes a row delete, which is harder to follow than a column update. The state has two values, so representing it by row presence gains nothing |
| A third value for `tag_names.canonical` | Rejected: the kind of a name row and the state of a tag are separate concerns, and a third value breaks the partial index `tag_names_canonical_idx` and the 014 invariants |

## R-2: Rejected names live in a name-only table `rejected_tag_names`, matched exactly like tag names

**Decision**: Rejecting is one transaction: delete the tag and insert its canonical
name into `rejected_tag_names (name primary key, created_at)`
([data-model.md §2](data-model.md#2-rejected_tag_names)). Matching uses the default
BINARY collation, the same as `tag_names.name`, so a name matches only when the
spelling is identical (requirement 13). A create with `tentative` true makes a
tentative tag only when the name does not exist and is not in this table
(requirement 11).

**Rationale**: A rejected name is neither a tag nor an assignment to a video. It
records only "do not create this again". In `tag_names` it would conflict with the
foreign key to `tags` and with the 014 invariant (each tag has exactly one
canonical name row). The list and removal in requirement 14, and "a manual
decision removes the name" in requirement 15, need only a `select` and a `delete`
on a table keyed by name.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Keep the tag row and add a "rejected" state | Rejected: requirement 8.2 requires deleting the tag and removing it from its videos. A surviving row would need a condition in every read to keep it out of `id` filters and `GET /api/tags` |
| Normalize rejected names beyond `NormalizeTagName` before matching | Rejected: requirement 13 specifies exact matching. Spelling variants are absorbed by merging |

## R-3: A transaction that writes a name to `tag_names` removes it from `rejected_tag_names`

**Decision**: Every transaction that adds a name row to `tag_names` (create,
synonym registration, create during attach-by-name, group-to-tag conversion) or
rewrites a name by rename removes that name from `rejected_tag_names` in the same
transaction. A tentative create has already skipped rejected names, so passing it
through this rule removes nothing
([data-model.md §3](data-model.md#3-write-rules)).

**Rationale**: Requirement 15 requires that a manual decision wins and removes the
name from the rejected list, for the screen's create, rename and synonym actions
and for the API with `tentative` false. All name writes go through
`insertTagName` and the one rename statement, so placing the removal there means
no path can forget it. The rule also guarantees that no name is in both
`tag_names` and `rejected_tag_names`, which can be checked as an invariant.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Remove the name in each API handler | Rejected: SQL concerns leak into `internal/httpapi`, and paths owned by another role, such as group-to-tag conversion, are missed |
| Allow a name in both tables and let reads prefer `tag_names` | Rejected: the list in requirement 14 would keep names that are already in use |

## R-4: Edits to a tentative tag (rename, synonym, merge target) confirm it in the same transaction

**Decision**: `RenameTag` (when the name changes), `AddSynonym` (when it adds a
name, including with an accepted merge), and the target of `MergeTag` set
`tentative = 0` in the same transaction when the tag is tentative (requirement 9).
A rename to the current name changes nothing, as in the current contract, so the
tag stays tentative. Merging one tentative tag into another confirms the target
and deletes the source (edge case). Confirming is also a standalone operation,
`ConfirmTag` (requirement 8.1).

**Rationale**: Giving a tentative tag a name or a synonym means the user has
decided to keep it. Writing the state in the same transaction leaves no
intermediate "renamed but still tentative" state. That a tentative tag has no
synonyms (requirement 9) follows from this rule and can be checked as an
invariant.

**Alternatives considered**: The screen sends `confirm` after the rename. Rejected:
between the two requests a reject from another tab can race and delete a tag that
was meant to be confirmed, and API users would have to follow the same two-step
sequence.

## R-5: Reject applies only to tentative tags; a confirmed tag gets `409 tag_not_tentative` and no change

**Decision**: `RejectTag` checks inside its transaction that the tag is tentative.
Otherwise it returns `domain.ErrTagNotTentative` (`409 tag_not_tentative` in the
API) and changes nothing. `ConfirmTag` on an already confirmed tag returns `200`
with the current state
([contracts/screen-api.md §2](contracts/screen-api.md#2-tentative-tag-operations)).

**Rationale**: Rejecting deletes the tag. A tag already confirmed from another tab
or through the API (edge case "conflicting operations") must not be deleted by
"Reject" on a stale screen. Deleting a confirmed tag stays with the existing
`DELETE`, which does not remember the name (requirement 10). Confirming only moves
the state forward and repeating it is harmless, so it does not need the treatment
`tag_not_found` gets. The screen handles `tag_not_tentative` like `tag_not_found`:
it reloads the list.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Accept "reject" on a confirmed tag as a delete | Rejected: violates requirement 10, and a race would produce a delete that remembers the name |
| Return `409` from `confirm` too | Rejected: the screen only reloads, so the user sees no difference, and API users find an idempotent call easier to handle |

## R-6: Rejection and a tentative attach of the same name rely on SQLite write serialization

**Decision**: The reject transaction (check tentative → delete from `tags` →
insert into `rejected_tag_names`) and the tentative attach transaction (look up
the name → check rejected names → create the tentative tag → attach) both start
as write transactions (the immediate-lock pool in `internal/store`). No extra
lock or retry is added.

**Rationale**: Write transactions run one at a time. If the reject runs first, the
attach sees the rejected name and skips it. If the attach runs first, the reject
deletes that tag and remembers the name. In either order no tag survives with a
rejected name (edge case "conflicting operations").

**Alternatives considered**: After the reject transaction, search for a tag with
the same name and delete it again. Rejected: it handles a case serialization rules
out, and it misleads readers into thinking a race exists.

## R-7: The external API adds `tentative` to the request and skipped names to the response

**Decision**: Add `tentative` (boolean, default false) to the body of
`POST /api/v1/video-tags` and `skippedTags: string[]` to its response (normalized
names, in `tags` order, without duplicates, empty when none). `add` and `replace`
skip rejected names. `remove` already does nothing for a name that does not
exist, so it accepts `tentative` but changes nothing, and `skippedTags` is empty.
Add `tentative` to `Tag` and `ExternalVideoTag`. MCP's `update_video_tags`
derives its input shape from the same body type, so `tentative` arrives with only
a description change
([contracts/external-api.md](contracts/external-api.md)).

**Rationale**: The contract takes the shape of requirements 1, 11 and 12 as is.
Adding a response field and an optional body field fits the 026 compatibility
policy (only fields and operations are added). Reporting "skipped" for `remove`
would be indistinguishable from removing a name that does not exist (which
already does nothing) and would make users read a field with no meaning.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Return `400` for a request that contains a rejected name | Rejected: requirement 11 says it must not fail |
| A separate operation for tentative attach | Rejected: one more field on the same body is the smallest change for a scraper, and one rule covers its combination with `replace` |

## R-8: The tentative-only filter and the rejected-name list live in the screen

**Decision**: The "tentative only" filter adds no parameter to `GET /api/tags`;
the tag management page filters the shared list (`web/src/api/tags.ts`) by
`tentative`. The rejected-name list is read separately from
`GET /api/tags/rejected-names` and is used only by the tag management page.

**Rationale**: The tag list is held whole from one request, and name search is
already filtered in the screen (the 014 tag management page). A parameter would
split the shared cache per condition. Rejected names are not tags and are not
needed by the candidates (combobox) or the chips, so they stay out of the shared
list.

**Alternatives considered**: Put `rejectedNames` in the `GET /api/tags` response.
Rejected: every screen that reads the tag list would receive an array it does not
use, and keeping them separate lets each be reloaded on its own.

## R-9: The screen API adds two `POST` routes for tentative tags and `GET` and `DELETE` for rejected names

**Decision**: Four routes: `POST /api/tags/{id}/confirm`,
`POST /api/tags/{id}/reject`, `GET /api/tags/rejected-names`, and
`DELETE /api/tags/rejected-names?name=…`
([contracts/screen-api.md](contracts/screen-api.md)). The name to remove is
passed in the query, as when removing a synonym.

**Rationale**: Confirm and reject are operations on one tag, so they take the same
`POST /api/tags/{id}/…` shape as merge and synonyms. Reject deletes the tag but
its result differs from `DELETE /api/tags/{id}` (it remembers the name), so a
separate route prevents confusing the two. Names stay out of the path, following
the 014 decision (names can contain `/` and `%`).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Add `tentative: false` to `PATCH /api/tags/{id}` | Rejected: it mixes with the rename request and makes `tentative: true` sendable. No requirement asks to make a tag tentative again |
| `DELETE /api/tags/{id}?reject=true` | Rejected: one route with two results, so a delete from a stale screen could remember the name by accident |
