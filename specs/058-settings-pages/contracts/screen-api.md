# Contract: Screen API for the settings pages

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This document
holds only what this feature changes. The error shape is
[023 error-api.md](../../023-english-i18n/contracts/error-api.md).
`api/external-v1.yaml` is unchanged.

## `OllamaSettings` delta

`OllamaSettings` is the body `getOllamaSettings` and `updateOllamaSettings`
answer; it gains one field
([R-4](../research.md#r-4-ollamasettings-reports-whether-the-connection-was-saved)).

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `saved` | boolean | Yes | The owner has saved the connection (a row exists, as the duplicate judge reads it); `false` while `endpoint` and `model` are the defaults the server answers for an unsaved connection |

`updateOllamaSettings` answers `saved: true`.

## `UpdateOllamaSettingsRequest`

The `PUT /api/settings/ollama` request body stops referring to
`OllamaSettings` and gets its own schema with the two fields it accepts
today, so `saved` is never part of a request.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `endpoint` | string | Yes | As `OllamaSettings.endpoint` |
| `model` | string | Yes | As `OllamaSettings.model` |

`additionalProperties: false`, as the handler's decoder rejects any other
field with `400`. The SPA's `updateOllamaSettings` sends `endpoint` and
`model` only, never the read body as it is. Endpoints, statuses and errors are
unchanged.

## Client use

The Auto-tagging and Library pages read `GET /api/settings/ollama` once when
they open and take `saved` as the connection's state
([R-5](../research.md#r-5-the-auto-tagging-page-is-gated-by-the-connection-with-three-other-states));
a request that fails leaves the state unknown. The Connections page keeps
reading the resource for its form as today.
