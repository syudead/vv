# 006 Local Development Environment

## Goal

2026-09-17: 起動と全検査が未確認だったため再開し、下記の検証を完了した。

ローカル開発を始めるまでの摩擦を下げる。既存の `Makefile` 契約は維持しつつ、
Windows PowerShell でも同じ入口を使えるようにする。

## Scope

- `mise.toml` で Go / Node / task のバージョンを固定する。
- PowerShell 用の `setup` / `doctor` / `dev` / `check` スクリプトを追加する。
- `Taskfile.yml` を薄い入口として追加する。
- README の開発手順を更新する。

## Out of scope

- `Makefile` の置き換え。
- Docker / ffmpeg / GNU make の自動インストール。
- CI 構成の変更。

## Validation

- `scripts/doctor.ps1` が不足ツールを分かる形で報告する。
- `scripts/check.ps1` が `make check` を優先し、`make` がない環境では同等の順序で検査する。
- `Taskfile.yml` の入口が既存コマンドと PowerShell スクリプトを呼ぶ。

## Progress

- [X] `mise.toml` を追加する。
- [X] PowerShell スクリプトを追加する。
- [X] `Taskfile.yml` を追加する。
- [X] README を更新する。
- [X] 可能な範囲で検証する。

## Results

- `mise trust mise.toml` と `mise install` を実行し、Node 22.23.0、task 3.45.4、
  Go 1.26.0 を mise 管理下に入れた。
- `mise exec --command "task doctor"` は成功した。Go / Node / npm / ffmpeg / ffprobe /
  bash / mise / task は検出でき、任意ツールの `make` と Docker は未導入として警告された。
- `mise exec --command "task setup"` は成功した。Go module、npm 依存、golangci-lint、
  Go build cache の準備が完了した。
- PowerShell スクリプト 4 本は parser で構文確認済み。
- 初回の `task check` 失敗は既存コードの書式不備ではなく、Windows の
  `core.autocrlf=true` による CRLF 変換だった。Go と Web を `.gitattributes` で
  LF に固定し、作業ファイルを正規化した。ソースの大量変更はコミットに含まれない。
- PowerShell 7.4 以上を必須とし、setup / check が外部コマンドの非ゼロ終了を
  検出するよう修正した。4 件の失敗注入テストを `task check` の先頭で実行する。
- Linux のパスを前提としていた設定・サムネイルのテストを OS に合わせた。
  コンテナ用の既定パスは変更せず、Windows で絶対パスとして拒否されることも検証する。
  scanner の権限テストが skip 前に開いたファイルを閉じるよう修正した。
- `task setup`、`task doctor`、`task check` 成功。
  Go 全パッケージ、Go lint、Web 型検査、ビルド、16 ファイル / 156 テスト、
  SDD 15 テスト、OpenAPI 生成物の一致を確認。
- 当初は chmod の権限テストと jq が必要な SDD ガード10ケースを省略していた。
  [追加修正](007-local-check-coverage.md)で jq を必須化し、OS に依存しない
  読み取り失敗の回帰テストを追加した。実権限の補助テストのみ環境により skip する。
  Docker は未導入で未検証。
- `task dev` の Go / Vite 起動、ブラウザーの一覧とサムネイル表示、
  8080 / 5173 両方の `/api/health` が status=ok を返すことを確認。
  Ctrl+C 後に両ポートが解放されることも確認した。
- 不正な `MDM_ADDR` で起動に失敗すると非ゼロ終了し、Vite も停止することを確認。
  Vite は strictPort で起動し、使用中のポートを黙って変更しない。
  子ジョブのログは UTF-8 で読み取り、日本語の文字化けを防ぐ。
- UI 変更なし。

## Notes

- 既存の `Makefile` は README / CI の契約なので維持した。
- `Taskfile.yml` は doctor / setup / dev / check に加え、up / down を提供する。
  up / down は Docker Compose を直接呼び、GNU make を要求しない。
- この環境の `mise registry` では task の tool 名は `task`。
- `mise exec --command "..."` は PowerShell でも安定して動くため、README の初回手順は
  これに寄せた。
