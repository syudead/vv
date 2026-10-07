---
source: specs/042-folder-watch-import/data-model.md
sourceHash: 38614d9c6555e003473e70875d84cd52248f45971db54ab1396f99bc5b3ef5fd
---

# データモデル: 変更されたメディアフォルダを自動で取り込む {#data-model-auto-import-changed-media-folders}

モデルの残りは変わらない: スキーマは [internal/store/migrations](../../internal/store/migrations) にあり、取り込みのテーブルは [024 データモデル](../024-import-progress/data-model.md) が説明する。テーブルは加えない。印の付いたディレクトリの集合と監視の問題はメモリの中にだけある ([research.md R-8](research.md#r-8-an-interrupted-watch-batch-is-closed-not-resumed))。

## マイグレーション {#migration}

`00033_scan_origin.sql` は `scans.origin` を加え、既存のすべての行に `'manual'` を入れる。

## `scans.origin` (加える列) {#scansorigin-added-column}

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `origin` | text、`check (origin in ('manual', 'watch'))`、既定値 `'manual'` | 不可 | 誰がスキャンを始めたか: `manual` は所有者 (または起動時の再開、または外部のクライアント)、`watch` はフォルダの監視 ([R-4](research.md#r-4-a-watch-batch-is-a-scan-row-with-origin-watch)) |

## `settings` のキー `library.auto_import` {#settings-key-libraryauto_import}

| 値 | 意味 |
| --- | --- |
| ない | 自動取り込みはオン (Issue の既定) |
| `true` | オン |
| `false` | オフ |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 実行するスキャンは、起点にかかわらず多くとも 1 つ | `scans_single_running_idx` (変わらない) |
| 監視によるスキャンが場所を削除するのは、そのパスが印の付いたディレクトリの直下か印の付いたサブツリーの下にあり、まとまりがそれを見つけなかったときだけである ([R-2](research.md#r-2-a-change-marks-a-directory-dirty-and-only-dirty-directories-are-re-read)) | `internal/scanner` |
| 監視によるスキャンのすべての追加は、最初の削除より先に書く ([R-3](research.md#r-3-a-batch-waits-for-quiet-imports-settled-files-then-removes)) | `internal/scanner` |
| 取って代わられた監視によるスキャンは削除を書かず、`done` で閉じる ([R-5](research.md#r-5-a-manual-scan-or-a-media-folder-change-supersedes-a-watch-batch)) | `internal/app` (`Scans`)、`internal/scanner` |
| 監視によるスキャンを始めるとき、前のスキャンの `scan_issues` の行のうちパスが印の付いた集合の外にあるものを残し、新しいスキャンに付け直す。集合の中の行は捨てて評価し直す。手動のスキャンは今までどおりすべてを消す | `internal/store` (`startScan`) |
| 監視によるスキャンは継承を準備のできた状態にしない。それを適用するのは `done` の手動のスキャンだけである ([R-9](research.md#r-9-same-path-content-changes-wait-for-a-full-scan)) | `internal/store` (`FinishScan`) |
| 起動時は、どちらの起点の中断されたスキャンも閉じ、`manual` のものだけを再開する ([R-8](research.md#r-8-an-interrupted-watch-batch-is-closed-not-resumed)) | `internal/app` (`Scans`) |

## 変わらないもの {#what-does-not-change}

`scan_videos` は今までどおり最新のスキャンに付くので、監視によるスキャンがキューに入れたジョブはそのスキャンの進み具合に数えられる。それを示すかどうかは画面が `origin` から決める。ユーザーのデータは今までどおり内容の鍵で引くので、削除の後に別のまとまりで同じファイルをコピーし直すと、2 回の手動のスキャンですでにそうなるように、タグと再生位置が戻る。
