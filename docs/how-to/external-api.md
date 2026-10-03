# Use the external API

External tools such as scrapers operate VVMDM with an API token. The reasons
behind the decisions are in
[specs/026-external-api/research.md](../../specs/026-external-api/research.md).

## Contract

- The source of truth for the contract is
  [api/external-v1.yaml](../../api/external-v1.yaml) (OpenAPI 3.1). Read it for
  the operations, the fields, and the error `code` and `reason` values. The
  server does not serve it.
- The base path is `/api/v1`. It is a separate contract from the `/api/*` the
  screens use ([api/openapi.yaml](../../api/openapi.yaml)); the screen API
  changes without notice. External tools use `/api/v1` only.

## Compatibility policy

- Within `v1`, fields and operations are only added. Write clients to skip
  unknown fields and unknown `code` and `reason` values.
- Changing the meaning, type or requiredness of an existing field, or removing
  an operation, adds `v2` and leaves `v1` unchanged.

## Create a token

1. Sign in as the owner, enter a name for the token's purpose in the
   "API tokens" section of the Settings page, and create the token.
2. Copy the plaintext token shown (47 characters starting with `vvt_`) right
   away. It is shown only this once. If you lose it, revoke it and create a new
   one.
3. Revoke tokens you no longer use in the same section. Changing the username or
   password (`mdm account`) revokes every token.

A token has the same rights as the owner (it can read private videos too). When
you keep it in a configuration file or a log, treat it like a password.

## Call the API

Send `Authorization: Bearer <token>` with every request. Cookies are not read.

```sh
BASE=http://localhost:8080
TOKEN=vvt_...

curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/tags"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=100"
curl -X POST -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans/current"
```

- A missing, malformed, invalid or revoked token returns `401`
  (`code: unauthenticated`) with `WWW-Authenticate: Bearer`. The cause is not
  distinguished.
- There is no same-origin check, so clients that send `Origin` work too.
  Operations that take a body require `Content-Type: application/json`.
- The error body is `{ code, message, reason?, limit?, index? }`. `message` is
  an English description; branch on `code` and `reason`.
- The last-used time appears in the token list on the Settings page. It is not
  updated more often than once a minute.
- Revoking a token also cuts off its requests in progress.

## Scans

- `POST /api/v1/scans` takes no body. It returns `201` when it starts a new
  scan, or `200` with the running scan without starting a new one. Without media
  folders it returns `409` (`media_folders_not_configured`).
- `GET /api/v1/scans/current` reads the latest state. The scan has finished when
  `status` is `done`, `partial` or `failed`. While it is `finding`, the video
  count is not settled, so `videos` and `settledVideos` are `null`. If no scan
  has ever run, it returns `404` (`no_scan`).

## Read the video list

- `GET /api/v1/videos` returns the videos under registered folders, oldest
  added first (`addedAt`, then `id`). Private videos are included. The page size
  is `limit` (default 100, 1 to 200; out of range returns `400`).
- The response is `{ items, nextCursor }`. When `nextCursor` is not an empty
  string, pass it unchanged as `cursor` to read the next page. An empty string
  means you have read everything. Do not interpret the cursor's content. A
  cursor that cannot be parsed returns `400` (`invalid_cursor`); read again from
  the start.

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

- The list does not track changes. To find new videos, read the list again from
  the start after a scan. Videos added, removed or moved while you page through
  do not make the next request fail, but they can be missed or repeated.
- A removed video only disappears from the list; no field reports the removal.
  To check whether a video you hold still exists, look it up with `lookup`; `404`
  (`video_not_found`) means it is gone. A video whose only remaining locations
  are outside registered folders is treated the same as a removed video.
- Fields of a list item:

  | Field | Meaning |
  | --- | --- |
  | `locations` | The current locations under registered folders, ordered by path; the first is the primary location |
  | `title` | The effective title: `displayName` when set, otherwise the title derived from the primary location's file name (`fileTitle`) |
  | `displayName` | The display name; `null` when unset |
  | `thumbnailPositionMs` | The position of the cover thumbnail in milliseconds; `null` when unset |
  | `durationMs` | `null` before analysis |
  | `tags.manual` | Tags added by hand |
  | `tags.fromFolder` | Tags derived from ancestor folder names |
  | `tags.tentative` | Tentative tags (see [Add tags as tentative tags](#add-tags-as-tentative-tags)) |
  | `updatedAt` | When the video's information (display name, tags, visibility, cover thumbnail) was last edited in vv; equal to `addedAt` if never edited. Changes through this API's `video-tags` and `display-names` advance it; a request that changes nothing does not |
  | `fileCreatedAt` | The creation time of the file at the primary location (the first of `locations`); the file's modification time (mtime) when the file system does not provide one |

## Look up one video

`GET /api/v1/videos/lookup` takes exactly one of `id`, `contentKey` and `path`
(zero or two or more return `400`). The response has the same shape as a list
item.

```sh
curl -G -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos/lookup" \
  --data-urlencode "path=/media/videos/clip.mp4"
```

- `path` is not normalized; it is compared byte for byte with the location's
  path. For file names spelled in NFD (as on macOS), pass `locations[].path`
  from the list unchanged.
- A video that does not exist, or has no location under a registered folder,
  returns `404` (`video_not_found`).

## Tag videos

`POST /api/v1/video-tags` adds, removes or replaces tags, given by name, on
several videos in one request. The body is JSON; send
`Content-Type: application/json`.

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }, { "contentKey": "…" }, { "id": 12 }],
  "action": "add",
  "tags": ["猫", "ねこ"] }
