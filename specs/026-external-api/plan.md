# Implementation Plan: External API and MCP (automatic tagging with API tokens)

**Branch**: `feature/026-external-api` | **Parent Issue**: #493

**Input**: The parent Issue. It is this feature's specification.

## Summary

With an API token (Bearer) that the owner issues on the settings page, an external tool runs "find
new videos, then tag them" without manual steps. The `/mcp` MCP server offers the same operations.

| Area | Plan |
| --- | --- |
| Tokens | The `api_tokens` table stores only the SHA-256. A token is revoked by a mismatched `account.version` and by deleting all rows on an account change ([research.md R-1](research.md#r-1-a-token-is-a-prefixed-256-bit-random-value-only-its-sha-256-is-stored), [R-2](research.md#r-2-an-account-change-deletes-the-token-rows-a-version-match-is-checked-too)). Only the Cookie owner manages tokens, through the UI API ([contracts/token-api.md](contracts/token-api.md)) |
| Boundary | `/api/v1/` and `/mcp` are classified as Bearer only. Every other `/api/*` stays Cookie only ([R-3](research.md#r-3-the-external-api-lives-under-apiv1-the-boundary-gets-a-bearer-class), [R-4](research.md#r-4-bearer-requests-skip-the-same-origin-check)) |
| External API v1 | A separate OpenAPI document, `api/external-v1.yaml`, is the source of truth ([R-5](research.md#r-5-the-external-api-contract-is-a-separate-openapi-document-only-go-is-generated)). The video list is read with a keyset cursor in insertion order, without change tracking ([R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)). Tags are attached and detached in bulk by name ([R-7](research.md#r-7-tag-operations-are-a-strict-bulk-operation-added-to-tagstore), [contracts/external-api.md](contracts/external-api.md)) |
| MCP | The official Go SDK in stateless mode; the tools call the same functions as REST ([R-8](research.md#r-8-mcp-uses-the-official-go-sdk-in-stateless-mode-inside-internalhttpapi), [contracts/mcp.md](contracts/mcp.md)) |
| UI | An "API tokens" section on the settings page. The Issue has the `ui` label, so the next stage, design, fixes the look and interaction in `ui-design.md` |

## Technical Context

**Canonical definitions**:

| Topic | Location |
| --- | --- |
| Boundaries, dependency direction, authentication boundary, data classes | [ARCHITECTURE.md](../../ARCHITECTURE.md), [.golangci.yml](../../.golangci.yml) (depguard) |
| Current authentication | [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md), [contracts/auth-api.md](../016-single-account-auth/contracts/auth-api.md), [contracts/account-cli.md](../016-single-account-auth/contracts/account-cli.md), [internal/httpapi/auth.go](../../internal/httpapi/auth.go), [internal/app/auth.go](../../internal/app/auth.go), [internal/store/auth.go](../../internal/store/auth.go) |
| UI API and error shape | [api/openapi.yaml](../../api/openapi.yaml), [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md), [internal/httpapi/router.go](../../internal/httpapi/router.go) (`mutationBoundary`, `requiresJSONBody`) |
| Tags | [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md), [internal/store/video_tags.go](../../internal/store/video_tags.go), [internal/domain/tag.go](../../internal/domain/tag.go) |
| Video list and location writes | [internal/store/listing.go](../../internal/store/listing.go), [internal/store/scan_index.go](../../internal/store/scan_index.go), [internal/store/ingest_results.go](../../internal/store/ingest_results.go) |
| Scans | [internal/app/scans.go](../../internal/app/scans.go) |
| Generation and check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`), [scripts/generate/main.go](../../scripts/generate/main.go), [api/oapi-codegen.yaml](../../api/oapi-codegen.yaml) |
| UI strings | [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md) |

**Feature-specific context**:

- One Go dependency is added: `github.com/modelcontextprotocol/go-sdk` (R-8). No npm dependency is
  added.
- One migration (`api_tokens`). It takes the next number in `internal/store/migrations` at
  implementation time (the current last one is `00019_scan_issues.sql`). The definition is in
  [data-model.md](data-model.md). `videos` gets no new column.
- One more generated output: `internal/httpapi/extgen/`. Do not hand-edit it; add it to the list in
  AGENTS.md.

## Constitution Check

- **Dependency direction** (ARCHITECTURE.md "Intended dependency direction"): pass.

  | Package | Adds |
  | --- | --- |
  | `internal/domain` | Token name rule, `APIToken`, `VideoRef` and the list query values. Pure values and functions only |
  | `internal/store` | Reads and writes of `api_tokens`, the external API video list and lookup, `TagStore.ApplyVideoTags` |
  | `internal/app` | Token issue, list, revoke and verify on `Auth` (random values built the same way as sessions) |
  | `internal/httpapi` | Boundary classification, token management in the UI API, the external API, MCP. Only request parsing and conversion; rules live in domain, store and app |
  | `cmd/mdm` | Wiring and the `mdm account` output only. No new imports between sibling packages (MCP is not a separate package, R-8) |

- **API sources of truth and generated output** (ARCHITECTURE.md, AGENTS.md): pass. The UI API uses
  `api/openapi.yaml` and the external API uses `api/external-v1.yaml`. `task generate` generates
  both, and `generate-check` checks the diff (R-5). The "single source of truth" wording in
  ARCHITECTURE.md is rewritten as one source for each of the two boundaries, UI and external.
- **Authentication boundary** (the authentication paragraph of ARCHITECTURE.md): pass.
  Classification still depends on the path only. A test of the same kind as
  `openapi_routes_test.go` confirms that every external API operation has `bearerAuth` and that the
  boundary classifies it as Bearer. The classification and behavior of the UI API do not change
  (Requirement 5).
- **Index versus user data** (ARCHITECTURE.md "Rebuildable and user data"): pass. `api_tokens` is
  added to the list as configuration data (data-model.md §1). Index tables do not change.
- **Domain events**: not applicable. Token and tag operations emit no events, like the current
  `AuthStore` and `TagStore`.
- **Server output is English** (gosmopolitan in `.golangci.yml`, 023): pass. `message`, logs and
  reason codes are English. The catalog in `web/src/i18n/` owns the UI strings.
- **Docs change in the same PR** (core-beliefs.md, AGENTS.md): pass. Each unit updates its own part
  of ARCHITECTURE.md and `docs/how-to/external-api.md`.

The result is the same after Phase 1. No violation goes into Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/026-external-api/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-10
├── data-model.md         # api_tokens, domain values
├── quickstart.md         # Steps to verify the acceptance criteria on a running server
└── contracts/
    ├── token-api.md      # Token management in the UI API and the mdm account output
    ├── external-api.md   # /api/v1 operations
    └── mcp.md            # /mcp tools
```

The next stage, design, creates `ui-design.md` (`ui` label).

### Source Code

**Affected boundaries**:

- `internal/domain`, `internal/store` (migration, `AuthStore`, `TagStore`, video list reads),
  `internal/app` (`Auth`)
- `internal/httpapi` (classification and ledger in `auth.go`, `mutationBoundary` in `router.go`, the
  UI API, the external API, MCP)
- `api/openapi.yaml`, `scripts/generate`, `cmd/mdm` (wiring, output in `account.go`)
- `web/src/api`, `web/src/settings`, `web/src/i18n`
- `ARCHITECTURE.md`, `AGENTS.md` (list of generated outputs), `docs/how-to/`

**New paths**:

- `api/external-v1.yaml`, `api/oapi-codegen-external.yaml`, `internal/httpapi/extgen/` (generated)
- `internal/httpapi/api_tokens.go`, `internal/httpapi/external_*.go`, `internal/httpapi/mcp.go`
- `internal/domain/api_token.go`, `internal/store/api_tokens.go`, `internal/store/external_videos.go`
- `web/src/settings/APITokensSection.tsx` (the name follows the design stage)
- `docs/how-to/external-api.md` (token usage, compatibility policy, an integration example from a
  scraper, an MCP connection example; linked from `docs/how-to/README.md`)

**Structure decision**: The external API and MCP live in `internal/httpapi`, like the UI API, with a
separate generated package and a separate handler type (`externalServer`). This keeps the generated
`ServerInterface` name from colliding with the UI API. The boundary and the response conversion are
shared (see also the rejected alternative in R-8).

## Implementation Work

### Issue, list and revoke API tokens (UI API and storage)

**Scope**: The `api_tokens` migration and the `AuthStore` and `app.Auth` operations
([data-model.md §1](data-model.md#1-api_tokens-r-1-r-2-r-9-r-10)), the name rule (R-10), the three UI
API operations and the `mdm account` output ([contracts/token-api.md](contracts/token-api.md)),
`api/openapi.yaml` and the generated output, and the data class and `AuthStore` paragraph of
ARCHITECTURE.md. Bearer verification exists as an `app.Auth` operation but is not yet wired to the
boundary.

**Dependencies**: None

**Acceptance**:

- `task check` passes.
- `POST /api/api-tokens` as the Cookie owner returns `201` with `secret`. `GET /api/api-tokens` has
  neither `secret` nor the hash.
- Without a Cookie, all three return `401`.
- An empty or 101-character name returns `400` with its `reason`.
- After `mdm account set-password`, the list is empty.

### "API tokens" section on the settings page

**Scope**: Add a section to the settings page for issuing a token, showing and copying the plaintext
once, listing tokens and revoking with confirmation. Strings go in the English catalog
(`web/src/i18n/en.ts`), and the new `reason` strings in `web/src/i18n/errors.ts`. The look and
interaction follow `ui-design.md`.

**Dependencies**: Issue, list and revoke API tokens (UI API and storage)

**Acceptance**:

- The change touches the UI, so it needs visual and interaction review.
- Vitest tests pass.
- A plaintext issued on the settings page is shown once. After a reload, only the name and the
  times remain (Acceptance criterion 1).
- A revoke removes the row from the list after confirmation.

### Bearer authentication and the tag and scan operations of External API v1

**Scope**:

- The base of `api/external-v1.yaml` (common errors, `bearerAuth`), the generation config,
  `scripts/generate` and the list of generated outputs in AGENTS.md (R-5).
- Bearer classification at the boundary, exemption from the same-origin check, last use time and
  cut-off through the ledger (R-3, R-4, R-9).
- The operations `GET /api/v1/tags` and the two scan operations
  ([contracts/external-api.md §3 and §5](contracts/external-api.md#3-tags)).
- `app.Scans.StartScan` returns `started`. The `Scans` interface in `internal/httpapi` and the UI's
  `POST /api/scans` change with it; the UI response does not change.
- Create `docs/how-to/external-api.md` with token usage and the compatibility policy. Update the
  authentication paragraph of ARCHITECTURE.md.

**Dependencies**: Issue, list and revoke API tokens (UI API and storage)

**Acceptance**:

- `task check` passes.
- With a valid token, `GET /api/v1/tags` returns `200`. An invalid, revoked or malformed token, or a
  Cookie alone, returns `401` with `WWW-Authenticate: Bearer`.
- With Bearer only, `GET /api/api-tokens` returns `401` and `GET /api/videos` returns
  `X-VV-Audience: guest`.
- `POST /api/v1/scans` with a different `Origin` succeeds.
- An app test: `started` from `StartScan` is true the first time and false when called again during
  a running scan. An httpapi test: `POST /api/v1/scans` returns `201` first and `200` during a
  running scan, and the UI's `POST /api/scans` still returns `202`.
- `GET /api/v1/scans/current` returns `null` for both `videos` and `settledVideos` right after the
  start (`finding`), and `0` for both for a scan that finished with 0 targets.
- An httpapi test confirms that a revoke in the UI cuts off a running Bearer response.
- A test confirms that every external API operation has `bearerAuth` and is classified as Bearer.

### Read the video list and look up one video through the external API

**Scope**:

- `GET /api/v1/videos` and `GET /api/v1/videos/lookup`
  ([contracts/external-api.md §2](contracts/external-api.md#2-videos),
  [R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)).
- The list is a read that returns videos with a location under a media folder, in ascending
  `(added_at, id)`, with a keyset cursor (`internal/store/external_videos.go`; no table or column is
  added).
- How to read the list in `docs/how-to/external-api.md`: follow until `nextCursor` is empty; read the
  list again to find new videos; learn that a held video is gone from the `404` of `lookup`.

**Dependencies**: Bearer authentication and the tag and scan operations of External API v1

**Acceptance**:

- `task check` passes.
- A store test: with more videos than `limit`, following `nextCursor` returns every video without
  duplicates in `(added_at, id)` order. The last response has an empty `nextCursor`. An unparsable
  cursor returns `domain.ErrInvalidCursor`.
- After the list is read to the end, a scan adds a video. Reading again from the start includes it
  (Acceptance criterion 3).
- Deleting a video during paging does not make the next request fail.
- A video that lost every location under the media folders and remains only with locations outside
  them is not returned by the list, like a deleted row, and `lookup` returns `404 video_not_found`.
- Private videos are returned too (Acceptance criterion 2).
- `lookup` returns the same video by id, content key and path (including a path spelled in NFD). An
  unknown reference returns `404 video_not_found`.

### Attach, detach and replace tags by name through the external API

**Scope**: `TagStore.ApplyVideoTags` (R-7) and `POST /api/v1/video-tags`
([contracts/external-api.md §4](contracts/external-api.md#4-video-tags)), plus the scraper
integration example in `docs/how-to/external-api.md` (read new videos, look one up, tag it).

**Dependencies**: Read the video list and look up one video through the external API

**Acceptance**:

- `task check` passes.
- `add` on a video referenced by path, with a new tag name and a synonym name, creates the tag and
  attaches the synonym as its original tag. Both appear in the UI API's video detail and in the `tag`
  filter (Acceptance criterion 4).
- Repeating the same request returns `200` and does not change the state (Acceptance criterion 5).
- A request with one video that cannot be found returns `404` with `index` and applies nothing.
- Name errors and count limits return `400` with their `reason`.
- After `replace`, the video's manually attached tags (`manual`) are exactly the given set, and
  folder-derived tags (`fromFolder`) remain.

### Serve the external operations as MCP tools on `/mcp`

**Scope**: The Go SDK and the `/mcp` handler (R-8), the six tools ([contracts/mcp.md](contracts/mcp.md)),
the MCP connection example in `docs/how-to/external-api.md` and the MCP paragraph of
ARCHITECTURE.md.

**Dependencies**: Attach, detach and replace tags by name through the external API

**Acceptance**:

- `task check` passes.
- An httpapi test: an SDK client connected to `/mcp` lists the six tools, and the result of
  `update_video_tags` appears in REST `GET /api/v1/videos/lookup`.
- Without a token or with an invalid token, the response is `401`.
- Run the [quickstart.md](quickstart.md) steps with Claude Code and record the result in the PR
  body (Acceptance criterion 9 and others).
