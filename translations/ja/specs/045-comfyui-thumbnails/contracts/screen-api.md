---
source: specs/045-comfyui-thumbnails/contracts/screen-api.md
sourceHash: 2bc9e1fe44ecc05f5a029110291dc4ccc86d83664a224cb0a9cb059ecf17a4ad
---

# 契約: 画面の API の ComfyUI の設定、サムネイルの編集、適用した画像 {#contract-comfyui-settings-thumbnail-edits-and-the-applied-image-in-the-screen-api}

正本: `api/openapi.yaml`。この文書は、この機能が加えるルートと `Video` のフィールド、そして振る舞いが変わる既存のルート 1 つを扱う。すべてのルートは所有者専用である: どれも `accessRoutes` に挙がっていないので、境界はゲストに `401` を返し (要件 15)、`TestAccessClassificationMatchesOpenAPISecurity` が各ルートの `security` を確かめる。エラーの本文は [023 error-api](../../023-english-i18n/contracts/error-api.md) に従う。この機能はコード `provider_failed` と、下の表が挙げる理由を加える。

## `Video` の変更 {#video-change}

| フィールド | 型 | 規則 |
| --- | --- | --- |
| `thumbnailGenerated` | boolean | 適用した画像がサムネイルである間は `true`。所有者への応答にだけある |
| `thumbnailUrl` | string (既存) | 画像を適用している間、バージョンは `<content key prefix>-g<revision>` になる ([data-model.md、Values added to `domain`](../data-model.md#values-added-to-domain))。ゲストは同じ文字列を得る |

## `GET /api/settings/comfyui`、`PUT /api/settings/comfyui` {#get-apisettingscomfyui-put-apisettingscomfyui}

`getComfyUISettings`、`updateComfyUISettings`。接続 ([R-9](../research.md#r-9-the-api-key-is-write-only))。`GET` は `Cache-Control: no-store` を送る。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `provider` | 両方 | `self_hosted`、`cloud` | はい | どのプロバイダーか |
| `url` | 両方 | string | `self_hosted` のとき | `http` または `https` のベース URL |
| `apiKey` | `PUT` のみ | string | いいえ | ないときは保存したキーを保ち、`""` は消す |
| `apiKeySet` | `GET` のみ | boolean | はい | キーが保存されている |

保存の前の `GET` は、`provider` のない `200` を返す。

| ステータス | `code` / 理由 | いつ |
| --- | --- | --- |
| `400` | `invalid_request` / `comfyui_url_invalid` | 有効な URL のない `self_hosted` |
| `400` | `invalid_request` / `comfyui_api_key_missing` | 保存も送信もされたキーのない `cloud` |

## `POST /api/settings/comfyui/check` {#post-apisettingscomfyuicheck}

`checkComfyUI`。[R-5](../research.md#r-5-provider-differences-stay-inside-the-adapter) のリクエストで保存した接続をテストし、`missing` で拡張した `200` `CheckResult` (Ollama の確認が使うスキーマ) を返す。使用不可の結果の `message` は原因を挙げる: 届かない、ComfyUI ではない、キーが拒否された、何も保存されていない、またはファイルが欠けている (受け入れ条件 1 と 2)。プロバイダーが応答するが、同梱のワークフローが必要とするものを欠くとき ([research.md R-13](../research.md#r-13-the-connection-test-checks-the-workflows-nodes-and-model-files))、`missing` が各項目を挙げる。

| `missing` の項目のフィールド | 型 | 意味 |
| --- | --- | --- |
| `kind` | `node`、`model` | ノードの種類またはモデルのファイル |
| `name` | string | ノードの `class_type` またはファイル名 |
| `folder` | string | モデルのとき: ComfyUI の `models` の下のそのフォルダー |
| `source` | string | モデルのとき: そのダウンロードのページ ([workflow.md、Required files](workflow.md#required-files)) |

## `GET /api/thumbnail-edits/readiness` {#get-apithumbnail-editsreadiness}

`getThumbnailEditReadiness`。動画のページが入口を有効にするために必要なもの (要件 14): `{ connection: boolean }`。`connection` は設定が保存されていることを意味し、確認が通ったことではない。

## `POST /api/videos/{id}/thumbnail-edits` {#post-apivideosidthumbnail-edits}

`startThumbnailEdit`。本文は `{ positionMs }`。フレームを取り出し、状態 `ready` の `201` `ThumbnailEdit` を返す。動画の既存のセッションは先に終わる ([R-11](../research.md#r-11-a-new-session-replaces-the-previous-one-a-run-is-limited-to-10-minutes))。

| ステータス | `code` / 理由 | いつ |
| --- | --- | --- |
| `400` | `invalid_request` / `thumbnail_position_out_of_range` | 位置の上書きと同じ |
| `404` | `not_found` / `video_not_found`、`file_unavailable` | 位置の上書きと同じ |
| `409` | `conflict` / `thumbnail_edit_not_ready` | 接続が保存されていない |
| `409` | `conflict` / `thumbnail_frame_unavailable` | `ffmpeg` がフレームを出さなかった |

## `ThumbnailEdit` {#thumbnailedit}

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `id` | string | セッション |
| `videoId` | integer | 動画 |
| `state` | [`ThumbnailEditState`](../data-model.md#edit-session) | 状態 |
| `frameUrl` | string | `GET /api/thumbnail-edits/{editId}/frame`。PNG |
| `frameWidth`、`frameHeight` | integer | フレームのピクセルの大きさ。マスクはこれに一致しなければならない |
| `elapsedMs` | integer | 実行を送信してからの時間。`ready` ではない |
| `progress` | `{ value, max }` | ステップの進み具合。プロバイダーが何も出さなかったときはない |
| `error` | `{ reason, message }` | `failed` のとき: [workflow.md、Failure reasons](workflow.md#failure-reasons) の理由と、プロバイダーが出したときはその文言 |
| `resultUrl` | string | `succeeded` のとき、`GET /api/thumbnail-edits/{editId}/result` |

## `GET /api/thumbnail-edits/{editId}` {#get-apithumbnail-editseditid}

`getThumbnailEdit`。`200` `ThumbnailEdit` を返し、ページのハートビートとして数える ([R-4](../research.md#r-4-an-edit-is-an-in-memory-session-per-video-read-by-polling))。終わったセッションは `404 thumbnail_edit_not_found` を返し、フレームと結果のルートも同じである。

## `POST /api/thumbnail-edits/{editId}/runs` {#post-apithumbnail-editseditidruns}

`runThumbnailEdit`。`prompt` (テキスト、最大 4000 文字)、`mask` (PNG)、`reference` (PNG、JPEG または WebP、最大 20 MiB) を持つ `multipart/form-data` で、どれも省略できる。実行中の実行をキャンセルし、接続のスナップショットを取り、ワークフローの種類を選び ([workflow.md、Workflows](workflow.md#workflows))、プロンプトを送信したら `queued` の `202` `ThumbnailEdit` を返す。

| ステータス | `code` / 理由 | いつ |
| --- | --- | --- |
| `400` | `invalid_request` / `thumbnail_edit_mask_invalid` | マスクがフレームの大きさの PNG ではない |
| `400` | `invalid_request` / `thumbnail_edit_reference_invalid` | 参照がデコードできる画像ではない |
| `409` | `conflict` / `thumbnail_edit_not_ready` | セッションの開始の後に接続が削除された |
| `502` | `provider_failed` / 失敗の理由 | アップロードまたは送信が失敗した。セッションは同じ理由で `failed` になる |

## `POST /api/thumbnail-edits/{editId}/cancel` {#post-apithumbnail-editseditidcancel}

`cancelThumbnailEdit`。`cancelled` の `200` `ThumbnailEdit` を返し、進んでいる実行がないときは今の状態のものを返す。サムネイルは変わらない (受け入れ条件 7)。

## `POST /api/thumbnail-edits/{editId}/apply` {#post-apithumbnail-editseditidapply}

`applyThumbnailEdit`。結果を合わせ込んで適用し ([R-6](../research.md#r-6-the-applied-image-is-user-data-in-sqlite-the-thumbnail-file-is-rebuilt-from-it)、[R-7](../research.md#r-7-the-generated-image-is-fitted-to-the-videos-frame-before-it-is-stored))、セッションを終え、`200` `Video` を返す。

| ステータス | `code` / 理由 | いつ |
| --- | --- | --- |
| `409` | `conflict` / `thumbnail_edit_no_result` | 状態が `succeeded` ではない |
| `409` | `conflict` / `content_changed` | 動画がなくなった、または別の内容を持つ ([R-10](../research.md#r-10-apply-revert-and-the-position-override-are-ordered-by-the-generation-lock))。セッションは終わる |
| `409` | `conflict` / `thumbnail_image_invalid` | 結果をデコードできない |

## `DELETE /api/thumbnail-edits/{editId}` {#delete-apithumbnail-editseditid}

`discardThumbnailEdit`。進んでいる実行をキャンセルし、セッションを捨て、`204` を返す。終わったセッションも `204` を返す。ページは、破棄のときとアンマウントのときにこれを呼ぶ。

## `DELETE /api/videos/{id}/thumbnail-image` {#delete-apivideosidthumbnail-image}

`revertThumbnailImage`。適用した画像を除き、フレームからサムネイルを作り直す (要件 13)。画像を適用していなかったときも `200` `Video` を返す。エラーは位置の上書きのものである。

## `PUT /api/videos/{id}/thumbnail-position` (変更) {#put-apivideosidthumbnail-position-changed}

位置の設定または解除は、同じトランザクションの中で、適用した画像も除く。ほかは何も変わらない。

## クライアントでの使い方 {#client-use}

動画のページは、入口を有効にするために `readiness` を読み、プレーヤーの位置で編集を始め、実行が進んでいる間は毎秒セッションを読んで `state`、`elapsedMs`、`progress` を示す。結果があるときは、所有者が適用するか、再実行するか、破棄するまで 60 秒ごとに読む。
