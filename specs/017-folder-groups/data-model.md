# Data model: Folder groups and folder-derived tags

This document covers only what parent Issue #326 stores, what it derives, and the rules for both.
The existing tables (`videos`, `video_locations`, `playback_progress`, `tags`, `tag_names`,
`video_tags`, `media_folders`, `public_videos`) do not change. The table classes (index and user
data) follow "Rebuildable and user data" in [ARCHITECTURE.md](../../ARCHITECTURE.md).

## 1. Migration

Add `internal/store/migrations/00012_folder_groups.sql`.

```sql
-- User data. Rescans, media folder changes and restarts must not erase it.
-- No foreign key to the media folders or to videos.
create table folder_group_overrides (
    -- The folder's absolute path, normalized with folderKey from §2.
    path       text    primary key,
    mode       text    not null check (mode in ('ungroup', 'group_direct')),
    updated_at integer not null
) without rowid;

-- The tables below are index data. The rebuild in §3 replaces them entirely.
create table folder_groups (
    id              integer primary key,
    -- The key that identifies the folder: folderKey from §2. It defines group identity.
    path_key        text    not null unique,
    -- The folder's absolute path as spelled (taken from the smallest path among the locations under it).
    -- The response's VideoFolder (rootId and relative path) is built from it with domain.LocateFolder (§2).
    path            text    not null,
    -- The folder name (the last segment, as in domain.FolderName).
    name            text    not null,
    -- Sort key for title order: domain.NaturalSortKey(name).
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    -- Order within the group (0-based).
    position  integer not null,
    unique (group_id, position)
);

-- Ancestor folder names per video (§4).
create table video_folder_names (
    video_id integer not null references videos (id) on delete cascade,
    name     text    not null,
    primary key (video_id, name)
) without rowid;
create index video_folder_names_name_idx on video_folder_names (name, video_id);

-- Versions of the rules that built the index. If either differs from the current value, startup rebuilds (§3).
-- title_key is built with domain.NaturalSortKey, so its version (SearchKeyVersion) is stored too.
create table folder_index_state (
    id             integer primary key check (id = 1),
    version        integer not null,   -- domain.FolderIndexVersion
    search_version integer not null,   -- domain.SearchKeyVersion
    -- 1 means "the previous rebuild failed". A successful rebuild resets it to 0 (§3).
    stale          integer not null default 0 check (stale in (0, 1))
);
```

When a `videos` row is deleted, its member and folder name rows are deleted by cascade. A group left
with one member stays a one-video group until the next rebuild (§3). The card artwork thumbnails
(`previews`) and the item `id` are not stored. Each read takes them from the remaining members, so
they never point to a deleted video.

## 2. Assignment rules

One pure function in `internal/domain/folder_group.go` takes the inputs below and returns the
groups (path, name, ordered members) and the ancestor folder names per video. The store only writes
this result to the tables.

- Input: the media folders, every location under them (video `id` and path), and the overrides
  (`folderKey` → `mode`).
- A folder `D` is identified by `folderKey(D)` in grouping (`direct`, `hasChild`, whether it is a
  root), in override matching, and in resolving `(rootId, path)` from the API. On Windows, locations
  whose spelling differs only in letter case fall in the same folder.
- **Representative location**: the video's location with the smallest path in byte order (same as
  `order by path limit 1` in `videoColumnsTemplate`).
- **Direct videos** `direct(D)`: videos whose representative location's parent folder is `D`.
  Another location of the same content does not count (Requirement 6).
- **Has a subfolder** `hasChild(D)`: some location (not only a representative one) lies under a
  subfolder of `D`. Like `folderCount` on the folder page, it is derived from locations. Empty
  folders do not count.
- **Becomes a group**: `len(direct(D)) >= 2`, and one of:
  - `mode(D) = group_direct`
  - `mode(D)` is absent, `hasChild(D)` is false, and `D` is not the media folder itself
- **Order**: the same total order as `compareSiblings` (natural file name order, byte order, `id`),
  the comparison used today for previous/next within a folder (Requirement 4). Only `direct(D)` is
  ordered. A video whose representative location is in another folder is in today's same-folder
  previous/next (`DirectVideoPaths`) but not in the group (Requirement 6).
