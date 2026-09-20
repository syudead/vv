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
`videos_fts`を同期し、`jobs`は既存foreign keyでcascadeする。対象`content_key`の
`playback_progress`も同じtransactionで削除する。完了済みの`scans`は履歴として残す。

## Atomic Folder Operations

### Add

1. running scanがないことを確認する
2. candidateのabsolute、exists、directory、readable、重複・包含を検証する
3. MediaFolderを1行insertする
4. commitする

追加transactionは`videos`、`videos_fts`、`jobs`、`scans`、`playback_progress`へ書き込まない。

### Replace Existing Path

1. `id`とexpected `version`、running scanなしを確認する
2. candidateを検証する
3. 旧root配下にあるvideoのIDと`content_key`を確保する
4. 対象folderのpathとversionを更新する
5. 手順3のvideosを削除し、FTSとjobsをcascadeさせる
6. 手順3の`playback_progress`を削除する
7. commitする

### Delete

Replaceと同じ対象特定・従属データ削除を行い、MediaFolder行を削除してcommitする。

いずれも途中失敗時はfolderとライブラリDBの双方を変更しない。他folderのvideo、job、
playback progressと全scan履歴には触れない。対象thumbnail filesはcommit後にbest-effort cleanupし、
失敗してもDBから参照されない。

## DirectoryListing

| Field         | Meaning                                       |
| ------------- | --------------------------------------------- |
| `currentPath` | 現在位置。filesystem root一覧ではnull         |
| `parentPath`  | 親。rootまたはroot一覧ではnull                |
| `directories` | 直下の読取可能なdirectory。ファイルを含まない |

## Scan Root Snapshot

scan開始時にMediaFolderの`id`、`path`、`version`を`id`順で複製する実行時値。走査中はfolder操作を
拒否するため、1回のscanは同じroot集合を最後まで使う。scannerは全rootを通じてseenを管理し、
snapshot内のroot配下にある既存videoのうち見つからなかったものだけを削除する。
