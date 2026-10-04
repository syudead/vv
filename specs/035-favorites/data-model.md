# Data model: Favorite videos and groups

Parent Issue: #574. The rest of the model is unchanged. Sources of truth:

| Topic | Source |
| --- | --- |
| Existing table definitions | [internal/store/migrations/](../../internal/store/migrations/) |
| Data classes | [running-vv.md, Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery) |
| User key | [specs/030-video-versions/data-model.md, User key](../030-video-versions/data-model.md#user-key) |
| Folder key | [specs/017-folder-groups/data-model.md, Migration](../017-folder-groups/data-model.md#migration) |

This file covers only the tables, values and read and write rules this feature adds. Tables not named here do
not change.

## Migration

`00029_favorites.sql` (the last migration on `main` is `00028_video_file_created_at.sql`; confirm the number
when implementing):

```sql
-- Favorites (specs/035-favorites/research.md R-1). Both are user data that cannot be rebuilt
-- and must survive rescans, media folder changes and folder index rebuilds.
-- No foreign keys to videos, folder_groups or media_folders.

-- Video favorites. The key is the user key ('bundle:<id>' for a bundle member, otherwise
-- content_key; the same as public_videos).
create table video_favorites (
    content_key  text    primary key,
    favorited_at integer not null
) without rowid;

-- Group favorites. The key is the folder's absolute path normalised by domain.FolderKey
-- (the same as folder_group_overrides.path).
create table folder_favorites (
    path         text    primary key,
    favorited_at integer not null
) without rowid;
```

Down drops both tables.

Invariants (added to `internal/store/invariants_test.go`):

| Rule | Enforced in |
| --- | --- |
| `video_favorites` has no row with an empty `content_key` | `internal/store/invariants_test.go` |
| `folder_favorites.path` equals `domain.FolderKey(path)` (only normalised keys are written) | `internal/store/invariants_test.go` |

Class: both are user data that cannot be rebuilt, added to the list in ARCHITECTURE.md. Generated-file cleanup
(`RemoveContent`), `releaseContentIndex`, `rebuildFolderIndex` and media folder removal touch neither table.

Succession and bundling:

| Path | Change |
| --- | --- |
| `moveUserData` (same-path succession, [030, Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path)) | Add `video_favorites` to its table list. A row on `from` replaces the row on `to` |
| `userDataTables` (bundle, change representative, remove, [030, Store operations (`VersionStore`)](../030-video-versions/data-model.md#store-operations-versionstore)) | Add `{"video_favorites", "favorited_at"}`. Bundling copies the representative's value to the bundle key; a removed member returns to the value under its own key |
| `folder_favorites` | No path copies it. Renaming or moving a folder drops its favorite, as required (Edge Case) |

## `domain` values added

| Value | Content |
| --- | --- |
| `Video.Favorite` | `bool`. Whether the user key is in `video_favorites`. Filled by every read that returns videos ([Reads and lists](#reads-and-lists)) |
| `LibraryGroup.Favorite` | `bool`. Whether the group folder's key is in `folder_favorites`. Filled by `loadGroups` |
| `VideoQuery.FavoriteOnly`, `FolderVideoQuery.FavoriteOnly` | `bool`. Limit to favorites ([Reads and lists](#reads-and-lists)) |
| `SortFavoritedAsc` / `SortFavoritedDesc` | `VideoSort` values `favoritedAsc` / `favoritedDesc`. Added to `Valid` |
| `FavoriteChange` | Input to `FavoriteStore.SetFavorites`: `VideoIDs []int64`, `FolderPaths []string` (absolute paths), `Favorite bool` |
| `FavoriteApplied` | Result: `Videos int` (the number of distinct ids in `VideoIDs` whose key resolved; ids in the same bundle each count), `Folders int` (the number of folders written because they are groups now) |
| `Audience.CheckVideoQuery` | For a guest, `FavoriteOnly` and `favorited*` return `ErrGuestQueryNotAllowed` ([research.md R-5](research.md#r-5-favorites-hidden-from-guests-filter-and-sort-return-400)) |

`favorited_at` is not exposed in `domain`. Only the store's SQL uses it for sorting, and responses do not carry
it ([contracts/screen-api.md, Fields added to `Video` and `LibraryGroup`](contracts/screen-api.md#fields-added-to-video-and-librarygroup)).

## Read columns

The reads that return videos (`videoColumnsTemplate`, `listColumns`; the external API's `readVideosByIDs` uses
`videoColumns` but does not copy the value into `ExternalVideo`) gain one column, which `scanVideo` copies to
`Video.Favorite`.

| Column | Expression |
| --- | --- |
| `favorite` | `exists (select 1 from video_favorites fav where fav.content_key = <userKeyExpr("videos")> and videos.content_key <> '')` (the same shape as `publicColumn`) |

For groups, `loadGroups` adds `left join folder_favorites ff on ff.path = g.path_key` to `folder_groups` and
copies `ff.path is not null` to `LibraryGroup.Favorite`. `FolderGroup` (the refetch) uses the same path and
returns the same value.

## Writes (`FavoriteStore`)

New role type `FavoriteStore struct{ sql *sql.DB }` (`db.Favorites()`). Like `VisibilityStore`, it holds only
the shared SQLite connection and depends on no index type, no notification and no other role's public method.
It publishes no domain event
([research.md R-6](research.md#r-6-no-domain-event-screens-reuse-the-visibility-subscription-pattern)).

`SetFavorites(ctx, change domain.FavoriteChange) (domain.FavoriteApplied, error)` does the following in one
transaction. A failure partway leaves nothing behind.

1. Resolve `change.VideoIDs` with `userKeysForVideoIDs` to the user keys of videos currently in the library
   (one per bundle; ids that do not resolve and empty `content_key` are excluded). `Videos` is not the number
   of keys but the number of distinct ids whose key resolved (`count(distinct v.id)` under the same conditions
   as `userKeysForVideoIDs`, in the same transaction). Sending two ids of the same bundle gives 2, so the
   screen does not mistake "fewer than the distinct ids sent" for a partial result (the same comparison as
   `api/visibility.ts`, [contracts/screen-api.md, `PUT /api/favorites`](contracts/screen-api.md#put-apifavorites)).
2. Turn each path in `change.FolderPaths` into `domain.FolderKey` and keep only those in
   `folder_groups.path_key` (folders that are groups for the owner now). The number kept is `Folders`.
   Duplicates of the same folder count once.
3. When `Favorite` is true, `insert or ignore into video_favorites (content_key, favorited_at)` and
   `insert or ignore into folder_favorites (path, favorited_at)` (existing rows keep their time); when false,
   `delete` from each. Each table is written with one statement over `json_each` regardless of the number of
   keys (the same shape as `touchEditedAt`).
4. Do not touch `video_edits` ([research.md R-8](research.md#r-8-favoriting-does-not-advance-video_edits)).

`favorited_at` is Unix milliseconds. The transaction computes
`max(<current time>, <largest favorited_at in both tables> + 1)` once and uses it for every target in that
transaction. SQLite serialises write transactions, so a later toggle always gets a larger value. "The last one
favorited comes first" and "unfavorite and favorite again to move to the top" then hold without relying on the
`id` tie-breaker, even for consecutive actions in the same second or millisecond and when the clock goes back.
The maximum is taken over both tables because library items sort videos and groups on the same column.

## Reads and lists

### Video lists (`ListVideos`, `ListFolderVideos`, `CountVideos`)

Add `left join video_favorites fav on fav.content_key = <userKeyExpr("videos")> and videos.content_key <> ''`
to `filteredFrom`. With `FavoriteOnly`, add the condition `fav.content_key is not null`. Add the following
sort in the same shape as [the 013 table](../013-library-search/contracts/list-api.md#videosort-values).

| Sort | Ascending | Descending | Value | When there is no value |
| --- | --- | --- | --- | --- |
| Date favorited | `favoritedAsc` | `favoritedDesc` | `fav.favorited_at` | NULL; last in either direction (`nullable`) |

### Library items (`libraryItemsCTE`, `ListLibrary`, `LibraryIDs`)

`matched`, `gm`, `live`, `hits`, `whole` and `mv` from
[027, How `GET /api/library` builds items](../027-partial-group-search/contracts/library-api.md#how-get-apilibrary-builds-items) do not change.
`items` changes as follows (`favoriteOnly` is `FavoriteOnly`, `gf(group_id)` is the groups whose folder is in
`folder_favorites`, and `vf(video_id)` is the videos whose key is in `video_favorites`).

| Item | Current condition | Added condition |
| --- | --- | --- |
| Group item | Groups in `whole` (playable when any member is) | `not favoriteOnly or group_id in gf` |
| Video item | Matched videos that do not belong to a `whole` group (playable when the video is) | With `favoriteOnly`: also an item when its group is in `whole` but not in `gf`; and `video_id in vf` |

Add a `favorited_at` column to `items`: `video_favorites.favorited_at` for a video item,
`folder_favorites.favorited_at` for a group item, NULL otherwise. `favoritedAsc` and `favoritedDesc` in
`itemOrderValues` use this column, `nullable` as in `listOrders`. Equal values are settled by `id`.

Watch state, playability, `total`, the keyset and `LibraryIDs` apply to these `items` as before.

`LibraryIDs` returns the ids of video items separately from each group item's folder path and member ids
(`domain.LibrarySelection{VideoIDs []int64; Groups []LibraryGroupSelection{Path string; VideoIDs []int64}}`).
`internal/httpapi` builds the existing `ids` (the union of all) and the new `groups`
([contracts/screen-api.md, Fields added to `GET /api/library/ids`](contracts/screen-api.md#fields-added-to-get-apilibraryids)).

Mapping to the acceptance criteria:

| Acceptance criterion | `favorite=true` list |
| --- | --- |
| 4 (only A is a favorite; G is not) | G is not in `gf`, so it is not a group item; A is in `vf`, so it is a video item. G's other members are not listed |
| 5 (G is a favorite) | G is in `whole` (no filter) and in `gf`, so it is one group item. Its members are not video items |
| 9 (with a tag) | The tag applies to `matched`. A group G with only some members matching is not in `whole`, so the matching members in `vf` are video items |

### Guests

`CheckVideoQuery` returns `400`, so `FavoriteOnly` and `favorited*` never reach the SQL of a guest list. The
`Video.Favorite` and `LibraryGroup.Favorite` columns are read, and `internal/httpapi` omits them from guest
responses.
