# Data model: Force rescan

The rest of the model is unchanged:
[`internal/store/migrations`](../../internal/store/migrations).

## Migration

`00071_force_rescan.sql` adds columns to `scans`, `scan_videos`, `jobs` and
`video_version_candidates`; it creates no table.

## `scans`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `rescan_scope` | text (`library`, `folder`, `video`) | Yes | Null for an ordinary scan ([R-1](research.md#r-1-a-force-rescan-is-a-scan-with-a-scope-and-a-force-flag)) |
| `rescan_folder_id` | integer | Yes | The media folder of a `folder` scope |
| `rescan_path` | text | Yes | That folder's path at start, read by the scanner and shown while the scan runs |
| `rescan_video_id` | integer | Yes | The video of a `video` scope |

**Relationships**: No foreign key; a folder cannot change while a scan runs
(`CheckMediaFolderMutation`), and a deleted video leaves a finished scan's
row as history.

## `scan_videos`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `forced` | integer (0/1) | No, default 0 | The force pass queued this video's jobs in this scan ([R-4](research.md#r-4-an-unchanged-file-in-force-mode)) |

## `jobs`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `force` | integer (0/1) | No, default 0 | Regenerate without adopting what is on disk ([R-2](research.md#r-2-force-travels-on-the-job-not-on-the-stage-states)) |

## `video_version_candidates`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `rejudge_of` | integer | Yes | The `activity_entries.id` of the automatic decision being judged again ([R-6](research.md#r-6-automatic-duplicate-decisions-are-judged-again-in-place)) |

## Rules

| Rule | Enforced in |
| --- | --- |
| A `folder` scope has `rescan_folder_id` and `rescan_path`; a `video` scope has `rescan_video_id`; `library` has neither | `ScanStore.StartRescan` |
| A resumed scan copies the rescan columns from the interrupted scan and its `scan_videos.forced` marks | `ScanStore.ResumeScan` |
| `force` is copied to the job a result queues: `probe` to `preview`, `seek_thumbnail` to `fingerprint` | `ApplyProbeForJob`, `SetSeekThumbnailStateForJob` |
| Queuing a forced job over a `queued` row sets its `force`; a `running` row is left as it is | `IngestStore.ForceJobs` |
| A candidate with `rejudge_of` is claimable although the pair is bundled or dismissed, and is removed once the new verdict is applied | `ClaimJudgement`, `ApplyJudgement` |

## What does not change

| Unchanged | Why |
| --- | --- |
| `videos.*_state` | Force lives on the job, so the old thumbnail stays visible (R-2) |
| `thumbnail_images`, `video_overrides.thumbnail_position_ms`, tags, bundles the owner made | User data survives a rebuild ([ARCHITECTURE.md](../../ARCHITECTURE.md#cross-cutting-invariants)); the forced thumbnail job reads the image and the position as the ordinary job does |
| `auto_tag_queue`, `face_queue` | A forced seek stage queues nothing ([R-5](research.md#r-5-a-forced-seek-stage-queues-no-follow-up-process)) |
