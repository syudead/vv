# Contract: 外部連携 API v1

親 Issue: #493（要件 5〜7・10・11）。正本は実装の時点で `api/external-v1.yaml` に作る
（[research.md R-5](../research.md#r-5-外部連携-api-の契約は別の-openapi-の文書にしgo-だけを生成する)）。
ここは、その文書が満たすべき操作と規則を決める。

## 1. 共通

- 基底は `/api/v1`。すべての操作が `security: bearerAuth`（`type: http`、`scheme: bearer`）。
- 認証（[research.md R-3](../research.md#r-3-外部連携-api-は-apiv1-の下に置き境界にbearerの分類を足す)）:
  `Authorization: Bearer <token>` が無い・形式が違う・無効・失効済みは
  `401 unauthenticated` と `WWW-Authenticate: Bearer`。Cookie は読まない。
- 同一オリジンの検査はかけない。本文を取る操作は `Content-Type: application/json` を要する
  （[R-4](../research.md#r-4-bearer-の要求には同一オリジンの検査をかけない)）。
- 見る人は常に所有者で、非公開の動画も返す（受け入れ条件 2）。
- 誤りは画面の API と同じ形 `{ code, message, reason?, limit?, index? }`。`index` はこの API だけの項目で、
  §4 の `videos` の何番目（0 から）が原因かを示す。`code` と `reason` の値は、ここに挙げたものだけを
  `external-v1.yaml` の enum に置く。

## 2. 動画

```yaml
ExternalVideo:
  required: [id, contentKey, title, durationMs, addedAt, locations, tags]
  properties:
    id: { type: integer, format: int64 }
    contentKey: { type: string }
    title: { type: string }
    durationMs: { type: [integer, "null"] }        # 解析前は null
    addedAt: { type: string, format: date-time }
    locations:                                    # 登録フォルダの下の今の所在。代表が先頭
      type: array
      items:
        required: [path, fileName]
        properties:
          path: { type: string }                  # 絶対パス
          fileName: { type: string }
    tags:
      type: array
      items: { required: [id, name], properties: { id: {type: integer, format: int64}, name: {type: string} } }
```

### `GET /api/v1/videos`

| 引数 | 既定 | 規則 |
| --- | --- | --- |
| `order` | `added` | `added`・`changed`（[R-6](../research.md#r-6-動画の一覧は単調に増える列でカーソルを作る)） |
| `cursor` | 先頭 | 前回の `nextCursor`。別の `order` のもの・解釈できないものは `400 invalid_request` / `invalid_cursor` |
| `limit` | 100 | 1〜`domain.MaxLimit`（200）。外れは `400 invalid_request` |

`200`: `{ items: ExternalVideo[], nextCursor: string, hasMore: boolean }`。`nextCursor` は最後まで読んだ
後も返し、そこから呼ぶと、その後に足された（`changed` なら変わった）動画だけが返る（受け入れ条件 3）。
項目が無いときは受け取ったカーソルと同じ位置を返す。

### `GET /api/v1/videos/lookup`

`id`・`contentKey`・`path` のちょうど 1 つを取る。0 個・2 個以上は `400 invalid_request`。

- `200`: `ExternalVideo`。
- `404 not_found` / `video_not_found`: 無い、または登録フォルダの下に所在が無い。

`path` は絶対パスで、NFC に正規化してから今の所在と完全一致で比べる。

## 3. タグ

### `GET /api/v1/tags`

`200`: `{ items: [{ id, name, synonyms: string[], videoCount }] }`。画面の `ListTags` と同じ並び。

## 4. 動画のタグ

### `POST /api/v1/video-tags`

```json
{ "videos": [{ "id": 1 }, { "contentKey": "…" }, { "path": "/media/a.mp4" }],
  "action": "add",
  "tags": ["名前", "シノニム"] }
```

規則は [research.md R-7](../research.md#r-7-タグの操作は厳格な一括操作として-tagstore-に足す)。

| 状況 | 応答 |
| --- | --- |
| 成功 | `200`: `{ items: [{ video: { id, contentKey }, tags: [{id, name}] }] }`（`videos` の順、操作後のタグ） |
| 各 `videos` の要素が id・内容キー・パスのちょうど 1 つでない、`action` が不正 | `400 invalid_request` |
| `videos` が 0 件か 20000 件超 | `400 invalid_request` / `too_many_videos`、`limit` |
| `tags` が `add`・`remove` で 0 件、または 100 件超 | `400 invalid_request` / `too_many_tags`、`limit` |
| 名前が空・制御文字・長すぎる | `400 invalid_request` / `tag_name_empty`・`tag_name_control_characters`・`tag_name_too_long`、`index` は `tags` の位置 |
| 引けない動画がある | `404 not_found` / `video_not_found`、`index`。何も反映しない |

名前の誤りの `index` は `tags` の位置を指す。それ以外の `index` は `videos` の位置を指す。

## 5. スキャン

```yaml
ExternalScan:
  required: [id, status, startedAt, finishedAt, videos, settledVideos, errorCode]
  properties:
    id: { type: integer, format: int64 }
    status: { enum: [finding, running, done, partial, failed] }   # domain の Scan.status と同じ
    startedAt: { type: string, format: date-time }
    finishedAt: { type: [string, "null"], format: date-time }
    videos: { type: integer }
    settledVideos: { type: integer }
    errorCode: { type: [string, "null"] }
```

- `POST /api/v1/scans`: `app.Scans.StartScan`。新しく始めたら `201`、実行中のものを返したら `200`。
  メディアフォルダが無ければ、画面の API と同じ `409 media_folders_not_configured`。
- `GET /api/v1/scans/current`: `200 ExternalScan`。一度も走査していなければ `404 not_found` / `no_scan`。
