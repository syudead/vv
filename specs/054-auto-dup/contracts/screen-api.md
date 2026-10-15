# Contract: Screen API for automatic duplicate judging

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This document
holds only what this feature adds or changes; the version routes are
[030 screen-api.md](../../030-video-versions/contracts/screen-api.md). The
error shape is
[023 error-api.md](../../023-english-i18n/contracts/error-api.md).

## `Video` delta

| Field | Before | After |
| --- | --- | --- |
| `versions` (`VideoVersionsRef {count, representativeId}`) | Only in `GET /api/videos/{id}` | In every screen-API response that returns a `Video` for a bundle member, list items included ([R-9](../research.md#r-9-list-items-carry-the-existing-versions-summary-read-per-page)). `count` is still the number of versions with a location the viewer may see |

Guests get it under the same rule as the detail does today. `api/external-v1.yaml`
is unchanged.

## `GET /api/settings/version-judging`

`operationId: getVersionJudgingSettings`. Owner only. Returns
`VersionJudgingSettings {enabled: boolean, waitingForConnection: integer}`.
`enabled` is `false` until the owner saves. `waitingForConnection` is the
number of candidates marked `no_connection`
([R-13](../research.md#r-13-the-model-is-asked-only-once-the-owner-has-saved-a-connection)),
so the section can say that they wait for an Ollama connection.

## `PUT /api/settings/version-judging`

`operationId: updateVersionJudgingSettings`. Owner only. Body
`{enabled: boolean}`; saves it and answers `200` with the saved
`VersionJudgingSettings`. Turning it on wakes the judging worker, which then
judges every waiting candidate.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | The body is not `{enabled: boolean}` |

The Ollama connection stays its own resource (`/api/settings/ollama`), so the
feature can be on while no connection is saved; the rules then still bundle
and dismiss, and pairs that need the model wait. `PUT /api/settings/ollama`
also wakes the judging worker, which first returns the waiting pairs to the
queue.

## Routes that change behaviour only

| Route | Change |
| --- | --- |
| `POST /api/videos/{id}/unbundle` | Also records the unbundled pairs and rebuilds the removed video's candidates, so its pairs that match by fingerprint are listed again ([R-8](../research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates)); the response is unchanged |
| `GET /api/version-candidates` | Unchanged shape; candidates the model judged same or different are gone from it |

## `GET /api/events`: `candidates`

A new notification kind with no payload, sent after a judgement result or a
removal from versions changes the candidates
([R-10](../research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification)).
The Duplicates page refetches on it. An automatic bundle also sends the `video`
notification for each member, as a bundle by hand does (030 R-9).
