# Contract: screen API changes for the peer's items

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This file lists
only what this feature adds or changes; every other operation gains the
peer's items with no change of shape
([research.md, R-1](../research.md#r-1-entity-operations-are-offered-for-remote-ids-unless-named-local-only)).

## `GET /api/watch-history` (`listWatchHistory`)

Answers both nodes' entries newest first; a request with the peer credential
also carries per-item cursors, as the video lists do.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `peerItems` | response | `PeerListItem[]` | peer requests only | One entry per item, in order: the cursor after it and its `playedAt` as the sort value |
| `items[].id` | response | int64 | yes | The peer's entry carries the offset form |
| `cursor` | query | string | no | One composite cursor holding each node's cursor, as the video list's |

**Response**: `200` `WatchHistoryPage`; a remote entry's `video` is rewritten
like any remote `Video`, with its `progress` and `favorite` kept.

## `GET /api/watch-history/dates` (`listWatchHistoryDates`)

Answers the union of both nodes' days, newest first. No field changes.

## `DELETE /api/watch-history` (`clearWatchHistory`) and `DELETE /api/watch-history/{id}` (`deleteWatchHistoryEntry`)

A clear is sent to the peer first, then applied here; an entry id in the
offset form is deleted on the peer.

| Status | `code` | When |
| --- | --- | --- |
| `503` | `service_unavailable`, reason `peer_unreachable` | The peer is not connected or gave no answer; this node's history is unchanged |

## `PUT /api/favorites` (`updateFavorites`)

`videoIds`, `groupIds`, `smartFolderIds` and `personIds` may name both nodes'
entities; the peer's part is sent first as one request in the peer's own ids
(people by name), then this node's part is applied. The `applied*` counts are
the sums.

| Status | `code` | When |
| --- | --- | --- |
| `503` | `service_unavailable`, reason `peer_unreachable` | The peer is not connected or gave no answer; nothing is changed on either node |

## `PUT /api/videos/{id}/progress` (`updateProgress`)

Forwarded for a remote id; the peer's `Progress` is answered as it is. A
remote `Video` now carries `progress` and `favorite`, and a remote
`SmartFolderCard` and group card carry their watch values and `favorite`.

## `POST /api/people/{id}/appearances/move` (`moveFaceAppearances`)

For a remote person, `targetId` may be a local or a remote id; it is
translated to the peer's person of that name, created there when missing, and
`appearanceIds` drop the offset. The answer's `person` is rewritten like any
remote `Person`.

| Status | `code` | When |
| --- | --- | --- |
| `400` | `invalid_request`, reason `mixed_nodes` | An `appearanceIds` entry names this node's appearance |

## `POST /api/people/{id}/face-candidates/{otherId}/{confirm,dismiss,reopen}`

For a remote person, `otherId` is translated to the peer's person by name;
a name the peer lacks answers `404 not_found`.

## `GET /api/people/{id}/similar-people` (`listSimilarPeople`)

For a remote person, `appearanceId` drops the offset; the answer's people join
by name.

## `GET /api/videos/{id}/face-appearances` (`listVideoFaceAppearances`) and `GET /api/face-appearances/{id}/crop` (`getFaceAppearanceCrop`)

A remote video's appearances keep the peer's `hasCrop`, and the crop of an
appearance id in the offset form is forwarded as media.

## Client use

The screens change no call: a remote id goes where a local id goes. Only the
gates that hid the favorite toggle, progress saving, versions, thumbnail edits
and the person page's faces for remote items are removed
([plan.md, Implementation Work](../plan.md#implementation-work)).
