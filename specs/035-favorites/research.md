# Research: 動画とグループのお気に入り

親 Issue: #574。技術スタック・境界・依存方向・索引と利用者データの区分は
[ARCHITECTURE.md](../../ARCHITECTURE.md) と
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) が正本で、
ここでは変えない。利用者データの鍵（集まりなら `bundle:<id>`、そうでなければ `content_key`）は
[specs/030-video-versions/data-model.md §3](../030-video-versions/data-model.md#3-利用者データの鍵)、
フォルダを指す鍵（`domain.FolderKey`）と手動のグループ設定は
[specs/017-folder-groups/data-model.md §1](../017-folder-groups/data-model.md#1-マイグレーション)、
ライブラリの項目の作り方は
[specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md)
にある。ここには、この feature が足す決定だけを書く。

## R-1: 動画のお気に入りは利用者データの鍵に結ぶ `video_favorites`、グループのお気に入りはフォルダの鍵に結ぶ `folder_favorites` の 2 つの表に持つ

- **Decision**: `video_favorites(content_key primary key, favorited_at)` と
  `folder_favorites(path primary key, favorited_at)` を足す（[data-model.md §1](data-model.md#1-マイグレーション)）。
  動画の鍵は `userKeyExpr`（集まりのメンバーなら集まりの鍵）で、`public_videos` と同じ。グループの鍵は
  `domain.FolderKey(フォルダの絶対パス)` で、`folder_group_overrides` と同じ。どちらも `videos`・
  `folder_groups`・`media_folders` への外部キーを張らない。同じパスの中身の引き継ぎ（`moveUserData`）と
  束ねる・解く操作（`userDataTables`）は `video_favorites` も写す。
- **Rationale**: 要件 2・3 と Edge Case（移動・再スキャン・登録解除・グループでなくなった後も記録を残す）は、
  再生位置・公開の設定・手動のグループ設定が今まさに満たしている性質で、同じ鍵に結べば同じ経路
  （`userKeysForVideoIDs`、`moveUserData`、`userDataTables`、索引の作り直しが触らない表）にそのまま乗る。
  Edge Case「#572 が入った場合は集まりで 1 組」は、030 が既に入っているので利用者データの鍵がそれである。
- **Alternatives considered**:
  - `public_videos` のように 1 つの表に「動画かフォルダか」の列を足す。鍵の意味（内容の識別子とフォルダの
    パス）が違い、引き継ぎと束ねの経路は動画の行だけを写すので、表を分けた方が各経路が自分の表だけを
    見ればよい。却下。
  - `videos`・`folder_groups` の列。どちらも作り直せる索引で、再スキャンとフォルダの索引の作り直しで行ごと
    消える（ARCHITECTURE.md「Rebuildable and user data」）。却下。

## R-2: 付け外しは所有者だけの 1 つの経路 `PUT /api/favorites` で、動画の id とフォルダを 1 つの取引で受け、無いものは数えずに飛ばす

- **Decision**: `PUT /api/favorites`（`videoIds`・`folders`・`favorite`）を足し、新しい役割の型 `FavoriteStore`
  （`sql` 接続だけを持つ。`VisibilityStore` と同じ）の `SetFavorites` が 1 つの取引で書く
  （[contracts/screen-api.md §1](contracts/screen-api.md#1-put-apifavorites)、[data-model.md §4](data-model.md#4-書き込みfavoritestore)）。
  `videoIds` は `userKeysForVideoIDs` でいまライブラリにある動画の鍵へ引き直し、`folders` は登録フォルダの
  id と相対パスから絶対パスにして `folder_groups.path_key` にあるもの（所有者から見て今グループのフォルダ）
  だけを書く。引けない id・登録されていない `rootId`・今グループでないフォルダは誤りにせず、応答の
  `appliedVideos`・`appliedFolders` に数えない。既に同じ状態のものは数える（Edge Case「既にお気に入りのものが
  含まれていてもエラーにしない」）。
- **Rationale**: 要件 6 の複数選択は動画とグループを一度に受ける。公開の切り替え（`PUT /api/video-visibility`）が
  「ライブラリに無い id は数えない」で一部反映を応答の数で伝える形を画面（`api/visibility.ts`）が既に
  扱っているので、同じ形にすれば画面の取り直しの仕組み（R-5）を流用できる。1 枚のカードの操作で
  `appliedFolders` が 0 なら、グループでなくなったことを画面が知り、グループのカードを取り直して外す
  （017 の「Refresh and removal」と同じ）。普通のフォルダは対象外（親 Issue）なので、今グループでない
  フォルダには書かない。
- **Alternatives considered**:
  - 動画用 `PUT /api/video-favorites` とフォルダ用 `PUT /api/folders/{rootId}/favorite` の 2 経路。一括が 2 つの
    取引になり、片方だけ成功した状態を画面が扱う必要がある。却下。
  - グループでないフォルダへの指定を `404 not_folder_group` にする。一括で 1 つでも外れると全体が失敗し、
    「すべて選択」の直後にグループが解けた場面で何も付かない。却下。
  - 今グループかを確かめずに書く。普通のフォルダのお気に入りが API からは作れてしまい、対象外の範囲に
    記録だけが残る。却下。

## R-3: お気に入りのみの絞り込みは `libraryItemsCTE` の項目を作る段で効かせ、お気に入りでないグループはお気に入りのメンバーを 1 本ずつ出す

- **Decision**: `VideoQuery.FavoriteOnly`（`favorite=true`）を足す。`GET /api/videos`・
  `GET /api/folders/{rootId}/videos` では動画ごとの条件（`video_favorites` の行がある）として `filteredFrom` に
  掛ける。`GET /api/library`・`GET /api/library/ids` では、検索語とタグで当たった動画（`matched`）と
  全メンバーが当たったグループ（`whole`）は今のまま作り、項目にする段で次のように変える
  （[data-model.md §5](data-model.md#5-読み出しと一覧)）。
  - グループの項目: `whole` のうち、絞り込みが無いか、そのフォルダが `folder_favorites` にあるもの。
  - 動画の項目: 当たった動画のうち、`whole` のグループに属さないもの、または絞り込みがあってそのグループが
    `folder_favorites` に無いもの。絞り込みがあれば、さらに `video_favorites` にあるものだけ。
  再生可否と視聴状態は今までどおり項目に掛け、`total` と `GET /api/library/ids` も同じ項目から数える。
- **Rationale**: 要件 8 は項目ごとに自分のお気に入りで判定し、要件 9 はお気に入りでないグループのお気に入りの
  メンバーを単独の項目として出す。027 の規則（当たったのが一部なら 1 本ずつ）を `matched` に
  `video_favorites` を足すだけで流用すると、お気に入りのグループ G のメンバーが 1 本もお気に入りでないとき G が
  出ず（要件 8・受け入れ条件 5）、G のメンバーが全部お気に入りのとき G がグループの項目になって
  しまう（要件 9）。項目を作る段で「グループ自身のお気に入り」を見るのが、両方を満たす最も小さい変更である。
- **Alternatives considered**:
  - `matched` にお気に入りを足し、027 の規則で項目にする。上のとおり要件 8・9 を満たさない。却下。
  - 項目を作った後に項目の絞り込み（視聴状態と同じ段）だけで効かせる。お気に入りでないグループの項目が
    落ちるので、そのメンバーのお気に入りの動画が出ない（受け入れ条件 4）。却下。

## R-4: 「お気に入りにした日時」の並び順は `favoritedAsc`・`favoritedDesc` の 2 値で、お気に入りでない項目は向きに関係なく末尾に置く

- **Decision**: `VideoSort` に `favoritedAsc`・`favoritedDesc` を足す。値は動画の項目では `video_favorites.favorited_at`、
  グループの項目では `folder_favorites.favorited_at`、お気に入りでなければ NULL で、`listOrder.nullable` の規則
  （値の無い行は向きに関係なく末尾、カーソルも同じ形）に乗せる。付け直し（外して付ける）は新しい日時に
  なる。既にお気に入りのものに付けても日時は変えない。
- **Rationale**: Edge Case「お気に入りでない項目は、お気に入りの項目の後にまとめて並ぶ」は、最後に再生した
  時刻（`played*`。記録の無い動画は末尾）と同じ形で、`listOrders`・`itemOrderValues`・カーソルの規則をそのまま
  使える。
- **Alternatives considered**:
  - お気に入りでない項目を追加日時などで並べ直す。カーソルが 2 つの値を持つことになり、013 の keyset の形
    （値 1 つと id）を変える。却下。

## R-5: ゲストにはお気に入りを出さず、絞り込みと並び順は視聴状態と同じ `400` にする

- **Decision**: `Audience.CheckVideoQuery` に `FavoriteOnly` と `favorited*` を足し、ゲストでは
  `ErrGuestQueryNotAllowed`（`400 guest_filter_not_allowed`）にする。応答の `Video.favorite` と
  `LibraryGroup.favorite` は所有者にだけ入れ、`forAudience` とグループの応答の組み立てでゲストから省く。
  `PUT /api/favorites` は所有者だけ（`/api/*` の既定）。画面は `guestListCriteria` で丸め、ゲストに入口も
  印も出さない。
- **Rationale**: 要件 12 と受け入れ条件 10 がそのまま決めている。視聴状態・再生日時の並び・タグと同じ扱いに
  すると、契約（guest-api.md §3）も画面の丸めも同じ場所に 1 行足すだけで済む。
- **Alternatives considered**: ゲストには `favorite: false` を常に返す。状態を「見せない」要件と、`public` が
  ゲストで意味を持たないのと違って項目自体が所有者のものであることから、省く方が契約として正しい。却下。

## R-6: 画面はドメインイベントを足さず、公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し、再生画面は動画を取り直す

- **Decision**: `web/src/api/favorites.ts` に `updateFavorites`（`PUT /api/favorites`）と、結果の購読
  （`subscribeFavorites`・一部反映の `subscribeFavoritesStale`）、一覧の控えへの反映
  （`applyFavoritesToListSnapshot`）を置く。`api/visibility.ts` と同じ形で、一覧（`useVideos`）は動画の項目の
  `favorite` をその場で差し替え、グループの項目は `GET /api/folders/{rootId}/group` で取り直す（017 の
  「Refresh and removal」の経路。`appliedFolders` が要求より少なければ該当のグループを取り直し、404 なら外す）。
  再生画面は成功後に `GET /api/videos/{id}` で取り直す（033 R-8 と同じ）。新しいドメインイベントと
  `/api/events` の種類は足さない。お気に入りのみで絞り込んだ一覧でカードから外しても、その場では一覧から
  外さず、次に一覧を読んだときに消える（Edge Case「次に取り直したときに最新の状態を見せる」）。
- **Rationale**: 公開の切り替えは「取り直さずに差し替える」「一部反映なら取り直す」「送った順に確定する」の
  3 つを既に解いていて、お気に入りの付け外しは同じ性質（所有者の 1 操作、成功の応答で結果が分かる、
  別タブは次の取り直しで追う）を持つ。サーバーからの知らせは別タブの同期にだけ効き、Edge Case は
  それを求めていない。
- **Alternatives considered**:
  - `domain.VideoOverrideChanged` のようなイベントを足して `/api/events` の `video` で知らせる。付け外しの
    たびに一覧の項目を取り直すことになり、一括で 2 万件を付けると 2 万回の取り直しになる。却下。
  - お気に入りのみの絞り込み中に外したカードをその場で一覧から外す。選択バーの「タグを外す」は取り直す
    方針（library-ui.md §6）だが、カードの 1 操作で一覧が動くとカードの位置が変わり、続けて押せない。
    次の読み込みに任せる。却下。

## R-7: 複数選択は選んだグループをグループとして覚え、一括のお気に入りではグループのメンバーを動画として送らない

- **Decision**: ライブラリの選択（`LibraryPage`）は、今の動画 id の集合に加えて、グループのカードのチェックで
  選んだグループ（フォルダ、そのメンバーの id）を持つ。メンバーのどれかが選択から外れたらそのグループは
  選んだグループから外す。「すべて選択」は `GET /api/library/ids` の応答に足す `groups`（グループの項目の
  フォルダとメンバーの id、[contracts/screen-api.md §3](contracts/screen-api.md#3-get-apilibraryids-に足す項目)）を
  選んだグループにする。選択バーのお気に入りは `videoIds` を「選んだ id のうち選んだグループのメンバーで
  ないもの」、`folders` を選んだグループにして送る。タグの付け外し・公開・束ねる操作は今までどおり
  動画の id の集合を送る。
- **Rationale**: 要件 6・受け入れ条件 7 は、選んだグループにはグループのお気に入りを付けてメンバーには
  付けないことを求める。今の選択は動画の id の集合だけで（017「Pressing and selection」）、一覧に載って
  いないページを「すべて選択」で含めると、どの id がグループのメンバーかを画面は知らない。
- **Alternatives considered**:
  - サーバーが `videoIds` から `folders` のメンバーを除く。画面が送る内容と結果が食い違い、`appliedVideos` の
    意味が「送った id のうち」でなくなる。却下。
  - 選んだグループを「読み込んだグループの項目のうち全メンバーが選択に入っているもの」から導く。
    「すべて選択」で読んでいないページのグループが動画として付いてしまう。却下。

## R-8: お気に入りの付け外しは動画の更新日時（`video_edits`）を進めない

- **Decision**: `FavoriteStore` は `touchEditedAt` を呼ばない。`Video.updatedAt` はお気に入りで変わらない。
- **Rationale**: 033 R-2 は、進めるのを「動画の情報を書く 4 種の操作」に限り、再生位置は進めないと決めた。
  お気に入りは動画の情報（表示名・タグ・公開・代表サムネイル）ではなく、再生位置と同じ所有者自身の
  印で、自分の日時（`favorited_at`）と並び順を持つ。一括で 2 万件に付けるたびに全件の更新日時が進むと、
  「この動画を最後にいつ直したか」が分からなくなる（033 R-2 と同じ理由）。
- **Alternatives considered**: 公開の設定と同じく進める。公開は他の人に見える範囲を変える動画の設定で、
  お気に入りは所有者だけの印である。却下。親 Issue はどちらとも書いていないので、PR の本文で確認を求める。
