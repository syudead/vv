# Implementation Plan: サービスを完全に英語化し、i18n の基盤を整える

**Branch**: `feature/023-english-i18n` | **Parent Issue**: #408

**Input**: The parent Issue. It is this feature's specification.

## Summary

画面・API・ログ・保存する失敗理由に散らばった日本語の固定文言を英語にし、画面の文言と書式を
1 か所で扱う i18n の基盤を作る。提供する言語は英語だけで、言語の選択は無い。

- 画面の文言は `web/src/i18n/` の型付きカタログに集め、コンポーネントはそこから引く。埋め込みは
  関数の引数、単数・複数は `Intl.PluralRules`、日付・数・相対時刻は同じロケールの `Intl` で扱う
  （[research.md R-1](research.md#r-1-画面文言の置き場は型付きの自前カタログにする)、
  [R-2](research.md#r-2-書式と単数複数はブラウザの-intl-で扱う)）。`index.html` は `lang="en"`、
  video.js の独自言語もカタログから作る（[R-9](research.md#r-9-videojs-の文言はカタログから作る)）。
- 画面は API の `message` を通常の表示に使わず、`code` と新しい `reason`・`limit`・`tagName` から英語の文言を
  出す。既存の `code` と HTTP 状態は変えず、同じ `code` の中の状況は `reason` で区別する
  （[contracts/error-api.md §1](contracts/error-api.md#1-error-の-reasonlimittagname)、
  [R-4](research.md#r-4-api-エラーの具体性はコードを変えずに-reason-で足す)、
  [R-5](research.md#r-5-画面は既知のエラーを-reasoncode-からそれ以外を安全な概要で表示する)）。
- 解析と取り込みの失敗には理由のコードを別の列に保存し、画面は自由文（ffprobe の出力、過去の
  日本語）を出さずにコードから説明する。保存済みの自由文は書き換えない
  （[data-model.md](data-model.md)、[R-6](research.md#r-6-失敗理由は機械可読なコードを保存し画面は自由文を出さない)）。
- サーバーの `message`・ログ・`cmd/mdm` の出力は Go に英語で直接書く
  （[R-7](research.md#r-7-サーバーには文言カタログを置かず英語を直接書く)）。
- 訳し漏れは、画面は ESLint の `no-restricted-syntax`、固定の文言を運ぶ値の branded 型 `UiText`、疑似ロケールで描画する画面テスト、既知のエラーはカタログの
  `Record<ErrorCode | ErrorReason, …>` の型、サーバーは `gosmopolitan` で検出する
  （[R-3](research.md#r-3-訳し漏れは-lint-と型で検出する)）。
- コメント、OpenAPI の `description`、設計文書、`scripts/` は対象にしない
  （[R-8](research.md#r-8-英語化しないもの)）。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、Web 層の責務: [ARCHITECTURE.md](../../ARCHITECTURE.md)（「Intended dependency
  direction」「Web layer」）
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`Error`・`Video.probeError`・
  `Scan.error`）、`task generate`
- エラー応答の書き方: [internal/httpapi/router.go](../../internal/httpapi/router.go)
  （`writeError`・`invalidRequest`・`notFound`・`internalError`）
- 画面の API 呼び出しとエラー: [web/src/api/client.ts](../../web/src/api/client.ts)
  （`RequestFailed`・`toRequestFailed`・`errorMessage`）
- 書式: [web/src/lib/format.ts](../../web/src/lib/format.ts)、プレイヤーの文言:
  [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx)
- 失敗理由を書く箇所: `internal/store/jobs.go`（`recordTerminalFailure`）、
  `internal/store/scans.go`（`FinishScan`・`FailInterruptedScans`）、`internal/app/scans.go`
- lint: [.golangci.yml](../../.golangci.yml)、[web/eslint.config.js](../../web/eslint.config.js)
- 画面の見た目と文言の設計: [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
  と、[設計文書の索引](../../docs/design-docs/index.md)に載る各 `ui-design.md`
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task test-e2e`）

**Feature-specific context**:

- 依存は増やさない。画面は `Intl`、lint は ESLint の組み込み規則と golangci-lint 同梱の
  `gosmopolitan` を使う。
- SQLite に 3 列を足す移行 `00016` がある（[data-model.md](data-model.md)）。
- API の変更は追加だけである（`Error.reason`・`Error.limit`・`Error.tagName`・`Video.probeErrorCode`・
  `Scan.errorCode`・`Scan.errorPath`）。既存の `code`・HTTP 状態・状態値は変えない。
- 各 `ui-design.md` が引用する日本語の文言は、英語のカタログが正本になる。

## Constitution Check

- **依存方向**（ARCHITECTURE.md）: 合格。失敗のコードと、それを包む型は `internal/domain` に置き、
  `internal/media`・`internal/scanner` が包み、`internal/app`・`internal/jobs`・`internal/store` は
  `errors.As` で取り出す。adapter 同士の import は増えない。
- **API の正本は `api/openapi.yaml`**（ARCHITECTURE.md、AGENTS.md）: 合格。`reason` と失敗のコードは
  enum として足し、`task generate` で Go と TypeScript を作る。生成物は手で直さない。
- **画面は `web/src/api/` だけがサーバーと話す**（ARCHITECTURE.md「Web layer」）: 合格。`reason` と
  `limit`・`tagName` を `RequestFailed` に載せるのは `client.ts` で、画面の文言は `web/src/i18n/` が作る。
- **索引と利用者データの区別**（ARCHITECTURE.md「Rebuildable and user data」）: 合格。足す列は
  作り直せる `videos` と `scans` にあり、利用者の名前やタグは触らない（要件 8）。
- **制約は検査で守る**（core-beliefs.md）: 合格。訳し漏れ・ロケールの固定・エラーの表示漏れは
  lint と型検査で `task check` が落とす（受け入れ条件 8）。
- **文書は変更と同じ PR で直す**（core-beliefs.md、AGENTS.md）: 合格。i18n の設計文書と索引への
  リンクは基盤の単位、ARCHITECTURE.md の API・データ・Web 層の記述はそれを変える単位が直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/023-english-i18n/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # カタログ、書式、検出、エラーの reason、失敗コード、対象外の決定
├── data-model.md         # videos.probe_error_code、scans.error_code・error_path
├── quickstart.md         # 日本語の残り、失敗の表示、過去の日本語データ、書式の確認
└── contracts/
    └── error-api.md      # Error.reason・limit・tagName、Video.probeErrorCode、Scan.errorCode・errorPath
```

`ui-design.md` は作らない。親 Issue に `ui` ラベルが無く、画面の構成・操作・見た目は変えずに文言を
置き換える。英語で長さが変わる箇所は各単位の視覚確認で扱う。

### Source Code

**Affected boundaries**:

- `api/openapi.yaml`・`internal/httpapi`: `Error.reason`・`limit`・`tagName`、失敗コードの項目、英語の `message`。
- `internal/domain`: 失敗コードと包む型、`NormalizeTagName` の理由付きの誤り、英語の誤りの文。
- `internal/media`・`internal/scanner`・`internal/app`・`internal/jobs`・`internal/store`: 失敗コードの
  付与と保存、英語の誤りの文とログ。
- `internal/artifacts`・`internal/mediafs`・`internal/opener`・`internal/password`・
  `internal/eventbus`・`cmd/mdm`: 英語の誤りの文、ログ、設定とアカウント操作の出力。
- `web/src/i18n`（新規）: カタログ、書式、エラー表示。`web/src/api/client.ts`: `reason`・`limit`・`tagName`。
- `web/src/*` の各領域と `web/e2e`: カタログの利用と英語の検証。`web/index.html`: `lang="en"`。
- `.golangci.yml`・`web/eslint.config.js`: 訳し漏れの検出。
- `ARCHITECTURE.md`・`docs/design-docs/`: i18n の設計文書と索引、API・データ・Web 層の記述。

**New paths**: `web/src/i18n/`、`internal/store/migrations/00016_failure_codes.sql`、
`docs/design-docs/i18n.md`。

**Structure decision**: 画面の文言と書式は `web/src/i18n/` に置き、`web/src/lib/format.ts` の
ロケールに依存する関数（日時、相対時刻）はそこへ移す。ロケールに依存しない関数（長さ、容量、
解像度）は `lib/` に残す。`i18n/` を `lib/` の中に置かない理由は、ESLint の規則が日本語と文字の
リテラルを許す唯一の場所として、ディレクトリ単位で区切れるためである。

## Implementation Work

単位の分け方: サーバーは API（`internal/httpapi` と domain の誤り）、失敗コード（データの流れ）、
残りのパッケージの英語化の 3 つ、画面は基盤と、互いのファイルが重ならない 4 つの領域に分けた。
画面の lint は基盤で有効にして未対応のディレクトリを除外し、各領域が自分の分を外す
（[R-3](research.md#r-3-訳し漏れは-lint-と型で検出する)）。

### API エラーに理由と表示用の値を足し、API の message を英語にする

**Scope**: `api/openapi.yaml` の `Error.reason`・`limit`・`tagName` と `message` の説明
（[contracts/error-api.md §0・§1](contracts/error-api.md)）、`internal/httpapi` のすべての
`message`・ログ・`spa.go` の平文の応答の英語化、表の状況への `reason`・`limit`・`tagName` の付与、
`internal/domain` の誤りの文の英語化と `NormalizeTagName` の理由付きの誤り、Go のテストの期待値、
ARCHITECTURE.md の API の記述。

**Dependencies**: None

**Acceptance**: `internal/httpapi` のテストが、表の各状況で変更前と同じ HTTP 状態・`code` と、
表の `reason`・`limit`・`tagName` を確かめて通る。`gosmopolitan` を一時的に有効にして `internal/httpapi` と
`internal/domain` を検査すると 0 件。`task generate` の差分が生成物だけで、`task check` が通る。

### 解析と取り込みの失敗に理由のコードを保存して API で返す

**Scope**: 移行 `00016` と [data-model.md](data-model.md) の 3 列、`internal/domain` の失敗コードと
包む型、`internal/media`（解析）と `internal/scanner` でのコードの付与と誤りの文・ログの英語化、
`internal/app`・`internal/jobs` での受け渡しと英語化、`internal/store` の書き込みと読み出し、
`Video.probeErrorCode`・`Scan.errorCode`・`Scan.errorPath`
（[contracts/error-api.md §2・§3](contracts/error-api.md)）、ARCHITECTURE.md のデータの記述。

**Dependencies**: API エラーに理由と表示用の値を足し、API の message を英語にする

**Acceptance**: 動画でないファイルの解析失敗で `GET /api/videos/{id}` が
`probeErrorCode: "probe_failed"` と英語の `probeError` を返し、読めないメディアフォルダの取り込みで
`GET /api/scans/current` が `errorCode: "media_folder_unreadable"` とそのパスを `errorPath` で返す
ことを、Go のテストが確かめる。移行前に入れた日本語の `probe_error`・`scans.error` が移行後も同じ
値で残り、コードは `null` であることを store のテストが確かめる。`task check` が通る。

### サーバーの残りの誤りの文・ログ・運用コマンドの出力を英語にし、gosmopolitan を有効にする

**Scope**: `internal/store`・`internal/artifacts`・`internal/mediafs`・`internal/opener`・
`internal/password`・`internal/eventbus`・`cmd/mdm`（設定の検査、起動の失敗、アカウント操作の
使い方と入出力、購読者名）と、前の 2 単位が触らなかった `internal/media`（サムネイル、プレビュー、
シーク用サムネイル）・`internal/jobs`・`internal/app` の残りの文とログ、テストの期待値。
`.golangci.yml` で `gosmopolitan` を `cmd/`・`internal/` の非テストに有効にする
（[R-3](research.md#r-3-訳し漏れは-lint-と型で検出する)）。

**Dependencies**: 解析と取り込みの失敗に理由のコードを保存して API で返す

**Acceptance**: `task lint` の `gosmopolitan` が 0 件で通る。[quickstart.md §2](quickstart.md#2-サーバーの出力が英語であること)
の操作のログに日本語の固定文言が出ない。`task check` が通る。

### 画面の i18n 基盤（文言カタログ・英語の書式・API エラーの表示）を作る

**Scope**: `web/src/i18n/` のカタログの型と英語のカタログ、単数・複数と `Intl` の書式関数、
エラー表示（`reason` → `code` → 未知のコードの `message` → HTTP 状態の概要、通信の失敗）
（[R-1](research.md#r-1-画面文言の置き場は型付きの自前カタログにする)・
[R-2](research.md#r-2-書式と単数複数はブラウザの-intl-で扱う)・
[R-5](research.md#r-5-画面は既知のエラーを-reasoncode-からそれ以外を安全な概要で表示する)）。
`client.ts` の `RequestFailed` の `reason`・`limit`・`tagName`、`UiText` 型と `ui/` の部品の文言の props、疑似ロケールと、描画した画面の文言がすべてカタログ由来か利用者のデータであることを確かめるテストの helper、`lib/format.ts` のロケール依存の関数の移動、
`web/src/api/`・`web/src/lib/`・`web/src/ui/`・`web/src/app/`・`main.tsx` の文言、`index.html` の `lang="en"`。ESLint の規則と
未対応のディレクトリの除外の一覧（[R-3](research.md#r-3-訳し漏れは-lint-と型で検出する)）。
`docs/design-docs/i18n.md` と索引へのリンク、各 `ui-design.md` の文言はカタログが正本である旨の
索引の注記、ARCHITECTURE.md の Web 層の記述。

**Dependencies**: 解析と取り込みの失敗に理由のコードを保存して API で返す

**Acceptance**: Vitest が、すべての `ErrorCode`・`ErrorReason`・`ProbeErrorCode`・`ScanErrorCode` に
英語の文があること（型検査）、`limit` の埋め込み、未知のコード・空の本文・JSON でない本文・
`fetch` の失敗の表示、1 と複数の件数、英語の日付・日時・相対時刻、疑似ロケールの helper が
`string` の変数を経由した英語のリテラルを描画した部品を検出することを確かめて通る。`web/src/api/`・`lib/`・`ui/`・`app/` に
`i18n/` の外の日本語や固定の文字が無いことを ESLint が確かめる。`task check` が通る。共通部品（コンボボックス、
トースト）の文言が変わるので、視覚と支援技術の確認を行う。

### 初回設定・ログイン・上部の枠・設定画面を英語にする

**Scope**: `web/src/auth/`・`web/src/shell/`・`web/src/settings/` の文言・読み上げ名・書式を
カタログへ移す。取り込みの失敗は `Scan.errorCode`・`errorPath` から表示し、`Scan.error` は出さない。
メディアフォルダ・フォルダ選択・ログインの失敗は API エラーの表示を使う。各ディレクトリの Vitest と
`web/e2e` の `auth`・`guest`（該当部分）・`settings`・`scan-progress`・`unconfigured` の期待値、
ESLint の除外の一覧からこの 3 ディレクトリを外す。

**Dependencies**: 画面の i18n 基盤（文言カタログ・英語の書式・API エラーの表示）を作る

**Acceptance**: 除外を外した ESLint が通る。通常・空・処理中・失敗の状態を疑似ロケールで描画したテストが、カタログ外の文言が無いことを確かめて通る。初回設定・ログイン（誤ったパスワード、試行の制限）・
設定画面（存在しない・重なるメディアフォルダ、取り込みの失敗、`errorCode` の無い過去の失敗）で
英語の説明が出ることを Vitest と `task test-e2e` が確かめる。画面が変わるので視覚と支援技術の確認を
行う。

### ライブラリ・フォルダ画面と共通の一覧を英語にする

**Scope**: `web/src/library/`・`web/src/videoList/`・`web/src/folders/` の文言・読み上げ名・
件数と日時の書式・検索の説明・選択と一括操作・グループのカードをカタログへ移す。各ディレクトリの
Vitest と `web/e2e` の `search`・`folders`・`guest`（該当部分）・`hover-preview` の期待値、ESLint の
除外の一覧からこの 3 ディレクトリを外す。

**Dependencies**: 画面の i18n 基盤（文言カタログ・英語の書式・API エラーの表示）を作る

**Acceptance**: 除外を外した ESLint が通る。通常・空・処理中・失敗の状態を疑似ロケールで描画したテストが、カタログ外の文言が無いことを確かめて通る。1 本と複数本の件数、空状態、読み込み失敗、検索語の
長さと一括操作の上限（`limit` の埋め込み）の表示と、日本語の名前の動画・フォルダが元の名前のまま
表示・検索できることを Vitest と `task test-e2e` が確かめる。画面が変わるので視覚と支援技術の
確認を行う。

### タグ管理画面を英語にする

**Scope**: `web/src/tags/` の文言・読み上げ名・件数をカタログへ移し、タグ名の不正・競合・統合の
失敗を API エラーの表示（`tag_name_*`・`merge_same_tag`・`tag_name_taken`・`tag_merge_required`）で
出す。Vitest と `web/e2e/tags.e2e.ts` の期待値、ESLint の除外の一覧から `tags/` を外す。

**Dependencies**: 画面の i18n 基盤（文言カタログ・英語の書式・API エラーの表示）を作る

**Acceptance**: 除外を外した ESLint が通る。通常・空・処理中・失敗の状態を疑似ロケールで描画したテストが、カタログ外の文言が無いことを確かめて通る。空のタグ名・長すぎるタグ名（`limit` の埋め込み）・
使用中の名前・別のタグのシノニムの名前・統合が要る名前（どれも競合先のタグ名を含む）・同じタグの統合で英語の具体的な説明が出ることと、日本語のタグ名と
同義語がそのまま表示・検索できることを Vitest と `task test-e2e` が確かめる。画面が変わるので視覚と
支援技術の確認を行う。

### 再生画面とプレイヤーを英語にする

**Scope**: `web/src/player/` の文言・読み上げ名・日付の書式、video.js の独自言語をカタログから作る
こと（[R-9](research.md#r-9-videojs-の文言はカタログから作る)）、解析失敗を `probeErrorCode` から
表示し `probeError` を出さないこと、再生・ファイル操作の失敗の API エラーの表示。Vitest と
`web/e2e/playback.e2e.ts` の期待値、ESLint の除外の一覧から `player/` を外し、一覧を無くす。

**Dependencies**: 画面の i18n 基盤（文言カタログ・英語の書式・API エラーの表示）を作る

**Acceptance**: 除外の一覧の無い ESLint が通る。通常・空・処理中・失敗の状態を疑似ロケールで描画したテストが、カタログ外の文言が無いことを確かめて通る。操作バーのボタンの読み上げ名とツールチップが英語で
キーを添えること、`probeErrorCode` ごとの説明、コードの無い過去の日本語の `probeError` が画面に出ず
英語の概要が出ること、ファイルが無いときの「開く」の失敗、キーボードショートカットが変わらない
ことを Vitest と `task test-e2e` が確かめる。画面が変わるので視覚と支援技術の確認を行う。
