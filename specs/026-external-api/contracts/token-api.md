# Contract: API token management (screen API)

Parent Issue: #493 (requirements 1–4 and 8). Source of truth: the operations go into
[api/openapi.yaml](../../../api/openapi.yaml) at implementation time; this file records only the
delta. The error shape follows [023 error-api.md](../../023-english-i18n/contracts/error-api.md).

All three operations are `security: sessionCookie` and are not added to `accessRoutes` (owner only).
The screen API does not read Bearer tokens
([research.md R-3](../research.md#r-3-the-external-api-lives-under-apiv1-and-the-boundary-gains-a-bearer-class)),
so a request with only a Bearer token gets `401 unauthenticated` (acceptance criterion 8).

## `GET /api/api-tokens`

`200`: `{ "items": APIToken[] }`, in descending `created_at` order.

```yaml
APIToken:
  type: object
  additionalProperties: false
  required: [id, name, createdAt, lastUsedAt]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }
    createdAt: { type: string, format: date-time }
    lastUsedAt: { type: [string, "null"], format: date-time }  # null when never used
```

## `POST /api/api-tokens`

Body `{ "name": string }` (JSON required; added to `requiresJSONBody`).

| Status | Body |
| --- | --- |
| `201` | `{ "token": APIToken, "secret": string }`. `secret` is the plaintext and appears only in this response (requirement 2). As with the existing API, the response is `Cache-Control: no-store`. |
| `400 invalid_request` | Carries a `reason` from the name rules ([research.md R-10](../research.md#r-10-token-names-follow-the-same-rules-as-tag-names)), listed below. |

| `reason` | `limit` |
| --- | --- |
| `api_token_name_empty` | — |
| `api_token_name_control_characters` | — |
| `api_token_name_too_long` | `domain.APITokenNameMaxLength` |

## `DELETE /api/api-tokens/{id}`

`204`. A missing id also returns `204` (revoking twice is not an error). Once committed, in-flight
external API and MCP responses for that token are cut off
([research.md R-9](../research.md#r-9-last-used-time-is-written-at-most-once-a-minute-revocation-also-stops-in-flight-requests)).

## `mdm account`

The completion output of `set-username` and `set-password` becomes a sentence saying that both the
sessions and the API tokens were revoked (the current text is in
[016 account-cli.md](../../016-single-account-auth/contracts/account-cli.md)).
