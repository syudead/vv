# Research: English service and i18n foundation

The inherited technology choices are in
[Technology selection](../../docs/design-docs/tech-stack-selection.md) and
[ARCHITECTURE.md](../../ARCHITECTURE.md). This page holds only the decisions this feature adds.

Facts found on `main` as of 2026-09:

| Area | Finding |
| --- | --- |
| UI | 124 non-test files in `web/src` contain Japanese; about 575 lines of that are UI text and the rest are comments. There is no i18n library and no central place for UI text. `ja-JP` appears in `lib/format.ts`, `player/properties.ts`, `settings/ScanStatusSection.tsx`, `videoList/listSummary.ts`, `library/SelectionBar.tsx` and `folders/*`. Relative time is a hand-written `formatRelative`. Counts only append the counters `件`, `本` or `個`, with no singular/plural distinction. video.js has a custom language `vv-ja` registered with `addLanguage`. `index.html` has `lang="ja"`. |
| UI API errors | `RequestFailed` in `web/src/api/client.ts` holds `status`, `code` and `message`. `errorMessage()` passes the server `message` straight to the screen in tags, settings, auth, shell and the list load failures. A non-JSON body produces the Japanese text `要求に失敗しました` ("Request failed"). |
| Server | `gosmopolitan` in `golangci-lint` (Han, Hiragana, Katakana; tests and comments excluded) finds 710 Japanese strings in `cmd/` and `internal/`: `internal/store` 274, `internal/httpapi` 206, `cmd/mdm` 61, `internal/media` 51, `internal/domain` 35, `internal/artifacts` 33, `internal/scanner` 20, `internal/app` 16, and a few elsewhere. Logs use `log/slog`; the message text is Japanese and the attribute keys are English. |
| `Error` schema | Only `code` (an enum of 22 values) and `message`, with `additionalProperties: false`. `invalid_request` is used in 48 places, `not_found` in 22, `conflict` in 7 and `forbidden` in 3. Only `message` tells the situations apart. |
| Stored failure reasons | Three free-text columns: `videos.probe_error` (`Video.probeError`; the UI shows it as-is in a `<pre>`), `scans.error` (`Scan.error`; the UI shows it as-is) and `jobs.last_error` (not in the API). None has a machine-readable reason. The text includes the first line of ffprobe stderr and absolute file paths. |

## R-1: UI text lives in a typed, in-house catalog

| | |
| --- | --- |
| **Decision** | `web/src/i18n/` holds the English catalog (`en`) and its type `Messages`. Text is a nested object per screen area. Text with interpolation is a function of its arguments (`selectedCount: (n: number) => …`). Components import the catalog statically. The language is one constant fixed at startup; this feature ships only `en`. |
| **Why** | TypeScript checks the keys and arguments, so a missing key or a forgotten argument fails the type check in `task check`. A future language is an object that satisfies `Messages`, so the type check also finds missing translations (Requirement 7). Language switching is out of scope, so there is no React context or provider. No external translator or translation service is planned. To add a language, an LLM writes the new catalog in TypeScript from `en`; the type check verifies its missing keys and argument mismatches directly, so no separate exchange format is needed. |
| **Rejected** | i18next / react-intl / Lingui: string keys give weak type checking, or they add an extraction build step or an ICU parser. ICU and extraction pay off when exchanging files with translator tools, which is not planned. With one language, the extra dependency brings no benefit. Distributing `t()` through a React context: the language never changes at runtime, so it only adds a provider to every test. |

## R-2: Formatting and plurals use the browser's `Intl`

| | |
| --- | --- |
| **Decision** | `web/src/i18n/` holds formatting functions that use the catalog's locale: numbers, dates, date-times, relative time and plural selection. Relative time uses `Intl.RelativeTimeFormat` and keeps the current buckets (just now, minutes, hours, days, weeks, months, years). Plurals use `Intl.PluralRules`, and the catalog function picks the `one` or `other` form. `ja-JP` and `toLocale*String` without a locale argument disappear outside `i18n/`. |
| **Why** | The locale is passed in one place, so no hard-coded Japanese locale remains (Requirement 3). `Intl` is built into every browser, so no dependency is added. |
| **Rejected** | Writing English plurals as `n === 1 ? … : …` at each call site: it cannot express other languages' plural rules (zero, few, many), so it cannot serve the future languages of Requirement 7. |

## R-3: Lint and types detect untranslated text

