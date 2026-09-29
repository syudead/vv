# Implementation Plan: 外部連携 API と MCP（API トークンによる自動タグ付け）

**Branch**: `feature/026-external-api` | **Parent Issue**: #493

**Input**: The parent Issue. It is this feature's specification.

## Summary

オーナーが設定ページで発行する API トークン（Bearer）で、外部ツールが「新しい動画を見つける →
タグを付ける」を人手なしで回せるようにする。同じ操作を `/mcp` の MCP サーバーでも出す。

- **トークン**: `api_tokens` 表に SHA-256 だけを置き、`account.version` の一致とアカウント変更での
  全件削除で失効させる（[research.md R-1](research.md#r-1-トークンは接頭辞付きの-256-ビットの乱数にしsha-256-だけを保存する)、
  [R-2](research.md#r-2-アカウントの変更はトークンの行を消し版の一致でも確かめる)）。管理は画面の API で
  Cookie の所有者だけ（[contracts/token-api.md](contracts/token-api.md)）。
- **境界**: `/api/v1/` と `/mcp` を Bearer だけの分類にし、それ以外の `/api/*` は今どおり Cookie だけ
  （[R-3](research.md#r-3-外部連携-api-は-apiv1-の下に置き境界にbearerの分類を足す)、
  [R-4](research.md#r-4-bearer-の要求には同一オリジンの検査をかけない)）。
- **外部連携 API v1**: 別の OpenAPI 文書 `api/external-v1.yaml` を正本にし
  （[R-5](research.md#r-5-外部連携-api-の契約は別の-openapi-の文書にしgo-だけを生成する)）、
  追加順の keyset のカーソルで動画の一覧を読ませ（変更の追跡はしない。[R-6](research.md#r-6-動画の一覧は-added_at-id-の-keyset-のカーソルで読む)）、
  名前でタグを一括で付け外しさせる（[R-7](research.md#r-7-タグの操作は厳格な一括操作として-tagstore-に足す)、
  [contracts/external-api.md](contracts/external-api.md)）。
- **MCP**: 公式の Go SDK を stateless で使い、REST と同じ関数を呼ぶツールにする
  （[R-8](research.md#r-8-mcp-は公式の-go-sdk-を-stateless-で-internalhttpapi-の中に置く)、
  [contracts/mcp.md](contracts/mcp.md)）。
- **画面**: 設定ページの「API トークン」節。`ui` ラベルがあるので、見た目と操作は次の design 段階の
  `ui-design.md` が決める。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・認証の境界・データの区分: [ARCHITECTURE.md](../../ARCHITECTURE.md)、
  [.golangci.yml](../../.golangci.yml)（depguard）
- 認証の今の作り: [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)、
  [contracts/auth-api.md](../016-single-account-auth/contracts/auth-api.md)、
  [contracts/account-cli.md](../016-single-account-auth/contracts/account-cli.md)、
  [internal/httpapi/auth.go](../../internal/httpapi/auth.go)、[internal/app/auth.go](../../internal/app/auth.go)、
  [internal/store/auth.go](../../internal/store/auth.go)
- 画面の API とエラーの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)、
  [internal/httpapi/router.go](../../internal/httpapi/router.go)（`mutationBoundary`・`requiresJSONBody`）
- タグ: [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
  [internal/store/video_tags.go](../../internal/store/video_tags.go)、[internal/domain/tag.go](../../internal/domain/tag.go)
- 動画の一覧と所在の書き込み: [internal/store/listing.go](../../internal/store/listing.go)、
  [internal/store/scan_index.go](../../internal/store/scan_index.go)、
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go)
- スキャン: [internal/app/scans.go](../../internal/app/scans.go)
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）、
  [scripts/generate/main.go](../../scripts/generate/main.go)、[api/oapi-codegen.yaml](../../api/oapi-codegen.yaml)
- 画面の文言: [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)

**Feature-specific context**:

- Go の依存を 1 つ足す: `github.com/modelcontextprotocol/go-sdk`（R-8）。npm の依存は足さない。
- 移行は 1 つ（`api_tokens`）。番号は実装の時点の `internal/store/migrations` の次を使う
  （今の最後は `00019_scan_issues.sql`）。定義は [data-model.md](data-model.md)。`videos` には列を足さない。
- 生成物が 1 つ増える: `internal/httpapi/extgen/`（手で直さない。AGENTS.md の一覧に足す）。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: トークンの名前の規則、`APIToken`・`VideoRef`・一覧の問い合わせの値。純粋な値と関数だけ。
  - `internal/store`: `api_tokens` の読み書き、外部連携 API の動画の一覧と引き当て、`TagStore.ApplyVideoTags`。
  - `internal/app`: `Auth` にトークンの発行・一覧・失効・確認を足す（乱数はセッションと同じ作り）。
  - `internal/httpapi`: 境界の分類、画面の API のトークン管理、外部連携 API、MCP。要求の解釈と変換だけで、
    規則は domain・store・app に置く。
  - `cmd/mdm`: 配線と `mdm account` の出力だけ。兄弟のパッケージ同士の import は増やさない
    （MCP を別パッケージにしない、R-8）。
- **API の正本と生成物**（ARCHITECTURE.md、AGENTS.md）: 合格。画面の API は `api/openapi.yaml`、外部連携
  API は `api/external-v1.yaml` を正本にし、どちらも `task generate` で生成し、`generate-check` が差分を見る
  （R-5）。この「正本が 2 つ」は ARCHITECTURE.md の「単一の正本」の記述を、画面と外部の 2 つの境界それぞれに
  1 つずつと書き直す。
- **認証の境界**（ARCHITECTURE.md の認証の段落）: 合格。分類は今と同じく経路だけで決め、
  `openapi_routes_test.go` と同じ種類の試験で、外部連携 API のすべての操作が `bearerAuth` で、境界が Bearer に
  分類することを確かめる。画面の API の分類と挙動は変えない（要件 5）。
- **索引と利用者データの区別**（ARCHITECTURE.md「Rebuildable and user data」）: 合格。`api_tokens` は設定の
  データとして一覧に足す（data-model.md §1）。索引の表は変えない。
- **ドメインイベント**: 該当なし。トークンの操作とタグの操作はイベントを出さない（今の `AuthStore`・`TagStore`
  と同じ）。
- **サーバーの出力は英語**（`.golangci.yml` の gosmopolitan、023）: 合格。`message`・ログ・理由のコードは英語で、
  画面の文言は `web/src/i18n/` のカタログが持つ。
- **文書は変更と同じ PR で直す**（core-beliefs.md、AGENTS.md）: 合格。各単位が ARCHITECTURE.md と
  `docs/how-to/external-api.md` の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/026-external-api/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-10
├── data-model.md         # api_tokens、domain の値
├── quickstart.md         # 受け入れ条件を動くサーバーで確かめる手順
└── contracts/
    ├── token-api.md      # 画面の API のトークン管理と mdm account の出力
    ├── external-api.md   # /api/v1 の操作
    └── mcp.md            # /mcp のツール
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。

### Source Code

**Affected boundaries**:

- `internal/domain`・`internal/store`（移行・`AuthStore`・`TagStore`・動画の一覧の読み出し）・
  `internal/app`（`Auth`）
- `internal/httpapi`（`auth.go` の分類と台帳、`router.go` の `mutationBoundary`、画面の API、外部連携 API、MCP）
- `api/openapi.yaml`・`scripts/generate`・`cmd/mdm`（配線、`account.go` の出力）
- `web/src/api`・`web/src/settings`・`web/src/i18n`
- `ARCHITECTURE.md`・`AGENTS.md`（生成物の一覧）・`docs/how-to/`

**New paths**:

- `api/external-v1.yaml`・`api/oapi-codegen-external.yaml`・`internal/httpapi/extgen/`（生成物）
- `internal/httpapi/api_tokens.go`・`internal/httpapi/external_*.go`・`internal/httpapi/mcp.go`
- `internal/domain/api_token.go`・`internal/store/api_tokens.go`・`internal/store/external_videos.go`
- `web/src/settings/APITokensSection.tsx`（名前は design 段階に従う）
- `docs/how-to/external-api.md`（トークンの使い方、互換の方針、スクレイパーからの連携例、MCP の接続例。
  `docs/how-to/README.md` から案内する）

**Structure decision**: 外部連携 API と MCP は、画面の API と同じ `internal/httpapi` の中に、別の生成パッケージと
別のハンドラの型（`externalServer`）で置く。生成された `ServerInterface` の名前が画面の API とぶつからないように
するためで、境界と応答への変換は共有する（R-8 の却下した代案も参照）。

## Implementation Work

### API トークンの発行・一覧・失効（画面の API と保存）

**Scope**: `api_tokens` の移行と `AuthStore`・`app.Auth` の操作（[data-model.md §1](data-model.md#1-api_tokensr-1r-2r-9r-10)）、
名前の規則（R-10）、画面の API の 3 つの操作と `mdm account` の出力（[contracts/token-api.md](contracts/token-api.md)）、
`api/openapi.yaml` と生成物、ARCHITECTURE.md のデータの区分と `AuthStore` の段落。Bearer の確かめ方は
`app.Auth` の操作として用意するが、境界にはまだつながない。

**Dependencies**: None

**Acceptance**: `task check` が通る。Cookie の所有者で `POST /api/api-tokens` すると `201` で `secret` が返り、
`GET /api/api-tokens` には `secret` もハッシュも無い。Cookie 無しでは 3 つとも `401`。名前が空・101 文字は `400` と
各 `reason`。`mdm account set-password` の後、一覧が空になる。

### 設定ページの「API トークン」節

**Scope**: 設定ページに節を足し、発行・一度だけの平文の表示とコピー・一覧・確認付きの失効を行う。英語の
カタログ（`web/src/i18n/en.ts`）の文言、新しい `reason` の文言（`web/src/i18n/errors.ts`）。見た目と操作は `ui-design.md` に従う。

**Dependencies**: API トークンの発行・一覧・失効（画面の API と保存）

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。Vitest の試験が通り、設定ページで発行した平文が一度だけ
出て、再読み込み後は名前と日時だけが残る（受け入れ条件 1）。失効は確認の後に一覧から消える。

### 外部連携 API v1 の Bearer 認証とタグ・スキャンの操作

**Scope**: `api/external-v1.yaml` の土台（共通の誤り、`bearerAuth`）と生成の設定・`scripts/generate`・AGENTS.md の
生成物の一覧（R-5）。境界の Bearer の分類・同一オリジンの検査の除外・最終使用日時・台帳による打ち切り
（R-3・R-4・R-9）。操作は `GET /api/v1/tags` とスキャンの 2 つ（[contracts/external-api.md §3・§5](contracts/external-api.md#3-タグ)）。
`app.Scans.StartScan` に `started` を返させ（`internal/httpapi` の `Scans` インターフェースと画面の `POST /api/scans` も合わせて
直す。画面の応答は変えない）。`docs/how-to/external-api.md` を作り、トークンの使い方と互換の方針を書く。ARCHITECTURE.md の認証の段落。

**Dependencies**: API トークンの発行・一覧・失効（画面の API と保存）

**Acceptance**: `task check` が通る。有効なトークンで `GET /api/v1/tags` が `200`、無効・失効済み・形式違い・Cookie
だけでは `401` と `WWW-Authenticate: Bearer`。Bearer だけの `GET /api/api-tokens` は `401`、`GET /api/videos` は
`X-VV-Audience: guest`。別の `Origin` を付けた `POST /api/v1/scans` が通る。app の試験で `StartScan` の `started` が
最初は真、実行中にもう一度呼ぶと偽になり、httpapi の試験で `POST /api/v1/scans` が最初は `201`、実行中は `200` を返し、
画面の `POST /api/scans` は今どおり `202` のまま。`GET /api/v1/scans/current` が、始めた直後（`finding`）は
`videos`・`settledVideos` とも `null`、対象 0 本で終わった走査は両方 `0` を返す。画面での失効が実行中の Bearer の応答を
打ち切ることを httpapi の試験で確かめる。外部連携 API の全操作が `bearerAuth` で Bearer に分類されることを試験が確かめる。

### 外部連携 API で動画の一覧を読み、1 本を引く

**Scope**: `GET /api/v1/videos` と `GET /api/v1/videos/lookup`（[contracts/external-api.md §2](contracts/external-api.md#2-動画)、
[R-6](research.md#r-6-動画の一覧は-added_at-id-の-keyset-のカーソルで読む)）。一覧は登録フォルダの下に所在を持つ動画を
`(added_at, id)` の昇順で keyset のカーソルで返す読み出し（`internal/store/external_videos.go`。表と列は足さない）。
`docs/how-to/external-api.md` の一覧の読み方（`nextCursor` が空になるまでたどる。新しい動画を知るには一覧を
読み直し、持っている動画が消えたことは `lookup` の `404` で知る）。

**Dependencies**: 外部連携 API v1 の Bearer 認証とタグ・スキャンの操作

**Acceptance**: `task check` が通る。store の試験で、`limit` より多い動画が `nextCursor` をたどると重複無く
`(added_at, id)` の順で全件返り、最後の応答の `nextCursor` が空で、解釈できないカーソルは `domain.ErrInvalidCursor`。
一覧を最後まで読んだ後にスキャンで動画が増え、先頭から読み直すとその動画が含まれる（受け入れ条件 3）。
ページングの途中で動画を消しても続きの要求が失敗しない。登録の下の所在をすべて失い登録外の所在だけで残った
動画は、行が消えた動画と同じく一覧に返らず、`lookup` は `404 video_not_found` を返す。非公開の動画も返る
（受け入れ条件 2）。`lookup` が id・内容キー・パス（NFD の綴りのパスを含む）のそれぞれで同じ動画を返し、
無い指定は `404 video_not_found`。

### 外部連携 API から名前でタグを付与・除去・置き換える

**Scope**: `TagStore.ApplyVideoTags`（R-7）と `POST /api/v1/video-tags`（[contracts/external-api.md §4](contracts/external-api.md#4-動画のタグ)）、
`docs/how-to/external-api.md` のスクレイパーからの連携例（新着を読む → 引く → タグを付ける）。

**Dependencies**: 外部連携 API で動画の一覧を読み、1 本を引く

**Acceptance**: `task check` が通る。パスで指定した動画に無いタグ名とシノニムの名前で `add` すると、タグが作られ、
シノニムは元のタグとして付き、画面の API の動画詳細と `tag` の絞り込みに出る（受け入れ条件 4）。同じ要求の繰り返しは
`200` で状態が変わらない（受け入れ条件 5）。引けない動画を 1 つ含む要求は `404`・`index` 付きで何も反映しない。
名前の誤りと件数の上限は `400` と各 `reason`。`replace` の後、動画の手で付けたタグ（`manual`）がちょうど指定の集合になり、フォルダ由来のタグ（`fromFolder`）は残る。

### `/mcp` で外部連携の操作を MCP のツールとして提供する

**Scope**: Go SDK の追加と `/mcp` のハンドラ（R-8）、6 つのツール（[contracts/mcp.md](contracts/mcp.md)）、
`docs/how-to/external-api.md` の MCP の接続例、ARCHITECTURE.md の MCP の段落。

**Dependencies**: 外部連携 API から名前でタグを付与・除去・置き換える

**Acceptance**: `task check` が通る。httpapi の試験で、SDK のクライアントから `/mcp` に接続して 6 つのツールが並び、
`update_video_tags` の結果が REST の `GET /api/v1/videos/lookup` に出る。トークン無し・無効なトークンでは `401`。
[quickstart.md](quickstart.md) の手順を Claude Code で通し、その結果を PR の本文に残す（受け入れ条件 9 ほか）。
