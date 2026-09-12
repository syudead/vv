# Tasks: 初期セットアップ（Phase 0 骨組み）

**Input**: Design documents from `/specs/001-initial-setup/`

**Prerequisites**: [plan.md](./plan.md)（必須）、[spec.md](./spec.md)（ユーザーストーリー）、[research.md](./research.md)、[data-model.md](./data-model.md)、[contracts/](./contracts/)

**Tests**: 本機能はテストを**含む**。spec が FR-010／FR-011／FR-014 で自動検証そのものを要件として求めており、US3 はテストの存在が成果物であるため。

**Organization**: タスクはユーザーストーリー単位にまとめてある。各ストーリーは独立して実装・検証でき、そこで止めても価値が残る。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 並行実行可（別ファイル・依存なし）
- **[Story]**: 対応するユーザーストーリー（US1 / US2 / US3）
- 説明には必ず対象ファイルのパスを書く

## Path Conventions

本機能は **web-service**（単一 Go バイナリ + 埋め込み React SPA）。パスはすべて
リポジトリ root からの相対で、[plan.md](./plan.md) の "Source Code" の配置に従う。

- Go: `cmd/mdm/`、`internal/{domain,httpapi,store,media,scanner,jobs}/`
- 契約: `api/openapi.yaml`（Go／TS 双方の生成元、唯一の真実）
- Web: `web/`
- モジュールパス: `github.com/syudead/vv`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: リポジトリに Go／Web のプロジェクトとしての体裁と、6 つのモジュール境界を置く

- [ ] T001 `go.mod` を作成する。モジュールパスは `github.com/syudead/vv`、`go` ディレクティブは `1.26.0`。`toolchain` 行は書かない（[R-002](./research.md)）
- [ ] T002 [P] `.gitignore` を更新する。既存の `dist/` 行が embed 用プレースホルダを除外してしまうため `!web/dist/` と `!web/dist/index.html` の否定パターンを追加し、あわせて `/bin/` を無視対象に加える（[R-008](./research.md)）
- [ ] T003 [P] 6 つのモジュール境界を `doc.go` だけで宣言する（FR-009／SC-005）: `internal/domain/doc.go`、`internal/httpapi/doc.go`、`internal/store/doc.go`、`internal/media/doc.go`、`internal/scanner/doc.go`、`internal/jobs/doc.go`。各ファイルにパッケージの責務と、`cmd → internal/{httpapi,store,media,scanner,jobs} → internal/domain` の一方向の依存だけが許されることを書く（`ARCHITECTURE.md`）
- [ ] T004 [P] Web プロジェクトを用意する: `web/package.json`（React 19 / Vite 8 / TypeScript、`engines` に Node 22）、`web/tsconfig.json`、`web/vite.config.ts`（`build.outDir` は `dist`）、`web/tailwind.config.ts`、`web/index.html`、`web/src/main.tsx`、`web/src/index.css`（Tailwind CSS 4 の読み込み）。`App.tsx` は US1 で作るのでここでは作らない
- [ ] T005 [P] embed 対象のプレースホルダを版管理に置く: `web/dist/.gitkeep` と最小の `web/dist/index.html`。`web` をビルドしていない状態でも `go build` が通る状態にする（[R-008](./research.md)）
- [ ] T006 [P] `api/openapi.yaml` を作成する。内容は [contracts/openapi.yaml](./contracts/openapi.yaml) をそのまま置き、以後はこのファイルを唯一の真実として扱う（[R-010](./research.md)）
- [ ] T007 `Makefile` を作成し、[contracts/developer-commands.md](./contracts/developer-commands.md) の 9 目標（`up` / `down` / `dev` / `build` / `generate` / `fmt` / `lint` / `test` / `check`）をすべて宣言する。この時点では中身が未実装の目標があってよい（`up`/`down`/`dev`/`build` は T027、`fmt`/`lint`/`test`/`check` は T030、`generate` は T008 で埋める）

