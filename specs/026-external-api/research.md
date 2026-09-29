# Research: External API and MCP

Parent Issue: #493.

Inherited technical decisions are in
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) and
[ARCHITECTURE.md](../../ARCHITECTURE.md): a single Go binary, SQLite, generation from
`api/openapi.yaml` as the source of truth, the authentication boundary `internal/httpapi/auth.go`, and
single-account sessions ([specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)).
This document records only the decisions this feature adds.

## R-1: A token is a prefixed 256-bit random value; only its SHA-256 is stored

| | |
| --- | --- |
| **Decision** | The plaintext is `vvt_` followed by the base64url (no padding, 43 characters) of 32 random bytes: 47 characters. The DB stores only the SHA-256 (hex) of the plaintext, and every request looks the token up by it. A value with the wrong format returns 401 without a DB lookup. |
| **Why** | It is built like sessions (`sessions.token_hash`), so the existing functions and test shapes of `internal/store` and `internal/app` apply unchanged. A 256-bit random value cannot be brute-forced, so a slow password hash is not needed. The prefix lets users tell a vv token apart in config files and logs, and lets leak scanners have a rule for it. |
| **Rejected** | Argon2id storage: tens of milliseconds per request, and no benefit for a high-entropy value. Splitting into an identifier and a secret and looking up by the identifier: the SHA-256 primary key already finds the row in one lookup, so the split has no benefit. |

## R-2: An account change deletes the token rows; a version match is checked too

| | |
| --- | --- |
| **Decision** | Each `api_tokens` row holds the `account.version` at issue time. A token is valid only when `api_tokens.account_version = account.version`. A username or password change (`AuthStore.changeCredentials`) deletes all `api_tokens` rows in the same transaction as `sessions`. The `mdm account` output adds that all API tokens were revoked too. |
| **Why** | Requirement 8. Deleting the rows also removes them from the settings page list, so no unusable token stays listed. The version match covers a race between an issue in the UI (a session that read an old version) and the host command: a token created under old credentials does not survive. This is the same reason as item 3 of §4 for sessions. |
| **Rejected** | Keeping the rows and showing them as "invalid": useless rows stay in the list, and the user recreates the token anyway. No requirement asks for it. |

## R-3: The external API lives under `/api/v1/`; the boundary gets a "Bearer" class

| | |
| --- | --- |
| **Decision** | The external API is at `/api/v1/…` and MCP at `/mcp` (fixed by Requirement 9). `classifyRequest` gets a fourth class, `accessBearer`, chosen when the `path.Clean` path is under `/api/v1/` or is `/mcp`. The rules are listed below. |
| **Why** | Splitting UI API and external API authentication by path prefix means the path alone decides which credential handles a request. This fits the current `accessRoutes` and `openapi_routes_test.go` unchanged. Under `/api/`, the API takes no name from the SPA route namespace, and the current mechanism returns the JSON 404 for undefined paths. |
| **Rejected** | Top-level `/v1/`: takes a name from the SPA routes (everything outside `/api/` is SPA) and needs separate SPA fallback handling and a separate JSON 404. Accepting both Cookie and Bearer on the same paths: Requirement 5 forbids Cookie use, and the UI API behavior would change too. |

- The Bearer class does not read Cookies. Only `Authorization: Bearer <token>` decides the owner. A
  missing, malformed, invalid or revoked token returns `401 unauthenticated` with
  `WWW-Authenticate: Bearer` (Requirement 7, Acceptance criterion 7).
- Every other `/api/*` path reads only Cookies as today and ignores `Authorization`. A request with
  only Bearer becomes a guest (second half of Acceptance criterion 7). The token management API is
  here too, so Bearer cannot use it (Requirement 4, Acceptance criterion 8).
- Undefined paths under `/api/v1/` are also in the Bearer class and return a JSON 404 after
  authentication.
- Encoded separators (`%2F` and similar) keep the current rule: if either form is under `/api/`, the
  request falls to owner only. The Bearer class is chosen only when the forms before and after
  decoding match.

## R-4: Bearer requests skip the same-origin check

| | |
| --- | --- |
| **Decision** | The same-origin check of `mutationBoundary` (`acceptsSameOrigin`) applies only to Cookie-class requests. Bearer-class requests accept any `Origin`. The rule that requires a JSON body applies to the external API too. |
| **Why** | The same-origin check stops CSRF that uses the Cookie a browser sends automatically. A browser never adds `Authorization` automatically, so that attack does not exist here. Scrapers and MCP clients send no `Origin`, or an origin different from vv. |
| **Rejected** | Applying the check as is: clients that send `Origin` (tools running in a browser, for example) are rejected for no reason. There is nothing to protect. |

