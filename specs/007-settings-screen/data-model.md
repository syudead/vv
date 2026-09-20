# Data Model: 設定画面

## MediaFolder

各行を独立して追加・変更・削除する。folder一覧全体のdraftやrevisionは持たない。

| Field        | Rule                                    |
| ------------ | --------------------------------------- |
| `id`         | primary key                             |
| `path`       | 正規化済みserver絶対path、unique        |
| `version`    | 1から始まり、その行のpath変更ごとに加算 |
| `created_at` | serverが設定                            |
| `updated_at` | serverが設定                            |

- 初回は0行。
- 同一pathと祖先・子孫関係のpathを同時に持たない。
- 表示順は`id`の昇順とし、並び替え設定は持たない。
- 1件以上ある場合だけscanを開始できる。

## Video and VideoLocation

`videos`は内容単位、`video_locations`はfilesystem上の実在path単位に分ける。同じ`content_key`の
ファイルが複数rootにあっても1つのvideoと複数locationとして保持する。

### videos

`content_key`、probe結果、再生可否、thumbnail stateなどpathに依存しない情報を持つ。既存の
`content_key` uniqueとvideo IDを維持する。path、title、size、mtimeはlocationへ移す。

### video_locations

| Field        | Rule                                      |
| ------------ | ----------------------------------------- |
| `id`         | primary key                               |
| `video_id`   | videosへのforeign key、video削除時cascade |
| `path`       | 正規化済み絶対path、unique                |
| `title`      | pathのbasenameから導出                    |
| `size_bytes` | locationの走査時点の値                    |
| `mtime`      | locationの走査時点の値                    |
| `created_at` | serverが設定                              |
| `updated_at` | serverが設定                              |

locationのpathがMediaFolder path配下にあるとき、そのfolder由来と判定する。単純な文字列prefixではなく、
volume・separator・case sensitivityをOS規則に従って扱う共通helperを使う。

APIで1つだけpath/titleを示す場合は、現在登録済みroot内にあるlocationをpath昇順で選ぶ。streamは
同じ順序で各locationを安全検証し、最初に利用可能な実fileを配信する。1locationが消失しても別locationが
あれば同じvideoを表示・配信できる。

`videos_fts`はlocation単位のtitle/pathを索引し、検索結果を`video_id`で重複排除する。locationの
insert/update/deleteでFTSを同期する。

migrationは各既存videoのpath、title、size、mtimeを1件のVideoLocationへ移し、video ID、
content key、probe結果、job、playback progressを保持する。既存library dataを削除しない。

## Atomic Folder Operations

### Add

1. running scanがないことを確認する
2. candidateのabsolute、exists、directory、readable、非symlink、非filesystem-root、重複・包含を検証する
3. MediaFolderを1行insertする
4. commitする

追加transactionは`videos`、`videos_fts`、`jobs`、`scans`、`playback_progress`へ書き込まない。

### Replace Existing Path

1. `id`とexpected `version`、running scanなしを確認する
2. candidateを検証する
3. 旧root配下にあるVideoLocationと影響を受けるvideo IDを確保する
4. 対象folderのpathとversionを更新する
5. 手順3のlocationsを削除し、location FTSを同期する
6. locationが0件になったvideosだけを削除し、そのjobsをcascadeさせる
7. commitする

### Delete

Replaceと同じ対象特定・従属データ削除を行い、MediaFolder行を削除してcommitする。

いずれも途中失敗時はfolderとライブラリDBの双方を変更しない。別rootのlocationが残るvideo、
そのjobとthumbnail、全`playback_progress`、全scan履歴には触れない。locationが0件になって削除した
videoのthumbnailだけをcommit後にbest-effort cleanupする。

## In-flight Job Write Protection

job workerはclaim後に取得した`video_id`と`content_key`を処理identityとして保持する。probe結果、
probe失敗、thumbnail stateの全write-backは`WHERE id = ? AND content_key = ?`で条件付ける。該当行が
0件なら、folder変更・削除またはvideo差し替え後のstale resultとして破棄し、別videoへ書き込まない。
job完了・失敗の記録も、削除済みjobが0件更新となる場合を正常なstale completionとして扱う。

## DirectoryListing

| Field         | Meaning                                                  |
| ------------- | -------------------------------------------------------- |
| `currentPath` | 現在位置。filesystem root一覧ではnull                    |
| `parentPath`  | 親。rootまたはroot一覧ではnull                           |
| `directories` | 直下の読取可能な実directory。ファイルとsymlinkを含まない |

## Scan Root Snapshot

scan開始時にMediaFolderの`id`、`path`、`version`を`id`順で複製する実行時値。走査中はfolder操作を
拒否するため、1回のscanは同じroot集合を最後まで使う。scannerはsymlinkを辿らず、全rootを通じて
seen location pathsと列挙不能prefixを管理する。

- 新しいpathはcontent keyに対応するvideoを作成または再利用し、locationをupsertする
- 同じcontentが別pathにあっても既存locationを移動せず、追加locationとして保持する
- rootを最後まで列挙できた場合だけ、そのroot内のmissing locationを削除候補にする
- root自体の列挙に失敗した場合は、そのroot配下の既存locationをすべて保持する
- subtreeの列挙に失敗した場合は、そのprefix配下の既存locationを保持する
- fileのstat/openに失敗した場合は、そのpathの既存locationを保持する
- missing locations削除後、locationが0件になったvideoだけを削除する
- DB/reporting errorやpanicでscan全体がfailedになった場合は、missing削除を実行しない

これにより、一時的なmount消失やpermission errorを「ファイルが削除された」と誤認しない。
