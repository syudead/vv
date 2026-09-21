# vv

A self-hosted media data management (MDM) system: it indexes video files on
local storage and plays them back in a browser. A single Go binary serves the
JSON API and the embedded React SPA, backed by SQLite. Everything ships as one
container.

See [ARCHITECTURE.md](ARCHITECTURE.md) for system boundaries and
[docs/design-docs/](docs/design-docs/) for the decisions behind the stack.

## Getting started

必要なのは **Task と Docker** で、最初に実行するコマンドは1つだけである。

```bash
git clone <repository-url> && cd vv
task up
```

`http://localhost:8080` を開くと動画の一覧が表示される。機械可読な稼働確認は次の
とおり。

```bash
curl -sS http://localhost:8080/api/health
# {"status":"ok","version":"dev"}
```

停止は `task down`。

## 動画を並べる

動画を置いた場所を読ませるには `MDM_MEDIA_HOST_DIR` を渡す（既定は `./media`）。

```bash
MDM_MEDIA_HOST_DIR=/path/to/videos task up
```

起動後に設定画面でメディアフォルダを登録し、画面の「取り込む」を押すか
`curl -X POST localhost:8080/api/scans` を実行すると、静止画・題名・長さ付きの一覧ができる。
あとから動画を足した場合も同じ手動操作で反映する。取り込み中も一覧と再生は普通に使える。

- 動画ファイルは**読み取りしかしない**。変更・移動・削除・変換は行わない
- 移動・改名しても同じ動画として扱われ、再生位置も引き継がれる
- ブラウザで再生できない形式（`mkv` など）も一覧には並び、再生を試みる前に
  その旨と理由が表示される
- 題名は1文字から検索できる

データの置き場所は2つに分かれる。

| 対象                             | 場所                                     | 消したら               |
| -------------------------------- | ---------------------------------------- | ---------------------- |
| 索引（`videos`・サムネイルなど） | `MDM_DATA_DIR`（Docker では `vv_data`）  | 再スキャンで作り直せる |
| 再生位置（利用者データ）         | 同じデータベース内の `playback_progress` | **作り直せない**       |

`task down && docker volume rm vv_data` のあとでも、スキーマは次の起動で作り直され
一覧は再スキャンで復元する。失われるのは再生位置だけである（バックアップ手順の
整備は Phase 3 の範囲）。

起動設定は環境変数、メディアフォルダは設定画面で管理する。取り込みは起動時に自動実行せず、
利用者が明示的に開始する。項目の一覧は
[specs/001-initial-setup/contracts/configuration.md](specs/001-initial-setup/contracts/configuration.md)
と[本機能での差分](specs/002-core-video-library/contracts/configuration.md)にある。

> [!WARNING]
> 現時点では認証を掛けていない。インターネットへの公開を前提にしないこと
> （認証は Phase 3 の範囲）。信頼できるネットワークの内側だけで使う。

## Developer commands

`task up` 以外のタスクはホストに Go・Node・`ffmpeg` を要求する。「必ず動く道」は
`task up` だけで、以下は開発者向けの補足である。タスクの契約は
[specs/001-initial-setup/contracts/developer-commands.md](specs/001-initial-setup/contracts/developer-commands.md)。

### ローカル開発環境

Go・Node・task・jq のバージョンは `mise.toml` に固定している。`mise` を使う場合は
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
検査ツールの版は `scripts/tool-versions.json` と `mise.toml` に固定する。
開発者コマンドの実体は `scripts/` の Go プログラムで、`task check` の一部として
`go test ./...` が検証する。追加のシェルやランタイムは要らない。
Windows で `task setup` を再実行するときは、先に開発サーバーを停止する。
起動中はネイティブ依存のファイルがロックされ、npm ci が失敗するためである。
Go と Web のソースは `.gitattributes` で LF に固定し、Windows の改行変換による
整形エラーを防ぐ。既存のチェックアウトで CRLF が残っている場合は、
`mise exec --command "task fmt"` で整形する。

