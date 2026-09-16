# 006 Local Development Environment

## Goal

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
- `mise exec --command "task check"` は入口として起動したが、最初の `fmt-check-go` で
  既存 Go ファイルが大量に `gofmt` 差分ありとして失敗した。今回の目的外なので
  自動整形は行っていない。

## Notes

- 既存の `Makefile` は README / CI の契約なので維持した。
- `Taskfile.yml` は `task doctor` / `task setup` / `task dev` / `task check` の入口だけを
  提供し、実体は PowerShell スクリプトまたは既存 `make` に寄せた。
- この環境の `mise registry` では task の tool 名は `task`。
- `mise exec --command "..."` は PowerShell でも安定して動くため、README の初回手順は
  これに寄せた。