| | |
| --- | --- |
| **Decision** | Four layers, listed below the table: UI lint, error tables typed by the generated enums, the branded type `UiText`, and a pseudo-locale rendering check; plus `gosmopolitan` on the server. |
| **Why** | Each check reads the AST, so Japanese comments that stay are not false positives. ESLint's built-in rules and a linter bundled with golangci-lint add no dependency. On current `main`, `gosmopolitan` reports the 710 strings above, per package. |
| **Rejected** | A line-based scan like `web/src/theme/tokens.test.ts`: it cannot tell Japanese comments from UI text. A custom Go AST test: the bundled `gosmopolitan` is enough. A runtime warning for unknown keys: nothing is found until the screen is opened. An ESLint report for English-like literals (English words separated by spaces): current `web/src` has 685 such literals even excluding `className` attributes, almost all Tailwind class lists in helpers or variables, which cannot be told apart from UI text. |

- UI: `web/eslint.config.js` adds `no-restricted-syntax`. In non-test files of `web/src` outside
  `i18n/`, it reports string literals, templates and JSX text that contain Japanese, and JSX text
  with letters.
- UI, continued: the same rule reports string literals passed to `aria-label`, `title`,
  `placeholder` or `alt`, `"ja-JP"`, and `toLocaleString`, `toLocaleDateString` or
  `toLocaleTimeString` without a locale.
- Known errors: the catalog's error tables are `Record<ErrorCode, …>` and
  `Record<ErrorReason, …>` (types generated from `api/openapi.yaml`). A code or reason added to
  the API without UI text fails the type check. The failure-reason codes
  ([data-model.md](data-model.md)) work the same way.
- English text outside the catalog (types): catalog values have the branded type `UiText` from
  `i18n/`, and every value that carries fixed text is `UiText`. This covers props and arguments of
  in-house components, state that holds text, helpers that return text, and the return value of
  the error display. An English literal cannot go there, so the type check fails. Values that carry
  user data (video names, tag names) stay `string`.
- Examples of `UiText` targets: toast bodies, headings and descriptions of status displays, empty
  states and dialogs, button names, the reason in `tags/tagNameField.ts`, and the display text in
  `api/folderGrouping.ts`.
- English text outside the catalog (screen): `i18n/` has a pseudo-locale that marks every catalog
  string, swappable only in tests. The main screen tests of each area render in the pseudo-locale
  and pass a helper. It checks that all visible text and every `aria-label`, `title`,
  `placeholder` and `alt` carries the mark or is user data that the test supplied.
- An English literal routed through a `string` variable has no mark on the rendered screen, so the
  pseudo-locale check fails on it.
- Server: `.golangci.yml` enables `gosmopolitan` (`watch-for-scripts: [Han, Hiragana, Katakana]`,
  `allow-time-local: true`) for non-test code in `cmd/` and `internal/`.

Types catch text when it is written. The pseudo-locale screen tests catch the paths that slip past
the types (display through a `string` variable) for the states the tests render. States the tests
do not render are covered because each area unit's acceptance criteria list the states to render
in the pseudo-locale: normal, empty, in progress and failed.

To keep lint passing during the migration, the foundation unit enables the UI rule and puts the
directories not yet in English on a temporary exclusion list. Each area unit removes its own
directories from the list, and the last unit removes the list. `gosmopolitan` is enabled in the
last server unit.

## R-4: API errors gain specificity through `reason`, without changing codes

