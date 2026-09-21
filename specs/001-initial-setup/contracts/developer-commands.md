# 開発者向けコマンドの契約（Phase 0）

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md) | 対応する要件: FR-001／FR-011／FR-016

`task` のタスク名がそのまま開発者との契約になる。README と CI は必ずこのタスクを呼び、
同じ検査を手元と CI の両方で再現できる状態を保つ。

| 目標            | 前提                                    | 内容                                                                                                | 満たす要件               |
| --------------- | --------------------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------ |
| `task up`       | Task・Docker                            | イメージを構築してアプリケーションを起動する。**導入手順の最初に実行する唯一のコマンド**            | FR-001 / SC-001          |
| `task down`     | Task・Docker                            | 起動したものを停止・削除する                                                                        | —                        |
| `task dev`      | Task・Go・Node・ffmpeg                  | Go サーバーと Vite の開発サーバーを起動する（変更の即時反映用）                                     | —                        |
| `task build`    | Task・Go・Node                          | SPA をビルドして埋め込み、単一バイナリを生成する                                                    | —                        |
| `task generate` | Task・Go・Node                          | `api/openapi.yaml` から Go と TypeScript の型を生成する                                             | FR-012                   |
| `task fmt`      | Task・Go・Node・jq                      | 書式を整える                                                                                        | FR-011                   |
| `task lint`     | Task・Go・Node・jq                      | `golangci-lint`（depguard を含む）と Web の静的検査                                                 | FR-010 / FR-011 / FR-013 |
| `task test`     | Task・Go・Node                          | Go のテストと Web の単体テスト                                                                      | FR-011 / FR-014          |
| `task test-e2e` | Task・Go・Node・ffmpeg・Chromium        | GoサーバーとVite proxyを通る主要操作を実ブラウザで検証する                                          | FR-011 / FR-014          |
| `task check`    | Task・Go・Node・jq・Git・bash           | `fmt` の差分確認 → `lint` → `test` → 生成物の差分確認 → 適用済みマイグレーションの不変性 を順に実行 | FR-011 / SC-002          |

## 約束

- CIは`task check`と`task test-e2e`だけを実行し、手元でも同じタスクで再現できる。CIでしか動かない検査を作らない。
  CI のステップに検査を直接並べない。並べると目標の側へ足し忘れたときに判定が食い違う。
- `task check` は手元で 5 分以内に終わる（SC-002）。超えるようになったら、
  まず原因を直す。ブラウザなど追加runtimeが必要な検査は、前提を明記した専用目標に分ける。
- 失敗時は、どの規則に違反したかが出力だけで分かる（FR-013）。depguard の
  禁止ルールには理由の文言を必ず付ける。
- `task generate` の再実行で差分が出る状態は失敗とみなす。生成物は版管理に含め、
  手編集しない（`AGENTS.md`）。

## 依存ツールの導入

`task up` 以外のタスクはホストに Go・Node・ffmpeg を要求する。`task check` はさらに
jq・Git・bash を要求する。README には
`task up` だけを「必ず動く道」として示し、その他は開発者向けの補足として扱う。

開発者コマンドの実体は `scripts/` の Go プログラムに置く。Taskfile へ直接書くのは
外部コマンドを1つ呼ぶだけの目標に限る。検査のためだけに別のランタイム
（かつての PowerShell 7.4）を前提にしない。実体が Go なので、その振る舞いは
`task test` が `go test ./...` として検証する。
