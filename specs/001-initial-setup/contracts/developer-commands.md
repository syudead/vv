# 開発者向けコマンドの契約（Phase 0）

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md) | 対応する要件: FR-001／FR-011／FR-016

`task` のタスク名がそのまま開発者との契約になる。README と CI は必ずこのタスクを呼び、
同じ検査を手元と CI の両方で再現できる状態を保つ。

| 目標            | 前提                                    | 内容                                                                                                | 満たす要件               |
| --------------- | --------------------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------ |
| `task up`       | Task・Docker                            | イメージを構築してアプリケーションを起動する。**導入手順の最初に実行する唯一のコマンド**            | FR-001 / SC-001          |
| `task down`     | Task・Docker                            | 起動したものを停止・削除する                                                                        | —                        |
| `task doctor`   | Task・Go                                | 手元に必要な外部コマンドが揃っているかを調べる。必須が欠けていれば失敗する                          | —                        |
| `task setup`    | Task・Go・Node                         | Go module、npm 依存、検査ツール、ビルドキャッシュを先に用意する。npm 依存は毎回入れ直す             | —                        |
| `task dev`      | Task・Go・Node・ffmpeg                 | Go サーバーを Air で自動再起動し、Vite の開発サーバーとともに起動する。Vite の API 転送先は `MDM_API_TARGET`、未指定なら `MDM_ADDR` から導出する | —                        |
| `task build`    | Task・Go・Node                          | SPA をビルドして埋め込み、単一バイナリを生成する                                                    | —                        |
| `task generate` | Task・Go・Node                          | `api/openapi.yaml` から Go と TypeScript の型を生成する                                             | FR-012                   |
| `task fmt`      | Task・Go・Node                         | 書式を整える                                                                                        | FR-011                   |
| `task lint`     | Task・Go・Node                         | `golangci-lint`（depguard を含む）と Web の静的検査                                                 | FR-010 / FR-011 / FR-013 |
| `task test`     | Task・Go・Node                          | Go のテストと Web の単体テスト                                                                      | FR-011 / FR-014          |
| `task test-e2e` | Task・Go・Node・ffmpeg・Chromium        | GoサーバーとVite proxyを通る主要操作を実ブラウザで検証する                                          | FR-011 / FR-014          |
| `task check`    | Task・Go・Node・Git・bash              | `fmt` の差分確認 → `lint` → `test` → 生成物の差分確認 → 適用済みマイグレーションの不変性 → Windows 向けビルドの成立 を順に実行 | FR-011 / SC-002          |

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
Git・bash を要求する。README には
`task up` だけを「必ず動く道」として示し、その他は開発者向けの補足として扱う。

判断や後始末を伴う処理は `scripts/` の Go プログラムに置く。Taskfile へ直接書くのは、
外部コマンドを順に並べるだけで済む目標に限る。検査のためだけに別のランタイム
（かつての PowerShell 7.4）を前提にしない。

Go に置くと、その中身を `task test` の `go test ./...` が検証できる。現に検証して
いるのは各プログラムの判断部分である —— `doctor` の合否と終了コード、`generate` の
生成物の差分判定と失敗時の打ち切り、`build` の `web/dist` の退避と復元、`dev` の
出力の行送りと API 転送先の導出。`dev` のプロセス終了だけは実プロセスを使い、起動元が
先に終了した場合も子プロセスが残らないことを Unix と Windows で検証する。その他の
外部コマンドを実際に起動する経路は `task check` 自身が通ることで確かめる。
