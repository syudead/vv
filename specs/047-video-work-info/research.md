# Research: Work information on videos (code, dates, people, organizations, URLs, external IDs)

Parent Issue: #967. Inherited decisions:

| Topic | Source of truth |
| --- | --- |
| Boundaries, dependency direction, index versus user data, events after commit | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| User key (`bundle:<id>` for a bundle member, otherwise `content_key`), bundle copy and restore, same-path succession | [specs/030-video-versions/data-model.md, User key](../030-video-versions/data-model.md#user-key), [Store operations (`VersionStore`)](../030-video-versions/data-model.md#store-operations-versionstore), [Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| Name rules (control characters, trim, empty, a code point limit, no folding of the stored spelling) | [specs/014-video-tags/data-model.md, Name rules](../014-video-tags/data-model.md#name-rules), [specs/029-video-overrides/research.md R-10](../029-video-overrides/research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit) |
| Search matching form (`FoldForMatch`) and how tag names join the search box | [specs/013-library-search/data-model.md, `search_key` rules](../013-library-search/data-model.md#search_key-rules), [specs/014-video-tags/data-model.md, Matching tag names in the search box](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box) |
| Paged admin lists with `q`, `sort`, `cursor`, `limit`, `total`, and the natural-order `sort_key` | [specs/036-tag-admin-scale/research.md R-1](../036-tag-admin-scale/research.md#r-1-the-server-pages-the-list-and-applies-search-filters-and-sort-to-every-tag), [R-10](../036-tag-admin-scale/research.md#r-10-a-natural-order-name-key-sort_key-on-tag_names-and-rejected_tag_names-filled-by-the-startup-key-refresh) |
| Tag filter on the list API and in the library URL | [specs/014-video-tags/contracts/tags-api.md, List filter and Select all](../014-video-tags/contracts/tags-api.md#list-filter-and-select-all), [list-url.md](../014-video-tags/contracts/list-url.md) |
| What guests see and which conditions they cannot use | [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| Edit time (`updatedAt`) and which writes advance it | [specs/033-video-dates/research.md R-2](../033-video-dates/research.md#r-2-only-the-four-writes-of-video-information-advance-the-edit-time-tag-level-operations-bundles-and-ingestion-do-not) |
| External API conventions, bulk operations, literal `POST` routes and MCP tools | [specs/026-external-api/contracts/external-api.md, Common rules](../026-external-api/contracts/external-api.md#common-rules), [specs/039-external-tag-admin/research.md R-2](../039-external-tag-admin/research.md#r-2-tag-operations-are-literal-post-routes-that-name-the-tag-in-the-body), [R-6](../039-external-tag-admin/research.md#r-6-the-external-handlers-call-the-same-tags-methods-as-the-screen-with-no-new-write-path) |

This file records only the decisions this feature adds. The superseded first
plan (six string fields, branch `archive/047-video-work-info-v1`) is not an
input; where this plan keeps one of its choices, the choice is restated here.

## R-1: Typed tables per item kind, so a later item is a new table or a new kind value

**Decision**: Work information is stored as one row of single-value fields
(`code`, `description`) plus one table per multi-value item, each row carrying
its kind or role as a text value from a fixed set
([data-model.md, Migration](data-model.md#migration)). A later item
(requirement 15: series, original title, …) is a new table or a new column
added by an additive migration, or a new value of an existing kind set; no
existing column changes meaning and no row is rewritten.

| Option | Verdict |
| --- | --- |
| **One table per multi-value item, kind as a text value** | Chosen |
| One JSON column holding the whole object | Rejected: the search (R-6), the list filters (R-7) and the merge of people (R-8) match one person or one name per row, and `json_each` on every candidate row runs the parse once per video per list page |
| One generic `video_work_values (kind, value)` table for dates, URLs and external IDs | Rejected: a date, a URL and a `(source, value)` pair have different columns and rules, and one table would hold them as untyped text with the rule in the kind |
| Columns on `videos` | Rejected: the index is rebuilt by a scan (requirement 13) |

**Rationale**: The parent Issue fixes the item set and the role sets and
asks that the shape survive later additions; a table per item kind gives each
item its own columns and index, and a kind value from a fixed set is what
`CHECK` constraints and the generated enums can carry, so the fixed sets of
requirements 2, 5 and 6 are enforced where the data lives.

## R-2: Per-video tables keyed by the user key, each standing alone, copied by the bundle and succession paths

**Decision**: Every per-video table (`video_work_info`, `video_work_dates`,
`video_work_urls`, `video_work_external_ids`, `video_work_people`,
`video_work_organizations`) has `content_key` holding the user key, no foreign
key to `videos` and no foreign key to `video_work_info`, and joins
`userDataTables` and `moveUserData`
([data-model.md, Bundles and succession](data-model.md#bundles-and-succession)).
A `video_work_info` row exists only while `code` or `description` is set.

| Option | Verdict |
| --- | --- |
| **User key, each table standalone** | Chosen |
| Content key, like `video_overrides` | Rejected: the Edge Case asks for one unit per bundle; the information describes the work, which every version shares |
| A parent row that the other tables reference with `on update cascade` (the superseded plan) | Rejected: `moveUserData` then cannot re-key the child tables with the statement it uses for every other table, and a video with only a date would need an empty parent row |

**Rationale**: Keying by the user key makes requirement 13 and the Edge Cases
(same content in several places, bundles, moves and renames) properties the
existing paths already give. With no row depending on another, every table is
copied and replaced by the same statements as `video_favorites`, and bundling
a video with only a date or only a URL loses nothing. Succession is the one
path that treats the six tables as a unit: when the old key has work
information, the new key's rows in all six are replaced, so a succession never
leaves one video's people beside another video's dates.

## R-3: People and organizations are id entities whose names are not unique, served by one `CreditStore`

**Decision**: `people` and `organizations` are tables of ids; their names and
aliases are rows of `people_names` and `organization_names` (one row per
spelling per entity, one `canonical = 1` row each), and two entities may share
a spelling. One role type, `CreditStore`, implements list, read, create,
update, merge and delete for both kinds, taking `domain.CreditKind` where the
tables differ by name only; the parent organization is the one
organization-only field ([data-model.md, Entities](data-model.md#entities)).

| Option | Verdict |
| --- | --- |
| **Two entity kinds, one store implementation** | Chosen |
| One `credits` table with a `kind` column | Rejected: an organization has a parent and a person does not, and the fixed role sets differ; one table carries a nullable parent that is meaningless for half its rows |
| Two role types (`PeopleStore`, `OrganizationStore`) | Rejected: the list, name, alias, external-id, merge and delete SQL is the same for both kinds, so two types are the copy that AGENTS.md rules out |
| Unique names, like tags | Rejected: the Edge Case asks that two people with the same name be two people |

**Rationale**: Requirements 3, 4 and 12 describe entities with their own
screens, renames and merges, and a merge or rename has to move every link at
once, which only an id makes possible. Because a spelling identifies nothing,
the screens and the APIs always carry the id with the name.

## R-4: A video write references people and organizations by id; creation has its own route

**Decision**: `WorkInfoInput` names a person or organization by `id` only.
The video page creates a missing person or organization through
`POST /api/people` or `POST /api/organizations` before it sends the `PUT`;
the external bulk operation references ids too, and a client finds an id by
name or external ID with the list routes
([contracts/screen-api.md, People and organizations](contracts/screen-api.md#people-and-organizations)).

| Option | Verdict |
| --- | --- |
| **Id only; a separate create route** | Chosen |
| `{ id }` or `{ name }` in the video write, creating on `name` (the shape of `TagInput`) | Rejected: a tag name is its identity, so creating on a name is idempotent; a person's name is not (R-3), and a resent `PUT` would create a second person |
| Match `{ name }` to an existing entity when exactly one has that spelling | Rejected: the second person with that name silently merges into the first, which is the duplicate the Edge Case asks to keep apart |

**Rationale**: Requirement 8 ("pick an existing one or create it on the
spot") is a screen flow, not a write shape; two calls from the page keep the
video write idempotent and give the list screens, the external API and the
page one create route.

## R-5: One `PUT` that replaces the whole work information object

**Decision**: `PUT /api/videos/{id}/work-info` takes the whole object and
replaces it; a field that is absent, `null` or empty after trimming is
cleared, an absent list is empty, and a video whose every item is cleared has
no rows. The response is `Video`. `domain.NormalizeWorkInfo` applies the rules
in [data-model.md, Fields](data-model.md#fields) and returns one
`InvalidWorkInfoError` naming the field path and the problem
([contracts/screen-api.md, `PUT /api/videos/{id}/work-info`](contracts/screen-api.md#put-apivideosidwork-info)).

| Option | Verdict |
| --- | --- |
| **Replace the whole object** | Chosen |
| One route per item kind (`/work-info/dates`, `/work-info/people`, …) | Rejected: the page saves one form and the external operation sends whole objects; seven routes multiply the error mapping and the tests by seven |
| `PATCH` with merge semantics | Rejected: "clear this list" then needs a sentinel distinct from "leave it", which the screen never needs |

**Rationale**: The editor edits the items together and a scraper writes them
together, so the unit of change is the object; replacement is idempotent, so a
resend after a network failure is safe. The editor itself does not call this
route: it saves the display name with the object through the route of R-14,
and this route serves the external operation's semantics and a client that
writes the work information alone.

## R-6: The search box matches the code and every name and alias of the linked people and organizations

**Decision**: For the owner, a search term matches a video when
`instr(video_work_info.code_key, term) > 0`, or when a `people_names` or
`organization_names` row linked to the video's user key has
`instr(search_key, term) > 0`; the three are branches of the OR that
`tagNameMatchCondition` builds ([data-model.md, Search](data-model.md#search)).
The description, the dates, the URLs and the external IDs are not matched.
Guests do not match any of it (R-10).

| Option | Verdict |
| --- | --- |
| **A key on each name row and on the code, matched with `instr` like tag names** | Chosen |
| One per-video `search_key` concatenating the code and every linked name | Rejected: a rename or merge of a person would have to rewrite the key of every linked video, while a key on the name row changes in one place |
| An FTS5 trigram table over the values | Rejected: a trigram needs three characters, and codes such as `AB-12` and two-character names are the common case; tag names use `instr` for the same reason |
| Include the description | Rejected: a long description matches generic words and turns the search box into a full-text search of notes, which requirement 10 does not ask for |

**Rationale**: Acceptance criterion 3 names the code, a name and an alias;
the name rows are where aliases live, so matching there covers both, and a
merge that moves name rows to the target (R-8) keeps acceptance criterion 5
true with no key rewrite.

## R-7: `person` and `organization` list parameters by id, matching every role

**Decision**: `GET /api/videos`, `GET /api/library` and `GET /api/library/ids`
gain `person` and `organization`, one id each, ANDed with every other
condition; a video matches when a link row of any role names that id
([contracts/screen-api.md, List parameters `person` and `organization`](contracts/screen-api.md#list-parameters-person-and-organization)).
The library URL carries `person=<id>` and `organization=<id>`; pressing a
person or organization on the video page opens `/?person=<id>` or
`/?organization=<id>`. `listFolderVideos` does not take them, as it does not
take `tag`. Guests get `400` (R-10).

| Option | Verdict |
| --- | --- |
| **Id parameters on the list API and the library URL, any role** | Chosen |
| A role with the id (`person=3&role=performer`) | Rejected: requirement 11 names the person, not the credit; a person who directed one video and performed in another has one list of videos |
| A page of its own (`/people/<id>`) | Rejected: the library already has the URL, snapshot and condition machinery, and a filter composes with the search box and tags |
| The name as the value, like the superseded plan's `performer=<name>` | Rejected: names are not unique (R-3) |

**Rationale**: Acceptance criterion 4 is "press the value, see the videos
with the same person"; an id in the URL does exactly that and survives a
rename, which a name in the URL would not.

## R-8: Entity edits are a `PATCH` of optional fields, a merge that moves links, and a delete that detaches

**Decision**: `PATCH /api/people/{id}` (and `/api/organizations/{id}`) takes
`name`, `aliases`, `externalIds` and, for an organization, `parentId`, each
optional; a given list replaces the stored list whole. `POST …/{id}/merge`
takes `sourceIds` and moves every link row, name row and external ID of the
sources to the target in one transaction; it is refused, changing nothing,
when the merged target would exceed the alias or external ID limit, or when a
source organization is an ancestor of the target. `DELETE …/{id}` removes the
entity and its link rows; the screen shows `videoCount` before asking. A
`parentId` that is the organization itself or one of its descendants is
refused. An external ID names at most one entity of a kind, so `POST` and
`PATCH` refuse one another entity holds
([contracts/screen-api.md, People and organizations](contracts/screen-api.md#people-and-organizations)).

| Option | Verdict |
| --- | --- |
| **`PATCH` with optional fields; aliases as one list** | Chosen |
| Tag-style synonym routes (`POST …/synonyms`, `DELETE …/synonyms?name=`) | Rejected: tag synonyms need per-name routes because adding one can trigger a merge; a person's alias conflicts with nothing (R-3), so the alias dialog saves one list |
| `PUT` replacing the whole entity | Rejected: the rename dialog would resend the aliases it loaded and overwrite an alias edit made in another tab |
| Refuse to delete an entity with linked videos | Rejected: the Edge Case says the count is shown and the links are detached |
| Merge a source that is an ancestor of the target by re-parenting the chain (the target takes the source's place) | Rejected: the organizations between the two would need their own rule to avoid a loop; refusing keeps the Edge Case's "a cycle is refused", and the owner can move the target first or merge the other way |
| Merge past the alias and external ID limits | Rejected: the target would hold lists that the next `PATCH` of those lists refuses |
| Let two entities share an external ID, and return every holder from the lookup | Rejected: a scraper looking up a performer could not tell which one to link; a duplicate is a mistake to merge, not a state to keep |

**Rationale**: The list screens follow the tag page (list, rename, merge),
and one `PATCH` serves rename, alias editing and the parent with one route
and one error mapping; the merge mirrors `MergeTags` so acceptance criterion 5
holds the way the tag merge does.

## R-9: The external API gets one bulk replace and literal `POST` routes per entity kind; `field` names the failing property

**Decision**: `POST /api/v1/video-work-info` takes `items` of
`{ video, workInfo }` (1 to 20000, one transaction, nothing applied on any
error) with the semantics of R-5, and `ExternalVideo` gains the required
`workInfo`. People and organizations get `GET /api/v1/people` (with
`q`, or `source` and `externalId` for a lookup), `POST /api/v1/people`,
`POST /api/v1/people/update`, `POST /api/v1/people/merge` and
`POST /api/v1/people/delete`, and the same five under `/organizations`; MCP
gets one tool per operation
([contracts/external-api.md](contracts/external-api.md)). Every error that
names a property carries `field`, a path such as `people[1].id` or
`aliases[0].language`, in both schemas; `index` keeps its meaning of the
position in `items`.

| Option | Verdict |
| --- | --- |
| **One bulk replace, five routes per kind, `field` as a path** | Chosen |
| A `{ source, externalId }` reference inside the video write | Rejected: the lookup route resolves the same reference, and one reference shape (`id`) keeps `WorkInfoInput` the same in both schemas |
| One reason per field and problem (`code_too_long`, `date_invalid`, …) | Rejected: seven item kinds and four problems give over twenty enum values, and the screen's catalog would need a message for each |
| A `merge` action beside `replace` | Rejected: the writers this feature serves hold the whole object; `v1` can gain `action` later without breaking clients |

**Rationale**: `POST /api/v1/video-display-names` and the tag routes of 039
are the shapes external writers already use; a scraper looks an entity up by
its external ID, creates it when missing and writes the video once.

## R-10: Guests receive no work information, in responses, search or filters

**Decision**: Guest responses omit `workInfo` (as they omit `displayName`),
the guest search condition has no work information branch, `person` or
`organization` from a guest returns `400 guest_filter_not_allowed` through
`Audience.CheckVideoQuery`, and every people and organization route is owner
only.

| Option | Verdict |
| --- | --- |
| **Owner only, like tags** | Chosen |
| Show it on public videos and let guests search and filter | Rejected: the guest contract removes every owner-entered field but the title, and the owner publishes a video, not notes about it |

**Rationale**: Every requirement of the parent Issue names the owner, and
widening the guest surface is a decision the Issue does not make.

## R-11: A work information write publishes `VideoWorkInfoChanged`; entity edits publish nothing

**Decision**: `WorkInfoStore` publishes `VideoWorkInfoChanged{VideoIDs}` after
commit, one event per write, and `httpapi.Events.Handle` maps it to a
`video` notification per id. A rename, merge or delete of a person or
organization publishes no event; an open video page shows the new name on its
next fetch, as it does after a tag rename.

| Option | Verdict |
| --- | --- |
| **One event for video writes, none for entity edits** | Chosen |
| Reuse `VideoOverrideChanged` | Rejected: overrides are per content and carry one id; a bundle write affects every member |
| Publish the ids of every linked video on an entity edit | Rejected: a merge of a prolific person lists thousands of ids, and no existing screen needs it |

**Rationale**: Acceptance criterion 8 (a value written through the external
API appears on the open page) needs the video event, which the page already
handles; the tag page sets the precedent that entity administration does not
notify.

## R-12: One date per kind, in `YYYY`, `YYYY-MM` or `YYYY-MM-DD`

**Decision**: `video_work_dates` holds at most one row per kind (`release`,
`recorded`, `published`), and `date` is the text `YYYY`, `YYYY-MM` or
`YYYY-MM-DD`, a calendar date when it has a day, year 1 to 9999. The text is
stored and returned as given.

| Option | Verdict |
| --- | --- |
| **Reduced-precision ISO text, one per kind** | Chosen |
| A full date plus a `precision` column | Rejected: a second column to validate and to render; the ISO text already carries its precision in its length |
| Several dates per kind | Rejected: requirement 2 gives one date per kind, and a second release date is a second release, which is a version or a series (`対象外`) |

**Rationale**: A scraper and the owner both have year-only dates (requirement
2), and the text form compares and sorts as a string in SQL without a parse.

## R-13: Work information writes advance the edit time; entity edits do not

**Decision**: A `PUT` or bulk item that changes a video's work information
calls `touchEditedAtForUserKeys` for its user key, as display names do; a
write that changes nothing leaves it. Renaming, merging or deleting a person
or organization leaves every video's edit time alone.

| Option | Verdict |
| --- | --- |
| **Video writes advance, entity edits do not** | Chosen |
| Entity edits advance every linked video | Rejected: 033 R-2 keeps tag-level operations out of the edit time, and a person is edited at the same level as a tag |

**Rationale**: The edit time answers "when did the owner last edit this
video"; a rename of a person is an edit of the person.

## R-14: The editor saves the display name and the work information with one `PUT /api/videos/{id}/details` in one transaction

**Decision**: The full-screen editor of requirement 8 saves with
`PUT /api/videos/{id}/details`, whose body is
`{ displayName: string, workInfo: WorkInfoInput }`, both required.
`WorkInfoStore.SetDetails` writes the display name under the content key
(the rows and rules of 029) and the work information under the user key (R-2)
in one transaction: a display name that breaks its rule or a work information
field that breaks its rule rolls back everything, so the title is never saved
without the information or the other way round. The display name is checked
first, then the work information in the order of the `WorkInfo` schema, and
the first failure is reported with `field` (`displayName`, or the path inside
`workInfo` as in R-9). After commit the store publishes `VideoOverrideChanged`
when the name changed and `VideoWorkInfoChanged` when the information changed
(R-11), and advances the edit time once when either changed (R-13).
`PUT /api/videos/{id}/display-name` and `PUT /api/videos/{id}/work-info`
stay: the first serves the in-place title edit that requirement 8 keeps, the
second the external semantics of R-5 and R-9
([contracts/screen-api.md, `PUT /api/videos/{id}/details`](contracts/screen-api.md#put-apivideosiddetails)).

| Option | Verdict |
| --- | --- |
| **One route taking both, one transaction** | Chosen |
| The editor sends `PUT …/display-name` and then `PUT …/work-info` | Rejected: a failure or a closed tab between the two leaves the title saved and the information not, which requirement 8 rules out; a client cannot make two requests atomic |
| `displayName` as an optional property of `WorkInfoInput` on `PUT …/work-info` | Rejected: `WorkInfoInput` is the item of the external bulk operation, which has `POST /api/v1/video-display-names` for titles, so the shared schema would carry a property half its users must not send; and "absent clears" (R-5) is wrong for a title, where absent has to mean "leave it" |
| `PATCH /api/videos/{id}` with merge semantics for every owner-written field | Rejected: no video field is written that way today, "clear the title" needs a sentinel distinct from "leave it", and the editor always holds both values, so optionality buys nothing |
| `SetDetails` on `OverrideStore`, calling into `WorkInfoStore` | Rejected: ARCHITECTURE.md forbids a role type calling another role's public methods; the display name write is already the package functions `registeredContentKeysForVideoIDs`, `displayNamesOf`, `writeDisplayName`, `touchEditedAt` and `refreshSearchKeysForContentKeys`, which `SetDetails` calls in its own transaction, and the work information SQL belongs to `WorkInfoStore` |

**Rationale**: The maintainer revised requirement 8 so that the editor saves
the title and the information as one unit; the API follows that screen, not
the other way round. The store already has the display name write as
transaction-scoped functions, so one transaction covering both is the
existing shape of `SetDisplayName` plus the rows of R-2, and the two kept
routes keep the in-place title edit and the external contract unchanged.
