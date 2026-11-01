# Contract: Peer links

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), operations
`getPeer`, `pairPeer`, `unpairPeer`, `updatePeerAddress`,
`confirmPeerCertificate` and `streamEvents`. This file describes only what
this feature changes.

## `GET /api/peer`

Answers every link this node reads, each with its own state, instead of one
`link` with a top-level `state`.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `links` | body | `PeerLinkState[]` | yes | In slot order; empty while this node reads nothing |
| `links[].slot` | body | integer | yes | The link's slot; its ids are `own id + slot × 2^40` |
| `links[].nodeId` | body | string | yes (owner) | The link's node id, the path parameter of the operations below |
| `links[].state` | body | `connected`, `unreachable`, `revoked` | yes | As today's `state`; `unpaired` no longer exists, since a link that is not there is not listed |
| `links[].reason` | body | `network`, `certificate` | no | As today |
| `links[].name`, `address`, `pinned`, `version`, `fingerprint` | body | | owner | As today's `link` and `fingerprint` |
| `grant`, `node` | body | | | Unchanged |

**Response**: `200` with `PeerState`. Guests receive `links[]` with `slot`,
`state` and `reason` only, so the shell notice can say that a link is down
without naming it.

## `POST /api/peer/pair`

Unchanged request; the answer is the whole `PeerState`.

| Status | `code` | When |
| --- | --- | --- |
| `409` | `conflict`, reason `already_paired` | The node at the address is one this node already reads, or that node is already read by another node |
| `409` | `conflict`, reason `peer_limit` | This node already reads 8 nodes |

## `DELETE /api/peer/links/{nodeId}`

Replaces `DELETE /api/peer/link`: removes that link only, telling the read
node best effort, and deletes its snapshot of media folders. A node id this
node does not read answers `204` too.

## `PUT /api/peer/links/{nodeId}/address`

Replaces `PUT /api/peer/address`; an unknown node id answers `404 not_found`.

## `POST /api/peer/links/{nodeId}/certificate`

Replaces `POST /api/peer/certificate`; the fingerprint waiting is that link's.

## `GET /api/events`

The `peer` event carries the new `PeerState` on any link's or the grant's
change. A link's forwarded `video`, `group`, `smartFolder` and
`mediaFolders` events carry ids offset by that link's slot.

## Errors naming a link

`Error` gains an optional `peer`, the link's name, on `409 peer_unreachable`
and on a link's refusal during a vocabulary write, so the screens show which
link caused it.

## Client use

`web/src/api/peer.ts` exposes the links; `PeerProvider` keeps them by slot and
`usePeerName(slot)` names one, where `remoteSlot(id)` from
`web/src/lib/remoteId.ts` gives the slot of an item's id.
