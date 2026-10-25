# Contract: what one node calls on the other

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). The peer calls
the screen API's own operations (`listVideos`, `listLibrary`, `getVideo`,
`streamVideo`, `listTags`, …) with the credential below, plus the
`/api/peer/*` operations of this file, which the screens never call. The
screen operations the screens use for pairing are in
[screen-api.md](screen-api.md).

## Peer credential and audience

A request with `Authorization: VV-Peer <secret>` on any `/api/*` path is
classified by the auth boundary like a session: the secret is hashed and
compared with `peer.incoming_secret_hash`, and the audience is the owner
unless `X-VV-Peer-Audience: guest` is present. The credential reaches only
the operations a table in `internal/httpapi/peer_operations.go` allows: the
reads the merged lists and forwarding need, the media operations, the
forwarded edits of videos, groups, smart folders, tags, people and
organizations, the operations over a selection, `/api/events`, `/api/health`
and `/api/peer/*`
([plan.md, Pairing and the peer credential](../plan.md#pairing-and-the-peer-credential)).

| Case | Response |
| --- | --- |
| Secret unknown, no peer row, or a row whose incoming secret was revoked | `401` `code: unauthenticated`, `reason: peer_unknown` |
| Valid secret on an operation the table does not allow (media folders, scans, settings, network, account, API tokens, activity, favorites, playlists and the rest) | `403` `code: forbidden`, `reason: peer_not_allowed` |
| `X-VV-Peer-Audience` with another value | `400 invalid_request` |
| Valid secret on `/api/v1` or `/mcp` | `401` as today; the peer credential is not a bearer token |

A peer request never sets a cookie and is not subject to the same-origin
check. `X-VV-Audience` in the response states the audience applied.

## `POST /api/peer/pairings`

Node B pairs with node A by sending A's pairing code; A calls B back before
answering ([plan.md, Pairing and the peer credential](../plan.md#pairing-and-the-peer-credential)).
No credential: the code authenticates the call.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `code` | body | string | Yes | The code A's Settings page shows |
| `node.id` | body | string | Yes | B's node id |
| `node.name` | body | string | Yes | B's host name, 1–100 code points |
| `node.address` | body | string | Yes | B's base URL as A can reach it |
| `node.version` | body | string | Yes | B's release version |
| `secret` | body | string | Yes | The secret A will present to B (`vvp_` plus 43 base64url characters) |

**Response**: `201` with `{ node: { id, name, version }, secret }`, where
`secret` is what B presents to A. A stores B's row before answering.

| Status | `code` | `reason` | When |
| --- | --- | --- | --- |
| `400` | `invalid_request` | `invalid_code` | Unknown, used or expired code |
| `409` | `conflict` | `already_paired` | A has a peer row |
| `409` | `conflict` | `unsupported_version` | `node.version` differs from A's |
| `409` | `conflict` | `peer_unreachable` | A's callback to `node.address` failed; the body's `message` carries the cause |
| `409` | `conflict` | `certificate_untrusted` | The callback's HTTPS certificate failed the system roots; the body carries `fingerprint` (base64 SHA-256 of the SPKI) |

## `GET /api/peer/handshake`

Returns this node's identity and version to a caller presenting a valid peer
credential, or, during pairing, the secret the pending pairing registered.

**Response**: `200` with `{ node: { id, name, version } }`.

| Status | `code` | `reason` | When |
| --- | --- | --- | --- |
| `401` | `unauthenticated` | `peer_unknown` | No peer row, no pending pairing, or another secret |

The peer compares `node.id` with its row and `version` with its own on every
reconnection; a difference gives `unreachable` with reason `version`.

## `DELETE /api/peer/pairings/{nodeId}`

The caller has unpaired; this node deletes its peer row when `nodeId` matches
and answers `204`. A missing or different row also answers `204`.

## `POST /api/peer/vocabulary`

Applies a rename, merge or deletion of a tag, person or organization by name,
in one transaction ([plan.md, Vocabulary by name](../plan.md#vocabulary-by-name)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `kind` | body | `tag`, `person`, `organization` | Yes | The entity kind |
| `action` | body | `rename`, `merge`, `delete` | Yes | |
| `name` | body | string | For `rename` and `delete` | The current name |
| `newName` | body | string | For `rename` | |
| `into` | body | string | For `merge` | The destination name, created when absent |
| `sources` | body | string[] | For `merge` | 1–20,000 names; absent names are skipped |

**Response**: `200` with `{ changed: boolean, skipped: string[] }`; `changed`
is false when the entity is absent on this node.

| Status | `code` | `reason` | When |
| --- | --- | --- | --- |
| `409` | `conflict` | `tag_name_taken`, `person_name_taken`, `organization_name_taken` | `newName` exists |
| `400` | `invalid_request` | `tag_name_*` | `newName` fails the name rule |

The caller checks the conflict on its own node first and refuses before
calling, so a `409` here is the race, not the common case.

## `POST /api/peer/related`

Scores this node's visible videos against the clues of a video held by the
caller and returns one page in the related order
([plan.md, Related videos](../plan.md#related-videos)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `clues` | body | `RelatedClues` | Yes | Fingerprint, tags by name, the words the scorer takes from the title and the file name, `addedAt` of the open video; no directory |
| `exclude` | body | int64[] | No | This node's ids to leave out (the open video and its bundle when held here) |
| `cursor` | body | string | No | Previous `nextCursor` |
| `limit` | body | integer | No | 1–100, default 30 |

**Response**: `200` with `{ items: [{ score, video }], nextCursor }`;
`video` is the `Video` schema for the request's audience, and `score` the
scorer's points.

## `GET /api/peer/videos/{id}/clues`

Returns the `RelatedClues` of one video of this node, for the request's
audience; a hidden or missing video answers `404 not_found`.

## `POST /api/peer/rule-counts`

Returns the counts of this node's visible videos matching each rule, for
smart folder cards ([plan.md, Collections](../plan.md#collections)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `rules` | body | `SmartRuleByName[]` | Yes | 1–60 rules with tags, kind, person and organization by name, `q` and no `favorite`; the caller sends no rule of a folder with `favorite` |

**Response**: `200` with `{ items: [{ videoCount, durationMs, sizeBytes }] }`
in the order of `rules`.

## Per-item cursors

A list response to a request with the peer credential carries
`peerItems`, one entry per item in order, on `VideoPage` and `LibraryPage`;
other callers never receive it. `GroupListPage` has none: the group picker
reads one node's list ([screen-api.md, Groups by node](screen-api.md#groups-by-node)).

| Field | Type | Meaning |
| --- | --- | --- |
| `cursor` | string | The cursor that continues the list after this item |
| `sortValue` | string or null | The item's value under the request's sort, as the cursor encodes it; null sorts last |

## Name filters

`listVideos`, `listLibrary` and `listFolderVideos` take `tagName` (repeated,
at most 16 with `tag` together), `tagKindName`, `personName` and
`organizationName` beside the id parameters, with the same semantics. Any
caller may use them; a name with no entity matches no video. The peer uses
them to send a filter by a local entity.

## Client use

`internal/peer` sends every request with a 5 s connection timeout and a 10 s
response timeout, except media operations and `/api/events`, which have no
response timeout. A failed request reports to `app.Pairing`, which moves the
state to `unreachable`; the merged read answers with the local part.
