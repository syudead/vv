# Implementation Plan: an English-only service with an i18n foundation

**Branch**: `feature/023-english-i18n` | **Parent Issue**: #408

**Input**: The parent Issue. It is this feature's specification.

## Summary

Hard-coded Japanese text spread across the UI, the API, the logs and the stored failure reasons
becomes English, and an i18n foundation handles UI text and formatting in one place. English is the
only language offered; there is no language selection.

- UI text is collected in a typed catalog in `web/src/i18n/`, and components read from it.
  Interpolation uses function arguments, plurals use `Intl.PluralRules`, and dates, numbers and
  relative times use `Intl` with the same locale
  ([research.md R-1](research.md#r-1-ui-text-lives-in-a-typed-in-house-catalog),
  [R-2](research.md#r-2-formatting-and-plurals-use-the-browsers-intl)). `index.html` has `lang="en"`,
  and the video.js custom language is built from the catalog too
  ([R-9](research.md#r-9-videojs-text-is-built-from-the-catalog)).
- The UI does not use the API `message` for normal display. It builds English text from `code` and
  the new `reason`, `limit` and `tagName`. Existing `code` values and HTTP statuses stay; `reason`
  tells situations apart within one `code`
  ([contracts/error-api.md §1](contracts/error-api.md#1-reason-limit-and-tagname-on-error),
  [R-4](research.md#r-4-api-errors-gain-specificity-through-reason-without-changing-codes),
  [R-5](research.md#r-5-the-ui-shows-known-errors-from-reason-and-code-and-everything-else-as-a-safe-summary)).
- Probe and scan failures store a reason code in a separate column. The UI explains the failure
  from the code and does not show free text (ffprobe output, old Japanese). Stored free text is not
  rewritten ([data-model.md](data-model.md),
  [R-6](research.md#r-6-failure-reasons-are-stored-as-machine-readable-codes-the-ui-does-not-show-free-text)).
- Server `message`s, logs and `cmd/mdm` output are written in English directly in Go
  ([R-7](research.md#r-7-the-server-has-no-text-catalog-go-code-contains-english-directly)).
- Detection of untranslated text: on the UI, ESLint `no-restricted-syntax`, the branded type
  `UiText` for values that carry fixed text, and screen tests rendered in a pseudo-locale. For
  known errors, the catalog's `Record<ErrorCode | ErrorReason, …>` type. On the server,
  `gosmopolitan` ([R-3](research.md#r-3-lint-and-types-detect-untranslated-text)).
- Comments, OpenAPI `description`s, design documents and `scripts/` are out of scope
  ([R-8](research.md#r-8-what-stays-untranslated)).

## Technical Context

**Canonical definitions**:

- Boundaries, dependency direction and the Web layer's responsibilities:
  [ARCHITECTURE.md](../../ARCHITECTURE.md) ("Intended dependency direction", "Web layer")
- API source of truth and generation: [api/openapi.yaml](../../api/openapi.yaml) (`Error`,
  `Video.probeError`, `Scan.error`), `task generate`
- Writing error responses: [internal/httpapi/router.go](../../internal/httpapi/router.go)
  (`writeError`, `invalidRequest`, `notFound`, `internalError`)
- UI API calls and errors: [web/src/api/client.ts](../../web/src/api/client.ts)
  (`RequestFailed`, `toRequestFailed`, `errorMessage`)
- Formatting: [web/src/lib/format.ts](../../web/src/lib/format.ts); player text:
  [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx)
- Where failure reasons are written: `internal/store/jobs.go` (`recordTerminalFailure`),
  `internal/store/scans.go` (`FinishScan`, `FailInterruptedScans`), `internal/app/scans.go`
- Lint: [.golangci.yml](../../.golangci.yml), [web/eslint.config.js](../../web/eslint.config.js)
- UI appearance and text design: [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
  and each `ui-design.md` listed in the
  [design document index](../../docs/design-docs/index.md)
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`,
  `task test-e2e`)

**Feature-specific context**:

- No new dependencies. The UI uses `Intl`; lint uses ESLint's built-in rules and `gosmopolitan`,
  bundled with golangci-lint.
- Migration `00016` adds three columns to SQLite ([data-model.md](data-model.md)).
- API changes are additions only (`Error.reason`, `Error.limit`, `Error.tagName`,
  `Video.probeErrorCode`, `Scan.errorCode`, `Scan.errorPath`). Existing `code` values, HTTP
  statuses and state values do not change.
- For the Japanese text quoted in each `ui-design.md`, the English catalog becomes the source of
  truth.

## Constitution Check

| Gate | Result | Reason |
| --- | --- | --- |
| **Dependency direction** (ARCHITECTURE.md) | Pass | Failure codes and their wrapping type live in `internal/domain`. `internal/media` and `internal/scanner` wrap; `internal/app`, `internal/jobs` and `internal/store` extract with `errors.As`. No new adapter-to-adapter imports. |
| **`api/openapi.yaml` is the API source of truth** (ARCHITECTURE.md, AGENTS.md) | Pass | `reason` and the failure codes are added as enums, and `task generate` produces Go and TypeScript. Generated files are not hand-edited. |
| **Only `web/src/api/` talks to the server** (ARCHITECTURE.md "Web layer") | Pass | `client.ts` puts `reason`, `limit` and `tagName` on `RequestFailed`; `web/src/i18n/` builds the UI text. |
| **Index data versus user data** (ARCHITECTURE.md "Rebuildable and user data") | Pass | The new columns are in the rebuildable `videos` and `scans`; user names and tags are untouched (Requirement 8). |
| **Constraints are enforced by checks** (core-beliefs.md) | Pass | Untranslated text, hard-coded locales and missing error displays fail `task check` through lint and type checks (Acceptance criterion 8). |
| **Docs change in the same PR** (core-beliefs.md, AGENTS.md) | Pass | The foundation unit adds the i18n design document and its index link. The unit that changes the API, data or Web-layer description in ARCHITECTURE.md updates it. |

The result is the same after Phase 1. Complexity Tracking has no violations.

## Project Structure

### Documentation (this feature)

```text
specs/023-english-i18n/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # Decisions: catalog, formatting, detection, error reason, failure codes, out of scope
├── data-model.md         # videos.probe_error_code, scans.error_code and error_path
├── quickstart.md         # Checks: remaining Japanese, failure display, old Japanese data, formatting
└── contracts/
    └── error-api.md      # Error.reason, limit, tagName; Video.probeErrorCode; Scan.errorCode, errorPath
```

No `ui-design.md`. The parent Issue has no `ui` label, and the change replaces text without
changing screen structure, interaction or appearance. Places where English changes the text length
are handled in each unit's visual check.

### Source Code

**Affected boundaries**:

- `api/openapi.yaml`, `internal/httpapi`: `Error.reason`, `limit`, `tagName`; the failure-code
  fields; English `message`s.
- `internal/domain`: failure codes and the wrapping type; the reasoned error from
  `NormalizeTagName`; English error text.
- `internal/media`, `internal/scanner`, `internal/app`, `internal/jobs`, `internal/store`:
  attaching and storing failure codes; English error text and logs.
- `internal/artifacts`, `internal/mediafs`, `internal/opener`, `internal/password`,
  `internal/eventbus`, `cmd/mdm`: English error text, logs, and settings and account command
  output.
- `web/src/i18n` (new): catalog, formatting, error display. `web/src/api/client.ts`: `reason`,
  `limit`, `tagName`.
- Each area of `web/src/*` and `web/e2e`: using the catalog and asserting English.
  `web/index.html`: `lang="en"`.
- `.golangci.yml`, `web/eslint.config.js`: detection of untranslated text.
- `ARCHITECTURE.md`, `docs/design-docs/`: the i18n design document and its index link; the API,
  data and Web-layer descriptions.

**New paths**: `web/src/i18n/`, `internal/store/migrations/00016_failure_codes.sql`,
`docs/design-docs/i18n.md`.

**Structure decision**:

| | |
| --- | --- |
| **Decision** | UI text and formatting live in `web/src/i18n/`. The locale-dependent functions of `web/src/lib/format.ts` (date-time, relative time) move there. Locale-independent functions (duration, size, resolution) stay in `lib/`. |
| **Why** | As its own directory, `i18n/` can be the one place where the ESLint rule allows Japanese and letter literals, delimited by directory. |
| **Rejected** | `i18n/` inside `lib/`: the rule could not be scoped by directory. |

## Implementation Work

How the units are split: the server has three units, API (`internal/httpapi` and domain errors),
failure codes (the data flow), and English for the remaining packages. The UI has a foundation
unit and four areas whose files do not overlap. The foundation unit enables the UI lint and
excludes the directories not yet migrated; each area unit removes its own entries
([R-3](research.md#r-3-lint-and-types-detect-untranslated-text)).

### Add reasons and display values to API errors, and make API messages English

**Scope**:

- `api/openapi.yaml`: `Error.reason`, `limit`, `tagName` and the `message` description
  ([contracts/error-api.md §0 and §1](contracts/error-api.md)).
- `internal/httpapi`: every `message`, the logs and the plain-text responses in `spa.go` in
  English; `reason`, `limit` and `tagName` for the situations in the table.
- `internal/domain`: English error text and a reasoned error from `NormalizeTagName`.
- Go test expectations, and the API description in ARCHITECTURE.md.

**Dependencies**: None

**Acceptance**:

- `internal/httpapi` tests pass. For each situation in the table they assert the same HTTP status
  and `code` as before the change, plus the table's `reason`, `limit` and `tagName`.
- Temporarily enabling `gosmopolitan` on `internal/httpapi` and `internal/domain` reports 0
  findings.
- The `task generate` diff touches only generated files, and `task check` passes.

### Store reason codes for probe and scan failures and return them from the API

**Scope**:

- Migration `00016` and the three columns in [data-model.md](data-model.md).
- `internal/domain`: failure codes and the wrapping type.
- `internal/media` (probing) and `internal/scanner`: attaching codes, English error text and logs.
- `internal/app`, `internal/jobs`: passing codes through, English text.
- `internal/store`: writing and reading the codes.
- `Video.probeErrorCode`, `Scan.errorCode`, `Scan.errorPath`
  ([contracts/error-api.md §2 and §3](contracts/error-api.md)), and the data description in
  ARCHITECTURE.md.

**Dependencies**: Add reasons and display values to API errors, and make API messages English

**Acceptance**:

- Go tests assert that a probe failure on a non-video file makes `GET /api/videos/{id}` return
  `probeErrorCode: "probe_failed"` and an English `probeError`.
- Go tests assert that a scan of an unreadable media folder makes `GET /api/scans/current` return
  `errorCode: "media_folder_unreadable"` and the path in `errorPath`.
- Store tests assert that Japanese `probe_error` and `scans.error` values inserted before the
  migration keep the same values after it, with `null` codes.
- `task check` passes.

### Make the remaining server error text, logs and operations command output English, and enable gosmopolitan

**Scope**:

- `internal/store`, `internal/artifacts`, `internal/mediafs`, `internal/opener`,
  `internal/password`, `internal/eventbus`.
- `cmd/mdm`: settings validation, startup failures, account command usage and I/O, subscriber
  names.
- The remaining text and logs in `internal/media` (thumbnails, previews, seek thumbnails),
  `internal/jobs` and `internal/app` that the previous two units did not touch; test
  expectations.
- `.golangci.yml`: enable `gosmopolitan` for non-test code in `cmd/` and `internal/`
  ([R-3](research.md#r-3-lint-and-types-detect-untranslated-text)).

**Dependencies**: Store reason codes for probe and scan failures and return them from the API

**Acceptance**:

- `gosmopolitan` in `task lint` passes with 0 findings.
- The logs from the steps in [quickstart.md §2](quickstart.md#2-server-output-is-english)
  contain no hard-coded Japanese text.
- `task check` passes.

### Build the UI i18n foundation (text catalog, English formatting, API error display)

**Scope**:

- `web/src/i18n/`: the catalog type and the English catalog; plural and `Intl` formatting
  functions; the error display (`reason` → `code` → `message` for an unknown code → HTTP status
  summary; network failure)
  ([R-1](research.md#r-1-ui-text-lives-in-a-typed-in-house-catalog),
  [R-2](research.md#r-2-formatting-and-plurals-use-the-browsers-intl),
  [R-5](research.md#r-5-the-ui-shows-known-errors-from-reason-and-code-and-everything-else-as-a-safe-summary)).
- `reason`, `limit` and `tagName` on `RequestFailed` in `client.ts`; the `UiText` type and text
  props of the `ui/` components.
- The pseudo-locale, and a test helper that asserts all rendered text comes from the catalog or is
  user data; moving the locale-dependent functions out of `lib/format.ts`.
- Text in `web/src/api/`, `web/src/lib/`, `web/src/ui/`, `web/src/app/` and `main.tsx`;
  `lang="en"` in `index.html`.
- The ESLint rule and the exclusion list of directories not yet migrated
  ([R-3](research.md#r-3-lint-and-types-detect-untranslated-text)).
- `docs/design-docs/i18n.md` and its index link; an index note that the catalog is the source of
  truth for the text in each `ui-design.md`; the Web-layer description in ARCHITECTURE.md.

**Dependencies**: Store reason codes for probe and scan failures and return them from the API

**Acceptance**:

- Vitest passes and asserts: English text exists for every `ErrorCode`, `ErrorReason`,
  `ProbeErrorCode` and `ScanErrorCode` (type check); `limit` interpolation; the display of an
  unknown code, an empty body, a non-JSON body and a `fetch` failure; counts of 1 and many; English
  dates, date-times and relative times.
- Vitest also asserts that the pseudo-locale helper detects a component that renders an English
  literal through a `string` variable.
- ESLint asserts that `web/src/api/`, `lib/`, `ui/` and `app/` contain no Japanese or letter
  literals outside `i18n/`. `task check` passes.
- Shared components (combobox, toast) change their text, so visual and assistive technology checks
  are performed.

### Make initial setup, login, the top frame and Settings English

**Scope**:

- Move the text, accessible names and formatting of `web/src/auth/`, `web/src/shell/` and
  `web/src/settings/` into the catalog.
- Show scan failures from `Scan.errorCode` and `errorPath`; do not show `Scan.error`.
- Media folder, folder picker and login failures use the API error display.
- Expectations in each directory's Vitest and in the `web/e2e` specs `auth`, `guest` (relevant
  parts), `settings`, `scan-progress` and `unconfigured`; remove these three directories from the
  ESLint exclusion list.

**Dependencies**: Build the UI i18n foundation (text catalog, English formatting, API error
display)

**Acceptance**:

- ESLint passes without the exclusions. Tests that render the normal, empty, in-progress and
  failed states in the pseudo-locale pass and assert no text outside the catalog.
- Vitest and `task test-e2e` assert English explanations for initial setup, login (wrong password,
  rate limiting) and Settings (a missing or overlapping media folder, a scan failure, an old
  failure without `errorCode`).
- The screens change, so visual and assistive technology checks are performed.

### Make the library, the folder pages and the shared list English

**Scope**:

- Move into the catalog the text, accessible names, count and date-time formatting, search
  descriptions, selection and bulk operations, and group cards of `web/src/library/`,
  `web/src/videoList/` and `web/src/folders/`.
- Expectations in each directory's Vitest and in the `web/e2e` specs `search`, `folders`, `guest`
  (relevant parts) and `hover-preview`; remove these three directories from the ESLint exclusion
  list.

**Dependencies**: Build the UI i18n foundation (text catalog, English formatting, API error
display)

**Acceptance**:

- ESLint passes without the exclusions. Tests that render the normal, empty, in-progress and
  failed states in the pseudo-locale pass and assert no text outside the catalog.
- Vitest and `task test-e2e` assert: counts of one and several videos, empty states, load
  failures, the search term length and bulk operation limits (interpolated `limit`), and that
  videos and folders with Japanese names are shown and searchable under their original names.
- The screens change, so visual and assistive technology checks are performed.

### Make tag management English

**Scope**:

- Move the text, accessible names and counts of `web/src/tags/` into the catalog.
- Show invalid, conflicting and failed-merge tag names with the API error display
  (`tag_name_*`, `merge_same_tag`, `tag_name_taken`, `tag_merge_required`).
- Expectations in Vitest and `web/e2e/tags.e2e.ts`; remove `tags/` from the ESLint exclusion list.

**Dependencies**: Build the UI i18n foundation (text catalog, English formatting, API error
display)

**Acceptance**:

- ESLint passes without the exclusions. Tests that render the normal, empty, in-progress and
  failed states in the pseudo-locale pass and assert no text outside the catalog.
- Vitest and `task test-e2e` assert specific English explanations for: an empty tag name, a tag
  name that is too long (interpolated `limit`), a name in use, another tag's synonym, a name that
  requires a merge (each including the conflicting tag name), and merging a tag with itself.
- Vitest and `task test-e2e` also assert that Japanese tag names and synonyms are shown and
  searchable unchanged.
- The screens change, so visual and assistive technology checks are performed.

### Make the playback page and the player English

**Scope**:

- The text, accessible names and date formatting of `web/src/player/`.
- Build the video.js custom language from the catalog
  ([R-9](research.md#r-9-videojs-text-is-built-from-the-catalog)).
- Show probe failures from `probeErrorCode`, without `probeError`; use the API error display for
  playback and file operation failures.
- Expectations in Vitest and `web/e2e/playback.e2e.ts`; remove `player/` from the ESLint exclusion
  list, which removes the list.

**Dependencies**: Build the UI i18n foundation (text catalog, English formatting, API error
display)

**Acceptance**:

- ESLint passes with no exclusion list. Tests that render the normal, empty, in-progress and
  failed states in the pseudo-locale pass and assert no text outside the catalog.
- Vitest and `task test-e2e` assert: control bar button accessible names and tooltips are English
  and include the key; the explanation for each `probeErrorCode`; an old Japanese `probeError`
  without a code is not shown and an English summary appears instead.
- They also assert the "Open file" failure when the file is missing, and that keyboard shortcuts
  do not change.
- The screens change, so visual and assistive technology checks are performed.
