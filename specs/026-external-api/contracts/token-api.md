# Contract: API token management (UI API)

Parent Issue: #493 (Requirements 1-4 and 8). The source of truth goes into
[api/openapi.yaml](../../../api/openapi.yaml) during implementation. This contract records only the
difference. The error shape follows
[023 error-api.md](../../023-english-i18n/contracts/error-api.md).

All three operations have `security: sessionCookie` and are not added to `accessRoutes` (owner
only). The UI API does not read Bearer
([research.md R-3](../research.md#r-3-the-external-api-lives-under-apiv1-the-boundary-gets-a-bearer-class)),
so a request with only Bearer gets `401 unauthenticated` (Acceptance criterion 8).

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

Body: `{ "name": string }`. JSON is required; the operation is added to `requiresJSONBody`.

- `201`: `{ "token": APIToken, "secret": string }`. `secret` is the plaintext and appears only in
  this response (Requirement 2). Like the current API, the response has `Cache-Control: no-store`.
- `400 invalid_request`: carries the `reason` of the name rule
  ([research.md R-10](../research.md#r-10-token-names-follow-the-same-rule-shape-as-tag-names)).

| `reason` | `limit` |
| --- | --- |
| `api_token_name_empty` | — |
| `api_token_name_control_characters` | — |
| `api_token_name_too_long` | `domain.APITokenNameMaxLength` |

## `DELETE /api/api-tokens/{id}`

`204`. An unknown id also returns `204`, so a double revoke is not an error. After the revoke
commits, running external API and MCP responses for that token are cut off
([research.md R-9](../research.md#r-9-the-last-use-time-is-written-at-most-once-a-minute-a-revoke-also-stops-running-requests)).

## `mdm account`

The completion output of `set-username` and `set-password` states that both the sessions and the API
tokens were revoked. The current text is in
[016 account-cli.md](../../016-single-account-auth/contracts/account-cli.md).
