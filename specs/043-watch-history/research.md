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
The video page creates one id (`crypto.randomUUID()`) at the first `play`
event for a video and sends it with every save and beacon until the video id
changes; saves before the first play carry none. The server inserts the entry
with `insert or ignore` on the unique `playback_id` in the same transaction as
the position, so pauses, resumes and seeks extend nothing and create nothing.
The id lives in the page's progress-saving hook, not in `VideoPlayer`, so a
player remount (error recovery, quality switch) keeps the entry.

| Option | Verdict |
| --- | --- |
| **Client playback id on the existing save** | Chosen |
| Server-side merging by time gap between saves | Rejected: two tabs playing the same video would merge into one entry (Edge Case), and a pause longer than the gap would split one viewing (acceptance criterion 3) |
| A separate `POST` at the first play | Rejected: one more round trip per play, and after the owner deletes the entry mid-playback the client would have to notice and post again; with the id on every save the next save recreates it, which is the Edge Case's "a new entry" |
| The id per `VideoPlayer` instance | Rejected: recovery and quality switches remount the player, which would split one viewing |

**Rationale**: The client is the only party that knows where one viewing
starts and ends (the page), and the server is the only party that must not
trust it for anything but identity: it decides the time and what the entry
points at. Reloading the page starts a new entry; the Issue calls a repeat
viewing "after a while" a separate entry, and a reload is the closest thing
the page can observe.

## R-3: The entry's time is the server time of the first save with its id

**Decision**: `played_at` is the server's clock at the first save carrying the
playback id. The client sends a save immediately at the first `play` (today
the first save waits for the 5 s timer or a pause), so the time is the start
of playback within a round trip. The list orders by `played_at` descending,
then `id` descending, and the entry keeps its place while it is being watched.

| Option | Verdict |
| --- | --- |
| **Server time of the first save** | Chosen |
| A client-reported start time | Rejected: a device clock the server does not control orders the owner's history |
| The time of the latest save (last activity) | Rejected: a long viewing would keep jumping to the top while two other viewings happen; "when I watched it" is when it started |

## R-4: Existing playback records are backfilled by the migration

**Decision**: The migration inserts one entry per `playback_progress` row
with `played_at` = its `updated_at`, no playback id, the content key of the
record (a bundle key resolves to the bundle's representative), and the title
the video page shows at that moment (the display name override, else the
file title; empty when the content is not in the library). Rows whose key
resolves to nothing are skipped.

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
