# Implementation Plan: 初期セットアップ（Phase 0 骨組み）

**Branch**: `claude/peaceful-tesla-as1pwp`（機能ディレクトリ: `001-initial-setup`） | **Date**: 2026-09-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-initial-setup/spec.md`

## Summary

コードがまだ1行も無いリポジトリに、「取得して1コマンドで起動し、ブラウザで稼働を
確認でき、壊れた変更は自動検証で止まる」最小の骨組みを置く。

技術的な進め方は、単一の Go バイナリが JSON API と埋め込み SPA を配信し、SQLite を
起動時に自動マイグレーションして使う構成を、機能ゼロの状態で通すことである。
経路（設定 → 起動 → DB → API → SPA → 停止）を1本だけ端から端まで通し、そこに
`golangci-lint` の depguard による依存方向の強制、OpenAPI からの Go／TypeScript
コード生成、GitHub Actions での自動検証を最初から掛ける。

Phase 0 最大のリスクだった「CGO 不要の SQLite ドライバで日本語の部分一致検索が
できるか」は実測で解消した（[research.md](./research.md) R-001）。trigram は使えるが
**2文字以下の検索語には `MATCH` が一致しない**ため、検索は `MATCH` と `LIKE` の
2経路にする方針を Phase 0 のテストで固定する。

## Technical Context

**Language/Version**: Go 1.26（`go.mod` の `go` ディレクティブ、[R-002](./research.md)） / TypeScript（`web/package.json` で固定、Node 22 LTS）

**Primary Dependencies**: 標準 `net/http`・`log/slog`・`embed`／`modernc.org/sqlite` v1.58.0（CGO 不要）／`github.com/pressly/goose/v3` v3.28.0／React 19 + Vite 8 + Tailwind CSS 4。開発ツールは `golangci-lint` v2.13.2、`oapi-codegen` v2.8.0、`openapi-typescript` v7.13.0

**Storage**: SQLite 単一ファイル（WAL モード）。Phase 0 は最小スキーマのみ

**Testing**: `go test` + `net/http/httptest`（API と Range 以外の経路）、`internal/store` の FTS5 実証テスト、Web は Vite のビルド検証まで（E2E は Phase 3）

**Target Platform**: Linux コンテナ（amd64／arm64、alpine + ffmpeg）。開発環境は Docker が動く Linux／macOS

**Project Type**: web-service（単一 Go バイナリ + 埋め込み React SPA）

**Performance Goals**: 起動から稼働確認の応答まで 2 秒以内／`make check` を手元で 5 分以内（SC-002）／CI 10 分以内（SC-004）

**Constraints**: 常駐する外部ミドルウェアを増やさない／`CGO_ENABLED=0` を維持／設定なしの既定値で起動できる／依存取得後はオフラインで再ビルドできる

**Scale/Scope**: 単一ユーザーのセルフホスト。Phase 0 の成果物に利用者向け機能はなく、追加コードは 1,500 行程度を上限の目安とする

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` は雛形のまま（プレースホルダのみ）で批准されていない。
そのためゲートは、実質的な規範として機能している
[`ARCHITECTURE.md`](../../ARCHITECTURE.md)、
[core-beliefs.md](../../docs/design-docs/core-beliefs.md)、
[技術選定文書 2. 選定の判断基準](../../docs/design-docs/tech-stack-selection.md)から導出した。
（正式な憲章を作るなら `/speckit-constitution` を別途実行すること。本計画はその結果と
矛盾しない範囲に収めてある。）

| ゲート | 根拠 | 初期判定 | Phase 1 後の再判定 |
| --- | --- | --- | --- |
| G1: 依存方向が一方向で、機械的に強制される | ARCHITECTURE.md | PASS — depguard を Phase 0 で導入（FR-010） | PASS — `internal/domain` は標準ライブラリの基本型のみに依存 |
| G2: 常駐する外部ミドルウェアを増やさない | 判断基準1 | PASS — SQLite と単一バイナリのみ | PASS — 追加の常駐プロセスなし |
| G3: DB は再構築可能なインデックスに留める | 判断基準2 | PASS — Phase 0 は利用者データを持たない | PASS — 削除→再起動で自動復旧（SC-006） |
| G4: 重要な制約は可能な限りテスト可能にする | core-beliefs | PASS — 依存方向・検索前提・起動経路を自動検証 | PASS — 各制約に対応するテストを配置 |
| G5: 文書は変更と同じ変更単位で更新する | AGENTS.md / core-beliefs | PASS — 実行計画と README を成果物に含む（FR-016／FR-017） | PASS |
| G6: 生成物は手編集せず、元ファイルから生成する | AGENTS.md | PASS — OpenAPI 生成物は `make generate` 由来、CI で差分検査 | PASS |
| G7: 後から重くできる境界を最初に引く | 判断基準4 | PASS — 6 パッケージの境界を Phase 0 で作る（FR-009） | PASS |

違反なし。justify が必要な逸脱は「Complexity Tracking」に1件記載する。

## Project Structure

