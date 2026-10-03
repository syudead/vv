# Implementation Plan: Bundle versions of the same video (detected at scan time and by hand)

**Branch**: `feature/030-video-versions` | **Parent Issue**: #572

**Input**: The parent Issue. It is this feature's specification.

## Summary

Separate files of the same footage (a re-encode, a different resolution, a different container, or a file
whose content was rewritten in place) are bundled into one **bundle** as "versions of the same video", and
the library shows only the representative. A bundle has one set of tags, one playback position and one
visibility setting; changing the representative does not change them, and a version removed from the bundle
gets back the values it had before it was bundled. When a scan finds that the content at a path changed, the
previous video's values carry over if the durations match. Other videos whose footage looks the same are found
by fingerprint and offered as candidates.

- **Storage**: bundles live in the user-data tables `video_bundles` and `video_bundle_members`. A bundle's
  values sit in the existing tables `playback_progress`, `video_tags` and `public_videos` under the bundle's own
  key (`bundle:<id>`) ([research.md R-1](research.md), [data-model.md §1 and §2](data-model.md)).
- **Key resolution**: every place that reads or writes user data resolves the **user key** (the bundle's key
  for a bundled video, `content_key` otherwise) with one expression, and every read that returns a video puts
  it in `Video.UserKey` ([R-2](research.md), [data-model.md §3](data-model.md)).
- **Listing**: the library, search, folders and groups list only **shown videos** (unbundled videos and the
  effective representative of each bundle). The search expression applies to every location of the bundle;
  the scope applies to the representative's locations ([R-3](research.md), [R-4](research.md),
  [data-model.md §4](data-model.md)).
- **Carry-over**: `UpsertVideo` records a change of content at the same path as a succession candidate. Once
  the scan that recorded it closes and the new content's probe has finished, the durations are compared and
  the values carry over ([R-5](research.md), [data-model.md §5](data-model.md)).
- **Detection**: a new ingest stage `fingerprint` makes a fingerprint from the pHash of each seek-sprite frame,
  and the transaction that writes the fingerprint computes candidates and stores them in a table. A "different
  videos" decision is kept as user data ([R-6](research.md), [R-7](research.md),
  [data-model.md §6 and §7](data-model.md)).
- **API**: new routes list versions, bundle, change the representative, remove a version, list candidates and
  dismiss a candidate. A bundle is addressed by a video id ([R-8](research.md),
  [contracts/screen-api.md](contracts/screen-api.md)). A bundle change is mapped to `video` on `/api/events`
  through `domain.VideoBundleChanged` ([R-9](research.md)). The external API does not collapse versions; only
  the values become the bundle's ([R-10](research.md)).
- **Screens**: a versions entry point on the video page, a bundle action from the library selection, and a
  candidate list screen. The Issue has the `ui` label, so the next stage (design) decides the look and
  interaction in `ui-design.md` against the parent Issue's `UI品質`. Library cards do not change.

## Technical Context

**Canonical definitions**:

- Boundaries, dependency direction, the split between index and user data, where generated files live,
  domain events, the authentication boundary: [ARCHITECTURE.md](../../ARCHITECTURE.md),
  [.golangci.yml](../../.golangci.yml) (depguard)
- Content keys and ingest: [internal/scanner/content_key.go](../../internal/scanner/content_key.go),
  [internal/store/scan_index.go](../../internal/store/scan_index.go) (`UpsertVideo`),
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go) (`ApplyProbeForJob`),
  [internal/app/ingest.go](../../internal/app/ingest.go), [internal/jobs/worker.go](../../internal/jobs/worker.go),
  [internal/domain/job.go](../../internal/domain/job.go) (`JobKinds`, `ClaimConditionFor`)
- Listing, library items, folders, related videos: [internal/store/listing.go](../../internal/store/listing.go)
  (`chosenLocationsCTE`, `filteredFrom`), [internal/store/library_items.go](../../internal/store/library_items.go),
  [internal/store/search.go](../../internal/store/search.go), [internal/store/folders.go](../../internal/store/folders.go),
  [internal/store/related.go](../../internal/store/related.go),
  [specs/013-library-search/data-model.md](../013-library-search/data-model.md),
  [specs/017-folder-groups/data-model.md](../017-folder-groups/data-model.md),
  [specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md)
