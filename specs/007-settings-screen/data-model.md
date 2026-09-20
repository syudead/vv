# Data Model: 設定画面

## MediaSettings

folder集合全体のrevisionを持つsingleton。

| Field        | Rule                            |
| ------------ | ------------------------------- |
| `id`         | 常に`1`                         |
| `version`    | 1から始まり、集合更新ごとに加算 |
| `updated_at` | serverが設定                    |

## MediaFolder

| Field         | Rule                                  |
| ------------- | ------------------------------------- |
| `id`          | primary key                           |
| `settings_id` | MediaSettings `id = 1`へのforeign key |
| `path`        | 正規化済みserver絶対path、unique      |
| `position`    | 画面表示順、0以上、settings内でunique |

- 初回はMediaSettings 1行、MediaFolder 0行。
- 同一pathと祖先・子孫関係のpathを同時に持たない。
- 1件以上ある場合だけscanを開始できる。

## Settings Update Transaction

folder集合が変わるPUTは次を1transactionで行う。

1. expected version、running scanなし、全候補の再検証を確認
2. MediaFolder rowsを置換
3. MediaSettings versionとupdated_atを更新
4. videosと同期FTS、jobs、scans、playback_progressを削除
5. commit

失敗時は設定集合と旧ライブラリDBの両方を変更しない。thumbnail filesはcommit後にcleanupし、
失敗してもDBから参照されない。

## DirectoryListing

| Field         | Meaning                                       |
| ------------- | --------------------------------------------- |
| `currentPath` | 現在位置。filesystem root一覧ではnull         |
| `parentPath`  | 親。rootまたはroot一覧ではnull                |
| `directories` | 直下の読取可能なdirectory。ファイルを含まない |

## Scan Root Snapshot

scan開始時にMediaFolderをposition順で複製する実行時値。走査中は設定更新を拒否するため、
1回のscanは同じroot集合を最後まで使う。
