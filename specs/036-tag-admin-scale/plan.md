# Implementation Plan: タグ管理画面を、タグが数千個あっても固まらず、並べ替え・絞り込み・まとめての操作で片付けられるようにする

**Branch**: `feature/036-tag-admin-scale` | **Parent Issue**: #651

**Input**: The parent Issue. It is this feature's specification.

## Summary

タグ管理画面（`/tags`）を、タグが数千〜数万個あっても開いてすぐ一覧が出て、検索・スクロール・行の
操作で固まらないようにし、並び順（名前・本数・作った日）と 0 本のタグの絞り込み、複数の行を選んでの
まとめての確定・却下・削除・統合を足す。却下した名前と、検索・絞り込み・並び順・まとめての操作には、
一覧をスクロールしても手が届くようにする。

### この改訂で変わること

親 Issue が改訂され（画面は表示に要る分だけを読み込み、スクロールで続きを読む。検索・並び順・
絞り込みは読み込んでいないタグも含めた全部に効く。「すべて選ぶ」は読み込んだ行だけ。規模にタグ
30,000 個が加わり、却下した名前も表示に要る分だけ読む）、改訂前の Plan の R-1「全件を 1 回で受け、
画面で絞り、見えている行だけ描く」がこれと相反する。子 Issue #678〜#687 はすべて feature branch に
merge 済みで、そのうち次はそのまま残る。

- **残る**: `Tag.createdAt`（R-8）、`POST /api/tags/batch`・`POST /api/tags/impact`（R-4・R-6）、
  `POST /api/tags/{id}/merge` の `sourceIds`（R-5）、行の仮想化（R-2）、並び順の端末への保存（R-7）、
  行のチェックと選択バー、統合の窓の幅、上部バーの下に留まる帯と却下した名前の窓、計測の道具
  （R-9）、`FoldForMatch` の移植と共有の検査（R-3。役割は変わる）。
