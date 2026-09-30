# Contract: 外部連携 API v1 と MCP の表示名とサムネイルの位置

正本は `api/external-v1.yaml` で、この文書は足す 2 つの操作と `ExternalVideo` の差分、MCP の 2 つの
ツールだけを書く。共通の規則（Bearer、誤りの形、`VideoRef`、`index`）は
[specs/026-external-api/contracts/external-api.md §1](../../026-external-api/contracts/external-api.md)
のまま。決定は [research.md R-9](../research.md#r-9-外部連携-api-は一括の-2-操作を足しmcp-は同じ-2-ツールを足す)。
互換の方針どおり、項目と操作の追加だけを行う。

## 0. `ExternalVideo` の差分

| 項目 | 型 | 規則 |
| --- | --- | --- |
| `title` | string（既存） | 有効な題名（表示名があればそれ）。説明を書き換える |
| `fileTitle` | string（`required`） | 代表の所在の、拡張子を除いたファイル名 |
| `displayName` | string \| null（`required`） | 表示名。未設定は `null` |
| `thumbnailPositionMs` | integer \| null（`required`） | 代表サムネイルの位置。未設定は `null` |

`VideoTagsItem.video`（`VideoTagsVideo`）は変えない。

## 1. `POST /api/v1/video-display-names`

`operationId: updateVideoDisplayNames`。

```json
{ "items": [ { "video": { "path": "/media/a.mp4" }, "displayName": "旅行 2024 夏" },
             { "video": { "id": 12 }, "displayName": null } ] }
```

`displayName` は string か `null`（`required`）。`null` と、整えて空になる文字列は解除。全件を
1 つの取引で行い、1 件でも誤りがあれば何も反映しない。

| 状況 | 応答 |
| --- | --- |
| 成功 | `200`: `{ items: [{ video: { id, contentKey }, title, fileTitle, displayName }] }`（`items` の順、反映後の値） |
| `items` が 0 件か 20000 件超 | `400 invalid_request` / `too_many_videos`、`limit` |
| `video` が id・内容キー・パスのちょうど 1 つでない | `400 invalid_request` |
| 制御文字、または 200 符号位置超 | `400 invalid_request` / `display_name_control_characters`・`display_name_too_long`（`limit`）、`index` は `items` の位置 |
| 引けない動画がある | `404 not_found` / `video_not_found`、`index`。何も反映しない |

本文の上限は `POST /api/v1/video-tags` と同じ 32 MiB。

## 2. `POST /api/v1/video-thumbnails`

`operationId: updateVideoThumbnails`。

```json
{ "items": [ { "video": { "path": "/media/a.mp4" }, "positionMs": 12500 },
             { "video": { "contentKey": "…" }, "positionMs": null } ] }
```

`positionMs` は整数か `null`（`required`）。`null` は解除（自動の位置に戻す）。先に全件の引き当てと
位置の検証を行い、誤りがあれば何も反映しない。検証を通ったら `items` の順に 1 件ずつ画像を作って
記録する（画面の `PUT /api/videos/{id}/thumbnail-position` と同じ処理。
[screen-api.md §2](screen-api.md#2-put-apivideosidthumbnail-position)）。

| 状況 | 応答 |
| --- | --- |
| 全件を反映した | `200`: `{ items: [{ video: { id, contentKey }, thumbnailPositionMs }] }`（`items` の順） |
| `items` が 0 件か 20 件超 | `400 invalid_request` / `too_many_videos`、`limit: 20` |
| `video` の指定が不正 | `400 invalid_request` |
| 尺以上か負の位置がある | `400 invalid_request` / `thumbnail_position_out_of_range`、`index`、`limit` = その動画の尺 |
| 解析が終わっていない動画がある | `409 conflict` / `duration_unknown`、`index` |
| 引けない動画がある | `404 not_found` / `video_not_found`、`index` |
| どの所在も開けない動画がある | `404 not_found` / `file_unavailable`、`index` |
| 途中の 1 件で画像を作れなかった | `409 conflict` / `thumbnail_frame_unavailable`、`index`。`index` より前の項目は反映済みで、その項目と後は未反映 |

`Error.code` に `conflict` を、`ErrorReason` に `display_name_control_characters`・`display_name_too_long`・
`duration_unknown`・`thumbnail_position_out_of_range`・`thumbnail_frame_unavailable`・`file_unavailable` を
足す。

## 3. MCP のツール

[specs/026-external-api/contracts/mcp.md §2](../../026-external-api/contracts/mcp.md) の表に足す。入力と
出力は上の操作の本文と応答と同じ形。

| ツール | 対応する操作 | ヒント |
| --- | --- | --- |
| `update_video_display_names` | `POST /api/v1/video-display-names` | `destructiveHint: true`（`null` で解除する）、`idempotentHint: true` |
| `update_video_thumbnails` | `POST /api/v1/video-thumbnails` | `destructiveHint: true`、`idempotentHint: true` |

`list_videos`・`get_video` の出力は `ExternalVideo` の差分（§0）をそのまま含む。ツールは 8 つになる。
