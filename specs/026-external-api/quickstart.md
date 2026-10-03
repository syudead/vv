# Quickstart: Check the external API and MCP end to end

These steps check the acceptance criteria of parent Issue #493 on a running server, through the
screen, a real MCP client and a scan; `task check` runs each unit's automated tests. The result is
recorded in the body of the implementation PR for the last unit ("Offer the external operations as
MCP tools at `/mcp`").

## Prerequisites

- Started with `task dev` as in [docs/how-to/development.md](../../docs/how-to/development.md),
  logged in as the owner, with one media folder registered and a scan finished. Below,
  `BASE=http://localhost:8080`.
- At least one private video (videos are private by default).
- Claude Code (the `claude` command) for the MCP check.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| 1. In the "API tokens" section of the settings page, enter a name and create a token. Call the plaintext `TOKEN` below. | The plaintext is shown once and can be copied. After a reload, only the name and dates remain in the list. | 1 |
| 2. `curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos"` | Returns videos including private ones. The last-used time appears in the list on the settings page. | 2 |
| 3. Follow `cursor` until `nextCursor` is empty. Add one video to the media folder, scan, and read again from the start. | Every video is returned. The re-read includes the new video. | 3 |
| 4. Look the video up with `GET /api/v1/videos/lookup?path=<absolute path of that video>`. Send `{"videos":[{"path":"…"}],"action":"add","tags":["新しい名前","<synonym of an existing tag>"]}` to `POST /api/v1/video-tags`, then send the same request again. | A new tag is created and the synonym is attached as its original tag; both show in the video details and the tag filter on the screen. The repeated request returns `200` and the tags do not change. | 4, 5 |
| 5. Call with a token revoked on the screen. Then create a new token, run `mdm account set-password`, and call with that token. | Both calls return `401`, and the second token is also gone from the list. | 6 |
| 6. Call `GET /api/v1/videos` with only the browser session cookie. Call `GET /api/videos` with only the Bearer token. Call `GET /api/api-tokens` and `POST /api/api-tokens` with only the Bearer token. | The first returns `401`. The second responds with `X-VV-Audience: guest` and shows no private videos. The last two return `401`. | 7, 8 |
| 7. Add the server with `claude mcp add --transport http vv "$BASE/mcp" --header "Authorization: Bearer $TOKEN"`. Tag a video with `update_video_tags`. Run `curl -X POST "$BASE/mcp"` (no token) and call with an invalid token. | The six tools in [contracts/mcp.md](contracts/mcp.md) are listed for `/mcp`. The tag shows on the screen. Both calls without a valid token return `401`. | 9 |
