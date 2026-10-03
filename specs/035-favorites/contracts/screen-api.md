# Contract: 画面の API の差分

親 Issue: #574。正本は [api/openapi.yaml](../../../api/openapi.yaml) で、ここには足す経路・項目・値だけを
書く。`task generate` で `internal/httpapi/gen/` と `web/src/api/gen/` を作り直す。外部連携 API
（`api/external-v1.yaml`）は変えない（対象外）。

## 0. `Video` と `LibraryGroup` に足す項目

```yaml
Video:
  properties:
    favorite:
      type: boolean
      description: |
        所有者がお気に入りにした動画か。所有者の応答にだけ入る
        （specs/035-favorites/data-model.md §3）

LibraryGroup:
  properties:
    favorite:
      type: boolean
      description: |
        所有者がグループをお気に入りにしたか。メンバーの動画のお気に入りとは独立で、
        所有者の応答にだけ入る（specs/035-favorites/data-model.md §3）
```

- `Video.favorite` は動画を返すすべての応答（一覧、`GET /api/videos/{id}`、関連、バージョン、
  `GET /api/folders/{rootId}/videos`、probe の再試行、表示名・サムネイルの位置の応答）に、所有者にだけ入る。
  `required` にはしない（ゲストの応答で省く）。`forAudience` が `public` と違って `favorite` を落とす。
- `LibraryGroup.favorite` は `GET /api/library` のグループの項目と `GET /api/folders/{rootId}/group` に、
  所有者にだけ入る。ゲストの応答では `watchState` などと同じく省く。
- お気に入りにした日時は応答に出さない（並び順はサーバーが決める）。

## 1. `PUT /api/favorites`

