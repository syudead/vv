# Research: Translating the service into English and the i18n foundation

Inherited decisions: [Tech stack selection](../../docs/design-docs/tech-stack-selection.md)
and [ARCHITECTURE.md](../../ARCHITECTURE.md). This file records only the
decisions this feature adds.

Facts found (on `main` as of 2026-09):

| Area | Finding |
| --- | --- |
| Screen | 124 non-test files in `web/src` contain Japanese: about 575 lines of screen text, the rest comments. There is no i18n library and no central place for text. `ja-JP` appears in `lib/format.ts`, `player/properties.ts`, `settings/ScanStatusSection.tsx`, `videoList/listSummary.ts`, `library/SelectionBar.tsx` and `folders/*`. Relative time is hand-written in `formatRelative`, and counts only append `件`, `本` or `個`, with no singular/plural distinction. video.js has its own language `vv-ja` through `addLanguage`. `index.html` has `lang="ja"`. |
| API errors on screen | `RequestFailed` in `web/src/api/client.ts` holds `status`, `code` and `message`, and `errorMessage()` passes the server's `message` straight to the screen in tags, settings, auth, shell and list load failures. When the body is not JSON, it builds the Japanese `要求に失敗しました`. |
| Server | `gosmopolitan` in `golangci-lint` (Han, Hiragana, Katakana; excluding tests and comments) finds 710 Japanese strings in `cmd/` and `internal/` (`internal/store` 274, `internal/httpapi` 206, `cmd/mdm` 61, `internal/media` 51, `internal/domain` 35, `internal/artifacts` 33, `internal/scanner` 20, `internal/app` 16, a few elsewhere). Logs use `log/slog` with Japanese messages and English attribute keys. |
| `Error` | Only `code` (an enum of 22) and `message`, with `additionalProperties: false`. `invalid_request` is used in 48 places, `not_found` in 22, `conflict` in 7, `forbidden` in 3, and situations are distinguished only by `message`. |
| Free-text failure reasons | Three columns: `videos.probe_error` (`Video.probeError`, shown as is in a `<pre>`), `scans.error` (`Scan.error`, shown as is), `jobs.last_error` (not exposed by the API). None has a machine-readable reason. The text includes the first line of ffprobe's stderr and absolute file paths. |

## R-1: Screen text in a typed in-house catalog

**Decision**: `web/src/i18n/` holds the English text catalog (`en`) and its type
`Messages`. Text is a nested object per screen area; text with embedded values
is a function that takes arguments (`selectedCount: (n: number) => …`).
Components import the catalog statically. The language is one constant fixed at
startup, and only `en` exists for now.

**Rationale**: TypeScript types check the keys and arguments, so a missing key
or a forgotten argument fails the type check in `task check`. A future language
is added as an object that satisfies `Messages`, so missing translations are
also caught by the type check (requirement 7). Switching languages is out of
scope, so no React context or provider sits in between. There is no plan to
hand text to outside translators or a translation management service; to add a
language, an LLM writes the new catalog's TypeScript from `en`. The type check
verifies missing keys and mismatched arguments in that output directly, so no
separate translation exchange format is needed.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| i18next / react-intl / Lingui | Rejected: keys are strings with weak type checking, or they add an extraction build step and an ICU parser. The advantage of ICU and extraction is exchange with translators' tools, which is not planned. With a single language, adding a dependency gains nothing |
| Distribute `t()` through React context | Rejected: the language never changes at runtime, so it only adds a provider to every test |

## R-2: Formatting and plurals through browser `Intl`

**Decision**: `web/src/i18n/` holds formatting functions that use the same
locale as the catalog (numbers, dates, date-times, relative time, singular/plural
choice). Relative time uses `Intl.RelativeTimeFormat` (keeping today's steps:
just now, minutes, hours, days, weeks, months, years); singular/plural uses
`Intl.PluralRules`, and catalog functions choose between the `one` and `other`
forms. `ja-JP` and `toLocale*String` without a locale argument are removed
outside `i18n/`.

**Rationale**: The locale is passed in one place, so no fixed Japanese locale
remains (requirement 3). It is built into every browser and adds no dependency.

**Alternatives considered**: Writing English plurals as `n === 1 ? … : …` in
each place was rejected: it cannot express other languages' plural rules (0,
few, many) and cannot handle the future languages of requirement 7.

## R-3: Missing translations caught by lint and types

**Decision**:

