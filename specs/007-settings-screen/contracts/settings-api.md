# Settings API Contract

機械可読な正本は実装時に更新する [api/openapi.yaml](../../../api/openapi.yaml) とする。

## GET /api/settings

```json
{
  "mediaFolders": ["/srv/videos", "/mnt/archive"],
  "version": 3,
  "updatedAt": "2026-09-21T00:00:00Z"
}
```

初回は `mediaFolders: []`。応答に `Cache-Control: no-store` を付ける。

## PUT /api/settings

folder集合全体を置換する。

```json
{
  "mediaFolders": ["/srv/videos", "/mnt/archive"],
  "version": 3
}
```

成功は200と更新後resource。集合が変わった場合は応答前に旧ライブラリDBデータを削除する。
保存からscanを開始しない。同じ集合・同じ順序のPUTはDB削除とversion加算を行わない。

| Status | Code                            | Condition                             |
| ------ | ------------------------------- | ------------------------------------- |
| `400`  | `invalid_request`               | JSON、version、path形式が不正         |
| `400`  | `invalid_media_directory`       | 存在しない、directoryでない、読取不能 |
| `400`  | `overlapping_media_directories` | 重複または祖先・子孫関係がある        |
| `409`  | `scan_in_progress`              | 走査中                                |
| `409`  | `conflict`                      | versionが古い                         |
| `500`  | `internal`                      | transactionを含むその他の失敗         |

## GET /api/directories

- path省略: Linuxでは`/`、Windowsでは利用可能なdrive rootsを `directories` に返す
- path指定: 正規化した絶対pathを `currentPath`、親を `parentPath`、直下directoryを返す
- directory entryは `{ name, path }`
- ファイルを返さない
- 応答は `Cache-Control: no-store`

| Status | Code                    | Condition                      |
| ------ | ----------------------- | ------------------------------ |
| `400`  | `invalid_request`       | pathが絶対pathでない、形式不正 |
| `404`  | `not_found`             | pathが消失、directoryでない    |
| `400`  | `directory_unavailable` | 読取不能                       |
| `500`  | `internal`              | 列挙失敗                       |

## POST /api/scans

- `mediaFolders` が0件ならscan rowを作らず `409 media_folders_not_configured`
- scan開始時のfolder集合を最後まで使う
- 一部root/directory/fileのI/O失敗はfailed countへ記録し、残りを継続する
- 継続不能なerror、panic、cancelはscanをfailedへ確定する
- panicをHTTP server processへ伝播させない

## Runtime

- `MDM_MEDIA_DIR` と `MDM_SCAN_ON_START` は読まない。
- settings変更後はライブラリAPIが空を返し、次の手動scan完了後に全rootの結果を返す。
