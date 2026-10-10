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
adds. The sections marked *revision* were added with requirements 12 to 18
([research.md, Revision](research.md#revision-filter-search-date-jump-and-resume-actions)).

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
| `content_key` | The record's key, whether or not a video has it at migration time; a `bundle:<id>` key becomes that bundle's `representative_key`. Only a `bundle:<id>` key without a `video_bundles` row is skipped ([R-4](research.md#r-4-existing-playback-records-are-backfilled-by-the-migration)) |
| `title` | `coalesce(video_overrides.display_name, <representative location's title>, '')` for that content key at migration time; `video_overrides` is keyed by content, so a removed video keeps its display name, else the title is empty |
| `played_at` | `playback_progress.updated_at * 1000` (the record is in seconds) |
| `playback_id` | Null |

Down drops the table.

`00035_watch_history_title_key.sql` (*revision*; confirm the number when
implementing) adds the search column of
[R-10](research.md#r-10-title-search-uses-the-librarys-query-syntax-on-the-title-alone):

```sql
alter table watch_history add column title_key text;
```

SQL cannot fold a title ([013 data-model, `search_key` rules](../013-library-search/data-model.md#search_key-rules)),
so the migration leaves the column null and `PlaybackStore.RefreshWatchHistoryTitleKeys`
fills every null row with the title's match form at startup, right after
migrations and before the listener opens, where `cmd/mdm` already refreshes the
location and tag search keys. The match form is the one the library uses for a
title line of `search_key` (`searchKeyPart` in `internal/store/search_keys.go`):
`FoldForMatch(title)` with every newline replaced by a space. The library's
condition reads a search term the same way, so a phrase matches a title that
contains a newline both in `search_key` and in `title_key`. Down drops the column.

## `watch_history`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer | no | Entry id; the delete target and the tie-breaker of the order |
| `content_key` | text | no | The content that was played; never empty |
| `playback_id` | text | yes | The playback id of [R-2](research.md#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id); unique, so a second save with the same id adds nothing |
| `title` | text | no | Snapshot of the effective title when the entry was written; shown when the content is not in the library |
| `played_at` | integer | no | Unix milliseconds; the server's clock at the entry's first save |
| `title_key` | text | yes | *Revision.* The title's match form, `FoldForMatch(title)` with newlines as spaces ([Migration](#migration)); the search target of an entry without a video. Null only between the migration and the startup fill |

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
| `WatchHistoryFilter` (*revision*) | `all`, `inProgress`, `watched`; the values of the `WatchHistoryFilter` schema. `ParseWatchHistoryFilter` rejects anything else |
| `WatchHistoryQuery` (*revision*) | The conditions of one list or dates read: `Filter WatchHistoryFilter`, `Search SearchExpr` (from `ParseSearchQuery`), `Before time.Time` (zero when no `date` was given) |
| `WatchHistoryPeriod` (*revision*) | A `date` parameter: `YYYY-MM-DD` or `YYYY-MM`. `ParseWatchHistoryPeriod` accepts those two forms and nothing else; `End(loc)` is the first instant after the day or month in `loc` |

## Rules

| Rule | Enforced in |
| --- | --- |
| A save without a playback id writes no entry; a save with one writes at most one entry per id | `PlaybackStore.SaveProgress` (`insert or ignore`), `watch_history.playback_id unique` |
| A save whose video has an empty `content_key` writes no entry | `PlaybackStore.SaveProgress`, `check (content_key <> '')` |
| The entry's time and title are fixed at its first save | `insert or ignore`: later saves change nothing |
| Deleting entries changes no row of any other table (requirement 9) | `PlaybackStore.DeleteWatchHistoryEntry`, `ClearWatchHistory` |
| An entry resolves to a video only when a video with its content key has a location the viewer may open | `PlaybackStore.ListWatchHistory` (`visibleVideoCondition` with the `domain.Audience` it is given, as every store read returning videos takes one: ARCHITECTURE.md, "Every read knows its viewer") |
| *Revision.* `inProgress` and `watched` keep only entries with a video whose `playback_progress` row, joined by `userKeyExpr`, satisfies `watchCondition`; `all` keeps every entry ([R-9](research.md#r-9-the-state-filter-reads-the-videos-current-watch-state-with-the-librarys-rule)) | `PlaybackStore.ListWatchHistory`, `ListWatchHistoryDays` |
| *Revision.* A search term matches an entry with a video when the title line or the display-name line of `search_key` of one of its registered locations contains the folded term; an entry without a video when `title_key` contains it; AND, OR and NOT as in the library ([R-10](research.md#r-10-title-search-uses-the-librarys-query-syntax-on-the-title-alone)) | `PlaybackStore.ListWatchHistory`, `ListWatchHistoryDays` |
| *Revision.* A `date` keeps the entries with `played_at` before `WatchHistoryPeriod.End` in the request's zone ([R-11](research.md#r-11-the-date-list-and-the-jump-are-computed-on-the-server-in-the-viewers-time-zone)) | `PlaybackStore.ListWatchHistory` |
| *Revision.* `title_key` is written with the entry and never updated; the startup fill touches only null rows | `PlaybackStore.SaveProgress`, `RefreshWatchHistoryTitleKeys` |

## Store operations (`PlaybackStore`)

`PlaybackStore` keeps holding only the shared SQLite connection; it reads
videos through the package-private column helpers (`videoColumns`,
`scanVideo`) that every role uses. The history stays on this role because the
entry is written in the position's transaction, and one table under two roles
would split one business operation.

| Operation | Behaviour |
| --- | --- |
| `SaveProgress(ctx, userKey, progress, play *domain.Play)` | As today, plus, when `play` is not nil and `play.ContentKey` is not empty, `insert or ignore into watch_history (content_key, playback_id, title, title_key, played_at)` with the current time in milliseconds and the match form of `play.Title` ([Migration](#migration)), in the same transaction |
| `ListWatchHistory(ctx, audience domain.Audience, query domain.WatchHistoryQuery, cursor string, limit int)` | The page after `cursor` in `(played_at desc, id desc)` order among the entries that satisfy `query` (*revision*: the filter, the search and `Before`, applied in the same SQL statement before the limit, with the video joined by content key under `visibleVideoCondition` and its progress by `userKeyExpr`), `limit` items, each with the `Video` that `audience` may open when present (the handler passes the audience the boundary classified, which on these owner-only routes is the owner). `NextCursor` is set when a further row exists |
| `ListWatchHistoryDays(ctx, audience domain.Audience, query domain.WatchHistoryQuery, loc *time.Location) ([]string, error)` (*revision*) | The distinct calendar days in `loc`, as `YYYY-MM-DD`, newest first, of the entries that satisfy `query` (its `Before` is ignored). Reads `played_at` of the matching rows and groups in Go ([R-11](research.md#r-11-the-date-list-and-the-jump-are-computed-on-the-server-in-the-viewers-time-zone)) |
| `RefreshWatchHistoryTitleKeys(ctx) (int, error)` (*revision*) | Fills `title_key` of every row where it is null and returns the count; called at startup by `cmd/mdm` |
| `DeleteWatchHistoryEntry(ctx, id int64) (bool, error)` | Deletes the row; false when it did not exist |
| `ClearWatchHistory(ctx) error` | Deletes every row |

No domain event is published
([research.md R-6](research.md#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads)).

## What does not change

`playback_progress`, its key, `EvaluateProgress`, `ResumePosition`, the watch
state, `lastPlayedAt` and the `played*` sorts read nothing from
`watch_history`, so deleting history leaves the card's progress bar, its watch
state and its place in "Last played" as they were (requirement 9). The
`Progress` response of the save does not change. The revision stores no watch
state and no position on the entry: the filter and the position bar read the
video's current `playback_progress` row at request time
([R-9](research.md#r-9-the-state-filter-reads-the-videos-current-watch-state-with-the-librarys-rule),
[R-13](research.md#r-13-the-resume-and-restart-actions-open-the-video-page-with-autoplay-and-the-existing-resume-rule-decides-the-position)).
