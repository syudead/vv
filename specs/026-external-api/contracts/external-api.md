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
      items: { $ref: ExternalVideoTag }

ExternalVideoTag:                                 # domain.VideoTag
  required: [id, name, manual, fromFolder]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }
    manual: { type: boolean }                     # 手で付けた（この API の付け外しの対象）
    fromFolder: { type: boolean }                 # 祖先のフォルダ名から付く（この API では外れない）
```

### `GET /api/v1/videos`

| 引数 | 既定 | 規則 |
| --- | --- | --- |
| `order` | `added` | `added`・`changed`（[R-6](../research.md#r-6-動画の一覧は単調に増える列でカーソルを作る)） |
| `cursor` | 先頭 | 前回の `nextCursor`。別の `order` のもの・解釈できないものは `400 invalid_request` / `invalid_cursor` |
| `limit` | 100 | 1〜`domain.MaxLimit`（200）。外れは `400 invalid_request` |

`200`: `{ items: ExternalVideo[], nextCursor: string, hasMore: boolean }`。カーソルは読み通しの
始まりで固定した上限を持ち、その読み通しの間に足された・変わった動画は返さない（同じ動画が一度の
読み通しで二度返らない）。`hasMore` が `false` になった後も `nextCursor` を返し、そこから呼ぶと、その後に
足された（`changed` なら変わった）動画だけが返る（受け入れ条件 3）。項目が無いときも同じ規則で
`nextCursor` を返す。

一覧は、`lookup` と同じく登録フォルダの下に所在を 1 つ以上持つ動画だけを返す。登録の下の所在をすべて失い
登録外の所在だけで残った動画は、行が消えた動画と同じく一覧に出ず、消えたことを知らせる項目（墓標）も返さない。
Issue は新着を読むことを求め、消えた動画の通知は求めていない（Webhook も対象外）。利用者は、持っている動画が
消えたことを `lookup` の `404` で知る（`docs/how-to/external-api.md` に書く）。その動画が後で登録の下に
戻れば、`changed` で返る。

`changed` で「変わった」と数える事実は [data-model.md §2](../data-model.md#2-videosadded_seqvideoschanged_seqr-6) の表のとおりで、
所在の付け替えで所在を失った動画と、メディアフォルダの追加・置き換えで所在が登録の下に入った動画も含む。
後者は `added` では元の位置のままなので、登録の変更で現れた動画を拾うには `changed` を読む
（`docs/how-to/external-api.md` に書く）。

### `GET /api/v1/videos/lookup`

`id`・`contentKey`・`path` のちょうど 1 つを取る。0 個・2 個以上は `400 invalid_request`。

- `200`: `ExternalVideo`。
- `404 not_found` / `video_not_found`: 無い、または登録フォルダの下に所在が無い。

`path` は絶対パスで、正規化せず今の所在の `path` とバイト列の完全一致で比べる。所在は
ファイルシステムの綴り（NFD を含む）のまま保存されている（`internal/scanner` の
`TestScanPreservesPathAndNormalizesTitleToNFC`）ので、一覧が返した `path` をそのまま渡せば引ける。

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

付け外しの対象は手で付けたタグだけで、フォルダ由来のタグは残る（`replace` の後も）。規則は [research.md R-7](../research.md#r-7-タグの操作は厳格な一括操作として-tagstore-に足す)。

| 状況 | 応答 |
| --- | --- |
| 成功 | `200`: `{ items: [{ video: { id, contentKey }, tags: ExternalVideoTag[] }] }`（`videos` の順、操作後のタグ） |
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
    videos: { type: [integer, "null"] }            # finding のあいだは null（本数を数え終えていない）
    settledVideos: { type: [integer, "null"] }     # 同上。数えたあとは 0 ≤ settledVideos ≤ videos
    errorCode: { type: [string, "null"] }
```

`videos`・`settledVideos` は `domain.ImportProgress.Counted` が真のときだけ数を入れ、偽なら両方 `null` にする
（画面の API が `videos` を省くのと同じ判定）。走査を始めた直後の `finding` と、対象が 0 本で `done` の
`{ videos: 0, settledVideos: 0 }` を区別できる。

- `POST /api/v1/scans`: 新しく始めたら `201`、実行中のものを返したら `200`。どちらかは、走査の行を作るか
  実行中の行を返すかを 1 つのトランザクションで決める `ScanStore.StartScan` の `started` で決め、
  `app.Scans.StartScan` は `(domain.Scan, started bool, error)` を返すように変えてそれを通す。画面の
  `POST /api/scans` は `started` を読まず、今どおり `202` を返す。応答の前に `GET /api/v1/scans/current` で
  状態を読んで決めることはしない（読む間に走査が始まる・終わる）。
  メディアフォルダが無ければ、画面の API と同じ `409 media_folders_not_configured`。
- `GET /api/v1/scans/current`: `200 ExternalScan`。一度も走査していなければ `404 not_found` / `no_scan`。