**Checkpoint**: `go build ./...` が通り、6 パッケージと web プロジェクトがリポジトリ上に存在する

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: どのユーザーストーリーからも使われる契約生成と保存層の基礎。ここが無いと US1 の起動経路も US3 の実証テストも書けない

**⚠️ CRITICAL**: このフェーズが完了するまで、ユーザーストーリーの作業は始められない

- [ ] T008 コード生成を配線する: `api/oapi-codegen.yaml`（出力 `internal/httpapi/gen/api.gen.go`、`oapi-codegen` v2.8.0）と、`openapi-typescript` v7.13.0 による `web/src/api/gen/openapi.ts` の生成を `Makefile` の `generate` 目標に実装する。生成器の版はコマンド行で固定する（[R-010](./research.md)）
- [ ] T009 `make generate` を実行し、生成物 `internal/httpapi/gen/api.gen.go` と `web/src/api/gen/openapi.ts` を版管理に含める。生成物は手編集しない（`AGENTS.md`）
- [ ] T010 [P] `internal/domain/health.go` に稼働情報の値を定義する。`Status` は enum(`ok`, `degraded`)、`Version` string、`Commit` string、`BuiltAt` time。判定は要求のたびに行い状態を保持しない。このパッケージは `net/http`・`database/sql`・`os/exec`・他の `internal/*` に依存しない（[data-model.md](./data-model.md)）
- [ ] T011 `internal/store/sqlite.go` に SQLite 接続を実装する。ドライバは `modernc.org/sqlite` v1.58.0（CGO 不要）。接続時に `journal_mode=WAL`、`busy_timeout`、`foreign_keys=ON` を設定し、疎通確認用の Ping を公開する。データベースのパスは `DataDir/mdm.db` に固定し、設定項目にしない（[data-model.md](./data-model.md)）
- [ ] T012 [P] `internal/store/migrations/00001_init.sql` を goose 形式で作成する。`videos` の列は data-model.md の定義どおり: `id` integer primary key（FTS5 の `rowid` と対応させる）、`path` text **not null, unique**（ファイルの絶対パス。保存前に Unicode NFC へ正規化する）、`title` text **not null**、`size_bytes` integer **not null**、`mtime` integer **not null**（Unix 秒）、`added_at` integer **not null, default**（Unix 秒）。あわせて `fts5(title, path, content='videos', content_rowid='id', tokenize='trigram')` の `videos_fts` と、`videos` への `INSERT`／`UPDATE`／`DELETE` を同期するトリガを作る
- [ ] T013 `internal/store/migrate.go` に起動時マイグレーションを実装する。`github.com/pressly/goose/v3` v3.28.0 のライブラリ API と `embed.FS` を使い、(1) 未適用があれば適用、(2) データベースがアプリケーションの知らない**将来の版**を持っていた場合は何も書き換えずに起動を中止、(3) ファイルが存在しない場合は作成してから適用、を満たす（[data-model.md](./data-model.md) / [R-006](./research.md)）

**Checkpoint**: 契約からの生成物がそろい、空のデータベースにスキーマを適用できる。ここから US1／US2／US3 は並行して進められる

---

## Phase 3: User Story 1 - clone から起動までを1コマンドで通す (Priority: P1) 🎯 MVP

**Goal**: 何も設定していない環境で `make up` だけを実行すれば、アプリケーションが起動し、ブラウザとヘルスエンドポイントから稼働を確認でき、停止も安全に行える

**Independent Test**: リポジトリを取得した直後の状態で `make up` を実行し、`http://localhost:8080` に稼働表示が出て、`curl http://localhost:8080/api/health` が `status` と `version` を含む JSON を返すこと（[quickstart.md](./quickstart.md) S1〜S6）

### Tests for User Story 1 ⚠️

> **NOTE: 先にこれらを書き、実装前に失敗することを確認する**

