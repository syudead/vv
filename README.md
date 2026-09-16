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

`http://localhost:8080` を開くと動画の一覧が表示される。機械可読な稼働確認は次の
とおり。

```bash
curl -sS http://localhost:8080/api/health
# {"status":"ok","version":"dev"}
```

停止は `make down`。

## 動画を並べる

動画を置いた場所を読ませるには `MDM_MEDIA_HOST_DIR` を渡す（既定は `./media`）。

```bash
MDM_MEDIA_HOST_DIR=/path/to/videos make up
```

**置くだけで並ぶ。** 手作業の登録は要らない。起動直後に取り込みが1回走り、
静止画・題名・長さ付きの一覧ができる。あとから動画を足したときは、画面の
「取り込む」を押すか `curl -X POST localhost:8080/api/scans` を叩けば追加分だけが
増える。取り込み中も一覧と再生は普通に使える。

- 動画ファイルは**読み取りしかしない**。変更・移動・削除・変換は行わない
- 移動・改名しても同じ動画として扱われ、再生位置も引き継がれる
- ブラウザで再生できない形式（`mkv` など）も一覧には並び、再生を試みる前に
  その旨と理由が表示される
- 題名は1文字から検索できる

データの置き場所は2つに分かれる。

| 対象 | 場所 | 消したら |
| --- | --- | --- |
| 索引（`videos`・サムネイルなど） | `MDM_DATA_DIR`（Docker では `vv_data`） | 再スキャンで作り直せる |
| 再生位置（利用者データ） | 同じデータベース内の `playback_progress` | **作り直せない** |

`make down && docker volume rm vv_data` のあとでも、スキーマは次の起動で作り直され
一覧は再スキャンで復元する。失われるのは再生位置だけである（バックアップ手順の
整備は Phase 3 の範囲）。

設定はすべて環境変数で与え、既定値だけで起動できる。起動直後の自動取り込みを
止めたい場合は `MDM_SCAN_ON_START=false` を渡す。項目の一覧は
[specs/001-initial-setup/contracts/configuration.md](specs/001-initial-setup/contracts/configuration.md)
と[本機能での差分](specs/002-core-video-library/contracts/configuration.md)にある。

> [!WARNING]
> 現時点では認証を掛けていない。インターネットへの公開を前提にしないこと
> （認証は Phase 3 の範囲）。信頼できるネットワークの内側だけで使う。

## Developer commands

`make up` 以外の目標はホストに Go・Node・`ffmpeg` を要求する。「必ず動く道」は
`make up` だけで、以下は開発者向けの補足である。目標の契約は
[specs/001-initial-setup/contracts/developer-commands.md](specs/001-initial-setup/contracts/developer-commands.md)。

### ローカル開発環境

Go・Node・task のバージョンは `mise.toml` に固定している。task の実行には
PowerShell 7.4 以上（`pwsh`）が必要である。`mise` を使う場合は
最初に次を実行する。

```bash
mise trust
mise install
mise exec --command "task setup"
mise exec --command "task doctor"
```

開発サーバーは `mise exec --command "task dev"` で起動し、
`http://localhost:5173` を開く。終了は Ctrl+C。
変更の検証は `mise exec --command "task check"` で実行する。
Go と Web のソースは `.gitattributes` で LF に固定し、Windows の改行変換による
整形エラーを防ぐ。既存のチェックアウトで CRLF が残っている場合は、
`mise exec --command "gofmt -w cmd internal web/embed.go"` と
`mise exec --command "npm --prefix web run format"` で整形する。

`task` は `Makefile` の置き換えではなく、Windows / PowerShell でも同じ入口を
使うための薄いラッパーである。`make` が使える環境では従来どおり `make check` や
`make dev` を使ってよい。`mise activate` 済みの shell では `task setup` のように
直接呼べる。

| 入口 | 内容 |
| --- | --- |
| `task doctor` / `scripts/doctor.ps1` | Go・Node・ffmpeg・bash・Docker などの有無を確認する |
| `task setup` / `scripts/setup.ps1` | Go module、npm 依存、lint ツール、ビルドキャッシュを準備する |
| `task dev` / `scripts/dev.ps1` | Go サーバーと Vite 開発サーバーを PowerShell で起動する |
| `task check` / `scripts/check.ps1` | `make check` を優先し、`make` が無ければ同等の順序で検査する |

`ffmpeg` / `ffprobe`、Docker、GNU make、bash は OS 側のツールであり、`mise.toml`
だけでは完結しない。足りないものは `task doctor` の出力に従って導入する。
Docker はコンテナ起動用の任意ツールで、`task up` / `task down` は make 不要。
Windows で Go バイナリを直接動かす場合、`MDM_MEDIA_DIR` / `MDM_DATA_DIR` は
ドライブ名を含む絶対パスにする。`task dev` はリポジトリ内の絶対パスを自動設定する。

| 目標 | 内容 |
| --- | --- |
| `make setup` | 依存と開発ツールを先に取得する |
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

Claude Code on the web でセッションを開くと、`.claude/hooks/session-start.sh` が
`make setup` を呼んで依存と `golangci-lint` を先に用意する。手元の CLI では何もしない。

## Repository structure

```text
.
├── AGENTS.md
├── ARCHITECTURE.md
├── Dockerfile              # multi-stage（web ビルド → go ビルド → alpine + ffmpeg）
├── Makefile                # 開発者向けコマンドの入口
├── .claude/
│   ├── hooks/              # Claude Code の SessionStart フック（依存の先出し）
│   └── settings.json
├── compose.yaml            # make up の実体
├── api/
│   └── openapi.yaml        # API 契約（Go/TS 双方の生成元、唯一の真実）
├── cmd/
│   └── mdm/                # 設定の読み込み、依存の組み立て、起動と停止
├── internal/
│   ├── domain/             # ドメインモデルとユースケース（外部 I/O 依存なし）
│   ├── httpapi/            # ルーティング、ハンドラ、SPA 配信
│   ├── store/              # SQLite 接続、マイグレーション、問い合わせと検索
│   ├── media/              # 外部ツール（ffmpeg／ffprobe）のアダプタ
│   ├── scanner/            # ファイル走査、内容由来の識別子、移動の検出
│   └── jobs/               # プロセス内のジョブワーカー（直列）
├── web/
│   └── src/
│       ├── api/            # 生成型を使う fetch ラッパと一覧のフック
│       ├── components/     # 一覧の1件、取り込みの進捗
│       └── pages/          # 一覧（/）と再生（/videos/:id）
├── docs/
│   ├── design-docs/
│   ├── exec-plans/
│   ├── generated/
│   ├── product-specs/
│   └── references/
└── specs/                  # 機能ごとの仕様・計画・契約
```

実行時に増える場所（版管理しない）:

| 場所 | 内容 |
| --- | --- |
| `MDM_DATA_DIR/mdm.db` | SQLite のデータベース |
| `MDM_DATA_DIR/thumbnails/` | サムネイル（`<先頭2文字>/<content_key>.jpg`） |

Directory-level README files are included so that intentionally empty
directories remain visible in Git and explain what belongs in each location.
