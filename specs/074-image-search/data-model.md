# Data model: Image search

The rest of the model is unchanged:
[`internal/store/migrations/`](../../internal/store/migrations). The feature
adds one rebuildable index table and two job kinds; it adds no user data.

## Migration

`00100_video_search_index.sql` creates `video_search_index` and rebuilds
`jobs` to add `frame_index` and `queue_frame_index` to its `kind` check, the
way [`00084_job_scheduling.sql`](../../internal/store/migrations/00084_job_scheduling.sql)
added kinds. The migration queues no jobs; the first startup's
`queue_frame_index` does.

## `video_search_index`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `content_key` | text, primary key | No | The content the entries were read from |
| `version` | integer | No | `domain.SearchIndexVersion` the entries were built with |
| `entries` | blob | No | 12 bytes per kept keyframe, in time order: a 4-byte big-endian time in milliseconds, then the 8-byte big-endian hash |
| `updated_at` | integer | No | Unix seconds of the write |

The table is `without rowid`, like `video_fingerprints`.

**Relationships**: No foreign key, like every content-keyed index. The row is
deleted by `releaseContentIndex` when the content loses its last reference.

## Rules

| Rule | Enforced in |
| --- | --- |
| Entries are in strictly increasing time, at least `max(1000, duration / 20,000)` ms apart, at most 20,000, none flat | `domain.NewSearchIndex` |
| A blob whose length is not a multiple of 12, or whose times do not increase, is unreadable and the content is treated as unindexed | `domain.DecodeSearchIndex` |
| A row of another version counts as missing for the search, the unindexed count and `queue_frame_index` | `internal/store` queries compare `version` with `domain.SearchIndexVersion` |
| A write applies only when the claimed job's content, location and generation are still current | `IngestStore.ApplySearchIndexForJob`, through `jobIdentityCurrent` |

## Job kinds

| Kind | Names a video | Class | Demand | Concurrency | Queued by |
| --- | --- | --- | --- | --- | --- |
| `frame_index` | Yes | Backfill (2) | 1 read, 1 CPU | 1 | The probe result's transaction for a content without a current row; `queue_frame_index` |
| `queue_frame_index` | No | Backfill (2) | None | 1 | Each startup, with the other backfill queue jobs |

Both stay out of import tracking, as every backfill kind does
([job-scheduling.md](../../docs/design-docs/job-scheduling.md#owner-requests-are-claimed-first)):
no activity entry, no effect on import progress or the desktop app's busy
state, and the row is deleted when the job ends.

## What does not change

`video_fingerprints` and the version-judging and related-video rules keep
their 81-frame fingerprint; the search index is separate, and neither version
constant moves the other. No user-data table is added: the query file and the
results are never stored.
