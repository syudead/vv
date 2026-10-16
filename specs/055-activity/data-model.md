# Data model: Activity

The rest of the model is unchanged: the schema is the migrations in
[`internal/store/migrations`](../../internal/store/migrations), and the
tables this feature writes into are
[030 data-model](../030-video-versions/data-model.md),
[054 data-model](../054-auto-dup/data-model.md) and
[auto-tagging.md](../../docs/design-docs/auto-tagging.md).

## Migration

One migration, numbered after the last one on `main` when it is written
(`00050` today), creates `activity_entries`, `activity_entry_tags` and
`video_version_judging_holds`, and adds a nullable `activity_id` to `jobs`,
`auto_tag_queue` and `video_version_candidates`. Down drops them. No entry is
written for anything that happened before it (the parent Issue's last edge
case).

| Table | Category ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)) |
| --- | --- |
| `activity_entries`, `activity_entry_tags` | User data: never rebuilt, never pruned ([R-1](research.md#r-1-a-persistent-activity-log-named-apart-from-the-scan-activity)) |
| `video_version_judging_holds` | User data: the owner's undo, kept like `video_version_unbundles` |

## `activity_entries`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer | No | Primary key; the page order ([R-9](research.md#r-9-keyset-paging-by-id-newest-first-filtered-by-status)) |
| `kind` | text | No | One of the kinds below |
| `subject_kind` | text | No | `video`, `pair`, `scan` or `folder` |
| `subject_key` | text | No | A content key, the smaller key of a pair, a scan id, or a removed media folder's path ([R-10](research.md#r-10-the-subject-is-a-content-key-with-a-label-kept-at-write-time)) |
| `subject_key_b` | text | Yes | The larger key of a pair |
| `subject_label` | text | No | The display name (both, for a pair), the scan's origin, or the folder path, as written |
| `status` | text | No | `running`, `attention` or `done` ([R-4](research.md#r-4-three-statuses-with-a-reason-and-a-newer-run-closes-an-older-problem)) |
| `reason` | text | Yes | For `running` and `attention`: see Reasons |
| `outcome` | text | Yes | For `done`: see Outcomes |
| `detail` | text | Yes | The provider's or stage's message for a failure, the failed stage, the tag or bundle the owner operation changed |
| `decider` | text | No | `rule`, `ai` or `owner` ([R-5](research.md#r-5-the-decider-is-who-made-the-recorded-decision-else-who-started-the-run)) |
| `probability` | real | Yes | The model's answer for a duplicate pair it decided or left undecided |
| `undo_keys` | text | Yes | For a bundled pair, a JSON object with the member keys of the side that joined and that side's former representative key ([R-7](research.md#r-7-undo-restores-the-state-and-always-records-the-exclusion)) |
| `undone_by` | integer | Yes | The owner entry that undid this one |
| `created_at`, `updated_at` | integer | No | Unix milliseconds |

**Relationships**: none to `videos`; `undone_by` refers to another entry.
Indexes on `(status, id)` and `(kind, subject_key, subject_key_b, id)`.

### Kinds

| `kind` | Subject | Written by |
| --- | --- | --- |
| `import` | video | Queuing a video's ingest jobs when it has no pending job ([R-3](research.md#r-3-one-entry-per-run-and-subject-an-import-is-one-entry-for-all-its-stages)) |
| `scan` | scan | A scan that ends `failed` for a reason other than `interrupted` |
| `auto_tag` | video | Queuing a video for auto-tagging |
| `version_judgement` | pair | A judgeable candidate appearing while judging is on, or judging being turned on |
| `work_info_fetch` | video | A plugin fetch ([R-11](research.md#r-11-a-failed-work-information-fetch-is-a-finished-entry-not-a-problem)) |
| `bundle`, `unbundle`, `make_representative`, `dismiss` | video or pair | The owner's version operations ([R-6](research.md#r-6-the-owner-operations-recorded)) |
| `tag_attach`, `tag_detach`, `tag_keep`, `tag_reject` | video | The owner's tag operations, one entry per video and tag |
| `work_info_apply` | video | Applying fetched work information |
| `media_folder_remove` | folder | Removing a media folder; `detail` holds the number of videos it removed |
| `organize_undo` | video | The drag-organize undo, one entry per changed video |
| `undo` | as the undone entry | Undoing an automatic decision ([R-7](research.md#r-7-undo-restores-the-state-and-always-records-the-exclusion)) |

### Reasons

| `reason` | Status | Kinds |
| --- | --- | --- |
| `waiting` | running | `import`, `auto_tag`, `version_judgement` before a worker claims it |
| `processing` | running | Claimed by a worker |
| `failed` | attention | `import` (a stage gave up after `MaxJobAttempts`; `detail` names it), `scan`, `auto_tag`, `version_judgement` (with its retry time) |
| `interrupted` | attention | An entry `running` with reason `processing` at startup |
| `no_connection` | attention | `version_judgement` that needs the model with no Ollama connection saved |
| `undecided` | attention | `version_judgement` left to the owner |

### Outcomes

| `outcome` | Meaning |
| --- | --- |
| `completed` | The run finished or the operation applied |
| `same`, `different` | A `version_judgement` bundled or dismissed the pair |
| `failed` | A `work_info_fetch` that returned an error |
| `superseded` | A newer entry of the same kind on the same subject started while this one was `attention` |
| `decided_by_owner` | The owner bundled or dismissed the pair of an open `version_judgement` |
| `gone` | The candidate of an open `version_judgement` disappeared with a video |
| `already_reverted` | An `undo` whose target state had already been changed by the owner |

## `activity_entry_tags`

The tags an `auto_tag` entry added; undo is per tag.

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `entry_id` | integer | No | The `auto_tag` entry; primary key with `tag_id` |
| `tag_id` | integer | No | The tag |
| `tag_name` | text | No | The name as written |
| `suggested` | integer | No | 1 when added as a suggested tag |
| `user_key` | text | No | The user key the attachment was written at (a content key or a bundle's key) |
| `attached_at` | integer | No | The `video_tags.created_at` written; undo deletes only a row that still has it |
| `probability` | real | No | The model's answer |
| `undone_by` | integer | Yes | The `undo` entry |

## `video_version_judging_holds`

Pairs whose automatic "Different videos" the owner undid; the judging worker
never claims them.

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `key_a`, `key_b` | text | No | Content keys, `key_a < key_b`; primary key |
| `created_at` | integer | No | When the undo happened |

## Rules

| Rule | Enforced in |
| --- | --- |
| An entry is written or updated in the transaction of the change it records, and `ActivityChanged` is published after commit ([R-2](research.md#r-2-an-entry-is-written-in-the-transaction-of-the-change-it-records)) | `store` (one package-private helper) |
| A job and an auto-tag queue row point to the entry they belong to; claiming a row whose entry is not `running` opens a new entry | `store` |
| A candidate inserted for a pair, including by a rebuild, attaches to that pair's open `version_judgement` entry, found by `(kind, subject_key, subject_key_b)`; an `interrupted` one returns to `running` on its next claim | `store` |
| An open `version_judgement` closes as `gone` only when one of its keys has left the library | `store` |
| Turning judging off leaves open `version_judgement` entries `waiting`; they are claimed again when it is turned on | `store` |
| An `import` entry is `running` while any of its jobs is pending, `attention` when one gave up, else `done` | `domain` (the rule over the stage states), `store` |
| A new entry of a kind on a subject closes that kind and subject's `attention` entries as `superseded`; a scan that ends `done` closes every open `scan` entry; a `version_judgement` keeps one entry per pair through its retries | `domain`, `store` |
| Startup, before the workers start, turns every entry `running` with reason `processing` into `attention`/`interrupted`; `waiting` entries stay | `store`, called from `cmd/mdm` beside `Scans.RecoverInterrupted` |
| Retry is offered for `attention` with reason `failed` or `interrupted`, of kind `import`, `scan`, `auto_tag` or `version_judgement` | `domain` |
| Undo is offered for an entry decided by `rule` or `ai` that is not yet undone: a `version_judgement` with outcome `same` or `different`, or a tag of an `auto_tag` entry | `domain` |
| The judging claim skips pairs in `video_version_judging_holds` as it skips `video_version_unbundles` | `store` (`judgeablePairs`) |
| A content change at the same path re-keys `video_version_judging_holds` and the subject keys of open entries as `moveKeyPairs` re-keys dismissals | `store` (successions) |

## What does not change

`video_version_dismissals`, `video_bundles`, `video_tags` and the queues keep
their meaning: the decider lives in the entry, not on the rows the decision
wrote, so 054's "an automatic bundle is the same rows as the owner's" still
holds.
