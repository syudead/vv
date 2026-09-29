# Using the external API

This procedure operates VVMDM from an external tool, such as a scraper, with an API token. The
reasons for the decisions are in
[specs/026-external-api/research.md](../../specs/026-external-api/research.md).

## Contract

- The source of truth for the contract is [api/external-v1.yaml](../../api/external-v1.yaml)
  (OpenAPI 3.1). Read that file for the operations, fields, and error `code` and `reason` values.
  The server does not serve it.
- The base path is `/api/v1`. It is a separate contract from the `/api/*` the UI uses
  ([api/openapi.yaml](../../api/openapi.yaml)); the UI API changes without notice. External tools
  use only `/api/v1`.

## Compatibility policy

- Within `v1`, only fields and operations are added. Write clients to skip unknown fields and
  unknown `code` and `reason` values.
- Changing the meaning, type or requiredness of an existing field, or removing an operation, adds
  `v2` and leaves `v1` unchanged.

## Issue a token

1. Log in as the owner. In the "API tokens" section of the Settings page, enter a name for the
   purpose and issue the token.
2. Copy the displayed plaintext (47 characters starting with `vvt_`) right away. The plaintext is
   shown only this once. If you lose it, revoke the token and issue a new one.
3. Revoke tokens you no longer use in the same section. Changing the username or password
   (`mdm account`) revokes every token.

A token has the same permissions as the owner (it can read private videos too). Treat it like a
password when you keep it in a configuration file or a log.

## Call

Send `Authorization: Bearer <token>` on every request. Cookies are not read.

```sh
BASE=http://localhost:8080
TOKEN=vvt_...

curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/tags"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=100"
curl -X POST -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans/current"
```

- A missing, malformed, invalid or revoked token returns `401` (`code: unauthenticated`) with
  `WWW-Authenticate: Bearer`. The causes are not distinguished.
- No same-origin check applies, so clients that send `Origin` work too. Operations that take a
  body require `Content-Type: application/json`.
- The error body is `{ code, message, reason?, limit?, index? }`. `message` is an English
  description; branch on `code` and `reason`.
- The last-used time appears in the list on the Settings page. It is not updated more often than
  once per minute.
- Revoking a token also aborts its in-flight requests.

## Scans

- `POST /api/v1/scans` takes no body. It returns `201` when it starts a new scan. When a scan is
  running, it returns that scan with `200` instead of starting one. Without media folders, it
  returns `409` (`media_folders_not_configured`).
- `GET /api/v1/scans/current` reads the latest state. The scan has finished when `status` is
  `done`, `partial` or `failed`. While `finding`, the count is not final, so `videos` and
  `settledVideos` are `null`. If no scan has ever run, it returns `404` (`no_scan`).

## List videos

- `GET /api/v1/videos` returns the videos under the registered folders, oldest added first
  (`addedAt`, then `id`). Private videos are included. The page size is `limit` (default 100,
  range 1–200; out of range returns `400`).
- The response is `{ items, nextCursor }`. When `nextCursor` is not an empty string, pass it as
  is as `cursor` to read the next page. An empty string means the end. Do not interpret the cursor
  contents. An uninterpretable cursor returns `400` (`invalid_cursor`); read again from the start.

  ```sh
  cursor=""
  while :; do
    page=$(curl -s -G -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos" \
      --data-urlencode "cursor=$cursor")
    echo "$page" | jq -c '.items[]'
    cursor=$(echo "$page" | jq -r '.nextCursor')
    [ -z "$cursor" ] && break
  done
  ```

- The list does not track changes. To find new videos, read the list again from the start after a
  scan. Videos added, removed or moved during a full read do not fail the following requests, but
  videos can be missed or duplicated in that window.
- A removed video just disappears from the list; no field reports the removal. Check whether a
  video you hold still exists with `lookup`: `404` (`video_not_found`) means it is gone. Treat a
  video whose only remaining locations are outside the registered folders the same as a removed
  one.
- `locations` are the current locations under the registered folders, sorted by path; the first
  is the representative (`title` is the representative's title). `durationMs` is `null` before
  analysis. In `tags`, `manual` holds manually added tags and `fromFolder` holds tags from
  ancestor folder names.

## Look up one video

`GET /api/v1/videos/lookup` takes exactly one of `id`, `contentKey` and `path` (zero, or two or
more, returns `400`). The response has the same shape as a list item.

```sh
curl -G -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos/lookup" \
  --data-urlencode "path=/media/videos/clip.mp4"
```

- `path` is not normalized; it is compared byte for byte with the location path. For a file name
  spelled in NFD, such as on macOS, pass `locations[].path` from the list as is.
- A missing video, or a video without a location under the registered folders, returns `404`
  (`video_not_found`).

## Tag videos

`POST /api/v1/video-tags` adds, removes or replaces tags, given by name, on several videos in one
request. The body is JSON; send `Content-Type: application/json`.

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }, { "contentKey": "…" }, { "id": 12 }],
  "action": "add",
  "tags": ["猫", "ねこ"] }
