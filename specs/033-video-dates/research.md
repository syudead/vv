# Research: Video edit time and file creation time

Parent Issue: #630.

Inherited decisions: the tech stack, boundaries, dependency direction, and the
split between index and user data are defined by
[ARCHITECTURE.md](../../ARCHITECTURE.md) and
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
and do not change here. The rule for keying user data to the content key is in
[specs/029-video-overrides/research.md R-1](../029-video-overrides/research.md),
and the user-data key of a bundle (versions) is in
[specs/030-video-versions/data-model.md, User key](../030-video-versions/data-model.md#user-key).
This file records only the decisions this feature adds.

## R-1: The edit time lives in a user-data table `video_edits` keyed by content key, and reads fall back to the added time

**Decision**: Add `video_edits(content_key primary key, edited_at)`. Reads that
return videos put `coalesce(video_edits.edited_at, videos.added_at)` into
`Video.EditedAt`. A video without a row has its added time as its edit time, so
existing videos need no backfill (requirement 2). The same-path content
succession (`moveUserData`) also moves this table's rows (edge case "a video
carried over to a new version").

**Rationale**: The edit time records the owner's actions; it is user data that a
scan cannot rebuild. Keyed by content key like `playback_progress`,
`public_videos` and `video_overrides`, it survives rescans, moves and renames, and
it rides the existing succession mechanism (one more entry in `moveUserData`'s
table list).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A column `videos.edited_at` | Rejected: `videos` is a rebuildable index, and its row disappears when the content changes. Succession would have to copy the vanished row's value into `video_successions` and carry it along, and it breaks the index/user-data split in ARCHITECTURE.md |
| Key by the user-data key (`bundle:<id>` for a bundle) | Rejected: the initial value is each video's added time, and bundle members have different added times, so a bundle cannot hold one value. When a bundle value (tags, visibility) changes, the time is written to the content key of every member the key points to (`contentKeysForUserKeys`, the same expansion as `VisibilityStore.SetVideosPublic` returning per-content keys) |

## R-2: Only the four writes of video information advance the edit time; tag-level operations, bundles and ingestion do not

**Decision**: Only these writes advance it:

| Information | Writes |
| --- | --- |
| Display name | `OverrideStore.SetDisplayName`, `SetDisplayNames` |
| Thumbnail position | `OverrideStore.SetThumbnailPosition` (set and clear) |
| Visibility | `VisibilityStore.SetVideosPublic` |
| Attaching and detaching manual tags | `TagStore.AttachTagByID`, `AttachTagByName`, `DetachTag`, `ApplyVideoTags` (bulk through the external API and MCP) |

These do not advance it: playback position (`PlaybackStore`); ingesting scans,
probes, thumbnails, previews and fingerprints (`ScanIndexStore`, `IngestStore`);
operations on tags themselves (create, rename, delete, merge, synonyms, confirming
and rejecting tentative tags; not even when `video_tags` rows change by cascade);
changes to folder-name tags (rebuilding the folder index, group-to-tag
conversion); bundling, changing the representative and unbundling versions
(`VersionStore`); and same-path succession.

**Rationale**: Requirement 1 in the parent Issue says "when the owner changes the
video's information", and the four writes in acceptance criterion 1 are exactly
those. Deleting or merging a tag changes `video_tags` for thousands of videos in
one action, so advancing on it would hide when each video was last edited.
Bundling is a structural operation that copies values and does not rewrite video
information. Folder-name tags are attached by the scan, not edited by the owner.

**Alternatives considered**: Advance on every change to `video_tags`,
`public_videos` and `video_overrides` (uniformly, with triggers). Rejected: for the
reasons above, it would also advance on tag delete, merge, succession and
bundling.

## R-3: Unchanged edits do not advance, and only the content keys a write actually changed advance

**Decision**: Each operation collects the user-data keys whose rows the write
actually changed and writes `touchEditedAt`
([data-model.md, Edit time rules](data-model.md#edit-time-rules)) only for the content keys
those keys point to (every member for a bundle). Change is determined as follows:

| Data | How change is detected |
| --- | --- |
| `video_tags`, `public_videos` | `RowsAffected` of `insert or ignore` and `delete` (paths that write one key at a time). `applyManualTags`, which writes a set through json_each, finds the changing keys with one statement in the same transaction before writing (add: keys missing some tag of the set; remove: keys carrying some tag of the set; replace: either of those, or keys carrying a tag outside the set). `returning content_key` is not used, because it would deliver every changed row (up to 20000 × 100, about 2 million rows) inside the write transaction |
| Display name | Read the targets' current `display_name` at the start of the transaction; if the name left at the end of the transaction (the last name when a `SetDisplayNames` bulk writes the same content key several times) is the same (including both unset), do not advance. Comparing before and after each single write would advance on an A→B→A bulk whose value does not change |
| Thumbnail position | Read the current `thumbnail_position_ms` before writing; if it is the same (including both cleared), do not advance. The image is still regenerated as before |

The number of statements is constant regardless of the number of keys
(`touchEditedAt` writes the set passed through json_each in one statement). Even
for a bulk operation at the limit, the write transaction receives at most as many
rows as keys and runs a few statements, so the time other writes wait is limited
to work inside SQLite.

If a write fails and the transaction rolls back, `video_edits`, written in the
same transaction, rolls back too (edge case "when an edit fails").

**Rationale**: The edge cases "do not advance the edit time when the value did not
change" and "in a bulk operation, each video that changed" require knowing change
per row, not per operation.

**Alternatives considered**: Advance every target when the operation succeeds.
Rejected: re-attaching an attached tag or saving the same display name would
advance it.

## R-4: The file creation time lives in a location column `video_locations.file_created_at`, null when unavailable, and reads fall back to mtime

**Decision**: Add a nullable `file_created_at` (Unix seconds) to
`video_locations`. The scan writes the creation time it could read, and null when
the file system does not provide one (a value from an earlier scan also returns to
null once it can no longer be read;
[R-6](#r-6-a-registered-location-is-rewritten-on-the-next-scan-when-its-creation-time-differs-even-if-the-file-is-unchanged)).
The value is stored in seconds, like `mtime`. Reads that return videos and the
sort use `coalesce(file_created_at, mtime)` of the listed location as
`Video.FileCreatedAt` and as the value for `createdAsc` / `createdDesc`
(requirements 3 and 5, edge case "videos already registered").

**Rationale**: The creation time is a fact about the file, so it is a location
column like `mtime` and `size_bytes` (a rebuildable index). Keeping null means
that when a file moves from a file system without creation times to one with them
(copied onto ext4, for example), the next scan can fill in the real value. The
fallback rule sits in one place, on the read side.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Write mtime when the creation time is unavailable | Rejected: a substitute cannot later be told from a real value, and it is never updated once the real value becomes available |
| A column on `videos` | Rejected: a video with several locations has a different creation time per location, and the value would change with the representative (edge case) |

## R-5: The file-system adapter `internal/scanner` reads the creation time per OS; Linux uses statx from `golang.org/x/sys/unix`

**Decision**: Put
`fileCreatedAt(path string, info fs.FileInfo) (time.Time, bool)` in
`internal/scanner`, split by build tag:

| OS | Implementation |
| --- | --- |
| Linux | `unix.Statx` requesting `STATX_BTIME`; returns a value only when the bit is set in `Mask`. `golang.org/x/sys` becomes a direct dependency (it is indirect today) |
| darwin, freebsd, netbsd | `Birthtimespec` of `info.Sys().(*syscall.Stat_t)`. One file, `file_created_at_bsd.go`, with `//go:build darwin \|\| freebsd \|\| netbsd` (the name `_darwin.go` would add an implicit GOOS constraint, and freebsd and netbsd would not compile it) |
| windows | `info.Sys().(*syscall.Win32FileAttributeData).CreationTime` |
| Others (`file_created_at_other.go`, a build tag excluding the OSes above) | Always unavailable |

A failed read is not an error; the video is registered without a creation time.
`domain` does not know this function.

**Rationale**: The standard library's `os.FileInfo` does not expose the creation
time on Linux, and vv's main deployment target is a Linux container
(ARCHITECTURE.md "Intended topology"). `x/sys/unix` is already an indirect
dependency, and reading file-system facts is the adapter's job (dependency
direction).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Standard library only, always falling back to mtime on Linux | Rejected: requirement 3's "the creation time when available" would not hold on the main deployment target |
| The `creation_time` tag from `ffprobe` | Rejected: it is container metadata, not necessarily when the file was written, and often missing. It is not the file creation time the requirement asks for |

## R-6: A registered location is rewritten on the next scan when its creation time differs, even if the file is unchanged

**Decision**: The scan reads the creation time even for unchanged files (same size
and mtime). If it differs from the indexed value (`IndexedVideo.FileCreatedAt`,
zero value when absent), the scan writes only the location column through
`Index.UpdateLocationCreatedAt(ctx, locationID, createdAt)`. It does not recompute
the content key, does not enqueue jobs, and moves neither `videos.indexed_at` nor
any event. For a file whose content changed, `UpsertVideo` writes
`VideoFile.FileCreatedAt` together with the other facts (edge case "the file was
replaced at the same path and its creation time changed"). The comparison is in
seconds, like `mtime`, and a failed read compares as the zero value (if the index
has a value, it returns to null; R-4).

**Rationale**: The edge case says existing videos get their creation time "on the
next scan", and only a user-started scan walks the media folders
(ARCHITECTURE.md). Cost: on darwin, the BSDs and Windows the value comes from
`entry.Info()`, which the scan already reads, so nothing is added. On Linux
`entry.Info()` (lstat) has no creation time, so each media file costs one more
`statx`. It is a metadata query that reads no content, but on a network mount it
adds a round trip per file. Requirement 3 and the edge case "videos already
registered" require the creation time for unchanged files too, so this cost is
accepted.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| `stat` every location at startup to fill the value | Rejected: it adds a path that touches the media folders outside a user-started scan |
| On Linux, skip `statx` for unchanged files whose index already has a value | Rejected: a location that can no longer be read cannot return to null, and a replacement that keeps size and mtime (`cp -p` and similar) leaves a stale creation time |
| Treat a creation-time difference as "changed" and pass it through `UpsertVideo` | Rejected: the content key (sha256 of the first and last 1 MiB) would be re-read for every file |

## R-7: The response fields are `updatedAt` (edit time in vv) and `fileCreatedAt` (location creation time), with the same names in the screen and external APIs

**Decision**: Add required `updatedAt` and `fileCreatedAt` (both date-time) to
`Video` (screen) and `ExternalVideo` (external). Guest responses include them too
(like the added time, they are facts about a public video and reveal no owner
data). Add `createdAsc` and `createdDesc` to `VideoSort` and allow them for
guests (unlike `played*`, they do not depend on owner data). `modifiedAsc` and
`modifiedDesc` keep their names and values (requirement 7). The `domain.Video`
field is named `EditedAt` so it is not confused with the existing `UpdatedAt`
(`videos.updated_at`, the update time of the ingested row; renamed to `IndexedAt`
/ `videos.indexed_at` in #650).

**Rationale**: The parent Issue's terms are `更新日時` (update time) and
`作成日時` (creation time), so `updatedAt` follows directly. API users read
`createdAt` as the row's creation time, so the name states that it is a fact
about the file.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| `editedAt` | Rejected: departs from the term in the parent Issue and the screen (update time) |
| `createdAt` | Rejected: the same name as `VideoLocation.createdAt` (creation of the location row) with a different meaning |
| Per-location `ExternalVideoLocation.createdAt` | Rejected: requirement 6 asks for the value in the video's information, and the listed location's value is enough. Per-location values stay out of scope |

## R-8: The video page reloads the video after tag and visibility changes, and no new domain event is added

**Decision**: Add a success callback `onChanged` to `VideoTags` and
`VisibilitySwitch`, and have `VideoPage` reload the video with `refresh` from
`useVideoDetail` (for the display name and thumbnail, the current path that
`replace`s the screen's video with the response's `Video` also swaps
`updatedAt`). `TagStore` and `VisibilityStore` still publish no events.

**Rationale**: Acceptance criterion 1 is that the edit time visibly advances on
the video page. Only the screen that performed the action knows the value
advanced, and one `GET /api/videos/{id}` is enough. ARCHITECTURE.md states that
tag changes have no side effects and therefore publish no events, and `TagStore`
does not depend on notifications.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Publish an event like `VideoOverrideChanged` from `TagStore` and `VisibilityStore` and map it to `video` on `/api/events` | Rejected: breaks the design above, and every card in the list would reload on each tag attach or detach |
| Change the tag attach/detach response to `Video` | Rejected: `POST /api/video-tags` is a bulk operation on several videos, which is no reason to change its response shape |
