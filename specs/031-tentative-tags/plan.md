# Implementation Plan: 仮のタグ: 自動で付けるときに新しく作られたタグを、確定・却下できるようにする

**Branch**: `feature/031-tentative-tags` | **Parent Issue**: #589

**Input**: The parent Issue. It is this feature's specification.

## Summary

外部連携 API と MCP の一括のタグ付けに `tentative` を足し、そのとき新しく生まれたタグを「仮のタグ」として
区別する。利用者は管理画面で仮のタグを見分け・絞り込み、確定・却下・統合で片付ける。却下した名前は
覚えておき、以後の仮の作成で飛ばす。

- **保存**: 仮かどうかは `tags.tentative` の 1 列、却下した名前は名前だけの表 `rejected_tag_names`。
  付け外し・絞り込み・検索・本数の SQL は変えない（[research.md R-1](research.md#r-1-仮かどうかは-tags-の-1-列で持つ)、
  [R-2](research.md#r-2-却下した名前は名前だけの表-rejected_tag_names-に置きタグの名前と同じ完全一致で引く)、
  [data-model.md](data-model.md)）。
- **規則**: 名前を `tag_names` に書く取引は同じ名前を却下の一覧から外す。仮のタグへの改名・シノニム・
  統合先は同じ取引で確定にする。却下は仮のタグにだけ効く（[R-3](research.md#r-3-名前を-tag_names-に書く取引は同じ名前を-rejected_tag_names-から外す)、
  [R-4](research.md#r-4-仮のタグへの手入れ改名シノニム統合先は同じ取引で確定にする)、
  [R-5](research.md#r-5-却下するは仮のタグにだけ効き確定したタグには-409-tag_not_tentative-で何もしない)、
  [R-6](research.md#r-6-却下と同じ名前の仮の付与はsqlite-の書き込みの直列化に任せる)）。
- **API**: タグを表す応答に `tentative`。画面は仮のタグに 2 つの `POST`、却下した名前に `GET`・`DELETE`
  （[contracts/screen-api.md](contracts/screen-api.md)、[R-9](research.md#r-9-画面の-api-は仮のタグに-2-つの-post却下した名前に-get-と-delete-を足す)）。
  外部連携 API は本文に `tentative`、応答に `skippedTags`。MCP は同じ型から入る
  （[contracts/external-api.md](contracts/external-api.md)、[R-7](research.md#r-7-外部連携-api-は-tentative-を要求の-1-項目飛ばした名前を応答の-1-項目として足す)）。
- **画面**: 管理画面の行の見分けと絞り込み、確定・却下・統合の操作、却下した名前の一覧。ライブラリのカードと
  再生画面のチップの見分け。`ui` ラベルがあるので、見た目と操作は次の design 段階の `ui-design.md` が
  親 Issue の「UI品質」を基準に決める（[R-8](research.md#r-8-仮のタグだけの絞り込みと却下した名前の一覧は画面の側で持つ)）。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・索引と利用者データの区分・役割の型の規則・認証の境界:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- タグの表・名前の規則・書き換えの規則・絞り込み・検索・本数:
  [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
  [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)、
  [internal/domain/tag.go](../../internal/domain/tag.go)、
  [internal/store/tags.go](../../internal/store/tags.go)、
  [internal/store/tag_lookup.go](../../internal/store/tag_lookup.go)、
  [internal/store/tag_synonyms.go](../../internal/store/tag_synonyms.go)、
  [internal/store/tag_listing.go](../../internal/store/tag_listing.go)、
  [internal/store/video_tags.go](../../internal/store/video_tags.go)、
  [internal/store/invariants_test.go](../../internal/store/invariants_test.go)
- フォルダ由来のタグとグループのタグ化:
  [specs/017-folder-groups/data-model.md §4](../017-folder-groups/data-model.md#4-フォルダ由来のタグ)、
  [internal/store/folder_tags.go](../../internal/store/folder_tags.go)、
  [internal/store/folder_groups.go](../../internal/store/folder_groups.go)
- ゲストへの応答: [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)
- 外部連携 API の一括のタグ付けと MCP: [api/external-v1.yaml](../../api/external-v1.yaml)、
  [specs/026-external-api/contracts/external-api.md §4](../026-external-api/contracts/external-api.md#4-動画のタグ)、
  [specs/026-external-api/contracts/mcp.md](../026-external-api/contracts/mcp.md)、
  [internal/domain/external_video_tags.go](../../internal/domain/external_video_tags.go)、
  [internal/store/external_video_tags.go](../../internal/store/external_video_tags.go)、
  [internal/httpapi/external_video_tags.go](../../internal/httpapi/external_video_tags.go)、
  [internal/httpapi/mcp.go](../../internal/httpapi/mcp.go)、
  [docs/how-to/external-api.md](../../docs/how-to/external-api.md)
- 画面の API と誤りの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [internal/httpapi/tags.go](../../internal/httpapi/tags.go)、
  [internal/httpapi/router.go](../../internal/httpapi/router.go)（`Tags` interface）、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)
- 画面: [specs/014-video-tags/ui-design.md](../014-video-tags/ui-design.md)（Tag chip、Library card、
  Video page tags、Tag management page）、[web/src/tags/](../../web/src/tags/)、
  [web/src/api/tags.ts](../../web/src/api/tags.ts)（共有の一覧）、
  [web/src/library/CardTagRow.tsx](../../web/src/library/CardTagRow.tsx)、
  [web/src/player/VideoTags.tsx](../../web/src/player/VideoTags.tsx)、
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)、
  [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- 移行は 1 つ（`00022_tentative_tags.sql`: `tags.tentative` の列と `rejected_tag_names` の表）。
  `tag_names`・`video_tags` は変えない。
- Go・npm とも依存は足さない。ドメインイベントは足さない（タグの変更は副作用を持たない）。
- `SearchKeyVersion` は上げない。却下した名前は照合用の鍵を持たない。
- `quickstart.md` は作らない。受け入れ条件は store・httpapi・Vitest の試験で確かめられ、リポジトリの
  検査の外で実行する手順はない。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: `TagRef.Tentative`・`Tag.Tentative`、`ErrTagNotTentative`、`VideoTagsOutcome`。
    純粋な値だけ。
  - `internal/store`: 移行、`TagStore` の操作の追加と変更、不変条件。
  - `internal/httpapi`: 要求の解釈、応答への変換、外部連携 API と MCP。規則は domain・store に置く。
  - `internal/app`・`cmd/mdm`: 触らない（タグの操作は今と同じく `internal/app` を通さない）。
- **役割の型は他の役割の公開メソッドを呼ばない**（ARCHITECTURE.md `store.DB` の段落）: 合格。却下した名前の
  削除は `insertTagName` の中にあり、`FolderGroupStore` のタグ化はパッケージ内の `findOrCreateTag` を
  今のまま使う（data-model.md §5）。
- **索引と利用者データの区別**: 合格。`tags.tentative` と `rejected_tag_names` は利用者データで、
  ARCHITECTURE.md の一覧に足す（data-model.md §1）。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml`・`api/external-v1.yaml` を直して
  `task generate`。外部連携 API は項目の追加だけ（026 の互換の方針）。
- **ゲストは所有者のデータを見ない**（guest-api.md）: 合格。ゲストの `tags` は空の配列のまま、
  新しい経路はすべて所有者だけ（要件 4）。
- **サーバーの出力は英語、画面の文言はカタログ**（`.golangci.yml` の gosmopolitan、i18n.md）: 合格。
- **設計文書は今どうなっているかを書く**（docs/design-docs/index.md「設計文書の方針」）: 合格。各単位が
  ARCHITECTURE.md・`docs/how-to/external-api.md`・`api/*.yaml` の説明の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/031-tentative-tags/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-9
├── data-model.md         # tags.tentative、rejected_tag_names、書き換えの規則、domain の値、保存層の操作
└── contracts/
    ├── screen-api.md     # 画面の API: スキーマの差分、4 つの経路、変わる既存の経路
    └── external-api.md   # /api/v1/video-tags の tentative と skippedTags、Tag の差分、MCP
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。`quickstart.md` は作らない（Technical Context）。

### Source Code

**Affected boundaries**:

- `internal/domain`（`TagRef`・`Tag` の項目、誤りの値、一括操作の結果）、`internal/store`（移行、
  `TagStore`、不変条件の検査）
- `internal/httpapi`（`tags.go` の 4 経路と `toAPITag`・`toAPITagRef`、`external_video_tags.go`、
  `mcp.go` の説明、`router.go` の `Tags` interface）、`api/openapi.yaml`・`api/external-v1.yaml` と生成物
- `web/src/api`（型と 4 つの関数、共有の一覧）、`web/src/tags`（管理画面）、`web/src/library`・
  `web/src/player`（チップ）、`web/src/i18n`
- `ARCHITECTURE.md`、`docs/how-to/external-api.md`

**New paths**:

- `internal/store/migrations/00022_tentative_tags.sql`、`internal/store/rejected_tag_names.go`
- `web/src/tags/` の絞り込み・仮のタグの操作・却下した名前の一覧の部品（名前は design 段階に従う）

**Structure decision**: 仮のタグの操作と却下した名前は `TagStore` に置き、新しい役割の型を作らない。
却下は「タグの削除 + 名前の記憶」の 1 つの取引で、仮の作成は「名前の引き当て + 却下した名前の照合 +
作成」の 1 つの取引であり、どちらも `tags`・`tag_names` と同じ取引で読み書きする必要がある。
別の役割にすると、役割の型は他の役割の公開メソッドを呼ばない規則の下で、その取引を組めない。

## Implementation Work

### 仮のタグと却下した名前を保存し、タグの読み出しに仮かどうかを載せる

**Scope**: `00022_tentative_tags.sql`、`domain` の値（[data-model.md §4](data-model.md#4-domain-に足す値)）、
`TagStore` の新しい操作と既存の操作の変更（[§3・§5](data-model.md#3-書き換えの規則)）: `ApplyVideoTags` の
`tentative` と飛ばした名前、`ConfirmTag`・`RejectTag`・`ListRejectedTagNames`・`ForgetRejectedTagName`、
改名・シノニム・統合先での確定、名前を書く入口での却下した名前の削除、`tentative` を載せる読み出し。
`invariants_test.go` の 2 つの不変条件（[§1](data-model.md#1-マイグレーション)）。`httpapi.Tags` interface
の追加と、それを満たすためのハンドラの最小の追従（`ApplyVideoTags` の署名）。ARCHITECTURE.md の利用者
データの一覧と `TagStore` の段落。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、`tentative: true` の `add` が無い名前を
`tentative = 1` で 1 つだけ作って全対象に付け、既存の確定したタグの名前・シノニムを付けても状態が変わらない
（受け入れ条件 1・3、Edge Case「同じ新しい名前が複数の動画」）。`tentative` が偽なら確定したタグとして作る
（受け入れ条件 2）。却下したタグの名前を `tentative: true` で付けると、その名前だけが飛ばした名前として
返り、残りの名前は付き、`replace` では置き換え後の集合に入らず、すべてが却下した名前なら空で置き換える
（受け入れ条件 9、Edge Case）。`RejectTag` でタグと付与が消え、名前が一覧に入り、確定したタグには
`ErrTagNotTentative` で何も変わらない（受け入れ条件 8）。`ConfirmTag` のあと付いている動画が変わらない
（受け入れ条件 7）。仮のタグの改名・シノニム登録・統合先で `tentative = 0` になり、同じ名前への改名では
変わらない（受け入れ条件 11）。却下した名前を `CreateTag`・`RenameTag`・`AddSynonym`・`AttachTagByName`・
`tentative` が偽の `ApplyVideoTags`・グループのタグ化で使うと一覧から消える（受け入れ条件 14）。
`ForgetRejectedTagName` のあと同じ名前が再び仮のタグとして作られ、無い名前を外しても誤りにならない
（受け入れ条件 13、Edge Case）。移行のあと既存のタグがすべて `tentative = 0` で、却下した名前の一覧が
空である（Edge Case「既存データの移行」）。`ListTags`・`TagsByContentKeys`・`Summary`・
`AttachTagByID` の結果に `Tentative` が載る。

### 画面の API で仮かどうかを返し、確定・却下・却下した名前の操作を足す

**Scope**: `api/openapi.yaml` の `TagRef`・`VideoTag`・`Tag` の `tentative`、`RejectedTagNameList`、
`tag_not_tentative`、4 つの経路と生成物（[contracts/screen-api.md §0〜§3](contracts/screen-api.md#0-スキーマの差分)）。
`internal/httpapi/tags.go` のハンドラ、`accessRoutes`・`requiresJSONBody`・`openapi_routes_test.go`
（`/api/tags/rejected-names` が `{id}` に取られない）。`web/src/api` の型と 4 つの関数、`errorText` の
文言（[§4](contracts/screen-api.md#4-websrcapi-の関数)）と、生成された型に `tentative` が必須になることへの
Vitest の fixture の追従。

**Dependencies**: 仮のタグと却下した名前を保存し、タグの読み出しに仮かどうかを載せる

**Acceptance**: `task check` が通る。httpapi の試験で、`GET /api/tags`・`GET /api/videos/{id}` の `tags`・
`POST /api/video-tags` の `tag`・`POST /api/video-tags/summary` の `items[].tag` に `tentative` が出る
（要件 5）。`POST /api/tags/{id}/confirm` が `200` と `tentative: false` の `Tag` を返し、確定済みでも
`200`。`POST /api/tags/{id}/reject` が `204` で、そのあと `GET /api/tags` に無く、動画の `tags` から
消え、`GET /api/tags/rejected-names` に名前が出る（受け入れ条件 8）。確定したタグへの `reject` は
`409 tag_not_tentative` で何も変わらない。`DELETE /api/tags/rejected-names?name=…` が `204` で一覧から
消え、無い名前でも `204`。`PATCH`・`synonyms`・`merge` の応答で仮のタグが `tentative: false` になる
（受け入れ条件 11）。ゲストは 4 経路とも `401`、ゲストの `Video.tags` は空のまま（受け入れ条件 4）。

### 外部連携 API と MCP で仮に付け、飛ばした名前を返す

**Scope**: `api/external-v1.yaml` の `VideoTagsRequest.tentative`、`VideoTagsResponse.skippedTags`、
`Tag`・`ExternalVideoTag` の `tentative` と生成物、`internal/httpapi/external_video_tags.go`、`mcp.go` の
`update_video_tags` の説明（[contracts/external-api.md](contracts/external-api.md)）。
`docs/how-to/external-api.md` の「動画にタグを付ける」とスクレイパーの例。

**Dependencies**: 仮のタグと却下した名前を保存し、タグの読み出しに仮かどうかを載せる

**Acceptance**: `task check` が通る。httpapi の試験で、トークン付きの `POST /api/v1/video-tags` に
`tentative: true` で無い名前を送ると `200`、応答の `tags` にその名前が `tentative: true` で出て、
`GET /api/v1/tags` と画面の `GET /api/tags` で仮として出る（受け入れ条件 1）。`tentative` を省くと
確定したタグとして作られ、却下した名前なら一覧から消える（受け入れ条件 2・15）。却下した名前と新しい
名前を一緒に送ると `200` で、新しい名前だけが付き、`skippedTags` に却下した名前が 1 回出る
（受け入れ条件 9）。`remove` では `skippedTags` が空。`tentative` に真偽値でない値を送ると `400`。
MCP の `update_video_tags` で同じ結果になり、`list_tags` の出力に `tentative` が出る（受け入れ条件 15）。

### 管理画面で仮のタグを見分けて絞り込み、確定・却下・統合し、却下した名前を片付ける

**Scope**: `web/src/tags/` の、行の仮の印、仮のタグだけの絞り込み、仮のタグの行の「確定する」・
「却下する」・「別のタグへ統合…」（「削除…」は出さない）、却下の確認、却下した名前の一覧と取り外し。
`tag_not_tentative` を `tag_not_found` と同じ取り直しにする。英語のカタログの文言。見た目と操作は
`ui-design.md` に従う（[R-8](research.md#r-8-仮のタグだけの絞り込みと却下した名前の一覧は画面の側で持つ)）。

**Dependencies**: 画面の API で仮かどうかを返し、確定・却下・却下した名前の操作を足す

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、仮の
タグの行が確定したタグと見分けられ（受け入れ条件 5）、絞り込みで仮のタグだけが並び（受け入れ条件 6）、
確定でその行が確定したタグになり（受け入れ条件 7）、却下で行が消えて却下した名前の一覧に出る
（受け入れ条件 8）。仮のタグの行に「削除」が無く「却下する」があり、確定したタグの行は今と同じ
（受け入れ条件 12）。却下した名前を一覧から外せる（受け入れ条件 13）。`tag_not_tentative` で窓が閉じ
一覧を取り直す（Edge Case「操作の競合」）。「新しいタグ」で却下した名前を作ると一覧から消える
（受け入れ条件 14）。

### ライブラリのカードと再生画面のチップで仮のタグを見分ける

**Scope**: `web/src/library/CardTagRow.tsx`（グループのカードを含む）と `web/src/player/VideoTags.tsx` の
チップの仮の印、名前で付けた応答の `tentative` の反映。英語のカタログの文言。見た目は `ui-design.md`
に従う（UI 品質「視覚的階層」）。

**Dependencies**: 画面の API で仮かどうかを返し、確定・却下・却下した名前の操作を足す

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、
`tentative: true` のタグのチップが確定したタグと見分けられ、名前は主のままである（受け入れ条件 5）。
確定したタグのチップは今と同じ。仮のタグのチップを押した絞り込みと、× での取り外しが今と同じに働く
（受け入れ条件 4）。ゲストの画面にはタグが出ない。
