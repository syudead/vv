# Contract: Screen API for automatic duplicate judging

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This document
holds only what this feature adds or changes; the version routes are
[030 screen-api.md](../../030-video-versions/contracts/screen-api.md). The
error shape is
[023 error-api.md](../../023-english-i18n/contracts/error-api.md).

## `Video` delta

| Field | Before | After |
| --- | --- | --- |
| `versions` (`VideoVersionsRef {count, representativeId}`) | Only in `GET /api/videos/{id}` | In every screen-API response that returns a `Video` for a bundle member, list items included ([R-9](../research.md#r-9-list-items-carry-the-existing-versions-summary)). `count` is still the number of versions with a location the viewer may see |

Guests get it under the same rule as the detail does today. `api/external-v1.yaml`
is unchanged.

## `GET /api/settings/version-judging`

`operationId: getVersionJudgingSettings`. Owner only. Returns
`VersionJudgingSettings {enabled: boolean}`; `enabled` is `false` until the
owner saves.

## `PUT /api/settings/version-judging`

`operationId: updateVersionJudgingSettings`. Owner only. Body
`{enabled: boolean}`; saves it and answers `200` with the saved
`VersionJudgingSettings`. Turning it on wakes the judging worker, which then
judges every waiting candidate.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | The body is not `{enabled: boolean}` |

The Ollama connection stays its own resource (`/api/settings/ollama`), so the
feature can be on while no connection is saved; every judgement then fails and
retries ([R-5](../research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends)).

## Routes that change behaviour only

| Route | Change |
| --- | --- |
| `POST /api/videos/{id}/unbundle` | Also records the unbundled pairs and returns them to the candidates ([R-8](../research.md#r-8-removing-a-version-records-the-pairs-and-lists-them-again)); the response is unchanged |
| `GET /api/version-candidates` | Unchanged shape; candidates the model judged same or different are gone from it |

## Notifications

An automatic bundle publishes `domain.VideoBundleChanged`, so screens receive
the `video` notification as for a bundle by hand (030 R-9). An automatic
"different videos" changes the candidates, and the worker publishes
`ProcessingChanged` after it, so the Duplicates page refetches on `scan` as it
does after a fingerprint job.
