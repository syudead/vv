# Data model: Video edit time and file creation time

Parent Issue: #630.

The rest of the model is unchanged. Table definitions are in
[internal/store/migrations/](../../internal/store/migrations/), the data
categories are in [ARCHITECTURE.md](../../ARCHITECTURE.md) "Rebuildable and user
data", and the user-data key is in
[specs/030-video-versions/data-model.md, User key](../030-video-versions/data-model.md#user-key).
This file records only the table, column, values and read/write rules this
feature adds. Tables not named here do not change.

## Migration

Two migrations, each owned by the implementation unit that adds it (the last
migration on `main` is `00026_video_version_candidates.sql`).

`00027_video_edits.sql`:

```sql
-- When the video's information was last edited in vv (specs/033-video-dates/research.md R-1).
-- User data that cannot be rebuilt; keyed by content key like video_overrides, with no foreign
-- key to videos. A video without a row has videos.added_at as its edit time.
create table video_edits (
    content_key text    primary key,
    edited_at   integer not null
) without rowid;
```

`00028_video_file_created_at.sql`:

```sql
-- Creation time (Unix seconds) of the listed location's file. Null when the file system does
-- not provide one (research.md R-4).
alter table video_locations add column file_created_at integer;
```

Down drops the table and the column respectively.

Invariant (added to `internal/store/invariants_test.go`): `video_edits` has no row
with an empty `content_key`.

Category: `video_edits` is user data that cannot be rebuilt and is added to the
list in ARCHITECTURE.md. `video_locations.file_created_at` is a location fact and
belongs to the rebuildable index (part of `video_locations`). Cleaning up
generated files (`RemoveContent`) and `releaseContentIndex` do not touch
`video_edits`.

## Values added to `domain`

| Value | Content |
| --- | --- |
| `Video.EditedAt` | `time.Time`. The edit time in vv. The store fills it with `coalesce(video_edits.edited_at, videos.added_at)` ([R-7](research.md#r-7-the-response-fields-are-updatedat-edit-time-in-vv-and-filecreatedat-location-creation-time-with-the-same-names-in-the-screen-and-external-apis)). `Video.UpdatedAt` (`videos.updated_at`) stays as is and is not in responses |
| `Video.FileCreatedAt` | `time.Time`. `coalesce(file_created_at, mtime)` of the listed location |
| `VideoFile.FileCreatedAt` | `time.Time`. The creation time the scan read. The zero value means unavailable |
| `IndexedVideo.FileCreatedAt` | `time.Time`. The value in the index. The zero value means null |
| `VideoLocation.FileCreatedAt` | `time.Time`. The zero value means null (read by `VideoLocations`) |
| `SortCreatedAsc` / `SortCreatedDesc` | `createdAsc` / `createdDesc` of `VideoSort`. Added to `Valid` |
| `ExternalVideo` | Unchanged. `Video` carries the new fields |

`domain` does not know how the creation time is read (the per-OS `stat`).

## Edit time rules

One package-internal function,
`touchEditedAt(ctx, tx, contentKeys []string, now time.Time) error`, runs
`insert into video_edits (content_key, edited_at) select distinct value, ? from json_each(?) where value <> '' on conflict (content_key) do update set edited_at = excluded.edited_at`
for non-empty content keys only, as one statement regardless of the number of
keys. Each role type calls it inside its own transaction and does not call
another role's public methods.

| Operation | Content keys it advances ([R-2](research.md#r-2-only-the-four-writes-of-video-information-advance-the-edit-time-tag-level-operations-bundles-and-ingestion-do-not), [R-3](research.md#r-3-unchanged-edits-do-not-advance-and-only-the-content-keys-a-write-actually-changed-advance)) |
| --- | --- |
| `OverrideStore.SetDisplayName` / `SetDisplayNames` | Content keys whose `display_name` before the transaction differs from the name left at the end of the transaction (the last name when a bulk writes the same content key several times). Not compared before and after each single write |
| `OverrideStore.SetThumbnailPosition` | Content keys whose `thumbnail_position_ms` before the write differs from the requested value (nil = clear) |
| `VisibilityStore.SetVideosPublic` | The user-data keys whose rows `insert or ignore` / `delete` changed, expanded with `contentKeysForUserKeys` |
| `TagStore.AttachTagByID` / `AttachTagByName` / `DetachTag` | Same as above (the keys `attachTagToVideoIDs` / `detachTagFromVideoIDs` changed) |
| `TagStore.ApplyVideoTags` (`applyManualTags`) | The keys whose rows will change, found with one statement before the write (add: keys missing some tag of the set; remove: keys carrying some tag of the set; replace: either of those, or keys carrying a tag outside the set; at most one row per key), expanded. Changed rows are not received one by one through `returning` |

`now` is the time the operation's transaction started (all targets in one
transaction get the same value). No other write touches `video_edits` (R-2).

Same-path content succession (`moveUserData`,
[specs/030-video-versions/data-model.md, Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path))
adds `video_edits` to its table list and moves it by the same rule as
`playback_progress` and `public_videos` (delete the successor's row, then
re-key). Creating and dissolving bundles (`VersionStore`) does not touch values
keyed by content key, so it does not touch `video_edits` either.

## Reads

Reads that return videos (`videoColumnsTemplate`, `listColumns`, and the external
`readVideosByIDs`, which uses `videoColumns`) gain two columns, mapped in
`scanVideo`.

| Column | Expression |
| --- | --- |
| `edited_at` | `coalesce((select e.edited_at from video_edits e where e.content_key = videos.content_key and videos.content_key <> ''), videos.added_at)` |
| `file_created_at` | `coalesce(l.file_created_at, l.mtime)` of the representative location (`videoColumnsTemplate` uses the same subquery as `mtime`; `listColumns` uses `coalesce(loc.file_created_at, loc.mtime)`) |

Sort (the same form as the table in
[specs/013-library-search/contracts/list-api.md, `VideoSort` values](../013-library-search/contracts/list-api.md#videosort-values);
that document is not changed, and this file holds the delta):

| Sort | Ascending | Descending | Value |
| --- | --- | --- | --- |
| Date created | `createdAsc` | `createdDesc` | `coalesce(file_created_at, mtime)` of the listed location |

`listOrders` uses `coalesce(loc.file_created_at, loc.mtime)`. Library items
(`libraryItemsCTE`) gain a `created_at` column in `items`: a video item takes its
location's value, and a group item takes the `max` of its members'
representative-location values (held in `mv` with a `coalesce` version of
`representativeLocationValue`). Equal values are settled by `id` (acceptance
criterion 6). The expressions for `modifiedAsc` and `modifiedDesc` do not change
(acceptance criterion 8).

Guest lists allow `createdAsc` and `createdDesc` (they do not depend on owner
data). The `400` table in `guest-api.md` does not change.

## Scan and location writes

`internal/scanner` reads `fileCreatedAt(path, info)`
([R-5](research.md#r-5-the-file-system-adapter-internalscanner-reads-the-creation-time-per-os-linux-uses-statx-from-golangorgxsysunix))
for each target.

| Path | Writes |
| --- | --- |
| `ScanIndexStore.UpsertVideo` (content changed, or a new path) | Includes `file_created_at` (seconds from `Unix()`; null for the zero value) in the location `insert` / `update` |
| `ScanIndexStore.UpdateLocationCreatedAt(ctx, locationID int64, createdAt time.Time) error` (new; added to `scanner.Index`) | `update video_locations set file_created_at = ? where id = ?` (seconds from `Unix()`; null for the zero value). Moves neither `updated_at` nor `version`, and publishes no event |
| `ScanIndexStore.IndexedVideosByPath` | Carries `IndexedVideo.FileCreatedAt` |

For an unchanged file (same size and mtime), the scan compares the creation time
it read (the zero value when unreadable) with `IndexedVideo.FileCreatedAt` (null
is the zero value) in `Unix()` seconds, like mtime (two zero values are equal),
and calls `UpdateLocationCreatedAt` only when they differ. The column holds
seconds, so a creation time with a fractional part is not rewritten on every
scan. A location whose index has a value but whose creation time can no longer be
read (moved to a file system without creation times) is called with the zero
value, which returns the column to null and makes reads fall back to mtime
([R-6](research.md#r-6-a-registered-location-is-rewritten-on-the-next-scan-when-its-creation-time-differs-even-if-the-file-is-unchanged)).
A failure is reported as `register_failed`, like a failure in
`ensurePendingJobs`. An unreadable creation time is not a failure.
