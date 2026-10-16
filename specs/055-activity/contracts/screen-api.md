# Contract: Screen API for activity

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This document
holds only what this feature adds or changes. The error shape is
[023 error-api.md](../../023-english-i18n/contracts/error-api.md). Every
operation here is owner only (the default `security`), so a guest gets `401`
and `internal/httpapi`'s `accessRoutes` lists none of them (acceptance 8).
`api/external-v1.yaml` is unchanged.

## `GET /api/activity`

`operationId: listActivity`. Returns one page of entries, newest first, and
the counts the tabs show
([R-9](../research.md#r-9-keyset-paging-by-id-newest-first-filtered-by-status)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `status` | query | `running` \| `attention` \| `done` | No | Only entries in that status; all when absent |
| `cursor` | query | string | No | The `nextCursor` of the previous page |
| `limit` | query | integer, 1–100 | No | Default 50 |

**Response**: `200` with `ActivityPage {items: ActivityEntry[], nextCursor?,
counts: {running, attention, done}}`.

| `ActivityEntry` field | Type | Meaning |
| --- | --- | --- |
| `id` | integer | The entry |
| `kind`, `status`, `reason?`, `outcome?`, `decider`, `probability?`, `detail?` | as [data-model.md](../data-model.md#activity_entries) | |
| `subject` | `{kind, label, videoIds?}` | `videoIds` holds the current video id of each key the viewer can open; a key with no video is left out ([R-10](../research.md#r-10-the-subject-is-a-content-key-with-a-label-kept-at-write-time)) |
| `candidate` | boolean | For a pair: its candidate is still on Duplicates |
| `tags?` | `{id, name, suggested, probability, undone}[]` | For `auto_tag` |
| `retryRun`, `undoable` | boolean | Whether the row offers retry and undo ([data-model.md, Rules](../data-model.md#rules)) |
| `undone` | boolean | Undone by the owner |
| `createdAt`, `updatedAt` | date-time | |

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | Unknown `status`, a malformed cursor, `limit` out of range |

## `POST /api/activity/{id}/retry`

`operationId: retryActivity`. Starts the entry's process again and answers
`202`. The new run is a new entry, which supersedes this one; a
`version_judgement` keeps its entry, which returns to `running`
([R-4](../research.md#r-4-three-statuses-with-a-reason-and-a-newer-run-closes-an-older-problem)).
A `scan` entry is closed by the next scan that ends `done`.

| Kind | What starts |
| --- | --- |
| `import` | The video's failed stages are queued again, as `POST /api/videos/{id}/probe` does |
| `scan` | A manual scan of every media folder, as `POST /api/scans` starts |
| `auto_tag` | The video is queued for auto-tagging |
| `version_judgement` | The pair's `retry_at` is set to now and the judging worker wakes |

| Status | `code` | When |
| --- | --- | --- |
| `404` | `not_found` | No such entry |
| `409` | `activity_not_retryable` | The entry does not offer retry, or its subject is gone |
| `409` | `scan_in_progress` | A `scan` entry while a scan is running, as `POST /api/scans` answers |
| `409` | `auto_tagging_off` | An `auto_tag` entry while auto-tagging is off |

## `POST /api/activity/{id}/undo`

`operationId: undoActivity`. Undoes an automatic decision and answers `200`
with the owner's `undo` entry; its `outcome` is `already_reverted` when the
owner had already changed the state
([R-7](../research.md#r-7-undo-restores-the-state-and-always-records-the-exclusion)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `tagId` | body | integer | For `auto_tag` | The tag to undo |

| Status | `code` | When |
| --- | --- | --- |
| `404` | `not_found` | No such entry, or the tag is not one this entry added |
| `409` | `activity_not_undoable` | The entry or tag does not offer undo, or is already undone |

## `GET /api/events`: `activity`

A new notification kind with no payload, sent to owner streams after any entry
is written or changes ([R-8](../research.md#r-8-the-screen-refreshes-on-an-activity-notification)).
The activity screen refetches its counts and loaded entries on it, and the
shell's entry point refetches the counts.
