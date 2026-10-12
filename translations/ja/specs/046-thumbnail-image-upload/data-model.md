---
source: specs/046-thumbnail-image-upload/data-model.md
sourceHash: a02b1866061c6af8a038c5e2004388019e42d6fbeba89582a53e95c6a269a0c1
---

# データモデル: ローカルの画像をサムネイルにし、今のサムネイルを ComfyUI で編集する {#data-model-set-a-local-image-as-the-thumbnail-and-edit-the-current-thumbnail-with-comfyui}

親 Issue: #980。

モデルの残りは変わらない: テーブルの正本は [internal/store/migrations/](../../internal/store/migrations/) であり、`thumbnail_images` そのものは [045 データモデル](../045-comfyui-thumbnails/data-model.md#thumbnail_images) にある。このファイルは、この機能がそこに加える 2 つの列と、セッションと API が得る値を扱う。

## マイグレーション {#migration}

`00041_thumbnail_image_sources.sql` (`main` の最後のマイグレーションは `00040_thumbnail_images.sql` である。別の機能が先に入ったら、実装は次の空いている番号を取る)。up は下の 2 つの列を加え、down はそれらを削除する。

## `thumbnail_images` (変更) {#thumbnail_images-changed}

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `source` | blob | はい | 動画の縦横比に合わせ、長辺を最大 2048 px にした適用した画像の JPEG ([R-3](research.md#r-3-the-row-keeps-the-applied-images-source-at-up-to-2048-px-and-its-origin))。このマイグレーションより前に書いた行では null |
| `origin` | text | いいえ | `generated` または `uploaded`。マイグレーションは既存の行を `generated` で埋める |

**関係**: 変わらない。

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| すべての適用は、`thumbnail_state` を `done` にする 1 つのトランザクションで `image`、`source`、`origin` を書く | `app.Ingest.ApplyThumbnailImage`、`OverrideStore` |
| 位置の変更と元に戻す操作は、元画像も含めて行を削除する | 変わらない: `OverrideStore.SetThumbnailPosition`、`RevertThumbnailImage` |
| サムネイルのジョブは `image` を公開し直す。`source` は決して読まない | 変わらない: `app.Ingest.Thumbnail` |
| 今のサムネイルからの編集は `source` を読み、`source` が null のときは `image` を読む | `app.ThumbnailEdits.Start` ([R-4](research.md#r-4-an-edit-from-the-current-thumbnail-starts-from-the-stored-source-or-from-the-thumbnails-frame)) |
| 空の `source` (null は許す) や 2 つの値以外の `origin` を持つ行はない | 書き込む側。`internal/store/invariants_test.go` |

## `domain` に加える値 {#values-added-to-domain}

| 値 | 内容 |
| --- | --- |
| `ThumbnailImageOrigin` | `generated`、`uploaded` |
| `Video.ThumbnailImageOrigin` | 行の `origin`。画像を適用していないときは空。`thumbnailImage` として返す ([R-5](research.md#r-5-thumbnailimage-names-the-applied-images-origin-in-place-of-thumbnailgenerated)) |
| `ThumbnailEditSource` | `frame`、`thumbnail` ([R-4](research.md#r-4-an-edit-from-the-current-thumbnail-starts-from-the-stored-source-or-from-the-thumbnails-frame)) |
| `ThumbnailUploadMaxBytes`、`ThumbnailUploadMaxPixels` | 64 MiB、1 億 2000 万 ([R-2](research.md#r-2-an-upload-is-a-png-jpeg-or-still-webp-of-at-most-64-mib-and-120-megapixels-decoded-one-at-a-time)) |
| `ErrThumbnailUploadTooLarge`、`ErrThumbnailUploadInvalid` | アップロードのルートの 2 つの `400` の理由 |

## 編集のセッション (変更) {#edit-session-changed}

[045 データモデル](../045-comfyui-thumbnails/data-model.md#edit-session) のセッションは 2 つのフィールドを得る。その状態と遷移は変わらない。

| フィールド | 意味 |
| --- | --- |
| `Source` | `frame` または `thumbnail`: 所有者が選んだもの |
| `PositionMs` | 画像がフレームのときにフレームを取り出した位置 (選んだもの、記録したもの、自動のもの、または最初のフレームへの代替の後の `0`)。保存した元画像のときはない |

セッションの `Frame` は、元が何であれ選んだ画像を保持し、マスクは今までどおりその大きさに対して確認する。

## 変わらないもの {#what-does-not-change}

`video_overrides` は画像を適用している間も位置を保ち、`thumbnailUrl` のバージョンは `-g<revision>` の形を保つので、サムネイルを示すすべての画面は既存のフィールドを通じてアップロードを反映する。シーク用のスプライト、カードのスクラブのフレーム、ホバーのプレビューは今までどおり動画から作る (Issue の `対象外`)。
