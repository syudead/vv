# Data model: In-app assistant over a local LLM

The rest of the model is unchanged:
[internal/store/migrations](../../internal/store/migrations). Proposals and
review rows are user data: they name videos by user key, carry no foreign key
to `videos`, and are never rebuilt.

## Migration

`00081_assistant.sql` creates `assistant_proposals`, `assistant_review_runs`,
`assistant_review_candidates` and `assistant_review_dismissals`, and adds the
`assistant_run` kind to the `activity_entries.kind` check.

## `settings`

| Key | Value | Meaning |
| --- | --- | --- |
| `ollama.generation_model` | A model name | Absent or empty until the owner saves one; read on every generator call |

## `assistant_proposals`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer primary key | No | The proposal |
| `status` | text | No | `proposed`, `approved`, `running`, `done`, `partial`, `stale`, `failed`, `discarded` |
| `request` | text | No | The owner's text, the activity entry's label |
| `plan` | text (JSON) | No | `domain.AssistantPlan`: the operation, the resolved ids with the labels the card showed, the arguments, the user keys of video targets |
| `total`, `applied`, `failed` | integer | No | Counts in user keys, or 0 and 1 for an operation without video targets |
| `error` | text | Yes | The first failed batch's `code` and `reason` |
| `activity_id` | integer | Yes | The `assistant_run` entry, set on approval |
| `created_at`, `approved_at`, `started_at`, `finished_at` | integer | `created_at` no, others yes | Unix milliseconds |

**Relationships**: `activity_id` refers to `activity_entries` without a
foreign key, as queue rows do.

## `assistant_review_runs`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer primary key | No | The run |
| `kind` | text | No | `tags`, `people`, `works` |
| `status` | text | No | `running`, `done`, `failed`, `interrupted` |
| `total`, `judged`, `listed` | integer | No | Candidate sets found, judged so far, and listed at or above 0.5 |
| `error` | text | Yes | The classifier's message when the run failed |
| `started_at`, `finished_at` | integer | `finished_at` yes | Unix milliseconds |

## `assistant_review_candidates`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer primary key | No | The candidate set |
| `run_id` | integer | No | The run |
| `members` | text (JSON) | No | `[{ kind, id, name }]`: tags or people by id, videos by user key; two for `tags` and `people`, two or more for `works` |
| `set_key` | text | No | The members' ids sorted and joined, the dismissal key |
| `probability` | real | No | The classifier's answer |
| `state` | text | No | `listed`, `adopted`, `rejected` |
| `proposal_id` | integer | Yes | The adoption's proposal |

**Relationships**: `run_id` references `assistant_review_runs` and cascades
on delete. Members are checked for existence when the list is read.

## `assistant_review_dismissals`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `kind` | text | No | The run kind |
| `set_key` | text | No | The rejected set |
| `rejected_at` | integer | No | Unix milliseconds |

Primary key `(kind, set_key)`.

## `activity_entries`

| Change | Meaning |
| --- | --- |
| Kind `assistant_run` | One per approved proposal; subject the proposal id, label the request, detail the operation and the counts |
| Feature `assistant` | The Activity screen's filter for these entries |

## `VideoQuery` and `SmartRule`

| Field | Type | Meaning |
| --- | --- | --- |
| `MinDurationMs`, `MaxDurationMs` | int64, 0 means unset | Duration bounds |
| `AddedFrom`, `AddedTo` | calendar day, zero means unset | Day bounds in the server's local time zone |
| `WatchState` | `unwatched`, `inProgress`, `watched`, empty means unset | From the playback position rows |

## Rules

| Rule | Enforced in |
| --- | --- |
| `proposed` moves only to `approved` or `discarded`; `approved` only to `running`; `running` only to `done`, `partial`, `stale` or `failed` | `AssistantStore`, one guarded `UPDATE` per transition, with `domain.AssistantProposal.CanMove` |
| A `proposed` row is deleted 1 hour after `created_at` | The assistant worker, and startup |
| A `running` row at startup becomes `failed` with error `interrupted`, and its entry `attention`/`interrupted` | Startup recovery |
| Video targets are split into batches of 500 user keys, in plan order | `domain.AssistantPlan.Batches` |
| Before the first batch, every label in the plan is re-read; a missing video or a changed label makes the run `stale` | The worker, through `AssistantStore.CurrentLabels` |
| At most one `running` review run per kind | A partial unique index on `(kind)` where `status = 'running'` |
| A set whose `set_key` is dismissed for the kind is not judged | The finder's query |
| A smart rule rejects a negative duration, a bound pair in the wrong order, a day that does not parse and an unknown watch state | `domain.SmartRule.Validate` |

## State transitions

The proposal's states are in
[plan.md, Proposals and execution](plan.md#proposals-and-execution); a
review run moves from `running` to `done`, `failed` or `interrupted` once.

## What does not change

| Entity | Why |
| --- | --- |
| `video_tags`, `favorites`, `playlist_items`, `group_items`, `smart_folders`, `tags`, `people`, `organizations` | The assistant writes them through the operations the screens use, which write their own rows and entries |
| `auto_tag_queue`, `version_candidates` | Review runs have their own tables; a tag or people merge adopted from the list runs the same merge the admin screens run |
| `ollama.endpoint`, `ollama.model` | The decision model and the endpoint keep their keys; the generation model is a third key |
