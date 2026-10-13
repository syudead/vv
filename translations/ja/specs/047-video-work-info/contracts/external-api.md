---
source: specs/047-video-work-info/contracts/external-api.md
sourceHash: 01c2207a1dae5e15bec20bb881e46a9b236e607353ea5baa919d3eb86b1db808
---

# 契約: 外部 API v1 と MCP の作品情報 {#contract-work-information-in-external-api-v1-and-mcp}

正本: `api/external-v1.yaml` の操作 `updateVideoWorkInfo` と、`ExternalVideo` の `workInfo` フィールド。この文書は、この機能が加えるものだけを扱う。共通のルール (Bearer、エラーの形、`VideoRef`、`index`) は [specs/026-external-api/contracts/external-api.md、共通のルール](../../026-external-api/contracts/external-api.md#common-rules) のままである。決定は [research.md R-6](../research.md#r-6-the-external-api-gets-one-bulk-operation-and-mcp-one-tool-workinfo-joins-externalvideo) にある。互換性の方針に従い、フィールド、1 つの操作、理由、1 つのエラーのプロパティだけを加える。

## `ExternalVideo` の変更 {#externalvideo-changes}

| フィールド | 型 | ルール |
| --- | --- | --- |
| `workInfo` | `WorkInfo` (`required`) | [screen-api.md、`WorkInfo` スキーマ](screen-api.md#workinfo-schema) の 6 つのフィールド。この API では未設定の単一値のフィールドはないのではなく `null` で、`performers` はいないときは `[]` なので、クライアントは有無を確かめずにすべてのプロパティを読める |
| `updatedAt` | date-time (既存) | その説明で、それを進める編集の一覧に作品情報が加わる |

`VideoTagsItem.video` は変わらない。

## `POST /api/v1/video-work-info` {#post-apiv1video-work-info}

`operationId: updateVideoWorkInfo`。

```json
{ "items": [ { "video": { "path": "/media/a.mp4" },
               "workInfo": { "code": "ABC-123", "releaseDate": "2024-06-01",
                             "maker": "Example Studio", "performers": ["山田 花子"],
                             "description": "…", "sourceUrl": "https://example.com/abc-123" } },
             { "video": { "id": 12 }, "workInfo": {} } ] }
```

各項目の `workInfo` は、画面の API の要求本文と共有する [`WorkInfoInput`](screen-api.md#workinfoinput-schema) であり、保存されているオブジェクトを置き換える。ない、`null`、またはトリム後に空のフィールドは消去され、`{}` はすべてを消去する。すべての項目は 1 つのトランザクションで適用する。どれかの項目にエラーがあれば、何も適用しない。同じ動画に対する後の項目が勝つ。

| 状況 | 応答 |
| --- | --- |
| 成功 | `200`: `items` の順の `{ items: [{ video: { id, contentKey }, workInfo }] }`。適用した値を `ExternalVideo` の形で持つ |
| `items` が 0 件、または 20000 件を超える | `400 invalid_request` / `too_many_videos`、`limit` |
| `video` が id、内容の鍵、パスのちょうど 1 つでない | `400 invalid_request` |
| フィールドがそのルールを破る ([screen-api.md、`PUT /api/videos/{id}/work-info`](screen-api.md#put-apivideosidwork-info)) | 同じ `reason`、`field`、`limit` を持つ `400 invalid_request`。`index` は `items` での位置 |
| 動画が見つからない | `404 not_found` / `video_not_found`、`index`。何も適用しない |

本文の上限は `POST /api/v1/video-tags` と同じ 32 MiB である。要求を繰り返しても状態は変わらず、`200` を返す。

`Error` は省略できる `field` を得て、`ErrorReason` は `work_info_control_characters`、`work_info_too_long`、`work_info_invalid_date`、`work_info_invalid_url`、`too_many_performers` を得る。

## MCP のツール {#mcp-tools}

[specs/026-external-api/contracts/mcp.md、ツール](../../026-external-api/contracts/mcp.md#tools) の表に加える。入力と出力は、上の操作の要求本文と応答と同じ形を持つ。

| ツール | 操作 | ヒント |
| --- | --- | --- |
| `update_video_work_info` | `POST /api/v1/video-work-info` | `destructiveHint: true` (消去したフィールドは失われる)、`idempotentHint: true` |

`list_videos` と `get_video` の出力は `workInfo` をそのまま含む。ツールは 15 個になる。

## クライアントでの使い方 {#client-use}

`docs/how-to/external-api.md` は「Set work information」の節を得る。そこには上の要求、表にしたフィールドのルール、スクレイパーは持っているオブジェクト全体を書くという注記がある。
