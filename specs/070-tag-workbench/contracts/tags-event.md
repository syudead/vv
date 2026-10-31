# Contract: `tags` server event

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), the
description of `streamEvents` (`GET /api/events`).

## `event: tags`

Tells an owner's screens that the rows of the tag list may have changed; the
data is an empty object, and the screen re-reads what it needs.

| Property | Value |
| --- | --- |
| Audience | Owners only; guests and the peer credential receive none |
| Data | `{}` |
| Coalescing | Several changes before a subscriber's next write become one event, as `candidates` does |
| Replay | None after a reconnect |
| Paired node | The peer's `tags` events are not forwarded; a peer change reaches the screen through `peerItems`, as today |

The table shows which committed writes send it.

| Write | Sends `tags` |
| --- | --- |
| Tag create, rename, delete, merge, confirm, reject, batch | Yes |
| Synonym add, remove, promote, move, detach | Yes |
| Tag kind create, rename, delete, card-first; a tag's kind set or cleared | Yes |
| A video's tag attached, detached, confirmed or rejected, by the owner, the external API, work-info apply or automatic tagging | Yes |
| Tentative tags created by the external API or work-info apply | Yes |
| A scan that changes folder names, so folder-derived counts | No |
| A rolled-back write | No |

The store publishes `domain.TagsChanged` after commit; `internal/httpapi`
maps it to the event.
