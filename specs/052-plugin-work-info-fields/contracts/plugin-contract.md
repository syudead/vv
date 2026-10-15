# Contract: Work details in the plugin's fetch response

Source of truth:
[Write a work-info plugin, Response](../../../docs/how-to/work-info-plugins.md#response),
which the "Accept work details in a plugin's fetch response" unit updates.
Contract version 1 is unchanged otherwise; the request, the host functions,
the limits and the addresses stay as published
([R-1](../research.md#r-1-the-fetch-response-gains-optional-work-detail-fields-in-contract-1)).

## `fetch` result

`{"workInfo": {...}}` gains these optional properties beside `title` and
`tags`. VVMDM trims every string of these properties, drops an entry that is
empty after trimming or whose `kind`, `role`, `source` or `value` is missing
or unknown, and keeps the first entries up to each limit, counted after
dropping. `title` and `tags` are read exactly as before: this trimming does
not reach them (requirement 8).

| Property | Shape | Kept |
| --- | --- | --- |
| `code` | string | — |
| `description` | string; line feeds allowed | — |
| `dates` | `[{"kind": "release" / "recorded" / "published", "date": "YYYY", "YYYY-MM" or "YYYY-MM-DD"}]` | The first entry per kind |
| `people` | `[{"role": "performer" / "director" / "photographer" / "streamer", "name": "...", "externalIds": [{"source": "...", "value": "..."}]}]`; `externalIds` optional | The first 100 per role, and the first 20 external IDs per entry |
| `organizations` | `[{"role": "maker" / "label", "name": "...", "externalIds": [...]}]` | The first 20 per role, and the first 20 external IDs per entry |
| `urls` | `["https://..."]` | The first 20 distinct |
| `externalIds` | `[{"source": "...", "value": "..."}]` | The first 20 distinct `(source, value)` |

A value that breaks a rule of
[Set work information](../../../docs/how-to/external-api.md#set-work-information)
(a `code` over 100 characters, a `date` in another form, a URL that is not
absolute) is kept in the result and shown to the owner; the apply skips it
and tells the owner
([R-4](../research.md#r-4-the-apply-merges-the-selected-fields-into-the-stored-work-information-in-one-transaction-and-skips-each-value-that-breaks-a-rule)).
The same person twice in one role is kept once.

## Client use

A plugin that returns only `title` and `tags` needs no change. A plugin
returns the external site's own identifier for a person as
`externalIds: [{"source": "<site>", "value": "<id>"}]` so that VVMDM links
the same performer across fetches and across plugins that use the same
`source`
([R-3](../research.md#r-3-people-and-organizations-are-matched-by-external-id-then-by-folded-name-or-alias-and-an-ambiguous-match-links-nothing)).