- **Name**: the folder's last segment, as in `domain.FolderName` (Requirement 5).
- **folderKey**: drop a trailing separator from the path; on Windows, normalize `/` to `\` and apply
  `LowerASCII`. Both override matching and the override primary key use it. It follows the same
  separator and case rules as the media folder checks (`registeredLocationCondition`,
  `LocateVideoFolder`).
- **VideoFolder of a folder**: add `domain.LocateFolder`, which computes `(rootId, path)` from the
  absolute path of the **folder itself** of a group or override. The current `LocateVideoFolder`
  takes a location (file) path and drops the last segment as the file name, so a folder path would
  resolve one level up.
  - Both share `pathBelowRoot` and differ only in whether they drop the last segment.
  - A test checks, with nested folders, that the group's `folder` matches the lookups of
    `GET /api/folders/{rootId}/group` and `getFolder`.
- An override is not deleted when no folder matches its path. It takes effect again when a folder
  with the same path returns (Edge case).

## 3. Rebuild points

`rebuildFolderIndex(tx)` reads every location under the media folders and the overrides, passes
them through the §2 function, deletes and rewrites `folder_groups`, `folder_group_members` and
`video_folder_names`, and sets `folder_index_state` to the current `domain.FolderIndexVersion` and
`domain.SearchKeyVersion`. It runs inside one write transaction, so a read sees only the state
before or after the rebuild.

| Point | Caller | Transaction |
| --- | --- | --- |
| Just before a scan closes (success or failure) | `app.Scans` → `ScanIndexStore.RebuildFolderIndex` | A rebuild-only transaction |
| Adding, replacing or removing a media folder | `SettingsStore` | The same transaction as the change |
| Setting or clearing an override, turning a group into a tag | `FolderGroupStore` | The same transaction as the change |
| Startup, when `version` or `search_version` differs from the current value, `stale = 1`, or no row exists | `cmd/mdm` → `ScanIndexStore.RefreshFolderIndex` | A rebuild-only transaction, after `RefreshSearchKeys` and before HTTP and the workers start |
| Startup, when scans interrupted by the previous shutdown are closed (`FailInterruptedScans` closes 1 or more) | `app.Scans.RecoverInterrupted` → `ScanIndexStore.RebuildFolderIndex` | A rebuild-only transaction |

During a scan (before it closes), added videos appear as single videos, and the group of a removed
video appears with its remaining members. The rebuild before the scan closes produces the correct
shape.

Failure handling (Structural Decision 14 in the plan):

- When the rebuild fails, its transaction rolls back. A separate transaction writes
  `folder_index_state.stale = 1`, the failure is logged, and the scan closes. The previous index
  stays in use until the next rebuild point (including the next startup).
- When writing `stale` also fails, the rebuild on the next scan, override change or media folder
  change repairs it.
- When the process stops in the middle of the rebuild transaction, the scan stays open, and the
  recovery of interrupted scans at startup rebuilds.
- Paging across a rebuild can duplicate or skip items, as during an import in 013.

## 4. Folder-derived tags

- **Folder names**: for **every** location of a video under its media folder, take the segments
  below the media folder and above the file (including the group's folder) and apply
  `domain.NormalizeTagName`. Names that fail are discarded. Deduplicate per video and write them to
  `video_folder_names` (Requirement 9, Edge case "Same content in several places").
- **Attached tags**: each `tag_names.tag_id` with a row where `video_folder_names.name =
  tag_names.name`. Both primary names and synonyms match. Matching is exact, like the `tag_names`
  primary key (014 §2, §3).
- **Tags of a video** = tags attached by hand (`video_tags` joined on `videos.content_key`) ∪
  folder-derived tags. A tag attached from both sources becomes one entry that carries both sources.
- Reads that use this:
  - `TagStore.TagsByContentKeys` (`Video.tags`): returns tags with their sources.
  - Tag filtering (014 §6): each condition's `exists` becomes an OR of "a row attached by hand" and
    "a folder name row".
  - Tag name matching in the search box (014 §7): whether a tag with the tag name key is attached to
    the video is also an OR of the two sources.
  - Video count per tag (014 §5): counts videos currently in the library with the tag from either
    source.
  - Selection summary: `count` counts either source; `manualCount` counts tags attached by hand
    only.
- Detaching (`DetachTag`) deletes only from `video_tags`. It needs no change (Requirement 13).
- The media folder's own name is not a folder name ("below the root" in Requirement 9). So when
  "Group this folder's videos" makes the media folder itself a group, it cannot become a tag (409,
  [contracts/folder-groups-api.md §2](contracts/folder-groups-api.md#2-turning-a-group-into-a-tag)).
  Creating that tag would not give the result of Requirement 11 (the videos inside get the tag).
- Turning a group into a tag (Requirement 11) runs in one `FolderGroupStore` transaction:
  1. Pass the folder name through `NormalizeTagName`. On an error, write nothing.
  2. Use the tag found by name or synonym, or create a new one.
  3. Write `ungroup` for the folder.
  4. Run the rebuild from §3.

## 5. Library items

The `GET /api/library` query adds a step that groups results into items to the 013 flow (scope and
search expression → `chosen` → filters → keyset).

1. **Per-member filtering**: join `videos` to `chosen` (scope is the library, plus the search
   expression) and apply the playability and tag AND (each tag an OR of the §4 sources). Videos
   that pass are the "matching videos". One video must satisfy every condition (Requirement 19).
2. **Grouping into items**: a matching video with a `folder_group_members` row goes into its group;
   one without becomes a video item. A group becomes one item when it has one or more matching
   members.
3. **Group aggregates**: computed from **all members** of the group, matching or not.

   | Value | Video item | Group item |
   | --- | --- | --- |
   | Added time | `videos.added_at` | Maximum over members |
   | Modified time | `mtime` of the `chosen` location | Maximum `mtime` of the members' representative locations |
   | Last played time | `playback_progress.updated_at` | Maximum over members (NULL when none) |
   | Title | `title_key` of the `chosen` location | `folder_groups.title_key` |
   | Length | `videos.duration_ms` | Sum over members with a known length; NULL when none is known |
   | File size | `size_bytes` of the `chosen` location | Sum of `size_bytes` of the members' representative locations |
   | Watch status | The current `watchCondition` | §6 |

4. **Per-item filtering**: apply the watch status filter to the watch status above (Requirement
   19).
5. **Sort and keyset**: sort values come from the table above. For ties and the keyset `id`, a
   video item uses the video `id`, and a group item uses the video `id` of its remaining member with
   the smallest `position`. Members do not overlap, so items do not overlap. The shuffle key is
   `vv_shuffle_key(seed, that id)`. The cursor shape is the same as in 013.
6. **Count**: the number of items that pass step 4 (Requirement 20).
7. **"Select all"**: for the items that pass step 4, the video `id` of each video item and the `id`
   of every member of each group (Requirement 21).

## 6. Group watch status and the member to open

A function in `internal/domain` decides both from the ordered members' playback records (none,
position, completed). The per-video definition is the same as `domain.ClassifyWatch`.

- **Watch status**: `unwatched` when no member has been started (position above 0, or completed);
  `watched` when every member is completed; `inProgress` otherwise. The **watched count** is the
  number of completed members (Requirement 17).
- **Member to open**: in order, the first member with a position above 0 that is not completed.
  Otherwise the first member that is not completed. When all are completed, the first member
  (Requirement 23).
- A test checks with the same inputs that the result equals the watch status in the item SQL (§5,
  step 4).

## 7. Visibility per viewer

The index (§1 to §3) does not depend on the viewer. There is exactly one, built from every location
the owner sees. A guest response applies the 016 public condition (`visibleLocationCondition`,
[016 data-model.md §3](../016-single-account-auth/data-model.md#3-audience-and-the-public-video-condition)) to the
members at read time.

- **Group members**: a guest counts only public members. `videoCount`, `videoIds`, `previews`, the
  length and size sums, the maximum added and modified times, `position` and `count` of
  `Video.group`, `group.items` of the related videos, and previous/next within the group are all
  built from public members only. A group with exactly one public member appears to a guest as a
  video item (no one-video group card). A group with no public member does not appear.
- **Watch data and tags**: a guest receives neither playback positions nor tags (016 guest-api §1).
  `watchedCount`, `watchState` and `lastPlayedAt` of `LibraryGroup` are omitted, and `tags` is
  empty. `openVideoId` is the first public member in order. Folder-derived tags are likewise not
  used in guest responses, filters or search (a `tag` condition returns 400 for a guest, and search
  does not match tag names).
- **Overrides and the grouping decision**: which folders are groups is decided from all of the
  owner's locations (§2). A guest receives only that result narrowed to public members, and never
  the count or names of private videos.
- `GET /api/folders/{rootId}/group`: 404 for a guest unless the group has 2 or more public members.