## R-5: The external API contract is a separate OpenAPI document; only Go is generated

| | |
| --- | --- |
| **Decision** | `api/external-v1.yaml` is the source of truth for the external API. `api/oapi-codegen-external.yaml` generates Go types and handler interfaces into `internal/httpapi/extgen/`. No TypeScript is generated. `scripts/generate` generates it too, and `task generate-check` checks the diff. The document itself is the published contract, linked from `docs/how-to/external-api.md`. The server does not serve it. Compatibility policy: within `v1`, only fields and operations are added; changing the meaning, type or requiredness of an existing field adds `v2`. |
| **Why** | The UI API changes with the UI's needs (Background); the external API promises not to change. In one document, you cannot tell from the document whether a change breaks the external promise. In a separate document, the diff of `api/external-v1.yaml` is the change of the promise itself. The UI never calls the external API, so it needs no TypeScript types. |
| **Rejected** | Adding to `api/openapi.yaml` and splitting by tag: the reason above, plus external types would mix into the TypeScript output for the UI. Serving it as `GET /api/v1/openapi.yaml`: needs an exception that embeds `api/` in the binary (the same kind as `web/embed.go`), and publishing it in the repository already meets the requirement. |

## R-6: The video list is read with an `(added_at, id)` keyset cursor

| | |
| --- | --- |
| **Decision** | `GET /api/v1/videos` returns videos with a location under a media folder in ascending `(added_at, id)`. It pages with the same keyset cursor as the UI list: an opaque string encoding the sort value and the id, built like `encodeCursor` and `decodeCursor` in `internal/store/listing.go`. The response is `{ items, nextCursor }`; `nextCursor` is empty when nothing follows. The rules are listed below. |
| **Why** | The parent Issue asks for running "find new videos, then tag them", not for fetching differences (Requirement 6.1, Acceptance criterion 3). Reading the list again is enough. No change-tracking column or counter is needed, and no rule that advances a number on every write of scans and media folder operations. The UI's `ListVideos` already uses keyset, so its read and test shapes carry over. `(added_at, id)` does not change when a row moves, so the next request resumes at the same place. |
| **Rejected** | See the list of rejected alternatives below. |

- No change tracking. There is no endpoint for "only new videos since last time" and none that
  reports changed or removed videos. A client that wants new videos reads the list again, and learns
  that a held video is gone from the `404` of `lookup`. No table or column is added.
- When videos are added, removed or moved during paging, the next request does not fail and returns
  what follows at that moment. The API does not promise to prevent misses or duplicates across one
  full read (Edge cases).
- The list returns only videos with a location under a media folder. A video that remains only with
  locations outside the media folders is not returned, like a deleted row.

Rejected alternatives:

| Alternative | Why rejected |
| --- | --- |
| A cursor on monotonic numbers (`added_seq`, `changed_seq`) that returns only videos added or changed since last time | Needs a rule that advances the number at every write site (creating a video row; adding, removing or reassigning a location; adding, replacing or removing a media folder; probing the duration) and tests for misses and duplicates at each. The requester judged a re-read of the list enough, so it is out of this feature. |
| `offset` pages | Misses and duplicates appear the moment a scan adds or removes rows. The same reason the UI list chose keyset (`listing.go`). |
| `updated_at` order | Changes with every ingest stage, so the order shifts during one full read. |

## R-7: Tag operations are a strict bulk operation added to `TagStore`

