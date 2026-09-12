# Phase 0 Research: 初期セットアップ（骨組み）

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-12

技術の採否そのものは
[技術選定文書](../../docs/design-docs/tech-stack-selection.md)で決定済みである。
本ドキュメントは、その決定を Phase 0 で実装に落とすために残っていた
未確定事項だけを扱い、実測できるものは実測して閉じる。

---

## R-001: FTS5 trigram は `modernc.org/sqlite` で使えるか（最大のリスク）

**Decision**: 使える。CGO 不要の `modernc.org/sqlite` をそのまま採用し、
`mattn/go-sqlite3` への切り替えは行わない。ただし **trigram は3文字未満の
検索語に一致しない**ため、検索は「3文字以上は `MATCH`、1〜2文字は同じ FTS5 表への
`LIKE '%…%'`」の2経路で実装する。

**Rationale**: 実測した（`modernc.org/sqlite v1.58.0`、同梱 SQLite 3.53.4、
Go 1.26、linux/amd64）。

| 検証項目 | 結果 |
| --- | --- |
| `create virtual table … using fts5(…, tokenize='trigram')` | 成功 |
| 3文字の検索語 `夏休み` に `MATCH` | 1件（一致する） |
| 語中の3文字 `みの旅` に `MATCH` | 1件（部分一致が効く） |
| **2文字の検索語 `旅行` に `MATCH`** | **0件（一致しない）** |
| 2文字 + 前方一致 `旅行*`、引用符 `"旅行"` | いずれも 0件 |
| FTS5 表に対する `LIKE '%旅行%'` | 1件（一致する） |
| 同上の `explain query plan` | `SCAN videos_fts VIRTUAL TABLE INDEX 0:L0` = trigram 索引が `LIKE` を処理 |
| 外部コンテンツ表（`content='videos'`）+ `rebuild` | 成功 |

trigram トークナイザは3文字単位で索引を作るため、2文字以下の検索語は
`MATCH` の対象になり得ない。一方 SQLite は trigram 表に対する `LIKE`／`GLOB` を
索引で処理する最適化を持ち、上の実行計画（`INDEX 0:L0`）でそれが効いていることを
確認した。日本語では「旅行」「花火」のような2文字の検索語が現実に多いため、
この2経路の切り替えは実装必須である。

**Alternatives considered**:

- `mattn/go-sqlite3` + `-tags sqlite_fts5`: CGO が必要でクロスコンパイルと
  alpine ビルドが面倒になる。上記の実測により採用理由がなくなった。技術選定文書の
  「代替として残す」記述は、本結果をもって当面凍結でよい。
- 2文字検索を諦める: 日本語の利用実態に合わない。却下。
- 別の全文検索エンジン（Meilisearch 等）の導入: 常駐プロセスが増え、
  技術選定文書の判断基準1（1プロセス・1コンテナ）に反する。却下。

**Phase 0 での落とし所**: 上の表のうち「trigram 表の作成」「3文字 `MATCH`」
「語中3文字 `MATCH`」「2文字 `LIKE`」「2文字 `MATCH` が0件であること」を
`internal/store` の自動テストとして固定する（FR-014／SC-007）。
2文字が `MATCH` で0件であることも**期待値としてテストに書く**。将来 SQLite 側の
挙動が変わったら気付ける。

---

## R-002: Go のバージョンとツールチェーンの固定方法

**Decision**: `go.mod` に `go 1.26.0` を書く。CI では `actions/setup-go` の
`go-version-file: go.mod` で同じ版を使い、`GOTOOLCHAIN` は既定（`auto`）のままにする。

**Rationale**: モジュールプロキシ上の最新安定版は 1.27.1、1.26 系は 1.26.8 まで出ている
（実測）。技術選定文書は「1.26 系以降」を想定しており、依存の
`modernc.org/sqlite v1.58.0` は Go 1.25.0 以上を要求する。最小要求を 1.26.0 に置けば
両方を満たし、開発者の手元が 1.26 でも 1.27 でもビルドできる。`toolchain` 行は
書かない（書くと開発者ごとのツールチェーン再取得を強制するだけで利点がない）。

