# Data Model: 設定画面

## MediaSettings

| Field        | Meaning                | Rule                                            |
| ------------ | ---------------------- | ----------------------------------------------- |
| `id`         | singleton ID           | 常に`1`                                         |
| `media_dir`  | 保存済みメディアルート | nullable。初回はnull、保存時にdirectoryを再検証 |
| `version`    | 更新版                 | 1から始まり、更新ごとに加算                     |
| `updated_at` | 最終更新時刻           | 初回未設定を含めserverが設定                    |

- `media_dir = null` の間はscanを開始できない。
- running scan中と、要求versionが現在versionと異なる更新では変更しない。
- 保存はscanを作成しない。

## DirectoryListing

| Field         | Meaning                                       |
| ------------- | --------------------------------------------- |
| `currentPath` | 現在位置。filesystem root一覧ではnull         |
| `parentPath`  | 親。rootまたはroot一覧ではnull                |
| `directories` | 直下の読取可能なdirectory。ファイルを含まない |

各directoryは表示名と絶対pathを持つ。選択結果は候補に留まり、PUT成功時だけMediaSettingsになる。