| | |
| --- | --- |
| **Decision** | Add `TagStore.ApplyVideoTags(ctx, videos []domain.VideoRef, action, names)`. In one transaction it resolves each video reference (id, content key or location path) to a video now in the library. If any one fails, the whole call fails with a video-not-found error (a value that wraps `domain.ErrNotFound` and holds the reference's position). `internal/app` is not involved. The rules per action are listed below. |
| **Why** | Requirements 6.4 and 10. The current attach (`AttachTagByName`) fits in one transaction, so it skips `internal/app` (see the comment on `httpapi.Tags`); this operation lives in store for the same reason. The UI attach skips removed videos, but for the external API the Edge case requires the whole request to fail, so the resolve rule differs. Repeating the same attach or detach succeeds without changing the state (Acceptance criterion 5). Attach and detach insert and delete per `(content_key, tag_id)` row, so both survive a concurrent attach from the UI. |
| **Rejected** | `replace` also removing folder-derived tags: those are derived from folder names every time, so cancelling them per video needs a new mechanism (an exclusion table). The parent Issue does not ask for it, and the UI has no such feature. One tag per call, like the UI's `POST /api/video-tags`: a scraper attaches several tags to one video, so requests multiply and the tags split across transactions. A new use case in `internal/app`: no side effects or events, and it closes in one transaction; it would no longer match where current tag operations live. |

- `add`: look up each name including synonyms, and create it when missing (`findOrCreateTag`, the
  same as the UI attach).
- `remove`: look up each name including synonyms, and detach only existing tags. A name that matches
  no tag does nothing.
- `replace`: look up or create each name like `add`, and make each video's manually attached tags
  (`video_tags` rows) exactly that set. An empty set detaches every manually attached tag.
- All three change only manually attached tags. Tags derived from ancestor folder names
  ([017 data-model.md §4](../017-folder-groups/data-model.md#4-folder-derived-tags)) do not change.
  This is the same rule as the UI's detach (`DetachTag`). Tags in the response carry their origin
  (`manual`, `fromFolder`, `domain.VideoTag`), so a client can tell which folder-derived tags remain.
- Names that resolve to the same tag are merged into one. The result is each video's tags after the
  operation.
- Names are validated with `domain.NormalizeTagName`. The video count limit is the same as the UI's
  `maxVideoTagsIDs` (20000). The name count limit is 100.

## R-8: MCP uses the official Go SDK in stateless mode, inside `internal/httpapi`

| | |
| --- | --- |
| **Decision** | Use `mcp.NewStreamableHTTPHandler` of `github.com/modelcontextprotocol/go-sdk` (as of v1.8.0) with `Stateless: true` and `JSONResponse: true`, mounted on `/mcp`. The handler lives in `internal/httpapi`, inside the boundary (the Bearer class of R-3). Tools map one to one to external API operations, and input and output are the same JSON as `api/external-v1.yaml`. The implementation calls the same functions as the REST handlers. |
| **Why** | Requirement 9. In stateless mode the server keeps no per-client conversation state and verifies Bearer on every request, so no conversation of a revoked token remains. Tools are request and response only; no server-initiated notification is needed. In `internal/httpapi`, the video response conversion and the authentication boundary are shared with REST, and the rule on imports between sibling packages (depguard) is not touched. |
| **Rejected** | `github.com/mark3labs/mcp-go`: widely used, but the official SDK is maintained with a v1 compatibility promise. Hand-written JSON-RPC: we would own version negotiation and the details of Streamable HTTP. Stateful (session ID and a GET stream): the server holds conversation state and must manage its lifetime apart from token revocation. A separate `internal/mcpapi` package: duplicates the video response conversion and needs separate boundary wiring. |

## R-9: The last use time is written at most once a minute; a revoke also stops running requests

| | |
| --- | --- |
| **Decision** | On a request with a valid token, when the stored `last_used_at` is empty or 60 s or more in the past, it is rewritten immediately (a conditional `update`). A write failure is logged, and the request continues. Bearer requests are also registered in the boundary's `sessionLedger`. A revoke in the UI immediately cuts off that token's running responses. A revoke by the host command cuts them off at the current 30 s recheck. |
| **Why** | Edge cases (the last use time may be thinned; long requests are cut off by a revoke). With a conditional `update`, any number of calls per minute write once, and no buffer-in-memory mechanism is needed. The current mechanism that stops session requests on logout and host commands applies unchanged. |
| **Rejected** | Buffering in memory and writing periodically: lost on shutdown, and adds a flush goroutine and shutdown ordering. |

## R-10: Token names follow the same rule shape as tag names

| | |
| --- | --- |
| **Decision** | A name is 1 to 100 characters (Unicode code points) after trimming leading and trailing whitespace, with no control characters. The rule is `domain.NormalizeAPITokenName`. Errors return the UI API's `invalid_request` with new `reason` values (`api_token_name_empty`, `api_token_name_control_characters`, `api_token_name_too_long`, the limit in `limit`). Duplicate names are allowed. |
| **Why** | Edge cases (an empty or too long name is not issued; several tokens may share a name). Fitting the UI error design ([023 error-api.md](../023-english-i18n/contracts/error-api.md)) lets the UI build the message from `reason`. |
| **Rejected** | Reusing the tag name `reason` values: the UI message would say "tag name" and point the user at the wrong thing. |