- **置き換える**: 一覧の読み方（全件 → サーバーのページ。[research.md R-1](research.md#r-1-一覧はサーバーのページで受け検索絞り込み並び順はサーバーが全部のタグに掛ける)）、
  検索・絞り込み・並び替えの場所（画面 → サーバー）、件数の行の数（画面の数 → 応答の `total`・
  `totalAll`）、「見えているものをすべて選ぶ」の対象（絞り込み後の全件 → 読み込んだ行）、却下した
  名前の読み方（全件 → ページ。[R-13](research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)）、
  統合の窓の統合先の候補の出どころ（画面の全件 → サーバーの検索。[R-14](research.md#r-14-統合の窓の統合先の候補は-get-apitagsqlimit-で引く)）。
- **足す**: 名前の自然順の鍵 `sort_key` と移行（[R-10](research.md#r-10-名前の自然順の鍵-sort_key-を-tag_names-と-rejected_tag_names-に持ち起動時の鍵の埋め直しで作る)）、
  続きの読み込みと重複・食い違い・失敗の扱い（[R-11](research.md#r-11-続きは画面の末尾に近づいたら-100-件ずつ読みid-で重複を捨て件数が食い違えば知らせて取り直させる)）、
  操作のあとの反映を読み込んだ行の中で行う規則と `NaturalSortKey` の移植（[R-12](research.md#r-12-操作のあとの反映は読み込んだ行の中で行い並びの位置は-naturalsortkey-の移植で決める)）、
  計測の規模 30,000 と場面（R-9）。

`## Implementation Work` は merge 済みの単位を再掲せず、今の feature branch の上に要る差分だけを
単位にする（`plan-to-issues` は既にある子 Issue を飛ばす）。改訂前の単位の記録は子 Issue #678〜#687
と git の履歴にある。

- **一覧**: `GET /api/tags` に `q`・`tentative`・`unused`・`sort`・`cursor`・`limit` を足し、画面は
  100 件ずつ受けてスクロールで続きを読む。`limit` を省いた全件は候補・絞り込みの確かめ・外部連携 API の
  ために残す（R-1、[contracts/screen-api.md §5](contracts/screen-api.md#5-get-apitags-のパラメータ)）。
  名前の順の keyset には `tag_names.sort_key`（`NaturalSortKey`）を足し、起動時の鍵の埋め直しで
  埋める（R-10、[data-model.md §0](data-model.md#0-マイグレーション)）。
- **描画**: 読み込んだ行は末尾まで読めば総数まで増えるので、見えている行だけを描く仮想化は残す
  （[R-2](research.md#r-2-行の仮想化は-tanstackreact-virtual-の-usewindowvirtualizer-で行う)）。
- **検索**: サーバーが `search_key`（`FoldForMatch`）で照らす。画面の移植は、操作したあとの 1 行が今の
  条件にまだ合うかをその場で決めるために使う
  （[R-3](research.md#r-3-照合形-foldformatch-の-typescript-移植は画面が読み込んだ行をその場で判定するために使う)）。
- **操作のあと**: 一覧を取り直さず読み込んだ行の中で書き換え、位置は `NaturalSortKey` の移植で決める。
  共有の保持の取り直しは購読者がいるときだけ（R-12）。
- **まとめての操作**: merge 済みのまま（R-4〜R-6）。対象は読み込んだ行で、上限 20,000 件を超えて
  読んだ一覧では disabled。
- **却下した名前**: `GET /api/tags/rejected-names` をページにし、窓の中で続きを読む（R-13、
  [contracts/screen-api.md §6](contracts/screen-api.md#6-get-apitagsrejected-names-のパラメータ)）。
- **計測**: `scripts/tagsbench` に規模 30,000（動画 30,000 本）と、開いたときの転送量・続きを読み込み
  ながらのスクロールの場面を足す（R-9、[quickstart.md](quickstart.md)）。
- `ui` ラベルがあるので、続きの読み込み中・失敗・食い違いの行、件数の行の文言（読み込んだ行の
  「すべて選ぶ」）、却下した名前の窓の続きの読み込み、統合の窓の候補の読み込み中の見え方は、
  `design` 段階が `ui-design.md` を改訂して決める。今の `ui-design.md` は改訂前の Plan に基づく。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・役割の型の規則・ドメインイベント・認証の境界・web 層の分け方:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- タグの表・名前の規則・本数の数え方・検索欄での照合: [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
  [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)、
  [internal/store/tags.go](../../internal/store/tags.go)・[tag_listing.go](../../internal/store/tag_listing.go)・
  [tag_lookup.go](../../internal/store/tag_lookup.go)・[tag_search_keys.go](../../internal/store/tag_search_keys.go)・
  [folder_tags.go](../../internal/store/folder_tags.go)（`taggedVideosSQL`）、
  [internal/domain/tag.go](../../internal/domain/tag.go)、[internal/domain/search.go](../../internal/domain/search.go)
  （`FoldForMatch`・`NaturalSortKey`・`SearchKeyVersion`）
- 仮のタグと却下した名前: [specs/031-tentative-tags/data-model.md](../031-tentative-tags/data-model.md)、
  [specs/031-tentative-tags/contracts/screen-api.md](../031-tentative-tags/contracts/screen-api.md)、
  [internal/store/tentative_tags.go](../../internal/store/tentative_tags.go)
- keyset のページとカーソル、題名の鍵 `title_key`: [specs/013-library-search/contracts/list-api.md](../013-library-search/contracts/list-api.md)、
  [specs/013-library-search/data-model.md](../013-library-search/data-model.md)（§4・§5）、
  [internal/store/listing.go](../../internal/store/listing.go)（`listOrder`・`encodeCursor`・`decodeCursor`）、
  [internal/store/search_keys.go](../../internal/store/search_keys.go)（起動時の鍵の埋め直し）
- 画面の API と変換: [api/openapi.yaml](../../api/openapi.yaml)、[internal/httpapi/tags.go](../../internal/httpapi/tags.go)、
  [internal/httpapi/router.go](../../internal/httpapi/router.go)（`Tags`）、
  [internal/httpapi/external.go](../../internal/httpapi/external.go)（`listTags` の全件の呼び手）
- 画面: [web/src/tags/](../../web/src/tags/)、[web/src/api/tags.ts](../../web/src/api/tags.ts)（共有の保持）、
  [web/src/api/videosData.ts](../../web/src/api/videosData.ts)（`appendUnique`・`inconsistent`）、
  [web/src/lib/foldForMatch.ts](../../web/src/lib/foldForMatch.ts)、
  [web/src/preferences/tagListPreferences.ts](../../web/src/preferences/tagListPreferences.ts)、
  [web/src/ui/Combobox.tsx](../../web/src/ui/Combobox.tsx)、[web/src/i18n/en.ts](../../web/src/i18n/en.ts)
- 画面の見た目の規則と仮想スクロールの判断: [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
  （§3・§6）、[ui-design.md](ui-design.md)（改訂前の Plan に基づく。`design` が改訂する）
- 計測: [docs/how-to/tags-admin-benchmark.md](../../docs/how-to/tags-admin-benchmark.md)、
  [scripts/tagsbench](../../scripts/tagsbench/)、[web/bench/](../../web/bench/)
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`・`task test-e2e`）

**Feature-specific context**:

- 移行を 1 つ足す（`00030_tag_sort_keys.sql`。[data-model.md §0](data-model.md#0-マイグレーション)）。
  派生の鍵の列だけで、表の意味は変えない。改訂前の「移行は無い」は変わる。
- npm・Go の依存は足さない（`@tanstack/react-virtual` は merge 済み）。ドメインイベントと `/api/events` の
  種類は足さない。
- 性能の予算は親 Issue の受け入れ条件 1〜4（最初の行まで 1 秒、開いたときの受け取る数と転送量が規模で
  変わらない、操作後の固まりが 0.2 秒以内、続きを読みながらのスクロールで 50ms を超えるフレームが
  続かない）で、規模はタグ 30,000 個まで。`task check` では確かめられないので、[quickstart.md](quickstart.md)
  の手順で測る。
- 外部連携 API（`api/external-v1.yaml`）と MCP は変えない。`GET /api/v1/tags` は `TagListQuery{}`（全件）で
  同じ結果を返す。
- `GET /api/tags` の `limit` を省いた全件は、候補・絞り込みの確かめ（`web/src/library/`・`web/src/player/`）の
  ために残す。それらを全件から外すのは親 Issue の「対象外」（#674・#675）。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。`internal/domain` は値
  （`TagSort`・`TagListQuery`・`TagPage`・`RejectedTagNamePage`）と純関数（`NaturalSortKey` は既存）。
  `internal/store` は `TagStore` の `ListTags`・`ListRejectedTagNames` の置き換えと `sort_key` の書き。
  `internal/httpapi` はパラメータの解釈と変換。`internal/app`・`cmd/mdm` は触らない。
- **役割の型は他の役割の公開メソッドを呼ばない、SQL は `internal/store` の中**: 合格。鍵の埋め直しは
  `TagStore.RefreshSearchKeys` の中で、起動の呼び手（`cmd/mdm`）は変わらない。カーソルの包み方は
  `listing.go` のものを共有する。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml` を直して `task generate`。`TagList` と
  `RejectedTagNameList` に `required` の項目を足すのは画面の契約で、呼び手は `web/src/api/tags.ts` だけ。
- **移行の規則**（`task migrations-check`、[014 の data-model.md §1](../014-video-tags/data-model.md#1-マイグレーション) の
  書き方）: 合格。`00030` は列の追加と `search_version` の戻しだけで、Down は列を落とす。鍵は作り直せる
  派生の値で、`SearchKeyVersion` の仕組みに乗る（[013 の data-model.md §5](../013-library-search/data-model.md#5-鍵を作る時点と-search_version)）。
- **仮想スクロールの判断**（library-ui.md §3）: 合格。ページで読んでも読み込んだ行は総数まで増えるので
  判断は変わらない（R-2）。§3 の「全件を 1 回で受けて持っている」の記述を直す。
- **サーバーの出力は英語、画面の文言はカタログ**（gosmopolitan、i18n.md）: 合格。新しい文言はすべて
  `web/src/i18n/en.ts`。
- **ゲストは所有者のデータを見ない**: 合格。`/tags` と新しいパラメータは所有者だけの経路の中。
- **設計文書は今どうなっているかを書く**（docs/design-docs/index.md）: 合格。各単位が ARCHITECTURE.md
  （`TagStore` の段落、`web/src/tags/` の段落）、`docs/design-docs/library-ui.md` §3、
  `docs/how-to/tags-admin-benchmark.md` の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/036-tag-admin-scale/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-14（R-1 は改訂で置き換え、R-10〜R-14 は改訂で足した）
├── data-model.md         # 移行 00030、domain の値、TagStore の操作、画面の側の状態の規則
├── quickstart.md         # 規模のデータで受け入れ条件 1〜4 を測る手順と期待
├── ui-design.md          # design 段階の成果物。改訂前の Plan に基づき、design が改訂する
└── contracts/
    └── screen-api.md     # Tag.createdAt、GET /api/tags のパラメータ、POST /api/tags/batch、merge の変更、
                          # POST /api/tags/impact、GET /api/tags/rejected-names のパラメータ
```

### Source Code

**Affected boundaries**（改訂の差分。merge 済みの境界は子 Issue #678〜#687 のとおり）:

- `internal/domain`（`TagSort`・`TagListQuery`・`TagPage`・`RejectedTagNamePage`・`MaxTagPageLimit`、
  `NaturalSortKey` の共有の検査の組）
- `internal/store`（`00030` の移行、`tag_names`・`rejected_tag_names` の `sort_key` の書き、
  `RefreshSearchKeys` の拡張、`ListTags(query)` と `ListRejectedTagNames(cursor, limit)` への置き換え、
  カーソルの共有、不変条件の試験）
- `internal/httpapi`（`tags.go` の `ListTags`・`ListRejectedTagNames` のパラメータ、`external.go` の呼び方、
  `router.go` の `Tags`）、`api/openapi.yaml` と生成物
- `web/src/api`（`tags.ts` の `listTagPage`・`listRejectedTagNamePage`・`tagPageLimit`、`afterTagChanged`）、
  `web/src/lib`（`naturalSortKey.ts`）、`web/src/tags`（`TagsPage` のページ読みと反映、`RejectedNames` の
  続き、`MergeTagDialog` の候補）、`web/src/i18n`
- `scripts/tagsbench`、`web/bench/`、`docs/how-to/tags-admin-benchmark.md`
- `ARCHITECTURE.md`、`docs/design-docs/library-ui.md`

**New paths**:

- `internal/store/migrations/00030_tag_sort_keys.sql`
- `internal/domain/testdata/natural_sort_key.json`（Go と Vitest が共に読む鍵の組。R-12）
- `web/src/lib/naturalSortKey.ts`

**Structure decision**: 一覧のページは `TagStore.ListTags` に置き、新しい役割の型は作らない。本数の
集計・絞り込み・並び替え・カーソルは 1 つの問い合わせで済み、`tags`・`tag_names`・`video_tags`・
`video_folder_names` の中で完結する（R-1・R-10）。カーソルの包み方と keyset の条件の組み立ては
`listing.go` のものを共有し、タグの側で書き直さない。画面は `TagsPage` が条件・ページ・選択を持ち、
描く行の決定だけを仮想化に任せる（[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。鍵の移植は
`foldForMatch.ts` と同じ `web/src/lib/` に置く（サーバーを呼ばない純関数）。計測の道具は製品のコードに
入れず、`scripts/` と `web/bench/` に置く（R-9）。

## Implementation Work

merge 済みの単位（#678〜#687）は再掲しない。次の単位は、今の feature branch の上に要る差分である。

### 名前の自然順の鍵 `sort_key` を `tag_names` と `rejected_tag_names` に持たせ、起動時に埋める

**Scope**: `internal/store/migrations/00030_tag_sort_keys.sql`（[data-model.md §0](data-model.md#0-マイグレーション)）、
名前の行を書く操作（`insertTagName`、改名、`RejectTag`・`BatchTags(reject)` の `rejected_tag_names` への
挿入）で `sort_key = domain.NaturalSortKey(name)` を同じ文で書く、`TagStore.RefreshSearchKeys` が
`search_key` と一緒に `sort_key` を埋め、`rejected_tag_names` も埋める（[data-model.md §2](data-model.md#2-保存層の操作)、
[research.md R-10](research.md#r-10-名前の自然順の鍵-sort_key-を-tag_names-と-rejected_tag_names-に持ち起動時の鍵の埋め直しで作る)）、
`invariants_test.go` の不変条件の追加。ARCHITECTURE.md の `TagStore` の段落（鍵の埋め直しが 2 つの鍵を
扱うこと）。一覧の並びはまだ変えない。

**Dependencies**: None

**Acceptance**: `task check`（`migrations-check` を含む）が通る。store の試験で、移行後の既存の行（`sort_key`
が空、`search_version` が 0）が `RefreshSearchKeys` で `NaturalSortKey(name)` になり、戻り値が
`tag_names` と `rejected_tag_names` の書き直した行の合計になる; `CreateTag`・`AddSynonym`・`RenameTag`・
`ApplyVideoTags`（仮のタグの作成）・`RejectTag`・`BatchTags(reject)` のあと、書いた行の `sort_key` が
`NaturalSortKey(name)` と一致し、`search_version` が現在の版である; 統合で `tag_id` を付け替えた行の
`sort_key` は変わらない; 不変条件の検査が、既存の試験のあとと、まとめての操作のあとに通る。

### `GET /api/tags` に検索・絞り込み・並び順・ページを足す

**Scope**: `domain` の `TagSort`・`TagListQuery`・`TagPage`・`MaxTagPageLimit`（[data-model.md §1](data-model.md#1-domain-に足す値)）、
`TagStore.ListTags(ctx, query)` への置き換え（本数の集計の CTE、`search_key` への `instr`、`tentative`・
0 本の絞り込み、5 つの並び順と keyset の条件、`Limit + 1` 件の読みと `NextCursor`、`Total`・`TotalAll`、
`Limit` 0 の全件。[data-model.md §2](data-model.md#2-保存層の操作)、
[research.md R-1](research.md#r-1-一覧はサーバーのページで受け検索絞り込み並び順はサーバーが全部のタグに掛ける)）、
`listing.go` のカーソルの包みの共有、`api/openapi.yaml` の `listTags` のパラメータ・`TagSort`・`TagList` の
`total`・`totalAll`・`nextCursor` と生成物、`internal/httpapi/tags.go` のパラメータの解釈と `400`、
`external.go` の呼び方、`router.go` の `Tags`（[contracts/screen-api.md §0・§5](contracts/screen-api.md#5-get-apitags-のパラメータ)）、
`web/src/api/tags.ts` の `listTagPage`・`tagPageLimit`・`TagSort` の型（`tagListOrder.ts` の `TagListSort` を
生成物の `TagSort` に寄せる。[§4](contracts/screen-api.md#4-websrcapi-の関数)）。画面はまだ変えない。

**Dependencies**: `名前の自然順の鍵 sort_key を tag_names と rejected_tag_names に持たせ、起動時に埋める`

**Acceptance**: `task check` が通る。store の試験で、タグ 250 個（名前に `tag 2`・`tag 10`・全角・かなを
含む）を `Limit` 100 で 3 ページ読むと、重複も抜けも無く全件が `sort_key, id` の順で並び、3 ページ目の
`NextCursor` が空で、各ページの `Total`・`TotalAll` が 250; `Search` に `ＡＣＴＩＯＮ` を渡すと名前が
`action` のタグとシノニムに `Action Movie` を持つタグが当たり `Total` がその数（受け入れ条件 9）;
`UnusedOnly` で `Total` が 0 本のタグの数、`Items` の本数がすべて 0、`TentativeOnly`・`Search` と
組み合わさる（受け入れ条件 8、要件 6）; `countDesc` で最多のタグが先頭・0 本が末尾、同数は名前の順
（受け入れ条件 5）; `createdDesc` で新しく作ったタグが先頭（受け入れ条件 6）; 本数と作った日の並びで
ページの境目の前後が同じ値のタグでも重複も抜けも無い; 別の並び順のカーソルは `ErrInvalidCursor`;
`Limit` 0 で全件が返り `NextCursor` が空; `ListTags` の既存の試験（本数・シノニム・`CreatedAt`）が
`TagListQuery{}` で通り続ける。httpapi の試験で、`GET /api/tags?limit=100&sort=countDesc` が `items` 100 件と
`total`・`totalAll`・`nextCursor` を返し、`cursor` で続きが読め、`limit=0`・`limit=201`・`sort=foo`・壊れた
`cursor`・101 文字の `q` は `400 invalid_request`、パラメータ無しは全件で `nextCursor` 無し、
`GET /api/v1/tags` の応答は変わらない; ゲストは `401`。生成物の検査が通る。

### `GET /api/tags/rejected-names` をページにし、件数を返す

**Scope**: `domain.RejectedTagNamePage`、`TagStore.ListRejectedTagNames(ctx, cursor, limit)` への置き換え
（`sort_key, name` の順、`Total`。[data-model.md §2](data-model.md#2-保存層の操作)）、`api/openapi.yaml` の
`listRejectedTagNames` の `cursor`・`limit` と `RejectedTagNameList` の `total`・`nextCursor` と生成物、
`internal/httpapi/tags.go`、`router.go` の `Tags`（[contracts/screen-api.md §6](contracts/screen-api.md#6-get-apitagsrejected-names-のパラメータ)、
[research.md R-13](research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)）、
`web/src/api/tags.ts` の `listRejectedTagNamePage`（今の `listRejectedTagNames` の呼び手 `TagsPage` は
`items` だけを使う形で通し、窓の続きは別の単位）。

**Dependencies**: `名前の自然順の鍵 sort_key を tag_names と rejected_tag_names に持たせ、起動時に埋める`

**Acceptance**: `task check` が通る。store の試験で、却下した名前 250 個を `limit` 100 で 3 ページ読むと
名前の自然順（`name 2` が `name 10` の前）で重複も抜けも無く、`Total` が 250、3 ページ目の `NextCursor` が
空; × で外した（`ForgetRejectedTagName`）あとの `Total` が減る。httpapi の試験で、
`GET /api/tags/rejected-names?limit=2` が `items` 2 件・`total`・`nextCursor` を返し、`cursor` で続きが
読め、`limit=0`・壊れた `cursor` は `400`、パラメータ無しは 100 件まで; `DELETE` の既存の試験が通り
続ける。Vitest で、今の `TagsPage.test.tsx` の却下した名前の試験が通り続ける。

### `NaturalSortKey` を TypeScript に移植する

**Scope**: `web/src/lib/naturalSortKey.ts`（`foldForMatch` の上に、ASCII の数字の連続を「先頭の 0 を除いた
桁数を 10 進 4 桁で表した接頭辞 + 数字」に置き換える。`internal/domain/search.go` の `NaturalSortKey` と
同じ手順）、`compareNaturalSortKeys(a, b)`（符号位置の順で比べる。UTF-16 のコード単位では比べない）、
`internal/domain/testdata/natural_sort_key.json` と、それを読む Go の試験（`NaturalSortKey`）と Vitest の
試験（[research.md R-12](research.md#r-12-操作のあとの反映は読み込んだ行の中で行い並びの位置は-naturalsortkey-の移植で決める)「移植の検査」。
`fold_for_match.json` と同じ要領）。

**Dependencies**: None

**Acceptance**: `task check` が通る。共有の組に、`tag 2` と `tag 10`（`2` → `00012`、`10` → `000210`）、
`0`・`00`（→ `0000`）、数字と文字の混在、全角の数字（NFKC で半角になる）、かな、サロゲートペアを含む
名前が入り、Go と Vitest の両方で同じ鍵になる。Vitest で、鍵の順が Go の `strings.Compare` の順
（共有の組に並べた順）と一致し、`compareNaturalSortKeys` が U+FFFF より大きい符号位置を U+E000〜U+FFFF
の後ろに置く（`<` では前に来る組を試験に入れる）。

### タグ管理画面の一覧を、条件ごとにサーバーから読み、スクロールで続きを読む

**Scope**: `TagsPage` を共有の保持（`getTags`・`subscribeTags`・`currentTags`）から外し、条件（検索語・
「Tentative only」・「Unused only」・並び順）が変わるたびに `listTagPage` で先頭のページを読み直す
（進行中の要求の打ち切り、世代の番号、選択を空にする、届くまで前の行を残す）、仮想化が描く最後の
行が末尾に近づいたら `nextCursor` で続きを足す（`id` の重複を捨てる、同時に 1 つ、失敗の行と「Retry」、
`totalAll` の食い違いの 1 行と「取り直す」）、件数の行を `total`・`totalAll` で出す、先頭のチェックを
「読み込んだものをすべて選ぶ」にする、1 件とまとめての操作のあとの反映を読み込んだ行の中で行う
（`naturalSortKey` と並び順の値で位置を決め、`foldForMatch`・`tentative`・`videoCount` で条件に合うかを
決め、`total`・`totalAll` を局所で増減する）、改名中の行を条件の変更で消さない、`afterTagChanged` を
購読者がいるときだけ取り直す形にする、`sortTags`（画面の並べ替え）と `matchesFilters` の全件への適用を
外す（[data-model.md §4](data-model.md#4-画面の側で持つ状態)、
[research.md R-1・R-3・R-11・R-12](research.md#r-11-続きは画面の末尾に近づいたら-100-件ずつ読みid-で重複を捨て件数が食い違えば知らせて取り直させる)）。
英語のカタログの文言（続きの読み込み中・失敗・食い違い、読み込んだ行の「すべて選ぶ」。形は改訂した
`ui-design.md` に従う）。ARCHITECTURE.md の `web/src/tags/` の段落、`docs/design-docs/library-ui.md` §3 の
「全件を 1 回で受けて持っている」の記述。

**Dependencies**: `GET /api/tags に検索・絞り込み・並び順・ページを足す`、`NaturalSortKey を TypeScript に移植する`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、開くと
`GET /api/tags` が `limit=100&sort=name` で 1 回だけ送られ、`getTags` の全件は送られない; 検索に
`ＡＣＴＩＯＮ` を入れると `q=ＡＣＴＩＯＮ` で先頭から読み直され、応答の行だけが並ぶ（受け入れ条件 9）;
「Unused only」で `unused=true` が送られ、件数の行が「〈total〉 of 〈totalAll〉」になる（受け入れ条件 8）;
並び順を変えると `sort=countDesc` で読み直され、選択が空になる（Edge Case）; 描く範囲が末尾に近づくと
`cursor` 付きの要求が 1 回送られ、応答の行が末尾に足され、重複する `id` は捨てられる; 続きの応答の
`totalAll` が違えば「一覧が変わった」の行が出て続きが止まり、「取り直す」で先頭から読み直す; 続きの
失敗で読み込んだ行が残り「Retry」で同じ `cursor` が送られる（Edge Case）; 続きを待つ間に検索を変えると
古い応答は捨てられる（Edge Case）; 先頭のチェックが読み込んだ行だけを選び、`nextCursor` があっても
それ以上は選ばない（要件 10）; 「Tentative only」で読み込んだ仮のタグをすべて選んで確定すると
`POST /api/tags/batch` が 1 回送られ、応答のあと行から仮の目印が消え、一覧の取り直しは送られない
（受け入れ条件 10）; 名前の順で作成したタグが鍵の位置に差し込まれ、`createdDesc` では先頭に出る
（受け入れ条件 6）; 検索中に改名して一致しなくなった行が取り除かれ、`total` が減る; 改名中に並び順を
変えても改名中の行が残り入力中の値を失わない（Edge Case）; 操作のあと `getTags` の購読者が無ければ
全件の `GET /api/tags` は送られず、購読者がいれば送られる; 先頭のページの失敗で、一覧をまだ持って
いなければ失敗の表示と「Retry」、持っていればその一覧が残る（Edge Case）; 今の `TagsPage.test.tsx` の
行の操作・選択バー・統合・却下した名前の試験が通り続ける。`web/e2e/tags.e2e.ts` の管理画面の試験
（検索 18、16・16b、9〜12、14、17 など）が通り続ける。`tagsbench` の 3 つの規模で、最初の行まで
1 秒以内、開いたときの `items` が 100 件で応答の大きさが規模で変わらず、検索の 1 文字目・Esc・
1 件の確定・改名の後の最長タスクが 0.2 秒以内（受け入れ条件 1〜3）。

### 却下した名前の窓を、開いたときに表示に要る分だけ読む

**Scope**: `TagsPage` の却下した名前の状態を先頭の 1 ページ（`items`・`total`・`nextCursor`）にし、入口の
件数を `total` で出す、`RejectedNames` の窓で中身を末尾までスクロールしたら `listRejectedTagNamePage` で
続きを足す（読み込み中・失敗・「Retry」の見え方は改訂した `ui-design.md`「Rejected names」に従う）、
× で外した名前を局所で取り除き `total` を減らす、031 のきっかけ（却下・作成・改名・シノニムの追加、
まとめての却下）での取り直しは先頭の 1 ページだけ
（[data-model.md §4](data-model.md#4-画面の側で持つ状態)「却下した名前」、
[research.md R-13](research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)）。
英語のカタログの文言。

**Dependencies**: `GET /api/tags/rejected-names をページにし、件数を返す`、
`タグ管理画面の一覧を、条件ごとにサーバーから読み、スクロールで続きを読む`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、開くと
`GET /api/tags/rejected-names` が `limit=100` で 1 回送られ、入口に `total`（応答の `items` より大きい数）が
出る; 窓を開いて末尾までスクロールすると `cursor` 付きの要求が送られ名前が足される; × で外すと
`DELETE` が送られ、その名前が消えて入口の件数が 1 減る; まとめての却下のあと先頭の 1 ページが取り直
される; 続きの失敗で読み込んだ名前が残り「Retry」が出る。`tagsbench` の 1,000 個の規模で、`/tags` を
開いた直後にスクロールせず却下した名前の窓を開ける（受け入れ条件 13）。

### 統合の窓の統合先の候補をサーバーの検索で引く

**Scope**: `MergeTagDialog` の `tags` の prop を外し、入力が変わるたびに `listTagPage({ q, limit })` で候補を
引く（統合元を除く、進行中の要求を打ち切る、届くまで前の候補を残す、読み込み中・失敗の見え方は改訂した
`ui-design.md`「Merge dialog」に従う）、候補の並びはサーバーの名前の順、入力と完全に一致する名前・
シノニムの扱い（`exactOption`）は応答の中から決める、`TagsPage` の呼び方
（[research.md R-14](research.md#r-14-統合の窓の統合先の候補は-get-apitagsqlimit-で引く)、
[data-model.md §4](data-model.md#4-画面の側で持つ状態)「統合の窓の候補」）。英語のカタログの文言。
ARCHITECTURE.md の `web/src/tags/` の段落（統合先を「全部のタグから」選ぶ仕組み）。

**Dependencies**: `GET /api/tags に検索・絞り込み・並び順・ページを足す`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest で、窓を開くと
`GET /api/tags?limit=8`（空の `q`）が送られ、統合元を除いた応答が候補に並ぶ; `ａｃｔ` と入れると
`q=ａｃｔ` で送られ、読み込んでいないタグ `Action` が候補に出る（要件 9）; 入力を続けて変えると前の要求が
打ち切られ、最後の応答だけが候補になる; 選んだ中から統合先を選ぶと `sourceIds` からその id が外れ、
統合元が統合先だけなら実行が押せない（Edge Case、merge 済みの試験が通り続ける）; `MergeTagDialog.test.tsx` の
既存の試験が `tags` の prop 無しで通る。`web/e2e/tags.e2e.ts` の 12・17・B3 と統合の試験が通り続ける。

### 計測の道具に規模 30,000 と、開いたときの転送量・続きを読み込みながらのスクロールの場面を足す

**Scope**: `scripts/tagsbench` に `-videos N`（省けば `-scale` の 10 倍）を足し、規模 30,000・動画 30,000 本の
データを作れるようにする、`web/bench/tags-admin.bench.ts` に「開いたときに受け取るタグ」（開いたときの
`GET /api/tags` の `items` の数と応答のバイト数）と、スクロールの場面を「続きを読み込みながら末尾まで」
（`nextCursor` が尽きるまで送り続ける）に変える、`docs/how-to/tags-admin-benchmark.md` と
[quickstart.md](quickstart.md) の表（[research.md R-9](research.md#r-9-受け入れ条件の計測は作り置きの規模のデータを-scriptstagsbench-が作りplaywright-の計測スクリプトが本番ビルドに対して測る)
「改訂で足す規模と場面」）。`task test-e2e` と CI には入れない。

**Dependencies**: `タグ管理画面の一覧を、条件ごとにサーバーから読み、スクロールで続きを読む`

**Acceptance**: `task check` と `task check-docs` が通る。`go run ./scripts/tagsbench -scale 30000 -videos 30000` が
タグ 30,000 個・動画 30,000 本のデータを作り、起動した本番ビルドの `GET /api/tags?limit=100` が 100 件を
返し、計測の表（quickstart.md の 8 場面）が出る。3 つの規模で、開いたときに受け取る `items` が 100 件で
応答の大きさが ±5% に収まり（受け入れ条件 2）、30,000 個の一覧を末尾まで続きを読み込みながら
スクロールしても 50ms を超えるフレームが 2 つ続かない（受け入れ条件 4）。改訂前（feature branch の
改訂前の先頭）と改訂後を同じ環境で測った表を PR の本文に残し、`GET /api/tags` の応答時間を別に出す
（quickstart.md「内訳の切り分け」）。
