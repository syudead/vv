---
source: specs/026-external-api/contracts/mcp.md
sourceHash: 15ac18db2116c6995bc97d52f4a058aa05c97b3b7306854d9e596797fce52dc3
---

# 契約: `/mcp` の MCP サーバー {#contract-mcp-server-at-mcp}

親 Issue: #493（要件 9）。設計: [research.md R-8](../research.md#r-8-mcp-uses-the-official-go-sdk-stateless-inside-internalhttpapi)。

## 接続 {#connection}

- トランスポートはステートレスな Streamable HTTP である。受け付けるのは `POST /mcp` だけで、応答は `application/json` である。`GET` と `DELETE` は `405` を返す。
- 認証は外部 API と同じく Bearer だけである（[external-api.md、共通の規則](external-api.md#common-rules)）。トークンがないか無効なとき、境界は MCP の処理が始まる前に `WWW-Authenticate: Bearer` 付きの `401` を返す（受け入れ条件 9）。
- サーバー名は `vv`、バージョンはバイナリのバージョンである。

接続の例（`docs/how-to/external-api.md` に記載）:

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

## ツール {#tools}

入力と出力（構造化された内容）は、[external-api.md](external-api.md) の同じ操作のパラメータと本文、および応答本文と同じ形を持つ。エラーは `isError: true` を持つツール結果として返り、その本文は外部 API のエラーと同じ `{ code, message, reason?, limit?, index?, field?, tagId?, tagName? }` である。

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
| `merge_tags` | `POST /api/v1/tags/merge` |
| `rename_tag` | `POST /api/v1/tags/rename` |
| `update_tag_synonyms` | `POST /api/v1/tags/synonyms` |
| `batch_tags` | `POST /api/v1/tags/batch` |
| `list_rejected_tag_names` | `GET /api/v1/tags/rejected-names` |
| `forget_rejected_tag_name` | `DELETE /api/v1/tags/rejected-names` |
| `update_video_work_info` | `POST /api/v1/video-work-info` |
| `list_people`、`list_organizations` | `GET /api/v1/people`、`GET /api/v1/organizations`。ツールでは `limit` の既定は 100 |
| `create_person`、`create_organization` | `POST /api/v1/people`、`POST /api/v1/organizations` |
| `update_person`、`update_organization` | `POST /api/v1/people/update`、`POST /api/v1/organizations/update` |
| `merge_people`、`merge_organizations` | `POST /api/v1/people/merge`、`POST /api/v1/organizations/merge` |
| `delete_people`、`delete_organizations` | `POST /api/v1/people/delete`、`POST /api/v1/organizations/delete` |
| `list_tag_kinds` | `GET /api/v1/tag-kinds` |
| `create_tag_kind` | `POST /api/v1/tag-kinds` |
| `rename_tag_kind` | `POST /api/v1/tag-kinds/update` |
| `delete_tag_kinds` | `POST /api/v1/tag-kinds/delete` |

`update_video_display_names` と `update_video_thumbnails` は 029 で追加された（[specs/029-video-overrides/contracts/external-api.md、MCP ツール](../../029-video-overrides/contracts/external-api.md#mcp-tools)）。039 は `list_tags` の一覧パラメータとその既定の `limit`、およびツール `merge_tags`、`rename_tag`、`update_tag_synonyms`、`batch_tags`、`list_rejected_tag_names`、`forget_rejected_tag_name` を追加した（[specs/039-external-tag-admin/contracts/external-api.md、MCP ツール](../../039-external-tag-admin/contracts/external-api.md#mcp-tools)）。047 は `update_video_work_info`、人物と組織それぞれの 5 つのツール、および `list_videos` と `get_video` の出力の `workInfo` を追加した（[specs/047-video-work-info/contracts/external-api.md、MCP ツール](../../047-video-work-info/contracts/external-api.md#mcp-tools)）。051 は 4 つのタグの種類のツール、`list_tags` の入力の `kind`、および `batch_tags` の入力の `setKind`、`clearKind`、`kindId` を追加した（[specs/051-tag-kinds/contracts/external-api.md、MCP ツール](../../051-tag-kinds/contracts/external-api.md#mcp-tools)）。

| ツール | ヒント | 理由 |
| --- | --- | --- |
| 読み取りのツール | `readOnlyHint: true` | — |
| `update_video_tags` | `destructiveHint: true`、`idempotentHint: true` | `remove` と `replace` は既存のタグを外す。 |
| `start_scan` | `destructiveHint: false`、`idempotentHint: false` | — |
| `update_video_display_names`、`update_video_thumbnails` | `destructiveHint: true`、`idempotentHint: true` | `null` は上書きを消去する。 |
| `merge_tags` | `destructiveHint: true`、`idempotentHint: true` | 統合元は削除される。 |
| `rename_tag` | `destructiveHint: false`、`idempotentHint: true` | — |
| `update_tag_synonyms` | `destructiveHint: true`、`idempotentHint: true` | `remove` は名前を外す。`mergeTagId` 付きの `add` はタグを統合する。 |
| `batch_tags` | `destructiveHint: true`、`idempotentHint: true` | `reject` と `delete` はタグを削除する。 |
| `create_tag_kind` | `destructiveHint: false`、`idempotentHint: false` | 呼び出しごとに 1 つ作成する。 |
| `rename_tag_kind` | `destructiveHint: false`、`idempotentHint: true` | — |
| `delete_tag_kinds` | `destructiveHint: true`、`idempotentHint: true` | 削除した種類はなくなる。その種類のタグは種類なしで残る。 |
| `update_video_work_info` | `destructiveHint: true`、`idempotentHint: true` | 消した項目は失われる。 |
| `create_person`、`create_organization` | `destructiveHint: false`、`idempotentHint: false` | 呼び出しごとに 1 つ作成する。 |
| `update_person`、`update_organization` | `destructiveHint: true`、`idempotentHint: true` | 与えた一覧は保存した一覧を置き換える。 |
| `merge_people`、`merge_organizations`、`delete_people`、`delete_organizations` | `destructiveHint: true`、`idempotentHint: true` | 統合した統合元と削除したエンティティはなくなる。 |
| `forget_rejected_tag_name` | `destructiveHint: true`、`idempotentHint: true` | その名前は、仮のタグとしての付与で飛ばされなくなる。 |