- [ ] T014 [P] [US1] `cmd/mdm/config_test.go`: 環境変数が未設定でも既定値（`MDM_ADDR`=`:8080`、`MDM_MEDIA_DIR`=`/media`、`MDM_DATA_DIR`=`/data`、`MDM_LOG_LEVEL`=`info`）で組み立てられること、および不正な値が**まとめて**列挙されること（1 つ見つけて即終了しない）を検証する（[contracts/configuration.md](./contracts/configuration.md)）
- [ ] T015 [P] [US1] `internal/httpapi/health_test.go`: `net/http/httptest` で `GET /api/health` が `200`、`Content-Type: application/json; charset=utf-8`、`Cache-Control: no-store` を返し、本文が `status` と `version` を必ず含むこと。保存層へ疎通できない場合は `503` と `status: degraded` になることを検証する（[contracts/openapi.yaml](./contracts/openapi.yaml) / [contracts/http-routes.md](./contracts/http-routes.md)）
- [ ] T016 [P] [US1] `internal/httpapi/spa_test.go`: 未知のパスが `index.html` を `200` で返すこと、`/api/` 配下の未定義経路は **HTML ではなく** `Error` スキーマの JSON `404` を返すこと、`index.html` が `Cache-Control: no-cache`、`/assets/*` が `public, max-age=31536000, immutable` を返すことを検証する（[contracts/http-routes.md](./contracts/http-routes.md)）
- [ ] T017 [P] [US1] `internal/store/migrate_test.go`: 空のディレクトリから起動してスキーマが自動適用されること、データベースファイルを削除して再実行しても手作業なしに復旧すること（SC-006）、`goose_db_version` が埋め込み済みより新しい版を持つ場合は書き換えずに失敗することを検証する

### Implementation for User Story 1

- [ ] T018 [US1] `cmd/mdm/config.go` に `MDM_*` 環境変数の解析と検証を実装する。検証規則は [data-model.md](./data-model.md) のとおり: `Addr` は `net.SplitHostPort` で解釈できること、`MediaDir` は絶対パスで存在と読み取り可否を確認し不可なら起動中止、`DataDir` は絶対パスで存在しなければ作成（作成失敗は起動中止）、`LogLevel` は `debug` / `info` / `warn` / `error` のいずれか。不正な項目は一度に列挙して終了する
- [ ] T019 [US1] `internal/media/preflight.go` に起動前確認を実装する。`exec.LookPath` で `ffprobe` と `ffmpeg` を確認し、欠けていれば**不足しているコマンド名と導入方法**を含む結果を返す（FR-008／SC-008、[R-009](./research.md)）
- [ ] T020 [US1] `internal/httpapi/health.go` に稼働確認ハンドラを実装する。`internal/domain` の Health と保存層への疎通結果から `ok`／`degraded` を決め、`200`／`503` を返す。版情報は `runtime/debug.ReadBuildInfo()` の `vcs.revision`／`vcs.time` から取り、リリース名は `-ldflags "-X main.version=…"` で上書き可能・既定は `dev`（[R-007](./research.md)）
- [ ] T021 [US1] `internal/httpapi/spa.go` に SPA 配信を実装する。`embed.FS` で `web/dist` を同梱し、`/assets/*` は静的配信、`/api/` 配下以外の未知のパスは `index.html` にフォールバックする。応答ヘッダは [contracts/http-routes.md](./contracts/http-routes.md) の表に従う
- [ ] T022 [US1] `internal/httpapi/router.go` に経路の分配を実装する。`/api/health` → JSON、`/api/*`（未定義） → `404` + `Error`（`index.html` を返してはならない）、それ以外 → SPA
- [ ] T023 [US1] `cmd/mdm/main.go` に起動と停止を実装する。順序は「設定読み込み → `log/slog` の JSON ハンドラ設定 → 起動前確認（T019） → データベース接続とマイグレーション（T011／T013） → HTTP サーバー起動」。起動時に有効な設定値とバージョン情報を1行で記録する（FR-007）。`SIGINT`／`SIGTERM` を受けたら新規受付を止め、処理中の要求を**猶予 10 秒**まで待って終了し、正常終了の終了コードは `0`（[contracts/http-routes.md](./contracts/http-routes.md)）
- [ ] T024 [P] [US1] `web/src/App.tsx` に稼働状態の画面を実装する。アプリケーション名と `/api/health` の `status`・`version` を表示し、型は `web/src/api/gen/openapi.ts`（T009 の生成物）から取る。ルーターとサーバー状態キャッシュは入れない（[R-011](./research.md)）
- [ ] T025 [US1] `Dockerfile` を multi-stage で作成する（web ビルド → go ビルド → alpine + ffmpeg）。`CGO_ENABLED=0` を維持し、最終段に `ffmpeg`／`ffprobe` を同梱する
- [ ] T026 [US1] `compose.yaml` を作成する。`8080` の公開、`MDM_MEDIA_DIR`／`MDM_DATA_DIR` に対応するマウント、データ用の名前付きボリューム（[quickstart.md](./quickstart.md) S4 が参照する名前と一致させる）を定義する
- [ ] T027 [US1] `Makefile` の `up`（`docker compose up --build`）、`down`、`dev`（Go サーバーと Vite 開発サーバー）、`build`（SPA ビルド → 埋め込み → 単一バイナリ）を実装する（[contracts/developer-commands.md](./contracts/developer-commands.md)）
- [ ] T028 [US1] `README.md` に導入手順を追記する（FR-016）。前提ツールは Docker のみ、最初に実行するコマンドは `make up` の1つだけであることを示し、その他の目標は開発者向けの補足として扱う。認証を掛けていないため外部公開を前提にしないことも明記する（[contracts/http-routes.md](./contracts/http-routes.md)）