| Target | Check |
| --- | --- |
| Screen | `web/eslint.config.js` gains `no-restricted-syntax`, reporting in non-test `web/src` outside `i18n/`: string literals, templates and JSX text containing Japanese; JSX text; string literals for `aria-label`, `title`, `placeholder` and `alt`; `"ja-JP"`; and `toLocaleString`, `toLocaleDateString` and `toLocaleTimeString` without a locale. |
| Known errors | The catalog's error tables are `Record<ErrorCode, …>` and `Record<ErrorReason, …>` (types generated from `api/openapi.yaml`), so a code or reason added to the API without display text fails the type check. Failure reason codes ([data-model.md](data-model.md)) work the same way. |
| English text outside the catalog (types) | Catalog values have the branded type `UiText` from `i18n/`, and every value that carries fixed text is `UiText`: props and arguments of in-house components (toast body; headings and descriptions of status displays, empty states and dialogs; button names, and so on), state that holds text (such as the reason in `tags/tagNameField.ts`), helpers that return text (such as the display text in `api/folderGrouping.ts`), and the return value of the error display. English literals cannot go there and fail the type check. Values that carry user data (video names, tag names) stay `string`. |
| English text outside the catalog (screen) | `i18n/` holds a pseudo-locale that marks every sentence of the catalog, swappable only in tests. Tests of each area's main screens render with the pseudo-locale and pass through a helper that checks that all visible text and every `aria-label`, `title`, `placeholder` and `alt` either carries the mark or is user data the test supplied. An English literal passed through a `string` variable also fails, because it has no mark on the rendered screen. |
| Server | `.golangci.yml` enables `gosmopolitan` (`watch-for-scripts: [Han, Hiragana, Katakana]`, `allow-time-local: true`) for non-test code in `cmd/` and `internal/`. |

**Rationale**: All of these look at the AST, so they do not misreport the
comments that stay in Japanese. They are a built-in ESLint rule and a linter
bundled with golangci-lint, so no dependency is added. `gosmopolitan` was
confirmed to report the 710 strings above per package on today's `main`.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Line-by-line scanning like `web/src/theme/tokens.test.ts` | Rejected: cannot tell Japanese comments from screen text |
| A custom Go AST test | Rejected: the bundled `gosmopolitan` is enough |
| Warn about unknown keys at runtime | Rejected: not found until the screen is opened |
| Report English-looking literals (space-separated English words) in ESLint | Rejected: today's `web/src` has 685 such literals even excluding `className` attributes, almost all Tailwind class lists kept in helpers or variables, indistinguishable from text |

Types catch problems when the code is written; the pseudo-locale screen tests
catch the paths that slip past the types (display through a `string` variable)
for the states the tests render. States the tests do not render are covered by
rendering, in pseudo-locale tests, the states each area unit's acceptance lists
(normal, empty, in progress, failure).

To keep lint passing during the migration, the screen rule is enabled in the
foundation unit, with the directories not yet translated in a temporary
exclusion list. Each area unit removes its own directories from the list, and
the list is gone after the last unit. `gosmopolitan` is enabled in the last
server unit.

## R-4: API error specificity added through `reason` without changing codes

