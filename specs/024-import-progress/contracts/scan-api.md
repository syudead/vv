# Contract: Import status and issue list

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml); the Go and
TypeScript code is generated with `task generate`. This document covers only the
routes and shapes this feature changes. Authorization does not change: every route
is owner-only through the global `security: [sessionCookie]`, as today, and is not
exposed to guests (requirement 11; not added to `accessRoutes` in
[internal/httpapi/auth.go](../../../internal/httpapi/auth.go)). Why the route names
stay is in
[research.md R-9](../research.md#r-9-api-reshapes-apiscans-and-removes-apiprocessing-and-the-sse-processing-event).

## 1. Changed, removed and added routes

| Route | Change |
| --- | --- |
| `POST /api/scans` | The response `Scan` takes the §2 shape. 409 and 403 do not change |
| `GET /api/scans/current` | The response `Scan` takes the §2 shape. 404 when no scan has ever run, as today |
| `GET /api/scans/current/issues` | New. §3 |
| `GET /api/processing` | Removed, along with the `Processing` schema |
| `GET /api/events` | The `processing` event is removed. `scan` is in §4 |

## 2. `Scan`

The latest import's state as the user sees it. The bottom-right indicator and the
Settings page both read this one value (requirement 8).

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | int64, required | The scan id. Used to detect that the import was replaced |
| `status` | `finding` \| `running` \| `done` \| `partial` \| `failed`, required | The status shown to the user ([R-4](../research.md#r-4-done-only-when-the-scan-is-closed-and-the-set-has-no-remaining-jobs)) |
| `videos` | object \| omitted | Progress. Omitted during `finding` |
| `videos.total` | int, required | The number of target videos ([R-5](../research.md#r-5-the-denominator-during-a-scan-adds-files-not-yet-registered)). 0 means "nothing changed" |
| `videos.settled` | int, required | The number of settled videos. `0 ≤ settled ≤ total` |
| `issues` | object, required | Issue counts, counting the grouped items of §3 |
| `issues.failed` | int, required | Items whose severity is failure |
| `issues.substituted` | int, required | Items whose severity is substitution |
| `issues.revision` | int, required | Increases whenever the issue list content changes ([data-model.md §1 and §3](../data-model.md#3-scan_issues-new-table)). It increases when kinds or rows change even if the counts stay the same |
| `settledAt` | date-time \| omitted | The time every target settled. Returned only for `done` and `partial` (requirement 4) |
| `activity` | object \| omitted | The current activity ([R-8](../research.md#r-8-current-activity-held-in-memory-by-internalapp-not-stored)). Omitted when nothing is running |
| `activity.kind` | `registering` \| `probe` \| `thumbnail` \| `seekThumbnail` \| `preview`, required | What is happening |
| `activity.fileName` | string, required | The file name |
| `activity.folder` | `VideoFolder` \| omitted | The folder that holds the file, to tell files with the same name apart |
| `activity.videoId` | int64 \| omitted | The video id, when the file is a registered video |
| `state` | `running` \| `done` \| `failed`, required | The state of the scan itself. Used to reload the list and to decide whether an import can start; not shown on screen |
| `errorCode`, `errorPath` | optional | The reason when the scan is `failed`. Keeps the shape the English i18n feature (023) added to `Scan`. `error` also stays, as in 023, and is not shown on screen |

Today's `startedAt`, `finishedAt`, `total`, `completed` and `failed` are removed.

`status` is decided by a pure function in `internal/domain`. The first rule from
the top that matches wins:

1. `failed`: `state = failed`
2. `finding`: `state = running`, and the scan has not finished counting its
   targets
3. `running`: `state = running`, or some target is not settled
4. `partial`: `issues.failed > 0`
5. `done`: otherwise

## 3. `GET /api/scans/current/issues`

Returns the latest import's issues, one item per video (per file when
unregistered) ([data-model.md §3](../data-model.md#3-scan_issues-new-table)).

Query parameters:

| Parameter | Meaning |
| --- | --- |
| `limit` | 1 to 200, default 50 |
| `cursor` | The previous response's `nextCursor`. An opaque string |

Response:

| Field | Type | Meaning |
| --- | --- | --- |
| `scanId` | int64, required | Which import the list belongs to. When it differs from `Scan.id`, the screen reloads |
| `items` | `ScanIssue[]`, required | Failures first; within a severity, by file name, then by folder |
| `nextCursor` | string \| omitted | Present only when there is more |

An item whose location is not inside any registered folder is neither returned nor
counted in `Scan.issues`, for example after a media folder is removed following
the import. This follows the parent Issue's Edge Case that a video that left the
target is not counted as an issue either.

When no scan has ever run, the response is 404 (as for `GET /api/scans/current`).
An invalid `cursor` returns 400.

The `ScanIssue` shape:

| Field | Type | Meaning |
| --- | --- | --- |
| `severity` | `failed` \| `substituted`, required | The grouped item's severity |
| `kinds` | `ScanIssueKind[]`, required, at least one | The kinds of data-model.md §3, most severe first |
| `fileName` | string, required | |
| `folder` | `VideoFolder`, required | The folder that holds the location, to tell files with the same name apart |
| `videoId` | int64 \| omitted | The video id, when registered. The screen can link to `/videos/{id}` (requirement 5) |

The SPA builds the impact wording (such as `一覧に追加できませんでした`) and the
reason wording from `kinds`
([R-10](../research.md#r-10-the-spa-builds-screen-text-from-kinds-the-server-returns)).

## 4. `/api/events`

- The `scan` event carries the §2 `Scan`. As today, it is read at send time,
  coalesced per connection, and sent once right after connecting.
- `scan` is sent on:
  - today's `domain.ScanChanged`
  - `domain.ProcessingChanged` (a job outcome changes the settled count)
  - the new `domain.ScanActivityChanged`
- The issue list is not sent over SSE. The screen reloads §3 when `Scan.id` or
  `Scan.issues.revision` changes. The same applies after reconnecting, so issues
  added while disconnected appear.
- The `video` event does not change.
