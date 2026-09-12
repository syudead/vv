# Implementation Plan: 絞られたコア機能（動画ライブラリの中核）

**Branch**: `claude/wizardly-hypatia-l44vre`（機能ディレクトリ: `002-core-video-library`） | **Date**: 2026-09-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-core-video-library/spec.md`

## Summary

001 で通した骨組み（設定 → 起動 → SQLite → API → SPA → 停止）の上に、
**置いた動画が自動で一覧に並び、ブラウザで再生でき、題名で探せる**までを載せる。
整理機能（タグ・お気に入り・コレクション）と画像は spec でスコープ外と決めている。

技術的な進め方は、既存の6つの `internal` パッケージの空いている境界を、宣言どおりの
責務で埋めることである。`scanner` がファイルを列挙して `store` に索引を作り、
重い処理（`ffprobe` の解析とサムネイル生成）は `jobs` の直列ワーカーが処理する。
`media` が外部プロセスを閉じ込め、再生可否の判定規則そのものは `domain` に置いて
外部プロセスなしでテストできるようにする。配信は `http.ServeContent` に任せる。

この機能で新しく判断が要ったのは、**移動・改名を越えて同一と判定する鍵**
（[R-101](./research.md)）と、**利用者データを索引から切り離す持ち方**
（[R-111](./research.md)）である。後者は `playback_progress` を `videos.id` ではなく
`content_key` で持つという決定で、FR-025／FR-026 と技術選定文書の判断基準2
（バックアップ対象の分離）を同時に満たす。

## Technical Context

**Language/Version**: Go 1.26（`go.mod`） / TypeScript（`web/package.json`、Node 22 LTS）

**Primary Dependencies**: 標準 `net/http`・`log/slog`・`os/exec`・`crypto/sha256`・`embed`／`modernc.org/sqlite`／`github.com/pressly/goose/v3`／React 19 + Vite 8 + Tailwind CSS 4。**新規に増やすのは 2 つだけ**: `golang.org/x/text`（Unicode NFC 正規化、[R-107](./research.md)）と `react-router` v7（2画面のルーティング、[R-113](./research.md)）

**Storage**: SQLite 単一ファイル（WAL）。`videos` を拡張し、`playback_progress`・`jobs`・`scans` を追加（[data-model.md](./data-model.md)）。サムネイルは `MDM_DATA_DIR/thumbnails/` にファイルで置く

**Testing**: `go test`（`internal/domain` の判定規則、`internal/scanner` の走査規則、`internal/store` の問い合わせと検索2経路）、`net/http/httptest`（一覧・詳細・Range 配信・進捗）、Web は型検査とビルド検証。再生の E2E は Phase 3

**Target Platform**: Linux コンテナ（amd64／arm64、alpine + ffmpeg）。ブラウザは `<video>` が使える現行世代

**Project Type**: web-service（単一 Go バイナリ + 埋め込み React SPA）

**Performance Goals**: 一覧1ページ目 2 秒以内（1万件、SC-003）／検索 1 秒以内（SC-006）／再生開始 3 秒以内・シーク再開 2 秒以内（SC-004）／1,000 本の初回取り込み 15 分以内（SC-002）

**Constraints**: 常駐する外部ミドルウェアを増やさない／`CGO_ENABLED=0` を維持／動画ファイルは読み取り専用（FR-009）／取り込み中も一覧と再生を止めない（FR-007）／ジョブの並列度は1（I/O 飽和の回避）

**Scale/Scope**: 数千〜1万本、総容量数 TB、単一利用者。追加コードは Go 2,500 行・TypeScript 800 行程度を上限の目安とする

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` は雛形のまま批准されていない。したがってゲートは
001 と同じく、実質的な規範として機能している
[`ARCHITECTURE.md`](../../ARCHITECTURE.md)、
[core-beliefs.md](../../docs/design-docs/core-beliefs.md)、
[技術選定文書 2. 選定の判断基準](../../docs/design-docs/tech-stack-selection.md)、
[`AGENTS.md`](../../AGENTS.md) から導出する。