**Checkpoint**: `make up` だけでアプリケーションが起動し、[quickstart.md](./quickstart.md) の S1〜S6 が通る。ここで止めても「動く骨組み」として価値がある

---

## Phase 4: User Story 2 - 壊れた変更が自動で止まる (Priority: P2)

**Goal**: 書式・静的検査・自動テスト・依存方向の違反を1コマンドで検出でき、同じ検査が変更提案に対して自動で実行される

**Independent Test**: `internal/domain` に禁止された import を一時的に足して `make lint` が失敗し、出力から禁止理由が読み取れること。戻せば再び成功すること（[quickstart.md](./quickstart.md) S7／S8／S10）

- [ ] T029 [P] [US2] `.golangci.yml` を `golangci-lint` v2 の書式（`version: "2"`）で作成する。`depguard` で `internal/domain` からの `net/http`・`database/sql`・`os/exec`・`modernc.org/sqlite`・他の `internal/*` パッケージの import を禁止し、**ルールごとに禁止理由の文言**を書いて違反時の出力だけで原因が分かるようにする（FR-010／FR-013、[R-005](./research.md)）
- [ ] T030 [US2] `Makefile` の `fmt`・`lint`・`test`・`check` を実装する。`check` は「`fmt` の差分確認 → `lint` → `test` → 生成物の差分確認」の順に実行し、`make generate` の再実行で差分が出る状態を失敗とみなす。CI でしか動かない検査を作らない（[contracts/developer-commands.md](./contracts/developer-commands.md)）
- [ ] T031 [P] [US2] Web 側の検査を `web/package.json` の scripts に定義し、`Makefile` の `lint`／`test` から呼ぶ。Phase 0 の Web はビルド検証（`tsc` + `vite build`）までとし、E2E は入れない（[plan.md](./plan.md) Technical Context）
- [ ] T032 [US2] `.github/workflows/ci.yml` を作成する。`main` への PR と push で実行し、Go と Web のジョブを分けて並行させ、どちらも `Makefile` の目標を呼ぶ。Go の版は `actions/setup-go` の `go-version-file: go.mod` で `go.mod` に合わせ、依存をキャッシュする。Docker イメージのビルドは別ジョブにする（FR-012／SC-004、[R-012](./research.md)）
- [ ] T033 [US2] `internal/domain` に `import _ "net/http"` を一時的に足して `make lint` が失敗し、出力に禁止理由が含まれることを確認する。`database/sql` と `os/exec` でも同様に確認し、確認後は変更を戻して `make lint` が成功することを確かめる（[quickstart.md](./quickstart.md) S8）
- [ ] T034 [US2] `Makefile` の `check` 目標を `time make check` で計測し、手元で 5 分以内に完了することを確認する（SC-002）。超える場合は検査を別目標へ切り出すのではなく原因を直す（[contracts/developer-commands.md](./contracts/developer-commands.md)）