`Taskfile.yml` が開発者コマンドの唯一の入口である。`mise activate` 済みの shell では
`task setup` のように直接呼べる。

| 入口                               | 内容                                                         |
| ---------------------------------- | ------------------------------------------------------------ |
| `task doctor` / `scripts/doctor`   | Go・Node・ffmpeg・bash・Docker などの有無を確認する          |
| `task setup`                       | Go module、npm 依存、lint ツール、ビルドキャッシュを準備する |
| `task dev` / `scripts/dev`         | Go サーバーと Vite 開発サーバーを同時に起動する              |
| `task check`                       | 静的検査、テスト、生成物とマイグレーションをまとめて検証する |

`ffmpeg` / `ffprobe`、Docker、bash は OS 側のツールであり、`mise.toml`
だけでは完結しない。足りないものは `task doctor` の出力に従って導入する。
Docker はコンテナ起動用の任意ツールである。
Windows で Go バイナリを直接動かす場合、`MDM_DATA_DIR` はドライブ名を含む絶対パスにする。
メディアフォルダは起動後に設定画面で選択する。`task dev` はデータ用の絶対パスを自動設定する。

| 目標                    | 内容                                                                          |
| ----------------------- | ----------------------------------------------------------------------------- |
| `task setup`            | 依存と開発ツールを先に取得する                                                |
| `task up` / `task down` | Docker で起動・停止する                                                       |
| `task dev`              | Go サーバーと Vite 開発サーバーを起動する                                     |
| `task build`            | SPA をビルドして埋め込み、`bin/mdm` を生成する                                |
| `task generate`         | `api/openapi.yaml` から Go と TypeScript の型を生成する                       |
| `task fmt`              | 書式を整える                                                                  |
| `task lint`             | `golangci-lint`（depguard を含む）と Web の型検査                             |
| `task test`             | Go のテストと Web のビルド検証                                                |
| `task test-e2e`         | Go・Vite・Chromiumを起動し、主要な画面操作を実ブラウザで検証する              |
| `task check`            | 静的検査、単体テスト、生成物、マイグレーションをまとめて検証する               |

CI はこの表の `task check` と `task test-e2e` だけを実行する。CI 側に検査を並べず、
目標そのものを呼ぶことで、手元と CI の判定を一致させている。

`api/openapi.yaml` が Go と TypeScript の境界の唯一の真実である。型を変えるときは
このファイルを直して `task generate` を実行する。生成物
（`internal/httpapi/gen/`、`web/src/api/gen/`）は手編集しない。

Claude Code on the web でセッションを開くと、`.claude/hooks/session-start.sh` が
`task setup` を呼んで依存と `golangci-lint` を先に用意する。手元の CLI では何もしない。

## Repository structure

```text
.
├── AGENTS.md
├── ARCHITECTURE.md
├── Dockerfile              # multi-stage（web ビルド → go ビルド → alpine + ffmpeg）
├── Taskfile.yml            # 開発者向けコマンドの入口
├── .claude/
│   ├── hooks/              # Claude Code の SessionStart フック（依存の先出し）
│   └── settings.json
├── compose.yaml            # task up の実体
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
├── scripts/                # 開発者コマンドの実体（Go）と固定した検査ツールの版
├── docs/
│   ├── design-docs/
│   ├── exec-plans/
│   ├── generated/
│   ├── product-specs/
│   └── references/
└── specs/                  # 機能ごとの仕様・計画・契約
```

実行時に増える場所（版管理しない）:

| 場所                       | 内容                                          |
| -------------------------- | --------------------------------------------- |
| `MDM_DATA_DIR/mdm.db`      | SQLite のデータベース                         |
| `MDM_DATA_DIR/thumbnails/` | サムネイル（`<先頭2文字>/<content_key>.jpg`） |

Directory-level README files are included so that intentionally empty
directories remain visible in Git and explain what belongs in each location.
