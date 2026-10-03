# Data model: Overriding a video's display name and representative thumbnail

Parent Issue: #517.

The rest of the model is unchanged. Existing table definitions are canonical in
[internal/store/migrations/](../../internal/store/migrations/), the data
categories are in [ARCHITECTURE.md](../../ARCHITECTURE.md) "Rebuildable and
user data", and the per-location search keys are in
[specs/013-library-search/data-model.md](../013-library-search/data-model.md).
This file covers only the table and values this feature adds and the rules for
reading and writing them. Tables not mentioned here do not change: no column is
added to `videos` or `video_locations`, and `video_locations.title` stays the
scan's fact.

## 1. `video_overrides`

Added as `00021_video_overrides.sql` (the last migration on `main` is
`00020_api_tokens.sql`).

```sql
-- The display name and representative thumbnail position the owner set
-- (specs/029-video-overrides/research.md R-1).
-- Non-rebuildable user data. Like playback_progress, video_tags and
-- public_videos, it is keyed by the content identifier and has no foreign key to videos.
create table video_overrides (
    content_key           text    primary key,
    -- The display name after domain.NormalizeDisplayName. null when unset (never an empty string).
    display_name          text,
    -- The position (milliseconds) of the frame used as the representative thumbnail. null when unset.
    thumbnail_position_ms integer check (thumbnail_position_ms is null or thumbnail_position_ms >= 0),
    -- A revision number written each time a position is recorded (R-6). Used as the thumbnailUrl
    -- version so the position value does not appear in the URL.
    -- null when the position is null.
    thumbnail_revision    integer,
    updated_at            integer not null
) without rowid;
```

Down drops the table.

**Invariants** (added to `internal/store/invariants_test.go`):

| Rule | Enforced in |
| --- | --- |
| No row has both `display_name` and `thumbnail_position_ms` null | The writer deletes such a row |
| `thumbnail_revision` is non-null only when `thumbnail_position_ms` is non-null | The writer |
| No row has an empty `content_key` | The writer |

**Category**: non-rebuildable user data, added to the list in ARCHITECTURE.md.
Artifact cleanup (`RemoveContent`) does not touch this table. A video whose
content changed and became a different logical video has a different
`content_key`, so the row is not carried over (edge case; same handling as
tags).

## 2. Values added to `domain`

