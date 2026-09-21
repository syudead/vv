# Backend hot reload

## Summary

開発ツールのバージョン管理を各エコシステムの標準機構へ移し、`task dev` で Go
バックエンドもソース変更時に自動再ビルド・再起動されるようにする。

## Decisions

### ツールの正本を標準機構へ寄せる

Go 製ツールは `tools/go.mod` の `tool` directive でアプリ依存から分離し、`go tool` で
直接実行する。開発ランタイムは `mise.toml` で管理し、独自の
`scripts/tool-versions.json` は削除する。TypeScript 7 と peer dependency が競合する
`openapi-typescript` は、分離した `tools/package.json` で固定する。

### Air は既存の開発入口から起動する

`.air.toml` は `cmd/mdm` を `.local/air` にビルドし、`cmd`、`internal`、`web` の Go・SQL
変更を監視する。Web の TypeScript や CSS は引き続き Vite が監視する。
`scripts/dev` の Go プログラムは Vite と Air を並行起動し、利用者の入口は引き続き
`task dev` とする。

## Implementation Work

- [x] ツール依存を標準のマニフェストへ移す。
- [x] Air の監視設定を追加して `task dev` から起動する。
- [x] セットアップ、生成、lint、テスト、文書を更新する。
- [x] 静的検査と実際のバックエンド再起動を確認する。

## Verification

- `task check`: 成功（Go / Web の format、lint、test、生成物、migration、ローカル開発スクリプト）。
- Air v1.67.4: `web/embed.go` の更新を検知し、再ビルド後にテスト用ポート 18081 で再起動。
- `GET /api/health`: Air 起動中に `status: ok` を確認。
- `scripts/dev`: Vite の起動失敗後に Air・MDM が残らないことを確認。
- `go test ./scripts/dev`: 起動元が即座に子プロセスを生成して先に終了しても、子を停止する実プロセステストに成功。
- `go test ./scripts/dev`: `MDM_ADDR` のワイルドカード・IPv4・IPv6 から Vite の API 転送先を導出するテストに成功。
