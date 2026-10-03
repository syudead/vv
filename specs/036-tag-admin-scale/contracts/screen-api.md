# Contract: 画面の API のページ読み・まとめての操作と `createdAt`

正本は [api/openapi.yaml](../../../api/openapi.yaml) で、この文書は足す項目・経路・パラメータ・誤りの差分
だけを書く。誤りの形、同一オリジンの検査、JSON 本文の読み方、`requiresJSONBody` と
`openapi_routes_test.go` への追加は [specs/014-video-tags/contracts/tags-api.md](../../014-video-tags/contracts/tags-api.md)
のまま。決定は [research.md R-1・R-4〜R-6・R-8・R-13・R-14](../research.md)。経路はすべて所有者だけ
（`security` は既存のタグの操作と同じ）。外部連携 API（`api/external-v1.yaml`）は変えない。

§1〜§3 と §0 の `Tag.createdAt`・まとめての操作のスキーマは feature branch に merge 済みで、親 Issue の
改訂で変わらない。§0 の一覧のスキーマ、§5・§6 と §4 の対応する関数は改訂で足した。

## 0. スキーマの差分

| スキーマ | 足す項目 | 規則 |
| --- | --- | --- |
| `Tag` | `createdAt: string, format: date-time`（`required`） | `tags.created_at`。`GET /api/tags` と、作成・改名・統合・シノニム・確定の応答に出る（merge 済み） |
| `TagList` | `total: integer`（`required`）、`totalAll: integer`（`required`）、`nextCursor: string`（任意） | `total` は条件（`q`・`tentative`・`unused`）に合うタグの数、`totalAll` は全部のタグの数。`nextCursor` は `limit` を付けた要求で続きがあるときだけ入る（`LibraryPage` と同じ） |
| `RejectedTagNameList` | `total: integer`（`required`）、`nextCursor: string`（任意） | `total` は却下した名前の全部の数 |

新しいスキーマ（merge 済みのまとめての操作）:

```yaml
TagBatchRequest:
  type: object
  required: [action, ids]
  additionalProperties: false
  properties:
    action: { type: string, enum: [confirm, reject, delete] }
    ids:
      type: array
      minItems: 1
      maxItems: 20000
      items: { type: integer, format: int64 }

TagBatchResponse:
  type: object
  required: [appliedIds, notFoundIds, notApplicableIds]
  additionalProperties: false
  properties:
    appliedIds:        { type: array, items: { type: integer, format: int64 } }  # 処理した id
    notFoundIds:       { type: array, items: { type: integer, format: int64 } }  # もう無かった id
    notApplicableIds:  { type: array, items: { type: integer, format: int64 } }  # 操作が働かない種類だった id

TagMergeResponse:
  type: object
  required: [tag, notFoundIds]
  additionalProperties: false
  properties:
    tag:         { $ref: "#/components/schemas/Tag" }                            # 統合後の統合先
    notFoundIds: { type: array, items: { type: integer, format: int64 } }        # もう無かった統合元

TagImpactRequest:
  type: object
  required: [action, ids]
  additionalProperties: false
  properties:
    action: { type: string, enum: [reject, delete, merge] }   # 確認をとる操作。数える種類がこれで決まる
    ids:
      type: array
      minItems: 1
      maxItems: 20000
      items: { type: integer, format: int64 }

TagImpactResponse:
  type: object
  required: [tagCount, videoCount]
  additionalProperties: false
  properties:
    tagCount:   { type: integer }   # ids のうち今あり、action が働くタグの数
    videoCount: { type: integer }   # そのどれかが付いた、いまライブラリにある動画の本数（重複なし）
```

新しいスキーマ（改訂で足す）:

```yaml
TagSort:
  type: string
  enum: [name, countDesc, countAsc, createdDesc, createdAsc]
  default: name
  # name は名前の自然順（向きは無い）。値が同じタグは名前の自然順、それも同じなら id
  # （specs/036-tag-admin-scale/data-model.md §0・§2）
```

`ErrorReason` に `too_many_tags` を足す（`limit` 付き、`too_many_videos` と同じ形。merge 済み）。

## 1. `POST /api/tags/batch`

| 本文 | 成功 | 誤り |
| --- | --- | --- |
| `TagBatchRequest` | 200 `TagBatchResponse` | 400 `invalid_request`（`action` が 3 値でない、`ids` が空、`too_many_tags`） |

- `ids` の重複は 1 つとして扱う。応答の 3 つの配列は互いに重ならず、合わせて重複を除いた `ids` になる。
  並びは `ids` に現れた順。空の配列は `[]`（`null` にしない）。
