# Data model: In-app assistant as a server-side tool agent

The rows that `00088_assistant.sql` created on the feature branch stay as
they are, except for the changes below: `assistant_proposals`, the review run
tables, the `assistant_run` activity kind and `ollama.generation_model`
([`internal/store/migrations/00088_assistant.sql`](../../internal/store/migrations/00088_assistant.sql)).
The rest of the model is unchanged. `00088` is not edited, because databases
on the feature branch have applied it.

## Migrations

| Migration | Change |
| --- | --- |
| `00089_assistant_steps.sql` | Adds `steps` (JSON) to `assistant_proposals`, moves nothing, and deletes the rows written by the one-shot build |
| `00090_assistant_turns.sql` | Creates `assistant_turns` |

## `assistant_proposals`

| Field | Change | Meaning |
| --- | --- | --- |
| `plan` | Kept; holds `{ steps: [AssistantPlan] }` | The steps exactly as the card showed them, each a shipped plan plus `createdBy` |
| `steps` | New, text (JSON), not null, default `[]` | Per step: `status`, `total`, `applied`, `failed`, `error` |
| `turn_id` | New, integer, null | The turn that built it; null for an adoption from a review list |
| `total`, `applied`, `failed`, `error` | Kept | The sums over the steps, and the error of the step the run stopped at, which the activity entry shows |

## `assistant_turns`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `id` | integer primary key | No | The turn |
| `status` | text | No | `running`, `done`, `clarify`, `stopped`, `interrupted` |
| `request` | text | No | The owner's text |
| `context` | text (JSON) | No | The screen as the panel sent it |
| `previous_turn_id` | integer | Yes | The turn a follow-up continues |
| `progress` | text (JSON) | No | The progress items in order |
| `answer` | text | Yes | The final text after the grounding check |
| `judged` | integer (0 or 1) | No | Whether the answer used `clef` answers |
| `directive` | text (JSON) | Yes | The last filter or navigation |
| `clarify` | text (JSON) | Yes | The name and its candidates |
| `proposal_id` | integer | Yes | The proposal the turn built |
| `stop` | text (JSON) | Yes | The reason a `stopped` turn ended, and an excluded operation |
| `started_at`, `finished_at` | integer | `finished_at` yes | Unix milliseconds |

**Relationships**: `previous_turn_id` and `proposal_id` refer to rows
without foreign keys, since either side expires on its own schedule.

## Rules

| Rule | Enforced in |
| --- | --- |
| At most one `running` turn | A partial unique index on `status` where `status = 'running'` |
| A `running` turn at startup becomes `interrupted` | Startup recovery |
| A turn finished more than 24 hours ago is deleted | The assistant worker |
| A turn moves from `running` to one finished status once | One guarded `UPDATE` |
| A step moves `pending` to `skipped` on approval, or to `running` and then `done`, `partial`, `stale` or `failed`; steps after the one the run stopped at become `notRun` | `domain.AssistantProposal`, written by the worker |
| A kept step whose `createdBy` names a step that is not kept refuses approval | `domain.AssistantProposal.Select` |
