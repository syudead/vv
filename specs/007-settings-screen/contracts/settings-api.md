# Settings API Contract

実装は最初に機械可読な正本 [api/openapi.yaml](../../../api/openapi.yaml) を更新し、その生成物に
handlerとclientを合わせる。本書はその変更内容を事前に定める設計契約とする。

## MediaFolder Resource

```json
{
  "id": 12,
  "path": "/srv/videos",
  "version": 2,
  "createdAt": "2026-09-21T00:00:00Z",
  "updatedAt": "2026-09-21T01:00:00Z"
}
```

一覧全体を置換するendpointは作らない。追加・変更・削除は常に1件単位で完了する。

## GET /api/media-folders

MediaFolder配列を`id`昇順で返す。初回は`[]`。応答に`Cache-Control: no-store`を付ける。

## POST /api/media-folders

1件を追加する。

```json
{ "path": "/mnt/archive" }
```

成功は`201`と作成したresource。MediaFolderへのinsertだけを行い、既存のvideos、FTS、jobs、scans、
playback progressを変更せず、scanも開始しない。

## PUT /api/media-folders/{id}

既存1件のpathを変更する。

```json
{
  "path": "/mnt/new-archive",
  "version": 2
}
```

成功は`200`と更新後resource。path更新と同じtransactionで、そのMediaFolder配下のvideo locationsを
削除する。locationが0件になったvideos、FTS、jobsだけを削除し、別MediaFolder側のlocationが残る
video、job、thumbnail、全playback progress、scan履歴は変更せず、scanも開始しない。
同じ正規化pathへのPUTはno-opとし、version加算とDB削除を行わない。

## DELETE /api/media-folders/{id}?version={version}

既存1件を削除する。成功は`204`。MediaFolder削除と同じtransactionで、そのfolder配下のvideo
locationsを削除し、locationが0件になったvideos、FTS、jobsだけを削除する。別MediaFolder側の
locationが残るvideo、job、thumbnail、全playback progress、scan履歴は変更しない。

## MediaFolder Mutation Errors

| Status | Code                            | Condition                                  |
| ------ | ------------------------------- | ------------------------------------------ |
| `400`  | `invalid_request`               | JSON、version、path形式が不正              |
| `400`  | `invalid_media_directory`       | 存在しない、directoryでない、読取不能      |
| `400`  | `unsupported_media_directory`   | symlinkまたはfilesystem/drive root         |
| `404`  | `media_folder_not_found`        | PUT/DELETE対象が存在しない                 |
| `409`  | `overlapping_media_directories` | 重複または別resourceと祖先・子孫関係がある |
| `409`  | `scan_in_progress`              | 走査中                                     |
| `409`  | `conflict`                      | 対象resourceのversionが古い                |
| `500`  | `internal`                      | transactionを含むその他の失敗              |

失敗時は対象MediaFolderとライブラリDBを変更しない。

## GET /api/directories

- path省略: Linuxでは`/`、Windowsでは利用可能なdrive rootsを`directories`に返す
- path指定: 正規化した絶対pathを`currentPath`、親を`parentPath`、直下directoryを返す
- directory entryは`{ name, path }`
- ファイルとsymlinkを返さない
- 応答は`Cache-Control: no-store`

| Status | Code                    | Condition                      |
| ------ | ----------------------- | ------------------------------ |
| `400`  | `invalid_request`       | pathが絶対pathでない、形式不正 |
| `404`  | `not_found`             | pathが消失、directoryでない    |
| `400`  | `directory_unavailable` | 読取不能                       |
| `500`  | `internal`              | 列挙失敗                       |

## POST /api/scans

- MediaFolderが0件ならscan rowを作らず`409 media_folders_not_configured`
- scan開始時のMediaFolder snapshotを最後まで使う
- 全rootを1回のscanとして扱い、root境界判定を変更・削除・streamと共通化する
- 同じcontent keyを別pathで発見した場合は既存locationを移動せず、同じvideoへlocationを追加する
- 一部root/directory/fileのI/O失敗はfailed countへ記録し、残りを継続する
- root列挙失敗時はroot全体、subtree失敗時はそのprefix、file失敗時はそのpathをmissing削除から除外する
- 継続不能なerror、panic、cancelはscanをfailedへ確定する
- panicをHTTP server processへ伝播させない

## Runtime

- `MDM_MEDIA_DIR` と `MDM_SCAN_ON_START` は読まない。
- directory APIとfolder mutationは認証導入前のtrusted-network運用に限定する。
- CORSを許可せず、全mutationをsame-originに限定し、POST/PUTは`application/json`だけを受理する。
- MediaFolder追加後も既存ライブラリAPIの結果を維持する。
- MediaFolder変更・削除後は対象folder側のlocationだけを除き、別folder側のlocationが残る動画は
  結果とstreamから除かない。
- folder操作後も再生位置と視聴済み状態を維持する。
- 次の手動scan完了後に現在の全MediaFolderの結果を返す。
