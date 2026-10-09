# Data model: Watch history screen

Parent Issue: #792. The rest of the model is unchanged. Sources of truth:

| Topic | Source |
| --- | --- |
| Existing table definitions | [internal/store/migrations/](../../internal/store/migrations/) |
| Data classes | [running-vv.md, Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery) |
| Playback position | [internal/store/progress.go](../../internal/store/progress.go), [internal/domain/progress.go](../../internal/domain/progress.go) |
| User key, succession, bundles | [030 data-model, User key](../030-video-versions/data-model.md#user-key), [Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| Effective title | [029 data-model, Values added to `domain`](../029-video-overrides/data-model.md#values-added-to-domain) |

This file covers only the table, values and read and write rules this feature
adds.

## Migration

`00034_watch_history.sql` (the last migration on `main` is
`00033_scan_origin.sql`; confirm the number when implementing) creates the
table and backfills it from `playback_progress`
([research.md R-1](research.md#r-1-a-watch_history-table-keyed-by-content-key-with-a-title-snapshot),
[R-4](research.md#r-4-existing-playback-records-are-backfilled-by-the-migration)).

```sql
create table watch_history (
    id          integer primary key autoincrement,
    content_key text    not null check (content_key <> ''),
    -- The client's id for one viewing (R-2). Null for backfilled rows.
    playback_id text    unique,
    -- The title the video page showed when the viewing started.
    title       text    not null,
    -- Unix milliseconds of the first save of this viewing (R-3).
    played_at   integer not null
);
create index watch_history_played_idx on watch_history (played_at desc, id desc);
create index watch_history_content_idx on watch_history (content_key);
```

The backfill inserts one row per `playback_progress` row:

| Column | Value |
| --- | --- |
| `content_key` | The record's key; a `bundle:<id>` key becomes that bundle's `representative_key`. A record whose key resolves to nothing is skipped |
| `title` | `coalesce(video_overrides.display_name, <representative location's title>, '')` for the video with that content key at migration time |
| `played_at` | `playback_progress.updated_at * 1000` (the record is in seconds) |
| `playback_id` | Null |

Down drops the table.

## `watch_history`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer | no | Entry id; the delete target and the tie-breaker of the order |
| `content_key` | text | no | The content that was played; never empty |
| `playback_id` | text | yes | The playback id of [R-2](research.md#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id); unique, so a second save with the same id adds nothing |
| `title` | text | no | Snapshot of the effective title when the entry was written; shown when the content is not in the library |
| `played_at` | integer | no | Unix milliseconds; the server's clock at the entry's first save |

**Relationships**: None. No foreign key to `videos` or `playback_progress`.

Class: user data that cannot be rebuilt. Added to the lists in ARCHITECTURE.md
("User data survives a rebuild") and running-vv.md ("Data and recovery").
Generated-file cleanup, `releaseContentIndex`, `rebuildFolderIndex` and media
folder removal do not touch it.

Succession and bundling:

| Path | Change |
| --- | --- |
| Same-path succession (`moveUserData`) | `update watch_history set content_key = <new> where content_key = <old>`, in the same transaction. Unlike the other tables there, rows already on the new key stay: both viewings are history |
| Bundle, change representative, remove (`userDataTables`) | Not added. Entries keep the content key of the version played ([R-1](research.md#r-1-a-watch_history-table-keyed-by-content-key-with-a-title-snapshot)) |

## `domain` values added

| Value | Content |
| --- | --- |
| `Play` | What a progress save says about the viewing: `PlaybackID string`, `ContentKey string`, `Title string`. `ValidatePlaybackID` accepts the 36-character RFC 4122 text form (`8-4-4-4-12` hexadecimal) and nothing else |
| `WatchHistoryEntry` | `ID int64`, `PlayedAt time.Time`, `Title string`, `Video *Video` (nil when the content is not in the library) |
| `WatchHistoryPage` | `Items []WatchHistoryEntry`, `NextCursor string` (empty when there is no more) |
| Cursor | `played_at` and `id` of the last item, encoded and decoded like the scan issue cursor; an unreadable cursor is `ErrInvalidCursor` |

## Rules

| Rule | Enforced in |
| --- | --- |
| A save without a playback id writes no entry; a save with one writes at most one entry per id | `PlaybackStore.SaveProgress` (`insert or ignore`), `watch_history.playback_id unique` |
| A save whose video has an empty `content_key` writes no entry | `PlaybackStore.SaveProgress`, `check (content_key <> '')` |
| The entry's time and title are fixed at its first save | `insert or ignore`: later saves change nothing |
| Deleting entries changes no row of any other table (requirement 9) | `PlaybackStore.DeleteWatchHistoryEntry`, `ClearWatchHistory` |
| An entry resolves to a video only when a video with its content key has a location the owner may open | `PlaybackStore.ListWatchHistory` (`visibleVideoCondition` for the owner) |

## Store operations (`PlaybackStore`)

`PlaybackStore` keeps holding only the shared SQLite connection; it reads
videos through the package-private column helpers (`videoColumns`,
`scanVideo`) that every role uses. The history stays on this role because the
entry is written in the position's transaction, and one table under two roles
would split one business operation.

| Operation | Behaviour |
| --- | --- |
| `SaveProgress(ctx, userKey, progress, play *domain.Play)` | As today, plus, when `play` is not nil and `play.ContentKey` is not empty, `insert or ignore into watch_history (content_key, playback_id, title, played_at)` with the current time in milliseconds, in the same transaction |
| `ListWatchHistory(ctx, cursor string, limit int)` | The page after `cursor` in `(played_at desc, id desc)` order, `limit` items, each with the owner's `Video` when present. `NextCursor` is set when a further row exists |
| `DeleteWatchHistoryEntry(ctx, id int64) (bool, error)` | Deletes the row; false when it did not exist |
| `ClearWatchHistory(ctx) error` | Deletes every row |

No domain event is published
([research.md R-6](research.md#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads)).

## What does not change

`playback_progress`, its key, `EvaluateProgress`, `ResumePosition`, the watch
state, `lastPlayedAt` and the `played*` sorts read nothing from
`watch_history`, so deleting history leaves the card's progress bar, its watch
state and its place in "Last played" as they were (requirement 9). The
`Progress` response of the save does not change.
