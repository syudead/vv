# Data model: Versions of the same video

Parent Issue: #572.

The rest of the model is unchanged. The existing tables are defined by
[internal/store/migrations/](../../internal/store/migrations/); the data categories are in
[running-vv.md, Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery); precedents for tying user data to the
content key are [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md) and
[specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md); the folder index is in
[specs/017-folder-groups/data-model.md](../017-folder-groups/data-model.md); library items are in
[specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md).
This file holds only the tables and values this feature adds and the rules that read and write them. Tables not
listed here do not change (no column is added to `videos`, `video_locations`, `playback_progress`,
`video_tags`, `public_videos` or `video_overrides`).

## 1. Migration

The last migration on `main` is `00021_video_overrides.sql`. `scripts/migrations-immutable.sh` forbids editing a
migration that exists at the PR's base, so each unit that adds tables adds one migration with a new number, and a
migration that has landed on the feature branch is never changed.

| Migration | Adds | Unit |
| --- | --- | --- |
| `00023_video_versions.sql` | `video_bundles`, `video_bundle_members`, `video_version_dismissals` | Store bundles and resolve user-data keys |
| `00024_video_successions.sql` | `video_successions` | Carry over values when the content at a path changes during a scan |
| `00025_video_fingerprints.sql` | `video_fingerprints`, `fingerprint` in `jobs.kind`, and queuing the job | Ingest stage that fingerprints footage from seek sprites |
| `00026_video_version_candidates.sql` | `video_version_candidates` | Compute candidates, and the API to list and dismiss them |

Each migration's Down reverts only what that migration added. The table definitions follow (separated per
migration by a header comment).

```sql
-- 00023_video_versions.sql
-- Bundles of versions of the same video (specs/030-video-versions/research.md R-1). User data that
-- cannot be rebuilt; no foreign key to videos. The bundle's tags, playback position and visibility
-- sit in video_tags, playback_progress and public_videos keyed by user_key.
create table video_bundles (
    id                 integer primary key autoincrement,
    -- Key of the bundle's values. The writer builds 'bundle:' || id. Never collides with a content_key (<hex>:<size>).
    user_key           text    not null unique,
    -- content_key of the representative version. It is one of the members (invariant).
    representative_key text    not null,
    created_at         integer not null,
    updated_at         integer not null
);

create table video_bundle_members (
    content_key text    primary key,
    bundle_id   integer not null references video_bundles (id) on delete cascade,
    added_at    integer not null
) without rowid;
create index video_bundle_members_bundle_idx on video_bundle_members (bundle_id, content_key);

-- Pairs judged to be "different videos" (requirement 10). User data. One row with key_a < key_b.
create table video_version_dismissals (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- The tables below are index data. They are deleted when nothing references the content any more,
-- and scanning and ingest can rebuild them.

-- 00024_video_successions.sql
-- Succession candidates where the content at a path changed (R-5). Decided and deleted when the
-- recording scan has closed and the new content's probe has finished.
create table video_successions (
    new_key         text    primary key,
    old_key         text    not null,
    old_duration_ms integer not null,
    -- 1 once a scan closes with done after the record. While 0 the previous content may still appear at another path, so no decision is made.
    ready           integer not null default 0 check (ready in (0, 1)),
    created_at      integer not null
) without rowid;

-- 00025_video_fingerprints.sql
-- Footage fingerprint (R-6). hashes holds 9 bytes per frame (1 flag byte + 8-byte big-endian hash).
create table video_fingerprints (
    content_key text    primary key,
    version     integer not null,   -- domain.FingerprintVersion
    interval_ms integer not null,   -- sprite layout interval when built; used to align frames by time
    hashes      blob    not null,
    updated_at  integer not null
) without rowid;

-- 00026_video_version_candidates.sql
-- "Possibly the same video" candidates (R-7). key_a < key_b.
create table video_version_candidates (
    key_a      text    not null,
    key_b      text    not null,
    distance   integer not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;
```

`00025_video_fingerprints.sql` then rebuilds `jobs` with the same steps as `00014` and adds `'fingerprint'` to
the `kind` check. It queues a `fingerprint` job for videos with a finished sprite (`seek_thumbnail_state =
'done'` and a registered location), the same way `00015` queues jobs. Its Down deletes those jobs, restores
`jobs` and drops `video_fingerprints`.

