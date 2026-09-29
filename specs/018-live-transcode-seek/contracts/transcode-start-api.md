# Contract: actual start position of a live transcode

The source of truth is `api/openapi.yaml`. This document covers only the changed parameter and the
added route. The response shape of the live transcode itself (`transcodeVideo`) does not change.

## 1. The `attempt` parameter of `transcodeVideo`

`GET /api/videos/{id}/transcode.mp4?startMs=…&attempt=…`

- `attempt`: optional. `[A-Za-z0-9_-]{1,64}`. A random value the player creates per request; it is
  the key for §2. A value in the wrong format returns 400 `invalid_request`.
- With `attempt`, the server enters it in the ledger when the transcode request starts. It records
  the actual start position once that is known (before it starts writing the response body). A
  second request with the same `attempt` overwrites the value with the later one (same video, same
  position, so the value is the same).
- A request without `attempt` works as before and is not entered in the ledger.

## 2. `GET /api/videos/{id}/transcode-start`

`operationId: getTranscodeStart`. `security` is the same as `transcodeVideo` (owner and guest).

- Parameter: `attempt` (required, same format as §1).
- 200: `{ "startMs": integer }`. The source-video time, in milliseconds, of time 0 of the transcode
  output.
  - For a transcode that starts by copying: the requested position or the keyframe just before it
    (the time of the track that starts earliest).
  - For a transcode that starts by encoding: the requested position itself.
- Waiting: when `attempt` is not in the ledger yet, the route waits until it appears. When it is in
  the ledger but unresolved, the route waits until it is resolved. The limit is 6 seconds, the same
  as `transcodeStartupTimeout`; when nothing is resolved by then, the result is 404.
- 404 `not_found`:
  - The video does not exist.
  - A guest points at a video that is not public (the same check as `transcodeVideo`).
  - `attempt` does not appear within the limit.
  - The transcode failed before producing its first data.
- 400 `invalid_request`: `attempt` is in the wrong format.
- The response has `Cache-Control: no-store`.
- A ledger row is deleted 60 seconds after its transcode request ends. This lets a report request
  made right after a reload still read the value of a finished transcode.

## 3. How the player uses it

- When `startMs > 0`, `liveSource` creates an `attempt`, adds it to the URL, and calls
  `getTranscodeStart` right after `setSource`. Until the response arrives, the current time uses the
  requested position (`pendingOffsetSeconds`) as the offset.
- On 200, the player replaces the offset with `startMs` and passes the same value to the playback
  position store through `vvOffsetChanged`. On 404 or an error, the offset stays at the requested
  position (the same display as today).
- A response for an old `attempt` that arrives after the source changed is discarded.