**Decision**: `Error` gains the optional `reason` (an enum of machine-readable
sub-reasons), `limit` (an integer) and `tagName` (the original name of the
colliding tag). `reason` is attached only where one code is used for several
situations the screen can trigger
([contracts/error-api.md §1](contracts/error-api.md#1-reason-limit-and-tagname-on-error)).
The screen looks up the catalog by `reason`, then by `code`.

**Rationale**: Requirement 5 keeps the existing `code` and HTTP status, so a new
`code` cannot be assigned to the same situation. The values today's `message`
embeds that the screen's explanation needs (the limit and the colliding tag
name) are returned as fields, so specificity is kept without using `message`.
Returning the limit in `limit` means the screen does not duplicate the server's
limits (username length, tag name length, search term length, bulk action
count).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Add a new `code` per situation | Rejected: `code` would change for API consumers who receive today's `invalid_request` and others, against requirement 5 |
| Make `params` a dictionary of arbitrary values | Rejected: only two values are needed now (the limit and the tag name), and an untyped dictionary escapes checks through the generated types |
| Attach a reason to all 48 `invalid_request` uses | Rejected: values the screen never sends (sort order names, the `attempt` format, Content-Type, and so on) are API misuse, and the code's English text and `message` are enough |

## R-5: The screen shows known errors from `reason` and `code`, others as a safe summary

**Decision**: `RequestFailed` in `web/src/api/client.ts` gains `reason`, `limit`
and `tagName`, and `errorMessage()` is replaced by the error display in
`i18n/`.

| Case | Display |
| --- | --- |
| Known `reason` or `code` | The catalog's English text (embedding `limit` and `tagName`) |
| Unknown `code` with a `message` | The server's English `message` (the fallback of requirement 6) |
| Body not JSON, empty, or no `message` | An English summary of the form `Request failed (HTTP <status>)` |
| `fetch` itself fails | An English summary saying the server was not reached; the browser's text is not shown |

A display that needs per-operation context (whether `タグ名として使えない`
refers to a folder name or a tag name) is wrapped by the caller's text, as
today.

**Rationale**: The API's `message` stays as English for API consumers, while the
screen's text lives in one place, the catalog. Even for an unknown code,
`status` and `code` remain in `RequestFailed`, so diagnostic clues are not
lost.

**Alternatives considered**: Discarding `message` and showing only a general
summary even for unknown codes was rejected: requirement 6 makes `message` the
English fallback for unknown codes.

## R-6: Store a machine-readable failure code and show no free text on screen

**Decision**: Probe and import failures store a reason code separately from the
free text ([data-model.md](data-model.md)). The adapters that create failures
(`internal/media`, `internal/scanner`) wrap them in a failure type from
`internal/domain`, and `internal/app`, `internal/jobs` and `internal/store`
extract it with `errors.As`. A failure that cannot be classified becomes
`internal`. The screen does not display the free text of `probeError` and
`Scan.error`; it builds an English explanation from the code (and, for an import
failure, the media folder path). A row without a code (from before the upgrade)
shows a general English summary.

**Rationale**: The free text contains ffprobe's stderr and OS error text, and
rows from before the upgrade are Japanese, so showing it on screen would
violate requirement 8 and the edge cases (do not show external programs' free
text directly; do not leak past Japanese). Keeping the code in a separate column
leaves the stored free text unchanged.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Parse and classify the free text on screen | Rejected: breaks whenever the wording changes, and cannot handle past Japanese |
| Rewrite stored Japanese into English in a migration | Rejected: machine translation is out of scope, and requirement 8 keeps stored reasons |
| Show the free text folded as "technical details" | Rejected: the screen cannot tell past Japanese from new English |

## R-7: English written directly in the server, with no text catalog

**Decision**: API `message`s, logs, and `cmd/mdm`'s configuration errors and
account operation output are written in English directly in the Go code. The
current shape where `internalError` uses the same sentence for the log and the
response is kept. Identifiers never shown to users, such as the subscriber names
of `/api/events`, also become English.

**Rationale**: Requirement 5 only asks that server output be uniformly English;
per-language display is done by the screen from codes (R-5). If the server
varied output by `Accept-Language`, the same text would live in two places, the
screen and the server.

**Alternatives considered**: A text catalog on the Go side too was rejected:
there is one language, English, and centralising text the screen never shows
gains nothing.

## R-8: What stays untranslated

**Decision**: Code comments, `description`s in `api/openapi.yaml`, design
documents and test names stay in Japanese. `scripts/` (developer tools such as
`task doctor`, `task dev` and `previewbench`) does not change. The Japanese font
(Noto Sans JP) stays.

**Rationale**: The requirements cover text the service generates for users and
operators, not the language of the repository's documents. `scripts/` holds
tools that run in the repository's development environment, not output of the
distributed service. The Japanese font is needed to display Japanese video,
folder and tag names correctly on the English screen (edge cases).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Translate `scripts/` too | Rejected: extends beyond the scope of the requirements |
| Remove the Japanese font | Rejected: Japanese names would be drawn in a fallback font and look broken |

## R-9: video.js text built from the catalog

**Decision**: The custom language `vv-ja` in `web/src/player/VideoPlayer.tsx` is
replaced by a custom language built from the catalog's English. The current
form that appends the key to buttons with a keyboard shortcut (`Play (Space)`
and so on) is kept.

**Rationale**: video.js's default English has no key hints, so a custom
language is still needed. The text lives in the catalog like all other text.

**Alternatives considered**: Relying on video.js's default `en` was rejected:
the keyboard hints would disappear from tooltips and accessible names, against
requirements 2 and 4.
