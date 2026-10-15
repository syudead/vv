# Contract: Tag creation settings in the screen API

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml), operations
`getAutoTaggingSettings`, `updateAutoTaggingSettings`, `getOllamaSettings`,
`updateOllamaSettings` and `checkOllama`. This document describes only the
fields this feature adds; every route stays owner only. The external API and
MCP do not change: a created tag is a tentative tag they already serve.

## Schema changes

| Schema | Field | Required | Meaning |
| --- | --- | --- | --- |
| `AutoTaggingSettings` | `createTags: boolean` | Yes | "Create new tags" ([R-8](../research.md#r-8-settings-for-tag-creation)) |
| `AutoTaggingSettings` | `tagLanguage: string` | Yes | "Language for new tags" |
| `UpdateAutoTaggingSettingsRequest` | `createTags: boolean` | No | Absent keeps the saved value |
| `UpdateAutoTaggingSettingsRequest` | `tagLanguage: string` | No | Absent keeps the saved value |
| `OllamaSettings` | `generationModel: string` | Yes | The generation model ([R-3](../research.md#r-3-a-separate-generation-model-default-gemma4e4b)); now the `GET` and `PUT` response only |
| `UpdateOllamaSettingsRequest` (new) | `endpoint`, `model` (required, as today) and `generationModel` (optional) | — | The `PUT` request body in place of `OllamaSettings`; an absent `generationModel` keeps the saved value |

The request fields are optional so that a screen built before this feature
still saves the other settings. A schema's `required` applies to both
directions, so the Ollama `PUT` gets its own request schema, as the
auto-tagging `PUT` already has.

## `PUT /api/settings/auto-tagging`

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` (as for the thresholds today) | `tagLanguage` is empty after trimming, longer than 40 characters, or has a control character |

## `POST /api/settings/ollama/check`

The check reads `GET /api/tags` once and looks for both saved models. A missing
model answers `available: false`, and `message` names each missing model and
its role, judging or generation; a model name without a tag means `:latest`,
as today ([ai-layer.md](../../../docs/design-docs/ai-layer.md#ollama-provider)).
