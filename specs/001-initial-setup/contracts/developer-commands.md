# 開発者向けコマンドの契約（Phase 0）

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md) | 対応する要件: FR-001／FR-011／FR-016

`make` の目標名がそのまま開発者との契約になる。README と CI は必ずこの目標を呼び、
同じ検査を手元と CI の両方で再現できる状態を保つ。

| 目標 | 前提 | 内容 | 満たす要件 |
| --- | --- | --- | --- |
| `make up` | Docker のみ | イメージを構築してアプリケーションを起動する。**導入手順の最初に実行する唯一のコマンド** | FR-001 / SC-001 |
| `make down` | Docker のみ | 起動したものを停止・削除する | — |
| `make dev` | Go・Node・ffmpeg | Go サーバーと Vite の開発サーバーを起動する（変更の即時反映用） | — |
| `make build` | Go・Node | SPA をビルドして埋め込み、単一バイナリを生成する | — |
| `make generate` | Go・Node | `api/openapi.yaml` から Go と TypeScript の型を生成する | FR-012 |
| `make fmt` | Go・Node | 書式を整える | FR-011 |
| `make lint` | Go・Node | `golangci-lint`（depguard を含む）と Web の静的検査 | FR-010 / FR-011 / FR-013 |
| `make test` | Go・Node | Go のテストと Web の単体テスト | FR-011 / FR-014 |
| `make check` | Go・Node | `fmt` の差分確認 → `lint` → `test` → 生成物の差分確認 を順に実行 | FR-011 / SC-002 |

## 約束

- `make check` は**手元と CI で同じ判定**になる。CI でしか動かない検査を作らない。
- `make check` は手元で 5 分以内に終わる（SC-002）。超えるようになったら、
  遅い検査を切り出して別目標にするのではなく、まず原因を直す。
- 失敗時は、どの規則に違反したかが出力だけで分かる（FR-013）。depguard の
  禁止ルールには理由の文言を必ず付ける。
- `make generate` の再実行で差分が出る状態は失敗とみなす。生成物は版管理に含め、
  手編集しない（`AGENTS.md`）。

## 依存ツールの導入

`make up` 以外の目標はホストに Go・Node・ffmpeg を要求する。README には
`make up` だけを「必ず動く道」として示し、その他は開発者向けの補足として扱う。
