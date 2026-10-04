# Contract: MCP server at `/mcp`

Parent Issue: #493 (requirement 9). Design:
[research.md R-8](../research.md#r-8-mcp-uses-the-official-go-sdk-stateless-inside-internalhttpapi).

## 1. Connection

- Transport is stateless Streamable HTTP. Only `POST /mcp` is accepted, and responses are
  `application/json`. `GET` and `DELETE` return `405`.
- Authentication is Bearer only, as for the external API
  ([external-api.md §1](external-api.md#1-common-rules)). With a missing or invalid token, the boundary
  returns `401` with `WWW-Authenticate: Bearer` before MCP processing starts (acceptance criterion 9).
- The server name is `vv`, and the version is the binary's version.

Connection example (documented in `docs/how-to/external-api.md`):

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

## 2. Tools

Input and output (structured content) have the same shape as the parameters and body, and the
response body, of the same operation in [external-api.md](external-api.md). Errors are returned as a
tool result with `isError: true` and, in the body, the same `{ code, message, reason?, limit?,
index?, tagId?, tagName? }` as external API errors.

| Tool | Operation |
| --- | --- |
| `list_videos` | `GET /api/v1/videos` |
| `get_video` | `GET /api/v1/videos/lookup` |
| `list_tags` | `GET /api/v1/tags`; `limit` defaults to 100 in the tool |
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

`update_video_display_names` and `update_video_thumbnails` were added in 029
([specs/029-video-overrides/contracts/external-api.md §3](../../029-video-overrides/contracts/external-api.md#3-mcp-tools)).
039 added the list parameters of `list_tags` and its default `limit`, and the
tools `merge_tags`, `rename_tag`, `update_tag_synonyms`, `batch_tags`,
`list_rejected_tag_names` and `forget_rejected_tag_name`
([specs/039-external-tag-admin/contracts/external-api.md §8](../../039-external-tag-admin/contracts/external-api.md#8-mcp-tools)).

| Tool | Hints | Reason |
| --- | --- | --- |
| Read tools | `readOnlyHint: true` | — |
| `update_video_tags` | `destructiveHint: true`, `idempotentHint: true` | `remove` and `replace` detach existing tags. |
| `start_scan` | `destructiveHint: false`, `idempotentHint: false` | — |
| `update_video_display_names`, `update_video_thumbnails` | `destructiveHint: true`, `idempotentHint: true` | `null` clears an override. |
| `merge_tags` | `destructiveHint: true`, `idempotentHint: true` | Merged sources are deleted. |
| `rename_tag` | `destructiveHint: false`, `idempotentHint: true` | — |
| `update_tag_synonyms` | `destructiveHint: true`, `idempotentHint: true` | `remove` drops a name; `add` with `mergeTagId` merges a tag. |
| `batch_tags` | `destructiveHint: true`, `idempotentHint: true` | `reject` and `delete` remove tags. |
| `forget_rejected_tag_name` | `destructiveHint: true`, `idempotentHint: true` | The name stops being skipped by tentative attaches. |