```

- Each element of `videos` has exactly one of `id`, `contentKey` and `path` (the same lookup as
  `lookup`). 1–20000 elements. The body is limited to 32 MiB (33554432 bytes); a larger body is
  rejected with `400` (`invalid_request`). Split the request when sending many long paths.
- `action` is one of the following. Each rewrites only manually added tags and never changes tags
  from ancestor folder names (`fromFolder`).
  - `add`: adds the named tags. An unknown name creates the tag.
  - `remove`: removes the named tags. A name that matches no tag does nothing.
  - `replace`: sets the manually added tags to exactly the `tags` set. An unknown name creates
    the tag. An empty `tags` removes every manually added tag.
- `tags` are tag names; synonyms work too (a synonym is applied as its original tag). Names that
  resolve to the same tag are merged into one. Up to 100 names; at least 1 for `add` and `remove`.
- The response is `{ items: [{ video: { id, contentKey }, tags }] }`: each video's tags after the
  operation, in `videos` order. `tags` has the same shape as in a list item.
- The whole request runs in one transaction. If any video cannot be resolved, it returns `404`
  (`video_not_found`, with `index` as the position in `videos`) and applies nothing (it creates
  no tags either).
- Repeating the same request does not change the state and returns `200`. After a network
  failure, resend the request as is.
- Count errors return `400` (`too_many_videos`, `too_many_tags`, with the limit in `limit`). Name
  errors return `400` (`tag_name_empty`, `tag_name_control_characters`, `tag_name_too_long`), with
  `index` as the position in `tags`.

## Example: integration from a scraper

This example looks up newly imported videos on an external site and adds the tags it finds.

1. Start a scan (`POST /api/v1/scans`) and wait until `status` from `GET /api/v1/scans/current`
   is `done`, `partial` or `failed`.
2. Read the list (`GET /api/v1/videos`) again from the start, and pick each `contentKey` without a
   local record as a new video. `contentKey` does not change when a file moves, so use it as the
   key of the local records.
3. For each new video, search the external site by `locations[].path` or `title`. To recheck its
   state later, look up one video with `GET /api/v1/videos/lookup?contentKey=…` (`404` means it
   is gone).
4. Add the found names with `POST /api/v1/video-tags`. Send videos that get the same set of names
   together in one request.

```sh
# 2. Pick contentKey and the representative path from the list (paging: see "List videos").
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=200" |
  jq -r '.items[] | [.contentKey, .locations[0].path] | @tsv'

# 4. Add the tags found by the search.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-tags" \
  -d '{"videos":[{"contentKey":"…"}],"action":"add","tags":["猫","旅行"]}'
```

- To realign the tags with the external results, use `replace`. It also replaces tags added
  manually in the UI, so when you use the UI as well, send only the difference with `add` and
  `remove`.
- When a request fails with `404`, drop the video at `index` from the records or look it up again
  with `lookup`, then resend.

## Use from MCP

The MCP server at `/mcp` (Streamable HTTP) exposes the same operations as tools. Use it from an
MCP client such as Claude Code, with the same token as the external API.

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

| Tool | Operation |
| --- | --- |
| `list_videos` | `GET /api/v1/videos` |
| `get_video` | `GET /api/v1/videos/lookup` |
| `list_tags` | `GET /api/v1/tags` |
| `update_video_tags` | `POST /api/v1/video-tags` |
| `start_scan` | `POST /api/v1/scans` |
| `get_current_scan` | `GET /api/v1/scans/current` |

- Tool arguments have the same shape as the operation's query and body, and the result
  (structured content) the same shape as the response body. An operation error returns a tool
  result with `isError: true` and the error body above (`{ code, message, reason?, limit?,
  index? }`).
- The server accepts only `POST /mcp` and responds with `application/json`. It keeps no session
  state (`GET` and `DELETE` return `405`).
- A missing or invalid token returns `401` with `WWW-Authenticate: Bearer` before MCP processing
  starts. Revoking a token also aborts its running tools.