| Value | Content |
| --- | --- |
| `Video.FileTitle` | The title from the file name (`video_locations.title`). `Title` becomes the effective title (the display name if there is one) ([R-2](research.md#r-2-the-store-decides-the-effective-title-domainvideo-carries-both-the-display-name-and-the-file-name-title)) |
| `Video.DisplayName` | The display name. Empty when unset |
| `Video.ThumbnailPositionMs` | `*int64`. nil when unset |
| `Video.ThumbnailRevision` | `int64`. The position revision number ([R-6](research.md#r-6-the-thumbnailurl-version-includes-the-position-revision)). 0 when unset. Used only for the `thumbnailUrl` version; not a response field |
| `NormalizeDisplayName(string) (name string, clear bool, err error)` | [R-10](research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit). `clear` means the input was empty after trimming. Errors are `*InvalidDisplayNameError` (`DisplayNameControlCharacters`, `DisplayNameTooLong`; the same shape as `InvalidTagNameError`) |
| `DisplayNameMaxLength = 200` | In code points |
| `CheckThumbnailPosition(Video, int64) error` | [R-11](research.md#r-11-position-validation-is-a-pure-function-in-internaldomain-with-three-distinct-errors). `ErrDurationUnknown`, `ErrThumbnailPositionOutOfRange` |
| `ErrThumbnailFrameUnavailable` | No image could be made at the chosen position. `internal/app` wraps the failure of `ThumbnailAt` in `internal/media` |
| `VideoOverrideChanged{VideoID}` | An override changed ([R-7](research.md#r-7-override-changes-publish-domainvideooverridechanged-mapped-to-the-screens-video-notification)). Implements `Event` |
| `ExternalVideo` | Unchanged. `Video` carries the added fields |

## 3. Store operations

Reads add a left join of `video_overrides`
(`ov.content_key = videos.content_key and videos.content_key <> ''`) to
`videoColumnsTemplate`, `listColumns` and the columns of the external API list,
and map `coalesce(ov.display_name, loc.title)` to `Title`, `loc.title` to
`FileTitle`, and `ov.display_name`, `ov.thumbnail_position_ms` and
`ov.thumbnail_revision` to their fields. `IndexedVideo` does not change.

A new role, `OverrideStore` (holding only the SQL connection, like
`VisibilityStore`):

| Operation | What one transaction does |
| --- | --- |
| `SetDisplayName(ctx, videoID, name string)` (clears when `name` is empty) | Resolves `videoID` to its content key with `registeredContentKeysForVideoIDs` (`ErrNotFound` when absent). Upserts the row, deleting it when both columns are null. Rewrites the `title_key` and `search_key` of every location of the videos with that content key (§4). After commit, publishes `VideoOverrideChanged` and returns the `Video` with the effective title |
| `SetDisplayNames(ctx, []DisplayNameChange)` (external API bulk) | Resolves each `VideoRef` (`ErrNotFound` with `index` when one is not found) and does the same as above for every item. If any item fails, nothing is kept |
| `SetThumbnailPosition(ctx, videoID, positionMs *int64)` | Upserts the row (`nil` is null; deleted when both are null). When writing a position, sets `thumbnail_revision` to the larger of the current time in milliseconds and the previous value + 1; `nil` sets it to null. In the same transaction, sets `thumbnail_state` of the videos with that content key to `done` and `updated_at` to now, and deletes the `thumbnail_first_frame` substitution row (the same table as `applySubstitution`). After commit, publishes `VideoOverrideChanged`. **Only `app.Ingest` calls it, after publishing has finished** ([R-4](research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded)) |

`IngestStore.GetVideo` also returns the overrides (the job rereads inside the
generation lock to decide the position and state;
[R-5](research.md#r-5-the-import-thumbnail-job-also-uses-the-chosen-position-the-automatic-rule-stays-in-internalmedia)).

The interface `app.Ingest` declares (`IngestStore`) gets
`SetThumbnailPosition`. The interfaces `internal/httpapi` declares get
`SetDisplayName` and `SetDisplayNames`, plus `ThumbnailPicker` for calling
`app.Ingest`'s `SetThumbnailPosition`. `cmd/mdm` wires them.

## 4. Sort and search keys

[R-3](research.md#r-3-sorting-and-search-fold-the-display-name-into-each-locations-title_key-and-search_key).
The keys of one location change as follows (adding the display name to the
rules in
[specs/013-library-search/data-model.md §3 and §4](../013-library-search/data-model.md)).

| Column | Without a display name (same as today) | With a display name |
| --- | --- | --- |
| `title_key` | `NaturalSortKey(title)` | `NaturalSortKey(display_name)` |
| `search_key` | `part(title) + "\n" + part(rel)` | `part(title) + "\n" + part(rel) + "\n" + part(display_name)` |

`searchKeyTarget` gets `displayName`. The four paths that rebuild keys read the
display name by joining `video_locations` with `videos` and `video_overrides`:

| Path | Caller |
| --- | --- |
| `refreshSearchKeysByPath` | Import |
| `refreshSearchKeysUnder` | `AddMediaFolder`, `ReplaceMediaFolder` |
| `refreshSearchKeysForContentKey` (new) | The display name transaction |
| `RefreshSearchKeys` | Startup |

`SearchKeyVersion` is not raised. Search term matching (`search.go` in
`domain`) does not change. The rule that uses line breaks as boundaries stays,
and `searchKeyPart` turns line breaks into spaces in the third part too.

## 5. Generated artifacts

The store layout does not change. The image at a chosen position also goes in
`<p>/<s>.jpg`
([R-5](research.md#r-5-the-import-thumbnail-job-also-uses-the-chosen-position-the-automatic-rule-stays-in-internalmedia)).
`internal/media` gets
`ThumbnailAt(ctx, videoPath string, positionMs int64, output string) error`.
It reuses `thumbnailArgs` and does not fall back to the first frame. The
`Generator` interface of `app.Ingest` gets the same signature.
