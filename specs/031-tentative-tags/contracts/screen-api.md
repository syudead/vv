# Contract: 画面の API の仮のタグと却下した名前

正本は `api/openapi.yaml` で、この文書は足す項目・経路・誤りの差分だけを書く。誤りの形、同一オリジンの
検査、JSON 本文の読み方、`requiresJSONBody` と `openapi_routes_test.go` への追加は
[specs/014-video-tags/contracts/tags-api.md](../../014-video-tags/contracts/tags-api.md) のまま。
決定は [research.md R-5・R-8・R-9](../research.md)。

## 0. スキーマの差分

| スキーマ | 足す項目 | 規則 |
| --- | --- | --- |
| `TagRef` | `tentative: boolean`（`required`） | 動画に付いたタグ、`POST /api/video-tags` の応答の `tag`、`summary` の `items[].tag` に出る |
| `VideoTag` | `tentative: boolean`（`required`） | `Video.tags` の 1 件。フォルダ由来だけで付いているタグも、そのタグの状態を出す |
| `Tag` | `tentative: boolean`（`required`） | `GET /api/tags` と、作成・改名・統合・シノニム・確定の応答 |

新しいスキーマ:

```yaml
RejectedTagNameList:
  type: object
  required: [items]
  additionalProperties: false
  properties:
    items:
      type: array
      description: 却下した名前。名前の自然順
      items: { type: string }
```

`Error.code` に `tag_not_tentative` を足す。

## 1. 変わる既存の経路

| 経路 | 変わること |
| --- | --- |
| `PATCH /api/tags/{id}` | 名前が変わるとき、仮のタグは確定したタグになって返る（[data-model.md §3](../data-model.md#3-書き換えの規則)）。今と同じ名前なら何も変えない |
| `POST /api/tags/{id}/synonyms` | 名前を足すとき、仮のタグ `{id}` は確定したタグになって返る |
| `POST /api/tags/{id}/merge` | 統合先 `{id}` は確定したタグになって返る。統合元は仮でも確定でも消える |
| `POST /api/tags`、`POST /api/video-tags`（`tag: { name }` で作るとき） | 今と同じく確定したタグを作る。その名前が却下した名前なら一覧から消える（要件 15） |
| `DELETE /api/tags/{id}` | 変えない。仮でも確定でも名前を覚えない。画面は仮のタグにこの操作を出さない（要件 10） |

## 2. 仮のタグの操作

| 経路 | 本文 | 成功 | 誤り |
| --- | --- | --- | --- |
| `POST /api/tags/{id}/confirm` | — | 200 `Tag`（`tentative: false`。既に確定していても何も変えずに返す） | 404 `tag_not_found` |
| `POST /api/tags/{id}/reject` | — | 204。タグは消え、付いていた動画から外れ、元の名前が却下した名前の一覧に入る | 404 `tag_not_found`、409 `tag_not_tentative`（確定したタグ。何も変えない） |

- どちらも所有者だけ（`security` は既存のタグの操作と同じ。境界の既定の分類で `401`）。
- 画面は `tag_not_tentative` を `tag_not_found` と同じく扱う（窓を閉じ、一覧を取り直す。Edge Case
  「操作の競合」）。
- `reject` のあと、画面は却下した名前の一覧（§3）を取り直す。応答に名前を載せないのは、一覧の
  取り直しで足り、ほかのタブの却下も同じ経路で反映するためである。

## 3. 却下した名前

| 経路 | 本文 | 成功 | 誤り |
| --- | --- | --- | --- |
| `GET /api/tags/rejected-names` | — | 200 `RejectedTagNameList` | — |
| `DELETE /api/tags/rejected-names?name=…` | — | 204（その名前が無くても、何も変えずに 204。Edge Case） | 400（`name` が無い） |

- 所有者だけ。
- `name` は `domain.NormalizeTagName` で整えてから照合する（登録時と同じ整え方。
  `DELETE /api/tags/{id}/synonyms` と同じ）。整えられない入力は何にも一致せず 204。
- `/api/tags/rejected-names` は `/api/tags/{id}` と、`ServeMux` の「字面の段が優先する」規則で
  区別される。`openapi_routes_test.go` に、`GET`・`DELETE` のこの経路が `{id}` に取られないことの
  検査を足す（`/api/videos/ids` と同じ）。
- 外した名前は、次に `tentative: true` で付けたとき再び仮のタグとして作られる（受け入れ条件 13）。

## 4. `web/src/api` の関数

`client.ts`（または `tags.ts`）に `confirmTag(id)`、`rejectTag(id)`、`listRejectedTagNames()`、
`forgetRejectedTagName(name)` を足す。`confirmTag`・`rejectTag` は `renameTag`・`deleteTag` と同じく
共有の一覧を取り直す。`errorText` に `tag_not_tentative` の文言を足す。