**Alternatives considered**: `go 1.27.0` に上げる（1.26 しか無い環境を弾く必要がない）、
`toolchain` 行で特定パッチ版に固定する（CI 側の指定で足りる）。いずれも却下。

---

## R-003: 「1コマンドで起動」を何にするか

**Decision**: `make up`（実体は `docker compose up --build`）を正の起動導線とする。
ホストに必要なのは Docker のみ。開発中の再読み込み用に `make dev`（Go と Vite を
それぞれ起動）を併置するが、導入手順の先頭に置くのは `make up` とする。

**Rationale**: SC-001 は「導入手順のみで15分以内・コマンド1つ」。`ffmpeg` や Go の
導入をホストに求めると15分に収まらないうえ、環境差で失敗する。コンテナ内に
`ffmpeg` を同梱すれば FR-008 の不足検知も「開発者の環境依存」ではなく
「異常系」として扱える。

**Alternatives considered**: `go run ./cmd/mdm` を正とする（ホストに Go・Node・ffmpeg が
必要で SC-001 を満たさない）、devcontainer（エディタ依存を持ち込む。将来追加は可能）。

---

## R-004: 設定の受け渡し方

**Decision**: 環境変数（接頭辞 `MDM_`）のみ。設定ファイル形式も設定ライブラリも
導入しない。解析は `cmd/mdm` 内の小さな関数で行い、既定値だけで起動できるようにする。
契約は [contracts/configuration.md](./contracts/configuration.md) を参照。

**Rationale**: 技術選定文書の構成では設定の読み込みは `main` の責務。項目数が
一桁で、Docker Compose・CI・手元実行のすべてで環境変数がそのまま使える。
依存を増やさない判断基準に合う。

**Alternatives considered**: `viper` 等の設定ライブラリ（項目数に対して過剰）、
YAML 設定ファイル（Compose と二重管理になる）。

---

## R-005: 依存方向をどう機械的に落とすか

**Decision**: `golangci-lint` v2（最新 v2.13.2）の `depguard` を使い、
`internal/domain` からの `net/http`・`database/sql`・`os/exec`・`modernc.org/sqlite`・
他の `internal/*` パッケージの import を禁止する。禁止理由の文言をルールごとに書き、
違反時の出力だけで原因が分かるようにする（FR-013）。

**Rationale**: `ARCHITECTURE.md` が明示的に「depguard で強制する」と定めている。
v2 は設定ファイルの書式が v1 と非互換（`version: "2"` が必要）なので、
最初から v2 の書式で書き、CI もメジャー版を固定して入れる。

**Alternatives considered**: `go-arch-lint` や自作の import 検査テスト
（`golangci-lint` を別途入れる以上、ルールを一箇所に集めたほうが安い）。

---

## R-006: マイグレーションの適用方法

**Decision**: `goose`（v3、最新 v3.28.0）のライブラリ API を使い、
SQL ファイルを `embed.FS` で同梱して起動時に自動適用する。CLI の `goose` は
必須にしない。適用済みより新しい構造をデータベース側が持っていた場合は、
何も書き換えずに起動を中止する。

**Rationale**: FR-005（初回起動で手作業不要）と SC-006（データ削除後も自動復帰）を
外部ツールなしで満たせる。バイナリ1つという配布形態も崩さない。

**Alternatives considered**: `golang-migrate`（CLI 前提の運用が中心）、
起動時に `CREATE TABLE IF NOT EXISTS` を並べる（変更履歴が残らず、
将来の構造変更で破綻する）。

---

## R-007: バージョン情報の埋め込み

**Decision**: `runtime/debug.ReadBuildInfo()` から `vcs.revision` と `vcs.time` を読む。
リリース名だけ `-ldflags "-X main.version=…"` で上書き可能にし、既定値は `dev`。

**Rationale**: コミットと時刻はビルド時に Go が自動で埋めるため、Makefile に
`git rev-parse` を書かずに済む。FR-002／稼働情報の返却に必要な情報が揃う。

**Alternatives considered**: すべて `ldflags` で渡す（Makefile と Dockerfile の
両方に同じ記述が要る）、埋め込まない（稼働中のビルドが特定できない）。

---

## R-008: SPA の配信と埋め込み

