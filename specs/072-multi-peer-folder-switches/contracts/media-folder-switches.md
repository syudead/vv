# Contract: Media-folder switches

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), operations
`listMediaFolders`, `getVideo`, `listVideos`, `listLibrary` and
`streamEvents`, plus the operations this feature adds below. The peer
credential boundary is in
[authentication.md](../../../docs/design-docs/authentication.md#paired-vvmdm).

## `PUT /api/media-folders/{id}/enabled`

Switches one local folder on or off; owner only.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `id` | path | int64 | yes | The folder |
| `enabled` | body | boolean | yes | `false` leaves the folder's videos out of this node's reads |

**Response**: `200` with `MediaFolder`, which gains `enabled`. Allowed during
a scan. Sends `mediaFolders` after commit.

| Status | `code` | When |
| --- | --- | --- |
| `404` | `not_found` | No such folder |

## `GET /api/peer/links/{nodeId}/media-folders`

Answers the snapshot of that link's media folders with their switches, and
the link's state, so the section renders while the link is unreachable.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `state`, `reason` | body | | yes | The link's connection state as `GET /api/peer` gives it |
| `folders[]` | body | `PeerMediaFolder[]` | yes | In the link's id order: `id` (the link's own id), `path`, `enabled`, `refreshedAt` |

**Response**: `200`. An unknown node id answers `404 not_found`.

## `PUT /api/peer/links/{nodeId}/media-folders/{folderId}/enabled`

Switches one of the link's folders on or off on this node only.

**Response**: `200` with the `PeerMediaFolder`; sends `mediaFolders` to this
node's viewers. A folder not in the snapshot answers `404 not_found`. The
link is never written.

## `GET /api/peer/media-folders`

Answered by the read node to the peer credential: its media folders as `id`,
`path` and `version`, in id order. `GET /api/media-folders` stays refused to
the credential with `403 peer_not_allowed`.

## `X-VV-Peer-Excluded-Folders`

A request header the Forwarder adds to every request to a link: the link's
own ids of the folders switched off on this node, comma-separated, absent
when none is off. The read node's boundary turns it into the request's folder
scope and applies none of its own switches to that request. A malformed
value answers `400 invalid_request`.

## Reads that leave switched-off folders out

Every list, search, count and folder read of this node, for every audience,
and the same reads of `/api/v1` and `/mcp`.

| Read | Behaviour |
| --- | --- |
| `GET /api/videos`, `/api/library`, `/api/folders`, `/api/folders/{rootId}`, `/api/folders/{rootId}/videos`, `/api/groups`, `/api/groups/{id}/items`, `/api/smart-folders/*`, `/api/watch-history`, `/api/videos/{id}/related`, tag, kind, people and organization lists and counts | The folder's videos are left out and not counted |
| `GET /api/videos`, `/api/library` with no item | `emptyReason: all_folders_off` when every folder this node shows is off |
| `GET /api/videos/{id}`, its media and subtitles, playlist items | Answered; `Video.folderOff: true` while the folder is off |
| A read of a link, forwarded or merged | The link applies the header's ids |

## `GET /api/events`

`mediaFolders` is a new event (an empty object) sent to every viewer after a
switch, an added, replaced or deleted folder commits; a link's `mediaFolders`
event is forwarded too and refreshes the snapshot.

## Client use

`MediaFoldersSection` reads `GET /api/media-folders` and one
`GET /api/peer/links/{nodeId}/media-folders` per link from `GET /api/peer`,
and rereads them on `mediaFolders` and `peer`. Open lists reload on
`mediaFolders` as they do on an all-folders `smartFolder` event.
