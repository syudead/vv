# Contract: 画面の API のまとめての操作と `createdAt`

正本は [api/openapi.yaml](../../../api/openapi.yaml) で、この文書は足す項目・経路・誤りの差分だけを書く。
誤りの形、同一オリジンの検査、JSON 本文の読み方、`requiresJSONBody` と `openapi_routes_test.go` への追加は
[specs/014-video-tags/contracts/tags-api.md](../../014-video-tags/contracts/tags-api.md) のまま。決定は
[research.md R-4〜R-6・R-8](../research.md)。経路はすべて所有者だけ（`security` は既存のタグの操作と同じ）。
外部連携 API（`api/external-v1.yaml`）は変えない。

## 0. スキーマの差分

| スキーマ | 足す項目 | 規則 |
| --- | --- | --- |
| `Tag` | `createdAt: string, format: date-time`（`required`） | `tags.created_at`。`GET /api/tags` と、作成・改名・統合・シノニム・確定の応答に出る |

新しいスキーマ:

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
  required: [ids]
  additionalProperties: false
  properties:
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
    tagCount:   { type: integer }   # ids のうち今あるタグの数
    videoCount: { type: integer }   # そのどれかが付いた、いまライブラリにある動画の本数（重複なし）
```

`ErrorReason` に `too_many_tags` を足す（`limit` 付き、`too_many_videos` と同じ形）。

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
  区別される。`openapi_routes_test.go` に、どちらも `{id}` に取られないことの検査を足す
  （`/api/tags/rejected-names` と同じ）。
- 画面は、`reject` のあと却下した名前の一覧（031 の §3）を取り直す（1 件の却下と同じ）。
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
| `TagImpactRequest` | 200 `TagImpactResponse` | 400 `invalid_request`（`ids` が空、`too_many_tags`） |

- 読みの経路で何も変えない。本文で `ids` を受けるのは、数千個の id が URL の長さに収まらないためで、
  `POST /api/video-tags/summary` と同じ形である。
- `videoCount` の数え方は [data-model.md §2](../data-model.md#2-保存層の操作) の `TagImpact`。
- 画面はまとめての却下・削除・統合の確認を開くときに呼び、応答が届くまで確認の数は読み込み中にする。
  失敗したら確認の中に理由を出し、実行は押せない（数の無い確認で実行させない）。まとめての確定は
  確認をとらないので呼ばない（要件 10）。

## 4. `web/src/api` の関数

`tags.ts` に足す・変える。

| 関数 | 中身 |
| --- | --- |
| `batchTags(action, ids)` | `POST /api/tags/batch`。成功したら `afterTagChanged` を 1 回呼ぶ |
| `mergeTag(id, sourceIds)` | 署名を `sourceIds: readonly number[]` に変え、`TagMergeResponse` を返す。`tag_not_found` は今までどおり `refreshOnStaleTagError` を通す |
| `tagImpact(ids)` | `POST /api/tags/impact`。共有の保持には触れない |
| `Tag` | 生成物から `createdAt` が入る |

`errorText` に `too_many_tags` の文言を足す。`maxTagBatch = 20000` を `maxVideoTagsSelection` と並べて置き、
画面は見えている行がこれを超えるときまとめての操作を disabled にする。