**Checkpoint**: US1 と US2 がそれぞれ独立して成立している。壊れた変更は人手のレビューに到達する前に止まる

---

## Phase 5: User Story 3 - 採用した基盤の前提を最初に実証する (Priority: P3)

**Goal**: 追加のミドルウェアなしに日本語の部分一致検索ができるという前提が、実際に適用されたスキーマの上で自動テストとして固定される

**Independent Test**: `go test ./internal/store/ -run FTS -v` を、追加のミドルウェアを起動していない環境で実行して成功すること（[quickstart.md](./quickstart.md) S9）

- [ ] T035 [US3] `internal/store/fts_test.go` に FTS5 trigram の実証テストを書く。T012 のマイグレーションを適用した上で、[R-001](./research.md) の実測に対応する5点を期待値として固定する: (1) `tokenize='trigram'` の仮想表が作成できる、(2) 3文字の語 `夏休み` が `MATCH` で一致する、(3) 語中の3文字 `みの旅` が `MATCH` で一致する、(4) **2文字の語 `旅行` は `MATCH` で0件**（この挙動自体を期待値として書く）、(5) 同じ FTS5 表への `LIKE '%旅行%'` は一致する（FR-014／SC-007）
- [ ] T036 [P] [US3] `internal/store/fts_test.go` の失敗時メッセージに、検討すべき代替手段の参照先（[research.md](./research.md) R-001 の "Alternatives considered"）を明示する。テストが落ちた開発者が、次に何を読むべきか出力だけで分かる状態にする（FR-015）
- [ ] T037 [US3] `internal/store/fts_test.go` に `insert into videos_fts(videos_fts) values ('rebuild')` による再構築が成功することを追加する。トリガの取りこぼしが疑われたときの復旧手段が実際に動くことを固定する（[data-model.md](./data-model.md)「一貫性と再構築」）

**Checkpoint**: 3 つのユーザーストーリーがすべて独立して成立している

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 受け入れ検証と、変更と同じ単位での文書更新

- [ ] T038 [quickstart.md](./quickstart.md) の S1〜S10 を順に実行し、すべて期待どおりであることを確認する
- [ ] T039 `docs/exec-plans/active/001-initial-setup.md` の「進捗」表と「決定の記録」を更新する（FR-017）。受け入れ検証まで完了したら `docs/exec-plans/completed/` へ移す（`AGENTS.md`）
- [ ] T040 [P] 実装中に受け入れた妥協点を `docs/exec-plans/tech-debt.md` に記録する
- [ ] T041 [P] `ARCHITECTURE.md` の "Intended topology" と "Intended dependency direction" を、実際に入った配置に合わせて更新する（「No application code has been introduced yet」の記述を含む）
- [ ] T042 [P] `README.md` の "Repository structure" を、`cmd/`・`internal/`・`api/`・`web/` が入った後の実際の構成に更新する
- [ ] T043 `Makefile` の `check` と `.github/workflows/ci.yml` の両方が緑であることを確認し、同じ判定になっていることを確かめる（FR-011／FR-012）

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 依存なし。すぐ開始できる
- **Foundational (Phase 2)**: Setup の完了に依存。**すべてのユーザーストーリーを塞ぐ**
- **User Stories (Phase 3〜5)**: すべて Foundational の完了に依存
  - 完了後は並行して進められる
  - あるいは優先度順（P1 → P2 → P3）に進める