| ゲート | 根拠 | 初期判定 | Phase 1 設計後の再判定 |
| --- | --- | --- | --- |
| G1: 依存方向が一方向で、機械的に強制される | ARCHITECTURE.md | PASS — 既存の depguard がそのまま効く | PASS — 再生可否・完了判定・再開位置は `domain` の純粋関数。`os/exec` は `media`、`database/sql` は `store` に閉じる |
| G2: 常駐する外部ミドルウェアを増やさない | 判断基準1 | PASS | PASS — ジョブはプロセス内 goroutine と `jobs` 表（[R-106](./research.md)） |
| G3: DB は再構築可能なインデックスに留める | 判断基準2 | 要設計 — 本機能で初めて利用者データ（再生位置）が生まれる | PASS — `playback_progress` のみを利用者データとして分離し、鍵を `content_key` にした（[R-111](./research.md)）。他はすべて再スキャンで復元（S10） |
| G4: 重要な制約は可能な限りテスト可能にする | core-beliefs | PASS | PASS — 判定規則は外部プロセス非依存、走査規則は一時ディレクトリ、Range は `httptest`（[quickstart.md](./quickstart.md) の対応表） |
| G5: 文書は変更と同じ変更単位で更新する | AGENTS.md / core-beliefs | PASS | PASS — ARCHITECTURE.md の「Not built yet」、README、技術選定文書のハッシュ関数の記述を成果物に含める |
| G6: 生成物は手編集せず、元ファイルから生成する | AGENTS.md | PASS | PASS — [contracts/openapi.yaml](./contracts/openapi.yaml) から Go/TS を生成できることを実行して確認済み |
| G7: 後から重くできる境界を最初に引く | 判断基準4 | PASS | PASS — トランスコード・並列化・検索基盤の差し替え位置は技術選定文書 6 のまま変わらない |
| G8: 画面の変更は画像で示す | AGENTS.md | 要対応 — 本機能は画面が増える初めての変更 | PASS — 一覧・再生の画面を PR に添える（[docs/how-to/ui-change-screenshots.md](../../docs/how-to/ui-change-screenshots.md)） |

違反なし。判断の分かれる点は「Complexity Tracking」に記載する。

## Project Structure

### Documentation (this feature)

```text
specs/002-core-video-library/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 未確定事項の解消（R-101〜R-115）
├── data-model.md        # Phase 1 output — 拡張する表と新しい表
├── quickstart.md        # Phase 1 output — 受け入れの検証手順（S0〜S10）
├── contracts/           # Phase 1 output
│   ├── openapi.yaml          # API 契約（api/openapi.yaml の次の姿）
│   ├── http-routes.md        # Range 配信・キャッシュ・経路の安全性
│   └── configuration.md      # 増える環境変数と既存項目の意味の変化
├── assets/
│   └── ui-mockup.webp   # 仕様の入力になった画面案
├── checklists/
│   └── requirements.md  # /speckit-specify の品質チェックリスト
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

追加・変更する場所だけを示す（`+` が新規、`~` が変更）。

```text
cmd/mdm/
~ main.go                  # スキャナ・ジョブワーカーの起動と停止の組み込み
~ config.go                # MDM_SCAN_ON_START の追加、thumbnails ディレクトリの確認

internal/
├── domain/                # 外部 I/O 依存なし。判定規則の置き場
│ + video.go               # Video・Probe・再生可否の許可リスト判定（R-103）
│ + progress.go            # 完了判定と再開位置の規則（R-111）
│ + scan.go                # ScanResult の集計
│ + *_test.go
├── httpapi/
│ + videos.go              # 一覧・詳細（カーソル・検索・並び順）
│ + stream.go              # ServeContent への委譲とパス封じ込め（R-105）
│ + thumbnail.go           # サムネイル配信（R-112）
│ + progress.go            # 再生位置の記録
│ + scans.go               # 取り込みの開始と進捗
│ ~ router.go              # 追加経路の登録
│ ~ gen/                   # oapi-codegen 生成物（手編集禁止）
│ + *_test.go              # httptest（Range・404・不正カーソル）
├── store/
│ + videos.go              # 取り込みの upsert、一覧・検索の問い合わせ（R-109/R-110）
│ + progress.go            # content_key 単位の upsert
│ + jobs.go                # ジョブの投入・専有・完了
│ + scans.go               # スキャンの記録
│ ~ migrations/00002_core.sql
│ + *_test.go
├── media/
│ + probe.go               # ffprobe の実行と JSON 解析（R-102）
│ + thumbnail.go           # ffmpeg による1枚抽出（R-104）
│ + *_test.go              # 解析部分は固定 JSON で、実行は統合テストで
├── scanner/
│ + scanner.go             # 走査・除外規則・NFC 正規化・移動検出（R-107）
│ + content_key.go         # 内容由来の識別子（R-101）
│ + *_test.go
└── jobs/
  + worker.go              # 直列ワーカー、再試行、起動時の巻き戻し（R-106）
  + *_test.go

