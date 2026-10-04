# Research: External API and MCP

Parent Issue: #493.

Inherited decisions: [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
and [ARCHITECTURE.md](../../ARCHITECTURE.md) (a single Go binary, SQLite, code generated from
`api/openapi.yaml` as the source of truth, the authentication boundary in `internal/httpapi/auth.go`,
and the single-account session in
[specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)).
This file records only the decisions this feature adds.

## R-1: Tokens are prefixed 256-bit random values, and only the SHA-256 is stored

**Decision**: The plaintext is `vvt_` followed by the unpadded base64url encoding of 32 random
bytes (43 characters), 47 characters in total. The database holds only the SHA-256 (hex) of the
plaintext, and every request looks the token up by that hash. A value with the wrong format is
rejected with 401 without querying the database.

**Rationale**: This is the same design as sessions (`sessions.token_hash`), so the existing
functions and test shapes in `internal/store` and `internal/app` carry over unchanged. A 256-bit
random value cannot be brute-forced, so a slow password hash is unnecessary. The prefix lets users
recognise a vv token in configuration files and logs, and makes it possible to write leak-detection
rules.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Store with Argon2id | Rejected: costs tens of milliseconds per request and adds nothing for a high-entropy value. |
| Split into an identifier and a secret and look up by identifier | Rejected: a SHA-256 primary key already finds the row in one lookup, so splitting gains nothing. |

## R-2: Changing the account deletes the token rows, and the version is checked as well

**Decision**: Each `api_tokens` row carries the `account.version` at the time it was issued. A
token is valid only when `api_tokens.account_version = account.version`. Changing the username or
password (`AuthStore.changeCredentials`) deletes every `api_tokens` row in the same transaction as
`sessions`. The `mdm account` output gains a statement that every API token was revoked too.

**Rationale**: Requirement 8. Because the rows are deleted, they also disappear from the list on the
settings page, so unusable tokens never linger in it. The version check covers a race between an
issue on the screen (by a session that read the old version) and the host command: a token created
under the old credentials must not survive. This is the same reason as item 3 of §4 of the session data model (016).

**Alternatives considered**: Keep the rows and show them as "invalid". Rejected: rows with no use
stay in the list and the user ends up creating new tokens anyway. The requirement does not ask for
it either.

## R-3: The external API lives under `/api/v1/`, and the boundary gains a Bearer class

**Decision**: The external API lives at `/api/v1/…` and MCP at `/mcp` (fixed by requirement 9).
`classifyRequest` gains a fourth class, `accessBearer`, chosen when the `path.Clean`ed path is under
`/api/v1/` or is `/mcp`.

| Request | Behaviour |
| --- | --- |
| Bearer class | Cookies are not read; the owner is determined from `Authorization: Bearer <token>` alone. A missing, malformed, invalid or revoked token returns `401 unauthenticated` with `WWW-Authenticate: Bearer` (requirement 7, acceptance criterion 7). |
| Any other `/api/*` path | As today, only the cookie is read and `Authorization` is ignored. A request carrying only a Bearer token is a guest (second half of acceptance criterion 7). The token management API is here too, so it cannot be used with a Bearer token (requirement 4, acceptance criterion 8). |
| Undefined path under `/api/v1/` | Also the Bearer class; after authentication it returns a JSON 404. |
| Encoded separators (`%2F` and so on) | The current rule stands: if either form is under `/api/`, the request falls to owner-only. The Bearer class is chosen only when the path before and after decoding agree. |

**Rationale**: Splitting authentication for the screen API and the external API by path prefix
means the path alone decides which credential handles a request, which fits the current
`accessRoutes` and `openapi_routes_test.go` design as is. Placing the API under `/api/` takes no name
from the SPA's route namespace, and the existing mechanism already returns JSON 404s for undefined
paths.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Top-level `/v1/` | Rejected: takes a name from the SPA routes (everything outside `/api/` is SPA), and needs separate SPA fallback handling and a separate JSON 404. |
| Accept both cookie and Bearer on the same paths | Rejected: requirement 5 forbids cookie use, and the screen API's behaviour would change. |

## R-4: Bearer requests skip the same-origin check

**Decision**: The same-origin check in `mutationBoundary` (`acceptsSameOrigin`) applies only to
cookie-class requests; Bearer-class requests are accepted whatever their `Origin`. The rule that
requires a JSON body applies to the external API too.

**Rationale**: The same-origin check exists to stop CSRF that rides on cookies the browser sends
automatically. Browsers never attach `Authorization` automatically, so that attack does not apply.
Scrapers and MCP clients either send no `Origin` or send an origin different from vv's.

**Alternatives considered**: Apply the check unchanged. Rejected: clients that send `Origin` (such as
browser-based tools) would be refused for no reason, and there is nothing to protect.

## R-5: The external API contract is a separate OpenAPI document, generating Go only

**Decision**: `api/external-v1.yaml` is the source of truth for the external API, and
`api/oapi-codegen-external.yaml` generates Go types and handler interfaces into
`internal/httpapi/extgen/`. No TypeScript is generated. `scripts/generate` generates it as well, and
`task generate-check` checks for drift. The document itself is the publication, linked from
`docs/how-to/external-api.md`; the server does not serve it. Compatibility policy: within `v1`, only
fields and operations are added; changing the meaning, type or required status of an existing field
means adding `v2`.

**Rationale**: The screen API changes to suit the screen (the Issue's background), while the external API
promises not to change. Mixing them in one document makes it impossible to tell from the document
whether a change breaks the external promise. With a separate document, the diff of
`api/external-v1.yaml` is the change to the promise. The screen does not call the external API, so it
needs no TypeScript types.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Add to `api/openapi.yaml` and separate by tag | Rejected: the reason above, and external types would also mix into the screen's generated TypeScript. |
| Serve it from the server at `GET /api/v1/openapi.yaml` | Rejected: needs an exception that embeds `api/` in the binary (the same kind as `web/embed.go`), and publishing it in the repository already satisfies the requirement. |

## R-6: The video list is read with an `(added_at, id)` keyset cursor

**Decision**: `GET /api/v1/videos` orders videos that have a location under a registered folder by
`(added_at, id)` ascending, and pages through them with the same keyset cursor as the screen's list
(an opaque string encoding the sort value and id, built like `encodeCursor` and `decodeCursor` in
`internal/store/listing.go`). The response is `{ items, nextCursor }`, and `nextCursor` is empty when
there is nothing more.

- There is no change tracking. There is no endpoint for "only new videos since last time", and none
  that reports changed or removed videos. A user who wants new videos reads the list again, and
  learns that a video they hold is gone from a `404` on `lookup`. No table or column is added.
- If videos are added, removed or moved during paging, the next request does not fail and returns
  the continuation as of that moment. There is no promise against missed or duplicated items across
  a full read (Edge Cases).
- The list returns only videos with a location under a registered folder. A video left only with
  locations outside the registered folders is omitted, the same as a video whose row is gone.

**Rationale**: The parent Issue asks for the loop "find new videos → tag them" to work, not for
fetching differences (requirement 6.1, acceptance criterion 3). Reading the list again is enough, so
this avoids change-tracking columns or counters and the rule that every scan and media-folder write
must advance a number. Keyset is the method the screen's `ListVideos` already uses, so the read code
and test shapes carry over. `(added_at, id)` does not change when a row moves, so the next request
resumes from the same place.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A cursor on monotonically increasing numbers (`added_seq`, `changed_seq`) that returns only videos added or changed since last time | Rejected: needs a rule to advance the number at every point that creates a video row, adds, removes or reassigns a location, adds, replaces or removes a media folder, or analyses the duration, plus tests for missed and duplicated items. The requester judged that re-reading the list is enough, so it was dropped from this feature. |
| `offset` paging | Rejected: rows added or removed by a scan cause missed and duplicated items at that instant; this is why the screen's list chose keyset (`listing.go`). |
| Order by `updated_at` | Rejected: it changes each time an ingest stage advances, so the order shifts during a full read. |

## R-7: Tag operations are added to `TagStore` as a strict batch operation

**Decision**: Add `TagStore.ApplyVideoTags(ctx, videos []domain.VideoRef, action, names)`. In one
transaction it resolves each video reference (an id, a content key or a location path) to a video now
in the library; if any one cannot be resolved, the whole call fails with a video-not-found error (a
value that wraps `domain.ErrNotFound` and carries the position of the reference).

| `action` | Behaviour |
| --- | --- |
| `add` | Looks up each name including synonyms and creates the tag if missing (`findOrCreateTag`, as the screen's attach does). |
| `remove` | Looks up each name including synonyms and detaches only tags that exist. A name that matches no tag does nothing. |
| `replace` | Looks up or creates each name as `add` does, then makes each video's manually attached tags (`video_tags` rows) exactly that set. An empty set detaches every manually attached tag. |

- All three rewrite only manually attached tags; tags derived from ancestor folder names
  ([017 data-model.md §4](../017-folder-groups/data-model.md#4-folder-derived-tags)) do not change. This
  is the same rule as the screen's detach (`DetachTag`). Tags in the response carry their source
  (`manual`, `fromFolder`, `domain.VideoTag`), so the user can tell which tags remained because they
  come from a folder.
- Names that resolve to the same tag are merged into one. The result is each video's tags after the
  operation.
- Names are validated with `domain.NormalizeTagName`; the video count limit is the screen's
  `maxVideoTagsIDs` (20000). The name count limit is 100.

It does not go through `internal/app`.

**Rationale**: Requirements 6.4 and 10. The existing attach (`AttachTagByName`) fits in one
transaction and therefore does not go through `internal/app` (see the comment on `httpapi.Tags`); the
same reason places this in the store. The screen's attach skips vanished videos, but the external API
has an Edge Case that requires the whole request to fail, so the resolution rule is separate.
Repeating the same add or remove succeeds without changing state (acceptance criterion 5). Add and
remove insert and delete per `(content_key, tag_id)` row, so tagging concurrently with the screen
keeps both.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Have `replace` remove folder-derived tags too | Rejected: those tags are derived from folder names every time, so cancelling them per video needs a new mechanism (an exclusion table). The parent Issue does not ask for it, and the screen does not have it. |
| One tag per request, like the screen's `POST /api/video-tags` | Rejected: scrapers attach several tags to one video, so requests multiply and the tags split across transactions. |
| A new use case in `internal/app` | Rejected: there are no side effects or events and it closes within one transaction; it would no longer match where the current tag operations live. |

## R-8: MCP uses the official Go SDK, stateless, inside `internal/httpapi`

**Decision**: Use `mcp.NewStreamableHTTPHandler` from `github.com/modelcontextprotocol/go-sdk` (as of
v1.8.0) with `Stateless: true` and `JSONResponse: true`, mounted at `/mcp`.

- The handler lives in `internal/httpapi`, inside the boundary (the Bearer class from R-3).
- Tools map one-to-one to external API operations, and their input and output are the same JSON as
  `api/external-v1.yaml`. The implementation calls the same functions as the REST handlers.

**Rationale**: Requirement 9. Stateless means the server keeps no per-client conversation state and
checks the Bearer token on every request, so no conversation outlives a revoked token. Tools are
request and response only; no server-initiated notifications are needed. Placing it in
`internal/httpapi` shares the video response conversion and the authentication boundary with REST,
and stays clear of the rule against imports between sibling packages (depguard).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| `github.com/mark3labs/mcp-go` | Rejected: widely used, but the official SDK promises v1 compatibility and is maintained. |
| Hand-written JSON-RPC | Rejected: we would carry version negotiation and the details of Streamable HTTP ourselves. |
| Stateful (session ID and a GET stream) | Rejected: the server keeps conversation state and has to manage its lifetime separately from token revocation. |
| A separate `internal/mcpapi` package | Rejected: duplicates the video response conversion and needs separate boundary wiring. |

## R-9: Last-used time is written at most once a minute; revocation also stops in-flight requests

**Decision**:

- On a request with a valid token, if the stored `last_used_at` is empty or at least 60 seconds old,
  it is rewritten on the spot (a conditional `update`). A write failure is logged and the request
  continues.
- Bearer requests are also registered in the boundary's `sessionLedger`. Revoking on the screen cuts
  off that token's in-flight responses immediately. Revocation by the host command cuts them off
  through the existing re-check every 30 seconds.

**Rationale**: Edge Cases (the last-used time may be thinned out; long-running requests are cut off
by revocation). A conditional `update` writes once however many times it is called per minute, so no
mechanism is needed to buffer in memory and write later. The existing mechanism that stops session
requests on logout and on the host command is reused as is.

**Alternatives considered**: Buffer in memory and write periodically. Rejected: lost on shutdown,
and adds a flushing goroutine and shutdown ordering.

## R-10: Token names follow the same rules as tag names

**Decision**: After trimming surrounding whitespace, a name is 1–100 characters (Unicode code points)
and contains no control characters. The rule lives in `domain.NormalizeAPITokenName`, and errors are
returned as the screen API's `invalid_request` with new `reason` values
(`api_token_name_empty`, `api_token_name_control_characters`, `api_token_name_too_long`, with the
limit in `limit`). Duplicate names are allowed.

**Rationale**: Edge Cases (an empty or too-long name is not issued; several tokens can share a name).
Using the screen's error design ([023 error-api.md](../023-english-i18n/contracts/error-api.md)) lets
the screen build its message from `reason`.

**Alternatives considered**: Reuse the tag-name `reason` values. Rejected: the screen's message would
say "tag name" and point the user at the wrong thing.