所有者だけ（`/api/*` の既定の分類）。`videoIds` の動画と `folders` のグループのお気に入りを `favorite` に
そろえる（[research.md R-2](../research.md#r-2-付け外しは所有者だけの-1-つの経路-put-apifavorites-で動画の-id-とフォルダを-1-つの取引で受け無いものは数えずに飛ばす)、
[data-model.md §4](../data-model.md#4-書き込みfavoritestore)）。

```yaml
/api/favorites:
  put:
    operationId: updateFavorites
    requestBody: FavoritesRequest
    responses:
      "200": FavoritesResponse
      "400": InvalidRequest

FavoritesRequest:
  required: [favorite]
  properties:
    videoIds:  { type: array, items: { type: integer, format: int64 } }
    folders:   { type: array, items: { $ref: VideoFolder } }   # グループのフォルダ（rootId と path）
    favorite:  { type: boolean }

FavoritesResponse:
  required: [appliedVideos, appliedFolders]
  properties:
    appliedVideos:  { type: integer }   # videoIds のうちいまライブラリにある異なる動画の id の数（同じ集まりの id も 1 本ずつ数える）
    appliedFolders: { type: integer }   # folders のうちいまグループのフォルダの数
```

| 条件 | 応答 |
| --- | --- |
| `videoIds` と `folders` の合計が 1 以上 20000 以下でない（重複は 1 つに数える） | `400 invalid_request`（`too_many_videos`、`limit` は 20000。`POST /api/video-tags` と同じ） |
| `folders` の `path` が `ValidateFolderPath` に通らない | `400 invalid_request`（`invalid_folder_path`） |
| `videoIds` にライブラリに無い id、空の `content_key` の動画 | 誤りにせず `appliedVideos` に数えない |
| `folders` の `rootId` が登録フォルダに無い、またはそのフォルダが今グループでない | 誤りにせず `appliedFolders` に数えない |
| 既に同じ状態 | 誤りにせず数える。付いているものに付けても日時は変えない |

`folders` の各要素は `GET /api/folders/{rootId}/group` と同じく `domain.FolderDir(root.Path, path)` で絶対パスに
し、保存層がフォルダの鍵にする。処理は 1 つの取引で、全部に反映するか 1 つも反映しない。確定後の
`/api/events` の知らせは無い（[research.md R-6](../research.md#r-6-画面はドメインイベントを足さず公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し再生画面は動画を取り直す)）。
`videoIds` にグループのメンバーを入れればその動画に付く。`folders` にグループを入れてもメンバーには
付かない（要件 4）。

## 2. 一覧の `favorite` と `VideoSort` に足す値

`GET /api/videos`、`GET /api/folders/{rootId}/videos`、`GET /api/library`、`GET /api/library/ids` に
`favorite`（boolean、既定 false）を足す。true でお気に入りのみにする。項目ごとに自分のお気に入りで判定し
（動画の項目は動画、グループの項目はグループ）、`query`・`tag`・`watch`・`playable` と AND で組み合わさる。
お気に入りでないグループに属するお気に入りの動画は動画の項目になる
（[data-model.md §5](../data-model.md#5-読み出しと一覧)）。`GET /api/library` と `GET /api/library/ids` の
`description` にその規則を足す。

```yaml
VideoSort:
  enum: [..., playedAsc, playedDesc, favoritedAsc, favoritedDesc, random]
  description: |
    …、favorited = お気に入りにした日時（お気に入りでない項目は向きに関係なく末尾。グループの項目は
    グループをお気に入りにした日時）、…
```

`GET /api/videos`、`GET /api/folders/{rootId}/videos`、`GET /api/library` の `sort` で受け付ける。
`GET /api/library/ids` は `sort` を持たず、変わらない。

ゲスト（[guest-api.md §3](../../016-single-account-auth/contracts/guest-api.md#3-ゲストが使えない条件) の表に足す）。
ゲストが読める `GET /api/videos`・`GET /api/folders/{rootId}/videos`・`GET /api/library` の 3 経路に掛かる。
`GET /api/library/ids` は所有者だけの経路で、ゲストには条件によらず今までどおり `401` を返す:

| 条件 | ゲストでの扱い |
| --- | --- |
| `favorite` が true | `400 invalid_request`（`guest_filter_not_allowed`） |
| `sort` が `favoritedAsc`・`favoritedDesc` | `400 invalid_request`（`guest_filter_not_allowed`） |

`guest_filter_not_allowed` の `message` に「お気に入り」を足す。

## 3. `GET /api/library/ids` に足す項目

```yaml
VideoIdsResponse:
  properties:
    groups:
      type: array
      description: |
        条件に合うグループの項目。各要素のフォルダと、そのグループの全メンバーの id（ids にも含まれる）。
        listLibraryIds の応答にだけ入り、1 つも無ければ省略される
      items:
        $ref: LibraryGroupIds

LibraryGroupIds:
  required: [folder, videoIds]
  properties:
    folder:   { $ref: VideoFolder }
    videoIds: { type: array, items: { type: integer, format: int64 } }
```

`ids` は今までどおり動画の項目の id とグループの項目の全メンバーの id の和で、並びは決めない。
`groups` は画面が「選んだグループ」を知るためのもので、選択バーのお気に入りが `folders` に送る
（[research.md R-7](../research.md#r-7-複数選択は選んだグループをグループとして覚え一括のお気に入りではグループのメンバーを動画として送らない)）。
`POST /api/video-tags/summary` など `VideoIdsResponse` を返す他の経路は `groups` を入れない。

## 4. 変わらない経路

`PUT /api/video-visibility`・`POST /api/video-tags`・`POST /api/video-bundles` は今のまま動画の id の集合を
受ける。`GET /api/folders/{rootId}/group` の引数は変わらない（応答に `favorite` が足される）。
外部連携 API と MCP は変わらない。

## 5. `web/src/api` の差分

- `videoSorts` に `favoritedAsc`・`favoritedDesc` を足す。画面の一覧の条件は `isListSort` で `sortKinds` に
  種類があるものだけを受け付ける（033 §3 と同じ）。
- `ListFilterParams` に `favorite?: boolean` を足し、`listVideos`・`listLibrary`・`listLibraryIds`・
  `listFolderVideos` が true のときだけ `favorite=true` を載せる。
- `favorites.ts` に `updateFavorites(videoIds, folders, favorite)`（`PUT /api/favorites`）と、結果の購読
  （[research.md R-6](../research.md#r-6-画面はドメインイベントを足さず公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し再生画面は動画を取り直す)）を置く。
- 生成された `Video`・`LibraryGroup` 型の `favorite` は省略可能なので、Vitest の fixture は追従不要。
