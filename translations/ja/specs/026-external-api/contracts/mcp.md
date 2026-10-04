---
source: specs/026-external-api/contracts/mcp.md
sourceHash: 5d519c69095169a5af470a986fe9024f6f3faa38cfd42444076422fc2b8b0d54
---

# 契約: `/mcp` の MCP サーバー {#contract-mcp-server-at-mcp}

親 Issue: #493（要件 9）。設計: [research.md R-8](../research.md#r-8-mcp-uses-the-official-go-sdk-stateless-inside-internalhttpapi)。

## 1. 接続 {#1-connection}

- トランスポートはステートレスな Streamable HTTP である。受け付けるのは `POST /mcp` だけで、応答は `application/json` である。`GET` と `DELETE` は `405` を返す。
- 認証は外部 API と同じく Bearer だけである（[external-api.md §1](external-api.md#1-common-rules)）。トークンがないか無効なとき、境界は MCP の処理が始まる前に `WWW-Authenticate: Bearer` 付きの `401` を返す（受け入れ条件 9）。
- サーバー名は `vv`、バージョンはバイナリのバージョンである。

接続の例（`docs/how-to/external-api.md` に記載）:

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

## 2. ツール {#2-tools}

入力と出力（構造化された内容）は、[external-api.md](external-api.md) の同じ操作のパラメータと本文、および応答本文と同じ形を持つ。エラーは `isError: true` を持つツール結果として返り、その本文は外部 API のエラーと同じ `{ code, message, reason?, limit?, index? }` である。

| ツール | 操作 |
| --- | --- |
| `list_videos` | `GET /api/v1/videos` |
| `get_video` | `GET /api/v1/videos/lookup` |
| `list_tags` | `GET /api/v1/tags`。ツールでは `limit` の既定は 100 |
| `update_video_tags` | `POST /api/v1/video-tags` |
| `start_scan` | `POST /api/v1/scans` |
| `get_current_scan` | `GET /api/v1/scans/current` |
| `update_video_display_names` | `POST /api/v1/video-display-names` |
| `update_video_thumbnails` | `POST /api/v1/video-thumbnails` |

最後の 2 つは 029 で追加された（[specs/029-video-overrides/contracts/external-api.md §3](../../029-video-overrides/contracts/external-api.md#3-mcp-tools)）。039 は `list_tags` の一覧パラメータとその既定の `limit` を追加した（[specs/039-external-tag-admin/contracts/external-api.md §8](../../039-external-tag-admin/contracts/external-api.md#8-mcp-tools)）。

| ツール | ヒント | 理由 |
| --- | --- | --- |
| 読み取りのツール | `readOnlyHint: true` | — |
| `update_video_tags` | `destructiveHint: true`、`idempotentHint: true` | `remove` と `replace` は既存のタグを外す。 |
| `start_scan` | `destructiveHint: false`、`idempotentHint: false` | — |
| `update_video_display_names`、`update_video_thumbnails` | `destructiveHint: true`、`idempotentHint: true` | `null` は上書きを消去する。 |