| | |
| --- | --- |
| **Decision** | `Error` gains an optional `reason` (a machine-readable sub-reason enum), `limit` (integer) and `tagName` (the original name of the conflicting tag). `reason` is added only where one code covers several situations the UI can trigger ([contracts/error-api.md §1](contracts/error-api.md#1-reason-limit-and-tagname-on-error)). The UI looks up the catalog by `reason`, then by `code`. |
| **Why** | Requirement 5 keeps the existing `code` and HTTP status, so a situation cannot get a new `code`. The values the current `message` embeds that the UI explanation needs (the bound and the conflicting tag name) come back as fields, so the explanation stays specific without `message`. Returning the bound in `limit` means the UI does not duplicate the server bounds (username length, tag name length, search term length, bulk operation count). |
| **Rejected** | A new `code` per situation: the `code` would change for API clients that receive `invalid_request` today, against Requirement 5. `params` as a dictionary of arbitrary values: only two values are needed now (the bound and the tag name), and an untyped dictionary gets no checking from the generated types. A reason on all 48 `invalid_request` sites: values the UI never sends (sort order names, the `attempt` format, Content-Type) are API misuse, and the code's English text plus `message` are enough. |

## R-5: The UI shows known errors from `reason` and `code`, and everything else as a safe summary

| | |
| --- | --- |
| **Decision** | `RequestFailed` in `web/src/api/client.ts` gains `reason`, `limit` and `tagName`. The error display in `i18n/` replaces `errorMessage()`, with the cases listed below the table. |
| **Why** | The API `message` stays as English for API clients, while UI text lives in one place, the catalog. For an unknown code, `status` and `code` stay in `RequestFailed`, so diagnostic clues are not lost. |
| **Rejected** | Dropping `message` for unknown codes too and showing only a generic summary: Requirement 6 makes `message` the English fallback for unknown codes. |

- Known `reason` or `code`: the catalog's English text, with `limit` and `tagName` interpolated.
- Unknown `code` with a `message`: the server's English `message` (the fallback of Requirement 6).
- Non-JSON body, empty body, or no `message`: an English summary of the form
  `Request failed (HTTP <status>)`.
- `fetch` itself fails: an English summary saying the server was not reached. The browser's text
  is not shown.
- Where a display needs per-operation context (whether "cannot be used as a tag name" refers to a
  folder name or a tag name), the caller's text wraps it, as today.

## R-6: Failure reasons are stored as machine-readable codes; the UI does not show free text

| | |
| --- | --- |
| **Decision** | Probe and scan failures store a reason code separate from the free text ([data-model.md](data-model.md)). The adapter that creates the failure (`internal/media`, `internal/scanner`) wraps it in the `internal/domain` failure type; `internal/app`, `internal/jobs` and `internal/store` extract it with `errors.As`. A failure that cannot be classified becomes `internal`. The UI does not show the free text of `probeError` or `Scan.error`; it builds the English explanation from the code (and, for a scan failure, the media folder path). Rows without a code (from before the upgrade) show a generic English summary. |
| **Why** | The free text contains ffprobe stderr and OS error text, and rows from before the upgrade are Japanese. Showing them would violate Requirement 8 and the Edge cases (do not show an external program's free text directly; do not leak old Japanese). The code lives in its own column, so the stored free text remains unchanged. |
| **Rejected** | Parsing the free text in the UI to classify it: it breaks every time the wording changes and cannot handle old Japanese. Rewriting the stored Japanese into English in a migration: machine translation is out of scope, and Requirement 8 requires keeping stored reasons. Showing the free text collapsed as "technical details": the UI cannot tell old Japanese from new English. |

## R-7: The server has no text catalog; Go code contains English directly

| | |
| --- | --- |
| **Decision** | API `message`s, logs, and the configuration errors and account command output of `cmd/mdm` are written in English directly in Go. `internalError` keeps using the same sentence for the log and the response. Identifiers that users never see, such as the `/api/events` subscriber names, also become English. |
| **Why** | Requirement 5 only asks for server output in English; per-language display is done by the UI from codes (R-5). If the server varied output by `Accept-Language`, the same text would live in two places, the UI and the server. |
| **Rejected** | A text catalog on the Go side too: only one language, English, is used, and centralizing text the UI never displays brings no benefit. |

## R-8: What stays untranslated

| | |
| --- | --- |
| **Decision** | Code comments, `description` in `api/openapi.yaml`, design documents and test names stay Japanese. `scripts/` (developer tools such as `task doctor`, `task dev` and `previewbench`) does not change. The Japanese font (Noto Sans JP) stays. |
| **Why** | The requirements cover user-facing and operator-facing text the service produces, not the language of repository documents. `scripts/` runs in the repository's development environment; it is not output of the distributed service. The Japanese font is needed to render Japanese video, folder and tag names correctly in the English UI (Edge cases). |
| **Rejected** | Translating `scripts/` too: it extends beyond the requirements' scope. Removing the Japanese font: Japanese names would render in a fallback font and look broken. |

## R-9: video.js text is built from the catalog

| | |
| --- | --- |
| **Decision** | The custom language `vv-ja` in `web/src/player/VideoPlayer.tsx` is replaced by a custom language built from the catalog's English. Buttons with keyboard shortcuts keep showing the key, as today (`Play (Space)` and so on). |
| **Why** | video.js's default English has no key hints, so a custom language is still needed. The text moves to the catalog like all other UI text. |
| **Rejected** | Relying on video.js's default `en`: the keyboard hints would disappear from tooltips and accessible names, against Requirements 2 and 4. |