- **Polish (Phase 6)**: 実施したいユーザーストーリーがすべて完了していることに依存

### User Story Dependencies

- **User Story 1 (P1)**: Foundational 後に開始可。他ストーリーに依存しない
- **User Story 2 (P2)**: Foundational 後に開始可。検査対象のコードは US1 の産物だが、`internal/domain`（T010）と 6 パッケージの骨組み（T003）があれば depguard も `make check` も成立するため、US1 の完了は待たない
- **User Story 3 (P3)**: Foundational 後に開始可。T012／T013 のみに依存し、US1・US2 とは独立

### Within Each User Story

- テスト（T014〜T017、T035）を先に書き、実装前に失敗することを確認する
- 値・スキーマ → 保存層 → ハンドラ → 組み立て（`main.go`）の順
- ストーリーを完了させてから次の優先度へ移る

### Parallel Opportunities

- Phase 1: T002〜T006 は T001 の後にすべて並行可（T007 のみ最後）
- Phase 2: T010 と T012 は並行可。T011 → T013 は逐次（T013 が接続を使う）
- Phase 3: T014〜T017 の 4 テストは並行可。T024（Web）は T018〜T023（Go）と並行可
- Phase 4: T029 と T031 は並行可
- Phase 6: T040・T041・T042 は並行可
- Foundational 完了後は、US1・US2・US3 を別々の担当者が並行で進められる

---

## Parallel Example: User Story 1

```bash
# US1 のテストをまとめて着手する:
Task: "cmd/mdm/config_test.go で既定値と不正値の一括列挙を検証"
Task: "internal/httpapi/health_test.go で 200/503 とヘッダを検証"
Task: "internal/httpapi/spa_test.go でフォールバックと /api/* の JSON 404 を検証"
Task: "internal/store/migrate_test.go で自動適用・削除後復旧・将来版の中止を検証"

# 実装フェーズで Go と Web を並行させる:
Task: "internal/httpapi/spa.go に埋め込み配信を実装"
Task: "web/src/App.tsx に稼働状態の画面を実装"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup を完了する
2. Phase 2: Foundational を完了する（**重要 — 全ストーリーを塞ぐ**）
3. Phase 3: User Story 1 を完了する
4. **止めて検証する**: [quickstart.md](./quickstart.md) の S1〜S6 を実行する
5. ここまでで「取得して1コマンドで起動し、ブラウザで稼働を確認できる」骨組みが手に入る

### Incremental Delivery

1. Setup + Foundational → 土台
2. US1 を足す → S1〜S6 で検証 → **MVP**
3. US2 を足す → S7／S8／S10 で検証 → 壊れた変更が自動で止まる
4. US3 を足す → S9 で検証 → 採用した基盤の前提が固定される
5. Phase 6 で文書を更新し、Phase 0 完了と判定する

### Parallel Team Strategy

複数人で進める場合:

1. Setup + Foundational を全員で終わらせる
2. Foundational 完了後:
   - 担当 A: User Story 1（起動経路と配信）
   - 担当 B: User Story 2（検査と CI）
   - 担当 C: User Story 3（FTS5 の実証）
3. 各ストーリーは独立して完了・統合できる

---

## Notes

- [P] のタスク = 別ファイル・依存なし
- [Story] ラベルはタスクとユーザーストーリーの対応を追えるようにするためのもの
- 生成物（`internal/httpapi/gen/`、`web/src/api/gen/`）は手編集しない。直すのは `api/openapi.yaml` 側（`AGENTS.md`）
- 実装前にテストが失敗することを確認する
- タスクごと、あるいは論理的なまとまりごとにコミットする
- 各 Checkpoint で止めて、そのストーリー単独で成立しているか検証してよい
