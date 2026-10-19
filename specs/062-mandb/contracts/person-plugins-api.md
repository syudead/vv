# Contract: Person plugin screen API

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml), operations
`listWorkInfoPlugins`, `uploadWorkInfoPlugin`, `searchWorkInfo`,
`fetchWorkInfo`, `applyWorkInfoFetch` and `getWorkInfoFetchPersonImage`,
which this feature extends, and the new operations below. All are owner-only.
Run failures answer as the existing plugin routes do (`502 plugin_failed`,
`503 plugin_busy`, `504 plugin_timed_out`, and the rest of Run failures in
[work-info-plugins.md](../../../docs/design-docs/work-info-plugins.md)).

## Changes to existing operations

| Operation | Change |
| --- | --- |
| `WorkInfoPlugin` schema | Adds `kind` (`work` or `person`), `sources` (empty for `work`) and `autoLookup` (always `false` for `work`) |
| `searchWorkInfo` | A person plugin answers `409 plugin_kind_mismatch` |
| `fetchWorkInfo` | A person plugin answers `409 plugin_kind_mismatch`; the result adds `personLookups` (below) and, per fetched person, `lookup` (the index into `personLookups`, or absent) and `lookupLimited` (`true` when an accepted ID was over the limit of 5) |
| `applyWorkInfoFetch` | The request adds `personLookups: [{index, items}]`; the response's `skipped` may carry the reasons `taken`, `stale` and `person_changed` |

`personLookups[i]` in the fetch result is `{pluginId, pluginName, source, id}`
in plan order ([R-4](../research.md#r-4-the-lookup-plan-is-decided-by-the-server-at-fetch-time)).

An apply item is `image`, `aliases`, `externalIds` or `profile.<field>`.
Selecting an item whose lookup has no result, or whose entries' role is not in
`fields`, answers `400 invalid_request`
([R-6](../research.md#r-6-a-lookup-is-applied-per-item-only-through-a-selected-role)).
The skip reasons are in
[R-6](../research.md#r-6-a-lookup-is-applied-per-item-only-through-a-selected-role).

## `POST /api/work-info-plugins/{id}/person-search`

Runs a person plugin's search once with `{"query": "..."}` (1 to 200
characters after trimming) and returns `{candidates: [{source, id, name, url,
detail?}]}`. A work-info plugin answers `409 plugin_kind_mismatch`; the other
errors are `searchWorkInfo`'s.

## `POST /api/videos/{id}/work-info-fetches/{fetchId}/person-lookups/{index}`

Runs planned lookup `index` once and keeps its result with the fetch.

**Response**: `200` with `PersonPluginResult`: `person` (the fetched name,
aliases, external IDs, profile, `hasImage`) and `current` (the matched
person's aliases, external IDs, profile and `imageRevision`, or `null` when no
existing person matched). Every entry of a lookup matched that same person
([R-4](../research.md#r-4-the-lookup-plan-is-decided-by-the-server-at-fetch-time)),
so one `current` and one item selection per lookup cover them all. A second request for a lookup that has a result
answers the kept result without running; a failed lookup keeps nothing and may
be requested again.

| Status | `code` | When |
| --- | --- | --- |
| `404` | `not_found` | Unknown video, or a fetch of another video |
| `404` | `not_found` | `index` outside `personLookups` |
| `409` | `work_info_fetch_stale` | The fetch expired or was applied |
| `409` | `plugin_changed` | The person plugin was replaced, revoked or deleted since the fetch |

## `GET /api/videos/{id}/work-info-fetches/{fetchId}/person-lookups/{index}/image`

Serves the lookup's kept image, as `getWorkInfoFetchPersonImage` serves a
fetched person's; `404 person_image_not_found` when it has none.

## `PUT /api/work-info-plugins/{id}/auto-lookup`

Sets `autoLookup` from `{"enabled": true|false}` and returns the plugin.
A work-info plugin answers `409 plugin_kind_mismatch`; a missing one
`404 not_found`.

## `POST /api/people/{id}/plugin-fetches`

Runs an approved person plugin's fetch for the person and keeps the result
for 10 minutes ([R-7](../research.md#r-7-the-person-page-fetches-by-a-stored-id-chosen-by-the-server)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `pluginId` | body | integer | Yes | The person plugin |
| `source`, `id` | body | string | Together or neither | A search candidate's ID; when absent, the person's first stored ID the plugin accepts |

**Response**: `201` with `{fetchId, expiresAt, person}`, `person` shaped as in
`PersonPluginResult`.

| Status | `code` | When |
| --- | --- | --- |
| `404` | `person_not_found` | The person does not exist |
| `409` | `person_external_id_missing` | No `source` was sent and the person has no ID the plugin accepts |
| `409` | `plugin_kind_mismatch` | A work-info plugin |
| `400` | `invalid_request` | A `source` the plugin does not declare, or an `id` outside 1 to 200 characters |

## `GET /api/people/{id}/plugin-fetches/{fetchId}/image`

Serves the result's kept image; `404 not_found` for an unknown, expired or
other person's result, `404 person_image_not_found` without an image.

## `POST /api/people/{id}/plugin-fetches/{fetchId}/apply`

Applies the selected `items` (as in `applyWorkInfoFetch`) under the rules of
[R-6](../research.md#r-6-a-lookup-is-applied-per-item-only-through-a-selected-role),
consumes the result, and returns `{person, skipped}`.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | Empty `items`, or an item without a value in the result |
| `404` | `person_not_found` | The person was deleted or merged away |
| `409` | `work_info_fetch_stale` | Expired or unknown result, or the plugin replaced, revoked or deleted since |
