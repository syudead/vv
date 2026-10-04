# Data model: Folder groups and folder-derived tags

This document covers only what parent Issue #326 stores, what it derives, and
the rules for both. The existing tables (`videos`, `video_locations`,
`playback_progress`, `tags`, `tag_names`, `video_tags`, `media_folders`,
`public_videos`) do not change. Table classes (index and user data) follow
"Rebuildable and user data" in [ARCHITECTURE.md](../../ARCHITECTURE.md).

## Migration

Adds `internal/store/migrations/00012_folder_groups.sql`.

```sql
-- User data. Must survive rescans, media folder changes and restarts.
-- No foreign key to registered folders or to videos.
create table folder_group_overrides (
    -- The folder's absolute path normalized by folderKey (see "Assignment rules").
    path       text    primary key,
    mode       text    not null check (mode in ('ungroup', 'group_direct')),
    updated_at integer not null
) without rowid;

-- The tables below are an index. The rebuild in "When the index is rebuilt" replaces them wholesale.
create table folder_groups (
    id              integer primary key,
    -- The key that identifies the folder: folderKey (see "Assignment rules"). Group identity is decided by this.
    path_key        text    not null unique,
    -- The folder's absolute path as spelled (taken from the smallest path among the locations under it).
    -- The response's VideoFolder (rootId and relative path) is built from this with domain.LocateFolder (see "Assignment rules").
    path            text    not null,
    -- The folder name (the last segment, as domain.FolderName).
    name            text    not null,
    -- Sort key for title order. domain.NaturalSortKey(name).
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    -- Order within the group (0-based).
    position  integer not null,
    unique (group_id, position)
);

-- Ancestor folder names per video (see "Folder-derived tags").
create table video_folder_names (
    video_id integer not null references videos (id) on delete cascade,
    name     text    not null,
    primary key (video_id, name)
) without rowid;
create index video_folder_names_name_idx on video_folder_names (name, video_id);

-- Versions of the rules that built the index. If either differs from the current value, startup rebuilds (see "When the index is rebuilt").
-- title_key is built with domain.NaturalSortKey, so its version (SearchKeyVersion) is kept too.
create table folder_index_state (
    id             integer primary key check (id = 1),
    version        integer not null,   -- domain.FolderIndexVersion
    search_version integer not null,   -- domain.SearchKeyVersion
    -- 1 means "the last rebuild failed". A successful rebuild resets it to 0 (see "When the index is rebuilt").
    stale          integer not null default 0 check (stale in (0, 1))
);
```