Invariants (added to `internal/store/invariants_test.go`):

- `video_bundles.representative_key` is in that bundle's `video_bundle_members`.
- No bundle has fewer than 2 members (the writer dissolves it).
- `user_key` starts with `bundle:`.
- `video_bundle_members.content_key` is not empty.
- A pair in `video_version_candidates` is not in `video_version_dismissals` and is not two videos of the same
  bundle.

Categories: `video_bundles`, `video_bundle_members` and `video_version_dismissals` are user data that cannot be
rebuilt (added to the list in ARCHITECTURE.md). `video_successions`, `video_fingerprints` and
`video_version_candidates` are index data. When nothing references a content any more (re-keying in
`UpsertVideo`, `DeleteVideos`, `deleteOrphanVideos`, deleting or replacing a media folder), the same transaction
that deletes the video row deletes that key's rows in these tables (`releaseContentIndex`). Cleaning up generated
files (`RemoveContent`) does not touch the tables.

`domain.FolderIndexVersion` becomes 2 (§4).

## 2. Values added to `domain`

| Value | Content |
| --- | --- |
| `Video.UserKey` | The user key (§3). The bundle's `user_key` for a bundle member, otherwise equal to `ContentKey`. Filled by the store; empty for a video with an empty `ContentKey` |
| `Video.Versions` | `*VideoVersionsRef{Count int, RepresentativeID int64}`. Filled only by the detail read, only for bundle members. `Count` is the number of members with a location the viewer may see |
| `VideoVersions{RepresentativeID int64, Items []Video}` | Every version of the bundle. Representative first, then by natural title order (ties by id) |
| `DurationsMatch(a, b int64) bool` | `abs(a-b) <= max(1000, max(a,b)*0.005)`. A duration of 0 or less never matches |
| `FingerprintVersion = 1` | Version of the fingerprint rules |
| `FrameHash{Hash uint64, Flat bool}` | One frame's hash and the flat flag (excluded from comparison) |
| `Fingerprint{Version int, IntervalMs int64, Frames []FrameHash}` | `Encode() []byte` and `DecodeFingerprint([]byte, version, interval)`. 9 bytes per frame |
| `HashFrame(luma [32][32]uint8) FrameHash` | Bits from comparing the 63 low-frequency coefficients of the 8×8 block of a 2D DCT (excluding DC) with their median. `Flat` when the luma variance is below `FingerprintFlatVariance` |
| `CompareFingerprints(a, b Fingerprint) (distance int, ok bool)` | See below |
| `FingerprintMatchMaxDistance = 12` | Upper bound on the median for a candidate |
| `JobFingerprint JobKind = "fingerprint"` | Last in `JobKinds`. `ClaimConditionFor` is `RegisteredLocation` and a new `SeekThumbnailFinished` (`seek_thumbnail_state = done`). Also added to `ScanActivityKind` |
| `VideoBundleChanged{VideoIDs []int64}` | A bundle change. Implements `Event` (R-9) |
| Errors | `ErrNotBundled` (changing the representative of, or removing, a video in no bundle), `ErrRepresentativeNotSelected` (`representativeId` is not in `videoIds`), `ErrTooFewVersions` (fewer than 2 videos resolved) |

`CompareFingerprints` returns `ok = false` when the versions differ. The intervals need not match: frame `i` of
`a` is paired with the frame of `b` that covers the midpoint of its interval (`i*a.IntervalMs +
a.IntervalMs/2`), using the same rule as `FrameAt` on `b`'s layout; a midpoint beyond `b`'s frame count is not
paired. The result is the median Hamming distance over pairs where neither frame is `Flat`. Videos longer than
405 seconds have an interval of `ceil(duration / 81)`, so a few milliseconds of difference in duration changes
the interval; frames are therefore aligned by time, not by index. When fewer than
`FingerprintMinComparableFrames` (3) pairs are compared, `ok = false`.

## 3. User key

A video `v`'s user key is decided by this expression (`userKeyExpr(alias)` in `internal/store/user_keys.go`).

