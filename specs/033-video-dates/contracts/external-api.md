# Contract: 外部連携 API の差分

親 Issue: #630。正本は [api/external-v1.yaml](../../../api/external-v1.yaml) で、ここには足す項目だけを
書く。`task generate` で `internal/httpapi/extgen/` を作り直す。外部連携 API の互換の方針（項目の追加だけ）は
[specs/026-external-api/contracts/external-api.md](../../026-external-api/contracts/external-api.md)。

## 0. `ExternalVideo` に足す項目

```yaml
ExternalVideo:
  required: [..., addedAt, updatedAt, fileCreatedAt, locations, tags]
  properties:
    updatedAt:
      type: string
      format: date-time
      description: |
        vv 上で動画の情報（表示名・タグ・公開設定・代表サムネイル）を最後に編集した日時。
        一度も編集していなければ addedAt と同じ。この API の video-tags・display-names も進める
    fileCreatedAt:
      type: string
      format: date-time
      description: |
        代表の所在（locations の先頭）のファイルの作成日時。ファイルシステムから取れないときは
        そのファイルの更新日時（mtime）
```

`GET /api/v1/videos`、`GET /api/v1/videos/lookup` と、`ExternalVideo` を返すほかの応答に入る。
`internal/httpapi/external_videos.go` の変換は `item.Video.EditedAt.UTC()`・`item.Video.FileCreatedAt.UTC()`
（`addedAt` と同じ扱い）。

## 1. 変わらないもの

- 一覧の並び（`addedAt`, `id` の昇順）とカーソル、`limit`、`lookup` の引き方。
- `POST /api/v1/video-tags`・`display-names` の要求と応答の形。更新日時が進むのは
  [data-model.md §3](../data-model.md#3-更新日時を進める規則) の規則による。
- MCP のツール（`list_videos`・`lookup_video` など）は同じハンドラの応答をそのまま返すので、
  構造化された出力に 2 項目が増えるだけで、ツールの定義は変えない。
- `docs/how-to/external-api.md`「動画の一覧を読む」の項目の説明に 2 項目を足す。
