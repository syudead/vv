# Implementation Plan: タグ管理画面を、タグが数千個あっても固まらず、並べ替え・絞り込み・まとめての操作で片付けられるようにする

**Branch**: `feature/036-tag-admin-scale` | **Parent Issue**: #651

**Input**: The parent Issue. It is this feature's specification.

## Summary

タグ管理画面（`/tags`）を、タグが数千個あっても開いてすぐ一覧が出て、検索・スクロール・行の操作で
固まらないようにし、並び順（名前・本数・作った日）と 0 本のタグの絞り込み、複数の行を選んでの
まとめての確定・却下・削除・統合を足す。却下した名前と、検索・絞り込み・並び順・まとめての操作には、
一覧をスクロールしても手が届くようにする。

- **描画**: サーバーの一覧はページングせず、画面が全件を持ったまま、見えている行だけを描く
  （[research.md R-1](research.md#r-1-一覧は-get-apitags-の全件を今までどおり-1-回で受け画面の側で見えている行だけを描く)、
  [R-2](research.md#r-2-行の仮想化は-tanstackreact-virtual-の-usewindowvirtualizer-で行う)）。
- **検索**: `domain.FoldForMatch` を TypeScript に移植し、同じ入力の組で両方を検査する
  （[R-3](research.md#r-3-一覧の検索は-domainfoldformatch-を-typescript-に移植して照合し同じ入力の組で両方を検査する)）。
- **まとめての操作**: `POST /api/tags/batch`（確定・却下・削除）が 1 つの取引で受け、働かない・無いタグは
  数えて飛ばす。統合は `POST /api/tags/{id}/merge` の本文を `sourceIds` にして 1 件も複数も同じ経路にする。
  確認の「影響を受ける動画の本数」は `POST /api/tags/impact` が重複を除いて数える
  （[R-4](research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かないないタグは数えて飛ばす)、
  [R-5](research.md#r-5-統合は-post-apitagsidmerge-の本文を-sourceids1-件以上にし1-件の統合もこれを使う)、
  [R-6](research.md#r-6-確認に出す影響を受ける動画の本数は-post-apitagsimpact-が重複を除いて数える)、
  [contracts/screen-api.md](contracts/screen-api.md)）。
- **並び順と絞り込み**: 画面の側で並べ替え・絞り込み、並び順だけを端末の設定に残す。「作った日」は今ある
  `tags.created_at` を `Tag.createdAt` として載せる
  （[R-7](research.md#r-7-並び順はこの画面の端末の設定として-localstorage-に持ち絞り込みは今までどおり画面の状態に留める)、
  [R-8](research.md#r-8-作った日は-tagscreated_at-を-tagcreatedat-として載せ同じ秒のタグは名前の順にする)）。
- **計測**: 規模のデータを作る `scripts/tagsbench` と Playwright の計測スクリプトで、受け入れ条件の数値を
  本番ビルドで測る（[R-9](research.md#r-9-受け入れ条件の計測は作り置きの規模のデータを-scriptstagsbench-が作りplaywright-の計測スクリプトが本番ビルドに対して測る)、
  [quickstart.md](quickstart.md)）。
- `ui` ラベルがあるので、並び順・絞り込みの操作の形（ライブラリのツールバー「表示と並び替え」にそろえる）、
  行の選択とまとめての操作の帯、確認の窓の文言、却下した名前への入口、スクロール中も届くツールバーの
  形、行の操作の文字の出し方（タッチの端末）、統合の窓の入力の幅は、次の design 段階の `ui-design.md` が
  親 Issue の「UI品質」を基準に決める。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・役割の型の規則・ドメインイベント・認証の境界・web 層の分け方:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- タグの表・名前の規則・本数の数え方・検索欄での照合: [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
  [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)、
  [internal/store/tags.go](../../internal/store/tags.go)・[tag_listing.go](../../internal/store/tag_listing.go)・
  [tag_lookup.go](../../internal/store/tag_lookup.go)・[folder_tags.go](../../internal/store/folder_tags.go)（`taggedVideosSQL`）、
  [internal/domain/tag.go](../../internal/domain/tag.go)、[internal/domain/search.go](../../internal/domain/search.go)（`FoldForMatch`）
- 仮のタグと却下した名前: [specs/031-tentative-tags/data-model.md](../031-tentative-tags/data-model.md)、
  [specs/031-tentative-tags/contracts/screen-api.md](../031-tentative-tags/contracts/screen-api.md)、
  [specs/031-tentative-tags/research.md](../031-tentative-tags/research.md)、
  [internal/store/tentative_tags.go](../../internal/store/tentative_tags.go)
- 画面の API と変換: [api/openapi.yaml](../../api/openapi.yaml)、[internal/httpapi/tags.go](../../internal/httpapi/tags.go)、
  [internal/httpapi/router.go](../../internal/httpapi/router.go)（`Tags`）、
  [internal/httpapi/video_tags.go](../../internal/httpapi/video_tags.go)（`json_each` に渡す id の集合の前例）
- 画面: [web/src/tags/](../../web/src/tags/)、[web/src/api/tags.ts](../../web/src/api/tags.ts)（共有の保持）、
  [web/src/api/tagOrder.ts](../../web/src/api/tagOrder.ts)（`compareTagRefs`）、
  [web/src/preferences/viewPreferences.ts](../../web/src/preferences/viewPreferences.ts)（端末の設定の形）、
  [web/src/videoList/SortControls.tsx](../../web/src/videoList/SortControls.tsx)・[FilterMenu.tsx](../../web/src/videoList/FilterMenu.tsx)
  （ライブラリの「表示と並び替え」）、[web/src/library/SelectionBar.tsx](../../web/src/library/SelectionBar.tsx)（一括操作の帯と上限の扱い）、
  [web/src/ui/Combobox.tsx](../../web/src/ui/Combobox.tsx)（`frameClassName`）、[web/src/i18n/en.ts](../../web/src/i18n/en.ts)
- 画面の見た目の規則と仮想スクロールの判断: [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
  （§3・§6）、[specs/014-video-tags/ui-design.md](../014-video-tags/ui-design.md)「Tag management page」、
  [specs/031-tentative-tags/ui-design.md](../031-tentative-tags/ui-design.md)「Tag management page」
- 計測の前例: [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md)、[scripts/previewbench](../../scripts/previewbench/)
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`・`task test-e2e`）

**Feature-specific context**:

- 移行は無い。`tags.created_at` は既にあり、新しい表も列も要らない（[data-model.md](data-model.md)）。
- npm の依存を 1 つ足す: `@tanstack/react-virtual`（R-2）。Go の依存は足さない。ドメインイベントと
  `/api/events` の種類は足さない（タグの変更は副作用を持たない。今のまま）。
- 性能の予算は親 Issue の受け入れ条件 1〜3（最初の行まで 1 秒、操作後の固まりが 0.2 秒以内、
  スクロール中に 50ms を超えるフレームが続かない）で、規模はタグ 3,000 個・動画 30,000 本まで。
  `task check` では確かめられないので、[quickstart.md](quickstart.md) の手順で測る。
- 外部連携 API（`api/external-v1.yaml`）と MCP は変えない。`listTags` の応答に `createdAt` は載せない（R-8）。
- `ui-design.md` は次の design 段階が作る。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。`internal/domain` は値と純関数
  `TagBatchApplies`。`internal/store` は `TagStore` の 3 操作と `created_at` の読み。`internal/httpapi` は経路の
  追加と変換。`internal/app`・`cmd/mdm` は触らない。`scripts/tagsbench` は `scripts/previewbench` と同じく
  `internal/` の公開の操作だけを呼ぶ。
- **役割の型は他の役割の公開メソッドを呼ばない、SQL は `internal/store` の中**: 合格。新しい操作はすべて
  `TagStore` で、`taggedVideosSQL`・`mergeTagInto`・`json_each` の前例を使う。計測の道具は役割の型の
  公開の操作で行を書き、SQL を持たない（R-9）。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml` を直して `task generate`。新しい経路は
  所有者だけの既定の分類に入り、`openapi_routes_test.go` が `security` と突き合わせる。`/api/tags/batch`・
  `/api/tags/impact` が `{id}` に取られないことの検査を足す。
- **仮想スクロールを使わない判断**（library-ui.md §3）: 判断の前提（折り返す格子、60 件ずつ読む）が
  この一覧には当たらず、同じ節が求める計測が親 Issue にある。§3 をタグ管理画面の一覧について書き直す
  （R-2）。格子の一覧は変えない。
- **依存の追加**: `@tanstack/react-virtual` を足す。実行時の依存を持たず、Renovate の運用に乗る。
  自前の窓切りを採らない理由は R-2。
- **サーバーの出力は英語、画面の文言はカタログ**（`.golangci.yml` の gosmopolitan、i18n.md）: 合格。
  新しい文言はすべて `web/src/i18n/en.ts`。
- **ゲストは所有者のデータを見ない**: 合格。`/tags` は所有者だけの画面で、新しい経路も所有者だけ。
- **設計文書は今どうなっているかを書く**（docs/design-docs/index.md）: 合格。各単位が ARCHITECTURE.md
  （`TagStore` の段落、`web/src/tags/` の段落）、`docs/design-docs/library-ui.md` §3、
  `docs/how-to/README.md` の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/036-tag-admin-scale/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-9
├── data-model.md         # domain の値、TagStore の操作、画面の側の状態の規則（移行なし）
├── quickstart.md         # 規模のデータで受け入れ条件 1〜3 を測る手順と期待
└── contracts/
    └── screen-api.md     # Tag.createdAt、POST /api/tags/batch、merge の変更、POST /api/tags/impact
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。

### Source Code

**Affected boundaries**:

- `internal/domain`（`Tag.CreatedAt`、`TagBatchAction`・`TagBatchOutcome`・`TagMergeOutcome`・`TagImpact`、
  `TagBatchApplies`、`MaxTagBatch`）
- `internal/store`（`TagStore.BatchTags`・`MergeTags`・`TagImpact`、`listCanonicalTags`・`tagByID` の
  `created_at`、`MergeTag` の削除、不変条件の試験）
- `internal/httpapi`（`tags.go` の 2 経路と `MergeTag` の本文・応答、`toAPITag` の `createdAt`、`router.go` の
  `Tags`、`openapi_routes_test.go`）、`api/openapi.yaml` と生成物
- `web/src/api`（`tags.ts` の `batchTags`・`mergeTag`・`tagImpact`）、`web/src/lib`（`foldForMatch.ts`）、
  `web/src/preferences`（`tagListPreferences.ts`）、`web/src/tags`（一覧の仮想化、並び順・絞り込み、選択と
  まとめての操作、統合の窓、ツールバーと却下した名前の置き場所）、`web/src/i18n`、`web/package.json`
- `scripts/tagsbench`、`web/bench/`、`docs/how-to/tags-admin-benchmark.md`
- `ARCHITECTURE.md`、`docs/design-docs/library-ui.md`、`docs/how-to/README.md`

**New paths**:

- `internal/domain/testdata/fold_for_match.json`（Go と Vitest が共に読む照合形の組。R-3）
- `web/src/lib/foldForMatch.ts`、`web/src/preferences/tagListPreferences.ts`、
  `web/src/tags/tagListOrder.ts`（並び順の比較。名前は `ui-design.md` に従って変えてよい）
- `scripts/tagsbench/main.go`、`web/bench/tags-admin.bench.ts`、`web/bench/playwright.config.ts`、
  `docs/how-to/tags-admin-benchmark.md`

**Structure decision**: まとめての操作は `TagStore` に置き、新しい役割の型は作らない。確定・却下・削除・統合は
今ある 1 件の取引の繰り返しで、どれも `tags`・`tag_names`・`video_tags`・`rejected_tag_names` の中で
完結する（R-4・R-5）。画面は `TagsPage` が全件・絞り込み・並び替え・選択を持ち、描く行の決定だけを仮想化に
任せる（[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。照合形の移植は `web/src/lib/`（locale に
依存しない整形の置き場）に置き、`web/src/api/` には置かない（サーバーを呼ばない純関数のため）。計測の
道具は製品のコードに入れず、`scripts/` と `web/bench/` に置く（R-9）。

## Implementation Work

### `GET /api/tags` の応答に `createdAt` を載せる

**Scope**: `domain.Tag.CreatedAt`、`listCanonicalTags`・`tagByID` の `created_at` の読み、`api/openapi.yaml` の
`Tag.createdAt` と生成物、`toAPITag`（[contracts/screen-api.md §0](contracts/screen-api.md#0-スキーマの差分)、
[data-model.md §1](data-model.md#1-domain-に足す値)）。外部連携 API の `listTags` は変えない（R-8）。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、作ったタグの `CreatedAt` が作成時刻（秒）と一致し、
`ListTags` の全件と `RenameTag`・`ConfirmTag`・`AddSynonym` の戻り値に載る。httpapi の試験で、
`GET /api/tags` の各件と `POST /api/tags` の応答に `createdAt`（RFC 3339）が入り、
`GET /api/v1/tags` の応答には入らない。生成物の検査が通る。

### まとめての確定・却下・削除の `POST /api/tags/batch` と確認用の `POST /api/tags/impact` を足す

**Scope**: `domain` の `TagBatchAction`・`TagBatchOutcome`・`TagImpact`・`TagBatchApplies`・`MaxTagBatch`、
`TagStore.BatchTags`・`TagImpact`（[data-model.md §1・§2](data-model.md#1-domain-に足す値)）、
`api/openapi.yaml` の 2 経路・4 スキーマ・`too_many_tags` と生成物、`internal/httpapi/tags.go` の経路、
`router.go` の `Tags`、`openapi_routes_test.go`（[contracts/screen-api.md §1・§3](contracts/screen-api.md#1-post-apitagsbatch)）、
`web/src/api/tags.ts` の `batchTags`・`tagImpact`・`maxTagBatch`、`errorText` の `too_many_tags`
（[§4](contracts/screen-api.md#4-websrcapi-の関数)）。ARCHITECTURE.md の `TagStore` の段落。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、仮 3 個・確定 2 個・無い id 1 個を `confirm` すると
`AppliedIDs` が仮の 3 個、`NotApplicableIDs` が確定の 2 個、`NotFoundIDs` が 1 個で、3 個の `tentative` が
偽になる; `reject` で仮のタグが消えて元の名前が `rejected_tag_names` に入り、確定したタグは残る; `delete`
で確定したタグが消え、仮のタグは残る; どの操作のあとも 031 の不変条件が通る。`TagImpact` で、同じ動画に
付いた 2 つのタグを渡すと `VideoCount` が 1（重複なし）、フォルダ名からだけ付いている動画も数え、
ライブラリに無い動画は数えない（受け入れ条件 11）。httpapi の試験で、`POST /api/tags/batch` が 3 つの配列を
`ids` の順で返し、`ids` が空と 20,001 件は `400`（後者は `too_many_tags` と `limit`）、`action` が 3 値以外は
`400`; `POST /api/tags/impact` が `tagCount`・`videoCount` を返す; 2 経路が `{id}` に取られない; ゲストは `401`。
生成物の検査が通る。

### `POST /api/tags/{id}/merge` を複数の統合元を受ける形にする

**Scope**: `domain.TagMergeOutcome`、`TagStore.MergeTags` と `MergeTag` の削除（[data-model.md §2](data-model.md#2-保存層の操作)）、
`api/openapi.yaml` の `MergeTagRequest.sourceIds`・`TagMergeResponse` と生成物、`internal/httpapi/tags.go`、
`router.go` の `Tags`（[contracts/screen-api.md §2](contracts/screen-api.md#2-post-apitagsidmerge-の変更)）、
`web/src/api/tags.ts` の `mergeTag(id, sourceIds)` と、1 件の統合の呼び手 `MergeTagDialog`・`TagsPage.performMerge`
の応答の読み替え（`tag` を使い、`notFoundIds` が空でなければ今の `tag_not_found` と同じ扱い）、
`web/e2e/tags.e2e.ts` が統合の API を直接呼んでいればその本文。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、統合元 3 個（うち 1 個は無い id）を統合すると、残り
2 個の付与（重複は 1 本）・元の名前・シノニムが統合先に移り、2 個が消え、統合先が確定になり、
`NotFoundIDs` が 1 個; 統合先が無ければ `ErrTagNotFound`。httpapi の試験で、`{ sourceIds: [a, b] }` が
`{ tag, notFoundIds }` を返し、`sourceIds` に `{id}` を含むと `400 merge_same_tag`、空は `400`、統合先が無いと
`404 tag_not_found`、統合元が全部無いと `200` で `tag` は変わらない。Vitest で、行の「別のタグへ統合…」が
`sourceIds: [source.id]` を送り、応答の `tag` で一覧が差し替わる（今の試験が通り続ける）。

### タグ管理画面の検索を照合形（`FoldForMatch`）で照らす

**Scope**: `web/src/lib/foldForMatch.ts`、`internal/domain/testdata/fold_for_match.json` と、それを読む Go の試験
（`FoldForMatch`）と Vitest の試験（`foldForMatch`）、`TagsPage` の `matchesFilters` を照合形の部分一致に変え、
タグごとの照合形をタグの配列から記憶する（[research.md R-3](research.md#r-3-一覧の検索は-domainfoldformatch-を-typescript-に移植して照合し同じ入力の組で両方を検査する)、
[data-model.md §4](data-model.md#4-画面の側で持つ状態)「検索」）。候補（combobox）の照合は変えない。

**Dependencies**: None

**Acceptance**: `task check` が通る。共有の組に、全角・半角（`ＡＣＴＩＯＮ` と `action`）、ひらがな・カタカナ
（`あくしょん` と `アクション`）、NFD と NFC、大文字小文字が入り、Go と Vitest の両方で同じ結果になる。
Vitest で、`ＡＣＴＩＯＮ` の検索で `action` のタグが見つかり（受け入れ条件 8）、シノニムでも見つかり、
一致しない語で「No tags match」が出る。

### 規模のデータを作って測る道具を足す

**Scope**: `scripts/tagsbench`（規模のデータを `.local/tagsbench/<規模>/` に役割の型で書き、ビルド済みの
バイナリをそのデータで起動して計測スクリプトを走らせる）、`web/bench/tags-admin.bench.ts` と
`web/bench/playwright.config.ts`（[quickstart.md](quickstart.md) の場面を測り、表で出す。`task test-e2e` と CI
には入れない）、`docs/how-to/tags-admin-benchmark.md` と `docs/how-to/README.md` の索引
（[research.md R-9](research.md#r-9-受け入れ条件の計測は作り置きの規模のデータを-scriptstagsbench-が作りplaywright-の計測スクリプトが本番ビルドに対して測る)）。
`tsconfig.e2e.json` と ESLint の対象に `web/bench/` を入れる。

**Dependencies**: None

**Acceptance**: `task check` と `task check-docs` が通る。`go run ./scripts/tagsbench -scale 1000` が
タグ 1,000 個（約 90 個が 0 本、半数が仮）・動画 10,000 本のデータを作り、起動した本番ビルドの
`GET /api/tags` が 1,000 件を返し、計測の表（quickstart.md の 7 場面）が出る。`-scale 3000` も同じ。
変更前の `main` で測った値を PR の本文に表で残し、親 Issue の表と同じ傾向（1,000 個で最初の行まで
数秒）が出ることを確かめる。

### タグ管理画面の一覧を見えている行だけ描くようにする

**Scope**: `web/package.json` に `@tanstack/react-virtual`、`TagsPage` の行の描画を `useWindowVirtualizer` に
載せ替え（`measureElement` で行の高さを測る。作成の行・改名中の行・シノニムの行を含む）、`TagRow` を
`React.memo` にして行の props を安定させる、`focusRow`・`focusAfterRemoval` が描かれていない行へ移すときに
先にその位置へスクロールする、1 件の確定・改名・削除・却下・統合のあとの反映を今のまま保つ
（[research.md R-1・R-2](research.md#r-1-一覧は-get-apitags-の全件を今までどおり-1-回で受け画面の側で見えている行だけを描く)、
[data-model.md §4](data-model.md#4-画面の側で持つ状態)「描く行」）。`docs/design-docs/library-ui.md` §3 と
ARCHITECTURE.md の `web/src/tags/` の段落。

**Dependencies**: `規模のデータを作って測る道具を足す`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る。見た目は変えない）。`task check` が通る。
Vitest で、今の `TagsPage.test.tsx` が通り続ける（jsdom では表示域の高さが無いので、仮想化が全行を
描く設定、または試験用の高さの指定で行う）。描かれていない行の確定・削除のあとフォーカスが次の行へ
移る。`go run ./scripts/tagsbench` の 2 つの規模で、最初の行まで 1 秒以内、検索の 1 文字目・Esc・
1 件の確定・改名の後の最長タスクが 0.2 秒以内、スクロール中に 50ms を超えるフレームが続かない
（受け入れ条件 1〜3）。変更前後の表を PR の本文に残す。

### タグ管理画面に並び順と 0 本の絞り込みを足し、並び順を端末に残す

**Scope**: `web/src/tags/tagListOrder.ts`（5 値の比較、同値は `compareTagRefs`）、
`web/src/preferences/tagListPreferences.ts`（並び順だけを保存する総関数）、`TagsPage` のツールバーに
並び順と「0 本のみ」の操作（形は `ui-design.md` に従い、ライブラリの「表示と並び替え」にそろえる）、
件数の行の「〈見えている数〉 of 〈全体〉」、改名中の行を並び順・絞り込みの変更で消さない
（[research.md R-7・R-8](research.md#r-7-並び順はこの画面の端末の設定として-localstorage-に持ち絞り込みは今までどおり画面の状態に留める)、
[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。英語のカタログの文言。

**Dependencies**: `GET /api/tags の応答に createdAt を載せる`、`タグ管理画面の一覧を見えている行だけ描くようにする`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、「本数」の
多い順で最多のタグが先頭・0 本が末尾（受け入れ条件 4）、本数が同じなら名前の順（Edge Case）;
「作った日」の新しい順で作成したタグが先頭に出る（受け入れ条件 5）; 並び順を変えて `TagsPage` を
unmount・mount し直すと同じ並び順で開き、`localStorage` が壊れていれば名前の順（受け入れ条件 6、
Edge Case）; 「0 本のみ」で件数の行が「〈0 本の数〉 of 〈全体〉」になり、出る行の本数がすべて 0 で、
「Tentative only」と検索と並び順と組み合わさる（受け入れ条件 7、要件 5）; 改名中の行は並び順・絞り込みを
変えても残り入力中の値を失わない（Edge Case）。

### タグ管理画面で複数の行を選び、まとめて確定・却下・削除する

**Scope**: `TagsPage` の選択（`Set<number>`、見えなくなった行の選択を外す、「見えているものをすべて選ぶ」）、
行のチェックとまとめての操作の帯（形は `ui-design.md`「Selection」に従う。選んでいる間だけ前に出る）、
まとめての確定（確認なし）、却下・削除の確認（`tagImpact` の数を出し、届くまで実行を押せない）、応答の
反映（`appliedIds` を選択から外し一覧へその場で反映、`notApplicableIds` の数をトーストで伝え、
`notFoundIds` があれば取り直す）、却下のあとの却下した名前の取り直し、上限（`maxTagBatch`）での disabled
（[data-model.md §4](data-model.md#4-画面の側で持つ状態)「選択」「まとめての操作の結果」、
[contracts/screen-api.md §1・§3](contracts/screen-api.md#1-post-apitagsbatch)）。英語のカタログの文言。
ARCHITECTURE.md の `web/src/tags/` の段落。

**Dependencies**: `まとめての確定・却下・削除の POST /api/tags/batch と確認用の POST /api/tags/impact を足す`、
`タグ管理画面の一覧を見えている行だけ描くようにする`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、「Tentative only」
で見えているタグをすべて選んで確定すると `POST /api/tags/batch` に `confirm` と全 id が 1 回送られ、応答の
あとすべての行から仮の目印が消え、選択が空になる（受け入れ条件 9）; 仮と確定を混ぜて削除すると確認に
`tagImpact` の `tagCount` と `videoCount` が出て（受け入れ条件 11）、実行後に `notApplicableIds` の数が
トーストに出る（Edge Case）; `notFoundIds` があると一覧を取り直す; 失敗すると選択が残る; 検索を変えて
見えなくなった行の選択が外れる（Edge Case）; 見えている行が上限を超えるとまとめての操作が押せない。
`tagsbench` の規模で、見えている仮のタグ全部の確定で最長タスクが 0.2 秒以内（受け入れ条件 9）。

### 選んだタグをまとめて 1 つのタグへ統合し、統合の窓の入力を窓の幅に合わせる

**Scope**: まとめての操作の帯からの「統合…」（`MergeTagDialog` を複数の統合元で開く。統合先は選んだ中からも
選んでいないタグからも選べ、統合先を選んだ中から選んだときは統合元から外し、統合元が無くなれば実行できない。
確認に `tagImpact` の数を出す）、応答の反映（統合元を取り除き、統合先を `tag` に差し替え、`notFoundIds` が
あれば取り直す）、統合の窓の Combobox に `frameClassName="w-full"`（今は既定の `w-40`。幅の規則は
`ui-design.md`「Merge dialog」に従う。要件 13）
（[contracts/screen-api.md §2](contracts/screen-api.md#2-post-apitagsidmerge-の変更)、
[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。英語のカタログの文言。

**Dependencies**: `POST /api/tags/{id}/merge を複数の統合元を受ける形にする`、
`タグ管理画面で複数の行を選び、まとめて確定・却下・削除する`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、4 個を選んで
「Action」へ統合すると `sourceIds` が 4 個で送られ、応答のあと 4 個が一覧から消え「Action」の本数が応答の
`videoCount` になる（受け入れ条件 10）; 統合先を選んだ中から選ぶと `sourceIds` からその id が外れ、統合元が
統合先だけなら実行が押せない（Edge Case）; 確認に `tagCount`・`videoCount` が出る（要件 10）; 窓の中の
統合先の入力が窓の内側の幅いっぱいに広がる（要件 13、1 件の統合でも同じ）。

### スクロール中もツールバーと却下した名前に届くようにする

**Scope**: 検索・絞り込み・並び順・まとめての操作の帯を、一覧をスクロールしても画面に残す（文書のスクロールは
変えず、`position: sticky` で上部バーの下に留める。形と高さは `ui-design.md`「Toolbar」に従う）、却下した
名前の一覧への入口を一覧の上から届く位置に置く（置き場所と開き方は `ui-design.md`「Rejected names」に
従う。中身・取り外し・取り直しの規則は 031 のまま）、仮想化のスクロール位置の計算に留めた帯の高さを
入れる。`docs/design-docs/library-ui.md` の該当箇所。英語のカタログの文言。

**Dependencies**: `タグ管理画面の一覧を見えている行だけ描くようにする`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、却下した名前の
入口が一覧の先頭より上（ツールバー側）にあり、押すと今と同じ一覧と取り外しが開く（受け入れ条件 12）。
`tagsbench` の 1,000 個の規模で、`/tags` を開いた直後にスクロールせず却下した名前の一覧を開け、一覧の
末尾までスクロールしても検索・絞り込み・並び順・まとめての操作が表示域に残る（要件 12）。1280×800 で
12 行以上が一画面に見える（親 Issue「UI品質」情報密度）。
