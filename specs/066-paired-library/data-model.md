# Data model: Pair two VVMDMs on one LAN into one library

The rest of the model is unchanged:
[`internal/store/migrations`](../../internal/store/migrations). No user-data
table gains a node column; a remote entity is never stored locally.

## Migration

`00079_peer.sql` creates `peer` and adds nothing else; the node id is a row of
the existing `settings` table.

## `peer`

One row at most: the node is paired with one peer or with none.

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `node_id` | text, primary key | No | The peer's node id (UUID), compared on every handshake |
| `name` | text | No | The peer's host name at pairing, shown on the screens |
| `address` | text | No | The peer's base URL (`http://` or `https://`), edited by the owner |
| `outgoing_secret` | text | No | The secret this node presents to the peer, in plain text |
| `incoming_secret_hash` | text | Yes | Hexadecimal SHA-256 of the secret the peer presents, as `api_tokens.token_hash`; null once the peer answered `401 peer_unknown` |
| `pinned_spki` | text | Yes | Base64 SHA-256 of the peer certificate's SPKI the owner confirmed; null when the system roots verify it or the address is `http://` |
| `paired_at` | integer | No | Unix seconds |

**Relationships**: None; the row refers to no other table and nothing refers
to it.

## `settings`

| Key | Value | Meaning |
| --- | --- | --- |
| `node.id` | UUID v4 | This node's identity, created on first start; the peer stores it as its `peer.node_id` |

## Rules

| Rule | Enforced in |
| --- | --- |
| At most one peer | `PeerStore.AddPeer` inside its transaction, answering `domain.ErrAlreadyPaired` |
| The incoming secret is stored only as a hash | `PeerStore.AddPeer`; `PeerStore.PeerBySecret` hashes the presented value |
| A credential change does not touch `peer` | `AuthStore` credential change leaves the table alone; the store test checks it |
| Unpairing deletes the row and nothing else | `PeerStore.DeletePeer` |
| `401 peer_unknown` from the peer revokes the incoming secret, and a row without it accepts no peer credential | `PeerStore.RevokeIncomingSecret`; `PeerStore.PeerBySecret` never matches a null hash |
| A wiped peer answers `401 peer_unknown` | The absence of a row, or a `node_id` different from the one presented |

## State transitions

The connection state is held in memory by `app.Pairing`
([plan.md, Connection state](plan.md#connection-state)) and is not stored; a
start begins as `revoked` when the row has no incoming secret hash,
`unreachable` with reason `network` when a row has one, and `unpaired`
otherwise.

## What does not change

| Expected change | Why not |
| --- | --- |
| A node column on `tags`, `people`, `organizations` | Remote entities are read from the peer on each request and joined by name |
| A `remote_videos` cache | Requirement 15 hides the peer's videos while it is unreachable, which a cache would contradict |
| `api_tokens` | The peer credential has no `account_version`, so it lives in its own table |
| The pairing code | Kept in memory by `app.Pairing`; a restart ends it |
