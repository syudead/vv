# Data Model: 失敗理由のコード

この feature が足すのは、解析と取り込みの失敗理由のコードを入れる 3 列だけである。既存の列、
特に自由文の `videos.probe_error`・`scans.error`・`jobs.last_error` は形も既存の値も変えない
（要件 8）。どちらの表も作り直せるデータ（[ARCHITECTURE.md「Rebuildable and user data」](../../ARCHITECTURE.md#rebuildable-and-user-data)）
で、区分は変わらない。

移行は `internal/store/migrations/00016_failure_codes.sql` の 1 本で、3 列を `null` 可の
`text` として足すだけである。既存の行は `null`（コードの無いアップグレード前の失敗）のまま残し、
過去の自由文を解析してコードを埋めない（[research.md R-6](research.md#r-6-失敗理由は機械可読なコードを保存し画面は自由文を出さない)）。

コードの値は `internal/domain` の定数と、`api/openapi.yaml` の enum
（[contracts/error-api.md §2・§3](contracts/error-api.md)）で同じ綴りにする。コードを付けて失敗を
包む型も `internal/domain` に置き、`errors.As` で取り出す。分類できない失敗は `internal` である。

## 1. `videos.probe_error_code`

`probe_state` を `failed` にするとき、`probe_error`（英語の自由文）と一緒に書く。`probe_error` を
`null` に戻す箇所（解析の成功、再解析の開始）では一緒に `null` に戻す。

| コード | 状況（失敗を作る箇所） |
| --- | --- |
| `file_unavailable` | 解析の前後でファイルを確かめられない、通常ファイルでない（`internal/media/probe.go`・`assets.go`） |
| `probe_unavailable` | `ffprobe` を起動できない（`internal/media/probe.go`） |
| `probe_failed` | `ffprobe` が失敗した。壊れた・対応しないファイル、時間切れを含む（同上） |
| `invalid_metadata` | `ffprobe` の出力を解釈できない、尺が読めない・不正（同上） |
| `internal` | それ以外（`internal/app`・`internal/store` の失敗、未知のジョブの種類など） |

`jobs.last_error` は API に出ないので、コードの列を足さない。文は英語にする。

## 2. `scans.error_code` と `scans.error_path`

`state` を `failed` にするとき（`FinishScan`、起動時の `FailInterruptedScans`）、`error`（英語の
自由文）と一緒に書く。`error_path` は理由が特定の場所に結び付くときだけ入れ、それ以外は `null`。

| コード | `error_path` | 状況（失敗を作る箇所） |
| --- | --- | --- |
| `media_folder_unreadable` | メディアフォルダ | メディアフォルダを読めない（`internal/scanner` の走査の開始、取り込み後の確認） |
| `media_folder_not_directory` | メディアフォルダ | メディアフォルダがディレクトリでない、シンボリックリンクである（同上） |
| `location_unreadable` | 読めなかった場所 | 走査の途中で読めない場所があった、取り込み後に親ディレクトリを確かめられない（同上） |
| `interrupted` | — | 停止の指示で打ち切った、取り込みの途中でアプリケーションが止まった（`internal/app/scans.go`・`internal/store/scans.go` の `FailInterruptedScans`） |
| `internal` | — | それ以外（進捗を記録できない、処理の panic など） |