```

- Each element of `videos` has exactly one of `id`, `contentKey` and `path`
  (the same lookup as `lookup`). 1 to 20000 elements. The body is limited to
  32 MiB (33554432 bytes); a larger body is refused with `400`
  (`invalid_request`). Split requests that send many long paths.
- `action` is one of the following. Each changes only tags added by hand and
  leaves tags derived from ancestor folder names (`fromFolder`) unchanged.

  | `action` | Effect |
  | --- | --- |
  | `add` | Adds the named tags. Creates a tag for a name that does not exist. |
  | `remove` | Removes the named tags. A name that matches no tag does nothing. |
  | `replace` | Makes the hand-added tags exactly the set in `tags`. Creates a tag for a name that does not exist. An empty `tags` removes every hand-added tag. |

- `tags` holds tag names; synonyms work too (a synonym adds its tag). Names that
  resolve to the same tag are merged. Up to 100; at least 1 for `add` and
  `remove`.
- The response is `{ items: [{ video: { id, contentKey }, tags }], skippedTags }`,
  with each video's tags after the operation, in the order of `videos`. `tags`
  has the same shape as in a list item.
- The whole request runs in one transaction. If any video cannot be found, it
  returns `404` (`video_not_found`, with `index` giving the position in
  `videos`) and applies nothing (it creates no tags either).
- Repeating the same request leaves the state unchanged and returns `200`.
  After a network failure, send it again as is.
- Count errors return `400` (`too_many_videos`, `too_many_tags`, with the limit
  in `limit`). Name errors return `400` (`tag_name_empty`,
  `tag_name_control_characters`, `tag_name_too_long`), with `index` giving the
  position in `tags`.

### Add tags as tentative tags

Adding `"tentative": true` to the body makes the tags that `add` and `replace`
newly create **tentative tags**. A tentative tag appears as tentative in the
screen's tag list and stays tentative until the user confirms or rejects it on
the Tags page.

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }],
  "action": "add",
  "tags": ["猫", "高画質"],
  "tentative": true }
```

- `tentative` is a boolean and defaults to `false`. A non-boolean returns `400`
  (`invalid_request`).
- A name that matches an existing tag's name or synonym adds that tag regardless
  of `tentative`, and does not change whether the tag is tentative or
  confirmed.
- A name that matches no tag is created as a tentative tag unless it is a
  rejected name (exact spelling match). A rejected name creates and adds no tag;
  it is skipped, and the response's `skippedTags` returns the normalized name
  once, in the order of `tags`. The other names are added and the request
  succeeds with `200`. With `replace`, skipped names are not in the resulting
  set.
- With `remove`, `tentative` changes nothing.
- Without `tentative` (`false`), a name that does not exist is created as a
  confirmed tag. If the name was rejected, it is removed from the rejected names.
- `skippedTags` is always present; it is an empty array when nothing was
  skipped. Each element of `tags` and each tag of `GET /api/v1/tags` reports
  whether it is tentative in `tentative`.
- Confirming and rejecting tentative tags, and viewing and clearing rejected
  names, are screen operations; this API does not have them.

## Set display names

`POST /api/v1/video-display-names` sets or clears the display names of several
videos at once. A display name appears on the screens and in this API's
`title`, and is used for sorting and search. The original file is not changed.

```json
{ "items": [{ "video": { "path": "/media/videos/clip.mp4" }, "displayName": "旅行 2024 夏" },
            { "video": { "id": 12 }, "displayName": null }] }
```

- `video` in each element of `items` has exactly one of `id`, `contentKey` and
  `path` (as in `video-tags`). `displayName` is required; `null`, or a string
  that is empty after trimming surrounding whitespace, clears it (the title
  returns to the one derived from the file name). 1 to 20000 elements; the body
  is limited to 32 MiB.
- A display name is bound to the content (`contentKey`). It survives moving the
  file, and other locations with the same content show the same name. The same
  name can be given to other videos.
- The response is
  `{ items: [{ video: { id, contentKey }, title, fileTitle, displayName }] }`,
  with the values after the change, in the order of `items`.
