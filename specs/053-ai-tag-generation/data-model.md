# Data model: Tag creation by the AI during auto-tagging

The rest of the model is unchanged: tags, names, rejected names and
assignments stay as in [014 data-model](../014-video-tags/data-model.md) and
[031 data-model](../031-tentative-tags/data-model.md), kinds as in
[051 data-model](https://github.com/syudead/vv/blob/feature/051-tag-kinds/specs/051-tag-kinds/data-model.md),
and the queue as in
[`00036_auto_tag_queue.sql`](../../internal/store/migrations/00036_auto_tag_queue.sql).
This file records the two queue columns, the settings and the domain values
this feature adds.

## Migration

One migration, numbered when it lands, adds two columns to `auto_tag_queue`
and drops them in its `Down` section. Existing rows get `attempts = 0` and
`retry_at = 0`, so a row that is `failed` today waits for the owner as before.

## `auto_tag_queue`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `attempts` | `integer not null default 0` | No | Failed runs since the row was last queued by a trigger |
| `retry_at` | `integer not null default 0` | No | Unix milliseconds after which a `failed` row may be claimed again; 0 means never |

The table stays rebuildable, as today: losing it re-judges videos and loses no
user data.

## Rules

| Rule | Enforced in |
| --- | --- |
| `Fail` increments `attempts` and sets `retry_at` to now plus 10 minutes, 1 hour or 6 hours for `attempts` 1, 2 or 3, and 0 from 4 ([R-9](research.md#r-9-failed-jobs-retry-automatically-three-times)) | `domain` (the delays), `AutoTagStore.Fail` |
| `Claim` takes the oldest `queued` row, or else the `failed` row with the earliest `retry_at` that is not 0 and not in the future | `AutoTagStore.Claim` |
| Every trigger that queues a row sets `attempts = 0` and `retry_at = 0` | `AutoTagStore.queue` |
| The worker sleeps until the earliest `retry_at` when nothing is queued | `app.AutoTagger.Run` |
| A returned name is attached by the table in [R-7](research.md#r-7-how-a-returned-name-is-attached), in the same transaction that records the job as done | `AutoTagStore.Finish`, through `resolveTagNames` |
| A created tag's `kind_id` is the id of the kind whose name the model chose, read in that transaction; null when the kind no longer exists | `AutoTagStore.Finish` |

## Settings

Stored in the `settings` table beside the existing auto-tagging and Ollama
keys ([`internal/store/auto_tags.go`](../../internal/store/auto_tags.go)).

| Key | Value | Default when absent |
| --- | --- | --- |
| `auto_tag.create_tags` | Boolean | On ([R-8](research.md#r-8-settings-for-tag-creation)) |
| `auto_tag.tag_language` | 1 to 40 characters after trimming, no control characters | `Japanese` |
| `ollama.generation_model` | A model name, normalised as `ollama.model` | `gemma4:e4b` ([R-3](research.md#r-3-a-separate-generation-model-default-gemma4e4b)) |

## Values added to `domain`

| Value | Holds |
| --- | --- |
| `GeneratorRequest` | `Input ClassifierInput`, `Instructions string`, `Prompt string`, `Schema []byte` ([R-1](research.md#r-1-a-second-task-agnostic-port-for-generation)) |
| `AutoTagSettings.CreateTags`, `AutoTagSettings.TagLanguage` | The two settings above |
| `OllamaSettings.GenerationModel` | The setting above |
| `AutoTagVocabulary` | The tags, synonyms, kinds and rejected names the prompt lists ([R-5](research.md#r-5-reuse-comes-from-the-prompt-and-from-name-lookup), [R-6](research.md#r-6-kinds-are-described-by-name-examples-and-preset-hints)) |
| `AutoTagProposal` | One returned `{name, kind}` |
| `AutoTagSelection.Proposals` | The proposals `Finish` resolves, in the model's order |

## What does not change

- `tags`, `tag_names`, `video_tags`, `rejected_tag_names` and
  `auto_tag_rejections`: a created tag is an ordinary tentative tag, so the
  tag screen, the external API and MCP confirm, reject and merge it as they
  do today (requirement 7).
- `domain.AutoTagApplied`: `Finish` already publishes it for the video;
  creating tags publishes nothing, as no tag write does.
