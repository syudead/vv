# シーク用スプライトの HTTP 契約

**Feature**: 親 Issue #389 | **Plan**: [plan.md](../plan.md)

machine-readable schema の正本は [api/openapi.yaml](../../../api/openapi.yaml) で、ここには
この feature が変える差分だけを書く。旧契約
[specs/009-seek-thumbnail-preview/contracts/seek-thumbnail.md](../../009-seek-thumbnail-preview/contracts/seek-thumbnail.md)
の `positionMs` の JPEG 応答は、この契約が置き換える。

## 1. `Video.seekThumbnailUrl` の意味

`Video.seekThumbnailUrl`（`/api/videos/{id}/seek-thumbnail?v=<内容由来の版>`）は、配置情報を返す
経路の版付き URL になる。載る条件（`probeState = done` かつ正の `durationMs`）と
`seekThumbnailState` の意味（[specs/020 contracts/processing-api.md §2](../../020-seek-thumbnail-stage/contracts/processing-api.md#2-seekthumbnailstate-の意味)）
は変えない。

## 2. `GET /api/videos/{id}/seek-thumbnail`（`getVideoSeekThumbnail`）

完成したスプライトの配置情報を返す。`positionMs` は無くなる。

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `v` | string | no | `Video.seekThumbnailUrl` が返した内容由来の版 |

### Success

- Status: `200 OK`、`Content-Type: application/json`
- `Cache-Control: private, no-cache` と `ETag`（本文の内容から作る）。`If-None-Match` が一致すれば
  `304`（guest-api.md §5）
- Body: `SeekThumbnailSprite`

```yaml
SeekThumbnailSprite:
  type: object
  required: [intervalMs, frameCount, columns, rows, frameWidth, frameHeight, sheets]
  additionalProperties: false
  properties:
    intervalMs:  { type: integer, format: int64, minimum: 5000 }  # コマ k が受け持つ位置は [k*intervalMs, (k+1)*intervalMs)
    frameCount:  { type: integer, minimum: 1, maximum: 600 }      # 全シートに載るコマの数
    columns:     { type: integer, minimum: 1 }                    # 1 シートの列数（10）
    rows:        { type: integer, minimum: 1 }                    # 1 シートの行数（10）
    frameWidth:  { type: integer, minimum: 2 }                    # 1 コマの幅（px）。1 本の中で全コマ同じ
    frameHeight: { type: integer, minimum: 2 }                    # 1 コマの高さ（px）
    sheets:                                                       # シートの版付き URL。番号順、最大 6
      type: array
      minItems: 1
      maxItems: 6
      items: { type: string }
```

コマ `k`（`0 <= k < frameCount`）はシート `floor(k / (columns * rows))` の、左上から数えて
`k mod (columns * rows)` 番目（列 `k mod columns`、行 `floor((k mod (columns * rows)) / columns)`）
に載る。位置 `p` ms のコマは `min(floor(p / intervalMs), frameCount - 1)` で、クライアントは
間隔を固定値としては持たない（要件 3）。最後のシートの `frameCount` を超える部分は黒く、
クライアントはそこを指さない。値の決め方は [research.md R-1](../research.md#r-1-上限と間隔の規則)。

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id` が不正 | `400` | `invalid_request` |
| 動画が無い、ゲストに見せない動画 | `404` | `not_found` |
| 解析未完了・尺なし、または `seekThumbnailState` が `done` でない（生成待ち・生成中・失敗） | `409` | `conflict` |
| `sprite.json` を読めない・形が違う | `500` | `internal` |

## 3. `GET /api/videos/{id}/seek-thumbnail/{sheet}`（`getVideoSeekThumbnailSheet`）

配置情報の `sheets[sheet]` が指す JPEG を返す。`sheet` は 0 から始まる整数。

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `v` | string | no | 配置情報の `sheets` に含まれていた内容由来の版 |

### Success

- Status: `200 OK`、`Content-Type: image/jpeg`
- `Cache-Control: private, no-cache` と `ETag`（画像の内容から作る）。`If-None-Match` が一致すれば
  `304`
- Body: `columns × frameWidth` 幅、`rows × frameHeight` 高さの JPEG

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id` または `sheet` が整数でない、負 | `400` | `invalid_request` |
| 動画が無い、ゲストに見せない動画、`sheet` が枚数以上 | `404` | `not_found` |
| 解析未完了・尺なし、または `seekThumbnailState` が `done` でない | `409` | `conflict` |
| 画像を読めない | `500` | `internal` |

## 4. 認証の分類

どちらの経路もゲストにも返す（[internal/httpapi/auth.go](../../../internal/httpapi/auth.go) の表に
`GET /api/videos/{id}/seek-thumbnail/{sheet}` を足す）。ゲストに見せない動画は、他の生成物の経路と
同じく `404` である。

## 5. 直接配信とライブ変換

コマを決める位置は、どちらの再生経路でも元動画の論理時刻（シークバーの位置 × `durationMs`）で、
ライブ変換の実際の開始位置（[docs/design-docs/live-transcode-seek.md](../../../docs/design-docs/live-transcode-seek.md)）
は関わらない。009 の判断のままである（要件 7）。
