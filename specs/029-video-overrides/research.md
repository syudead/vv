# Research: Overriding a video's display name and representative thumbnail

Inherited decisions: the tech stack, the boundaries and dependency direction,
the split between index and user data, where generated artifacts live, and the
authentication boundary follow the canonical documents
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[ARCHITECTURE.md](../../ARCHITECTURE.md),
[internal/artifacts/store.go](../../internal/artifacts/store.go),
[specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)).
This file records only the decisions this feature adds.

## R-1: Overrides live in one user-data table keyed by `content_key`

**Decision**: The display name and the thumbnail position are one row of
`video_overrides (content_key primary key, display_name, thumbnail_position_ms, thumbnail_revision, updated_at)`
([data-model.md §1](data-model.md#1-video_overrides)). There is no foreign key
to `videos`, and a row whose two columns are both null is deleted.

**Rationale**: Requirement 4 and the edge case "the same content is in several
locations" require the override to survive rescans, moves, renames and added
locations, and to be the same regardless of location. That makes it
non-rebuildable user data like `playback_progress`, `video_tags` and
`public_videos`, and like them it is keyed by the content identifier
(ARCHITECTURE.md "Rebuildable and user data"). Whether a row exists directly
answers "is this video overridden" (requirement 7).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Add columns to `videos` | Rejected: video rows are index and disappear when the last location is lost. When the content is put back, the override does not return, violating requirement 4. |
| Separate tables for the display name and the thumbnail | Rejected: reads would need two joins, and "is it overridden" would be gathered from two places. No external API handles both in one operation either. |

## R-2: The store decides the effective title; `domain.Video` carries both the display name and the file-name title

**Decision**: Every read that returns videos (list, detail, related, and the
external API's list and lookup) left-joins `video_overrides` and sets
`Video.Title` to "the display name if there is one, otherwise the location's
title". `Video.FileTitle` carries the title from the file name,
`Video.DisplayName` the display name (empty when unset), and
`Video.ThumbnailPositionMs` the thumbnail position (nil when unset)
([data-model.md §2](data-model.md#2-values-added-to-domain)). This is built the
same way as `Video.Public` is filled from `public_videos`.

**Rationale**: The title appears in lists, folder views, the video page,
related videos, guest screens, the external API and MCP (requirement 1).
Deciding it in one place in the read means the code building responses outputs
`Title` as is, and no place can forget it. The "file-name title" and "is it
set" of requirements 7 and 8 come from the same row.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Replace `title` in `internal/httpapi` | Rejected: sorting and search are in the store, where the replacement has no effect. Violates requirement 3. |
| Rewrite `video_locations.title` | Rejected: the title is a fact of the location and is rewritten on every scan. The override would disappear on the next scan. |

## R-3: Sorting and search fold the display name into each location's `title_key` and `search_key`

**Decision**: `video_locations.title_key` becomes the `domain.NaturalSortKey`
of the effective title (the display name if there is one). `search_key` adds
the display name as a third part to the current "title `\n` relative path"
(`domain.SearchKeyVersion` is not raised; locations without a display name get
the same key as today). Keys are rewritten in three places
([data-model.md §4](data-model.md#4-sort-and-search-keys)):

| Path | Trigger |
| --- | --- |
| The transaction that rewrites the display name | All locations of the videos with that content |
| Adding or updating a location during import | `refreshSearchKeysByPath` reads the display name |
| Adding or changing a media folder | `refreshSearchKeysUnder` in `AddMediaFolder` and `ReplaceMediaFolder` reads the display name |

**Rationale**: Requirement 3 requires that title order places the video at its
display name and that search matches both the display name and the file name.
Sorting reads `loc.title_key` and search reads `location_search_fts`
([specs/013-library-search/data-model.md](../013-library-search/data-model.md));
putting the value there leaves every list, folder and library item query
unchanged. Paths that rebuild the keys already exist.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| An FTS index on `video_overrides`, ORed into the search expression | Rejected: changes both `chosenLocationsCTE` and the library item CTE, and splits search matching across two indexes. |
| Sort by `coalesce(display name, title_key)` | Rejected: SQL cannot build the natural-order key of a display name. `title_key` is stored in natural order because Go writes `NaturalSortKey`. |

## R-4: A thumbnail position is generated within the request, then recorded

**Decision**: `app.Ingest.SetThumbnailPosition` sets and clears the position.
Inside the generation lock (`artifactThumbnail` in `artifacts.generate`), it
has `PublishThumbnail` write the new image (`ThumbnailAt` for a chosen
position, `Thumbnail` with the current automatic rule when clearing). Once
published, one transaction writes the position to `video_overrides`, sets the
video's `thumbnail_state` to `done` and deletes the `thumbnail_first_frame`
substitution row. When generation fails, nothing is written and the error is
returned
([contracts/screen-api.md §2](contracts/screen-api.md#2-put-apivideosidthumbnail-position)).
The response returns after generation finishes.

**Rationale**: The edge cases require "on failure, keep the previous image and
let the owner see that it failed" and "when another choice arrives before
generation finishes, the last choice wins". `PublishThumbnail` writes to a
temporary location and swaps, so the previous image survives a failure.
Generating within the request reports a failure in that response, and choices
for the same content are serialized by the lock, so the image of the last
recorded position remains. A one-frame extraction with `-ss` before the input
finishes in a few seconds (`internal/media/thumbnail.go`).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Queue a `thumbnail` job for the worker | Rejected: on failure `thumbnail_state` becomes `failed` and the previous image disappears from `thumbnailUrl`. The failure reaches the owner through the import problem list, not the video page. A running job and the next choice collide on `jobs_pending_kind_video_idx`. |
| The screen API calls `internal/media` directly | Rejected: `internal/app` owns the generation lock and the serialization with deletion, and httpapi makes no generation decisions (ARCHITECTURE.md). |

## R-5: The import thumbnail job also uses the chosen position; the automatic rule stays in `internal/media`

**Decision**: `Ingest.Thumbnail` (the job) generates at
`Video.ThumbnailPositionMs` when it is set (`generator.ThumbnailAt`) and with
the current rule otherwise. The position and `thumbnail_state` are decided from
the values reread with `IngestStore.GetVideo` after entering the generation
lock (`artifacts.generate`). The `video` read earlier outside the lock is used
only to check the duration and existence. `internal/media` gets
`ThumbnailAt(ctx, path, positionMs, output) error`. When the chosen position
yields nothing, there is no fallback to the first frame; the failure follows
the current retry and `failed` recording. The automatic position rule (10%,
1 s to 60 s) stays a constant in `internal/media` and does not move to
`internal/domain`.

**Rationale**: After a video row disappears and its artifacts are cleaned up,
putting the same content back makes the `thumbnail` job regenerate the image.
If the job did not know the position, the row (user data) would hold the
chosen position while the image showed the automatic frame (edge case "the
same thumbnail from every location"). A position that worked once on a file
with the same content works again, so the job needs no other fallback.

The job rereads inside the lock because, after the job reads the video outside
the lock, `SetThumbnailPosition` can take the lock first while the job waits
and record a position and `done`. With the earlier values, the job would
overwrite the image with the stale state (not `done`) and the stale position,
and the recorded position and the image would disagree (edge case "the last
choice wins"). After rereading, it sees the recorded `done` and skips
generation, or generates at the recorded position. The position rule is a
generation constraint (ffmpeg outputs 0 frames at the very start), not a
decision, so it does not move.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Put the chosen image in a separate file (`<s>.pick.jpg`) | Rejected: changes the layout of the store and adds branches to `RemoveContent`, ETags and serving. One representative image per content is enough. |
| Delete the row when the job fails at the chosen position | Rejected: the job would rewrite user data, and an import failure would erase the setting. |

## R-6: The `thumbnailUrl` version includes the position revision

**Decision**: `video_overrides.thumbnail_revision` is a revision number written
each time a position is recorded: the larger of the current time in
milliseconds and the previous value + 1 (null when cleared). When a position is
set, the `v` of `thumbnailURL` is `<content key prefix>-r<thumbnail_revision>`;
when unset, it stays as today. The position value itself is not put in the URL.

**Rationale**: Images are served with `private, no-cache` and an `ETag`, but
the browser does not request an `<img>` again when its `src` string is
unchanged. For the list and the video page to show the new image right after
the position changes, `src` has to change. Clearing returns to the "no
position" URL, so that changes too. `thumbnailUrl` is also given to guests, so
the version is a value unrelated to the position, to keep the URL from leaking
`thumbnailPositionMs`, which guest responses omit. Because it increases on
every record, choosing the same position again still changes the URL, and a
row deleted and recreated does not collide with an earlier value thanks to the
time.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Put `positionMs` in the version as is | Rejected: guests could see an owner-only field. |
| Use `updated_at` as the version | Rejected: a display name change would also change the thumbnail URL and cause needless refetches. |
| Use `updated_at` as the version on every response | Rejected: URLs, including those of videos without `video_overrides`, would no longer be stable, weakening ETag-based 304s. |

## R-7: Override changes publish `domain.VideoOverrideChanged`, mapped to the screen's `video` notification

**Decision**: After a transaction that writes `video_overrides` commits, the
store publishes `VideoOverrideChanged{VideoID}` (for both the display name and
the position). The screen subscription in `cmd/mdm/events.go` maps it to
`video` on `/api/events`, and the list refetch and video page refetch work
through the existing mechanism. It is not tied to waking the worker.

**Rationale**: Lists are restored from a snapshot (`listSnapshot.ts`), so
without a `video` notification, a title or thumbnail changed on the video page
would be stale when the user goes back. Changes from another tab or the
external API (edge case) arrive through the same notification.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Reuse `VideoIngestChanged{Stage: thumbnail}` | Rejected: a display name change is not a change of import stage, and the seek_thumbnail worker wakes on that event. |
| The screen updates only from the save response | Rejected: changes from other tabs and the external API would not arrive. |

## R-8: The screen API is two `PUT`s per video; an empty display name and a `null` position clear

**Decision**: `PUT /api/videos/{id}/display-name` (body `{ displayName }`;
cleared when empty after trimming) and `PUT /api/videos/{id}/thumbnail-position`
(body `{ positionMs }`; `null` clears). Both are owner-only and respond with the
same `Video` as `GET /api/videos/{id}`
([contracts/screen-api.md](contracts/screen-api.md)).

**Rationale**: The edge case decides that "an empty or whitespace-only display
name clears it", so clearing needs no separate operation. Setting a position
involves generation and takes seconds, while saving a name is immediate;
combining them into one operation would mean responses where only one half
failed. The response returns the detail `Video` so the video page can replace
the title, file name and thumbnail without navigating (UI quality: "changing
the display name completes without leaving the video page").

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| `PATCH /api/videos/{id}` | Rejected: split for the same reason as above. |
| Add `DELETE` | Rejected: empty and `null` already clear, so it only adds operations. |

## R-9: The external API adds two bulk operations, and MCP the same two tools

**Decision**: Add `POST /api/v1/video-display-names` (1–20000 items, one
transaction) and `POST /api/v1/video-thumbnails` (1–20 items; validate all
first, then generate in order, stopping at the first generation failure), and
add `fileTitle`, `displayName` and `thumbnailPositionMs` to `ExternalVideo`. MCP
gets `update_video_display_names` and `update_video_thumbnails`
([contracts/external-api.md](contracts/external-api.md)).

**Rationale**: Requirement 8 and the out-of-scope note "bulk changes are made
through the external API and MCP". Video references with `VideoRef`, the error
`index`, and validate-all-then-apply match `POST /api/v1/video-tags`, so users
learn no new shape. Thumbnails run ffmpeg per item, so the 20000 items of tags
do not fit in one request; the limit is 20. Generation can fail partway, so
the items up to that point stay applied, `index` reports where it stopped, and
the user resends the rest.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Roll back every item on a generation failure | Rejected: published images cannot be rolled back by a transaction. |
| Return per-item results with 200 | Rejected: users would read the array to find errors, unlike the other operations where errors are status codes. |
| One operation for display names and thumbnails | Rejected: the thumbnail item limit would constrain bulk display names. |

## R-10: Display name rules follow tag name rules, with a 200 code point limit

**Decision**: `domain.NormalizeDisplayName` rejects input containing control
characters (`display_name_control_characters`), trims surrounding whitespace,
returns "clear" when the result is empty, and rejects input longer than
`DisplayNameMaxLength = 200` code points (`display_name_too_long`, `limit`).

**Rationale**: A title containing a line break does not fit on one card line
and does not match search terms, so it is rejected for the same reason as tag
names ([specs/014-video-tags/data-model.md §2](../014-video-tags/data-model.md#2-name-rules)).
Clearing on empty is the edge case's decision. The name replaces a file name
(255 bytes), and cards and the video page already fit titles of that length, so
the limit is 200, which does not go below that.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| 100, the same as tag names | Rejected: shorter than file-name titles, so some titles shown today could not become display names. |
| No limit | Rejected: the card line and the `title_key` index would grow without bound. |

## R-11: Position validation is a pure function in `internal/domain`, with three distinct errors

**Decision**: `domain.CheckThumbnailPosition(video Video, positionMs int64) error`
returns:

| Condition | Error | Response |
| --- | --- | --- |
| `ProbeState` is not `done`, or `DurationMs` is missing or 0 | `ErrDurationUnknown` | 409 `conflict` / `duration_unknown` |
| `positionMs < 0` or `>= DurationMs` | `ErrThumbnailPositionOutOfRange` | 400 `invalid_request` / `thumbnail_position_out_of_range`, `limit` = duration |
| Generation fails | `ErrThumbnailFrameUnavailable` | 409 `conflict` / `thumbnail_frame_unavailable` |

**Rationale**: The edge cases "reject a video whose length is unknown before
analysis, or a position past the duration, and return the reason" and "make a
generation failure visible". Out of range is an input error, while
before-analysis and generation failure are problems with the video's current
state, so the codes differ. This matches the existing
`ErrSeekFrameUnavailable`, which is 409.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Clamp an out-of-range position to the end | Rejected: the image differs from the frame the user pointed at, and no reason is returned. |
| 400 before analysis too | Rejected: the input is correct and succeeds once analysis finishes. |
