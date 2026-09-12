# vv

A self-hosted media data management (MDM) system: it indexes video files on
local storage and plays them back in a browser. A single Go binary serves the
JSON API and the embedded React SPA, backed by SQLite. Everything ships as one
container.

See [ARCHITECTURE.md](ARCHITECTURE.md) for system boundaries and
[docs/design-docs/](docs/design-docs/) for the decisions behind the stack.

## Getting started

必要なのは **Docker だけ**で、最初に実行するコマンドも1つだけである。

```bash
git clone <repository-url> && cd vv
make up
```

`http://localhost:8080` を開くと稼働状態が表示される。機械可読な稼働確認は次の
とおり。

```bash
curl -sS http://localhost:8080/api/health
# {"status":"ok","version":"dev"}
```

停止は `make down`。データベースは名前付きボリューム `vv_data` に置かれ、消しても
次の起動でスキーマが作り直される。

動画を置いた場所を読ませるには `MDM_MEDIA_HOST_DIR` を渡す（既定は `./media`）。

```bash
MDM_MEDIA_HOST_DIR=/path/to/videos make up
```

設定はすべて環境変数で与え、既定値だけで起動できる。項目の一覧は
[specs/001-initial-setup/contracts/configuration.md](specs/001-initial-setup/contracts/configuration.md)
にある。

> [!WARNING]
> 現時点では認証を掛けていない。インターネットへの公開を前提にしないこと
> （認証は Phase 3 の範囲）。信頼できるネットワークの内側だけで使う。

## Developer commands

`make up` 以外の目標はホストに Go・Node・`ffmpeg` を要求する。「必ず動く道」は
`make up` だけで、以下は開発者向けの補足である。目標の契約は
[specs/001-initial-setup/contracts/developer-commands.md](specs/001-initial-setup/contracts/developer-commands.md)。

| 目標 | 内容 |
| --- | --- |
| `make up` / `make down` | Docker で起動・停止する |
| `make dev` | Go サーバーと Vite 開発サーバーを起動する |
| `make build` | SPA をビルドして埋め込み、`bin/mdm` を生成する |
| `make generate` | `api/openapi.yaml` から Go と TypeScript の型を生成する |
| `make fmt` | 書式を整える |
| `make lint` | `golangci-lint`（depguard を含む）と Web の型検査 |
| `make test` | Go のテストと Web のビルド検証 |
| `make check` | 上記をまとめて実行する。CI と同じ判定になる |

`api/openapi.yaml` が Go と TypeScript の境界の唯一の真実である。型を変えるときは
このファイルを直して `make generate` を実行する。生成物
（`internal/httpapi/gen/`、`web/src/api/gen/`）は手編集しない。

## Repository structure

```text
.
├── AGENTS.md
├── ARCHITECTURE.md
├── Dockerfile              # multi-stage（web ビルド → go ビルド → alpine + ffmpeg）
├── Makefile                # 開発者向けコマンドの入口
├── compose.yaml            # make up の実体
├── api/
│   └── openapi.yaml        # API 契約（Go/TS 双方の生成元、唯一の真実）
├── cmd/
│   └── mdm/                # 設定の読み込み、依存の組み立て、起動と停止
├── internal/
│   ├── domain/             # ドメインモデルとユースケース（外部 I/O 依存なし）
│   ├── httpapi/            # ルーティング、ハンドラ、SPA 配信
│   ├── store/              # SQLite 接続、マイグレーション、FTS5 実証
│   ├── media/              # 外部ツール（ffmpeg／ffprobe）のアダプタ
│   ├── scanner/            # ファイル走査（Phase 0 は境界の宣言のみ）
│   └── jobs/               # ジョブキュー（Phase 0 は境界の宣言のみ）
├── web/                    # React SPA（ビルド結果を Go バイナリへ埋め込む）
├── docs/
│   ├── design-docs/
│   ├── exec-plans/
│   ├── generated/
│   ├── product-specs/
│   └── references/
└── specs/                  # 機能ごとの仕様・計画・契約
```

Directory-level README files are included so that intentionally empty
directories remain visible in Git and explain what belongs in each location.
