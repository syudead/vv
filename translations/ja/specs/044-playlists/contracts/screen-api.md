---
source: specs/044-playlists/contracts/screen-api.md
sourceHash: 37c212bbdb29742d13c58a9d2c93713ca00a966830c1164aa40091ffd8dd6122
---

# 契約: プレイリストの画面の API {#contract-screen-api-for-playlists}

親 Issue: #787。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは加えるルートとスキーマだけを挙げる。既存のルートは変わらない。`task generate` が `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API (`api/external-v1.yaml`) と MCP は変わらない (範囲外)。

すべてのルートは `/api/*` の既定により所有者専用なので、`accessRoutes` には何も加わらず、ゲストは `401` を受ける ([research.md R-8](../research.md#r-8-owner-only-through-the-existing-rules))。`POST` と `PATCH` のルートは `requiresJSONBody` に加わる。

| ルート | 目的 |
| --- | --- |
| `GET /api/playlists` | プレイリストの一覧の 1 ページ |
| `POST /api/playlists` | 1 つ作成する |
| `GET /api/playlists/{id}` | 1 つのプレイリストと、その表示される項目 |
| `PATCH /api/playlists/{id}` | 名前を変える |
| `DELETE /api/playlists/{id}` | 削除する |
| `POST /api/playlists/{id}/items` | 動画とグループを加える |
| `POST /api/playlists/{id}/items/{itemId}/move` | 1 つの項目を並べ替える |
| `DELETE /api/playlists/{id}/items/{itemId}` | 1 つの項目を取り除く |
| `GET /api/playlists/{id}/playback` | 動画のページのための位置と次の一歩 |

## スキーマ {#schemas}

```yaml
PlaylistSummary:
  required: [id, name, itemCount, updatedAt]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }
    itemCount: { type: integer }        # shown items (R-4)
    updatedAt: { type: string, format: date-time }
    covers: { type: array, maxItems: 4, items: { $ref: FolderPreview } }   # GET /api/playlists only; see below

PlaylistList:
  required: [items, total]
  properties:
    items: { type: array, items: { $ref: PlaylistSummary } }   # one page, updatedAt desc then id desc
    total: { type: integer }        # every playlist, independent of paging
    version: { type: string }       # opaque; changes with any create, delete, rename or item change
    nextCursor: { type: string }    # only while more follow

Playlist:
  allOf: [PlaylistSummary without covers]
  required: [items]
  properties:
    items: { type: array, items: { $ref: PlaylistItem } }   # position order, shown items only

PlaylistItem:
  required: [id, kind]
  properties:
    id: { type: integer, format: int64 }
    kind: { type: string, enum: [video, group] }
    video: { $ref: Video }          # kind video
    group: { $ref: LibraryGroup }   # kind group; openVideoId is where the item starts

PlaylistName:
  required: [name]
  properties:
    name: { type: string, minLength: 1, maxLength: 100 }   # trimmed by the server (R-11)
```

## `GET /api/playlists` {#get-apiplaylists}

クエリ: `cursor` (前の応答の `nextCursor`、中身は不透明) と `limit` (1 から 200、既定は 60)。ライブラリの一覧のページングのパラメータである。

`200 PlaylistList`: `cursor` より後のプレイリストを、`updatedAt` の降順、次に `id` の降順で、最大 `limit` 個返す。`nextCursor` は続きがある間だけ返し、`total` はページングによらないすべてのプレイリストの数であり、`version` は一覧の順を表す不透明なトークンである。cursor はそのページの最後のプレイリスト (`updatedAt`、`id`) を指す。所有者がページを送る間に変わったプレイリストは cursor より上に移り、cursor より後には決して返らないので、サーバーは `version` をプレイリストの数とすべての `updated_at` の和から導く。この 2 つは作成、削除、名前の変更、項目の変更で必ず変わる (`updated_at` は変更のたびに、1 ミリ秒の内でも、厳密に進む)。クライアントは各ページの `version` を最初のページのものと比べ、異なるときは、追加する代わりに最初のページから一覧を読み直す。同じ `version` のページは同じ順を共有するので、重なることも飛ばすこともない。読めない `cursor` は `400 invalid_cursor` であり、範囲外の `limit` は `400 invalid_request` である。

各要約は、返したページの分だけ計算される、プレイリスト一覧の画面のカードの絵（[ui-design.md の Playlists screen](../ui-design.md#playlists-screen)）である `covers` を持つ。位置の順の表示される項目がそれぞれ 1 つの `FolderPreview` を出す。動画の項目はその動画を、グループの項目はグループの順でサムネイルを持つ最初のメンバー（そのグループのカードが最初に出すメンバー）を出す。サムネイルを生成済みの動画だけを残し、最大 4 つで、形と `previewUrl` の規則は `LibraryGroup.previews` のものである。空のプレイリスト、または表示される項目がサムネイルを持たないプレイリストは `covers: []` を持つ。見える規則は `itemCount` のもの（R-4）である: 隠れた項目は何も出さない。`POST`、`PATCH`、`GET /api/playlists/{id}`、`PlaylistPlayback.playlist` は `covers` を省く。それらの画面は項目を持つか、絵を必要としない。

## `POST /api/playlists`、`PATCH /api/playlists/{id}`、`DELETE /api/playlists/{id}` {#post-apiplaylists-patch-apiplaylistsid-delete-apiplaylistsid}

| ルート | 成功 | 失敗 |
| --- | --- | --- |
| `PlaylistName` 付きの `POST` | `201 PlaylistSummary` | トリム後に空か 100 文字を超える名前で `400 invalid_request` |
| `PlaylistName` 付きの `PATCH` | `200 PlaylistSummary` | 上と同じ `400`。`404 not_found` |
| `DELETE` | `204` | `404 not_found` |

## `GET /api/playlists/{id}` {#get-apiplaylistsid}

`200 Playlist`。`404 not_found`。`video` と `group` は、ライブラリの項目と同じく所有者のフィールド (`progress`、`watchState`、`watchedCount`、`favorite`) を持つ。同じ応答が、プレイリストの画面と、動画の画面のプレイリストのパネル（[ui-design.md の Playlist panel](../ui-design.md#playlist-panel)）に使われる。パネルは、画面が必要としないものを何も必要としない。

## `POST /api/playlists/{id}/items` {#post-apiplaylistsiditems}

```yaml
PlaylistItemsAdd:
  properties:
    videoIds: { type: array, maxItems: 20000, items: { type: integer, format: int64 } }
    folders: { type: array, maxItems: 20000, items: { $ref: VideoFolder } }   # rootId and path, as PUT /api/favorites
PlaylistItemsAdded:
  required: [added, existing]
  properties:
    added: { type: integer }      # appended to the end
    existing: { type: integer }   # resolved but already in the playlist
```

| 条件 | 応答 |
| --- | --- |
| いくつかの id かフォルダが解決する | `200 PlaylistItemsAdded`。ライブラリにない id と、今グループではないフォルダはどちらの数にも数えず、繰り返した id かフォルダは 1 回だけ数える ([R-5](../research.md#r-5-one-owner-only-bulk-add-for-videos-and-groups-appended-in-request-order)) |
| 両方の配列が空、または `FolderPath` の規則を破る `folders.path` | `400 invalid_request` |
| 異なる id とフォルダが合わせて 20000 を超える | `PUT /api/favorites` と同じく `400 too_many_videos` |
| プレイリストが存在しない | `404 not_found` |

## `POST /api/playlists/{id}/items/{itemId}/move` {#post-apiplaylistsiditemsitemidmove}

本文は `{afterItemId: integer | null}`。`null` は項目を先頭に移す。`204`。プレイリスト、項目、`afterItemId` がそこにないときは `404 not_found`。`afterItemId` が `itemId` と等しいときは `400` ([R-3](../research.md#r-3-dense-integer-positions-reordered-by-move-after))。

## `DELETE /api/playlists/{id}/items/{itemId}` {#delete-apiplaylistsiditemsitemid}

`204`。どちらかがなくなっているときは `404 not_found`。動画かグループ、そのお気に入り、その再生位置は変わらない (受け入れ条件 4)。

## `GET /api/playlists/{id}/playback` {#get-apiplaylistsidplayback}

クエリ: `item` (必須、再生中の項目)、`video` (必須、ページで開いている動画)。

```yaml
PlaylistPlayback:
  required: [playlist, itemId, position, count]
  properties:
    playlist: { $ref: PlaylistSummary }
    itemId: { type: integer, format: int64 }
    position: { type: integer }   # 1-based among shown items; 0 when the current item is not shown (data-model.md, Playback order)
    count: { type: integer }
    next: { $ref: PlaylistStep }          # absent at the end of the playlist
    previousItem: { $ref: PlaylistStep }  # start of the previous shown item
    nextItem: { $ref: PlaylistStep }      # start of the next shown item
PlaylistStep:
  required: [itemId, video]
  properties:
    itemId: { type: integer, format: int64 }
    video: { $ref: Video }
```

| 条件 | 応答 |
| --- | --- |
| プレイリストと項目が存在する | `200`。[data-model.md、Playback order](../data-model.md#playback-order) で決まる |
| プレイリストか項目がなくなっている | `404 not_found`。ページはプレイリストの文脈を捨てる ([R-7](../research.md#r-7-a-playlist-playback-overrides-the-group-and-folder-neighbours)) |
| `item` か `video` がない、または整数ではない | `400 invalid_request` |

## クライアントの使い方 {#client-use}

| 画面 | 呼び出し |
| --- | --- |
| プレイリストの一覧の画面 (`/playlists`) | 既定の `limit` で `GET /api/playlists`、その後、グリッドの末尾が見えてくるたびにそれぞれの `nextCursor` で読む。`version` が読み込んだページのものと異なるページは捨て、それらの代わりに最初のページを読み直す ([ui-design.md の Paging](../ui-design.md#paging))。`POST /api/playlists` |
| プレイリストの画面 (`/playlists/{id}`) | `GET`、`PATCH`、`DELETE /api/playlists/{id}`。`move` と項目の `DELETE`。再生は `state: {from: "/playlists/{id}", autoplay: true}` 付きで `/videos/{videoId}?playlist={id}&item={itemId}` を開く。`videoId` は動画か、グループの `openVideoId` である |
| 追加メニュー (カード、選択バー、動画のページ) | 開いたときに `limit=200` で `GET /api/playlists` を読み、最後のページまで `nextCursor` を辿る。メニューは一覧全体を名前でクライアント側で絞るからである (これらのページの `covers` は使わない。メニューのページは少ないので、それを省くパラメータは加えなかった)。ページの `version` が最初のページのものと異なるときは、最初のページから辿り直す。辿り直しは 3 回までで、その後は欠けのある一覧を出さずに失敗する。`POST /api/playlists/{id}/items`。メニューからプレイリストを作成するのは `POST /api/playlists` の後に追加である |
| `playlist` と `item` 付きの動画のページ、所有者だけ | 開いたとき、動画が終わったとき、通知が次の動画がなくなったことに気づいたときに `GET /api/playlists/{id}/playback`。それらの読み込みのそれぞれと並べて、横の列のプレイリストのパネル（[ui-design.md の Playlist panel](../ui-design.md#playlist-panel)）のために `GET /api/playlists/{id}` を読む。組は両方が同じ `updatedAt` を持つときだけ使い、異なるときは読み直すので（[ui-design.md の One order](../ui-design.md#one-order)）、パネルと `next` は同じ順を示す。どちらの読み込みからの `404` でも文脈を外す。一歩へ移るときは `playlist` を保ち、`item` をその一歩の `itemId` にし、`state.from` を引き継ぐ |
