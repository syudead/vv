# Contract: Allowing connections from the LAN (`/api/settings/network`)

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This document
covers only the routes, schemas and errors this feature adds. Authentication,
the same-origin check and the shape of `Error` follow the existing contracts
([specs/016-single-account-auth](../../016-single-account-auth/),
[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md)).
The reasons for the decisions are in
[research.md R-14](../research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only).

## 1. Schemas

```yaml
NetworkSettings:
  type: object
  required: [lanAccess, port, addresses]
  additionalProperties: false
  properties:
    lanAccess:
      type: boolean       # The saved choice. false when there is no row
    port:
      type: integer       # The port currently listened on
    addresses:
      type: array         # Has elements only when lanAccess is true; empty when false
      items:
        type: string      # In the form "http://192.168.1.20:47880/". One per non-loopback IPv4 address that is up
UpdateNetworkSettingsRequest:
  type: object
  required: [lanAccess]
  additionalProperties: false
  properties:
    lanAccess:
      type: boolean
```

The `Error.code` enumeration gets no `not_desktop` (`404` is `not_found`).
`Error.reason` gets `listen_failed`.

## 2. `GET /api/settings/network`

| Situation | Response |
| --- | --- |
| Not an owner session | The existing authentication boundary returns `401` `unauthenticated` (the same as `/api/settings/transcoding`; the request does not reach the handler) |
| Not the desktop app (Docker, direct start) | `404` `not_found` |
| Otherwise | `200` `NetworkSettings` |

The rows are checked from the top. The SPA (owner) reads `404` as "do not show
this section".

## 3. `PUT /api/settings/network`

| Situation | Response |
| --- | --- |
| Not an owner session | `401` `unauthenticated` (authentication boundary) |
| Not the same origin | `403` `forbidden`, reason `cross_origin` (existing boundary) |
| Not the desktop app | `404` `not_found` |
| Malformed body | `400` `invalid_request` |
| Same value as now | Does nothing and returns `200` `NetworkSettings` |
| The listener cannot be reopened on the new address | Restores the listener on the original address, leaves the saved value unchanged, and returns `409` `conflict`, reason `listen_failed` |
| Reopened, but saving failed (disk full, I/O error) | Reopens the listener on the original address and returns `500` `internal` |
| Otherwise | Reopens the listener, saves, then returns `200` `NetworkSettings` (the values after reopening) |

- Reopening does not drop established connections (this request itself, SSE
  on `/api/events`, videos being streamed).
- After a change from `true` to `false`, new connections from LAN devices are
  refused at the TCP level (acceptance criterion 7).
- Two concurrent `PUT` requests are processed one at a time.
- After any error response, the listener's address matches the saved value
  (the listener never stays on `0.0.0.0` when access is not allowed).
