# Implementation Plan: Let the owner change a video's display name and representative thumbnail

**Branch**: `feature/029-video-overrides` | **Parent Issue**: #517

**Input**: The parent Issue. It is this feature's specification.

## Summary

The owner can override, per video, the display name and the frame used as the
representative thumbnail, from the screen (the video page) as well as from the
external API and MCP. The original file is not touched.

| Concern | Approach |
| --- | --- |
| Storage | An override is one row of `video_overrides`, a user-data table keyed by `content_key`, and survives rescans, moves, renames and added locations ([research.md R-1](research.md#r-1-overrides-live-in-one-user-data-table-keyed-by-content_key), [data-model.md §1](data-model.md#1-video_overrides)). |
| Title | The store decides the effective title (the display name if there is one, otherwise from the file name) and puts it in `Video.Title`, carrying `FileTitle` and `DisplayName` separately. Sorting and search fold the display name into each location's `title_key` and `search_key` ([R-2](research.md#r-2-the-store-decides-the-effective-title-domainvideo-carries-both-the-display-name-and-the-file-name-title), [R-3](research.md#r-3-sorting-and-search-fold-the-display-name-into-each-locations-title_key-and-search_key)). |
| Thumbnail | Setting and clearing a position generate the image within the request before recording, and a failure keeps the previous image and position. The import `thumbnail` job also generates at the chosen position. The store layout does not change, and the `thumbnailUrl` version includes a revision number per recorded position ([R-4](research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded), [R-5](research.md#r-5-the-import-thumbnail-job-also-uses-the-chosen-position-the-automatic-rule-stays-in-internalmedia), [R-6](research.md#r-6-the-thumbnailurl-version-includes-the-position-revision)). |
| Notification | Override changes are mapped through `domain.VideoOverrideChanged` to `video` on `/api/events` ([R-7](research.md#r-7-override-changes-publish-domainvideooverridechanged-mapped-to-the-screens-video-notification)). |
| API | The screen gets two `PUT`s per video ([contracts/screen-api.md](contracts/screen-api.md)); the external API gets two bulk operations and three `ExternalVideo` fields; MCP gets the same two tools ([contracts/external-api.md](contracts/external-api.md), [R-8](research.md#r-8-the-screen-api-is-two-puts-per-video-an-empty-display-name-and-a-null-position-clear), [R-9](research.md#r-9-the-external-api-adds-two-bulk-operations-and-mcp-the-same-two-tools)). |
| Rules | The display name is trimmed like tag names, limited to 200 code points, and empty clears it. Pure functions in `internal/domain` own position validation and the split of errors ([R-10](research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit), [R-11](research.md#r-11-position-validation-is-a-pure-function-in-internaldomain-with-three-distinct-errors)). |
| Screen | Title editing on the video page, display of the original file name, and the player's "use the current frame as the thumbnail". With the `ui` label, the next stage, design, decides the look and interaction in `ui-design.md` against the parent Issue's `UI品質` section. List cards only show `title` and do not change. |

## Technical Context

**Canonical definitions**:

| Topic | Source |
| --- | --- |
| Boundaries, dependency direction, index versus user data, where artifacts live, domain events, the authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md), [.golangci.yml](../../.golangci.yml) (depguard) |
| Video reads and where the title comes from | [internal/store/videos.go](../../internal/store/videos.go) (`videoColumnsTemplate`), [internal/store/listing.go](../../internal/store/listing.go) (`listColumns`), [internal/store/library_items.go](../../internal/store/library_items.go), [internal/store/external_videos.go](../../internal/store/external_videos.go) |
| Search and sort keys | [specs/013-library-search/data-model.md](../013-library-search/data-model.md), [internal/store/search_keys.go](../../internal/store/search_keys.go), [internal/domain/search.go](../../internal/domain/search.go) (`NaturalSortKey`, `FoldForMatch`) |
| Precedents for user data keyed by content key | [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md), [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md), [internal/store/visibility.go](../../internal/store/visibility.go) |
| Thumbnail generation and publishing | [internal/media/thumbnail.go](../../internal/media/thumbnail.go), [internal/app/ingest.go](../../internal/app/ingest.go) (`Thumbnail`), [internal/app/artifacts.go](../../internal/app/artifacts.go) (the generation lock), [internal/artifacts/store.go](../../internal/artifacts/store.go) (`PublishThumbnail`), [internal/store/ingest_results.go](../../internal/store/ingest_results.go) (`setStageStateForJob`), [specs/024-import-progress/research.md](../024-import-progress/research.md) R-7 (substitution records) |
| The rule for which locations may be opened | [internal/httpapi/stream.go](../../internal/httpapi/stream.go) (`openMediaFile`), [internal/mediafs](../../internal/mediafs/media_file.go) |
| The screen API and the error shape | [api/openapi.yaml](../../api/openapi.yaml), [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md), [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| The external API and MCP | [api/external-v1.yaml](../../api/external-v1.yaml), [specs/026-external-api/contracts/external-api.md](../026-external-api/contracts/external-api.md), [specs/026-external-api/contracts/mcp.md](../026-external-api/contracts/mcp.md), [internal/httpapi/mcp.go](../../internal/httpapi/mcp.go), [docs/how-to/external-api.md](../../docs/how-to/external-api.md) |
| Screens | [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md), [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx), [web/src/player/VideoFacts.tsx](../../web/src/player/VideoFacts.tsx), [web/src/api/useVideoDetail.ts](../../web/src/api/useVideoDetail.ts), [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md), [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md) |
| Generation and check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`) |

**Feature-specific context**:

- One migration (`video_overrides`, `00021`). No column is added to `videos` or
  `video_locations`.
- No Go or npm dependency is added. `ffmpeg` is used with the same arguments as
  today's one-frame extraction; only the position is specified.
- `SearchKeyVersion` is not raised. Locations without a display name keep the
  same key value as today (data-model.md §4).
- No `quickstart.md` is created. The acceptance criteria are checked by store,
  app, httpapi and Vitest tests, and there are no steps to run outside the
  repository's checks.

## Constitution Check

| Gate | Verdict |
| --- | --- |
| Dependency direction (ARCHITECTURE.md "Intended dependency direction") | Pass. See the package table below. |
| A role type does not call another role's public methods (the `store.DB` paragraph in ARCHITECTURE.md) | Pass. Resolving content keys (`registeredContentKeysForVideoIDs`), rewriting keys (`writeSearchKeys`) and the substitution row (the `applySubstitution` table) are shared as package-level functions. |
| Index versus user data | Pass. `video_overrides` is added to the list of user data, and artifact cleanup does not touch it (data-model.md §1). |
| The artifact store layout does not change (ARCHITECTURE.md "Generated files have one owner") | Pass. Chosen images also go in `<p>/<s>.jpg` (R-5). |
| Domain events are published after commit, and subscriptions are registered in one place, `cmd/mdm/events.go` | Pass (R-7). |
| API sources of truth and generated files (AGENTS.md) | Pass. `api/openapi.yaml` and `api/external-v1.yaml` change, then `task generate`. The external API only adds fields and operations (compatibility policy). |
| Guests do not see the owner's data (guest-api.md) | Pass. `fileTitle`, `displayName` and `thumbnailPositionMs` are only in owner responses, and writes get `401` through the boundary's default category (requirement 9). |
| Server output is English and screen strings come from the catalog (gosmopolitan in `.golangci.yml`, i18n.md) | Pass. |
| Documents are fixed in the same PR as the change (core-beliefs.md) | Pass. Each unit fixes its own part of ARCHITECTURE.md, `docs/how-to/external-api.md` and the descriptions in `api/*.yaml`. |

How the dependency direction holds, per package:

| Package | Role in this feature |
| --- | --- |
| `internal/domain` | Display name trimming, position validation, error values, `Video` fields, `VideoOverrideChanged`. Pure values and functions only. |
| `internal/store` | Migration, `OverrideStore`, the left join in reads, key rewriting, publishing after commit. |
| `internal/media` | `ThumbnailAt` (only calls ffmpeg). |
| `internal/app` | `Ingest.SetThumbnailPosition` (generate then record inside the generation lock) and the job's switch to the chosen position. Generation, storage and artifacts go through declared interfaces. |
| `internal/httpapi` | Parsing requests, resolving locations (the same rule as `openMediaFile`), converting to responses, the external API and MCP. Rules live in domain, store and app. |
| `cmd/mdm` | Wiring and registering the `VideoOverrideChanged` subscription. |

The verdicts are the same after Phase 1. There is no violation for Complexity
Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/029-video-overrides/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1–R-11
├── data-model.md         # video_overrides, domain values, store operations, key rules
└── contracts/
    ├── screen-api.md     # The two PUTs of the screen API and the Video changes
    └── external-api.md   # The two /api/v1 operations, the ExternalVideo changes, the two MCP tools
```

The next stage, design, creates `ui-design.md` (`ui` label). No
`quickstart.md` is created (see Technical Context).

### Source Code

**Affected boundaries**:

| Boundary | What changes |
| --- | --- |
| `internal/domain` | Display name and position rules, `Video` fields, the event |
| `internal/store` | Migration, `OverrideStore`, reads, keys |
| `internal/media` | `ThumbnailAt` |
| `internal/app` | `Ingest` |
| `internal/httpapi` | The two screen routes, `toAPIVideo`, the two external operations, `toExternalVideo`, MCP |
| `api/openapi.yaml`, `api/external-v1.yaml` and generated files | Schemas |
| `cmd/mdm` | Wiring and subscription |
| `web/src/api` | `setVideoDisplayName`, `setVideoThumbnailPosition` |
| `web/src/player`, `web/src/i18n` | Screen and strings |
| `ARCHITECTURE.md`, `docs/how-to/external-api.md` | Documentation |

**New paths**:

- `internal/store/migrations/00021_video_overrides.sql`,
  `internal/store/overrides.go`, `internal/domain/video_override.go`
- `internal/httpapi/video_overrides.go`,
  `internal/httpapi/external_video_overrides.go`
- The editing components in `web/src/player/` (names follow the design stage)

**Structure decision**: The use case for setting a position goes in
`app.Ingest`. `Ingest` owns the lock that serializes creating and deleting
artifacts, and both in-request generation and job generation have to go
through that lock (R-4). Saving a display name involves no generation, so like
`VisibilityStore` it completes within a store role alone, and
`internal/httpapi` calls it directly.

## Implementation Work

### Store the display name and reflect it in the title, sorting and search

**Scope**: `00021_video_overrides.sql`, `domain.NormalizeDisplayName` and the
error values, `Video.FileTitle`, `DisplayName` and `ThumbnailPositionMs`, and
`VideoOverrideChanged`
([data-model.md §1 and §2](data-model.md#1-video_overrides)).
`OverrideStore.SetDisplayName` and `SetDisplayNames`, the left join in every
read that returns videos (detail, list, library items, related, external API,
`IngestStore.GetVideo`), and the four key-rewriting paths
([§3 and §4](data-model.md#3-store-operations)). The screen subscription in
`cmd/mdm/events.go`. The user data list and the `store.DB` role list in
ARCHITECTURE.md.

**Dependencies**: None

**Acceptance**: `task check` passes. In store tests, a video with a display
name has `Title` equal to the display name and `FileTitle` equal to the
location's title, sorts at the display name's position under `titleAsc`, and
is found by search on both the display name and the file name (acceptance
criterion 4). Upserting the location to another path (a rescan after a rename
or move) keeps the display name and keys (acceptance criterion 3). An empty or
whitespace-only name deletes the row, and control characters or 201 code
points return `*InvalidDisplayNameError` and write nothing. The second location
of the same content returns the same `Title`. Re-registering a media folder
that contains a video with a display name through `AddMediaFolder` or
`ReplaceMediaFolder` leaves `title_key` and `search_key` containing the display
name. `VideoOverrideChanged` is published once after commit.

### Set and clear the display name through the screen API

**Scope**: `PUT /api/videos/{id}/display-name`, the three `Video` fields and
their omission for guests, the two `ErrorReason` values, the `title`
description in `api/openapi.yaml` and generated files, the function in
`web/src/api/client.ts` and the strings in `web/src/i18n/errors.ts`
([contracts/screen-api.md §0, §1 and §3](contracts/screen-api.md#0-video-changes)).
`accessRoutes` tests (owner only).

**Dependencies**: Store the display name and reflect it in the title, sorting and search

**Acceptance**: `task check` passes. In httpapi tests, the owner's `PUT`
returns `200` and the updated `Video`, and the `title` of
`GET /api/videos/{id}`, `GET /api/library`, `GET /api/videos/{id}/related` and
the guest `GET /api/videos/{id}` is the display name (acceptance criterion 1).
A `PUT` with an empty string restores the original title (acceptance
criterion 2). Guest responses have no `fileTitle` or `displayName`, and a guest
`PUT` gets `401` (acceptance criterion 9). Control characters and a name that
is too long get `400` with their `reason` and `limit: 200`. `video` is sent on
`/api/events`.

### Regenerate and record the representative thumbnail at a chosen position

**Scope**: `media.ThumbnailAt`
([data-model.md §5](data-model.md#5-generated-artifacts)),
`domain.CheckThumbnailPosition` and the error values,
`OverrideStore.SetThumbnailPosition` ([§3](data-model.md#3-store-operations)),
`app.Ingest.SetThumbnailPosition` (R-4) and the job's switch to the chosen
position (R-5), and the `thumbnailURL` version (R-6).

**Dependencies**: Store the display name and reflect it in the title, sorting and search

**Acceptance**: `task check` passes. In app tests, `SetThumbnailPosition` calls
`PublishThumbnail` then `SetThumbnailPosition` inside the generation lock, and
on a generation failure does not call the store and returns
`ErrThumbnailFrameUnavailable`. Before analysis it returns
`ErrDurationUnknown`, and at or past the duration
`ErrThumbnailPositionOutOfRange`. `Thumbnail` (the job) calls `ThumbnailAt` for
a video with a position and `Thumbnail` for one without. When the job reads the
video outside the lock and `SetThumbnailPosition` records a position and `done`
while the job waits for the lock, the job rereads inside the lock and skips
generation, and the image of the recorded position remains. In store tests,
the recording transaction sets `thumbnail_state` to `done` and deletes
`thumbnail_first_frame`, and `nil` makes the row's columns null (no row when
both are null). In `media` tests (with ffmpeg), `ThumbnailAt` writes the image
at the given second.

### Set and clear the representative thumbnail position through the screen API

**Scope**: `PUT /api/videos/{id}/thumbnail-position`, location resolution (the
same rule as `openMediaFile`), the three `ErrorReason` values,
`api/openapi.yaml` and generated files, the function in
`web/src/api/client.ts` and the strings
([contracts/screen-api.md §2 and §3](contracts/screen-api.md#2-put-apivideosidthumbnail-position)).

**Dependencies**: Regenerate and record the representative thumbnail at a chosen position, Set and clear the display name through the screen API

**Acceptance**: `task check` passes. In httpapi tests, the owner's `PUT`
returns `200` and a `Video` with `thumbnailPositionMs`, the `thumbnailUrl`
version differs from before, and the same video in `GET /api/library` has the
same URL (acceptance criterion 5). Choosing the same position again still
changes the version, and the guest `thumbnailUrl` does not contain the position
value. `null` removes the position and the URL returns to the original
(acceptance criterion 6). Before analysis `409 duration_unknown`; at or past
the duration `400 thumbnail_position_out_of_range` with `limit`; on a
generation failure `409 thumbnail_frame_unavailable` with the previous
`thumbnailUrl` unchanged. Guests get `401`.

### Set and clear display names and thumbnail positions in bulk through the external API and MCP

**Scope**: `POST /api/v1/video-display-names` and
`POST /api/v1/video-thumbnails`, the three `ExternalVideo` fields, `code` and
`reason` in `api/external-v1.yaml` and generated files, and the two MCP tools
([contracts/external-api.md](contracts/external-api.md)). Usage in
`docs/how-to/external-api.md` (an example of a scraper tidying titles, the
20-item limit for thumbnails, handling `index` on a partway failure), the MCP
paragraph in ARCHITECTURE.md (8 tools), and the table in 026's
`contracts/mcp.md`.

**Dependencies**: Set and clear the representative thumbnail position through the screen API

**Acceptance**: `task check` passes. In httpapi tests, a
`POST /api/v1/video-display-names` with a token returns `200`, and the result
shows in the `title` of the screen's `GET /api/videos/{id}` and in the
`title`, `displayName` and `fileTitle` of `GET /api/v1/videos/lookup`
(acceptance criterion 8). `null` restores it. A request containing a video that
cannot be found gets `404` with `index` and applies nothing.
`POST /api/v1/video-thumbnails` returns `200` and the screen's `thumbnailUrl`
changes. 21 items get `400 too_many_videos`; a generation failure partway gets
`409 thumbnail_frame_unavailable` with `index`, and only the earlier items are
applied. An MCP client lists 8 tools, and the result of
`update_video_display_names` shows in `get_video`. Every operation is
classified as Bearer through `bearerAuth`.

### Edit the display name on the video page and show the original file name

**Scope**: Title editing on the video page (save, clear, error display) and
display of the original file name when a display name is set. Replacing the
`useVideoDetail` response, and strings in the English catalog. The look and
interaction follow `ui-design.md`. Guests get no entry point for editing.

**Dependencies**: Set and clear the display name through the screen API

**Acceptance**: This unit changes a screen (visual and interaction review
required). `task check` passes. In Vitest tests, when the owner saves a display
name on the video page without navigating, the title is replaced, the original
file name is visible as secondary, and clearing restores it (acceptance
criteria 1, 2 and 7). The guest video page has no entry point for editing
(acceptance criterion 9). A name that is too long shows the reason (edge
case).

### Use the player's current frame as the representative thumbnail

**Scope**: An entry point that sends the player's current playback position
(playing or paused) to `PUT /api/videos/{id}/thumbnail-position` in one action,
clearing, and display of waiting and failure. Not shown to guests. The look and
interaction follow `ui-design.md`.

**Dependencies**: Set and clear the representative thumbnail position through the screen API

**Acceptance**: This unit changes a screen (visual and interaction review
required). `task check` passes. In Vitest tests, the paused position is sent in
milliseconds, the screen updates from the response `Video`, and a failure shows
the reason and keeps the previous state (acceptance criteria 5 and 6, edge
case). There is no entry point that asks for a time as a number (`UI品質`).