api/
~ openapi.yaml             # contracts/openapi.yaml を反映（唯一の真実）

web/src/
~ App.tsx                  # ルーティング（/ と /videos/:id）
+ pages/LibraryPage.tsx    # 一覧・検索・並び替え・無限スクロール
+ pages/VideoPage.tsx      # 再生・続きから・再生位置の送信
+ components/VideoCard.tsx # サムネイル・長さ・視聴状態・再生不可の表示
+ components/ScanStatus.tsx# 取り込みの進捗
+ api/client.ts            # 生成型を使う薄い fetch ラッパ
~ api/gen/                 # openapi-typescript 生成物（手編集禁止）
```

**Structure Decision**: 新しいパッケージは作らない。001 が `doc.go` で責務と依存の
向きだけを宣言していた `scanner`・`jobs`・`media` を、その宣言どおりに埋める。
ARCHITECTURE.md の `cmd → internal/{httpapi,store,media,scanner,jobs} → internal/domain`
は変えず、スキャナ・ワーカーの組み立て（誰が誰を呼ぶか）は `cmd/mdm` で行う。

Web は画面が2つになるため `pages/` と `components/` を導入する。状態管理ライブラリは
入れない（[R-113](./research.md)）。

## Phase 2 以降へ送る判断

| 論点 | 送り先 | 理由 |
| --- | --- | --- |
| シークプレビュー用スプライト | Phase 2 | 一覧の要求は1枚で満たせる |
| タグ・お気に入り・コレクション | Phase 2 | spec でスコープ外と決定済み |
| データ取得ライブラリ（TanStack Query 等） | Phase 2 | 要求が3種類の間は自前の薄いフックで足りる（R-113） |
| `sqlc` によるクエリ生成 | Phase 2 以降 | 動的な組み立てが2箇所あり、生成と手書きの混在が読みにくい（R-115） |
| 字幕の変換と表示 | Phase 2 | 技術選定文書 8 のフェーズ分けどおり |
| 認証・バックアップ手順・再生の E2E | Phase 3 | 同上 |
| 非対応コーデックのトランスコード | 将来 | 技術選定文書 6 の拡張ポイント |

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 技術選定文書 3.2 が指定する BLAKE3 ではなく標準ライブラリの SHA-256 を使う（[R-101](./research.md)） | 読む量が1ファイル 2MiB に固定されており、ハッシュ関数の速度は取り込み時間を律速しない。依存を1つ増やさずに済む | BLAKE3（`lukechampine.com/blake3`）を入れる案は、2MiB では利点が出ないまま依存とビルドの検証対象が増える。文書側をこの決定に合わせて更新する |
| 検索の経路が `MATCH` と `LIKE` の2本になる | trigram は2文字以下の検索語に一致せず、日本語では2文字の検索語が多い（001 R-001 の実測、[TD-001](../../docs/exec-plans/tech-debt.md)） | 1本にする案は、2文字検索を捨てる（利用実態に合わない）か、外部の検索基盤を常駐させる（判断基準1に反する）かのどちらかになる |
| `playback_progress` が `videos` への外部キーを持たない | 動画が一覧から消えても再生位置を残すため（FR-025）。参照整合性より、利用者データの保全を優先する | 外部キー + 連鎖削除は、ファイルを一時的に外しただけで再生位置が消える。利用者から見て復旧不能な損失になる |
| 画面が2つになる時点で `react-router` を導入する | 再生中の動画を URL で開ける必要があり（共有・再読み込み）、一覧へ戻る操作の復元も要る | 自前の分岐で済ませる案は、戻る・進む・スクロール位置の復元まで自作することになり、無限スクロールの一覧で体験が落ちる |
