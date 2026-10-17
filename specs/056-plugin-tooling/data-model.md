# Data model: Work-info plugin tooling

The rest of the model is unchanged:
[internal/store/migrations/](../../internal/store/migrations/) and
[049 data-model.md](../049-work-info-plugins/data-model.md). The proxy is a
row of the existing `settings` table, and the new manifest fields travel in
the existing `work_info_plugins.settings` JSON, so neither needs a migration.

## Migration

`00054_work_info_plugin_runs.sql` creates `work_info_plugin_runs`. It
takes the next number after the migrations that landed on `main` first
(`00050` to `00053`); if another migration lands on `main` before this one,
the number moves to the next free one.

## `work_info_plugin_runs`

The last run's output per plugin
([R-6](research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked)).

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `plugin_id` | integer | No | Primary key; the plugin |
| `generation` | integer | No | The plugin row's `generation` when the run started; it changes on every replacement, including one with the same bytes |
| `operation` | text | No | `search` or `fetch` |
| `failure` | text | Yes | The run failure code; null for a successful run |
| `ran_at` | integer | No | Unix seconds when the run ended |
| `stderr` | text | No | The first 64 KiB of standard error, secret values masked |
| `truncated` | integer | No | 1 when output after 64 KiB was dropped |

**Relationships**: `plugin_id` references `work_info_plugins.id` with
`on delete cascade`. The table is owner data like `work_info_plugins`, listed
in [Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery).

## `settings`

| Key | Value | Meaning |
| --- | --- | --- |
| `work_info_plugins.proxy_url` | The proxy URL of [contracts/screen-api.md](contracts/screen-api.md#get-and-put-apisettingsplugin-proxy) | No row or `""` means no proxy ([R-4](research.md#r-4-the-proxy-is-one-url-in-settings-read-on-each-run-and-passed-to-the-runtime-as-a-run-option)) |

## Rules

| Rule | Enforced in |
| --- | --- |
| The run row is written only when `work_info_plugins` still has the row with the generation that ran; otherwise nothing is written | `internal/store`, one statement conditioned on `id` and `generation` |
| Run rows are written in the order the runs end: the app writes the row in the runtime's `record` call, before the run slot is released | `internal/app`, `internal/wasmplugin` |
| A truncated output has a trailing start of a secret value masked as well as every whole value | `internal/domain`, the masking function |
| A replacing upload deletes the plugin's run row in the upload's transaction | `internal/store` |
| A setting's `default` and `options` follow [contracts/plugin-contract.md, Manifest](contracts/plugin-contract.md#manifest) | `internal/domain`, at upload and at `mdm plugin pack` |
| The effective value of a setting is the stored value when set and inside the options, otherwise the default; a stored value outside the options is kept and reported as `staleValue` | `internal/domain` (`Entry`, the request's values, the `required` check) |
| A stored value outside the options is replaced by the next write of that key | `internal/store`, as any value write |
| `work_info_plugins.proxy_url` follows the URL rule of the contract | `internal/domain`, checked by `internal/app` before the write |

## What does not change

`work_info_plugin_settings` keeps its columns: a default is never written
into it ([R-3](research.md#r-3-a-setting-declares-default-and-options-the-stored-value-or-the-default-is-what-the-run-receives)).
The fetch result stays in memory; the selected tag names are a parameter of
the apply, not stored.