- The whole request runs in one transaction and applies nothing on any of these
  errors, each with `index` giving the position in `items`:

  | Condition | Error |
  | --- | --- |
  | The video cannot be found | `404` (`video_not_found`) |
  | The name contains control characters | `400` (`display_name_control_characters`) |
  | The name is longer than 200 characters | `400` (`display_name_too_long`, `limit: 200`) |

## Change the cover thumbnail position

`POST /api/v1/video-thumbnails` regenerates the cover thumbnails of several
videos from the given scenes.

```json
{ "items": [{ "video": { "contentKey": "…" }, "positionMs": 12500 },
            { "video": { "id": 12 }, "positionMs": null }] }
```

- `positionMs` is required: milliseconds from the start of the video (0 or more,
  less than the duration). `null` clears it and regenerates at the automatic
  position.
- An image is generated per item, so one request takes 1 to 20 items (more
  returns `400` `too_many_videos`, `limit: 20`). To change many videos, send
  them 20 at a time in sequence.
- All items are validated first; on any error nothing is generated. Each error
  returns `index`:

  | Condition | Error |
  | --- | --- |
  | The video cannot be found | `404` (`video_not_found`) |
  | No location of the video can be opened | `404` (`file_unavailable`) |
  | The position is at or past the duration, or negative | `400` (`thumbnail_position_out_of_range`, with the video's duration in `limit`) |
  | The video has not been analysed yet | `409` (`duration_unknown`) |

- After validation, images are generated one item at a time in the order of
  `items`. If one item's image cannot be generated, the request stops with `409`
  (`thumbnail_frame_unavailable`) and `index`. Items before `index` are applied;
  that item and the ones after it are not. To continue, change or drop the
  position of the item at `index` and resend only the items from `index` on
  (resending an applied item only regenerates it at the same position).
- A scan can change videos or locations during generation, so each item reloads
  its video and resolves its location again just before generation. If that
  hits one of the validation errors above (`video_not_found`,
  `file_unavailable` and so on) or an unexpected failure (`500`), the request
  also stops with `index`, and items before `index` are applied.
- The response is `{ items: [{ video: { id, contentKey }, thumbnailPositionMs }] }`.
  It returns after the images are generated, which can take several seconds per
  item.

## Example: integrate a scraper

This flow looks up newly scanned videos on an external site and tags them with
what it finds.

1. Start a scan (`POST /api/v1/scans`) and wait until `status` from
   `GET /api/v1/scans/current` is `done`, `partial` or `failed`.
2. Read the list (`GET /api/v1/videos`) again from the start and pick up each
   `contentKey` you have no record of as a new video. `contentKey` does not
   change when a file moves, so use it as the key of your records.
3. For each new video, search the external site using `locations[].path` or
   `title`. To recheck a video's state later, look it up with
   `GET /api/v1/videos/lookup?contentKey=…` (`404` means it is gone).
4. Add the names you found with `POST /api/v1/video-tags`. Send videos that get
   the same set of names in one request. Add names produced automatically, such
   as by an LLM, with `"tentative": true`, and let the user confirm or reject
   the newly created tags on the Tags page. A name rejected once is not created
   again and comes back in `skippedTags`.
5. When you find the external site's official title, set it as the display name
   with `POST /api/v1/video-display-names`. This adjusts the title on the
   screens without renaming the file.

```sh
# 2. Pick up contentKey and the primary path from the list (paging is in "Read the video list").
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=200" |
  jq -r '.items[] | [.contentKey, .locations[0].path] | @tsv'

# 4. Add the tags found. Names produced automatically are created as tentative tags.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-tags" \
  -d '{"videos":[{"contentKey":"…"}],"action":"add","tags":["猫","旅行"],"tentative":true}'

# 5. Set the title found as the display name.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-display-names" \
  -d '{"items":[{"video":{"contentKey":"…"},"displayName":"旅行 2024 夏"}]}'
```

- To make the tags match the external results again, use `replace`. It also
  replaces tags added by hand on the screens, so when you also use the screens,
  send only the difference with `add` and `remove`.
- When a request fails with `404`, drop the video at `index` from your records or
  look it up again with `lookup`, then resend.

## Use from MCP

The MCP server at `/mcp` (Streamable HTTP) exposes the same operations as tools.
Use it from an MCP client such as Claude Code, with the same token as the
external API.

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
| `update_video_display_names` | `POST /api/v1/video-display-names` |
| `update_video_thumbnails` | `POST /api/v1/video-thumbnails` |

- A tool's arguments have the same shape as the operation's query and body, and
  its result (structured content) the same shape as the response body. An
  operation error comes back as a tool result with `isError: true` and the error
  body above (`{ code, message, reason?, limit?, index? }`).
- Only `POST /mcp` is accepted, and responses are `application/json`. The server
  keeps no session state (`GET` and `DELETE` return `405`).
- A missing or invalid token returns `401` with `WWW-Authenticate: Bearer`
  before MCP processing starts. Revoking the token also cuts off tools in
  progress.
