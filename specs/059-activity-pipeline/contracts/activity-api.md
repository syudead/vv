# Contract: activity screen API

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml), operations
`listActivity`, `retryActivity`, `undoActivity` and the new `getActivity`. This
contract lists only what this feature adds or changes; the `activity` event on
`/api/events` is unchanged.

## `GET /api/activity`

Lists **top-level entries**: `pipeline` entries and entries that belong to no
pipeline ([R-7](../research.md#r-7-the-list-pages-top-level-entries)). Members
are not listed on their own; a pair entry in two pipelines appears under both.

| Parameter | Change |
| --- | --- |
| `status` | Matches the top-level entry's own status; a pipeline's status is its roll-up, so a member needing attention makes the pipeline match `attention` |
| `feature`, `decider`, `q`, `since` | Match a pipeline when the pipeline or any of its members matches |
| `cursor`, `limit` | Unchanged; page by the top-level entry's id |

| Response field | Change |
| --- | --- |
| `counts`, `features` | Count top-level entries; a pipeline counts once per status and once under each feature it or a member has |
| `items[].members` | New, pipelines only: `ActivityMemberSummary[]` in join order |

`ActivityMemberSummary`: `id`, `kind`, `feature`, `status`, `reason?`,
`outcome?`, `stage?`, `progress?`, `retryRun`, `undoable`, `replaced` (a
retried or reopened member kept for history), `matched` (the member satisfies
every filter given, absent when no filter is given). It carries no `detail`
and no `tags`.

`ActivityKind` adds `pipeline`. A `pipeline` entry has `retryRun: false` and
`undoable: false`.

## `GET /api/activity/{id}`

Returns one entry of any kind with what the list leaves out
([R-9](../research.md#r-9-the-detail-is-one-owner-only-read)). Owner only, like
the other activity operations.

**Response**: `200` with `ActivityDetail`: every `ActivityEntry` field (`detail`
in full, `tags` with probabilities for `auto_tag`) plus:

| Field | Present when | Meaning |
| --- | --- | --- |
| `members` | `pipeline` | `ActivityEntry[]` in join order, `replaced` members included |
| `pipelines` | A member | `{id, subject, status, createdAt}[]`, the pipelines it belongs to, newest first |
| `stages` | `import` with recorded stages | `{stage, state, detail?, updatedAt}[]` in `JobKinds` order |
| `stagesRecorded` | `import` | `false` for an entry written before this feature, so the screen says the breakdown was not recorded |

| Status | `code` | When |
| --- | --- | --- |
| 403 | — | Not the owner |
| 404 | `not_found` | No entry with that id |

## `POST /api/activity/{id}/retry`, `POST /api/activity/{id}/undo`

Unchanged for members. A `pipeline` id answers `409 activity_not_retryable` or
`409 activity_not_undoable`. The new entry a retry writes joins the retried
entry's pipelines, so the response and the next list show it under the same
parent.

## Client use

The Activity screen refetches the open detail on the `activity` event and on
reconnect, the same trigger as the list
([activity.md](../../../docs/design-docs/activity.md#the-screen-refreshes-on-an-activity-notification)).