- User data tied to the content key: [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md),
  [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md),
  [specs/029-video-overrides/data-model.md](../029-video-overrides/data-model.md),
  [internal/store/visibility.go](../../internal/store/visibility.go),
  [internal/store/video_tags.go](../../internal/store/video_tags.go),
  [internal/store/progress.go](../../internal/store/progress.go)
- Seek sprites and generated files: [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md),
  [internal/domain/seek_sprite.go](../../internal/domain/seek_sprite.go),
  [internal/artifacts/store.go](../../internal/artifacts/store.go) (`SeekSprite`, `SeekSpriteSheet`)
- Ingest progress and issues: [specs/024-import-progress/research.md](../024-import-progress/research.md)
- Screen API and error shape: [api/openapi.yaml](../../api/openapi.yaml),
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md),
  [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md),
  [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)
- External API and MCP: [api/external-v1.yaml](../../api/external-v1.yaml),
  [specs/026-external-api/contracts/external-api.md](../026-external-api/contracts/external-api.md)
- Screens: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) ("Video facts"),
  [specs/017-folder-groups/ui-design.md](../017-folder-groups/ui-design.md) (selection bar),
  [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx),
  [web/src/library/SelectionBar.tsx](../../web/src/library/SelectionBar.tsx),
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md),
  [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- Earlier study: `docs/design-docs/video-identity.md` on branch `claude/video-detection-synonym-zgwmlw`
  (closed PR #492). Its option A (re-pointing to an alias key) is not adopted here (R-1).
- Generation and check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`,
  `task generate`)

**Feature-specific context**:

- One migration per unit that adds tables. `scripts/migrations-immutable.sh` forbids editing a migration that
  exists at the PR's base, so a migration that has landed on the feature branch is never changed and later
  units add a migration with a new number. `00023_video_versions.sql` adds three user-data tables
  (`video_bundles`, `video_bundle_members`, `video_version_dismissals`), `00024_video_successions.sql` adds
  `video_successions`, `00025_video_fingerprints.sql` adds `video_fingerprints` and `fingerprint` in
  `jobs.kind`, and `00026_video_version_candidates.sql` adds `video_version_candidates`
  ([data-model.md §1](data-model.md)). No column is added to `videos`, `video_locations` or the existing
  user-data tables.
- No Go or npm dependency is added. The fingerprint is built from the standard library's `image/jpeg` and a
  few dozen lines of DCT, and ffmpeg is not started even once more (R-6).
- The duration match tolerance is `max(1 s, 0.5% of the duration)` (`domain.DurationsMatch`). Carry-over
  (requirement 8) and candidate narrowing (requirement 9) use the same value.
- The fingerprint thresholds (minimum of 3 compared frames, an upper bound of 12 on the median Hamming
  distance, the luma variance below which a frame counts as flat) are constants in `internal/domain`, tied to
  `FingerprintVersion`. The values are set with re-encoded test videos; when they are tuned on real data, the
  version is bumped and the fingerprints are rebuilt (R-6).
- `domain.FolderIndexVersion` goes to 2. The index rule changes (locations of non-representative versions are
  left out), so the index is rebuilt at startup (R-4).
- `SearchKeyVersion` is not bumped. Search keys stay per location; only how they are applied changes (R-3).
- No `quickstart.md`. The acceptance criteria are verified by store, app, media (with ffmpeg), httpapi and
  Vitest tests, and there is no step that runs outside the repository's checks.
- The parent Issue does not say whether requirement 8's carry-over includes the display name
  (`video_overrides.display_name`). Carry-over covers the three values the parent Issue names (tags, playback
  position, visibility); the display name is put to the requester as a question in the Plan PR's body.

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"): pass.
  - `internal/domain`: `Video.UserKey`, `VideoVersions`, duration matching, fingerprints (`FrameHash`,
    `Fingerprint`, `CompareFingerprints`), `JobFingerprint` and its claim condition, `VideoBundleChanged`. Pure
    values and functions only (no `image`; JPEG decoding is in `internal/media`).
  - `internal/store`: migrations, `VersionStore`, key resolution, the shown-videos CTE, succession recording and
    carry-over, writing fingerprints and candidates, the SQLite function `vv_fingerprint_distance` (registered
    the same way as `vv_shuffle_key`).
  - `internal/media`: extracts each frame's 32×32 luma from the sprite sheets and turns it into a fingerprint
    (does not call ffmpeg).
  - `internal/app`: `Ingest.Fingerprint` (the job). Generated files are read through a declared interface.
  - `internal/httpapi`: parses requests and converts results to responses. `cmd/mdm`: wiring for the worker and
    subscriptions.
- **A role type does not call another role's public methods** (the `store.DB` paragraph of ARCHITECTURE.md):
  pass. The user-key expression, the shown-videos CTE, copying bundle values and rebuilding the folder index
  (`rebuildFolderIndex`) are shared as package-level functions.
- **Index versus user data** ("Rebuildable and user data"): pass. Bundles and dismissals are user data with no
  foreign key to `videos`; fingerprints, candidates and successions are index data, deleted when nothing
  references the content any more (data-model.md §1 and §6).
- **Reads shown to guests go through `visibleLocationCondition`**: pass. The shown-videos CTE takes the
  viewer's condition, and visibility is decided by the bundle's key (R-2, R-3).
- **Generated-file locations do not change** ("Generated files have one owner"): pass. The fingerprint only
  reads the sprite and stores no new generated file.
- **Domain events are published after commit, and subscriptions are registered in one place,
  `cmd/mdm/events.go`**: pass (R-9).
- **API source of truth and generated code** (AGENTS.md): pass. Edit `api/openapi.yaml` and run
  `task generate`. `api/external-v1.yaml` does not change (R-10).
- **Constraints are verified by tests** (core-beliefs.md): pass. The invariants (a bundle's representative is a
  member; no bundle has one member) are added to `invariants_test.go`, and the fingerprint thresholds are pinned
  by tests on videos re-encoded with ffmpeg.
- **Documentation changes in the same PR as the change** (core-beliefs.md): pass. Each unit updates its own part
  of ARCHITECTURE.md and `api/openapi.yaml`.

The verdict is the same after Phase 1. There is no violation for Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/030-video-versions/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-11
├── data-model.md         # Tables, domain values, key rule, shown videos, carry-over, fingerprints and candidates, store operations
└── contracts/
    └── screen-api.md     # Screen API delta (versions, bundling, candidates, Video, events)
```

The next stage (design) writes `ui-design.md` (`ui` label). There is no `quickstart.md` (see Technical
Context).

### Source Code

**Affected boundaries**:

- `internal/domain` (keys, versions, duration matching, fingerprints, job kinds, events), `internal/store`
  (migrations, `VersionStore`, read and write keys, CTEs, successions, fingerprints and candidates, the folder
  index), `internal/media` (sprite fingerprints), `internal/app` (`Ingest.Fingerprint`, activity kinds in
  `Scans`), `internal/jobs` (adding the stage is wiring in `cmd/mdm`)
- `internal/httpapi` (version, bundle and candidate routes, `Video.versions`, key resolution), `api/openapi.yaml`
  and generated code, `cmd/mdm` (worker, subscriptions)
- `web/src/api` (functions for the new routes), `web/src/player`, `web/src/library` (selection bar), the new
  candidate screen, `web/src/shell` (entry point), `web/src/i18n`
- `ARCHITECTURE.md`

**New paths**:

- `internal/store/migrations/00023_video_versions.sql`, `00024_video_successions.sql`,
  `00025_video_fingerprints.sql`, `00026_video_version_candidates.sql`, `internal/store/versions.go`
  (`VersionStore`), `internal/store/user_keys.go` (the key expression and the shown-videos CTE),
  `internal/store/successions.go`, `internal/store/fingerprints.go`
- `internal/domain/video_version.go`, `internal/domain/fingerprint.go`
- `internal/media/fingerprint.go`
- `internal/httpapi/video_versions.go`, `internal/httpapi/version_candidates.go`
- The versions components in `web/src/player/`, the bundle components in `web/src/library/`, and a directory for
  the candidate screen (names follow the design stage)

**Structure decision**: bundle reads and writes go in a new role, `VersionStore`. Bundling, removing and
changing the representative write user-data tables, rebuild the folder index in the same transaction and
publish an event after commit, so like `OverrideStore` the role holds `*DB` (R-1, R-4). Writing fingerprints and
candidates is the result of an ingest stage, so it goes in `IngestStore`; recording successions reflects a scan,
so it goes in `ScanIndexStore` (R-5, R-7). For computing the fingerprint, `internal/media` handles the image and
`internal/domain` owns the hash and comparison rules (R-6).

## Implementation Work

### Store bundles and resolve user-data keys

**Scope**: the user-data tables of `00023_video_versions.sql` (`video_bundles`, `video_bundle_members`,
`video_version_dismissals`) and the `FolderIndexVersion` bump; `domain.Video.UserKey`, `VideoVersions`,
`VideoBundleChanged`; `VersionStore`'s `Bundle`, `MakeRepresentative`, `Unbundle` and `Versions`
([data-model.md §1, §2 and §8](data-model.md)). The user-key expression and every read and write path that
uses it (`Video.UserKey`, `publicVideoCondition`, tag filtering and matching, watch state, `SaveProgress`,
adding and removing tags, tag summaries and counts, toggling visibility and cutting off guests, the external
API's list and lookup; [§3](data-model.md)). The screen subscription in `cmd/mdm/events.go`. The user-data list
and the `store.DB` role list in ARCHITECTURE.md. Collapsing the listing is not done yet (next unit).

**Dependencies**: None

**Acceptance**: `task check` passes. In a store test, bundling A (tag X) and B (tag Y) with A as representative
gives A and B the same bundle key as `UserKey`, that key has only tag X, and tag Y stays on the row for B's
`content_key` (the storage part of acceptance criterion 6). Changing the representative to B leaves the bundle
key and tags unchanged (the storage part of acceptance criterion 8). Removing B returns B's `UserKey` to its
`content_key`, tag Y is found again, and the bundle's tags stay X (acceptance criterion 9). When one member is
left, the bundle row is deleted and the bundle's values are copied to the remaining video's `content_key`.
Bundling bundles together gives one bundle whose values are those of the chosen representative's bundle; the
absorbed bundle's row is deleted (the values under its key remain). Writing `SaveProgress` to B's `UserKey`
advances A's `progress` (the storage part of acceptance criterion 7). `SetVideosPublic` writes to the bundle
key, and the keys it returns include every member's `content_key`. The invariants (the representative is a
member; no bundle has fewer than 2 members; no `user_key` does not start with `bundle:`) pass in
`invariants_test.go`. `VideoBundleChanged` is published once after commit.

### Collapse each bundle to its representative in the library, search, folders and groups

**Scope**: the shown-videos CTE (including the effective-representative rule) and every read that goes through
it: `chosenLocationsCTE` (applies the search expression to every location of the bundle), `libraryItemsCTE`
and `LibraryIDs`, videos directly in a folder and folder counts (`folders.go`), `DirectVideoPaths` and
`VideosAddedNear`, the folder index input (`folderIndexLocations`) and its rebuild on bundle operations, tag
counts ([data-model.md §4](data-model.md)). In `api/openapi.yaml`, the descriptions of `listLibrary`,
`listVideos`, `listFolderVideos` and folders, and the generated code; the listing paragraph of ARCHITECTURE.md.

**Dependencies**: Store bundles and resolve user-data keys

**Acceptance**: `task check` passes. In a store test, after bundling with A as representative, `ListLibrary`
and `ListVideos` return only A with `total` 1, and filtering by tag Y does not return it (acceptance criterion
6). Searching for a word found only in B's title returns A (acceptance criterion 10). When A and B are in
different folders, `ListFolderVideos` for B's folder does not return B, and A's folder returns A. A folder
holding only B counts as a folder with no videos, and B joins no folder group (acceptance criterion 11).
Changing the representative to B makes the single item's title and thumbnail B's (acceptance criterion 8). When
the representative's location disappears, a remaining version appears as the item; when all are gone, nothing
appears. A guest (bundle made public) sees A (acceptance criterion 12). `GET /api/videos/{id}` also returns
non-representative versions.

### Screen API for versions and bundling

**Scope**: `GET /api/videos/{id}/versions`, `POST /api/video-bundles`,
`POST /api/videos/{id}/make-representative`, `POST /api/videos/{id}/unbundle`, `Video.versions` on the detail
response, new `ErrorReason` values, `api/openapi.yaml` and generated code, the functions in
`web/src/api/client.ts` and the messages in `web/src/i18n/errors.ts`
([contracts/screen-api.md §0 to §4](contracts/screen-api.md)). Tests in `accessRoutes` (`GET` on `versions` is
open to guests; the rest are owner-only). The API paragraph of ARCHITECTURE.md.

**Dependencies**: Collapse each bundle to its representative in the library, search, folders and groups

**Acceptance**: `task check` passes. In an httpapi test, the owner's `POST /api/video-bundles` returns `200` with
`VideoVersions` (representative first; each item has resolution, codec, size and `location`),
`GET /api/videos/{id}/versions` returns the same order, and `versions.count` in `GET /api/videos/{id}` is 2.
After `make-representative`, the single item in `GET /api/library` is B and `tags` stays X (acceptance criterion
8). After `unbundle`, B is back in the list with tag Y (acceptance criterion 9). `progress` in B's
`GET /api/videos/{id}` is the bundle's, and `PUT /api/videos/{id}/progress` advances A's watch state in the
list (acceptance criterion 7). A guest's `GET /api/videos/{id}/versions` returns every version of a public
bundle, and `POST` returns `401` (acceptance criterion 12). Operations with a single video, without the
representative among the videos, or on an unbundled video return the contract's `400`. `video` is sent on
`/api/events` for every member.

### Carry over values when the content at a path changes during a scan

**Scope**: `video_successions` from `00024_video_successions.sql` ([data-model.md §1 and §5](data-model.md));
recording a succession in `UpsertVideo` and cancelling it (when the previous content appears at another path);
comparing durations and carrying over in `FinishScan` (`done`) and in `ApplyProbe` and `ApplyProbeForJob` (for
a bundle member, the member's place carries over; otherwise the rows of the three tables are re-keyed);
deleting the rows when nothing references the content any more; `domain.DurationsMatch`. The scan paragraph of
ARCHITECTURE.md.

**Dependencies**: Store bundles and resolve user-data keys

**Acceptance**: `task check` passes. In a store test, upserting a file with the same duration and a different
`content_key` at the path of video A (which has tags and a playback position) and writing its probe gives the
video at that path the tags and the playback position (acceptance criterion 1). If the duration is outside the
tolerance, nothing carries over (acceptance criterion 2). If the previous content was a bundle's
representative, the new content becomes the representative; if it was another member, the new content becomes
a member of the same bundle (Edge Case). If the previous duration is null, nothing is recorded. If the previous
content appears at another path in the same scan, the record is deleted and nothing carries over (Edge Case).
If the new content's probe finishes in the middle of the scan, nothing carries over until the scan closes with
`done`, and nothing carries over either when the previous content is found at another path afterwards. If the
probe fails, the record stays and is decided when a retry succeeds. On carry-over, `VideoBundleChanged` is
published for every member of the bundle the previous content belonged to.

### Ingest stage that fingerprints footage from seek sprites

**Scope**: `domain.JobFingerprint` (last in `JobKinds`; its claim condition is a finished seek thumbnail);
`domain.Fingerprint`, `FrameHash`, `CompareFingerprints`, the thresholds and `FingerprintVersion`
([data-model.md §6](data-model.md)); `media.SpriteFingerprint` (each frame's 32×32 luma from the sheet JPEGs);
`app.Ingest.Fingerprint`; `IngestStore.ApplyFingerprintForJob` (computing candidates is the next unit); the
migration of `video_fingerprints` and `jobs.kind` in `00025_video_fingerprints.sql` (it queues the job for
videos with a finished sprite; [data-model.md §1](data-model.md)); queuing the job in the transaction that
finishes the seek thumbnail; re-queuing missing fingerprints during a scan
(`IndexedVideo.FingerprintMissing` and `EnsureJob`); wiring for the worker and wake-ups; the `ScanActivity`
kind and its screen text. The ingest paragraph of ARCHITECTURE.md.

**Dependencies**: None

**Acceptance**: `task check` passes. In a domain test, the Hamming distance between an image and the same image
with changed brightness and contrast is within the threshold, and a different image is outside it. In a media
test (with ffmpeg), the sprite fingerprints of two re-encodes of the same video at different resolutions match
under `CompareFingerprints` (two videos longer than 405 seconds whose intervals differ also match, because
frames are aligned by time), and a different video with only the same duration does not match (the matching
part of acceptance criteria 3 and 4). In an app test, the job reads the sprite and writes the fingerprint, and
returns a failure when there is no sprite. In a store test, finishing a seek thumbnail queues a `fingerprint`
job, claiming waits for the seek thumbnail to finish, and the job counts as remaining work of the scan. A
`fingerprint` job that failed up to the limit is queued again by the next scan.

### Compute candidates, and the API to list and dismiss them

**Scope**: `video_version_candidates` from `00026_video_version_candidates.sql`
([data-model.md §1](data-model.md)), `vv_fingerprint_distance`, computing candidates in the transaction that
writes the fingerprint ([data-model.md §7](data-model.md)), deleting candidates on bundle operations and when
nothing references the content any more, `VersionStore.Candidates` and `Dismiss`,
`GET /api/version-candidates` and `POST /api/version-candidates/dismiss`
([contracts/screen-api.md §5](contracts/screen-api.md)), `api/openapi.yaml` and generated code, the functions in
`web/src/api/client.ts`. The ARCHITECTURE.md paragraph.

**Dependencies**: Store bundles and resolve user-data keys; Ingest stage that fingerprints footage from seek
sprites

**Acceptance**: `task check` passes. In a store test, writing two videos whose durations are within the
tolerance and whose fingerprints match creates one candidate pair (acceptance criterion 3); two videos with only
the same duration and different fingerprints do not (acceptance criterion 4). A dismissed pair does not become a
candidate again even when its fingerprints are rewritten (acceptance criterion 5). Two videos in the same bundle
are not a candidate, and bundling removes the candidate. When one video's row is deleted, the candidate is
removed (Edge Case). In an httpapi test, the owner's `GET /api/version-candidates` returns the pair with both
`Video`s; bundling with `POST /api/video-bundles` removes it; `dismiss` removes it and it does not come back
after a rescan. A guest gets `401`.

### Show other versions on the video page, and play, change the representative or remove them

**Scope**: the versions entry point in the video page's facts line (one line with the count) and, when opened,
the list of versions (resolution, codec, size, location); choosing one to play it (moves to that `id`'s video
page); making it the representative; removing it. When the bundle's playback position is at or past that
version's duration, playback starts from the beginning ([R-11](research.md)). Replacing the `useVideoDetail`
response and refetching on the `video` event. The look and interaction follow `ui-design.md`. Guests get only
the version switch for playback.

**Dependencies**: Screen API for versions and bundling

**Acceptance**: this unit changes a screen (visual and interaction review required). `task check` passes. In a
Vitest test, the page of a bundled video shows one line with the count; opening it lists how the versions
differ; choosing B moves to B's page and plays B's file (acceptance criterion 7). Making B the representative
makes the single item in the library B (acceptance criterion 8), and removing B returns it to a separate item
(acceptance criterion 9). Guests have no representative change or remove action, and can switch playback
(acceptance criterion 12). When the bundle's playback position is at or past the version's duration, playback
starts at 0. The hierarchy of title, player and tags is kept, and no always-open large panel or tab is added
(`UI品質`).

### Bundle several videos as versions from the library selection

**Scope**: the selection bar's bundle action, the step that picks the representative, sending
`POST /api/video-bundles`, refetching the list and clearing the selection on success, and showing failures.
Pairs that never become candidates (a theatrical cut and a TV cut, for example) can also be bundled this way.
The look and interaction follow `ui-design.md`.

**Dependencies**: Screen API for versions and bundling

**Acceptance**: this unit changes a screen (visual and interaction review required). `task check` passes. In a
Vitest test, selecting two or more videos shows the bundle action, and picking a representative and sending
leaves only the representative in the list (requirement 11, acceptance criterion 6). Selecting one video does
not show it. A selected group is expanded to its members before sending. Library cards look the same
(`UI品質`).

### Candidate list screen for possible duplicates

**Scope**: an owner-only screen listing candidates (its location and entry point are in `ui-design.md`). It shows
the two videos of each pair so their differences are visible, and puts "same video" (pick a representative and
`POST /api/video-bundles`) and "different videos" (`dismiss`) first. It refetches on the `scan` event. It copes
with candidates being added during ingest and with a candidate disappearing when one of its videos goes away.

**Dependencies**: Compute candidates, and the API to list and dismiss them

**Acceptance**: this unit changes a screen (visual and interaction review required). `task check` passes. In a
Vitest test, candidate pairs are listed with the two videos' differences (resolution, codec, size, location);
"same video" with a chosen representative removes the pair and leaves one item in the library (acceptance
criteria 3 and 6); "different videos" removes the pair (acceptance criterion 5). Guests have no entry point
(requirement 12).
