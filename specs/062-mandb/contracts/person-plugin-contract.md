# Contract: Person plugin modules

Source of truth: [Write a work-info plugin](../../../docs/how-to/work-info-plugins.md),
contract `1`. This file describes only what a person plugin adds or changes;
the module format, host functions, settings, limits and address rule are
unchanged ([R-2](../research.md#r-2-the-person-plugin-contract)).

## Manifest

| Field | Type | Required | Rule |
| --- | --- | --- | --- |
| `kind` | string | No | `work` (the default when absent) or `person` |
| `sources` | string array | With `"kind": "person"` | 1 to 10 distinct external-ID sources, each following the people API's `source` rule (1 to 50 characters); not allowed for `work` |
| `operations` | string array | Yes | For `person`, exactly `search` and `fetch` |

A manifest that breaks a rule is refused with `manifest_invalid`, naming the
field, as for the existing fields. `mdm plugin pack` prints the kind and the
sources with the rest.

## Request

| Operation | Body |
| --- | --- |
| `search` | `{"contract": 1, "operation": "search", "query": "<the person's name, 1 to 200 characters>", "settings": {...}}` |
| `fetch` | `{"contract": 1, "operation": "fetch", "source": "<one of sources>", "id": "<1 to 200 characters>", "settings": {...}}` |

`source` is present only in a person plugin's `fetch`; a work-info plugin's
request does not change.

## Response

| Case | Body |
| --- | --- |
| `search` result | `{"candidates": [{"source": "...", "id": "...", "name": "...", "url": "...", "detail": "..."}]}`: `source` (one of `sources`), `id`, `name` and `url` required, `detail` optional; VVMDM keeps the first 50, and a candidate with another `source` fails the run with `plugin_failed` |
| `fetch` result | `{"person": {"name": "...", "aliases": [{"name": "...", "language": "ja"}], "externalIds": [{"source": "...", "value": "..."}], "image": "https://...", "profile": {...}}}`: `name` required, the rest optional |
| Failure | Unchanged |

`image` and `profile` follow the rules of a person in a work-info result
([People's images and profiles](../../../docs/how-to/work-info-plugins.md#peoples-images-and-profiles)).
VVMDM keeps the first 50 aliases. It puts the request's `(source, id)` first
in `externalIds`, then keeps the plugin's distinct IDs after it up to 20 in
all, so the ID a person was found by always survives the limit.
VVMDM fetches the one `image` after the run, through the cover's path.

## `mdm plugin run`

`mdm plugin run plugin.wasm fetch <id> --source <source>` runs a person
plugin's fetch; `--source` is required for a person plugin and refused for a
work-info plugin, with exit code 2. `search` takes the name as input. The
output is `{"candidates": [...]}` or `{"person": {...}}` after VVMDM's limits.
