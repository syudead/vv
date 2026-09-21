# Backend hot reload

## Summary

開発ツールのバージョン管理を各エコシステムの標準機構へ移し、`task dev` で Go
バックエンドもソース変更時に自動再ビルド・再起動されるようにする。

## Decisions

### ツールの正本を標準機構へ寄せる

Go 製ツールは `tools/go.mod` の `tool` directive でアプリ依存から分離し、`task setup` が
`.local/bin` へインストールする。開発ランタイムは `mise.toml` で管理し、独自の
`scripts/tool-versions.json` は削除する。TypeScript 7 と peer dependency が競合する
`openapi-typescript` は、生成時に `npm exec` で固定版を実行する。

### Air は既存の開発入口から起動する

`.air.toml` は `cmd/mdm` を `.local/air` にビルドし、`cmd` と `internal` の Go・SQL
変更を監視する。開発時の SPA は Vite が配信するため、Web ソースは Air で監視しない。
`scripts/dev.ps1` は Vite と Air を並行起動し、利用者の入口は引き続き `task dev` とする。

## Implementation Work

- [x] ツール依存を標準のマニフェストへ移す。
- [x] Air の監視設定を追加して `task dev` から起動する。
- [x] セットアップ、生成、lint、テスト、文書を更新する。
- [x] 静的検査と実際のバックエンド再起動を確認する。

## Verification

- `task check`: 成功（Go / Web の format、lint、test、生成物、migration、ローカル開発スクリプト）。
- `task setup`: 成功。途中で壊れた `node_modules` は Vite 実体の検査で再取得されることを確認。
- Air v1.67.4: `cmd/mdm/main.go` の更新を検知し、再ビルド後にテスト用ポート 18080 で再起動。
- `GET /api/health`: Air 起動中に `status: ok` を確認。
- `task dev`: Air と Vite の同時起動、Ctrl+C 後の Air・MDM・Vite 全プロセス終了を確認。
