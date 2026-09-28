# Contract: `/mcp` の MCP サーバー

親 Issue: #493（要件 9）。作りは [research.md R-8](../research.md#r-8-mcp-は公式の-go-sdk-を-stateless-で-internalhttpapi-の中に置く)。

## 1. 接続

- 転送は Streamable HTTP の stateless。`POST /mcp` だけを受け、応答は `application/json`。
  `GET`・`DELETE` は `405`。
- 認証は外部連携 API と同じ Bearer だけ（[external-api.md §1](external-api.md#1-共通)）。無い・無効なら、
  MCP の処理に入る前に境界が `401` と `WWW-Authenticate: Bearer` を返す（受け入れ条件 9）。
- サーバー名は `vv`、版はバイナリの版。

接続例（`docs/how-to/external-api.md` に書く）:

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

## 2. ツール

入力と出力（structured content）は [external-api.md](external-api.md) の同じ操作の引数・本文と応答の本文と
同じ形にする。誤りはツールの結果の `isError: true` と、外部連携 API の誤りと同じ `{ code, message,
reason?, limit?, index? }` を本文に入れて返す。

| ツール | 対応する操作 |
| --- | --- |
| `list_videos` | `GET /api/v1/videos` |
| `get_video` | `GET /api/v1/videos/lookup` |
| `list_tags` | `GET /api/v1/tags` |
| `update_video_tags` | `POST /api/v1/video-tags` |
| `start_scan` | `POST /api/v1/scans` |
| `get_current_scan` | `GET /api/v1/scans/current` |

`update_video_tags` と `start_scan` には `destructiveHint: false`・`idempotentHint` を
（`update_video_tags` は `true`、`start_scan` は `false`）、読み出しのツールには `readOnlyHint: true` を付ける。
