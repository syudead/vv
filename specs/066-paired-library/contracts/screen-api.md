# Contract: what the screens gain

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This file
lists the operations and fields this feature adds for the screens; what one
node calls on the other is in [peer-api.md](peer-api.md). Every operation
below is owner-only unless the table says otherwise.

## `GET /api/peer`

Returns the pairing state for the Settings page and the notice.

**Response**: `200` with `PeerState`:

| Field | Type | Audience | Meaning |
| --- | --- | --- | --- |
| `state` | `unpaired`, `connected`, `unreachable`, `revoked` | Guests too | [plan.md, Connection state](../plan.md#connection-state) |
| `reason` | `network`, `version`, `certificate` | Guests too | Present with `unreachable` |
| `peer.name` | string | Owner | The peer's host name |
| `peer.address` | string | Owner | The peer's base URL |
| `peer.version` | string | Owner | The version last seen, with reason `version` the one that differs |
| `fingerprint` | string | Owner | With reason `certificate`: the SPKI fingerprint waiting for confirmation |
| `node.id`, `node.name`, `node.version` | string | Owner | This node, shown beside the pairing code |

Guests get `state` and `reason` only. The guest-capable classification
follows `GET /api/smart-folders`.

## `POST /api/peer/code`

Issues this node's pairing code, replacing an unused one.

**Response**: `201` with `{ code, expiresAt }`.

| Status | `code` | `reason` | When |
| --- | --- | --- | --- |
| `409` | `conflict` | `already_paired` | A peer row exists |

## `POST /api/peer/pair`

Pairs this node with the peer whose code the owner typed, by calling the
peer's `POST /api/peer/pairings`
([peer-api.md](peer-api.md#post-apipeerpairings)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `address` | body | string | Yes | The peer's base URL, `http://` or `https://`, no path |
| `code` | body | string | Yes | The peer's pairing code |
| `ownAddress` | body | string | Yes | This node's base URL as the peer can reach it |
| `fingerprint` | body | string | No | The SPKI fingerprint the owner confirmed after `certificate_untrusted` |

**Response**: `201` with `PeerState` in `connected`.

| Status | `code` | `reason` | When |
| --- | --- | --- | --- |
| `400` | `invalid_request` | `invalid_address` | A URL without scheme or host, or with a path; `field` names it |
| `400` | `invalid_request` | `invalid_code` | The peer refused the code |
| `409` | `conflict` | `already_paired` | This node or the peer has a peer |
| `409` | `conflict` | `unsupported_version` | The peer's version differs; the body carries both |
| `409` | `conflict` | `peer_unreachable` | The peer did not answer, or its callback to `ownAddress` failed; `message` says which |
| `409` | `conflict` | `certificate_untrusted` | The peer's certificate failed the system roots; the body carries `fingerprint` to confirm |

## `PUT /api/peer/address`

Changes the peer's address without pairing again; the connection is retried
at once.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `address` | body | string | Yes | As in `POST /api/peer/pair` |

**Response**: `200` with `PeerState`. A missing peer answers `404 not_found`.

## `POST /api/peer/certificate`

Confirms the fingerprint an `unreachable` state with reason `certificate`
shows, pins it and reconnects.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `fingerprint` | body | string | Yes | Must equal the one `GET /api/peer` shows, else `409 conflict` with reason `fingerprint_changed` |

**Response**: `200` with `PeerState`.

## `DELETE /api/peer`

Unpairs: deletes the peer row, tells the peer (best effort) and answers
`204`; also clears a `revoked` state. A missing peer answers `204`.

## `peer` event

`/api/events` sends `peer` with `PeerState` in the audience's shape on every
state change, to owners and guests, and once on connection.

## Remote ids

An id at or above `2^40` (`1099511627776`) in any path, query or body names
an entity of the peer, whose own id is the value minus the offset
([plan.md, Remote ids](../plan.md#remote-ids)).

| Entity | Id in responses |
| --- | --- |
| Video, group, group item, smart folder, media-folder root (`rootId`) | Always the offset form |
| Tag, tag kind, person, organization | The local entity's id when one has the same name; else the offset form |

| Operation on a remote id | Behaviour |
| --- | --- |
| Reads and single edits under `/api/videos/{id}`, `/api/groups/{id}`, `/api/smart-folders/{id}`, `/api/folders/{rootId}`, `/api/tags/{id}`, `/api/people/{id}`, `/api/organizations/{id}` | Forwarded; the answer's ids and URLs rewritten; `progress` and `favorite` absent from a remote `Video` |
| `open`, `progress`, `auto-tag`, `probe`, favorites, playlists, watch history, activity | `404 not_found` |
| Any forwarded operation while the peer is `unreachable` or `revoked` | `503 service_unavailable` with reason `peer_unreachable` |
| Rename, merge, delete of a remote-only tag, person or organization | Applied on the peer after the local conflict check ([plan.md, Vocabulary by name](../plan.md#vocabulary-by-name)) |

## Operations over a selection

`POST /api/video-tags`, `/api/video-visibility`, `/api/video-edits`,
`/api/video-code-edits`, their `summary` operations and
`POST /api/collections/delete` accept ids of both nodes.

| Field | Type | Meaning |
| --- | --- | --- |
| `remote.applied` | integer | Videos (or collections) changed on the peer |
| `remote.failedVideoIds` | int64[] | Remote ids not changed, in the offset form; empty on success |
| `remote.reason` | `peer_unreachable`, or the peer's `reason` | Present when `failedVideoIds` is not empty |

The local part is applied first in one transaction; a peer failure leaves it
applied. A summary merges counts by name across nodes. `too_many_videos`
counts the union of both nodes' resolved videos.

| Status | `code` | `reason` | When |
| --- | --- | --- | --- |
| `400` | `invalid_request` | `mixed_nodes` | `POST /api/groups`, `/api/groups/{id}/items`, `/api/organize/*`, `/api/video-bundles` or a favorites or playlist write with videos of both nodes, or a remote video in a favorites or playlist write |
| `400` | `invalid_request` | `provisional_person_remote` | A credit edit with a provisional person and a remote target |

## Smart folder cards

`GET /api/smart-folders/cards` adds the peer's matches to `videoCount`,
`durationMs` and `sizeBytes` of each folder while the peer is `connected`;
covers, `watchedCount`, `watchState` and `lastPlayedAt` stay local.