**Decision**: `web/` の Vite ビルド結果（`web/dist`）を `internal/httpapi` から
`embed.FS` で配信する。`/api/` 配下以外の未知のパスは `index.html` に落とす
（クライアント側ルーティングのため）。`make build` を
「SPA ビルド → 埋め込み → Go ビルド」の単一目標にする。

**Rationale**: 技術選定文書の「5. 既知のリスクと対処」がこの構成を指定している。
`embed` は対象ディレクトリが存在しないとコンパイルが通らないため、
`web/dist` を生成しない状態でも `go build` が通るよう、空の
プレースホルダ（`web/dist/.gitkeep` と最小の `index.html`）を版管理に置く。

**Alternatives considered**: SPA を別コンテナ（配布形態が2つになる）、
`http.Dir` で実行時にディスクから配信（単一バイナリの利点を失う）。

---

## R-009: 外部ツール（ffmpeg／ffprobe）不足の扱い

**Decision**: 起動時に `exec.LookPath` で `ffmpeg` と `ffprobe` を確認し、
欠けていれば**不足しているコマンド名と導入方法を出力して終了コード非0で停止**する。
`internal/media` に確認関数を置き、`cmd/mdm` が起動前に呼ぶ。

**Rationale**: FR-008／SC-008。Phase 0 ではまだ解析を行わないが、Phase 1 で
必ず必要になるため、「起動できた＝前提が揃っている」を Phase 0 の時点で
保証しておくほうが、後から不可解な失敗を追うより安い。

**Alternatives considered**: 警告のみで起動を継続する（Phase 1 で初めて失敗し、
原因が分かりにくい）、確認しない（FR-008 を満たさない）。

---

## R-010: API 契約とコード生成をどこまで Phase 0 で回すか

**Decision**: `api/openapi.yaml` を置き、`GET /api/health` の1本だけを定義する。
生成は Phase 0 から回す（Go: `oapi-codegen` v2.8.0 / TS: `openapi-typescript` v7.13.0）。
生成物は版管理に含め、`make generate` の再実行結果と差分がないことを CI で確認する。

**Rationale**: 「2言語構成のずれをコンパイルエラーで検出する」という仕組みは、
経路が1本のうちに通しておかないと、後から全経路に適用する作業になる。
生成物を版管理に入れるのは、生成器がなくてもビルドできる状態を保つため
（`AGENTS.md` の「生成物は手編集しない」に従い、編集は元ファイル側で行う）。

**Alternatives considered**: Phase 1 まで手書きの型で進める（ずれの検出が遅れる）、
生成物を版管理から外す（ビルドに生成器の導入が必須になり SC-001 に響く）。

---

## R-011: フロントエンドの最小構成

**Decision**: Vite + React + TypeScript + Tailwind CSS を入れる。
TanStack Router／Query は**画面が1つしかない Phase 0 では入れない**。
Phase 1 で一覧と詳細が出た時点で導入する。

**Rationale**: Phase 0 の画面は「稼働状態の表示」1つで、ルーティングも
サーバー状態のキャッシュも不要。使わない依存を先に入れると、
設定だけが残って腐る。技術選定文書は採用を決めているが、導入時期は縛っていない。

**Alternatives considered**: 最初から全部入れる（未使用の設定が増える）、
素の HTML で済ませる（Phase 1 で React 化する作業が丸ごと発生する）。

---

## R-012: CI の構成

**Decision**: GitHub Actions で1つのワークフロー。`main` への PR と push で
`make check`（書式・静的検査・Go テスト・Web ビルド・生成物の差分確認）を実行し、
Docker イメージのビルドも通す。ジョブは Go と Web で分け、並行させる。

**Rationale**: SC-004（10分以内）を満たすため、依存の取得をキャッシュし、
重いイメージビルドを別ジョブにする。ローカルの `make check` と CI が同じ
目標を呼ぶ構成にして、手元と CI の判定を一致させる（FR-011／FR-012）。

**Alternatives considered**: ワークフローを検査ごとに分割（設定の重複が増える）、
CI でのみ動く検査を作る（手元で再現できず FR-011 に反する）。

---

## 未解決事項

なし。Phase 1 の設計に持ち越す論点は
[plan.md](./plan.md) の「Phase 1 以降へ送る判断」に記載する。
