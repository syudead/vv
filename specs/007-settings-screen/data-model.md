# Data Model: 設定画面

既存モデルは変更せず、次の singleton だけを追加する。

## MediaSettings

| Field        | Meaning                    | Rule                                               |
| ------------ | -------------------------- | -------------------------------------------------- |
| `id`         | singleton ID               | 常に `1`                                           |
| `media_dir`  | サーバー上のメディアルート | 保存時に絶対パス、存在、directory、readable を検証 |
| `version`    | 更新版                     | 1から始まり、更新ごとに加算                        |
| `updated_at` | 最終更新時刻               | サーバーが設定                                     |

- 初回だけ `MDM_MEDIA_DIR` から作成し、以後はDB値を優先する。
- running scan 中と、要求版が現在版と異なる更新では変更しない。
- 保存は scan を作成しない。
- scan は開始時の `media_dir` を処理終了まで使う。
