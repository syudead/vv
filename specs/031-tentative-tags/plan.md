# Implementation Plan: Tentative tags: confirm or reject tags newly created by automatic tagging

**Branch**: `feature/031-tentative-tags` | **Parent Issue**: #589

**Input**: The parent Issue. It is this feature's specification.

## Summary

Add `tentative` to bulk tagging in the external API and MCP, and mark tags newly
created by such a call as **tentative tags**. On the tag management page the user
can tell tentative tags apart, filter to them, and settle them by confirming,
rejecting or merging. Rejected names are remembered and skipped by later
tentative creates.

| Area | Decision |
| --- | --- |
| Storage | Tentative state is one column, `tags.tentative`; rejected names are a name-only table, `rejected_tag_names`. The SQL for attaching, detaching, filtering, search and counts does not change ([research.md R-1](research.md#r-1-tentative-state-is-one-column-on-tags), [R-2](research.md#r-2-rejected-names-live-in-a-name-only-table-rejected_tag_names-matched-exactly-like-tag-names), [data-model.md](data-model.md)) |
| Rules | A transaction that writes a name to `tag_names` removes the same name from the rejected list. Renaming a tentative tag, adding a synonym to it, or making it a merge target confirms it in the same transaction. Reject applies only to tentative tags ([R-3](research.md#r-3-a-transaction-that-writes-a-name-to-tag_names-removes-it-from-rejected_tag_names), [R-4](research.md#r-4-edits-to-a-tentative-tag-rename-synonym-merge-target-confirm-it-in-the-same-transaction), [R-5](research.md#r-5-reject-applies-only-to-tentative-tags-a-confirmed-tag-gets-409-tag_not_tentative-and-no-change), [R-6](research.md#r-6-rejection-and-a-tentative-attach-of-the-same-name-rely-on-sqlite-write-serialization)) |
| API | Every response that represents a tag gains `tentative`. The screen API gains two `POST` routes for tentative tags and `GET` and `DELETE` for rejected names ([contracts/screen-api.md](contracts/screen-api.md), [R-9](research.md#r-9-the-screen-api-adds-two-post-routes-for-tentative-tags-and-get-and-delete-for-rejected-names)). The external API gains `tentative` in the body and `skippedTags` in the response; MCP gets them from the same type ([contracts/external-api.md](contracts/external-api.md), [R-7](research.md#r-7-the-external-api-adds-tentative-to-the-request-and-skipped-names-to-the-response)) |
| Screens | Marking and filtering rows on the tag management page, confirm, reject and merge actions, and the rejected-name list. Marking chips on library cards and the video page. The Issue has the `ui` label, so the next stage, design, settles appearance and interaction in `ui-design.md` against the parent Issue's `UI品質` section ([R-8](research.md#r-8-the-tentative-only-filter-and-the-rejected-name-list-live-in-the-screen)) |

## Technical Context

**Canonical definitions**:

| Topic | Sources |
| --- | --- |
| Boundaries, dependency direction, index versus user data, role-type rules, authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md), [.golangci.yml](../../.golangci.yml) (depguard) |
| Tag tables, name rules, write rules, filtering, search, counts | [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md), [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md), [internal/domain/tag.go](../../internal/domain/tag.go), [internal/store/tags.go](../../internal/store/tags.go), [internal/store/tag_lookup.go](../../internal/store/tag_lookup.go), [internal/store/tag_synonyms.go](../../internal/store/tag_synonyms.go), [internal/store/tag_listing.go](../../internal/store/tag_listing.go), [internal/store/video_tags.go](../../internal/store/video_tags.go), [internal/store/invariants_test.go](../../internal/store/invariants_test.go) |
| Folder-derived tags and group-to-tag conversion | [specs/017-folder-groups/data-model.md, Folder-derived tags](../017-folder-groups/data-model.md#folder-derived-tags), [internal/store/folder_tags.go](../../internal/store/folder_tags.go), [internal/store/folder_groups.go](../../internal/store/folder_groups.go) |
| Responses to guests | [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| Bulk tagging in the external API, and MCP | [api/external-v1.yaml](../../api/external-v1.yaml), [specs/026-external-api/contracts/external-api.md, Video tags](../026-external-api/contracts/external-api.md#video-tags), [specs/026-external-api/contracts/mcp.md](../026-external-api/contracts/mcp.md), [internal/domain/external_video_tags.go](../../internal/domain/external_video_tags.go), [internal/store/external_video_tags.go](../../internal/store/external_video_tags.go), [internal/httpapi/external_video_tags.go](../../internal/httpapi/external_video_tags.go), [internal/httpapi/mcp.go](../../internal/httpapi/mcp.go), [docs/how-to/external-api.md](../../docs/how-to/external-api.md) |
| Screen API and error shape | [api/openapi.yaml](../../api/openapi.yaml), [internal/httpapi/tags.go](../../internal/httpapi/tags.go), [internal/httpapi/router.go](../../internal/httpapi/router.go) (`Tags` interface), [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md) |
| Screens | [specs/014-video-tags/ui-design.md](../014-video-tags/ui-design.md) (Tag chip, Library card, Video page tags, Tag management page), [web/src/tags/](../../web/src/tags/), [web/src/api/tags.ts](../../web/src/api/tags.ts) (shared list), [web/src/library/CardTagRow.tsx](../../web/src/library/CardTagRow.tsx), [web/src/player/VideoTags.tsx](../../web/src/player/VideoTags.tsx), [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md), [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md) |
| Generation and check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`) |

**Feature-specific context**:

- One migration (`00022_tentative_tags.sql`: the `tags.tentative` column and the
  `rejected_tag_names` table). `tag_names` and `video_tags` do not change.
- No new Go or npm dependency. No new domain event (tag changes have no side
  effects).
- `SearchKeyVersion` is not raised. Rejected names have no matching key.
- No `quickstart.md`. The acceptance criteria are verified by store, httpapi and
  Vitest tests, and no step runs outside the repository's checks.

## Constitution Check

| Gate | Verdict |
| --- | --- |
| Dependency direction (ARCHITECTURE.md "Intended dependency direction") | Pass. `internal/domain`: `TagRef.Tentative`, `Tag.Tentative`, `ErrTagNotTentative`, `VideoTagsOutcome`; plain values only. `internal/store`: the migration, added and changed `TagStore` operations, invariants. `internal/httpapi`: request parsing, response conversion, the external API and MCP; the rules live in domain and store. `internal/app` and `cmd/mdm`: untouched (tag operations still bypass `internal/app`) |
| A role type does not call another role's public methods (ARCHITECTURE.md, `store.DB` paragraph) | Pass. Deleting a rejected name happens inside `insertTagName`, and group-to-tag conversion in `FolderGroupStore` keeps using the package-internal `findOrCreateTag` (data-model.md, [Store operations](data-model.md#store-operations)) |
| Index versus user data | Pass. `tags.tentative` and `rejected_tag_names` are user data and are added to the list in ARCHITECTURE.md (data-model.md, [Migration](data-model.md#migration)) |
| API sources of truth and generated files (AGENTS.md) | Pass. Edit `api/openapi.yaml` and `api/external-v1.yaml`, then run `task generate`. The external API only gains fields (the 026 compatibility policy) |
| Guests do not see the owner's data (guest-api.md) | Pass. A guest's `tags` stays an empty array, and every new route is owner-only (requirement 4) |
| Server output in English, screen text in the catalog (`.golangci.yml` gosmopolitan, i18n.md) | Pass |
| Design documents describe the current state (docs/design-docs/index.md, design-document policy) | Pass. Each unit updates its own part of ARCHITECTURE.md, `docs/how-to/external-api.md` and the descriptions in `api/*.yaml` |

The verdicts are the same after Phase 1. No violation goes in Complexity
Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/031-tentative-tags/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-9
├── data-model.md         # tags.tentative, rejected_tag_names, write rules, domain values, store operations
└── contracts/
    ├── screen-api.md     # Screen API: schema changes, four routes, changed existing routes
    └── external-api.md   # tentative and skippedTags on /api/v1/video-tags, Tag changes, MCP
```

The next stage, design, creates `ui-design.md` (`ui` label). No `quickstart.md`
(see Technical Context).

### Source Code

**Affected boundaries**:

- `internal/domain` (`TagRef` and `Tag` fields, the error value, the bulk
  operation result) and `internal/store` (migration, `TagStore`, invariant checks)
- `internal/httpapi` (the four routes in `tags.go` and `toAPITag` /
  `toAPITagRef`, `external_video_tags.go`, descriptions in `mcp.go`, the `Tags`
  interface in `router.go`), `api/openapi.yaml`, `api/external-v1.yaml` and the
  generated files
- `web/src/api` (types, four functions, the shared list), `web/src/tags` (tag
  management page), `web/src/library` and `web/src/player` (chips),
  `web/src/i18n`
- `ARCHITECTURE.md`, `docs/how-to/external-api.md`

**New paths**:

- `internal/store/migrations/00022_tentative_tags.sql`,
  `internal/store/rejected_tag_names.go`
- Components in `web/src/tags/` for the filter, the tentative-tag actions and the
  rejected-name list (names follow the design stage)

**Structure decision**: Tentative-tag operations and rejected names live in
`TagStore`; no new role type is created. Rejecting is one transaction ("delete the
tag + remember the name"), and a tentative create is one transaction ("resolve
the name + check rejected names + create"); both must read and write in the same
transaction as `tags` and `tag_names`. A separate role could not build that
transaction under the rule that a role type does not call another role's public
methods.

## Implementation Work

### Store tentative tags and rejected names, and carry tentative state in tag reads

**Scope**: `00022_tentative_tags.sql`; the `domain` values
([data-model.md, Values added to `domain`](data-model.md#values-added-to-domain)); new `TagStore`
operations and changes to existing ones
([Write rules](data-model.md#write-rules) and [Store operations](data-model.md#store-operations)): `tentative` and skipped names in
`ApplyVideoTags`, `ConfirmTag`, `RejectTag`, `ListRejectedTagNames`,
`ForgetRejectedTagName`, confirming on rename, synonym and merge target, deleting
rejected names at the name-writing entry points, and reads that carry
`tentative`. The two invariants in `invariants_test.go`
([Migration](data-model.md#migration)). Additions to the `httpapi.Tags` interface and
the minimum handler changes to satisfy it (the `ApplyVideoTags` signature). The
user-data list and the `TagStore` paragraph in ARCHITECTURE.md.

**Dependencies**: None

**Acceptance**: `task check` passes. Store tests show:

- An `add` with `tentative: true` creates a missing name exactly once with
  `tentative = 1` and attaches it to every target; attaching the name or synonym
  of an existing confirmed tag leaves its state unchanged (acceptance criteria 1
  and 3, edge case "the same new name on several videos").
- With `tentative` false, the tag is created as confirmed (acceptance criterion 2).
- Attaching a rejected tag's name with `tentative: true` returns only that name as
  skipped and attaches the other names; with `replace` the name does not enter
  the replacement set, and when every name is rejected the set is replaced with
  an empty one (acceptance criterion 9, edge case).
- `RejectTag` deletes the tag and its assignments and adds the name to the list;
  on a confirmed tag it returns `ErrTagNotTentative` and changes nothing
  (acceptance criterion 8).
- After `ConfirmTag`, the videos the tag is on are unchanged (acceptance
  criterion 7).
- Renaming a tentative tag, registering a synonym on it, or making it a merge
  target sets `tentative = 0`; a rename to the same name does not (acceptance
  criterion 11).
- Using a rejected name with `CreateTag`, `RenameTag`, `AddSynonym`,
  `AttachTagByName`, `ApplyVideoTags` with `tentative` false, or group-to-tag
  conversion removes it from the list (acceptance criterion 14).
- After `ForgetRejectedTagName`, the same name is created again as a tentative
  tag, and removing an absent name is not an error (acceptance criterion 13, edge
  case).
- After the migration every existing tag has `tentative = 0` and the rejected-name
  list is empty (edge case "migrating existing data").
- The results of `ListTags`, `TagsByContentKeys`, `Summary` and `AttachTagByID`
  carry `Tentative`.

### Return tentative state in the screen API and add confirm, reject and rejected-name operations

**Scope**: `tentative` on `TagRef`, `VideoTag` and `Tag` in `api/openapi.yaml`,
`RejectedTagNameList`, `tag_not_tentative`, the four routes and the generated
files ([contracts/screen-api.md, Schema changes](contracts/screen-api.md#schema-changes) to [Rejected names](contracts/screen-api.md#rejected-names)).
The handlers in `internal/httpapi/tags.go`, `accessRoutes`, `requiresJSONBody`
and `openapi_routes_test.go` (`/api/tags/rejected-names` is not captured by
`{id}`). The types and four functions in `web/src/api`, the `errorText` text
([`web/src/api` functions](contracts/screen-api.md#websrcapi-functions)), and updates to Vitest
fixtures now that the generated types require `tentative`.

**Dependencies**: Store tentative tags and rejected names, and carry tentative
state in tag reads

**Acceptance**: `task check` passes. httpapi tests show:

- `tentative` appears in `GET /api/tags`, the `tags` of `GET /api/videos/{id}`,
  the `tag` of `POST /api/video-tags`, and `items[].tag` of
  `POST /api/video-tags/summary` (requirement 5).
- `POST /api/tags/{id}/confirm` returns `200` and a `Tag` with
  `tentative: false`, and `200` for an already confirmed tag.
- `POST /api/tags/{id}/reject` returns `204`; afterwards the tag is absent from
  `GET /api/tags` and from the video's `tags`, and its name appears in
  `GET /api/tags/rejected-names` (acceptance criterion 8). `reject` on a
  confirmed tag returns `409 tag_not_tentative` and changes nothing.
- `DELETE /api/tags/rejected-names?name=…` returns `204` and removes the name,
  and returns `204` for an absent name.
- The responses of `PATCH`, `synonyms` and `merge` return a tentative tag with
  `tentative: false` (acceptance criterion 11).
- A guest gets `401` on all four routes, and a guest's `Video.tags` stays empty
  (acceptance criterion 4).

### Attach tentatively and return skipped names in the external API and MCP

**Scope**: `VideoTagsRequest.tentative`, `VideoTagsResponse.skippedTags`, and
`tentative` on `Tag` and `ExternalVideoTag` in `api/external-v1.yaml` with the
generated files; `internal/httpapi/external_video_tags.go`; the
`update_video_tags` description in `mcp.go`
([contracts/external-api.md](contracts/external-api.md)). The section on tagging
videos (`動画にタグを付ける`) and the scraper example in
`docs/how-to/external-api.md`.

**Dependencies**: Store tentative tags and rejected names, and carry tentative
state in tag reads

**Acceptance**: `task check` passes. httpapi tests show:

- A token-authenticated `POST /api/v1/video-tags` with `tentative: true` and a
  missing name returns `200`, the response's `tags` show the name with
  `tentative: true`, and `GET /api/v1/tags` and the screen's `GET /api/tags`
  show it as tentative (acceptance criterion 1).
- Omitting `tentative` creates a confirmed tag, and a rejected name is removed
  from the list (acceptance criteria 2 and 15).
- Sending a rejected name together with a new name returns `200`, attaches only
  the new name, and lists the rejected name once in `skippedTags` (acceptance
  criterion 9). With `remove`, `skippedTags` is empty.
- A non-boolean `tentative` returns `400`.
- MCP's `update_video_tags` gives the same results, and the output of `list_tags`
  shows `tentative` (acceptance criterion 15).

### Mark, filter, confirm, reject and merge tentative tags on the tag management page, and manage rejected names

**Scope**: In `web/src/tags/`: the tentative mark on rows, the tentative-only
filter, the "Confirm", "Reject…" and "Merge into another tag…" actions on
tentative rows ("Delete…" is not offered), the reject confirmation, and the
rejected-name list with removal. `tag_not_tentative` triggers the same reload as
`tag_not_found`. English catalog text. Appearance and interaction follow
`ui-design.md` ([R-8](research.md#r-8-the-tentative-only-filter-and-the-rejected-name-list-live-in-the-screen)).

**Dependencies**: Return tentative state in the screen API and add confirm, reject
and rejected-name operations

**Acceptance**: The screen changes (visual and interaction review required).
`task check` passes. Vitest tests show:

- A tentative row is distinguishable from a confirmed one (acceptance
  criterion 5), and the filter lists only tentative tags (acceptance criterion 6).
- Confirming turns the row into a confirmed tag (acceptance criterion 7).
- Rejecting removes the row and the name appears in the rejected-name list
  (acceptance criterion 8).
- A tentative row has "Reject…" and no "Delete…", and a confirmed row is
  unchanged (acceptance criterion 12).
- A rejected name can be removed from the list (acceptance criterion 13).
- `tag_not_tentative` closes the dialog and reloads the list (edge case
  "conflicting operations").
- Creating a rejected name with "New tag" removes it from the list (acceptance
  criterion 14).

### Mark tentative tags on library cards and video page chips

**Scope**: The tentative mark on chips in `web/src/library/CardTagRow.tsx`
(including group cards) and `web/src/player/VideoTags.tsx`, and reflecting
`tentative` from the attach-by-name response. English catalog text. Appearance
follows `ui-design.md` (`UI品質` "visual hierarchy").

**Dependencies**: Return tentative state in the screen API and add confirm, reject
and rejected-name operations

**Acceptance**: The screen changes (visual and interaction review required).
`task check` passes. Vitest tests show that the chip of a tag with
`tentative: true` is distinguishable from a confirmed tag while the name stays
primary (acceptance criterion 5). Chips of confirmed tags are unchanged. Clicking
a tentative chip to filter and removing it with × work as before (acceptance
criterion 4). Guest screens show no tags.
