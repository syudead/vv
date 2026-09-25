# Contract: ライブラリの一覧（動画とグループの項目）

正本は `api/openapi.yaml` で、この文書は足す経路とスキーマだけを書く。誤りの形・パラメータの検査・
カーソルの不透明さは [013 の list-api.md](../../013-library-search/contracts/list-api.md) と
[014 の tags-api.md §5](../../014-video-tags/contracts/tags-api.md) に従う。問い合わせの中身は
[data-model.md §5](../data-model.md#5-ライブラリの項目)。

`GET /api/videos` は1本ずつの一覧のまま変えない（フォルダ画面のルートの検索結果が使う）。
`GET /api/videos/ids` は、ライブラリを `GET /api/library/ids` に切り替える単位で消す。

## 1. `GET /api/library`

- パラメータ: `GET /api/videos` と同じ（`query`・`watch`・`playable`・`sort`・`seed`・`cursor`・`limit`・`tag`）。
- 応答 `LibraryPage`:

```yaml
LibraryPage:
  required: [items, total]
  properties:
    items: { type: array, items: { $ref: LibraryItem } }
    total: { type: integer }        # 絞り込み後の項目（カード）の数
    nextCursor: { type: string }
    missingTagIds: { type: array, items: { type: integer, format: int64 } }

LibraryItem:
  required: [kind]
  properties:
    kind: { type: string, enum: [video, group] }
    video: { $ref: Video }          # kind = video のときだけ
    group: { $ref: LibraryGroup }   # kind = group のときだけ

LibraryGroup:
  required: [folder, name, videoCount, watchedCount, watchState, sizeBytes, addedAt,
             cover, openVideoId, videoIds, tags]
  properties:
    folder: { $ref: VideoFolder }   # グループのフォルダ。これがグループを指す鍵
    name: { type: string }
    videoCount: { type: integer }
    watchedCount: { type: integer }
    watchState: { type: string, enum: [unwatched, inProgress, watched] }
    durationMs: { type: integer, format: int64 }   # 1本も分からなければ省く
    sizeBytes: { type: integer, format: int64 }
    addedAt: { type: string, format: date-time }
    lastPlayedAt: { type: string, format: date-time }  # 無ければ省く
    cover: { $ref: Video }          # 並びで最初のメンバー。サムネイルと hover プレビューに使う
    openVideoId: { type: integer, format: int64 }  # 押したときに開くメンバー（data-model.md §6）
    videoIds: { type: array, items: { type: integer, format: int64 } }  # 全メンバー、並びの順
    tags: { type: array, items: { $ref: VideoTag } }  # メンバーのタグの和集合（出所も和）
```

グループの `id` は応答に出さない。作り直しで変わるためで、グループは `folder` で指す。

## 2. `GET /api/library/ids`

- パラメータ: `GET /api/library` から `sort`・`seed`・`cursor`・`limit` を除いたもの。
- 応答: 既存の `VideoIdsResponse`（`ids`・`missingTagIds`）。`ids` は当たった項目の動画の `id` と、
  当たったグループの**全メンバー**の `id`。上限と誤りは今の `GET /api/videos/ids` と同じ。

## 3. グループ1件

`GET /api/folders/{rootId}/group?path=…`

- 一覧に残っているグループのカードを取り直すための経路（plan の Structural Decisions 10）。
- 応答 200: `LibraryGroup`。絞り込みに関係なく、グループの全メンバーから作る。
- 400: パスの規則違反（`FolderPath` と同じ）。
- 404 `not_found`: そのフォルダが今グループでない、または無い。画面はそのカードを一覧から外す。
