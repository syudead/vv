# Contract: Screen API for the settings pages

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This document
holds only what this feature changes. The error shape is
[023 error-api.md](../../023-english-i18n/contracts/error-api.md).
`api/external-v1.yaml` is unchanged.

## `OllamaSettings` delta

`getOllamaSettings` and `updateOllamaSettings` answer the same schema; it
gains one field ([R-4](../research.md#r-4-ollamasettings-reports-whether-the-connection-was-saved)).

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `saved` | boolean | Yes | The owner has saved the connection (a row exists, as the duplicate judge reads it); `false` while `endpoint` and `model` are the defaults the server answers for an unsaved connection |

`updateOllamaSettings` answers `saved: true`. Endpoints, bodies, statuses and
errors are unchanged.

## Client use

The Auto-tagging and Library pages read `GET /api/settings/ollama` once when
they open and take `saved` as the connection's state
([R-5](../research.md#r-5-the-auto-tagging-page-is-gated-by-the-connection-with-three-other-states));
a request that fails leaves the state unknown. The Connections page keeps
reading the resource for its form as today.
