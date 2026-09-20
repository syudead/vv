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

## Video Root Membership

videoの正規化済み絶対`path`がMediaFolderの正規化済み`path`配下にあるとき、そのfolder由来と判定する。
単純な文字列prefixではなく、volume・separator・case sensitivityをOS規則に従って扱う共通helperを使う。
folder同士の重複・包含を禁止するため、1つのvideoが複数folderへ所属することはない。

`videos`へfolder IDを追加せず、既存videoをmigration時に削除・更新しない。folderの変更・削除時は
transaction内で旧root配下のvideo IDと`content_key`を抽出する。`videos`を削除すると既存triggerが
`videos_fts`を同期し、`jobs`は既存foreign keyでcascadeする。`playback_progress`は再構築不能な
利用者データなので削除しない。完了済みの`scans`も履歴として残す。

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
3. 旧root配下にあるvideoのIDと`content_key`を確保する
4. 対象folderのpathとversionを更新する
5. 手順3のvideosを削除し、FTSとjobsをcascadeさせる
6. commitする

### Delete

Replaceと同じ対象特定・従属データ削除を行い、MediaFolder行を削除してcommitする。

いずれも途中失敗時はfolderとライブラリDBの双方を変更しない。全`playback_progress`、他folderの
video・job、全scan履歴には触れない。対象thumbnail filesはcommit後にbest-effort cleanupし、
失敗してもvideoから参照されない。

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
seenと列挙不能prefixを管理する。

- rootを最後まで列挙できた場合だけ、そのroot内のmissing videoを削除候補にする
- root自体の列挙に失敗した場合は、そのroot配下の既存videoをすべて保持する
- subtreeの列挙に失敗した場合は、そのprefix配下の既存videoを保持する
- fileのstat/openに失敗した場合は、そのpathの既存videoを保持する
- DB/reporting errorやpanicでscan全体がfailedになった場合は、missing削除を実行しない

これにより、一時的なmount消失やpermission errorを「ファイルが削除された」と誤認しない。
