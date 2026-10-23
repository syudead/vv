# Contract deltas: person plugins fetch by a stored link

Each table lists only what this feature adds to a contract; the sources of
truth are [api/openapi.yaml](../../../api/openapi.yaml), the
[plugin how-to](../../../docs/how-to/work-info-plugins.md) and
[person-plugins.md](../../../docs/design-docs/person-plugins.md). The reasons
are in [plan.md, Decisions](../plan.md#decisions).

## Manifest

| Field | Type | Required | Rule |
| --- | --- | --- | --- |
| `domains` | string array | No | 1 to 10 distinct host names, lowercase, 1 to 253 characters of `a-z`, `0-9`, `-` and `.`, without scheme, port or path; allowed only with `"kind": "person"`, where it marks the plugin as accepting the `url` form of `fetch` |

A manifest that breaks the rule is `manifest_invalid` with the field
`domains` or `domains[i]`.

## Run request

| Request | Body |
| --- | --- |
| `fetch` by link | `{"contract": 1, "operation": "fetch", "url": "<the link as stored, an http or https URL>", "settings": {...}}`; sent only to a person plugin whose `domains` match the link's host |

The response is the same `person` object as for a fetch by ID. VVMDM
prepends no ID to `externalIds`; it keeps the plugin's distinct IDs, up to
20. A link matches a domain when its host, lowercased, equals the domain or
ends with `.` and the domain.

## HTTP API

`POST /api/people/{id}/plugin-fetches`:

| Request | Behaviour |
| --- | --- |
| `pluginId` only | The first stored accepted external ID; else the person's one matching link after folding; else `409 person_link_ambiguous` with the folded links as `links` when several match; else `409 person_external_id_missing` |
| `pluginId` and `link` | Fetches that link; `400 person_link_unsupported` when no declared domain matches its host, `400 invalid_request` when it is not an `http` or `https` URL or comes with `source` or `id` |
| `pluginId`, `source` and `id` | Unchanged |

| Response or schema | Delta |
| --- | --- |
| `PersonPluginFetch` | Optional `link`: the link the fetch ran with |
| `WorkInfoPlugin` | `domains`: the manifest's domains in order; empty for a work-info plugin and for a person plugin without them |
| `Error` | `links`: the folded matching links, in stored order; only for `person_link_ambiguous` |
| Error codes | `person_link_ambiguous` (409), `person_link_unsupported` (400) |

Folding treats links that differ only in scheme, host case or a trailing
slash as one and keeps the first stored one; query strings and fragments are
kept.

## CLI

| Command | Delta |
| --- | --- |
| `mdm plugin pack` | Prints `Domains: <list>` after the sources for a person plugin that declares them |
| `mdm plugin run <module> fetch <link> --url` | `--url` makes the positional input the link of a person plugin's fetch; `--url` with `--source`, on a work-info plugin, or on a plugin whose `domains` do not match the link exits with code 2 |