### Documentation (this feature)

```text
specs/001-initial-setup/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 未確定事項の解消（実測含む）
├── data-model.md        # Phase 1 output — Phase 0 で作る最小スキーマ
├── quickstart.md        # Phase 1 output — 受け入れの検証手順
├── contracts/           # Phase 1 output
│   ├── openapi.yaml          # Phase 0 の API 契約（api/openapi.yaml の原型）
│   ├── http-routes.md        # OpenAPI に書けない経路・ヘッダ・停止の約束
│   ├── configuration.md      # 環境変数と起動前確認の契約
│   └── developer-commands.md # make 目標の契約
├── checklists/
│   └── requirements.md  # /speckit-specify の品質チェックリスト
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
cmd/
└── mdm/
    ├── main.go            # 設定読み込み、依存の組み立て、起動と停止
    ├── config.go          # MDM_* 環境変数の解析と既定値
    └── config_test.go

internal/
├── domain/                # ドメインモデルとユースケース（外部 I/O 依存なし）
│   ├── doc.go
│   └── health.go          # 稼働情報の値と判定
├── httpapi/               # ルーティング、ハンドラ、SPA 配信
│   ├── router.go
│   ├── health.go
│   ├── spa.go             # web/dist の埋め込みと index.html フォールバック
│   ├── gen/               # oapi-codegen 生成物（手編集禁止）
│   └── *_test.go          # httptest による経路テスト
├── store/                 # SQLite 接続、マイグレーション、FTS5 実証
│   ├── sqlite.go
│   ├── migrate.go
│   ├── migrations/        # goose の SQL を embed
│   │   └── 00001_init.sql
│   └── fts_test.go        # FR-014／SC-007 の実証テスト
├── media/                 # 外部ツールのアダプタ（Phase 0 は存在確認のみ）
│   ├── doc.go
│   └── preflight.go
├── scanner/               # ファイル走査（Phase 0 は境界の宣言のみ）
│   └── doc.go
└── jobs/                  # ジョブキュー（Phase 0 は境界の宣言のみ）
    └── doc.go

api/
└── openapi.yaml           # API 契約（Go/TS 双方の生成元、唯一の真実）

web/                       # React SPA
├── src/
│   ├── main.tsx
│   ├── App.tsx            # 稼働状態の表示
│   └── api/gen/           # openapi-typescript 生成物（手編集禁止）
├── dist/                  # ビルド成果物（embed 対象。プレースホルダを版管理）
├── index.html
├── package.json
├── vite.config.ts
└── tailwind.config.ts

.github/workflows/ci.yml   # FR-012 の自動検証
Dockerfile                 # multi-stage（web ビルド → go ビルド → alpine + ffmpeg）
compose.yaml               # make up の実体
Makefile                   # up / dev / build / generate / check / test / lint / fmt
.golangci.yml              # depguard による依存方向の強制（v2 書式）
```

**Structure Decision**: 技術選定文書
[3.1 リポジトリ構成](../../docs/design-docs/tech-stack-selection.md)の配置をそのまま採る。
Phase 0 の時点で 6 つの `internal` パッケージをすべて作り（FR-009／SC-005）、
中身が無いものは `doc.go` で責務と依存の向きだけを宣言する。空ディレクトリを
置かないのは、Git に残らず depguard の対象にもならないため。

設定の読み込みは技術選定文書の記述どおり `cmd/mdm` に置き、`internal/config` を
新設しない。依存グラフの節点を増やさず、ARCHITECTURE.md の
`cmd → internal/{…} → internal/domain` をそのまま保つ。

## Phase 1 以降へ送る判断

Phase 0 では決めず、該当フェーズの計画で扱う。

| 論点 | 送り先 | 理由 |
| --- | --- | --- |
| `videos` の全列と `content_key` の算出方式 | Phase 1 | 走査とメタデータ取得の実装と同時に決めるほうが手戻りが少ない |
| TanStack Router／Query の導入 | Phase 1 | 画面が1つの間は不要（[R-011](./research.md)） |
| `sqlc` によるクエリ生成の本格運用 | Phase 1 | Phase 0 のクエリは実証テストのみで、生成する対象が無い |
| 2文字検索の `LIKE` 経路の実装と閾値 | Phase 2 | Phase 0 は「両経路が動く」ことの固定まで |
| Playwright による再生 E2E | Phase 3 | 再生機能が存在してから |
| 認証とセッション | Phase 3 | 技術選定文書のフェーズ分けに従う |

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| spec の Key Entities に無い `videos` / `videos_fts` の最小テーブルを Phase 0 の マイグレーションに含める | FR-014／SC-007 の「日本語の部分一致検索が追加ミドルウェアなしに動く」実証を、実際に適用されたスキーマの上で行う必要がある。SC-006（DB 削除後の自動復旧）も、適用対象が無いと検証にならない | テスト内だけで一時表を作る案は、マイグレーション経路（FR-005）を一度も通さずに Phase 0 を終えることになり、初回起動の自動初期化が未検証のまま残る |
