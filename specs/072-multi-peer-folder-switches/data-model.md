# Data model: Several read VVMDMs and media-folder switches

The rest of the model is unchanged:
[`internal/store/migrations`](../../internal/store/migrations), with the peer
tables in `00083_peer_links.sql` and the media folders in
`00003_media_folders.sql`.

## Migration

Each unit that needs a table change adds its own file after the last one on
`main` (`00092`):

| File | Unit | Change |
| --- | --- | --- |
| `00093_peer_link_slots.sql` | Remote ids by slot | Adds `slot` to `peer_link` (the existing row gets 1) and the `peer.next_slot` setting (2 after that row, else 1) |
| `00094_media_folder_switches.sql` | Folder scope | Adds `enabled` to `media_folders` (every row 1) |
| `00095_peer_media_folders.sql` | Link folder snapshot | Creates `peer_media_folders` |

## `peer_link`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `slot` | integer | no | Unique, 1 or more; the link's ids are offset by `slot × 2^40` |

The other columns are unchanged. The table holds up to 8 rows.

## `media_folders`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `enabled` | integer | no | 1 (the default) shows the folder's videos to this node's viewers; 0 leaves them out of this node's reads |

## `peer_media_folders`

The snapshot of a link's media folders with the owner's switches.

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `node_id` | text | no | The link; references `peer_link.node_id`, deleted with it |
| `folder_id` | integer | no | The folder's id on the link |
| `path` | text | no | The folder's path on the link, as last listed |
| `version` | integer | no | The folder's version on the link, as last listed |
| `enabled` | integer | no | The owner's switch on this node; 1 by default |
| `refreshed_at` | integer | no | Unix seconds of the last reconciliation |

**Relationships**: Primary key `(node_id, folder_id)`. Deleting the link
deletes its rows.

## Rules

| Rule | Enforced in |
| --- | --- |
| A slot is taken from `peer.next_slot` and the setting is advanced in the same transaction; a slot is never reused | `PeerStore.AddLink` |
| A ninth link answers `domain.ErrPeerLimit`; a link to a node already read answers `domain.ErrAlreadyPaired` | `domain.CheckNewLink` over the existing links, re-read in `PeerStore.AddLink` |
| Reconciling a snapshot deletes the rows whose id the link no longer lists, replaces a row whose `path` or `version` differs (with `enabled` 1), and inserts new rows with `enabled` 1 | `domain.ReconcilePeerFolders`, applied in `PeerStore.ReplacePeerFolders` |
| A switch of a local folder changes `enabled` only; `path` and `version` stay, so no rescan, ingest or sweep is caused | `SettingsStore.SetMediaFolderEnabled` |
| A viewer's folder scope either applies `media_folders.enabled` or an explicit list of ids, never both | `domain.Audience` and `listedLocationCondition` |

## What does not change

`peer_grant` stays at most one row: the Issue leaves being read by several
nodes out of scope. `video_locations` and the registered-location condition
are unchanged, so scans, watching, ingest and jobs keep treating a
switched-off folder as registered. No table stores a link's videos, groups or
vocabulary.
