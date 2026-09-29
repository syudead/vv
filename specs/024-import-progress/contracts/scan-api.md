# Contract: import status and issue list

The canonical definition is [api/openapi.yaml](../../../api/openapi.yaml). `task generate` produces
the Go and TypeScript code. This document covers only the routes and shapes this feature changes.
Authorization does not change:

- Every route stays owner-only through the global `security: [sessionCookie]`, as today.
- No route is exposed to guests (Requirement 11). Do not add them to `accessRoutes` in
  [internal/httpapi/auth.go](../../../internal/httpapi/auth.go).

[research.md R-9](../research.md#r-9-the-api-rebuilds-apiscans-and-removes-apiprocessing-and-the-sse-processing-event)
explains why the route names stay.

## 1. Changed, removed and added routes

| Route | Change |
| --- | --- |
| `POST /api/scans` | The response `Scan` takes the shape in §2. 409 and 403 do not change |
| `GET /api/scans/current` | The response `Scan` takes the shape in §2. Returns 404 when no scan has ever run, as today |
| `GET /api/scans/current/issues` | New. See §3 |
| `GET /api/processing` | Removed. The `Processing` schema is removed too |
| `GET /api/events` | The `processing` event is removed. `scan` is described in §4 |

## 2. `Scan`

`Scan` is the user-facing state of the latest import. The bottom-right indicator and the Settings
screen both read this one object (Requirement 8).

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | int64, required | Scan id. Used to detect that the import was replaced |
| `status` | `finding` \| `running` \| `done` \| `partial` \| `failed`, required | State shown to the user ([R-4](../research.md#r-4-done-means-the-scan-closed-and-no-job-remains-in-the-set)) |
| `videos` | object \| omitted | Progress. Omitted during `finding` |
| `videos.total` | int, required | Number of target videos ([R-5](../research.md#r-5-while-scanning-add-target-files-not-yet-registered-to-the-denominator)). 0 means "nothing changed" |
| `videos.settled` | int, required | Number of settled videos. `0 ≤ settled ≤ total` |
| `issues` | object, required | Issue counts. Counts the merged entries from §3 |
| `issues.failed` | int, required | Number of entries with severity failed |
| `issues.substituted` | int, required | Number of entries with severity substituted |
| `issues.revision` | int, required | Number that increases whenever the issue list content changes ([data-model.md §1 and §3](../data-model.md#3-scan_issues-new-table)). It increases when the kinds or rows change, even if the counts stay the same |
| `settledAt` | date-time \| omitted | Time when every target settled. Returned only for `done` and `partial` (Requirement 4) |
| `activity` | object \| omitted | Current activity ([R-8](../research.md#r-8-current-activity-is-not-stored-internalapp-keeps-it-in-memory)). Omitted when nothing is running |
| `activity.kind` | `registering` \| `probe` \| `thumbnail` \| `seekThumbnail` \| `preview`, required | What is being done |
| `activity.fileName` | string, required | File name |
| `activity.folder` | `VideoFolder` \| omitted | Folder that holds the file. Tells files with the same name apart |
| `activity.videoId` | int64 \| omitted | The video id, when the video is registered |
| `state` | `running` \| `done` \| `failed`, required | State of the scan itself. Used to reload the list and to decide whether an import can start. Not shown on screen |
| `errorCode`, `errorPath` | optional | Reason when the scan is `failed`. Carries over the shape that the English localization (023) added to `Scan`. `error` also stays as in 023 but is not shown on screen |

The current `startedAt`, `finishedAt`, `total`, `completed` and `failed` are removed.

`status` is a pure function owned by `internal/domain`. It takes the first rule that matches, top
to bottom:

1. `failed`: `state = failed`
2. `finding`: `state = running`, and the scan has not finished counting its targets
3. `running`: `state = running`, or some target is not settled
4. `partial`: `issues.failed > 0`
5. `done`: otherwise

## 3. `GET /api/scans/current/issues`

Returns the issues of the latest import, one entry per video (per file when unregistered)
([data-model.md §3](../data-model.md#3-scan_issues-new-table)).

Query parameters:

| Parameter | Meaning |
| --- | --- |
| `limit` | 1 to 200, default 50 |
| `cursor` | `nextCursor` from the previous response. An opaque string |

Response:

| Field | Type | Meaning |
| --- | --- | --- |
| `scanId` | int64, required | The import this list belongs to. When it differs from `Scan.id`, the screen reloads |
| `items` | `ScanIssue[]`, required | Failed first. Within the same severity, ordered by file name, then by folder |
| `nextCursor` | string \| omitted | Present only when more items follow |

An entry whose location is outside every registered folder is not returned and is not counted in
`Scan.issues`. Example: a media folder removed after the import. This follows the parent Issue's
Edge cases entry "a video that drops out of the target … is not counted as an issue either".

Returns 404 when no scan has ever run (same as `GET /api/scans/current`). Returns 400 for an
invalid `cursor`.

Shape of `ScanIssue`:

| Field | Type | Meaning |
| --- | --- | --- |
| `severity` | `failed` \| `substituted`, required | Severity of the merged entry |
| `kinds` | `ScanIssueKind[]`, required, at least 1 | Kinds from data-model.md §3, most severe first |
| `fileName` | string, required | |
| `folder` | `VideoFolder`, required | Folder that holds the location. Tells files with the same name apart |
| `videoId` | int64 \| omitted | The video id, when the video is registered. The screen can go to `/videos/{id}` (Requirement 5) |

The SPA builds the impact text (such as "Not added to the library.") and the reason text from
`kinds` ([R-10](../research.md#r-10-the-spa-builds-the-screen-text-from-kinds-the-server-returns)).

## 4. `/api/events`

- The `scan` event carries the `Scan` from §2. As today, it is read at send time, coalesced per
  connection, and sent once right after connecting.
- These events trigger a `scan` send:
  - The existing `domain.ScanChanged`
  - `domain.ProcessingChanged` (job outcomes change the settled count)
  - The new `domain.ScanActivityChanged`
- SSE does not carry the issue list. The screen reloads §3 when `Scan.id` or
  `Scan.issues.revision` changes. The same applies after a reconnect, so issues added while
  disconnected appear.
- The `video` event does not change.