- 働く種類は [data-model.md §1](../data-model.md#1-domain-に足す値) の `TagBatchApplies`: `confirm`・`reject` は
  仮のタグ、`delete` は確定したタグ。合わない id は何も変えずに `notApplicableIds`。無い id は `notFoundIds`。
- 残りを 1 つの取引で処理する（[data-model.md §2](../data-model.md#2-保存層の操作)）。取引が失敗したら
  `500` で、何も変わらない。
- `/api/tags/batch` と `/api/tags/impact` は `/api/tags/{id}` と、`ServeMux` の「字面の段が優先する」規則で
  区別される。`openapi_routes_test.go` に、どちらも `{id}` に取られないことの検査がある
  （`/api/tags/rejected-names` と同じ）。
- 画面は、`reject` のあと却下した名前の先頭のページ（§6）を取り直す（1 件の却下と同じ）。
- 1 件の経路（`POST /api/tags/{id}/confirm`・`/reject`、`DELETE /api/tags/{id}`）は変えない。行の操作は
  今までどおりそれを使う。

## 2. `POST /api/tags/{id}/merge` の変更

| 本文 | 成功 | 誤り |
| --- | --- | --- |
| `{ sourceIds: int64[] }`（1 件以上 20,000 件以下） | 200 `TagMergeResponse` | 400 `invalid_request`（`sourceIds` が空・`too_many_tags`、`sourceIds` に `{id}` を含む: reason `merge_same_tag`）、404 `tag_not_found`（統合先が無い） |

- `MergeTagRequest` の `sourceId` は廃止し、`sourceIds` を `required` にする。1 件の統合（行の「別のタグへ
  統合…」）も `sourceIds: [sourceId]` で送る。
- 応答は `Tag` から `TagMergeResponse` に変わる。無い統合元は飛ばして `notFoundIds` に載せ、残りを 1 つの
  取引で統合する。統合元がすべて無かったときも 200 で、`tag` は変わらない統合先、`notFoundIds` は全部。
  1 件の統合でこれを受けた画面は、今の `tag_not_found` と同じ扱い（窓を閉じ、一覧を取り直す）にする。
- 統合先が選んだタグの中にあるときは画面が統合元から外して送る（Edge Case）。統合元が統合先だけに
  なったら送らない（実行できない）。

## 3. `POST /api/tags/impact`

| 本文 | 成功 | 誤り |
| --- | --- | --- |
| `TagImpactRequest` | 200 `TagImpactResponse` | 400 `invalid_request`（`action` が 3 値でない、`ids` が空、`too_many_tags`） |

- 読みの経路で何も変えない。本文で `ids` を受けるのは、数千個の id が URL の長さに収まらないためで、
  `POST /api/video-tags/summary` と同じ形である。
- 数えるのは、`ids` のうち今あり、`action` が働くタグだけである（[data-model.md §1](../data-model.md#1-domain-に足す値)
  の `TagImpactApplies`）: `reject` は仮のタグ、`delete` は確定したタグ、`merge` はどちらも。`POST /api/tags/batch`
  が飛ばす種類のタグとその動画は、確認の数に入らない。統合では、画面は統合先を外した統合元を `ids` に送る。
- `videoCount` の数え方は [data-model.md §2](../data-model.md#2-保存層の操作) の `TagImpact`。
- 画面はまとめての却下・削除・統合の確認を開くときに呼び、応答が届くまで確認の数は読み込み中にする。
  失敗したら確認の中に理由を出し、実行は押せない（数の無い確認で実行させない）。まとめての確定は
  確認をとらないので呼ばない（要件 11）。

## 4. `web/src/api` の関数

`tags.ts` に足す・変える。

| 関数 | 中身 |
| --- | --- |
| `listTagPage(query, signal)` | 改訂で足す。`GET /api/tags` に §5 のパラメータを付けて呼び、`TagList`（`items`・`total`・`totalAll`・`nextCursor`）を返す。共有の保持には触れない。呼び手ごとの `AbortSignal` を受ける（条件を変えたら打ち切るため） |
| `listRejectedTagNamePage(cursor, limit, signal)` | 改訂で足す。`GET /api/tags/rejected-names` に §6 のパラメータを付けて呼び、`RejectedTagNameList` を返す。今の `listRejectedTagNames` はこれに置き換える |
| `getTags`・`refreshTags`・`subscribeTags`・`currentTags` | 変えない（`limit` を省いた `GET /api/tags` の全件。候補と絞り込みの確かめが使う）。`listTags` は応答の `items` だけを使い続ける |
| `afterTagChanged` | 改訂で変える。購読者（`subscribeTags`）がいれば今までどおり取り直し、いなければ `held` を捨てて次の `getTags` に取らせる（[research.md R-12](../research.md#r-12-操作のあとの反映は読み込んだ行の中で行い並びの位置は-naturalsortkey-の移植で決める)）。`clearListSnapshot` は変えない |
| `batchTags(action, ids)` | `POST /api/tags/batch`。成功したら `afterTagChanged` を 1 回呼ぶ（merge 済み） |
| `mergeTag(id, sourceIds)` | `TagMergeResponse` を返す。`tag_not_found` は `refreshOnStaleTagError` を通す（merge 済み） |
| `tagImpact(action, ids)` | `POST /api/tags/impact`。共有の保持には触れない（merge 済み） |
| `Tag`・`TagSort` | 生成物から |

`errorText` に `too_many_tags` の文言がある（merge 済み）。`maxTagBatch = 20000` は `maxVideoTagsSelection` と
並んで置かれる。上限は送る id の数に掛かるので、画面は読み込んだ行がこれを超えるときは「読み込んだ
ものをすべて選ぶ」（先頭のチェック）だけを disabled にし、まとめての操作は選択の数がこれを超えるときだけ
disabled にする（[research.md R-4](../research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かない無いタグは数えて飛ばす)）。
`tagPageLimit = 100` を足し、画面の 1 ページの件数にする。

## 5. `GET /api/tags` のパラメータ

| パラメータ | 型 | 既定 | 意味 |
| --- | --- | --- | --- |
| `q` | string、`maxLength: 100` | 空 | 検索語。`FoldForMatch` を掛けて前後の空白を落とし、空でなければ元の名前かシノニムの照合形に部分一致するタグだけにする（[014 の data-model.md §7](../../014-video-tags/data-model.md#7-検索欄でのタグ名の照合) と同じ照合形。語の分解はしない） |
| `tentative` | boolean | false | true なら仮のタグだけ |
| `unused` | boolean | false | true なら本数 0 のタグだけ。`tentative`・`q` と AND |
| `sort` | `TagSort` | `name` | 並び順。値が同じタグは名前の自然順、それも同じなら `id` |
| `cursor` | string | — | 前回の応答の `nextCursor`。中身は不透明 |
| `limit` | integer、1〜200 | — | 1 ページの件数。**省くと全件**を返し、`nextCursor` は入らない（候補・絞り込みの確かめ・外部連携 API の経路が使う今までの形） |

| 成功 | 誤り |
| --- | --- |
| 200 `TagList`（`items`・`total`・`totalAll`、`limit` 付きで続きがあれば `nextCursor`） | 400 `invalid_request`（`sort` が 5 値でない、`limit` が範囲外、`cursor` が解釈できない・別の並び順のもの、`q` が 100 文字を超える） |

- `total` は `q`・`tentative`・`unused` をすべて掛けた数、`totalAll` は何も掛けない数。どちらもページングと
  独立に返る（`LibraryPage.total` と同じ）。
- `cursor` は同じ `q`・`tentative`・`unused`・`sort` で続けて使う。条件を変えたら画面はカーソルを捨てて
  先頭から読む。別の `sort` のカーソルは `400`、別の条件（`q` など）で同じカーソルを渡したときの結果は
  保証しない（ライブラリと同じ）。
- ページ送りの途中で別のタブがタグを増減・改名したときの保証は、ライブラリの keyset と同じ範囲
  （[013 の list-api.md §5](../../013-library-search/contracts/list-api.md#5-カーソルと誤り)）。画面は `id` の
  重複を捨て、`totalAll` の食い違いを知らせる（[data-model.md §4](../data-model.md#4-画面の側で持つ状態)）。
- 本数（`videoCount`）、シノニム、`tentative`、`createdAt` の載せ方は変えない。
- `GET /api/v1/tags`（外部連携 API）は変えない（`limit` 無しの全件と同じ結果を今の形で返す）。
- `Cache-Control: no-store` は今のまま。

## 6. `GET /api/tags/rejected-names` のパラメータ

| パラメータ | 型 | 既定 | 意味 |
| --- | --- | --- | --- |
| `cursor` | string | — | 前回の応答の `nextCursor` |
| `limit` | integer、1〜200 | 100 | 1 ページの件数 |

| 成功 | 誤り |
| --- | --- |
| 200 `RejectedTagNameList`（`items`・`total`、続きがあれば `nextCursor`） | 400 `invalid_request`（`limit` が範囲外、`cursor` が解釈できない） |

- 並びは名前の自然順（`sort_key`、同じなら `name` のバイト順。[data-model.md §0](../data-model.md#0-マイグレーション)）。
- `DELETE /api/tags/rejected-names?name=…` は変えない（[031 の contracts/screen-api.md §3](../../031-tentative-tags/contracts/screen-api.md#3-却下した名前)）。
- 画面は開いたときに先頭の 1 ページを受けて入口に `total` を出し、窓の中で末尾までスクロールしたら
  `nextCursor` で続きを足す（[research.md R-13](../research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)）。
