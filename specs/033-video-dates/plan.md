# Implementation Plan: Keep each video's edit time and file creation time, and use them in display, sorting and the external API

**Branch**: `feature/033-video-dates` | **Parent Issue**: #630

**Input**: The parent Issue. It is this feature's specification.

## Summary

For each video, keep "when its information was last edited in vv" (the edit time)
and "the creation time of the listed location's file" (the creation time); show
both in the video page's facts row, add "Date created" to the list sort orders,
and include both on videos in the external API. The existing "Date modified"
(mtime) sort keeps its name and behaviour.

| Area | Decision |
| --- | --- |
| Edit time | Kept in a user-data table `video_edits` keyed by content key; reads fall back to the added time. Only four kinds of writes advance it (display name, thumbnail position, visibility, attaching and detaching manual tags), and only for content keys whose rows actually changed. Same-path succession re-keys it ([research.md R-1](research.md#r-1-the-edit-time-lives-in-a-user-data-table-video_edits-keyed-by-content-key-and-reads-fall-back-to-the-added-time), [R-2](research.md#r-2-only-the-four-writes-of-video-information-advance-the-edit-time-tag-level-operations-bundles-and-ingestion-do-not), [R-3](research.md#r-3-unchanged-edits-do-not-advance-and-only-the-content-keys-a-write-actually-changed-advance), [data-model.md §1 and §3](data-model.md)) |
| Creation time | The scan writes the location column `video_locations.file_created_at` (null when unavailable); reads and sorting use `coalesce(file_created_at, mtime)`. `internal/scanner` reads it per OS (statx from `x/sys/unix` on Linux) and rewrites only the location column when an unchanged file's value differs ([R-4](research.md#r-4-the-file-creation-time-lives-in-a-location-column-video_locationsfile_created_at-null-when-unavailable-and-reads-fall-back-to-mtime), [R-5](research.md#r-5-the-file-system-adapter-internalscanner-reads-the-creation-time-per-os-linux-uses-statx-from-golangorgxsysunix), [R-6](research.md#r-6-a-registered-location-is-rewritten-on-the-next-scan-when-its-creation-time-differs-even-if-the-file-is-unchanged), [data-model.md §4 and §5](data-model.md)) |
| API | Required `updatedAt` and `fileCreatedAt` on `Video` and `ExternalVideo`; `createdAsc` and `createdDesc` on `VideoSort` ([R-7](research.md#r-7-the-response-fields-are-updatedat-edit-time-in-vv-and-filecreatedat-location-creation-time-with-the-same-names-in-the-screen-and-external-apis), [contracts/screen-api.md](contracts/screen-api.md), [contracts/external-api.md](contracts/external-api.md)) |
| Screens | Two facts in the video page's facts row and "Date created" in the list sort orders. The video is reloaded after tag and visibility changes ([R-8](research.md#r-8-the-video-page-reloads-the-video-after-tag-and-visibility-changes-and-no-new-domain-event-is-added)). The Issue has the `ui` label, so the next stage, design, settles appearance, the facts' names and order, and the sort name in `ui-design.md` against the parent Issue's `UI品質` section. List cards do not change |

## Technical Context

**Canonical definitions**:

| Topic | Sources |
| --- | --- |
| Boundaries, dependency direction, index versus user data, role-type rules, domain events, authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md), [.golangci.yml](../../.golangci.yml) (depguard) |
| Scan and locations | [internal/scanner/scanner.go](../../internal/scanner/scanner.go) (`Index` interface, the unchanged-file branch), [internal/store/scan_index.go](../../internal/store/scan_index.go) (`UpsertVideo`, `IndexedVideosByPath`), [internal/domain/video.go](../../internal/domain/video.go) (`VideoFile`, `IndexedVideo`) |
| Video reads and lists | [internal/store/videos.go](../../internal/store/videos.go) (`videoColumnsTemplate`), [internal/store/listing.go](../../internal/store/listing.go) (`listColumns`, `listOrders`), [internal/store/library_items.go](../../internal/store/library_items.go) (`libraryItemsCTE`, `itemOrderValues`), [internal/domain/library.go](../../internal/domain/library.go) (`VideoSort`), [specs/013-library-search/contracts/list-api.md](../013-library-search/contracts/list-api.md), [specs/017-folder-groups/contracts/library-api.md](../017-folder-groups/contracts/library-api.md) |
| User data keyed by content key, and its writes | [specs/029-video-overrides/data-model.md](../029-video-overrides/data-model.md), [internal/store/overrides.go](../../internal/store/overrides.go), [internal/store/visibility.go](../../internal/store/visibility.go), [internal/store/video_tags.go](../../internal/store/video_tags.go), [internal/store/external_video_tags.go](../../internal/store/external_video_tags.go), [internal/store/user_keys.go](../../internal/store/user_keys.go) (`userKeysForVideoIDs`, `contentKeysForUserKeys`), [internal/store/successions.go](../../internal/store/successions.go) (`moveUserData`), [specs/030-video-versions/data-model.md](../030-video-versions/data-model.md) §3 and §5 |
| Responses to guests | [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| External API | [api/external-v1.yaml](../../api/external-v1.yaml), [specs/026-external-api/contracts/external-api.md](../026-external-api/contracts/external-api.md), [internal/httpapi/external_videos.go](../../internal/httpapi/external_videos.go), [docs/how-to/external-api.md](../../docs/how-to/external-api.md) |
| Screen API and conversion | [api/openapi.yaml](../../api/openapi.yaml), [internal/httpapi/videos.go](../../internal/httpapi/videos.go) (`toAPIVideo`) |
| Screens | [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) "Video facts", [specs/013-library-search/ui-design.md](../013-library-search/ui-design.md) "Sort and direction", [web/src/player/VideoFacts.tsx](../../web/src/player/VideoFacts.tsx), [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx), [web/src/videoList/listCriteria.ts](../../web/src/videoList/listCriteria.ts) (`sortKinds`), [web/src/videoList/SortControls.tsx](../../web/src/videoList/SortControls.tsx), [web/src/preferences/viewPreferences.ts](../../web/src/preferences/viewPreferences.ts), [web/src/i18n/en.ts](../../web/src/i18n/en.ts), [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md) |
| Generation and check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`) |

**Feature-specific context**:

- Two migrations (`00027_video_edits.sql`: the `video_edits` table;
  `00028_video_file_created_at.sql`: the `video_locations.file_created_at`
  column). Each belongs to the implementation unit that adds it.
- `golang.org/x/sys` becomes a direct Go dependency (it is indirect today; only
  the Linux file in `internal/scanner` uses it). No npm dependency is added.
- No domain event is added (R-8). `SearchKeyVersion` is not raised.
- `quickstart.md` holds the verification steps for acceptance criteria 4 and 5,
  which depend on real file-system creation times.

## Constitution Check

| Gate | Verdict |
| --- | --- |
| Dependency direction (ARCHITECTURE.md "Intended dependency direction") | Pass. `internal/domain`: `time.Time` fields on `Video`, `VideoFile`, `IndexedVideo` and `VideoLocation`, and two `VideoSort` values; touches neither `os` nor `x/sys`. `internal/scanner`: reading the creation time (per-OS build tags); the adapter reads file-system facts. `internal/store`: migrations, read columns, `touchEditedAt`, `UpdateLocationCreatedAt`. `internal/httpapi`: response conversion and `sort` validation. `internal/app` and `cmd/mdm`: untouched |
| A role type does not call another role's public methods (ARCHITECTURE.md, `store.DB` paragraph) | Pass. `touchEditedAt` is a package-internal function that `OverrideStore`, `VisibilityStore` and `TagStore` each call inside their own transactions (data-model.md §3) |
| Index versus user data | Pass. `video_edits` is user data and `video_locations.file_created_at` is index; both are added to the list in ARCHITECTURE.md (data-model.md §1) |
| Only a user-started scan walks the media folders (ARCHITECTURE.md, `internal/scanner` paragraph) | Pass. Creation times of existing locations are also filled in during the scan (R-6) |
| API sources of truth and generated files (AGENTS.md) | Pass. Edit `api/openapi.yaml` and `api/external-v1.yaml`, then run `task generate`. The external API only gains fields (the 026 compatibility policy) |
| Guests do not see the owner's data (guest-api.md) | Pass. The two fields and the `created*` sort orders are facts about public videos, and no owner-operation route is added (R-7) |
| Server output in English, screen text in the catalog (`.golangci.yml` gosmopolitan, i18n.md) | Pass |
| Design documents describe the current state (docs/design-docs/index.md, design-document policy) | Pass. Each unit updates its own part of ARCHITECTURE.md (the user-data list, "thirteen sort orders", and the `TagStore`, `VisibilityStore`, `OverrideStore` and `ScanIndexStore` paragraphs), `docs/how-to/external-api.md`, and `api/*.yaml` |

The verdicts are the same after Phase 1. No violation goes in Complexity
Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/033-video-dates/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-8
├── data-model.md         # video_edits, file_created_at, domain values, advance rules, reads, scan
├── quickstart.md         # Checking creation times on a real file system (acceptance criteria 4 and 5)
└── contracts/
    ├── screen-api.md     # Two Video fields, two VideoSort values, web/src/api changes
    └── external-api.md   # Two ExternalVideo fields
```

The next stage, design, creates `ui-design.md` (`ui` label).

### Source Code

**Affected boundaries**:

- `internal/domain` (values and `VideoSort`), `internal/scanner` (reading the
  creation time and the unchanged-file branch), `internal/store` (migrations,
  reads, `touchEditedAt` and the four kinds of writes, `UpdateLocationCreatedAt`,
  succession, invariants)
- `internal/httpapi` (`toAPIVideo`, `external_videos.go`, `sort` validation),
  `api/openapi.yaml`, `api/external-v1.yaml` and the generated files, `go.mod`
- `web/src/api` (`videoSorts`, types), `web/src/player` (facts row, reload),
  `web/src/videoList` (sort order), `web/src/preferences`, `web/src/i18n`
- `ARCHITECTURE.md`, `docs/how-to/external-api.md`. Nothing is added to
  `specs/013-library-search/contracts/list-api.md`; this feature's
  [data-model.md §4](data-model.md#4-reads) holds the delta

**New paths**:

- `internal/store/migrations/00027_video_edits.sql` and
  `00028_video_file_created_at.sql`, `internal/store/video_edits.go`
  (`touchEditedAt`)
- `internal/scanner/file_created_at_linux.go`, `file_created_at_bsd.go`
  (`//go:build darwin || freebsd || netbsd`; the file-name suffix `_darwin` would
  become a GOOS constraint and exclude freebsd and netbsd, so it is not used),
  `file_created_at_windows.go`, `file_created_at_other.go`
  (`//go:build !linux && !darwin && !freebsd && !netbsd && !windows`)

**Structure decision**: Writing the edit time is not a new role type; the three
roles that cause changes each call the package-internal `touchEditedAt` inside
their own transactions. Advancing only changed rows (R-3) requires knowing the
change in the same transaction as the write, and a separate role could not build
that transaction under the rule that a role type does not call another role's
public methods. Reading the creation time lives in `internal/scanner`, not in
`internal/mediafs`. `mediafs` owns the rules for what may be opened, and the
place that uses the scan's `stat` result is `scanner`.

## Implementation Work

### Advance a video's edit time on edits and carry it in video reads

**Scope**: `00027_video_edits.sql` ([data-model.md §1](data-model.md#1-migration)),
`Video.EditedAt` in `domain` ([§2](data-model.md#2-values-added-to-domain)),
`touchEditedAt` with its calls and change detection in the four kinds of writes
([§3](data-model.md#3-edit-time-rules)), the addition to `moveUserData`, the
`edited_at` column in reads that return videos ([§4](data-model.md#4-reads)), and
the invariant in `invariants_test.go`. The user-data list and the `TagStore`,
`VisibilityStore` and `OverrideStore` paragraphs in ARCHITECTURE.md.

**Dependencies**: None

**Acceptance**: `task check` passes. Store tests show:

- After `SetDisplayName`, `SetThumbnailPosition`, `SetVideosPublic`,
  `AttachTagByID`, `DetachTag` and `ApplyVideoTags`, `EditedAt` from `GetVideo`
  is the operation's time (acceptance criterion 1).
- It does not change for the same display name, the same position, an already
  attached tag, or an unchanged visibility, nor when one `SetDisplayNames` bulk
  writes A→B→A for the same video (edge case).
- Bulk tagging advances only the videos that changed (edge case).
- Saving playback position, `UpsertVideo`, probe results, renaming and deleting
  tags, and bundling do not change it (acceptance criterion 2, R-2).
- `EditedAt` of an unedited video equals `AddedAt` (acceptance criterion 3).
- Tagging a bundle member advances every member.
- Same-path succession gives the new content the previous `EditedAt` (edge case).
- After the migration `video_edits` is empty.

### Store each location's file creation time and use it in video reads and the "Date created" sort

**Scope**: `00028_video_file_created_at.sql`
([data-model.md §1](data-model.md#1-migration)); `FileCreatedAt` on `Video`,
`VideoFile`, `IndexedVideo` and `VideoLocation` in `domain`, and the two
`VideoSort` values ([§2](data-model.md#2-values-added-to-domain)); the
`file_created_at` column in reads that return videos and in `VideoLocations`
([§4](data-model.md#4-reads)); the list sort values (`listOrders`,
`itemOrderValues`, `libraryItemsCTE` and `VideoSort.Valid` for `createdAsc` and
`createdDesc`); `file_created_at` in `UpsertVideo`, `IndexedVideosByPath`, and the
new `UpdateLocationCreatedAt`
([§5](data-model.md#5-scan-and-location-writes); the call from the scan is the
next unit). The `ScanIndexStore` paragraph in ARCHITECTURE.md.

**Dependencies**: Advance a video's edit time on edits and carry it in video reads
(to add the migration numbers, the columns in reads that return videos, and
`scanVideo` in order)

**Acceptance**: `task check` passes. Store tests show:

- `FileCreatedAt` from `UpsertVideo` is stored on the location, and with the zero
  value `FileCreatedAt` equals `MTime` (acceptance criterion 5).
- `UpdateLocationCreatedAt` with the zero value returns the column to null, and
  `updated_at` and `version` do not change.
- `createdAsc` and `createdDesc` order `ListVideos`, `ListFolderVideos` and
  `ListLibrary` (groups by their members' maximum) by
  `coalesce(file_created_at, mtime)`, ties are settled by id, and the order of
  `modifiedAsc` and `modifiedDesc` is unchanged in `listing_sort_test.go`
  (acceptance criteria 6 and 8).
- After the migration, `file_created_at` of existing locations is null.

### Read file creation times during the scan and record them on locations

**Scope**: The per-OS `fileCreatedAt` in `internal/scanner`
([R-5](research.md#r-5-the-file-system-adapter-internalscanner-reads-the-creation-time-per-os-linux-uses-statx-from-golangorgxsysunix)),
making `golang.org/x/sys` a direct dependency in `go.mod`, setting
`VideoFile.FileCreatedAt`, and calling `UpdateLocationCreatedAt` for unchanged
files
([R-6](research.md#r-6-a-registered-location-is-rewritten-on-the-next-scan-when-its-creation-time-differs-even-if-the-file-is-unchanged),
[data-model.md §5](data-model.md#5-scan-and-location-writes)); the addition to the
`scanner.Index` interface and updates to the test fakes. The `internal/scanner`
paragraph in ARCHITECTURE.md. The checks in [quickstart.md](quickstart.md).

**Dependencies**: Store each location's file creation time and use it in video
reads and the "Date created" sort

**Acceptance**: `task check` passes (including `build-windows-check`). Scanner
tests show:

- The creation time reaches `UpsertVideo` for a new file (non-zero on an OS that
  has creation times).
- `UpdateLocationCreatedAt` is called for an unchanged file whose value differs
  from the index, and is not called when the seconds match (even if only the
  fraction differs).
- For a file whose index has a value but which can no longer be read, it is
  called with the zero value, and neither `UpsertVideo` nor job re-enqueueing
  happens (edge case "videos already registered").
- An unreadable creation time is not a failure.

The results of steps 1 to 4 in quickstart.md are recorded in the PR body
(acceptance criteria 4 and 5).

### Return the edit time and creation time in the screen API and accept the "Date created" sort

**Scope**: `Video.updatedAt`, `Video.fileCreatedAt` and the two `VideoSort` values
in `api/openapi.yaml`, with the generated files
([contracts/screen-api.md §0 and §1](contracts/screen-api.md#0-fields-added-to-video));
the conversion in `toAPIVideo`; `sort` validation (allowed for guests too);
`videoSorts` in `web/src/api` and updates to Vitest fixtures
([§3](contracts/screen-api.md#3-websrcapi-changes)). "thirteen sort orders" in
ARCHITECTURE.md.

**Dependencies**: Advance a video's edit time on edits and carry it in video
reads; Store each location's file creation time and use it in video reads and
the "Date created" sort

**Acceptance**: `task check` passes. httpapi tests show:

- `Video` in `GET /api/videos/{id}`, lists, related and versions carries
  `updatedAt` and `fileCreatedAt`; before any edit `updatedAt` equals `addedAt`,
  and the response of `PUT /api/videos/{id}/display-name` shows it advanced
  (acceptance criteria 1 and 3). Guest responses carry them too.
- `sort=createdDesc` and `createdAsc` are accepted by `GET /api/videos`,
  `GET /api/folders/{rootId}/videos` and `GET /api/library`, and do not return
  `400` for guests (requirement 5).
- `openapi_routes_test.go` and the generated-file checks pass.

### Include the edit time and creation time on videos in the external API

**Scope**: `ExternalVideo.updatedAt` and `fileCreatedAt` in
`api/external-v1.yaml` with the generated files, the conversion in
`internal/httpapi/external_videos.go`
([contracts/external-api.md](contracts/external-api.md)), and the description in
the section on listing videos (`動画の一覧を読む`) of
`docs/how-to/external-api.md`.

**Dependencies**: Advance a video's edit time on edits and carry it in video
reads; Store each location's file creation time and use it in video reads and
the "Date created" sort

**Acceptance**: `task check` passes. httpapi tests show that videos in
token-authenticated `GET /api/v1/videos` and `GET /api/v1/videos/lookup` carry
`updatedAt` and `fileCreatedAt` (acceptance criterion 7); after tagging through
`POST /api/v1/video-tags`, `updatedAt` in `lookup` has advanced, and attaching the
same tag again does not advance it. The output of MCP's `lookup_video` shows the
two fields.

### Show the edit time and creation time in the video page's facts row

**Scope**: Two facts in `web/src/player/VideoFacts.tsx` (alongside length, size
and date added; names, order, icons and format follow `ui-design.md`); the
success callback `onChanged` on `VideoTags` and `VisibilitySwitch` and the reload
in `VideoPage`
([R-8](research.md#r-8-the-video-page-reloads-the-video-after-tag-and-visibility-changes-and-no-new-domain-event-is-added));
English catalog text.

**Dependencies**: Return the edit time and creation time in the screen API and
accept the "Date created" sort

**Acceptance**: The screen changes (visual and interaction review required).
`task check` passes. Vitest tests show that the facts row shows the edit time and
creation time in the same format as the date added, and the three can be told
apart by accessible name (requirement 4, `UI品質`). Guests see them too. After
saving the display name, attaching or detaching a tag, toggling visibility, and
setting or clearing the thumbnail, the edit time on screen is the reloaded value
(acceptance criterion 1).

### Add "Date created" to the list sort orders

**Scope**: `sortKinds` in `web/src/videoList/listCriteria.ts` (`created`,
descending when chosen), the icon and the below-`md` grouping in
`SortControls.tsx`, `viewPreferences.ts`, the URL round trip in `listCriteria`,
and English catalog text (how "Date created" is told apart from the existing
"Date modified" (file) follows `ui-design.md`). The library and folder screens
share `SortControls` (requirement 5). The toolbar description in
`docs/design-docs/library-ui.md` notes the added sort kind.

**Dependencies**: Return the edit time and creation time in the screen API and
accept the "Date created" sort

**Acceptance**: The screen changes (visual and interaction review required).
`task check` passes. Vitest tests show that "Date created" appears in the sort
menu, choosing it fetches the list with `sort=createdDesc`, toggling the
direction gives `createdAsc`, and the choice round-trips through the URL and the
device setting (requirement 5, acceptance criterion 6). The existing "Date
modified" kind keeps its name and values (`modified*`) (requirement 7). Guest
menus show it too.
