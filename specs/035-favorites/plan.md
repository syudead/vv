# Implementation Plan: 動画とグループをお気に入りにする

**Branch**: `feature/035-favorites` | **Parent Issue**: #574

**Input**: The parent Issue. It is this feature's specification.

## Summary

所有者が動画とフォルダのグループにそれぞれお気に入り（オン・オフ）を付け外しでき、再生画面・ライブラリの
カード・複数選択の 3 つの入口で今の状態が見え、ライブラリとフォルダの一覧を「お気に入りのみ」で絞り、
「お気に入りにした日時」で並べられるようにする。ゲストには状態も入口も出さない。

- **保存**: 動画は利用者データの鍵に結ぶ `video_favorites`、グループはフォルダの鍵に結ぶ
  `folder_favorites` の 2 表。再スキャン・移動・束ね・引き継ぎは公開の設定と手動のグループ設定の経路に
  そのまま乗る（[research.md R-1](research.md#r-1-動画のお気に入りは利用者データの鍵に結ぶ-video_favoritesグループのお気に入りはフォルダの鍵に結ぶ-folder_favorites-の-2-つの表に持つ)、
  [data-model.md §1](data-model.md#1-マイグレーション)）。
- **付け外し**: 所有者だけの `PUT /api/favorites` が動画の id とグループのフォルダを 1 つの取引で受け、
  無いもの・今グループでないフォルダは数えずに飛ばす（[R-2](research.md#r-2-付け外しは所有者だけの-1-つの経路-put-apifavorites-で動画の-id-とフォルダを-1-つの取引で受け無いものは数えずに飛ばす)、
  [contracts/screen-api.md §1](contracts/screen-api.md#1-put-apifavorites)）。更新日時は進めない
  （[R-8](research.md#r-8-お気に入りの付け外しは動画の更新日時video_editsを進めない)）。
- **一覧**: `favorite=true` は項目ごとに自分のお気に入りで判定し、お気に入りでないグループのお気に入りの
  メンバーは動画の項目にする。`favoritedAsc`・`favoritedDesc` はお気に入りでない項目を末尾に置く。ゲストは
  `400`（[R-3](research.md#r-3-お気に入りのみの絞り込みは-libraryitemscte-の項目を作る段で効かせお気に入りでないグループはお気に入りのメンバーを-1-本ずつ出す)、
  [R-4](research.md#r-4-お気に入りにした日時の並び順は-favoritedascfavoriteddesc-の-2-値でお気に入りでない項目は向きに関係なく末尾に置く)、
  [R-5](research.md#r-5-ゲストにはお気に入りを出さず絞り込みと並び順は視聴状態と同じ-400-にする)、
  [data-model.md §5](data-model.md#5-読み出しと一覧)）。
- **画面**: ドメインイベントは足さず、公開の切り替えと同じ購読で一覧と再生画面に反映する
  （[R-6](research.md#r-6-画面はドメインイベントを足さず公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し再生画面は動画を取り直す)）。
  複数選択は選んだグループをグループとして覚え、「すべて選択」は `GET /api/library/ids` に足す `groups` で
  それを知る（[R-7](research.md#r-7-複数選択は選んだグループをグループとして覚え一括のお気に入りではグループのメンバーを動画として送らない)）。
  `ui` ラベルがあるので、印と付け外しの見た目・置き場所・文言・絞り込みと並び順の名前は次の design 段階の
  `ui-design.md` が親 Issue の「UI品質」を基準に決める。カードの行を増やさず、再生画面では二次的な操作の
  段に置く（親 Issue）。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・索引と利用者データの区分・役割の型の規則・ドメインイベント・認証の境界:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- 利用者データの鍵と書き込み: [specs/030-video-versions/data-model.md §3・§5・§8](../030-video-versions/data-model.md)、
  [internal/store/user_keys.go](../../internal/store/user_keys.go)（`userKeyExpr`・`userKeysForVideoIDs`）、
  [internal/store/visibility.go](../../internal/store/visibility.go)（`VisibilityStore`・`publicColumn`）、
  [internal/store/successions.go](../../internal/store/successions.go)（`moveUserData`）、
  [internal/store/versions.go](../../internal/store/versions.go)（`userDataTables`）、
  [internal/store/roles.go](../../internal/store/roles.go)
- フォルダの鍵とグループの索引: [specs/017-folder-groups/data-model.md](../017-folder-groups/data-model.md)、
  [internal/domain/folder_group.go](../../internal/domain/folder_group.go)（`FolderKey`）、
  [internal/store/folder_groups.go](../../internal/store/folder_groups.go)
- 一覧と項目: [internal/store/listing.go](../../internal/store/listing.go)（`filteredFrom`・`listOrders`）、
  [internal/store/library_items.go](../../internal/store/library_items.go)（`libraryItemsCTE`・`itemOrderValues`・
  `loadGroups`・`LibraryIDs`）、[internal/domain/library.go](../../internal/domain/library.go)（`VideoQuery`・`VideoSort`）、
  [internal/domain/library_item.go](../../internal/domain/library_item.go)、
  [specs/013-library-search/contracts/list-api.md](../013-library-search/contracts/list-api.md)、
  [specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md)
- ゲスト: [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)、
  [internal/domain/auth.go](../../internal/domain/auth.go)（`CheckVideoQuery`）、
  [internal/httpapi/videos.go](../../internal/httpapi/videos.go)（`parseVideoQuery`・`forAudience`）
- 画面の API と変換: [api/openapi.yaml](../../api/openapi.yaml)、[internal/httpapi/library.go](../../internal/httpapi/library.go)、
  [internal/httpapi/folders.go](../../internal/httpapi/folders.go)（`resolveFolderRootIn`）、
  [internal/httpapi/video_tags.go](../../internal/httpapi/video_tags.go)（`parseIDsQuery`・`writeVideoIDs`）
- 画面: [web/src/api/visibility.ts](../../web/src/api/visibility.ts)・[useVideos.ts](../../web/src/api/useVideos.ts)・
  [listSnapshot.ts](../../web/src/api/listSnapshot.ts)（公開の切り替えの反映）、
  [web/src/videoList/VideoCard.tsx](../../web/src/videoList/VideoCard.tsx)、
  [web/src/library/GroupCard.tsx](../../web/src/library/GroupCard.tsx)、
  [web/src/library/LibraryPage.tsx](../../web/src/library/LibraryPage.tsx)・[SelectionBar.tsx](../../web/src/library/SelectionBar.tsx)、
  [web/src/player/VideoFacts.tsx](../../web/src/player/VideoFacts.tsx)（二次的な操作の段）、
  [web/src/videoList/listCriteria.ts](../../web/src/videoList/listCriteria.ts)・[FilterMenu.tsx](../../web/src/videoList/FilterMenu.tsx)・
  [SortControls.tsx](../../web/src/videoList/SortControls.tsx)、[web/src/folders/useConditions.ts](../../web/src/folders/useConditions.ts)、
  [web/src/i18n/en.ts](../../web/src/i18n/en.ts)、[docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)、
  [specs/016-single-account-auth/ui-design.md](../016-single-account-auth/ui-design.md)「Visibility toggle」、
  [specs/017-folder-groups/ui-design.md](../017-folder-groups/ui-design.md)「Pressing and selection」
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- 移行は 1 つ（`00029_favorites.sql`、[data-model.md §1](data-model.md#1-マイグレーション)）。Go・npm の依存は
  足さない。ドメインイベントと `/api/events` の種類は足さない（R-6）。`FolderIndexVersion`・
  `SearchKeyVersion` は上げない。
- 外部連携 API（`api/external-v1.yaml`）と MCP は変えない（対象外）。
- `quickstart.md` は作らない。受け入れ条件は store・httpapi・Vitest の試験で確かめられ、feature 固有の
  手順は無い。`ui-design.md` は次の design 段階が作る。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。`internal/domain` は値と `VideoSort` の
  2 値、`CheckVideoQuery` の 1 行。`internal/store` は移行・`FavoriteStore`・読み出しの列と一覧の SQL。
  `internal/httpapi` は経路の追加と検査・変換。`internal/app`・`cmd/mdm` は触らない（`cmd/mdm` は
  `db.Favorites()` を配線する 1 行だけ）。
- **役割の型は他の役割の公開メソッドを呼ばない**（`store.DB` の段落）: 合格。`FavoriteStore` は
  `userKeysForVideoIDs`（パッケージ内の関数）と自分の SQL だけを使い、`folder_groups` は自分の取引の中で読む。
- **索引と利用者データの区別**: 合格。2 表はどちらも利用者データで外部キーを持たず、索引の作り直し・
  メディアフォルダの削除・生成物の片付けは触らない（data-model.md §1）。ARCHITECTURE.md の一覧に足す。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml` を直して `task generate`。認証の分類は
  `/api/favorites` を既定の所有者だけに入れ、`openapi_routes_test.go` が `security` と突き合わせる。
- **ゲストは所有者のデータを見ない**（guest-api.md）: 合格。項目は所有者の応答にだけ入れ、条件は `400`
  （R-5）。
- **サーバーの出力は英語、画面の文言はカタログ**（`.golangci.yml` の gosmopolitan、i18n.md）: 合格。
- **設計文書は今どうなっているかを書く**（docs/design-docs/index.md）: 合格。各単位が ARCHITECTURE.md
  （利用者データの一覧、役割の型の一覧、「fifteen sort orders」、`GET /api/library` の段落）と
  `docs/design-docs/library-ui.md` の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/035-favorites/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-8
├── data-model.md         # 2 表、domain の値、読み出しの列、FavoriteStore、一覧の規則
└── contracts/
    └── screen-api.md     # Video・LibraryGroup の favorite、PUT /api/favorites、favorite と favorited*、ids の groups
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。`quickstart.md` は作らない（上の Technical Context）。

### Source Code

**Affected boundaries**:

- `internal/domain`（`Video.Favorite`・`LibraryGroup.Favorite`・`VideoQuery.FavoriteOnly`・`VideoSort` の 2 値・
  `FavoriteChange`・`LibrarySelection`・`CheckVideoQuery`）
- `internal/store`（移行、`FavoriteStore`、`favorite` 列、`loadGroups`、`filteredFrom`・`listOrders`・
  `libraryItemsCTE`・`itemOrderValues`・`LibraryIDs`、`moveUserData`・`userDataTables`、不変条件）
- `internal/httpapi`（`favorites.go`、`parseVideoQuery`・`parseIDsQuery`・`listFolderVideos` の `favorite`、
  `forAudience`、グループの応答、`writeVideoIDs` の `groups`）、`api/openapi.yaml` と生成物、`cmd/mdm` の配線
- `web/src/api`（`favorites.ts`、`client.ts` の `videoSorts`・`ListFilterParams`、`useVideos`・`useVideoDetail`・
  `listSnapshot` の購読）、`web/src/videoList`（カードの印と付け外し、絞り込み・並び順）、`web/src/library`
  （グループのカード、選択、選択バー）、`web/src/folders`（条件の配線）、`web/src/player`（二次的な操作の段）、
  `web/src/preferences`、`web/src/i18n`
- `ARCHITECTURE.md`、`docs/design-docs/library-ui.md`。`guest-api.md`・`list-api.md`・027 の契約へは足さず、
  この feature の [contracts/screen-api.md](contracts/screen-api.md) と [data-model.md §5](data-model.md#5-読み出しと一覧)
  が差分を持つ

**New paths**:

- `internal/store/migrations/00029_favorites.sql`、`internal/store/favorites.go`（`FavoriteStore`）、
  `internal/httpapi/favorites.go`（`PUT /api/favorites`）
- `web/src/api/favorites.ts`、`web/src/videoList/FavoriteToggle.tsx`（カード・行・再生画面で共有する付け外し。
  名前は `ui-design.md` に従って変えてよい）

**Structure decision**: 付け外しは新しい役割の型 `FavoriteStore` に置く。`VisibilityStore` と同じく SQL 接続だけを
持ち、動画とフォルダの 2 表を 1 つの取引で書く。`FolderGroupStore` に足さないのは、動画のお気に入りが
フォルダの索引と無関係で、`FolderGroupStore` の操作が索引の作り直しを伴うのに対し、お気に入りは索引に
触れないからである。一覧の規則は `libraryItemsCTE` の項目を作る段に置く（R-3）。画面の反映は
`api/visibility.ts` の形を `api/favorites.ts` に写し、`useVideos`・`useVideoDetail`・`listSnapshot` の購読に
1 種類足す（R-6）。

## Implementation Work

### お気に入りの表と `FavoriteStore` を足し、動画とグループの読み出しにお気に入りを載せる

**Scope**: `00029_favorites.sql` と不変条件（[data-model.md §1](data-model.md#1-マイグレーション)）、`domain` の
`Video.Favorite`・`LibraryGroup.Favorite`・`FavoriteChange`・`FavoriteApplied`（[§2](data-model.md#2-domain-に足す値)）、
`favorite` 列と `loadGroups` の結合（[§3](data-model.md#3-読み出しの列)）、`FavoriteStore.SetFavorites` と
`db.Favorites()`（[§4](data-model.md#4-書き込みfavoritestore)）、`moveUserData` と `userDataTables` への追加。
ARCHITECTURE.md の利用者データの一覧と役割の型の一覧。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、`SetFavorites` のあと `GetVideo`・`ListVideos`・`ListLibrary`
の `Video.Favorite` と `ListLibrary`・`FolderGroup` の `LibraryGroup.Favorite` が真になり、外すと偽に戻る
（受け入れ条件 1）。グループに付けてもメンバーの `Favorite` は偽、メンバーに付けてもグループは偽
（受け入れ条件 3、要件 4）。ライブラリに無い id・今グループでないフォルダ・登録フォルダの外のパスは
`Applied` に数えず誤りにならず、既に同じ状態のものは数えて日時を変えない（Edge Case）。付けた動画の
`EditedAt` は変わらない（R-8）。同じ内容の所在を別のフォルダへ移して `UpsertVideo` と所在の削除をしても
`Favorite` は真のまま（受け入れ条件 2）。再生位置を完了にしても真のまま（受け入れ条件 6）。束ねると集まりの
全バージョンが真になり、外したバージョンは自分の値に戻る。同じパスの引き継ぎで新しい内容に移る。
`rebuildFolderIndex`・メディアフォルダの削除・`RemoveContent` のあとも 2 表の行が残る（Edge Case）。移行のあと
2 表が空で、不変条件が通る。

### 一覧でお気に入りのみに絞り、お気に入りにした日時で並べる

**Scope**: `domain` の `VideoQuery.FavoriteOnly`・`FolderVideoQuery.FavoriteOnly`・`SortFavoritedAsc`・
`SortFavoritedDesc`・`Valid`・`CheckVideoQuery`（[data-model.md §2](data-model.md#2-domain-に足す値)）、
`filteredFrom`・`listOrders`（[§5 動画の一覧](data-model.md#動画の一覧listvideoslistfoldervideoscountvideos)）、
`libraryItemsCTE` の項目の条件と `favorited_at` 列・`itemOrderValues`（[§5 ライブラリの項目](data-model.md#ライブラリの項目libraryitemsctelistlibrarylibraryids)）、
`LibraryIDs` の `domain.LibrarySelection`（`internal/httpapi` は今の `ids` を和で作る。`groups` は次の API の
単位）。ARCHITECTURE.md の `GET /api/library` の段落と「fifteen sort orders」。

**Dependencies**: `お気に入りの表と FavoriteStore を足し、動画とグループの読み出しにお気に入りを載せる`

**Acceptance**: `task check` が通る。store の試験で、fixture のグループ G（メンバー A・B・C）について: A だけに
付けて `FavoriteOnly` の `ListLibrary` が A の動画の項目 1 件で `total` が 1、G の項目は無い（受け入れ条件 4）;
G に付けると G のグループの項目 1 件で A・B・C の動画の項目は無い（受け入れ条件 5）; A・B・C の全部に付けて
G に付けないと 3 本の動画の項目（要件 9）; 2 本だけに付いたタグと同時に使うと、当たってお気に入りの動画だけ
（受け入れ条件 9）; `WatchUnwatched` と組み合わさる（要件 8）; `LibraryIDs` が同じ項目の id とグループの
フォルダ・メンバーを返す。`ListVideos`・`ListFolderVideos` の `FavoriteOnly` がお気に入りの動画だけを返す
（要件 11）。`favoritedDesc` で最後に付けたものが先頭、お気に入りでない項目は昇順・降順のどちらでも末尾で、
カーソルをまたいでも並びが保たれる（受け入れ条件 8、Edge Case）。外して付け直すと先頭に来る。ゲストの
`CheckVideoQuery` が `FavoriteOnly` と `favorited*` を `ErrGuestQueryNotAllowed` にする。

### `PUT /api/favorites` と応答の `favorite` を足す

**Scope**: `api/openapi.yaml` の `/api/favorites`・`FavoritesRequest`・`FavoritesResponse`・`Video.favorite`・
`LibraryGroup.favorite` と生成物（[contracts/screen-api.md §0・§1](contracts/screen-api.md#0-video-と-librarygroup-に足す項目)）、
`internal/httpapi/favorites.go`（検査、`folders` の絶対パスへの解決、`FavoriteStore` の呼び出し）、`toAPIVideo` と
グループの応答への `favorite`、`forAudience` と `itemLookup.group` でのゲストからの省略、`cmd/mdm` の配線、
`openapi_routes_test.go`。

**Dependencies**: `お気に入りの表と FavoriteStore を足し、動画とグループの読み出しにお気に入りを載せる`

**Acceptance**: `task check` が通る。httpapi の試験で、`PUT /api/favorites` に動画 2 本とグループ 1 つを送ると
`appliedVideos` 2・`appliedFolders` 1 で、`GET /api/videos/{id}` の 2 本と `GET /api/folders/{rootId}/group` が
`favorite: true`、グループのメンバーは `false`（受け入れ条件 7）。`videoIds` と `folders` の合計が 0 と 20001 は
`400 too_many_videos`、不正な `path` は `400 invalid_folder_path`、無い `rootId`・今グループでないフォルダ・無い
id は数えない。ゲストの `PUT` は `401`、ゲストの `GET /api/videos/{id}`・`GET /api/library`・
`GET /api/folders/{rootId}/group` の応答に `favorite` が無い（受け入れ条件 10）。所有者の一覧・関連・
バージョン・`GET /api/folders/{rootId}/videos` の `Video` に `favorite` が入る。`openapi_routes_test.go` と
生成物の検査が通る。

### 一覧 API に `favorite` の絞り込み・`favorited*` の並び順・`ids` の `groups` を足す

**Scope**: `api/openapi.yaml` の 4 経路の `favorite`、`VideoSort` の 2 値、`VideoIdsResponse.groups`・
`LibraryGroupIds` と生成物（[contracts/screen-api.md §2・§3](contracts/screen-api.md#2-一覧の-favorite-と-videosort-に足す値)）、
`parseVideoQuery`・`parseIDsQuery`・`listFolderVideos` の `favorite`、`checkAudienceQuery` の `message`、
`writeVideoIDs` の `groups`（登録フォルダから `VideoFolder` を作る）、`web/src/api/client.ts` の `videoSorts`・
`ListFilterParams.favorite`・`listLibraryIds` の型（[§5](contracts/screen-api.md#5-websrcapi-の差分)）。

**Dependencies**: `一覧でお気に入りのみに絞り、お気に入りにした日時で並べる`、
`PUT /api/favorites と応答の favorite を足す`（`api/openapi.yaml` の `VideoSort` と `description` を同じ節で直すため）

**Acceptance**: `task check` が通る。httpapi の試験で、所有者の `GET /api/library?favorite=true` がお気に入りの
項目だけを返し `total` が合う（受け入れ条件 4・5）、`&tag=` と同時に使うと両方に当たる項目だけ
（受け入れ条件 9）、`GET /api/library/ids?favorite=true` の `ids` と `groups` が同じ項目から作られ、グループの
`folder` が `GET /api/library` の `group.folder` と一致する。`GET /api/videos`・`GET /api/folders/{rootId}/videos`
の `favorite=true` と `sort=favoritedDesc` が効く（要件 11、受け入れ条件 8）。ゲストの `favorite=true` と
`sort=favoritedAsc`・`favoritedDesc` が 4 経路で `400 guest_filter_not_allowed`（受け入れ条件 10）。`openapi_routes_test.go`
と生成物の検査が通る。

### ライブラリとフォルダ画面のカードにお気に入りの印と付け外しを置く

**Scope**: `web/src/api/favorites.ts`（`updateFavorites`、結果と一部反映の購読、一覧の控えへの反映。
`api/visibility.ts` と同じ形、[R-6](research.md#r-6-画面はドメインイベントを足さず公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し再生画面は動画を取り直す)）、
`useVideos`・`videosData`・`listSnapshot` の購読（動画の項目はその場で差し替え、グループの項目は
`GET /api/folders/{rootId}/group` で取り直し、404 なら外す）、`FavoriteToggle`（所有者だけ、`aria-pressed`、
リンクの外に置く）を `VideoCard`・`VideoRow`・`GroupCard`・`GroupRow` に置く（見た目と置き場所は `ui-design.md`
「Card」に従う）、英語のカタログの文言、`web/e2e/guest.e2e.ts` の「ゲストの画面に…出ない」の場面に
お気に入りの印と `/api/favorites` を足す。`docs/design-docs/library-ui.md` §6 のカードの説明。

**Dependencies**: `PUT /api/favorites と応答の favorite を足す`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、所有者の
動画のカードと行の付け外しを押すと `PUT /api/favorites` に `videoIds: [id]` が送られ、応答のあと同じ場所の
印が付いた状態になり、もう一度押すと外れる（要件 7、受け入れ条件 1）; グループのカードは `folders: [folder]`
を送り、応答のあとグループのカードを取り直して印が変わり、メンバーには影響しない（受け入れ条件 3）;
`appliedFolders` が 0 のときグループのカードを取り直して 404 なら外す; 押してもカードのリンク（開く）と
選択のチェックは動かず、選択モードでも付け外しできる（UI 品質「操作の優先順位」）; ゲストのカードと行に
印も付け外しも無い（受け入れ条件 10）; 一覧の控えを復元したときもお気に入りの結果が反映されている。
`task test-e2e` のゲストの場面をローカルで通し、結果を PR の本文に書く。

### 再生画面にお気に入りの付け外しを置く

**Scope**: `VideoFacts` の二次的な操作の段（ファイルを開く・パスをコピー・今の場面をサムネイルにと同じ
`actionsRef` の中。位置と見た目は `ui-design.md`「Video page」に従う）に `FavoriteToggle` を置き、成功後に
`VideoPage` の `refresh()` で動画を取り直す（R-6）。`useVideoDetail` の購読で、一覧側の付け外しが再生画面の
1 件に反映される。英語のカタログの文言。`docs/design-docs/library-ui.md` §8 の再生画面の説明。

**Dependencies**: `ライブラリとフォルダ画面のカードにお気に入りの印と付け外しを置く`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、所有者の
再生画面で付け外しを押すと `PUT /api/favorites` に `videoIds: [id]` が送られ、応答のあと `GET /api/videos/{id}`
を取り直して状態が変わる（受け入れ条件 1）; 送信中は二重に送らない; 失敗したら文言が出て状態は変わらない;
再生画面から戻ったライブラリの一覧（控え）でそのカードの印が変わっている（受け入れ条件 1）; 見終わった
動画でも印が残り、先頭から再生される（受け入れ条件 6、要件 5）; ゲストの再生画面に印も付け外しも無い
（受け入れ条件 10）。

### 複数選択で選んだ動画とグループに一括でお気に入りを付け外す

**Scope**: `LibraryPage` の選択に「選んだグループ」（フォルダとメンバーの id）を足し、グループのチェック・
タグの行の切り替え・メンバーの個別の解除・条件の変化・「すべて選択」の `groups` で保つ
（[R-7](research.md#r-7-複数選択は選んだグループをグループとして覚え一括のお気に入りではグループのメンバーを動画として送らない)）。
`SelectionBar` にお気に入りの操作（付ける・外す。形は `ui-design.md`「Selection bar」に従う）を置き、
`videoIds`（選んだ id のうち選んだグループのメンバーでないもの）と `folders` を `updateFavorites` に送り、
トーストで結果を伝えて選択は残す。上限（20000）の扱いはタグ・公開と同じ。英語のカタログの文言。
`docs/design-docs/library-ui.md` §6 の選択バーの説明。

**Dependencies**: `一覧 API に favorite の絞り込み・favorited* の並び順・ids の groups を足す`、
`ライブラリとフォルダ画面のカードにお気に入りの印と付け外しを置く`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、動画 2 本と
グループ 1 つのチェックを入れて「付ける」と、`PUT /api/favorites` の `videoIds` がその 2 本だけ、`folders` が
そのグループ 1 つで、応答のあと 3 枚のカードの印が付き、グループのメンバーには付かない（受け入れ条件 7）;
グループのメンバーを 1 本外してから付けると、残ったメンバーは `videoIds` に入りグループは `folders` に
入らない; 「すべて選択」のあとの一括は応答の `groups` を `folders` に、残りを `videoIds` に送る; 既に
お気に入りのものを含めても誤りにならず、トーストに `appliedVideos + appliedFolders` が出る（Edge Case）;
「外す」も同じ分け方で送る。タグ・公開・束ねる操作の送る内容は変わらない。

### ライブラリとフォルダ画面にお気に入りのみの絞り込みと「お気に入りにした日時」の並び順を足す

**Scope**: `listCriteria` の `favorite`（URL は `fav=1`、`hasConditions`・`clearConditions`・`guestListCriteria` の
丸め、`criteriaKey`）、`sortKinds` の `favorited`（選んだときの向きは降順、`ownerOnlySort` に足す）、
`FilterMenu` のお気に入りのみ（所有者だけ。数字に数える）、`SortControls` の種類（名前・アイコン・`md` 未満の
まとめは `ui-design.md`「Filter menu」「Sort and direction」に従う）、`viewPreferences` の往復、`useConditions`・
`LibraryPage`・`FolderPage` の配線（`listLibrary`・`listLibraryIds`・`listFolderVideos` に `favorite` を渡す）、
`listSummary` の条件の表示、英語のカタログの文言。`docs/design-docs/library-ui.md` §6 のツールバーの説明
（並び順の種類が 9 つになる）。

**Dependencies**: `一覧 API に favorite の絞り込み・favorited* の並び順・ids の groups を足す`

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、絞り込みの
ポップオーバーでお気に入りのみを入れると `favorite=true` で一覧を取り、URL に `fav=1` が付き、ボタンの数字に
数えられ、「条件を解除」で外れる（要件 8）; タグの絞り込みと同時に使うと両方を送る（受け入れ条件 9）;
並び順のメニューに「お気に入りにした日時」が出て、選ぶと `sort=favoritedDesc`、向きの切り替えで
`favoritedAsc` になり、URL と端末の設定に往復する（要件 10、受け入れ条件 8）; フォルダ画面でも同じ絞り込みと
並び順が使える（要件 11）; ゲストにはどちらも出ず、URL に残っていれば既定に丸めてから要求する
（受け入れ条件 10）。
