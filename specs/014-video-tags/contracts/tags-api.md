# Contract: タグの API

親 Issue: #193。Plan: [plan.md](../plan.md)。

API の正本は [api/openapi.yaml](../../../api/openapi.yaml) で、この文書は足す経路・
スキーマ・誤りの差分だけを書く。誤りの形（`Error {code, message}`）、同一オリジンの
検査、JSON 本文の読み方は既存のまま使う（`internal/httpapi/router.go`・
`media_folders.go` の `readJSONBody`）。本文を持つ経路は、`requiresJSONBody` と
`openapi_routes_test.go` の対応にも足す。`message` は既存と同じく画面にそのまま出せる
日本語にする。

## 1. スキーマ

```yaml
TagRef:            # 動画に付いたタグ。名前は常に元の名前
  type: object
  required: [id, name]
  properties:
    id:   { type: integer, format: int64 }
    name: { type: string }

Tag:               # 管理画面と候補の1件
  type: object
  required: [id, name, synonyms, videoCount]
  properties:
    id:         { type: integer, format: int64 }
    name:       { type: string }
    synonyms:   { type: array, items: { type: string } }   # 名前の自然順
    videoCount: { type: integer }  # いまライブラリにある動画の本数（data-model.md §5）

TagInput:          # 付与で使うタグの指定
  type: object
  additionalProperties: false
  properties:
    id:   { type: integer, format: int64 }
    name: { type: string }
```

`TagInput` は `id` と `name` のちょうど一方を持つ。両方あるか、どちらも無いときは
`invalid_request`（400）にする。`oneOf` にしないのは、既存のスキーマがすべて
`additionalProperties: false` の平たい object で、`oneOf` から生成される Go と
TypeScript の型を扱う前例がリポジトリに無いためである。

`Video` に、必須の `tags: TagRef[]` を足す。並びは名前の自然順（`domain.CompareNatural`、
同じなら `id`）である。`Video` を返す経路はすべてこの欄を埋める。今それに当たるのは、
`progressFor` を呼んでいる次の5つである。

- `listVideos`（`GET /api/videos`）
- `getVideo`（`GET /api/videos/{id}`）
- `listFolderVideos`（`GET /api/folders/{rootId}/videos`）
- `getRelatedVideos`（`GET /api/videos/{id}/related`）
- `reprobeVideo`（`POST /api/videos/{id}/probe`）

タグが1つも無い動画は空の配列を返し、`null` にしない。フォルダ画面は、この欄を受け取っても
表示しない（Plan の Structural Decisions 10）。

## 2. 足す誤りの `code`

| code | 状態 | 意味 |
| --- | --- | --- |
| `tag_not_found` | 404 | 指定したタグがもう無い（別のタブで削除・統合された） |
| `tag_name_taken` | 409 | その名前は既に別のタグの名前かシノニムである。`message` はどのタグの名前か、どのタグのシノニムかを示す |
| `tag_merge_required` | 409 | シノニムにしようとした名前が既存のタグの元の名前で、統合の承諾が無い |

