# Settings API Contract

機械可読な正本は実装時に更新する [api/openapi.yaml](../../../api/openapi.yaml) とする。

## Endpoints

| Method / Path       | Request                 | Success                                    |
| ------------------- | ----------------------- | ------------------------------------------ |
| `GET /api/settings` | -                       | `200` + `{ mediaDir, version, updatedAt }` |
| `PUT /api/settings` | `{ mediaDir, version }` | `200` + 更新後の設定                       |

settings 応答は `Cache-Control: no-store` とする。PUT は走査を開始しない。

## PUT Errors

| Status | Code                      | Condition                             |
| ------ | ------------------------- | ------------------------------------- |
| `400`  | `invalid_request`         | JSON、version、絶対パス形式が不正     |
| `400`  | `invalid_media_directory` | 存在しない、directoryでない、読取不能 |
| `409`  | `scan_in_progress`        | 走査中                                |
| `409`  | `conflict`                | version が古い                        |
| `500`  | `internal`                | その他の失敗                          |

失敗時は保存値を変更しない。保存後も旧索引は次の手動走査まで残るが、動画配信は現行の
`mediaDir` 外を `404 not_found` として拒否する。`MDM_SCAN_ON_START` の値にかかわらず、
起動時走査は行わない。
