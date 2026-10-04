# Implementation Plan: External API and MCP (automatic tagging with API tokens)

**Branch**: `feature/026-external-api` | **Parent Issue**: #493

**Input**: The parent Issue. It is this feature's specification.

## Summary

With an API token (Bearer) the owner issues on the settings page, an external tool can run "find new
videos → tag them" without a person in the loop. The same operations are also offered by an MCP
server at `/mcp`.

- **Tokens**: the `api_tokens` table holds only the SHA-256, and tokens become invalid through the
  `account.version` match and through deletion of every row when the account changes
  ([research.md R-1](research.md#r-1-tokens-are-prefixed-256-bit-random-values-and-only-the-sha-256-is-stored),
  [R-2](research.md#r-2-changing-the-account-deletes-the-token-rows-and-the-version-is-checked-as-well)).
  They are managed through the screen API, by the cookie owner only
  ([contracts/token-api.md](contracts/token-api.md)).
- **Boundary**: `/api/v1/` and `/mcp` become a Bearer-only class, and every other `/api/*` path stays
  cookie-only as today
  ([R-3](research.md#r-3-the-external-api-lives-under-apiv1-and-the-boundary-gains-a-bearer-class),
  [R-4](research.md#r-4-bearer-requests-skip-the-same-origin-check)).
- **External API v1**: a separate OpenAPI document, `api/external-v1.yaml`, is the source of truth
  ([R-5](research.md#r-5-the-external-api-contract-is-a-separate-openapi-document-generating-go-only)).
  Clients read the video list with a keyset cursor in order of addition (no change tracking;
  [R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)) and attach or
  detach tags in bulk by name
  ([R-7](research.md#r-7-tag-operations-are-added-to-tagstore-as-a-strict-batch-operation),
  [contracts/external-api.md](contracts/external-api.md)).
- **MCP**: the official Go SDK, stateless, with tools that call the same functions as REST
  ([R-8](research.md#r-8-mcp-uses-the-official-go-sdk-stateless-inside-internalhttpapi),
  [contracts/mcp.md](contracts/mcp.md)).
- **Screen**: an "API tokens" section on the settings page. The Issue has the `ui` label, so the next
  stage, design, decides the look and interaction in `ui-design.md`.

## Technical Context

**Canonical definitions**:

| Area | Source |
| --- | --- |
| Boundaries, dependency direction, the authentication boundary, data classes | [ARCHITECTURE.md](../../ARCHITECTURE.md), [.golangci.yml](../../.golangci.yml) (depguard) |
| Current authentication design | [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md), [contracts/auth-api.md](../016-single-account-auth/contracts/auth-api.md), [contracts/account-cli.md](../016-single-account-auth/contracts/account-cli.md), [internal/httpapi/auth.go](../../internal/httpapi/auth.go), [internal/app/auth.go](../../internal/app/auth.go), [internal/store/auth.go](../../internal/store/auth.go) |
| Screen API and error shape | [api/openapi.yaml](../../api/openapi.yaml), [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md), [internal/httpapi/router.go](../../internal/httpapi/router.go) (`mutationBoundary`, `requiresJSONBody`) |
| Tags | [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md), [internal/store/video_tags.go](../../internal/store/video_tags.go), [internal/domain/tag.go](../../internal/domain/tag.go) |
| Video list and location writes | [internal/store/listing.go](../../internal/store/listing.go), [internal/store/scan_index.go](../../internal/store/scan_index.go), [internal/store/ingest_results.go](../../internal/store/ingest_results.go) |
| Scans | [internal/app/scans.go](../../internal/app/scans.go) |
| Generation and check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`), [scripts/generate/main.go](../../scripts/generate/main.go), [api/oapi-codegen.yaml](../../api/oapi-codegen.yaml) |
| Screen text | [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md) |

**Feature-specific context**:

- One Go dependency is added: `github.com/modelcontextprotocol/go-sdk` (R-8). No npm dependency is
  added.
- One migration (`api_tokens`). Its number is the next one in `internal/store/migrations` at
  implementation time (the last one today is `00019_scan_issues.sql`). The definition is in
  [data-model.md](data-model.md). No column is added to `videos`.
- One more generated tree: `internal/httpapi/extgen/` (never hand-edited; added to the list in
  AGENTS.md).

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"): pass.

  | Package | What this feature puts there |
  | --- | --- |
  | `internal/domain` | Token name rules, `APIToken`, `VideoRef` and the list query values. Pure values and functions only. |
  | `internal/store` | Reading and writing `api_tokens`, the external API's video list and lookup, `TagStore.ApplyVideoTags`. |
  | `internal/app` | `Auth` gains issuing, listing, revoking and verifying tokens (randomness built the same way as sessions). |
  | `internal/httpapi` | Boundary classification, token management in the screen API, the external API, MCP. Request parsing and conversion only; the rules live in domain, store and app. |
  | `cmd/mdm` | Wiring and the `mdm account` output only. |

  No new imports between sibling packages (MCP is not a separate package, R-8).
- **API source of truth and generated code** (ARCHITECTURE.md, AGENTS.md): pass. The screen API's
  source is `api/openapi.yaml` and the external API's is `api/external-v1.yaml`; both are generated
  with `task generate`, and `generate-check` checks the diff (R-5). Having two sources rewrites
  ARCHITECTURE.md's "single source of truth" as one source for each of the two boundaries, screen and
  external.
- **Authentication boundary** (the authentication paragraph in ARCHITECTURE.md): pass. Classification
  is still decided by path alone, and a test of the same kind as `openapi_routes_test.go` checks that
  every external API operation is `bearerAuth` and that the boundary classifies it as Bearer. The
  screen API's classification and behaviour do not change (requirement 5).
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): pass. `api_tokens` is
  added to the list as settings data (data-model.md, [`api_tokens` (R-1, R-2, R-9, R-10)](data-model.md#api_tokens-r-1-r-2-r-9-r-10)). No index table changes.
- **Domain events**: not applicable. Token and tag operations emit no events (as with today's
  `AuthStore` and `TagStore`).
- **Server output is English** (gosmopolitan in `.golangci.yml`, 023): pass. `message`, logs and
  reason codes are English; screen text lives in the catalogue under `web/src/i18n/`.
- **Documents change in the same PR as the code** (core-beliefs.md, AGENTS.md): pass. Each unit
  updates its part of ARCHITECTURE.md and `docs/how-to/external-api.md`.

The verdicts are the same after Phase 1. There is no violation for Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/026-external-api/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-10
├── data-model.md         # api_tokens and the domain values
├── quickstart.md         # Steps to check the acceptance criteria on a running server
└── contracts/
    ├── token-api.md      # Token management in the screen API and the mdm account output
    ├── external-api.md   # /api/v1 operations
    └── mcp.md            # /mcp tools
```

The next stage, design, creates `ui-design.md` (`ui` label).

### Source Code

**Affected boundaries**:

- `internal/domain`, `internal/store` (migration, `AuthStore`, `TagStore`, reading the video list),
  `internal/app` (`Auth`)
- `internal/httpapi` (classification and the ledger in `auth.go`, `mutationBoundary` in `router.go`,
  the screen API, the external API, MCP)
- `api/openapi.yaml`, `scripts/generate`, `cmd/mdm` (wiring, output of `account.go`)
- `web/src/api`, `web/src/settings`, `web/src/i18n`
- `ARCHITECTURE.md`, `AGENTS.md` (list of generated files), `docs/how-to/`

**New paths**:

- `api/external-v1.yaml`, `api/oapi-codegen-external.yaml`, `internal/httpapi/extgen/` (generated)
- `internal/httpapi/api_tokens.go`, `internal/httpapi/external_*.go`, `internal/httpapi/mcp.go`
- `internal/domain/api_token.go`, `internal/store/api_tokens.go`, `internal/store/external_videos.go`
- `web/src/settings/APITokensSection.tsx` (the name follows the design stage)
- `docs/how-to/external-api.md` (how to use tokens, the compatibility policy, an integration example
  from a scraper, an MCP connection example; linked from `docs/how-to/README.md`)

**Structure decision**: the external API and MCP live in the same `internal/httpapi` as the screen
API, with a separate generated package and a separate handler type (`externalServer`). This keeps the
generated `ServerInterface` names from colliding with the screen API's, while the boundary and the
response conversion are shared (see also the rejected alternatives in R-8).

## Implementation Work

### Issue, list and revoke API tokens (screen API and storage)

**Scope**: the `api_tokens` migration and the `AuthStore` and `app.Auth` operations
([data-model.md, `api_tokens` (R-1, R-2, R-9, R-10)](data-model.md#api_tokens-r-1-r-2-r-9-r-10)), the name rules (R-10), the three
screen API operations and the `mdm account` output ([contracts/token-api.md](contracts/token-api.md)),
`api/openapi.yaml` and the generated code, and the data-class and `AuthStore` paragraphs of
ARCHITECTURE.md. Bearer verification is provided as an `app.Auth` operation but not yet connected to
the boundary.

**Dependencies**: None

**Acceptance**: `task check` passes. `POST /api/api-tokens` as the cookie owner returns `201` with a
`secret`, and `GET /api/api-tokens` contains neither the `secret` nor the hash. Without the cookie,
all three return `401`. An empty name and a 101-character name return `400` with their `reason`.
After `mdm account set-password`, the list is empty.

### "API tokens" section on the settings page

**Scope**: add a section to the settings page that issues tokens, shows the plaintext once with a
copy action, lists tokens, and revokes with confirmation. Text in the English catalogue
(`web/src/i18n/en.ts`) and text for the new `reason` values (`web/src/i18n/errors.ts`). The look and
interaction follow `ui-design.md`.

**Dependencies**: Issue, list and revoke API tokens (screen API and storage)

**Acceptance**: the screen changes (visual and interaction review required). Vitest tests pass; the
plaintext issued on the settings page appears exactly once, and after a reload only the name and
dates remain (acceptance criterion 1). After confirmation, a revoked token disappears from the list.

### Bearer authentication and tag and scan operations in external API v1

**Scope**: the base of `api/external-v1.yaml` (shared errors, `bearerAuth`), the generation settings,
`scripts/generate` and the list of generated files in AGENTS.md (R-5). The boundary's Bearer class,
exemption from the same-origin check, last-used time and cut-off through the ledger (R-3, R-4, R-9).
Operations: `GET /api/v1/tags` and the two scan operations
([contracts/external-api.md, Tags](contracts/external-api.md#tags) and [Scans](contracts/external-api.md#scans)). `app.Scans.StartScan`
returns `started` (the `Scans` interface in `internal/httpapi` and the screen's `POST /api/scans`
change to match; the screen's response does not change). Create `docs/how-to/external-api.md` and
describe how to use tokens and the compatibility policy. The authentication paragraph of
ARCHITECTURE.md.

**Dependencies**: Issue, list and revoke API tokens (screen API and storage)

**Acceptance**: `task check` passes.

- With a valid token, `GET /api/v1/tags` returns `200`; with an invalid, revoked or malformed token,
  or a cookie alone, it returns `401` with `WWW-Authenticate: Bearer`.
- With a Bearer token alone, `GET /api/api-tokens` returns `401` and `GET /api/videos` returns
  `X-VV-Audience: guest`.
- `POST /api/v1/scans` with a different `Origin` succeeds.
- In app tests, `StartScan` returns `started` true the first time and false when called again while
  running; in httpapi tests, `POST /api/v1/scans` returns `201` the first time and `200` while
  running, and the screen's `POST /api/scans` still returns `202`.
- `GET /api/v1/scans/current` returns `null` for both `videos` and `settledVideos` right after start
  (`finding`), and `0` for both for a scan that finished with no videos.
- An httpapi test confirms that revoking on the screen cuts off an in-flight Bearer response.
- A test confirms that every external API operation is `bearerAuth` and classified as Bearer.

### Read the video list and look up one video through the external API

**Scope**: `GET /api/v1/videos` and `GET /api/v1/videos/lookup`
([contracts/external-api.md, Videos](contracts/external-api.md#videos),
[R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)). The list is a read
that returns videos with a location under a registered folder in `(added_at, id)` ascending order with
a keyset cursor (`internal/store/external_videos.go`; no table or column is added). The part of
`docs/how-to/external-api.md` on reading the list (follow until `nextCursor` is empty; read the list
again to find new videos; learn that a held video is gone from a `404` on `lookup`).

**Dependencies**: Bearer authentication and tag and scan operations in external API v1

**Acceptance**: `task check` passes.

- In store tests, with more videos than `limit`, following `nextCursor` returns every video without
  duplicates in `(added_at, id)` order, the last response's `nextCursor` is empty, and an unparsable
  cursor yields `domain.ErrInvalidCursor`.
- After reading the list to the end, a scan adds a video, and reading again from the start includes
  that video (acceptance criterion 3).
- Removing a video in the middle of paging does not make the next request fail.
- A video that lost every location under the registered folders and remains only with locations
  outside them is left out of the list, the same as a video whose row is gone, and `lookup` returns
  `404 video_not_found`.
- Private videos are returned too (acceptance criterion 2).
- `lookup` returns the same video by id, by content key and by path (including a path spelled in
  NFD), and a missing reference returns `404 video_not_found`.

### Attach, detach and replace tags by name through the external API

**Scope**: `TagStore.ApplyVideoTags` (R-7) and `POST /api/v1/video-tags`
([contracts/external-api.md, Video tags](contracts/external-api.md#video-tags)), and the integration example
from a scraper in `docs/how-to/external-api.md` (read new videos → look them up → tag them).

**Dependencies**: Read the video list and look up one video through the external API

**Acceptance**: `task check` passes.

- `add` on a video referenced by path, with a tag name that does not exist and a synonym name,
  creates the tag, attaches the synonym as its original tag, and both appear in the screen API's video
  details and in the `tag` filter (acceptance criterion 4).
- Repeating the same request returns `200` and does not change state (acceptance criterion 5).
- A request containing one video that cannot be resolved returns `404` with `index` and applies
  nothing.
- Name errors and count limits return `400` with their `reason`.
- After `replace`, the video's manually attached tags (`manual`) are exactly the given set, and
  folder-derived tags (`fromFolder`) remain.

### Offer the external operations as MCP tools at `/mcp`

**Scope**: adding the Go SDK and the `/mcp` handler (R-8), six tools
([contracts/mcp.md](contracts/mcp.md)), the MCP connection example in `docs/how-to/external-api.md`,
and the MCP paragraph of ARCHITECTURE.md.

**Dependencies**: Attach, detach and replace tags by name through the external API

**Acceptance**: `task check` passes. In an httpapi test, an SDK client connects to `/mcp`, the six
tools are listed, and the result of `update_video_tags` shows in REST `GET /api/v1/videos/lookup`.
Without a token or with an invalid token, the response is `401`. The steps in
[quickstart.md](quickstart.md) are run with Claude Code and the result is recorded in the PR body
(acceptance criterion 9 and others).
