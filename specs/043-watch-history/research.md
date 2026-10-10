# Research: Watch history screen

Parent Issue: #792. Inherited decisions:

| Topic | Source of truth |
| --- | --- |
| Boundaries, user data versus the rebuildable index, the authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| Playback position: keyed by the user key, written by `PUT /api/videos/{id}/progress`, watched and resume rules | [internal/store/progress.go](../../internal/store/progress.go), [internal/domain/progress.go](../../internal/domain/progress.go), [internal/httpapi/progress.go](../../internal/httpapi/progress.go) |
| User key and same-path succession | [030 data-model, User key](../030-video-versions/data-model.md#user-key), [Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| Owner-only screens and routes | [016 ui-design, Guest degradation](../016-single-account-auth/ui-design.md#guest-degradation), [016 auth-api, Three access classes](../016-single-account-auth/contracts/auth-api.md#three-access-classes) |
| Opaque keyset cursors | [024 scan-api, `GET /api/scans/current/issues`](../024-import-progress/contracts/scan-api.md#get-apiscanscurrentissues) |

This file records only the decisions this feature adds.

## R-1: A `watch_history` table keyed by content key, with a title snapshot

**Decision**: Add `watch_history` ([data-model.md](data-model.md#migration)):
one row per playback, keyed by the `content_key` of the version that was
played, with the title shown at the time, no foreign key to `videos`, and no
link to `playback_progress`. Same-path succession moves the rows to the new
content key; bundling and unbundling leave them where they are.

| Option | Verdict |
| --- | --- |
| **Own table, `content_key`, title snapshot** | Chosen |
| Derive the history from `playback_progress` | Rejected: one row per content, overwritten on every save, so requirements 2 and 4 (one entry per viewing, several entries for one video) cannot be met |
| Key by the user key (`bundle:<id>` for a bundle member), like `playback_progress` | Rejected: the entry would no longer say which version was played, and dissolving a bundle could not give its rows back to the members. A member opened by its own id still plays (`GET /api/videos/{id}` returns every version), so the "opens the current video" edge case holds without the bundle key |
| Foreign key to `videos`, cascade on delete | Rejected: a video row is deleted when its last location goes; the Edge Case keeps the entry |

**Rationale**: The entry answers "which content, when". The content key is
the value a rescan reproduces, so a file that comes back makes the entry
playable again, and the title snapshot keeps the entry readable while it is
gone. Deleting history rows touches no other table, which is requirement 9.

## R-2: One entry per playback, identified by a client-generated playback id

**Decision**: `PUT /api/videos/{id}/progress` gains an optional `playbackId`
([contracts/screen-api.md](contracts/screen-api.md#playbackid-on-put-apivideosidprogress)).
The video page creates one id at the first `play` event for a video and sends
it with every save and beacon until the video id changes or the video plays to
its end; saves before the first play carry none. The id is an RFC 4122
version 4 id formatted from 16 bytes of `crypto.getRandomValues()`:
`crypto.randomUUID()` exists only in a secure context, and the owner can open
vv over plain HTTP from another device on the local network
([running-vv.md](../../docs/how-to/running-vv.md)), as `newAttempt` in
`web/src/player/liveOffset.ts` already accounts for. The server inserts the entry
with `insert or ignore` on the unique `playback_id` in the same transaction as
the position, so pauses, resumes and seeks extend nothing and create nothing.
The id lives in the page's progress-saving hook, not in `VideoPlayer`, so a
player remount (error recovery, quality switch) keeps the entry. The save the
player sends at `ended` still carries the id; then the hook drops it, so
**Replay** in the ended overlay, or playing again from the end, starts a new
entry on its `play`. Pauses, seeks and remounts before the end keep the id.

| Option | Verdict |
| --- | --- |
| **Client playback id on the existing save** | Chosen |
| Server-side merging by time gap between saves | Rejected: two tabs playing the same video would merge into one entry (Edge Case), and a pause longer than the gap would split one viewing (acceptance criterion 3) |
| A separate `POST` at the first play | Rejected: one more round trip per play, and after the owner deletes the entry mid-playback the client would have to notice and post again; with the id on every save the next save recreates it, which is the Edge Case's "a new entry" |
| The id per `VideoPlayer` instance | Rejected: recovery and quality switches remount the player, which would split one viewing |
| Keep the id across **Replay** after the end | Rejected: a viewing that reached the end is finished, and watching it again is the repeat viewing requirement 4 counts; keeping the id would fold it into the first entry |
| `crypto.randomUUID()` | Rejected: undefined over plain HTTP on the local network, so no entry would be written from a phone |

**Rationale**: The client is the only party that knows where one viewing
starts and ends (the page), and the server is the only party that must not
trust it for anything but identity: it decides the time and what the entry
points at. Reloading the page starts a new entry; the Issue calls a repeat
viewing "after a while" a separate entry, and a reload or a replay after the
end are the boundaries the page can observe.

## R-3: The entry's time is the server time of the first save with its id

**Decision**: `played_at` is the server's clock at the first save carrying the
playback id. The client sends that save immediately at the first `play`
(today the first save waits for the 5 s timer or a pause), but not before the
player reports `positioned` (`PlayerStatus.positioned` in
`web/src/player/VideoPlayer.tsx`: the first metadata arrived and the resume
seek or the transcode's start report settled). Autoplay calls `play()` right
after setting the source, so the first `play` can fire while the position is
still 0 and the stored resume position not yet applied; a save then would
overwrite the resume position. When `play` comes first, the hook holds the id
and sends the save when `positioned` arrives; until then no save or beacon
carries the id, so leaving that early records no viewing. The time is
therefore the start of playback within the metadata load, the resume seek and
one round trip. The list orders by `played_at` descending,
then `id` descending, and the entry keeps its place while it is being watched.

| Option | Verdict |
| --- | --- |
| **Server time of the first save** | Chosen |
| A client-reported start time | Rejected: a device clock the server does not control orders the owner's history |
| The time of the latest save (last activity) | Rejected: a long viewing would keep jumping to the top while two other viewings happen; "when I watched it" is when it started |
| Record the start apart from the position (a save that writes no position, or a separate request) | Rejected: one more request shape for a gain of the resume seek's duration, usually under a second; the device clock stays out of the order either way |

## R-4: Existing playback records are backfilled by the migration

**Decision**: The migration inserts one entry per `playback_progress` row
with `played_at` = its `updated_at`, no playback id, the content key of the
record (a bundle key resolves to the bundle's representative), and the title
the video page shows at that moment (the display name override, else the
file title; the display name, else empty, when the content is not in the
library). A record whose content has no video at migration time still gets
its entry: playback positions are user data keyed by content and outlive
removal from the library (ARCHITECTURE.md, "User data survives a rebuild";
nothing in `internal/store` deletes a `playback_progress` row), and the entry
without a video is the case R-5 already shows; the backfill runs once, so
skipping it would lose that viewing for good even after the content returns.
A `bundle:<id>` key with no `video_bundles` row is skipped, because no content
key can be derived from it; bundling and dissolving move user data in the
same transaction (`internal/store/versions.go`), so such a row is not
expected.

| Option | Verdict |
| --- | --- |
| **Backfill in the migration** | Chosen |
| Read `playback_progress` into the list at request time when it has no entry | Rejected: two sources for one list, and deleting such an entry would have to delete the playback position, against requirement 9 |
| No backfill | Rejected by requirement 10 |

**Rationale**: SQL alone can produce these rows, the migration runs once at
startup before the listener opens, and from then on the history has one
source.

## R-5: An entry whose content left the library stays, without a video

**Decision**: The list joins each entry to the video that has its content key
and a location the owner may open (`visibleVideoCondition`, the per-video
condition, not the shown-representative rule). When there is one, the entry
carries that `Video` and the screen opens it; otherwise the entry carries only
the snapshot title, the screen marks it as not playable and renders no link.
A bundle member that is not the representative still counts as present, so
its entry opens that version.

| Option | Verdict |
| --- | --- |
| **Keep the entry, mark it unplayable** | Chosen |
| Hide entries whose content is gone | Rejected by the Edge Case: the entry stays and comes back to life when the content returns |
| Resolve to the bundle's representative instead of the version played | Rejected: the entry says which version was watched, and that video's page lists the other versions |

## R-6: Deleting entries is a plain `DELETE`; a vanished entry answers `404`, and the screen reloads

**Decision**: `DELETE /api/watch-history/{id}` answers `204` when the row
existed and `404 not_found` when it did not; `DELETE /api/watch-history`
clears every row and answers `204`. Neither publishes a domain event or an
`/api/events` notification. On `404` the screen reloads the list from the
top without an error, because the list it shows is stale; on any other
failure it keeps the row and shows a toast.

| Option | Verdict |
| --- | --- |
| **`204`/`404`, screen reloads on `404`** | Chosen |
| Idempotent `204` for a missing row | Rejected: the screen could not tell that its list is stale, and the Edge Case asks it to bring the list up to date |
| A `watchHistory` notification on `/api/events` | Rejected: the only cross-tab case the Issue names is the stale delete, which `404` covers; lists refetch when opened |

## R-7: Watch history is one more owner-only screen and route, with the existing gate rules

**Decision**: The screen is `/history`, listed in the sidebar's top group for
the owner only, added to the gate's owner-only paths (a guest at `/history`
goes to `/login?next=/history`), and the three routes are owner-only by the
`/api/*` default, so a guest gets the same `401` as every other owner-only
route. The history is not added to the external API.

| Option | Verdict |
| --- | --- |
| **The existing owner-only path and route rules** | Chosen |
| `404` for a guest at `/history`, as for hidden videos | Rejected: the screen already sends guests from `/settings`, `/tags` and `/duplicates` to the login page, and acceptance criterion 9 only requires that the history is not shown |

## Revision: filter, search, date jump and resume actions

Requirements 12 to 18 were added to the parent Issue after R-1 to R-7 were
decided. R-8 to R-13 are the decisions that revision adds; R-1 to R-7
stand.

## R-8: Filter, search and date jump are conditions of the list request

**Decision**: `GET /api/watch-history` gains `watch`, `query`, `date` and
`tz`, and the store applies them in SQL before the page limit
([contracts/screen-api.md](contracts/screen-api.md#get-apiwatch-history)); the
screen never filters loaded rows itself.

| Option | Verdict |
| --- | --- |
| **Conditions on the list request** | Chosen |
| Filter and search the loaded pages in the browser | Rejected: requirement 6 reads the whole history, so a filter that matched three entries in a year would load every page to show them; the date list (requirement 15) needs the unloaded entries too |
| A separate search endpoint | Rejected: requirement 14 combines the state and the search, and one request with two conditions is the shape the library uses (`listVideos`) |

**Rationale**: The server already owns the order and the paging; a condition
is one more `where` clause on the same query, and the cursor keeps working
because it points into the filtered order.

## R-9: The state filter reads the video's current watch state, with the library's rule

**Decision**: `watch` is `all`, `inProgress` or `watched`
(`WatchHistoryFilter`, a new schema); `inProgress` and `watched` keep the
entries whose video is in the library and whose `playback_progress` row, read
by the user key as the cards read it, satisfies `watchCondition` of
`internal/store/listing.go`; entries without a video and entries whose video is
unwatched appear only under `all` (Edge Case).

| Option | Verdict |
| --- | --- |
| **A three-value filter on the video's current state** | Chosen |
| Reuse `WatchFilter` and answer `400` to `unwatched` | Rejected: the generated client type would offer a value the screen has to refuse, and `unwatched` has no meaning for a viewing |
| Decide by the position of that viewing (the entry) | Rejected by requirement 12: the state is the video's now, so every entry of one video falls on the same side |
| Store the state on the entry | Rejected: the state changes with every save, and requirement 12 ties it to the library's "watched" |

**Rationale**: One SQL condition shared with the library guarantees that a
card marked watched is listed under `watched` (acceptance criterion 10), and the
user key keeps a bundle's shared position in step with its cards.

## R-10: Title search uses the library's query syntax on the title alone

**Decision**: `query` is parsed by `domain.ParseSearchQuery` and matched with
`instr` on the folded title: for an entry with a video, the title line and
the display-name line of `search_key` of any of its registered locations
(the lines `locationSearchKey` builds, without the path line); for an entry
without a video, `watch_history.title_key`, the snapshot title in the same
match form as a title line of `search_key`, newlines as spaces
([data-model.md, Migration](data-model.md#migration)). Tags are not matched.

| Option | Verdict |
| --- | --- |
| **Library syntax and match form, title lines only** | Chosen |
| The library's whole condition (`searchExprCondition`) | Rejected: it matches the relative path and the tag names, so an entry whose title lacks the word would be listed, against requirement 13 |
| Match the snapshot title for every entry | Rejected: a renamed video would be found by its old name and shown with its new one; the row shows the current title (ui-design.md, Entry row) |
| Full-text index on the title | Rejected: `location_search_fts` indexes the whole key, path included, and the owner's viewings are far fewer than the library's locations, so `instr` is enough |

**Rationale**: Requirement 13 asks for the library's matching (substring,
NFKC, case and kana folding), not the library's targets; the parser and
`FoldForMatch` are reused, the targets are the two name lines. A word in a
renamed video's file title still finds the entry, which is accepted: the file
title is one of the video's names.

## R-11: The date list and the jump are computed on the server in the viewer's time zone

**Decision**: `GET /api/watch-history/dates` returns the distinct days that
have entries under the current `watch` and `query`, newest first, as
`YYYY-MM-DD` in the IANA zone `tz` the screen sends; the screen folds older
days into months. `date` (`YYYY-MM-DD` or `YYYY-MM`) with `tz` on the list
request keeps the entries whose `played_at` is before the end of that day or
month in `tz`, and paging continues with `nextCursor`. The server loads the
zone with `time.LoadLocation` and embeds `time/tzdata` so a host without a
zone database still answers.

| Option | Verdict |
| --- | --- |
| **IANA zone from the browser, days computed in Go** | Chosen |
| A UTC offset in minutes | Rejected: one offset is wrong for the months on the other side of a daylight-saving change, so a day list over a year puts late-evening entries on the wrong day |
| The screen computes the day boundaries and sends an instant (`before`) | Rejected: the day list still needs the zone on the server, and two places doing the date arithmetic drift apart |
| Group by day in SQL (`date(played_at / 1000, 'unixepoch')`) | Rejected: SQLite knows no zones; reading one integer per entry and grouping in Go is cheap for an owner's history |
| The server cuts the list into days and months | Rejected: where days become months is a screen decision (`ui-design.md`), and the screen needs the days to draw either |
| A list over the whole history, ignoring the filter and the search | Rejected: choosing a date would then land on the "no matches" state; acceptance criterion 13 lists the dates that have entries |

**Rationale**: The day of an entry depends on where the viewer is, and only
the browser knows that; the server knows every entry. Passing the zone name
once gives one source of truth for both the list and the jump.

## R-12: The filter, the search and the date live in the screen's URL

**Decision**: `/history` carries `watch`, `q` and `date` as query parameters,
written and read as [list-url.md](../013-library-search/contracts/list-url.md)
does for the library (defaults omitted, unreadable values treated as the
default), and the row link's `state.from` carries the full URL.

| Option | Verdict |
| --- | --- |
| **URL query parameters** | Chosen |
| Component state | Rejected: opening a video leaves the route, so coming back with `×` or Esc would reset the filter and the date |
| `sessionStorage` | Rejected: the library already answers this with the URL; a second mechanism, and a view that cannot be reloaded or shared |

## R-13: The resume and restart actions open the video page with autoplay, and the existing resume rule decides the position

**Decision**: Both actions (the Issue's `続きから` and `最初から`; the English
words are `ui-design.md`'s) navigate to `/videos/{id}` with
`state: { from, autoplay: true }`, as "Play next" does; the page's
`resumePosition` then starts an in-progress video at its saved position and a
watched one at 0 (requirement 18 restates that rule). The row shows
`video.progress.positionMs` over `video.durationMs`.

| Option | Verdict |
| --- | --- |
| **Autoplay through the existing state, position from the resume rule** | Chosen |
| A `startMs` in the navigation state | Rejected: a second source for the start position that can disagree with a plain open of the same video (requirement 5 keeps that rule); the rule already yields the displayed outcome |
| Show `resumePosition` in the row instead of the saved position | Rejected: requirement 16 asks for the current position; the two differ only under `MinResumeMs` (5 s) and when a bundle member's shared position exceeds this version's length ([030 R-11](../030-video-versions/research.md)), where the viewer sees the video start from 0 |

**Rationale**: The page already owns autoplay and the resume rule, so the
history adds a link, not a player feature.
