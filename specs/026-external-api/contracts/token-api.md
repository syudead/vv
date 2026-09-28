# Contract: API トークンの管理（画面の API）

親 Issue: #493（要件 1〜4・8）。正本は実装の時点で [api/openapi.yaml](../../../api/openapi.yaml) に
入れ、ここはその差分だけを書く。エラーの形は
[023 error-api.md](../../023-english-i18n/contracts/error-api.md) に従う。

3 つの操作はどれも `security: sessionCookie` で、`accessRoutes` に足さない（所有者だけ）。
Bearer は画面の API では読まれない（[research.md R-3](../research.md#r-3-外部連携-api-は-apiv1-の下に置き境界にbearerの分類を足す)）
ので、Bearer だけの要求は `401 unauthenticated` になる（受け入れ条件 8）。

## `GET /api/api-tokens`

`200`: `{ "items": APIToken[] }`。`created_at` の降順。

```yaml
APIToken:
  type: object
  additionalProperties: false
  required: [id, name, createdAt, lastUsedAt]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }
    createdAt: { type: string, format: date-time }
    lastUsedAt: { type: [string, "null"], format: date-time }  # 未使用は null
```

## `POST /api/api-tokens`

本文 `{ "name": string }`（JSON 必須、`requiresJSONBody` に足す）。

- `201`: `{ "token": APIToken, "secret": string }`。`secret` は平文で、この応答にしか出ない
  （要件 2）。応答は今の API と同じく `Cache-Control: no-store`。
- `400 invalid_request`: 名前の規則（[research.md R-10](../research.md#r-10-トークンの名前の規則はタグ名と同じ形にする)）の
  `reason` を付ける。

| `reason` | `limit` |
| --- | --- |
| `api_token_name_empty` | — |
| `api_token_name_control_characters` | — |
| `api_token_name_too_long` | `domain.APITokenNameMaxLength` |

## `DELETE /api/api-tokens/{id}`

`204`。無い id も `204`（二重の失効を誤りにしない）。確定した後、そのトークンの実行中の外部連携 API と
MCP の応答を打ち切る（[research.md R-9](../research.md#r-9-最終使用日時は-1-分に-1-回だけ書く失効は実行中の要求も止める)）。

## `mdm account`

`set-username`・`set-password` の完了の出力を、セッションと API トークンの両方が失効したことを
伝える文にする（今の文は [016 account-cli.md](../../016-single-account-auth/contracts/account-cli.md)）。
