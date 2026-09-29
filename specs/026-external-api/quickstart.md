# Quickstart: Verify the external API and MCP end to end

These steps verify the acceptance criteria of parent Issue #493 on a running server. `task check`
runs each unit's automated tests. These steps cover the UI, a real MCP client and a scan. Record the
result in the PR body of the last unit ("Serve the external operations as MCP tools on `/mcp`").

## Prerequisites

- The server runs with `task dev` as in
  [docs/how-to/development.md](../../docs/how-to/development.md). You are logged in as the owner, one
  media folder is added, and a scan has finished. Below, `BASE=http://localhost:8080`.
- At least one private video exists (private is the default).
- Claude Code (the `claude` command) for the MCP check.

## Steps

1. In the "API tokens" section of the settings page, enter a name and create a token. The plaintext
   is shown once and can be copied. After a reload, only the name and the times remain in the list
   (Acceptance criterion 1). Below, that plaintext is `TOKEN`.
2. `curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos"` returns videos, including private
   ones. The last use time appears in the list on the settings page (Acceptance criterion 2).
3. Follow `cursor` until `nextCursor` is empty; every video is returned. Add one video to the media
   folder and scan. Reading again from the start includes that video (Acceptance criterion 3).
4. Look it up with `GET /api/v1/videos/lookup?path=<absolute path of that video>`. Send
   `{"videos":[{"path":"…"}],"action":"add","tags":["新しい名前","<synonym of an existing tag>"]}` to
   `POST /api/v1/video-tags`. The new tag is created, and the synonym is attached as its original
   tag. Both appear in the UI's video detail and tag filter (Acceptance criterion 4). Sending the
   same request again returns `200` and does not change the tags (Acceptance criterion 5).
5. A call with a token revoked in the UI returns `401`. Create a new token, then run
   `mdm account set-password`. A call with that token returns `401`, and the token is gone from the
   list (Acceptance criterion 6).
6. `GET /api/v1/videos` with only the browser's session Cookie returns `401`. `GET /api/videos` with
   only Bearer returns `X-VV-Audience: guest` and no private videos. `GET /api/api-tokens` and
   `POST /api/api-tokens` with only Bearer return `401` (Acceptance criteria 7 and 8).
7. Add the server with
   `claude mcp add --transport http vv "$BASE/mcp" --header "Authorization: Bearer $TOKEN"`. The
   `/mcp` list shows the six tools of [contracts/mcp.md](contracts/mcp.md). Tags attached with
   `update_video_tags` appear in the UI. `curl -X POST "$BASE/mcp"` (no token) and an invalid token
   return `401` (Acceptance criterion 9).

## Expected results

Every step above holds.
