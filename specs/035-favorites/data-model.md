# Data model: 動画とグループのお気に入り

親 Issue: #574。既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が
正本で、データの区分は [ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」、利用者
データの鍵は [specs/030-video-versions/data-model.md §3](../030-video-versions/data-model.md#3-利用者データの鍵)、
フォルダの鍵は [specs/017-folder-groups/data-model.md §1](../017-folder-groups/data-model.md#1-マイグレーション)
にある。ここには、この feature が足す表、値、読み書きの規則だけを書く。書いていない表は変えない。

## 1. マイグレーション

`00029_favorites.sql`（`main` の最後は `00028_video_file_created_at.sql`。実装の時点で番号を確かめる）:

```sql
-- お気に入り（specs/035-favorites/research.md R-1）。どちらも作り直せない利用者データで、
-- 再スキャン・メディアフォルダの変更・フォルダの索引の作り直しで消えてはならない。
-- videos・folder_groups・media_folders への外部キーを張らない。

-- 動画のお気に入り。鍵は利用者データの鍵（集まりのメンバーなら 'bundle:<id>'、そうでなければ
-- content_key。public_videos と同じ）。
create table video_favorites (
    content_key  text    primary key,
    favorited_at integer not null
) without rowid;

-- グループのお気に入り。鍵はフォルダの絶対パスを domain.FolderKey で整えたもの
-- （folder_group_overrides.path と同じ）。
create table folder_favorites (
    path         text    primary key,
    favorited_at integer not null
) without rowid;
```

Down は 2 つの表を落とす。

不変条件（`internal/store/invariants_test.go` に足す）: `video_favorites` に空の `content_key` の行は無い。
`folder_favorites.path` は `domain.FolderKey(path)` と等しい（整えた鍵しか書かない）。

区分: どちらも作り直せない利用者データで、ARCHITECTURE.md の一覧に足す。生成物の片付け
（`RemoveContent`）、`releaseContentIndex`、`rebuildFolderIndex`、メディアフォルダの削除はどちらにも触れない。

引き継ぎと束ね:

| 経路 | 変更 |
| --- | --- |
| `moveUserData`（同じパスの中身の引き継ぎ、[030 §5](../030-video-versions/data-model.md#5-同じパスの中身の引き継ぎ)） | 表の一覧に `video_favorites` を足す。`from` に行があれば `to` の行を置き換える |
| `userDataTables`（束ねる・代表を替える・外す、[030 §8](../030-video-versions/data-model.md#8-保存層の操作versionstore)） | `{"video_favorites", "favorited_at"}` を足す。束ねると代表の値が集まりの鍵に写り、外したメンバーは自分の鍵の値に戻る |
| `folder_favorites` | どの経路も写さない。フォルダの改名・移動で外れるのは要件どおり（Edge Case） |

## 2. `domain` に足す値

| 値 | 中身 |
| --- | --- |
| `Video.Favorite` | `bool`。利用者データの鍵が `video_favorites` にあるか。動画を返す読み出しが埋める（§5） |
| `LibraryGroup.Favorite` | `bool`。グループのフォルダの鍵が `folder_favorites` にあるか。`loadGroups` が埋める |
| `VideoQuery.FavoriteOnly`・`FolderVideoQuery.FavoriteOnly` | `bool`。お気に入りのみに絞る（§5） |
| `SortFavoritedAsc` / `SortFavoritedDesc` | `VideoSort` の `favoritedAsc` / `favoritedDesc`。`Valid` に足す |
| `FavoriteChange` | `FavoriteStore.SetFavorites` の入力。`VideoIDs []int64`、`FolderPaths []string`（絶対パス）、`Favorite bool` |
| `FavoriteApplied` | 結果。`Videos int`（引き直せた鍵の数）、`Folders int`（今グループで書いたフォルダの数） |
| `Audience.CheckVideoQuery` | ゲストでは `FavoriteOnly` と `favorited*` を `ErrGuestQueryNotAllowed` にする（[research.md R-5](research.md#r-5-ゲストにはお気に入りを出さず絞り込みと並び順は視聴状態と同じ-400-にする)） |

`favorited_at` は `domain` に出さない。並び順は保存層の SQL だけが使い、応答にも載せない
（[contracts/screen-api.md §0](contracts/screen-api.md#0-video-と-librarygroup-に足す項目)）。

## 3. 読み出しの列

動画を返す読み出し（`videoColumnsTemplate`・`listColumns`。外部連携の `readVideosByIDs` は `videoColumns` を
使うが `ExternalVideo` には写さない）に 1 列を足し、`scanVideo` が `Video.Favorite` に写す。

| 列 | 式 |
| --- | --- |
| `favorite` | `exists (select 1 from video_favorites fav where fav.content_key = <userKeyExpr("videos")> and videos.content_key <> '')`（`publicColumn` と同じ形） |

グループ（`loadGroups`）は `folder_groups` に `left join folder_favorites ff on ff.path = g.path_key` を足し、
`ff.path is not null` を `LibraryGroup.Favorite` に写す。`FolderGroup`（取り直し）も同じ経路なので同じ値を返す。

## 4. 書き込み（`FavoriteStore`）

新しい役割の型 `FavoriteStore struct{ sql *sql.DB }`（`db.Favorites()`）。`VisibilityStore` と同じく共有する
SQLite 接続だけを持ち、索引の型・通知・他の役割の公開メソッドに依存しない。ドメインイベントは発行しない
（[research.md R-6](research.md#r-6-画面はドメインイベントを足さず公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し再生画面は動画を取り直す)）。

`SetFavorites(ctx, change domain.FavoriteChange) (domain.FavoriteApplied, error)` は 1 つの取引で次を行う。
途中で失敗したら何も残さない。

1. `change.VideoIDs` を `userKeysForVideoIDs` で、いまライブラリにある動画の利用者データの鍵へ引き直す
   （同じ集まりは 1 つ、引けない id と空の `content_key` は含めない）。鍵の数が `Videos`。
2. `change.FolderPaths` の各パスを `domain.FolderKey` にし、`folder_groups.path_key` にあるものだけを残す
   （所有者から見て今グループのフォルダ）。残った数が `Folders`。同じフォルダの重複は 1 つに数える。
3. `Favorite` が真なら `insert or ignore into video_favorites (content_key, favorited_at)` と
   `insert or ignore into folder_favorites (path, favorited_at)`（既にある行の日時は変えない）、偽なら
   それぞれ `delete`。鍵の数によらず json_each で 1 文ずつ書く（`touchEditedAt` と同じ形）。
4. `video_edits` には触れない（[research.md R-8](research.md#r-8-お気に入りの付け外しは動画の更新日時video_editsを進めない)）。

`favorited_at` は取引を始めた時刻の Unix 秒で、同じ取引の対象はすべて同じ値になる。

## 5. 読み出しと一覧

### 動画の一覧（`ListVideos`・`ListFolderVideos`・`CountVideos`）

`filteredFrom` に `left join video_favorites fav on fav.content_key = <userKeyExpr("videos")> and videos.content_key <> ''`
を足す。`FavoriteOnly` なら条件 `fav.content_key is not null` を掛ける。並べ替えは
[013 の表](../013-library-search/contracts/list-api.md#3-videosort-の値)と同じ形で次を足す。

| 並べ替え | 昇順 | 降順 | 使う値 | 値が無いとき |
| --- | --- | --- | --- | --- |
| お気に入りにした日時 | `favoritedAsc` | `favoritedDesc` | `fav.favorited_at` | NULL。向きに関係なく末尾（`nullable`） |

### ライブラリの項目（`libraryItemsCTE`、`ListLibrary`・`LibraryIDs`）

[027 §1](../027-partial-group-search/contracts/library-api.md#1-get-apilibrary-の項目の作り方) の `matched`・`gm`・
`live`・`hits`・`whole`・`mv` は変えない。`items` を次のように変える（`favoriteOnly` は `FavoriteOnly`、
`gf(group_id)` はフォルダが `folder_favorites` にあるグループ、`vf(video_id)` は鍵が `video_favorites` にある
動画）。

| 項目 | 今の条件 | 足す条件 |
| --- | --- | --- |
| グループの項目 | `whole` のグループ（再生可否はメンバーのどれか） | `not favoriteOnly or group_id in gf` |
| 動画の項目 | 当たった動画で `whole` のグループに属さない（再生可否はその動画） | `favoriteOnly` のとき: 属するグループが `whole` でも `gf` に無ければ項目にする。さらに `video_id in vf` |

`items` に `favorited_at` 列を足す。動画の項目は `video_favorites.favorited_at`、グループの項目は
`folder_favorites.favorited_at`、無ければ NULL。`itemOrderValues` の `favoritedAsc`・`favoritedDesc` はこの列で、
`listOrders` と同じ `nullable`。値が同じなら `id` で決着させる。

視聴状態・再生可否・`total`・keyset・`LibraryIDs` は、この `items` に今までどおり掛かる。

`LibraryIDs` は動画の項目の id と、グループの項目ごとのフォルダのパスとメンバーの id を分けて返す
（`domain.LibrarySelection{VideoIDs []int64; Groups []LibraryGroupSelection{Path string; VideoIDs []int64}}`）。
`internal/httpapi` が今の `ids`（全部の和）と新しい `groups` を作る
（[contracts/screen-api.md §3](contracts/screen-api.md#3-get-apilibraryids-に足す項目)）。

受け入れ条件との対応:

| 受け入れ条件 | `favorite=true` の一覧 |
| --- | --- |
| 4（A だけがお気に入り、G はお気に入りでない） | G は `gf` に無いのでグループの項目にならず、A は `vf` にあるので動画の項目。G の他のメンバーは出ない |
| 5（G がお気に入り） | G は `whole`（絞り込みが無い）かつ `gf` にあるのでグループの項目 1 件。メンバーは動画の項目にならない |
| 9（タグと同時） | `matched` にタグが掛かる。一部のメンバーだけが当たった G は `whole` でないので、当たったメンバーのうち `vf` にあるものが動画の項目 |

### ゲスト

`CheckVideoQuery` が `400` にするので、ゲストの一覧の SQL に `FavoriteOnly`・`favorited*` は来ない。
`Video.Favorite`・`LibraryGroup.Favorite` の列は読むが、`internal/httpapi` がゲストの応答から省く。