```sql
coalesce((select b.user_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
          where m.content_key = v.content_key), v.content_key)
```

An empty `content_key` is tied to no user data, as before (the `content_key <> ''` condition stays).

Reads and writes that go through this expression (R-2):

| Path | Now | Change |
| --- | --- | --- |
| `videoColumnsTemplate`, `listColumns`, external API columns | `videos.content_key` | Also put `userKeyExpr` in `Video.UserKey` |
| `publicColumn`, `publicVideoCondition` | `public_videos.content_key = videos.content_key` | Join on the key expression |
| `playback_progress` join in `filteredFrom` and `libraryItemsCTE` | `p.content_key = videos.content_key` | Join on the key expression |
| `videoHasTagCondition`, `tagNameMatchCondition`, tag counts and summaries | `video_tags.content_key = v.content_key` | Join on the key expression. Tags from folder names (`video_folder_names`, `video_id`) do not change |
| `TagsByContentKeys`, `ProgressByContentKeys` | The caller passes `ContentKey` | `internal/httpapi` passes `UserKey`. Hand-added tags are read by that key; the second source of `tagsByContentKeys` goes through `videos.id` so that folder-name tags are still read from `videos.content_key` |
| `PUT /api/videos/{id}/progress` | `SaveProgress(video.ContentKey)` | `SaveProgress(video.UserKey)`. The duration is that of the version that played |
| `registeredContentKeysForVideoIDs` (adding and removing tags, tag summaries, toggling visibility) | `content_key` | `userKeysForVideoIDs` (one entry per bundle). `SetVideosPublic` includes every member's `content_key` of the affected bundles in the keys it returns (cutting off guest streams is per content) |
| `OverrideStore` (display name, thumbnail position) | `content_key` | No change (per content) |
| Generated files, fingerprints, live-transcode probe data | `content_key` | No change |

## 4. Shown videos and listing

`shownVideosCTE(audience)` returns the videos shown to the viewer as `shown(video_id, bundle_id)`
(`internal/store/user_keys.go`):

- Videos in no bundle that have a location the viewer may see (`visibleLocationCondition`).
- The **effective representative** of each bundle: the `representative_key` video if it has a location the
  viewer may see; otherwise the member with the smallest `videos.id` among those with such a location. When no
  member has one, the bundle does not appear (R-3).

Reads that go through it (R-3, R-4):

