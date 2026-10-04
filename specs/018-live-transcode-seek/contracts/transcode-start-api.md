# Contract: Actual start position of a live transcode

Source of truth: `api/openapi.yaml`. This document describes only the parameter
this feature changes and the path it adds. The response shape of the live
transcode itself (`transcodeVideo`) does not change.

## `attempt` parameter of `transcodeVideo`

`GET /api/videos/{id}/transcode.mp4?startMs=…&attempt=…`

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `attempt` | query | string, `[A-Za-z0-9_-]{1,64}` | No | A random value the player creates per request; the key for [`GET /api/videos/{id}/transcode-start`](#get-apivideosidtranscode-start). A value that does not match the format returns 400 `invalid_request`. |

- With `attempt`, the server enters it in the ledger when it starts serving the
  transcode request, and records the actual start position once it is known
  (before writing the first byte of the response body). When two requests
  arrive with the same `attempt`, the later request's value overwrites the
  earlier one (same video and same position, so the value is the same).
- A request without `attempt` behaves as before and is not entered in the
  ledger.

## `GET /api/videos/{id}/transcode-start`

`operationId: getTranscodeStart`. `security` is the same as `transcodeVideo`
(owner and guest).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `attempt` | query | string, same format as [`attempt` parameter of `transcodeVideo`](#attempt-parameter-of-transcodevideo) | Yes | The `attempt` sent with the transcode request. |

**Response**: 200 `{ "startMs": integer }` — the time in the source video, in
milliseconds, that time 0 of the transcode output corresponds to.

| How the transcode started | `startMs` |
| --- | --- |
| Copy | The requested position, or the time of the keyframe before it (the time of the track that starts earliest) |
| Encode | The requested position itself |

Waiting: when `attempt` is not in the ledger yet, the request waits until it
appears; when it is in the ledger but unresolved, it waits until it is
resolved. The limit is 6 seconds, the same as `transcodeStartupTimeout`; if
nothing is resolved by then, the response is 404.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | The video does not exist |
| 404 | `not_found` | A guest named a video that is not public (same check as `transcodeVideo`) |
| 404 | `not_found` | `attempt` does not appear within the limit |
| 404 | `not_found` | The transcode failed before producing its first data |
| 400 | `invalid_request` | `attempt` has the wrong format |

- The response carries `Cache-Control: no-store`.
- A ledger row is removed 60 seconds after the transcode request ends, so that a
  report request made right after a reload can still read the value of a
  transcode that has finished.

## Client use

- When `startMs > 0`, `liveSource` creates an `attempt`, adds it to the URL, and
  calls `getTranscodeStart` right after `setSource`. Until the answer arrives,
  the current time is reported with the requested position
  (`pendingOffsetSeconds`) as the offset.
- On 200, the offset is replaced with `startMs`, and `vvOffsetChanged` passes
  the same value on to saving the playback position. On 404 or an error, the
  offset stays at the requested position (the same display as before).
- A response for an old `attempt` that arrives after the source was replaced is
  discarded.
