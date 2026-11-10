# Contract: Background work status and pause

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), operations
`getBackgroundWork` and `setBackgroundPause`, schemas `BackgroundWork` and
`SetBackgroundPauseRequest`, and the `background` event of `/api/events`. Both
operations are owner-only (the schema's default security).

## `GET /api/background`

Answers the pause switch, the work running now and the work left per group.
Uses `Cache-Control: no-store`.

**Response**: `200` with `BackgroundWork`.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `paused` | boolean | yes | The saved switch |
| `running` | array of `BackgroundActivity` | yes | Work running now, oldest first; entries remain after a pause until their job returns |
| `groups` | array of `BackgroundGroup` | yes | One entry per group, always in the order `scan`, `import`, `auto_tag`, `face`, `version_judge`, `backfill` |

`BackgroundActivity`:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `group` | string enum | yes | The group of the work |
| `kind` | string | yes | `registering` for a scan, otherwise the job kind |
| `video` | object with `id` and `title` | no | The video the work is for; absent when the work names none or the owner's audience leaves it out |
| `fileName` | string | no | The file being registered or read, when the work has a location |

`BackgroundGroup`:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `group` | string enum | yes | `scan`, `import`, `auto_tag`, `face`, `version_judge` or `backfill` |
| `remaining` | integer | yes | Files for `scan`, videos for `import`, jobs otherwise; 0 when nothing is left |
| `waiting` | boolean | no | `scan` only: a paused scan or held directories wait for the resume |
| `featureOff` | boolean | no | The group's feature holds its jobs back, so they do not run after a resume |

| Status | `code` | When |
| --- | --- | --- |
| `401` | `unauthenticated` | No owner session |

## `PUT /api/background/pause`

Saves the switch and applies it: `true` stops the running scan and jobs and
holds new ones, `false` resumes them.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `paused` | body | boolean | yes | The state to reach |

**Response**: `200` with `BackgroundWork` after the change. A request for the
current state changes nothing and answers the same body. Concurrent requests
are applied one at a time, and the last one decides the state.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request` | The body is missing or `paused` is not a boolean |
| `401` | `unauthenticated` | No owner session |

## Event `background`

Sent on `/api/events` without a payload when the switch, the running list or a
count changes, at most once a second, with one trailing event for changes
within that second.

## Changed: `Scan.state`

The enum gains `paused`. `startScan` and `startRescan` answer a scan in that
state when background work is paused; `startRescan` answers
`409 scan_in_progress` while a paused scan exists.

## Client use

The owner's screens read `GET /api/background` once and again on each
`background` event and on reconnecting to `/api/events`. A guest's screens
call neither operation.
