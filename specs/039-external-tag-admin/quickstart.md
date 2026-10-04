# Quickstart: an agent tidies tags through MCP

These steps prove, with a real MCP client, that one `list_tags` response fits
an agent's reading and that a merge made from a tool is what the tag admin
screen shows. `task check` runs the same operations in Go tests against the
handlers; it does not run an MCP client.

## Prerequisites

- A running server with an API token
  ([Create a token](../../docs/how-to/external-api.md#create-a-token)) and a
  library with at least 1,000 tentative tags. `scripts/tagsbench` seeds such a
  library ([docs/how-to/tags-admin-benchmark.md](../../docs/how-to/tags-admin-benchmark.md)).
- Claude Code with the server registered:
  `claude mcp add --transport http vv http://localhost:8080/mcp --header "Authorization: Bearer vvt_…"`.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Ask the agent to list the tentative tags whose name contains a chosen term, and to keep reading until there are none left | Each `list_tags` call it makes carries `tentative: true`, `q` and `cursor`; no response exceeds 100 items; the last response has no `nextCursor`; the agent lists every matching tag | 1 |
| Ask the agent to merge one of those tags (the source) into another (the target), by name | It calls `merge_tags` with the two ids; the result's `tag` carries the source's name among `synonyms` and `tentative: false`, and `notFoundIds` is `[]` | 2 |
| Open the tag admin screen and search for the source's name | The source is gone from the list; the target shows the source's name as a synonym; a video that carried the source now carries the target | 2, 5 |
| Ask the agent to reject one tentative tag and then to list the rejected names | `batch_tags` returns the id in `appliedIds`; `list_rejected_tag_names` returns the tag's name | 3 |
| Attach the rejected name with `tentative: true` through `update_video_tags` | The name is returned in `skippedTags` and no tag is created | 3 |
