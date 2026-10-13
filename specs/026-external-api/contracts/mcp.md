# Contract: MCP server at `/mcp`

Parent Issue: #493 (requirement 9). Design:
[research.md R-8](../research.md#r-8-mcp-uses-the-official-go-sdk-stateless-inside-internalhttpapi).

## Connection

- Transport is stateless Streamable HTTP. Only `POST /mcp` is accepted, and responses are
  `application/json`. `GET` and `DELETE` return `405`.
- Authentication is Bearer only, as for the external API
  ([external-api.md, Common rules](external-api.md#common-rules)). With a missing or invalid token, the boundary
  returns `401` with `WWW-Authenticate: Bearer` before MCP processing starts (acceptance criterion 9).
- The server name is `vv`, and the version is the binary's version.

Connection example (documented in `docs/how-to/external-api.md`):

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

## Tools

Input and output (structured content) have the same shape as the parameters and body, and the
response body, of the same operation in [external-api.md](external-api.md). Errors are returned as a
tool result with `isError: true` and, in the body, the same `{ code, message, reason?, limit?,
index?, field?, tagId?, tagName? }` as external API errors.

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
| `update_video_work_info` | `POST /api/v1/video-work-info` |
| `list_people`, `list_organizations` | `GET /api/v1/people`, `GET /api/v1/organizations`; `limit` defaults to 100 in the tool |
| `create_person`, `create_organization` | `POST /api/v1/people`, `POST /api/v1/organizations` |
| `update_person`, `update_organization` | `POST /api/v1/people/update`, `POST /api/v1/organizations/update` |
| `merge_people`, `merge_organizations` | `POST /api/v1/people/merge`, `POST /api/v1/organizations/merge` |
| `delete_people`, `delete_organizations` | `POST /api/v1/people/delete`, `POST /api/v1/organizations/delete` |

`update_video_display_names` and `update_video_thumbnails` were added in 029
([specs/029-video-overrides/contracts/external-api.md, MCP tools](../../029-video-overrides/contracts/external-api.md#mcp-tools)).
039 added the list parameters of `list_tags` and its default `limit`, and the
tools `merge_tags`, `rename_tag`, `update_tag_synonyms`, `batch_tags`,
`list_rejected_tag_names` and `forget_rejected_tag_name`
([specs/039-external-tag-admin/contracts/external-api.md, MCP tools](../../039-external-tag-admin/contracts/external-api.md#mcp-tools)).
047 added `update_video_work_info` and the five tools each for people and
organizations, and `workInfo` in the output of `list_videos` and `get_video`
([specs/047-video-work-info/contracts/external-api.md, MCP tools](../../047-video-work-info/contracts/external-api.md#mcp-tools)).

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
| `update_video_work_info` | `destructiveHint: true`, `idempotentHint: true` | A cleared item is lost. |
| `create_person`, `create_organization` | `destructiveHint: false`, `idempotentHint: false` | Each call creates one. |
| `update_person`, `update_organization` | `destructiveHint: true`, `idempotentHint: true` | A given list replaces the stored one. |
| `merge_people`, `merge_organizations`, `delete_people`, `delete_organizations` | `destructiveHint: true`, `idempotentHint: true` | Merged sources and deleted entities are gone. |
| `forget_rejected_tag_name` | `destructiveHint: true`, `idempotentHint: true` | The name stops being skipped by tentative attaches. |
