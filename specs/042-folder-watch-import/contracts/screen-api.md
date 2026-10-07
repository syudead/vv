# Contract: Auto-import setting and scan origin

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml) (`Scan`, and the
new `getAutoImportSettings` and `updateAutoImportSettings`). Both are
owner-only, like every route under `/api/settings`. The external API
(`api/external-v1.yaml`) does not change: an external client's scan is
`manual`.

## `Scan.origin`

`Scan` ([024 contract, `Scan`](../../024-import-progress/contracts/scan-api.md#scan))
gains one required field.

| Field | Type | Meaning |
| --- | --- | --- |
| `origin` | `manual` \| `watch`, required | Who started the import ([data-model.md, `scans.origin`](../data-model.md#scansorigin-added-column)) |

The `scan` event on `/api/events` carries it unchanged. A client shows a `watch`
scan's progress nowhere and notifies it only when `status` is `partial` or
`failed` (`要件 7`); Settings lists its issues as it does today.

## `GET /api/settings/auto-import`

Returns the stored choice and what the watcher is doing now.

**Response**: `200` with `AutoImportSettings`.

| Field | Type | Meaning |
| --- | --- | --- |
| `enabled` | boolean, required | The stored choice ([data-model.md, `settings` key](../data-model.md#settings-key-libraryauto_import)) |
| `watch.state` | `off` \| `starting` \| `active` \| `limited`, required | `off` when disabled or no media folder exists; `starting` while the watches are being added; `active` when every directory is watched; `limited` when a watch problem exists ([R-6](../research.md#r-6-lost-events-and-watch-limits-are-reported-never-repaired-by-a-scan)) |
| `watch.problem` | `watch_limit` \| `events_lost` \| `folder_unreachable` \| `permission_denied` \| omitted | The most recent problem; present only when `state` is `limited` |
| `watch.path` | string \| omitted | The directory the problem concerns, when it concerns one |

A problem clears when the owner turns auto-import off and on, when the media
folders change, or when a manual scan finishes `done` (`events_lost` only:
the full read covers the lost events).

## `PUT /api/settings/auto-import`

Stores the choice and arms or removes the watches. Starts no scan (`要件 6`).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `enabled` | body | boolean | yes | The new choice |

**Response**: `200` with `AutoImportSettings` after the change; `watch.state` is
`starting` or `off`. The same value as today changes nothing.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | The body is not `{ "enabled": boolean }` |

## Client use

Settings reads `GET /api/settings/auto-import` when the section mounts and when
the window regains focus, and every 2 s while `watch.state` is `starting`, so
the state settles on screen without a push event or a focus change.
