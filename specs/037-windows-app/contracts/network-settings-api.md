# Contract: LAN からの接続の許可（`/api/settings/network`）

正本は [api/openapi.yaml](../../../api/openapi.yaml)。ここに書くのは、この feature が足す経路・スキーマ・
エラーだけで、認証・同じオリジンの確認・`Error` の形は今の契約
（[specs/016-single-account-auth](../../016-single-account-auth/)、
[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md)）に従う。
決定の理由は [research.md R-14](../research.md#r-14-lan-からの接続の許可は設定表に保存し切り替えたら待ち受けを開き直す既定はループバックだけ)。

## 1. スキーマ

```yaml
NetworkSettings:
  type: object
  required: [lanAccess, port, addresses]
  additionalProperties: false
  properties:
    lanAccess:
      type: boolean       # 保存した選択。行が無ければ false
    port:
      type: integer       # 今の待ち受けのポート
    addresses:
      type: array         # lanAccess が true のときだけ要素が入る。false なら空
      items:
        type: string      # "http://192.168.1.20:47880/" の形。上がっている非ループバックの IPv4 ごとに 1 つ
UpdateNetworkSettingsRequest:
  type: object
  required: [lanAccess]
  additionalProperties: false
  properties:
    lanAccess:
      type: boolean
```

`Error.code` の列挙に `not_desktop` は足さない（`404` は `not_found`）。`Error.reason` に `listen_failed` を足す。

## 2. `GET /api/settings/network`

| 状況 | 応答 |
| --- | --- |
| デスクトップ版でない（Docker・直接起動） | `404` `not_found`。認証の判定より前に返す |
| 所有者のセッションでない | `403` `forbidden`（`/api/settings/transcoding` と同じ） |
| それ以外 | `200` `NetworkSettings` |

SPA は `404` を「この節を出さない」と読む。

## 3. `PUT /api/settings/network`

| 状況 | 応答 |
| --- | --- |
| デスクトップ版でない | `404` `not_found` |
| 所有者のセッションでない・同じオリジンでない | `403` `forbidden` |
| 本文が不正 | `400` `invalid_request` |
| 今と同じ値 | 何もせず `200` `NetworkSettings` |
| 新しいアドレスで待ち受けを開き直せない | 元のアドレスで待ち受けを戻し、保存値を変えずに `409` `conflict`・reason `listen_failed` |
| それ以外 | 待ち受けを開き直し、保存してから `200` `NetworkSettings`（開き直したあとの値） |

- 開き直しのあいだも、確立済みの接続（この要求自身、SSE の `/api/events`、配信中の動画）は切らない。
- `true` から `false` にしたあと、LAN の端末からの新しい接続は TCP の段で拒まれる（受け入れ条件 7）。
- 同時に来た 2 つの `PUT` は 1 つずつ処理する。