名前が空・100 符号位置超は、既存の `invalid_request`（400）にする（[data-model.md §2](../data-model.md#2-名前の規則)）。

## 3. タグの管理

| 経路 | 本文 | 成功 | 誤り |
| --- | --- | --- | --- |
| `GET /api/tags` | — | 200 `{ items: Tag[] }`。名前の自然順、本数 0 を含む | — |
| `POST /api/tags` | `{ name }` | 201 `Tag` | 400、409 `tag_name_taken` |
| `PATCH /api/tags/{id}` | `{ name }` | 200 `Tag`（今と同じ名前なら何も変えずに返す） | 400、404 `tag_not_found`、409 `tag_name_taken` |
| `DELETE /api/tags/{id}` | — | 204 | 404 `tag_not_found` |
| `POST /api/tags/{id}/merge` | `{ sourceId }` | 200 `Tag`（統合先） | 400（`sourceId` が `id` と同じ）、404 `tag_not_found`（どちらかが無い） |
| `POST /api/tags/{id}/synonyms` | `{ name, merge?: boolean }` | 200 `Tag` | 400、404 `tag_not_found`、409 `tag_name_taken`、409 `tag_merge_required` |
| `DELETE /api/tags/{id}/synonyms?name=…` | — | 204（その名前がこのタグのシノニムでなければ、何も変えずに 204） | 404 `tag_not_found`（タグが無い） |

- 既にこのタグのシノニムである名前の登録は、何も変えずに 200 を返す。このタグの元の
  名前の登録は 409 `tag_name_taken` にする（[data-model.md §4](../data-model.md#4-書き換えの規則)）。
- `merge` を省くと偽である。偽で名前が別のタグの元の名前なら `tag_merge_required` を返し、
  何も変えない。真なら、そのタグをこのタグへ統合してから名前をシノニムにする
  （受け入れ条件 17）。画面は、`GET /api/tags` の本数で先に確認をとってから `merge: true`
  で送る。`tag_merge_required` は、確認の後に別のタブで状態が変わったときの守りである。
- シノニムの解除で名前をパスに置かないのは、`/` や `%` を含む名前を1段のパスとして
  扱う取り決めを増やさないためである。
- 削除と統合の確認に出す本数は、`GET /api/tags` の `videoCount` を使う。確認のための
  経路は足さない。

## 4. 付与と取り外し

| 経路 | 本文 | 成功 | 誤り |
| --- | --- | --- | --- |
| `POST /api/video-tags` | `{ videoIds, action: "add", tag: TagInput }` | 200 `{ tag: TagRef, applied }` | 400、404 `tag_not_found`（`tag.id` が無い） |
| `POST /api/video-tags` | `{ videoIds, action: "remove", tag: TagInput }` | 200 `{ tag: TagRef, applied }` | 400（`name` での指定）、404 `tag_not_found` |
| `POST /api/video-tags/summary` | `{ videoIds }` | 200 `{ total, items: [{ tag: TagRef, count }] }` | 400 |

- 再生画面の1本も、選択バーの複数本も、同じ `POST /api/video-tags` を使う。
- `videoIds` は 1 件以上 20000 件以下。重複は1つとして扱う。20000 は、規模の前提
  （1万本）の全件を選んでも収まる数で、`id` が最大の 19 桁でも本文が `readJSONBody` の
  1 MiB に収まる（20000 × 20 バイト）。上限を設けない案は、本文の上限で 400 になる
  境目が `id` の桁数で変わり、利用者に説明できないので採らない。サーバーは `json_each` に
  1つの引数で渡し、SQLite の引数の上限に掛からないようにする。
- 取り外しは `name` で指定できない。画面が外す候補はいつも付いているタグで、`id` を
  持っているからである。
- `tag: { name }` の付与は、名前をシノニムを含めて引き、無ければ作る
  （[data-model.md §3・§4](../data-model.md#3-名前の引き方)）。
- `applied` は、`videoIds` のうち、いまライブラリにある動画の数である。既に付いていた・
  付いていなかった動画も数に入る（重複した付与と取り外しは誤りにしない）。
- `summary` の `total` は `videoIds` のうちライブラリにある動画の数、`count` はそのうち
  そのタグが付いている数である。`count < total` のタグが「一部にだけ付いている」
  （要件 2）。`items` は1本以上に付いているタグだけで、名前の自然順に並ぶ。
- 処理は1つのトランザクションで、全部に反映するか1つも反映しない。

## 5. 一覧の絞り込みと「すべて選択」

#195 の `specs/013-library-search/contracts/list-api.md` の一覧に、次を足す。

| 名前 | 型 | 既定 | 意味 |
| --- | --- | --- | --- |
| `tag` | integer の配列（`tag=3&tag=8`、最大 16 個） | 空 | 各タグをすべて持つ動画だけにする（AND）。存在しない `id` は無視する |

`VideoPage` に、任意の `missingTagIds: integer[]` を足す。`tag` のうち存在しなかった
`id` で、1つも無ければ省く。画面はこれを受けて、そのタグがもう無いことを伝え、タグの
一覧を取り直し、URL からその `id` を取り除く（[list-url.md §1](list-url.md#1-パラメータ)、
Edge Case「ほかの画面での並行した変更」）。無い `id` を `404` にしないのは、同じ Edge Case が
「URL に残った存在しないタグの絞り込みは無視し、ほかの条件だけで一覧を出す」と求めるから
である。

- `listVideos`（`GET /api/videos`）に足す。`listFolderVideos` には足さない（フォルダ画面の
  タグ絞り込みは対象外）。
- `total` は、`tag` も含めたすべての条件を適用した数である。
- 17 個以上は `400` にする。画面の選択欄が 16 個を超えさせない。

「すべて選択」のために、次の経路を足す。

| 経路 | パラメータ | 成功 |
| --- | --- | --- |
| `GET /api/videos/ids` | `listVideos` の `query`・`watch`・`playable`・`tag` | 200 `{ ids: integer[] }` |

- 返す `id` の集合は、同じ条件の `listVideos` の全ページの `id` の集合と同じである。
  並びは決めない。
- `/api/videos/{id}` とは、Go の `ServeMux` の「字面の段が優先する」規則で区別される。
  `openapi_routes_test.go` にこの経路が `{id}` に取られないことの検査を足す。
