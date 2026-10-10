# Contract: Screen API changes

Parent Issue: #792. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml).
This file lists only the field, routes and schemas added. `task generate`
regenerates `internal/httpapi/gen/` and `web/src/api/gen/`. The external API
(`api/external-v1.yaml`) and MCP do not change (out of scope). The parts marked
*revision* were added with requirements 12 to 18
([research.md, Revision](../research.md#revision-filter-search-date-jump-and-resume-actions)).

All four routes are owner-only by the `/api/*` default
([research.md R-7](../research.md#r-7-watch-history-is-one-more-owner-only-screen-and-route-with-the-existing-gate-rules)),
so `accessRoutes` gains nothing and `TestAccessClassificationMatchesOpenAPISecurity`
keeps passing with the operations' default `security`.

## `playbackId` on `PUT /api/videos/{id}/progress`

The save gains an optional field
([research.md R-2](../research.md#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id)).

```yaml
ProgressUpdate:
  required: [positionMs]
  properties:
    positionMs: { type: integer, format: int64, minimum: 0 }
    playbackId:
      type: string
      minLength: 36
      maxLength: 36
      description: |
        The client's id for this viewing (RFC 4122 text form). The first save
        with an id adds one watch history entry for the video; later saves with
        the same id add nothing. Absent before playback has started.
```

| Condition | Response |
| --- | --- |
| `playbackId` absent | As today; no entry |
| `playbackId` present and the video has a `content_key` | `200 Progress` as today; the entry exists after the response, with the video's effective title and the server's time |
| `playbackId` present but not in the 36-character form | `400 invalid_request` |
| The video has an empty `content_key` | `200 Progress`; no entry |

The `Progress` response does not change. The body limit (1 KiB) still holds.

## `GET /api/watch-history`

Returns the owner's watch history, newest first
([data-model.md, Store operations](../data-model.md#store-operations-playbackstore)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `cursor` | query | string | no | The previous response's `nextCursor`; opaque |
| `limit` | query | integer, 1 to 200 | no | Items per page; default 60 |
| `watch` | query | `WatchHistoryFilter` | no | *Revision.* `all` (default), `inProgress` or `watched`: the video's current watch state ([R-9](../research.md#r-9-the-state-filter-reads-the-videos-current-watch-state-with-the-librarys-rule)) |
| `query` | query | string, at most 100 characters | no | *Revision.* The library's query syntax ([013 list-api, Query syntax](../../013-library-search/contracts/list-api.md#query-syntax)), matched against the title alone ([R-10](../research.md#r-10-title-search-uses-the-librarys-query-syntax-on-the-title-alone)); no term left means no filtering |
| `date` | query | string, `YYYY-MM-DD` or `YYYY-MM` | no | *Revision.* Keep the entries played before the end of that day or month in `tz` ([R-11](../research.md#r-11-the-date-list-and-the-jump-are-computed-on-the-server-in-the-viewers-time-zone)); the first page then starts at that date and `nextCursor` continues into older entries |
| `tz` | query | string, an IANA zone name | with `date` | *Revision.* The zone the day and month boundaries are taken in |

The conditions combine with `and`. `cursor` works under any combination, and
the screen sends the same `watch`, `query`, `date` and `tz` with every page of
one view.

```yaml
WatchHistoryFilter:            # revision
  type: string
  enum: [all, inProgress, watched]
  default: all
```

**Response**: `200 WatchHistoryPage`.

```yaml
WatchHistoryPage:
  required: [items]
  properties:
    items:      { type: array, items: { $ref: WatchHistoryEntry } }
    nextCursor: { type: string }   # present only when more entries follow

WatchHistoryEntry:
  required: [id, playedAt, title]
  properties:
    id:       { type: integer, format: int64 }
    playedAt: { type: string, format: date-time }   # when the viewing started
    title:    { type: string }                       # the title at that time; may be empty
    video:
      $ref: Video   # the video now in the library with the played content;
                    # absent when there is none (not playable from the history)
```

`video` is the same shape as a list entry, with `progress`, `tags` and
`favorite` filled as for the owner, so the screen can open it and show its
state. Requirement 16's position bar reads `video.progress.positionMs` and
`video.durationMs` from it, so the revision adds no field to the entry
([R-13](../research.md#r-13-the-resume-and-restart-actions-open-the-video-page-with-autoplay-and-the-existing-resume-rule-decides-the-position)). When present, `video.title` is the current title and the screen shows
it; `title` is for entries without a video. A backfilled entry whose
content had left the library and had no display name has an empty `title`;
`ui-design.md` decides the text shown for it.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | `limit` out of range; *revision*: `watch` not one of the three values, `query` over 100 characters, `date` not in one of the two forms, `date` without `tz`, or `tz` not a zone the server knows |
| `400` | `invalid_cursor` | `cursor` cannot be read |

## `GET /api/watch-history/dates`

*Revision.* Returns the days that have entries under the given filter and
search, for the screen's date list
([R-11](../research.md#r-11-the-date-list-and-the-jump-are-computed-on-the-server-in-the-viewers-time-zone)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `tz` | query | string, an IANA zone name | yes | The zone whose calendar days are counted |
| `watch` | query | `WatchHistoryFilter` | no | As on the list; the days of the entries that filter keeps |
| `query` | query | string, at most 100 characters | no | As on the list |

**Response**: `200 WatchHistoryDates`.

```yaml
WatchHistoryDates:
  required: [days]
  properties:
    days:
      type: array           # newest first; empty when nothing matches
      items: { type: string, pattern: '^\d{4}-\d{2}-\d{2}$' }
```

The screen derives the months from the days and decides where days end and
months begin (`ui-design.md`). Each day is a valid `date` for the list, and so
is its month.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | `tz` missing or unknown, `watch` not one of the three values, `query` over 100 characters |

## `DELETE /api/watch-history/{id}`

Deletes one entry
([research.md R-6](../research.md#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `id` | path | integer, minimum 1 | yes | `WatchHistoryEntry.id` |

**Response**: `204` with no body.

| Status | `code` | When |
| --- | --- | --- |
| `404` | `not_found` | No entry has that id (already deleted elsewhere) |

## `DELETE /api/watch-history`

Deletes every entry. **Response**: `204` with no body, also when the history
was already empty. The confirmation is the screen's.

## Client use

| Change | Detail |
| --- | --- |
| `saveProgress(id, positionMs, playbackId?)` and `beaconProgress(id, positionMs, playbackId?)` in `web/src/api/client.ts` | Send `playbackId` when given. The per-video send queue and `recordSavedProgress` do not change |
| `useProgressSaving` (`web/src/player/useProgressSaving.ts`) | Holds the playback id for the current video id. `markPlayed()` creates it on the first `play`, from `crypto.getRandomValues()` in the RFC 4122 version 4 form (`crypto.randomUUID()` is missing over plain HTTP); the one immediate save with it is sent once the player's status is `positioned`, at once when it already is, so the save cannot overwrite the resume position before the seek ([R-3](../research.md#r-3-the-entrys-time-is-the-server-time-of-the-first-save-with-its-id)); no save or beacon carries the id before that save. `markEnded()` lets the save at `ended` carry the id and then drops it, so the next `play` starts a new entry. The id is also dropped when the video id changes |
| `VideoPlayer` | Reports the `play` event (`onPlay`) so the page can call `markPlayed()`; the page passes `PlayerStatus.positioned` and `ended` to the hook |
| `web/src/api/history.ts` (new) | `listWatchHistory({ cursor, limit })`, `deleteWatchHistoryEntry(id)`, `clearWatchHistory()`; *revision*: `listWatchHistory` also takes `watch`, `query`, `date` and `tz`, and `listWatchHistoryDates({ watch, query, tz })` is added. `tz` is `Intl.DateTimeFormat().resolvedOptions().timeZone` |
| The history screen | On `204` removes the row; on `404` reloads from the first page without a message; on any other failure keeps the row and shows a toast (`errorText`) |
| The history screen's URL (*revision*) | `/history?watch=inProgress&q=…&date=2026-09`: `watch` (omitted for `all`), `q` (trimmed, cut at 100 code points) and `date`, read and written as [list-url.md](../../013-library-search/contracts/list-url.md) does; an unreadable value is the default ([R-12](../research.md#r-12-the-filter-the-search-and-the-date-live-in-the-screens-url)). The row link's `state.from` is this full URL. Changing the filter, the search or the date reads the first page again with the new conditions and the dates again when the filter or the search changed |
| The resume and restart actions (*revision*) | A `Link` to `/videos/{video.id}` with `state: { from, autoplay: true }`; nothing else is sent ([R-13](../research.md#r-13-the-resume-and-restart-actions-open-the-video-page-with-autoplay-and-the-existing-resume-rule-decides-the-position)). An entry without `video` has no such action |
