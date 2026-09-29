# Contract: search, filter and sort for the list APIs

Parent Issue: #195.

The source of truth for the API is [api/openapi.yaml](../../../api/openapi.yaml). This document
records only the differences for `listVideos` (`GET /api/videos`) and `listFolderVideos`
(`GET /api/folders/{rootId}/videos`). Other routes do not change. The meaning of the existing
parameters (`cursor`, `limit`, and `path` on `listFolderVideos`) does not change.

## 1. Search syntax

`query` uses the same syntax on both routes. The maximum of 100 characters stays. One function
in `internal/domain` interprets it, and it returns no error for any input (Requirement 5).

1. Normalize the whole input to NFKC. This also turns full-width spaces, `＂`, `－` and `｜`
   into half-width characters.
2. Split on whitespace (Unicode White_Space). A `"` at the start of a term (including right
   after the exclusion `-`) up to the next `"` is one term (a phrase), spaces included. An
   unclosed `"` is kept in the term as a literal character.
3. A leading `-` followed by a term or a phrase makes an exclusion term. A term that is only
   `-` searches for a literal `-`.
4. A standalone `OR` (upper case) or `|` is the OR operator only when a term precedes it and a
   term follows it. Any other `OR` (at the start, at the end, or right after an operator) is a
   literal term (Acceptance criterion 5). Any other `|` is dropped (Edge case "input of only
   `|` returns everything"). A `|` without spaces, as in `a|b`, is part of the term.
   - Rejected: also splitting on a `|` without spaces. It would make a `|` in a Unix file name
     unsearchable, and it would behave differently from `OR`.
5. Drop empty phrases (`""`, `-""`) and phrases that contain only whitespace.
6. Apply the conversion to the matching form (`fold`,
   [data-model.md §3](../data-model.md#3-search_key-rules)) to each term.
7. Use at most the first 16 terms. Ignore the 17th and later terms and the operators attached
   to them.
   - Counting from the start lets the user predict that terms apply in typing order.
   - 16 is enough for listing remembered fragments, and keeps the condition clauses and
     arguments of one query small.
   - Rejected: no limit, returning an error at the SQLite expression depth or argument limit.
     It violates the Edge case (return no error).
8. The meaning is as follows.
   - Terms separated by spaces must all match (AND).
   - A chain of terms joined by OR is one term that matches when any of them matches. OR binds
     tighter than a space.
   - An exclusion term matches when the text does not contain it. The same applies to an
     exclusion term inside an OR.
   - When no term remains, nothing is filtered.
9. A video matches when one of its locations under a registered media folder satisfies the
   whole expression (Requirement 9). The match runs against that location's `search_key`.
10. A term of 3 or more characters in matching form is checked with `MATCH` on
    `location_search_fts` (the whole term quoted as one phrase). A term of 1 to 2 characters is
    checked with `instr` on `search_key`. Both are substring matches, and every symbol the user
    types is treated literally.

Examples (against the titles in Acceptance criteria 1 to 5):

| Input | Meaning |
| --- | --- |
| `京都 2024` | Contains `京都` and `2024` |
| `"京都旅行 2024"` | Contains `京都旅行 2024` |
| `京都 -2023` | Contains `京都` and does not contain `2023` |
| `2024 京都 OR 奈良` | Contains `2024`, and contains `京都` or `奈良` |
| `OR` / `-` / `"京都` | Contains the literal `or`, `-` or `"京都` |
| `|` / `""` / whitespace only | No filtering |

## 2. Added parameters

Added to both routes:

| Name | Type | Default | Meaning |
| --- | --- | --- | --- |
| `watch` | `all` \| `unwatched` \| `inProgress` \| `watched` | `all` | Filters by watch status. Defined in [data-model.md §6](../data-model.md#6-deriving-the-watch-status) |
| `playable` | boolean | `false` | When `true`, returns only videos with `playable = true` |
| `seed` | integer (0 to 2147483647 inclusive) | `0` | Determines the order for `sort=random`. Ignored for other orders |

Added to `listFolderVideos` only:

| Name | Type | Default | Meaning |
| --- | --- | --- | --- |
| `query` | string (at most 100 characters) | empty | Filters with the syntax in §1 |
| `scope` | `direct` \| `subtree` | `direct` | `direct` covers only the locations directly in the folder. `subtree` covers the locations in the folder and everything below it |

`scope` and `query` are independent. The UI uses `subtree` only when there is a search term
([list-url.md](list-url.md)). The match (§1-9) covers only the locations inside that scope.

`total` is the count of all items after the search terms, `watch`, `playable` and the scope are
applied (Requirement 13).

## 3. `VideoSort` values

The existing `addedDesc` and `titleAsc` keep their names, and the values below are added. The
default stays `addedDesc`.

| Sort | Ascending | Descending | Value used |
| --- | --- | --- | --- |
| Date added | `addedAsc` | `addedDesc` | `videos.added_at` |
| Date modified | `modifiedAsc` | `modifiedDesc` | `mtime` of the listed location (§4) |
| Title | `titleAsc` | `titleDesc` | `title_key` of the listed location (natural order) |
| Length | `durationAsc` | `durationDesc` | `videos.duration_ms`. Videos without it go last in either direction |
| File size | `sizeAsc` | `sizeDesc` | `size_bytes` of the listed location |
| Recently played | `playedAsc` | `playedDesc` | `playback_progress.updated_at`. Videos without a record go last in either direction |
| Random | `random` | — | A value derived from `seed` and `id` |

The meaning of `titleAsc` changes from byte order to natural order (Requirement 14). Every order
breaks ties by `id`: ascending `id` for an ascending order, descending `id` for a descending
order, and ascending `id` for `random`.

The `random` value depends only on `seed` and `id`. With the same `seed`, no video appears twice,
across pages and even when a scan adds or removes rows (Requirement 15, Edge case "random order
and scans").

## 4. The listed location and `Video.folder`

For one video, the list item's `title`, `sizeBytes` and sort value come from the first location,
in ascending path order, that meets these conditions:

- It is inside the target scope (under a registered media folder for the library; the range of
  `scope` for a folder).
- When there is a search term, it satisfies the whole expression from §1.

For the library list without a search term, this is the same as the current "first location
under a registered folder". With a search term, the title of the matching location is shown.

Both routes add the optional field below to each item. `GET /api/videos/{id}` also returns it,
as the folder of the representative location (the breadcrumb in the player page heading). Only
in that response does it also carry `rootName`, the display name of the registered folder.

```yaml
VideoFolder:
  type: object
  required: [rootId, path]
  properties:
    rootId: { type: integer, format: int64 }   # registered media folder that contains the location
    path:   { type: string }                    # `/`-separated path from there to the folder; empty when directly inside
    rootName: { type: string }                  # registered folder display name; GET /api/videos/{id} only
```

`Video.folder` points to the folder that holds the location above. The folder page builds the
location relative to the open folder from it (Requirement 19).

## 5. Cursors and errors

- When a scan runs during paging, the guarantee stays within the scope of the current keyset
  method. A video that matched the conditions when the first page was fetched, and whose sort
  value does not change, is neither duplicated nor missed, even when other rows are added or
  removed.
- A video that first matches mid-paging (a newly scanned video, or an existing video that gained
  a matching location) appears on a later page when it sorts after the cursor, and does not
  appear when it sorts before it.
- A video whose value changes mid-paging can appear again on the next page, or on no page at
  all. Examples: a location with an earlier path is added and changes the title, size or
  modification time; or analysis finishes and fills in the length.
  - For the reappearing case, `useVideos` in the UI drops duplicates by `id` when it appends a
    later page.
  - Both cases are reflected on the next search or reload (Edge case "during a scan").
  - The values for `random`, `addedAsc` and `addedDesc` do not change mid-paging.
- A cursor wraps the sort order name, the sort value (including a marker for a missing value)
  and `id`. Passing a cursor made for another sort order or another `seed` returns `400`
  (`ErrInvalidCursor`).
- Unknown values of `watch`, `scope` and `sort`, and an out-of-range `seed`, are checked at the
  entry of `internal/httpapi` and return `400`, the same as `sort` today. The UI resets them to
  the defaults before sending ([list-url.md](list-url.md)).
- When the folder does not exist, `listFolderVideos` returns the same `404` as today ("The
  folder wasn't found."), regardless of `scope` and `query`.
