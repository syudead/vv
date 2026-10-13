# Research: Work information on videos

Parent Issue: #967. Inherited decisions:

| Topic | Source of truth |
| --- | --- |
| Boundaries, dependency direction, index versus user data, events after commit | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| User key (`bundle:<id>` for a bundle member, otherwise `content_key`), bundle copy and restore, same-path succession | [specs/030-video-versions/data-model.md, User key](../030-video-versions/data-model.md#user-key), [Store operations (`VersionStore`)](../030-video-versions/data-model.md#store-operations-versionstore), [Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| Display name rules (control characters, trim, empty clears, a code point limit) | [specs/029-video-overrides/research.md R-10](../029-video-overrides/research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit) |
| Search matching form (`FoldForMatch`) and how tag names join the search box | [specs/013-library-search/data-model.md, `search_key` rules](../013-library-search/data-model.md#search_key-rules), [specs/014-video-tags/data-model.md, Matching tag names in the search box](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box) |
| Tag filter on the list API and in the library URL | [specs/014-video-tags/contracts/tags-api.md, List filter and Select all](../014-video-tags/contracts/tags-api.md#list-filter-and-select-all), [list-url.md](../014-video-tags/contracts/list-url.md) |
| What guests see and which conditions they cannot use | [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| Edit time (`updatedAt`) | [specs/033-video-dates/research.md](../033-video-dates/research.md) |
| External API conventions, bulk operations and MCP tools | [specs/026-external-api/contracts/external-api.md, Common rules](../026-external-api/contracts/external-api.md#common-rules), [specs/029-video-overrides/research.md R-9](../029-video-overrides/research.md#r-9-the-external-api-adds-two-bulk-operations-and-mcp-the-same-two-tools) |

This file records only the decisions this feature adds.

## R-1: Six fixed fields; performers and the maker get lists

**Decision**: Work information is the fixed set `code`, `releaseDate`,
`maker`, `performers` (an ordered list of names), `description` and
`sourceUrl` ([data-model.md, Fields](data-model.md#fields)). Lists of videos
with the same value (requirement 5) exist for `performers` and `maker` only.

| Field | List by value | Reason |
| --- | --- | --- |
| `performers` | Yes | A name recurs across works; "every video with this performer" is the list the requirement names |
| `maker` | Yes | Same: one maker, many works |
| `code` | No | One code is one work; files with the same code are versions of it and belong in a bundle ([030](../030-video-versions/plan.md)) |
| `releaseDate` | No | Same dates recur by chance; a date is a range or a sort, which is another mechanism and not asked for |
| `description`, `sourceUrl` | No | Free text and a link; neither groups videos |

| Option | Verdict |
| --- | --- |
| **Six fixed fields, two of them with lists** | Chosen |
| Owner-defined fields | Rejected: the parent Issue's `対象外` |
| One free-text block | Rejected: requirements 4 and 5 need the code and the names as values that can be matched and listed |

**Rationale**: The parent Issue names these six as its example and defers the
choice to the plan; they are the fields a scraper (#968) can fill for a
published work, and each one has a different rule (a date, a URL, a list), so
one typed field per item keeps validation and display per field.

## R-2: Two user-data tables keyed by the user key, copied by the bundle and succession paths

**Decision**: `video_work_info` (one row per key, the five single-value fields)
and `video_work_performers` (one row per name and position) are user-data
tables keyed by the user key, the same key as `video_tags` and
`video_favorites` ([data-model.md, Migration](data-model.md#migration)). Both
join `userDataTables` (bundling copies the representative's rows to the bundle
key, a removed member gets its own rows back) and `moveUserData` (same-path
succession). Neither table has a foreign key to `videos`.

| Option | Verdict |
| --- | --- |
| **User key, like tags and favorites** | Chosen |
| Content key, like `video_overrides` | Rejected: the Edge Case asks for one unit per bundle; the information describes the work, which every version shares, and per-content rows would have to be copied to every member on bundling |
| A JSON column holding the whole object | Rejected: the performer list (R-4) and the search condition (R-3) match one name per row, and `json_each` on every candidate row runs the parse once per video per list page |
| Columns on `videos` | Rejected: the index is rebuilt by a scan (requirement 6) |

**Rationale**: Keying by the user key makes requirement 6 and the three Edge
Cases (same content in several places, bundles, moves and renames) properties
the existing paths already give: a rescan reproduces the content key, a bundle
has one key, and `userDataTables` and `moveUserData` are the one place the
bundle and succession operations copy user data.

## R-3: The search box matches the code, the maker and the performer names through a per-key search key

**Decision**: `video_work_info.search_key` holds `FoldForMatch` of the code,
the maker and every performer name, joined by newlines, written in the same
transaction as the values and refilled at startup when `search_version` is
behind `domain.SearchKeyVersion` (as `tag_names.search_key` is). The search
condition gains, for the owner, `instr(search_key, <term>) > 0` on the row of
the location's video's user key, as a third branch of the OR that
`tagNameMatchCondition` builds ([data-model.md, Search](data-model.md#search)).
Guests do not match it (R-7). The description, the date and the URL are not in
the key.

| Option | Verdict |
| --- | --- |
| **A key per user key, matched with `instr` like tag names** | Chosen |
| Fold the values into each location's `search_key` (as the display name is) | Rejected: the values belong to the user key, so a bundle's values would be written into every member's locations, and every bundle, unbundle and succession would have to refresh location keys |
| An FTS5 trigram table over the values | Rejected: a trigram index needs three characters, and codes such as `AB-12` and two-character names are the common case in this data; tag names already use `instr` for the same reason |
| Include the description | Rejected: a long description matches generic words and turns the search box into a full-text search of notes, which requirement 4 does not ask for |

**Rationale**: The search box has to find a video by its code or a performer
name (acceptance criterion 3). Tag names are the precedent for a per-video
value matched beside the location key, and sharing their shape means the
owner's OR gains one branch and nothing else changes.

## R-4: `performer` and `maker` list parameters, matched by folded value, library only

**Decision**: `GET /api/videos`, `GET /api/library` and `GET /api/library/ids`
gain `performer` and `maker`, one value each, ANDed with every other
condition. A value matches when `FoldForMatch` of it equals the stored
`name_key` or `maker_key` ([contracts/screen-api.md, List
parameters](contracts/screen-api.md#list-parameters-performer-and-maker)). The
library URL carries `performer=<name>` and `maker=<name>`; pressing a
performer or the maker on the video page opens `/?performer=…` or
`/?maker=…`. `listFolderVideos` does not take them, as it does not take `tag`.
Guests get `400` (R-7).

| Option | Verdict |
| --- | --- |
| **Value parameters on the list API and the library URL** | Chosen |
| Performers and makers as entities with ids, like tags | Rejected: nothing in the parent Issue manages them (no rename, merge or synonyms), so an id table would be a second registry with no owner-facing operation |
| A page of its own (`/performers/<name>`) | Rejected: the library already has the URL, snapshot and condition machinery, and a filter composes with the search box and tags, which a separate page would have to rebuild |
| Match the raw spelling | Rejected: `山田 花子` and `山田　花子` would be two performers; the search box already matches by the folded form |

**Rationale**: Acceptance criterion 6 is "press the value, see the videos with
the same value"; a value in the URL does exactly that, and a renamed value
changing the URL is acceptable because nothing links to it but the page the
value is on.

## R-5: One `PUT` that replaces the whole object; a cleared field is absent, `null` or empty

**Decision**: `PUT /api/videos/{id}/work-info` takes the whole object and
replaces it; a field that is absent, `null` or empty after trimming is cleared,
and a row whose every field is cleared is deleted. The response is `Video`.
`domain.NormalizeWorkInfo` applies the rules in
[data-model.md, Rules](data-model.md#rules) and returns one
`InvalidWorkInfoError` naming the field and the problem
([contracts/screen-api.md, `PUT /api/videos/{id}/work-info`](contracts/screen-api.md#put-apivideosidwork-info)).
A write that changes a value advances the edit time of every content key under
the user key, as tags do; a write that changes nothing leaves it.

| Option | Verdict |
| --- | --- |
| **Replace the whole object** | Chosen |
| One `PUT` per field (`/work-info/code`, …) | Rejected: the page saves a form, and the external bulk operation (R-6) sends whole objects; per-field routes multiply the routes and the error mapping by six |
| `PATCH` with merge semantics | Rejected: "clear this field" then needs a sentinel distinct from "leave it", which the screen never needs; a client that wants a merge reads the object first |

**Rationale**: The page edits the fields together and the scraper writes them
together, so the unit of change is the object; replacement is idempotent, so a
resend after a network failure is safe.

## R-6: The external API gets one bulk operation and MCP one tool; `workInfo` joins `ExternalVideo`

**Decision**: `POST /api/v1/video-work-info` takes `items` of `{ video,
workInfo }` (1 to 20000, one transaction, nothing applied on any error) with
the same replacement semantics as R-5, and `ExternalVideo` gains the required
field `workInfo` ([contracts/external-api.md](contracts/external-api.md)). MCP
gains `update_video_work_info`; `list_videos` and `get_video` carry the new
field. Errors add `field` to the error body, so a client learns which field of
which item was refused (`index`).

| Option | Verdict |
| --- | --- |
| **One bulk replace, `field` in the error body** | Chosen |
| One reason per field and problem (`code_too_long`, `maker_too_long`, …) | Rejected: eighteen enum values for six fields and three problems, and the screen's catalog would need a message for each |
| A `merge` action beside `replace` | Rejected: the one writer this feature serves (#968) holds the whole object; `v1` can gain `action` later without breaking clients |

**Rationale**: `POST /api/v1/video-display-names` is the shape external
writers already use; keeping it means the scraper in #968 adds one call. A
`field` property is an addition under the compatibility policy, and the same
property serves the screen's error line.

## R-7: Guests receive no work information, in responses, search or filters

**Decision**: Guest responses omit `workInfo` (as they omit `displayName` and
empty `tags`), the guest search condition has no work-information branch, and
`performer` or `maker` from a guest returns `400`
`guest_filter_not_allowed`, through `Audience.CheckVideoQuery`.

| Option | Verdict |
| --- | --- |
| **Owner only, like tags** | Chosen |
| Show it on public videos and let guests search and filter | Rejected: the guest contract removes every owner-entered field but the title; #968 will fill these fields from third parties, and the owner publishes a video, not notes about it |

**Rationale**: Every requirement of the parent Issue names the owner, and the
guest contract's rule is that conditions depending on the owner's data are not
offered; adding a field to that rule costs one line in `CheckVideoQuery` and
one in `forAudience`, while widening the guest surface is a decision the Issue
does not make.

## R-8: A change publishes `domain.VideoWorkInfoChanged`, mapped to the screen's `video` notification

**Decision**: `WorkInfoStore` publishes `VideoWorkInfoChanged{VideoIDs}` after
commit, one event per write, and `httpapi.Events.Handle` maps it to a `video`
notification for each id, as `VideoBundleChanged` is mapped. No worker
subscribes to it.

| Option | Verdict |
| --- | --- |
| **A new event with the ids** | Chosen |
| Reuse `VideoOverrideChanged` | Rejected: overrides are per content and carry one id; a bundle write affects every member, and a reader of the event should not have to know that the name covers two tables |
| No event; the page refetches after its own `PUT` | Rejected: the external API writes while the page is open (acceptance criterion 5), and the page shows the value without a reload only through a notification |

**Rationale**: The video page and the list already reread a video on `video`,
so one mapping line gives acceptance criterion 5 for free.
