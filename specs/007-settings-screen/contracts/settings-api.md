# Settings API Contract

機械可読な正本は実装時に更新する [api/openapi.yaml](../../../api/openapi.yaml) とする。

## GET /api/settings

```json
{
  "mediaDir": null,
  "version": 1,
  "updatedAt": "2026-09-21T00:00:00Z"
}
```

`mediaDir` は未設定時にnull、設定後はサーバー上の絶対パス。`Cache-Control: no-store` を付ける。

## PUT /api/settings

folder pickerが返した候補を保存する。

```json
{
  "mediaDir": "/srv/videos",
  "version": 1
}
```

成功は200と更新後resource。保存からscanを開始しない。

| Status | Code                      | Condition                             |
| ------ | ------------------------- | ------------------------------------- |
| `400`  | `invalid_request`         | JSON、version、path形式が不正         |
| `400`  | `invalid_media_directory` | 存在しない、directoryでない、読取不能 |
| `409`  | `scan_in_progress`        | 走査中                                |
| `409`  | `conflict`                | versionが古い                         |
| `500`  | `internal`                | その他の失敗                          |

## GET /api/directories

- path省略: filesystem rootを `directories` に返す。Linuxでは`/`、Windowsでは利用可能なdrive
- path指定: 正規化した絶対pathを `currentPath`、親を `parentPath`、直下directoryを
  `directories` に返す
- directory entryは `{ name, path }`
- ファイルを返さない
- 応答は `Cache-Control: no-store`

| Status | Code                    | Condition                      |
| ------ | ----------------------- | ------------------------------ |
| `400`  | `invalid_request`       | pathが絶対pathでない、形式不正 |
| `404`  | `not_found`             | pathが消失、directoryでない    |
| `400`  | `directory_unavailable` | 読取不能                       |
| `500`  | `internal`              | 列挙失敗                       |

## Runtime

- `MDM_MEDIA_DIR` と `MDM_SCAN_ON_START` は読まない。
- `mediaDir = null` では手動scanを開始しない。
- 保存後も旧索引は次の手動走査まで残るが、streamは現行mediaDir外を404として拒否する。
