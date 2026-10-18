# Research: faces and profiles for people

Inherited decisions: [tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[thumbnail-editing.md](../../docs/design-docs/thumbnail-editing.md#uploaded-images),
[work-info-plugins.md](../../docs/design-docs/work-info-plugins.md),
[library-ui.md](../../docs/design-docs/library-ui.md),
[design-system.md](../../docs/design-docs/design-system.md).
This file records only the decisions this feature adds.

## R-1: Profile fields are nullable columns on `people`, links a child table

**Decision**: Each profile field is a nullable column on `people`. Links go in
`people_links`, ordered by position and removed with their person.

| Option | Reads | Age sort | Verdict |
| --- | --- | --- | --- |
| **Columns on `people`** | No join for list, page or merge | One indexed column | Chosen |
| `person_profiles` table, one row per person | One more join on every list page | Join inside the keyset CTE | Rejected: a 1:1 table adds a join and a missing-row case for nothing |
| Key-value rows (`person_attributes`) | One row per field, pivot on read | Pivot before sorting | Rejected: the field set is fixed by the parent Issue, and typed columns keep the validation in one place |

**Rationale**: The parent Issue fixes the field set, and nothing asks for
owner-defined fields (`対象外`). Organizations keep no profile, so the columns
stay on `people` only.

## R-2: Partial dates reuse the work-date format

**Decision**: Birth and death dates are `YYYY`, `YYYY-MM` or `YYYY-MM-DD`
text, validated by the same rule as work dates
([`isWorkDate`](../../internal/domain/work_info.go)).

| Option | Verdict |
| --- | --- |
| **Text at its own precision** | Chosen |
| A full date plus a precision column | Rejected: two columns can disagree, and the work dates already settled on one text value |
| Year, month and day as three integers | Rejected: a second partial-date model next to the work dates |

**Rationale**: Plugins and owners often know only the year. A date is
compared at the precision both values share, so `1990` before `1990-05` is
valid, and a death date before the birth date is not (parent Issue,
`Edge Cases`).

## R-3: Age is computed in the browser, as a range at partial precision

**Decision**: The API returns dates, not ages. The web computes the age on
the viewer's current date, or on the death date when there is one. When the
dates are partial, it shows the youngest and oldest ages they allow
(`34–35`), and a single age when both are equal.

| Option | Verdict |
| --- | --- |
| **Browser, from the dates** | Chosen |
| Server returns `age` | Rejected: the value changes at midnight in the viewer's time zone, and a cached response would show yesterday's age |
| Hide the age at partial precision | Rejected: the parent Issue asks for the age shown "in the range the dates allow" |

## R-4: Age sort is a birth-date sort with the unknown last

**Decision**: People gain two sorts, `ageAsc` (youngest first, newest birth
date first) and `ageDesc`. People without a birth date come last in both. A
partial date sorts as its first possible day. Organizations answer either sort
with `400 invalid_request`.

| Option | Verdict |
| --- | --- |
| **Person-only sorts on the shared `CreditSort`** | Chosen |
| Add the sort to `TagSort` | Rejected: tags and organizations have no birth date |
| Sort the loaded rows in the browser | Rejected: the list is paged by keyset, so the browser holds only the first page |

**Rationale**: The keyset cursor stores an integer today. The birth date is
also stored as `YYYYMMDD` (missing parts as `00`), and each sort reads an
unknown date as the value that sorts last in its direction (`0` for
`ageAsc`, `99999999` for `ageDesc`), so the cursor stays an integer and the
comparison needs no null branch.

## R-5: A face is one JPEG blob in SQLite, fitted on the server in pure Go

**Decision**: An accepted image is decoded (PNG, JPEG, still WebP), fitted to
at most 800 px on its long side with `golang.org/x/image/draw`, encoded as
JPEG and stored in `person_images`. It is served from that row.

| Option | Survives a rebuild | Removed with the person | Verdict |
| --- | --- | --- | --- |
| **Blob in `person_images`** | Yes | `on delete cascade` | Chosen |
| File under `MDM_DATA_DIR/thumbnails` through `internal/artifacts` | No: the thumbnails directory is rebuildable | Needs its own reference count | Rejected: a face is user data, and `artifacts` removes files by video content |
| Fit through ffmpeg, as video thumbnails do | Yes | Yes | Rejected: needs the `media` adapter for a still image, while `x/image/draw` already scales seek frames without cgo |
| Keep the original upload | Yes | Yes | Rejected: an upload may be 64 MiB, and the screens show at most a portrait card |

**Rationale**: An 800 px JPEG is under about 200 KiB, so a blob costs little,
and a person with its face stays one unit for delete, merge and backup. The
upload reuses the thumbnail upload's limits, decode gate and animated-WebP
refusal ([thumbnail-editing.md](../../docs/design-docs/thumbnail-editing.md#uploaded-images)),
shared rather than copied.

## R-6: The image is addressed by person and revision

**Decision**: `Person` and the video page's `WorkPerson` carry
`imageRevision` when the person has a face. The browser loads
`/api/people/{id}/image?v={revision}`, and the response is `private, no-cache`
with the revision as ETag.

| Option | Verdict |
| --- | --- |
| **Revision on the existing responses, kept on `people`** | Chosen: the counter survives a removal, so a face uploaded after a removal never reuses an old ETag |
| Revision on the `person_images` row | Rejected: removing the row resets it, and the next face would answer `304` to the removed face's ETag |
| Inline data URLs in list responses | Rejected: 100 faces in one JSON page |
| A boolean `hasImage` | Rejected: a replaced face would stay cached under the same URL |

## R-7: Profile is a nested object, person-only in the shared credit schemas

**Decision**: `Person`, `CreditInput` and `CreditPatch` carry an optional
`profile` object. An organization request with `profile` answers
`400 invalid_request`, as `parentId` does for people. In a patch, each
property present replaces that field and `null` clears it.

| Option | Verdict |
| --- | --- |
| **Nested `profile`** | Chosen |
| Eleven flat properties on `CreditInput` | Rejected: eleven rejection rules for organizations, and the shared schema reads as a person schema |
| Separate `/api/people/{id}/profile` endpoint | Rejected: the person page saves name, aliases and profile from one form, and the external API and MCP already map `CreditInput` |

**Rationale**: The external API and MCP take their input schemas from the
same types, so the profile reaches agents without a second mapping. Images
stay on the screen API only, as video thumbnail uploads do.

## R-8: Merge fills the target's empty fields from the sources

**Decision**: The merge target keeps every profile field, link list and face
it has. A field the target lacks takes the first source's value in
`sourceIds` order that has one, and so does the face.

| Option | Verdict |
| --- | --- |
| **Fill empty fields from sources in request order** | Chosen |
| Keep the target only | Rejected: merging a duplicate created by a plugin would drop the profile the plugin brought |
| Let the owner pick per field | Rejected: the parent Issue keeps the existing one-step merge |

## R-9: Plugins return a face URL and a profile per person, fetched like the cover

**Decision**: A `people` entry may carry `image` (a URL) and `profile`, as
optional fields under contract 1. After the run, VVMDM fetches at most 7 person
images, in result order, with the cover's client, limits and format check,
within the run's deadline. An image that cannot be fetched is dropped. A
refused address fails the run with `blocked_address`, as a cover does.

| Option | Verdict |
| --- | --- |
| **Fetch after the run, up to 7, kept with the fetch** | Chosen |
| Fetch at apply | Rejected: the site could return a different image than the one the run saw, the reason the cover is fetched after the run |
| Fetch every person's image | Rejected: 100 people per role at 4 MiB each exceeds the 32 MiB result budget and the 30 s deadline |
| Contract 2 | Rejected: the contract ignores unknown fields, and `cover` was added the same way |

**Rationale**: The cover and seven images at the 4 MiB limit make 32 MiB, the
existing result budget (`WorkInfoFetchCoverBudget`), which now counts them
together; an eighth image would let one result exceed the budget and be
evicted as soon as it is kept.

## R-10: Plugin data only fills what is empty, after the apply commits

**Decision**: When `people` is applied, each linked person (existing or new)
gets the fetched profile's fields that are empty on the person, inside the
apply transaction. A fetched face is stored after the commit, only when the
person has none at that moment, as the cover is applied after its commit.

| Option | Verdict |
| --- | --- |
| **Fill empty fields; never overwrite** | Chosen (parent Issue, `要件 4`) |
| Show the profile in the comparison and let the owner pick | Rejected: the parent Issue asks for filling without a choice, and the comparison stays one row per field |
| Store the face inside the apply transaction | Rejected: decoding and fitting an image would hold the write transaction |

An invalid fetched value is dropped per field and reported like other skipped
values. A face that fails to decode is dropped without failing the apply.

## R-11: The person page lists videos by person and role on the server

**Decision**: `listVideos` gains an owner-only `personRole` parameter that
requires `person` and keeps only videos linked to that person in that role.

| Option | Verdict |
| --- | --- |
| **Server filter** | Chosen |
| Filter loaded pages in the browser | Rejected: totals and paging would be wrong, and pages could come back empty |

## R-12: The people list becomes a `ListPage` card grid; organizations keep the table

**Decision**: `/people` uses `ListPage` with `CardGrid` and the library's
infinite scroll; `/organizations` keeps the `AdminTablePage` built from the
tag pieces. The design stage settles the card and the page layout.

| Option | Verdict |
| --- | --- |
| **`ListPage` + `CardGrid`** | Chosen: the design system's skeleton for browsing items |
| Cards inside `AdminTablePage` | Rejected: the design system forbids cards in that skeleton |
| Virtualized grid | Rejected: library card grids are not virtualized, and the people list pages 100 at a time like the library |

## R-13: Person favorites join the existing favorites

**Decision**: A favorite person is a row in `person_favorites`, set and
cleared through `personIds` on `PUT /api/favorites`, as videos, groups and
smart folders are.

| Option | Verdict |
| --- | --- |
| **`person_favorites` and `PUT /api/favorites`** | Chosen |
| A `favorite` column on `people` | Rejected: every other favorite is a table of its own with its timestamp, and the shared endpoint already sets several kinds atomically |
| `PUT /api/people/{id}/favorite` | Rejected: a second way to set favorites beside the shared endpoint |

A merge makes the target a favorite when any source is one, so merging a
duplicate never drops the mark; deleting a person removes its row.