| Read | Change |
| --- | --- |
| `chosenLocationsCTE` | Applies the scope to the locations of `shown` videos, and changes the search expression to `exists (the expression is true on a registered location of some video in the same bundle)` (the video's own locations when it is in no bundle). `chosen.path` stays the smallest path among the video's own locations in scope |
| `libraryItemsCTE`, `LibraryIDs`, `ListVideos`, `ListFolderVideos`, `CountVideos` | Follow, since they go through `chosen`. Tags, watch state, sort order and `total` are the shown video's values |
| Videos directly in a folder and folder counts (`DirectVideoPaths` and folder-list counts in `folders.go`) | Count only the locations of `shown` videos. A folder holding only non-representative versions is a folder with no videos (Edge Case) |
| `VideosAddedNear`, `VideosByIDs` (related) | Limited to `shown` videos |
| `folderIndexLocations` | Only the locations of `shown` videos as the owner sees them. `FolderIndexVersion = 2` |
| `GetVideo`, `VideoLocations`, streaming, subtitles, `versions` | No change. Non-representative versions are returned too |

For guests, visibility is decided by the bundle's key (§3), so every version of a public bundle has a location
guests may see.

## 5. Carry-over of content at the same path

In `UpsertVideo` (`internal/store/scan_index.go`), when `locationExists && oldKey != file.ContentKey`, `newVideo`
holds, and the previous video row loses its last location and is deleted:

1. Read `duration_ms` before deleting the previous row. If it is null, record nothing (videos whose probe has not
   finished or has failed are out of scope; Edge Case).
2. Write `(new_key = file.ContentKey, old_key, old_duration_ms, ready = 0)` to `video_successions` (replacing any
   existing row for `new_key`).
3. If the key `newVideo` creates equals a `video_successions.old_key`, delete that row (the previous content moved
   to another path; Edge Case "files swapped").

The decision waits until the recording scan has seen every path. The scan visits paths in order, and the worker
processes the new content's probe job while the scan is still running, so the probe can finish before the
previous content is found at another path (step 3). Deciding at that point would leave no record to cancel, and
the outcome of the Edge Case "files swapped" would depend on order. So two places call the same
`applySuccession` and decide only rows with `ready = 1`:

| Caller | When |
| --- | --- |
| `ScanStore.FinishScan` | In the transaction that closes the scan with `state = done`: sets every `video_successions` row to `ready = 1` and decides each row whose `new_key` video has a known `duration_ms`. A scan that closes with `failed` has not seen every path, so it leaves `ready` unchanged (the next scan that closes with `done` decides) |
| `ApplyProbe`, `ApplyProbeForJob` | In the same transaction, after writing the probe result (when the probe finishes after the scan has closed) |

`applySuccession(tx, contentKey, durationMs)`:

1. Read the row with `new_key = contentKey` and `ready = 1`; if there is one, delete it. If there is none, stop
   (rows with `ready = 0` are kept).
2. If not `DurationsMatch(old_duration_ms, durationMs)`, stop (overwritten by a different video; requirement 8).
3. If any video references `old_key`, stop.
4. Carry over. If `old_key` is in `video_bundle_members`, re-key its `content_key` to `new_key`, and if
   `representative_key = old_key`, set it to `new_key`. Otherwise re-key these rows: `playback_progress` (if a
   `new_key` row exists, replace it with the `old_key` row), `public_videos` (same), `video_tags` (union),
   `video_version_dismissals` (`old_key` becomes `new_key`, reordered, duplicates collapsed to one). For a bundle
   member too, `old_key`'s own rows in these tables (its values from before joining the bundle) are re-keyed the
   same way.
5. Publish `VideoBundleChanged` after commit. `VideoIDs` is the video that received the carry-over and, if
   `old_key` was a bundle member, the video ids of every member of that bundle (looked up after re-keying). The
   remaining members' `versions` (representative and count) change too, so every member's video page refetches
   ([contracts/screen-api.md §6](contracts/screen-api.md)).

If the probe fails up to the limit, the row stays and is decided when a retry (`RetryProbe`) succeeds. The row is
deleted when nothing references the `new_key` content any more (§1).

## 6. Fingerprints

The `fingerprint` job (R-6):

1. `ClaimJob` claims only videos with `seek_thumbnail_state = done`.
2. `app.Ingest.Fingerprint` checks `JobIdentityCurrent`, reads the layout with
   `ArtifactStore.SeekSprite(contentKey)`, reads each sheet with `SeekSpriteSheet`, and passes them to
   `media.SpriteFingerprint(sprite, sheets)`. With no sprite it returns an error (the job retries and becomes
   `failed` at the limit; issues are recorded as for the other stages).
3. `media.SpriteFingerprint` cuts out each frame, crops black bars on all four sides (rows and columns with
   average luma below 16), shrinks the frame to 32×32 luma, and returns `Fingerprint{Version, IntervalMs,
   Frames}` built from `domain.HashFrame` results.
4. `IngestStore.ApplyFingerprintForJob(job, fingerprint)` replaces the `video_fingerprints` row and rebuilds the
   candidates of §7 in the same transaction.

Finishing a seek thumbnail (`done` in `SetSeekThumbnailStateForJob`) and finishing a rebuild in
`RequeueMissingSeekThumbnails` queue a `fingerprint` job with `requeueJob`. Fingerprints have no state column in
`videos`, so a job that fails up to the limit (for example, a transient failure reading generated files) is never
queued again from those two places. The scan therefore restores consistency: `domain.IndexedVideo` gains
`FingerprintMissing` (`seek_thumbnail_state = done` and no `video_fingerprints` row for the current
`FingerprintVersion`), and `Scanner.ensurePendingJobs` calls `EnsureJob(JobFingerprint)` when it is true. For
`fingerprint`, `EnsureJob` uses this condition instead of `pending` in a state column, discards a `failed` row and
queues again (it does not queue when a queued or running row exists). A failed fingerprint is thus rebuilt on the
next scan, and a version bump catches up without waiting for a migration. Fingerprints built from old six-sheet
sprites align frames by time too (§2 `CompareFingerprints`), so they can be compared with fingerprints of the
current layout.

## 7. Candidates

`ApplyFingerprintForJob` deletes the candidates for that `content_key` (`K`) and rebuilds them with the statement
below. `vv_fingerprint_distance(a, b)` is a deterministic function that runs `DecodeFingerprint`, calls
`CompareFingerprints`, and returns -1 when the two cannot be compared.

```sql
insert or ignore into video_version_candidates (key_a, key_b, distance, created_at)
select min(?, f.content_key), max(?, f.content_key), vv_fingerprint_distance(?, f.hashes), ?
  from video_fingerprints f
  join videos v on v.content_key = f.content_key
 where f.content_key <> ? and f.version = ?
   and v.duration_ms is not null and abs(v.duration_ms - ?) <= max(1000, max(v.duration_ms, ?) * 5 / 1000)
   and not exists (select 1 from video_version_dismissals d
                   where d.key_a = min(?, f.content_key) and d.key_b = max(?, f.content_key))
   and not exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
                   where ma.content_key = ? and mb.content_key = f.content_key)
   and vv_fingerprint_distance(?, f.hashes) between 0 and ?
```

| Event | Effect on candidates |
| --- | --- |
| Bundling | Deletes the candidates of pairs that ended up in the same bundle |
| Dismissal (`Dismiss`) | Writes `video_version_dismissals` and deletes that pair's candidate |
| Nothing references a content any more | Deletes that key's candidates (§1) |

`Candidates(audience = owner)` returns pairs where both keys have a video with a registered location, newest
`created_at` first (at most 200; `total` counts all). The response's `Video`s are each key's video
([contracts/screen-api.md §5](contracts/screen-api.md)).

## 8. Store operations (`VersionStore`)

A role that holds `*DB` (to publish after commit and to rebuild the folder index). Each operation runs in one
transaction and leaves nothing behind if it fails part-way.

| Operation | Rule |
| --- | --- |
| `Bundle(videoIDs, representativeID) (VideoVersions, error)` | See below |
| `MakeRepresentative(videoID) (VideoVersions, error)` | `ErrNotBundled` if not a member. Changes `representative_key`; values are not touched. `rebuildFolderIndex`, `VideoBundleChanged{every member}` |
| `Unbundle(videoID) (Video, error)` | `ErrNotBundled` if not a member. Deletes the member row (the video returns to the values under its own `content_key`). If it was the representative, one of the rest chosen by the effective-representative rule (§4) becomes the representative. If one member remains, the bundle is dissolved: the rows of the three tables under the bundle's key are copied to the remaining video's `content_key` (replacing existing rows) and the bundle row is deleted (members cascade). `rebuildFolderIndex`, `VideoBundleChanged{every former member}` |
| `Versions(audience, videoID) (VideoVersions, error)` | `ErrNotFound` if the video cannot be shown. A video in no bundle returns itself alone. Returns the members with a location the viewer may see, representative first |
| `Candidates() (CandidatePage, error)`, `Dismiss(videoIDs [2]int64) error` | §7 |

`Bundle`:

1. Resolve the ids to the `content_key`s of videos currently in the library (an id that does not resolve is
   `ErrNotFound`). Fewer than 2 is `ErrTooFewVersions`; a representative not among them is
   `ErrRepresentativeNotSelected`.
2. Create a new bundle and move each video into it (for a video already in a bundle, every member of that
   bundle).
3. Copy the `playback_progress`, `video_tags` and `public_videos` rows under the representative's `UserKey` (the
   key of the representative's bundle, if it is in one) to the new `user_key`.
4. Once its members have moved, delete each absorbed bundle's `video_bundles` row (no bundle is left without
   members; the invariant in §1). The values under an absorbed bundle's key in the three tables are not deleted
   (Edge Case "bundling bundles together": the values stay under that key).
5. Delete the candidates of pairs now in the same bundle, call `rebuildFolderIndex`, and publish
   `VideoBundleChanged{every member}`.

`getVideo` (detail) fills `Video.Versions`. It is not in the list columns.