When a `videos` row is deleted, its member and folder name rows cascade. A group
left with one member stays a one-video group until the next rebuild ([When the index is rebuilt](#when-the-index-is-rebuilt)). The
card artwork thumbnails (`previews`) and entry `id`s are not stored; they are
read from the remaining members each time, so they never point at a deleted
video.

## Assignment rules

One pure function in `internal/domain/folder_group.go` returns the groups (path,
name, ordered members) and each video's ancestor folder names from the inputs
below. The store only writes the result to the tables.

- Inputs: registered folders, every location under them (video `id` and path),
  and overrides (`folderKey` → `mode`).
- A folder `D` is identified by `folderKey(D)` in grouping (`direct`,
  `hasChild`, whether it is a root), in matching overrides, and in lookups from
  the API's `(rootId, path)`. On Windows, locations whose spelling differs only
  in case fall into the same folder.

| Term | Definition |
| --- | --- |
| **Representative location** | The location with the smallest path in byte order among the video's locations (as `order by path limit 1` in `videoColumnsTemplate`) |
| **Direct videos** `direct(D)` | Videos whose representative location's parent folder is `D`. Other locations of the same content are not counted (requirement 6) |
| **Has a child folder** `hasChild(D)` | Some location (not only representative ones) is under a child folder of `D`. Derived from locations, as the folder screen's `folderCount`. Empty folders do not count |
| **Becomes a group** | `len(direct(D)) >= 2`, and either `mode(D) = group_direct`, or there is no `mode(D)`, `hasChild(D)` is false, and `D` is not a registered folder itself |
| **Order** | The same total order as `compareSiblings` (natural order of file names, byte order, `id`), compared as the current previous/next within the same folder (requirement 4). Only `direct(D)` is ordered: a video whose representative location is in another folder is in the current same-folder previous/next (`DirectVideoPaths`) but not in the group (requirement 6) |
| **Name** | The folder's last segment, as `domain.FolderName` (requirement 5) |
| **folderKey** | Drop the trailing separator from the path; on Windows, turn `/` into `\` and apply `LowerASCII`. Used both to match locations to overrides and as the overrides' primary key. The same separator and case rules as the registered folder checks (`registeredLocationCondition`, `LocateVideoFolder`) |

- **A folder's VideoFolder**: a new function `domain.LocateFolder` computes
  `(rootId, path)` from the absolute path of **the folder itself** of a group
  or override. The current `LocateVideoFolder` takes a location (file) path and
  drops the last segment as the file name, so given a folder path it points one
  level up. The two share `pathBelowRoot` and differ only in whether the last
  segment is dropped. A test checks, with nested folders, that a group's
  `folder` matches the lookups of `GET /api/folders/{rootId}/group` and
  `getFolder`.
- An override is kept even when no folder matches its path; it takes effect
  again when a folder with the same path returns (Edge Case).

## When the index is rebuilt

`rebuildFolderIndex(tx)` reads every location under the registered folders and
the overrides, passes them through the function of [Assignment rules](#assignment-rules), deletes and rewrites
`folder_groups`, `folder_group_members` and `video_folder_names`, and sets
`folder_index_state` to the current `domain.FolderIndexVersion` and
`domain.SearchKeyVersion`. It runs inside one write transaction, so reads see
either the state before or after the rebuild.

| When | Caller | Transaction |
| --- | --- | --- |
| Just before a scan closes (success or failure) | `app.Scans` → `ScanIndexStore.RebuildFolderIndex` | A transaction for the rebuild only |
| A media folder is added, replaced or removed | `SettingsStore` | The same transaction as that change |
| An override is set or removed, or a group is turned into a tag | `FolderGroupStore` | The same transaction as that change |
| At startup, when `version` or `search_version` differs from the current value, `stale = 1`, or there is no row | `cmd/mdm` → `ScanIndexStore.RefreshFolderIndex` | A transaction for the rebuild only. After `RefreshSearchKeys`, before HTTP and the workers start |
| At startup, when a scan interrupted by the previous shutdown is closed (`FailInterruptedScans` closes 1 or more) | `app.Scans.RecoverInterrupted` → `ScanIndexStore.RebuildFolderIndex` | A transaction for the rebuild only |

During a scan (before it closes), videos added appear as single videos, and
groups whose videos disappeared appear with the remaining members. The rebuild
before the scan closes produces the correct shape. When a rebuild fails, its
transaction rolls back; a separate transaction writes
`folder_index_state.stale = 1`, the failure is logged, and the scan closes. The
previous index is used until the next rebuild point (including the next
startup) (Structural Decisions 14 in the plan). If writing `stale` also fails,
the next rebuild from a scan, an override change or a media folder change fixes
it. If the process stops in the middle of a rebuild transaction, the scan stays
unclosed, so startup's interrupted scan recovery rebuilds. Paging across a
rebuild can duplicate or skip entries, as during import in 013.

## Folder-derived tags

- **Folder names**: for **every** location of a video under its registered
  folder, take the segments below the registered folder and above the file
  (including the group's folder) and apply `domain.NormalizeTagName`. Names
  that produce an error are dropped. Each video's names are written to
  `video_folder_names` without duplicates (requirement 9, Edge Case
  `同じ内容が複数の場所にある`).
- **Attached tags**: the `tag_names.tag_id` of rows with
  `video_folder_names.name = tag_names.name`. Both original names and synonyms
  match. Matching is exact, as the `tag_names` primary key ([014 data-model.md, Name rules](../014-video-tags/data-model.md#name-rules) and [Name lookup](../014-video-tags/data-model.md#name-lookup)).
- **A video's tags** = tags attached by hand (`video_tags` joined on
  `videos.content_key`) ∪ folder-derived tags. A tag attached from both is one
  entry carrying both sources.
- Reads that use this:

| Read | Use |
| --- | --- |
| `TagStore.TagsByContentKeys` (`Video.tags`) | Returns tags with their sources |
| Tag filter ([014 data-model.md, Tag filter](../014-video-tags/data-model.md#tag-filter)) | Each condition's `exists` becomes an OR of "a hand-attached row" or "a folder name row" |
| Tag name matching in the search box ([014 data-model.md, Matching tag names in the search box](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box)) | Whether a tag with the name key is attached to the video is, likewise, an OR of the two sources |
| Per-tag counts ([014 data-model.md, Video counts](../014-video-tags/data-model.md#video-counts)) | Counts videos currently in the library with the tag from either source |
| Selection summary | `count` counts either source; `manualCount` counts only hand-attached |

- Removal (`DetachTag`) deletes only from `video_tags` and needs no change
  (requirement 13).
- The registered folder's own name is not a folder name (requirement 9, "below
  the root"). So even when `直下をまとめる` makes the registered folder itself a
  group, it cannot be turned into a tag (409,
  [contracts/folder-groups-api.md, Turning a group into a tag](contracts/folder-groups-api.md#turning-a-group-into-a-tag)):
  creating the tag would not produce requirement 11's result (the videos inside
  get the tag).
- Turning a group into a tag (requirement 11) is one `FolderGroupStore`
  transaction: pass the folder name through `NormalizeTagName` (write nothing on
  error), use the tag found by name or synonym or create a new one, write
  `ungroup` for the folder, and run the rebuild of [When the index is rebuilt](#when-the-index-is-rebuilt).

## Library items

The query of `GET /api/library`. It adds a step that combines videos into
entries to 013's flow (scope and search expression → `chosen` → filters →
keyset).

1. **Per-member filtering**: join `videos` to `chosen` (scope: the library,
   search expression) and apply playability and the tag AND (the OR of sources
   from [Folder-derived tags](#folder-derived-tags)). Videos that pass are "matched videos". A single video must satisfy
   every condition (requirement 19).
2. **Combining into entries**: matched videos with a `folder_group_members` row
   go into that group; the rest become video entries. A group is one entry when
   at least one of its members matched.
3. **Group aggregates**: computed from **every member** of the group, matched
   or not.

   | Value | Video entry | Group entry |
   | --- | --- | --- |
   | Date added | `videos.added_at` | Maximum over members |
   | Date modified | `mtime` of the `chosen` location | Maximum `mtime` of the members' representative locations |
   | Last played | `playback_progress.updated_at` | Maximum over members (NULL when none) |
   | Title | `title_key` of the `chosen` location | `folder_groups.title_key` |
   | Duration | `videos.duration_ms` | Sum over members with a known duration. NULL when none is known |
   | File size | `size_bytes` of the `chosen` location | Sum of `size_bytes` of the members' representative locations |
   | Watch state | The current `watchCondition` | [Group watch state and the member to open](#group-watch-state-and-the-member-to-open) |

4. **Per-entry filtering**: the watch state filter applies to the watch state
   above (requirement 19).
5. **Sorting and keyset**: sort values are the values in the table above. The
   tie-breaker and the keyset `id` are the video's `id` for a video entry, and
   for a group entry the `id` of the remaining member with the smallest
   `position`. Members do not overlap, so entries do not either. The shuffle key
   is `vv_shuffle_key(seed, that id)`. The cursor shape is the same as 013's.
6. **Count**: the number of entries that passed step 4 (requirement 20).
7. **Select all**: the video `id`s of entries that passed step 4, plus the
   `id`s of every member of those groups (requirement 21).

## Group watch state and the member to open

A function in `internal/domain` decides both from the ordered members' playback
records (none, position, completed). A single video is classified as in
`domain.ClassifyWatch`.

| Value | Rule |
| --- | --- |
| **Watch state** | `unwatched` when no member has started (position above 0 or completed); `watched` when every member is completed; otherwise `inProgress`. The **watched count** is the number of completed members (requirement 17) |
| **Member to open** | In order, the first member with a position above 0 that is not completed; else the first member not completed; when all are completed, the first member (requirement 23) |

- A test checks, with the same inputs, that this matches the watch state of the
  entry SQL (step 4 of [Library items](#library-items)).

## Visibility per audience

The index (from [Migration](#migration) to [When the index is rebuilt](#when-the-index-is-rebuilt)) does not depend on the audience: one index is built from
every location as the owner sees them. Guest responses apply 016's public
condition (`visibleLocationCondition`,
[016 data-model.md, Audience and the public video condition](../016-single-account-auth/data-model.md#audience-and-the-public-video-condition))
to members at read time.

- **Group members**: guests count only public members. `videoCount`,
  `videoIds`, `previews`, the duration and size sums, the maximum dates added
  and modified, `position` and `count` of `Video.group`, `group.items` of
  related videos, and previous/next within the group are all built from public
  members only. A group with exactly one public member is shown to guests as a
  video entry (no one-video group card). A group with none is not shown.
- **Watching and tags**: guests get no playback position and no tags ([016
  guest-api.md, What guests see](../016-single-account-auth/contracts/guest-api.md#what-guests-see)). `LibraryGroup`'s `watchedCount`, `watchState` and
  `lastPlayedAt` are omitted and `tags` is empty. `openVideoId` is the first
  public member in order. Folder-derived tags are likewise not used in guest
  responses, filters or search (the `tag` condition is 400 for guests, and
  search does not match tag names).
- **Overrides and grouping decisions**: which folders are groups is decided from
  all of the owner's locations ([Assignment rules](#assignment-rules)). Guests receive only that result narrowed to
  public members; the number or names of non-public videos are never exposed.
- `GET /api/folders/{rootId}/group`: 404 for guests unless at least two members
  are public.
