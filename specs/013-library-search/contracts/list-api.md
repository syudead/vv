# Contract: Search, filters and sort in the list APIs

Parent Issue: #195.

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This document
describes only the changes to `listVideos` (`GET /api/videos`) and
`listFolderVideos` (`GET /api/folders/{rootId}/videos`). Other endpoints, and the
meaning of the existing parameters (`cursor`, `limit`, and `path` on
`listFolderVideos`), do not change.

## 1. Query syntax

`query` uses the same syntax on both endpoints. The maximum of 100 characters
stays. One function in `internal/domain` interprets it, and it returns no error
for any input (requirement 5).

1. Normalise the whole input to NFKC. This also turns full-width spaces, `＂`,
   `－` and `｜` into their half-width forms.
2. Split on white space (Unicode White_Space). From a `"` at the start of a term
   (including right after the exclusion `-`) to the next `"` is one term, spaces
   included (a phrase). An unclosed `"` is kept in the term as a literal
   character.
3. A `-` followed by a term or phrase makes an excluded term. A term that is only
   `-` searches for a literal `-`.
4. A standalone `OR` (upper case) or `|` is the OR operator only when a term
   precedes it and a term follows it. Any other `OR` (at the start, at the end, or
   right after an operator) is a literal term (acceptance criterion 5), and any
   other `|` is dropped (Edge Case "input that is only `|` lists everything"). A
   `|` without surrounding spaces, as in `a|b`, is part of the term. Splitting on
   `|` without spaces too was rejected: it would make it impossible to search for
   `|`, which appears in Unix file names, and its behaviour would not match `OR`.
5. Drop empty phrases (`""`, `-""`) and phrases that contain only spaces.
6. Apply the conversion to the match form (`fold`,
   [data-model.md §3](../data-model.md#3-search_key-rules)) to each term.
7. Use the first 16 terms. The 17th term onwards, and the operators attached to
   them, are ignored. Counting from the start lets the user predict that terms
   take effect in the order typed. 16 was chosen as enough for listing the
   fragments a user remembers, while keeping the condition clauses and arguments
   of one query small. Having no limit and returning an error at SQLite's
   expression-depth or argument limit was rejected because it violates the Edge
   Case (never return an error).
8. Meaning:

   | Construct | Meaning |
   | --- | --- |
   | Terms separated by spaces | All must match (AND) |
   | Terms joined by OR | One item; any of them matches. OR binds tighter than a space |
   | Excluded term | Matches when the text does not contain it; the same inside an OR |
   | No term left | No filtering |

9. A video matches when one of its locations under a registered media folder
   satisfies the whole expression (requirement 9). The match target is that
   location's `search_key`.
10. Terms of three or more characters in match form are checked with `MATCH`
    against `location_search_fts` (the whole term quoted as one phrase); terms of
    one or two characters with `instr` on `search_key`. Both are substring
    matches, and every symbol the user typed is treated literally.

Examples (against the titles of acceptance criteria 1 to 5):

| Input | Meaning |
| --- | --- |
| `京都 2024` | Contains `京都` and `2024` |
| `"京都旅行 2024"` | Contains `京都旅行 2024` |
| `京都 -2023` | Contains `京都` and does not contain `2023` |
| `2024 京都 OR 奈良` | Contains `2024`, and contains `京都` or `奈良` |
| `OR` / `-` / `"京都` | Contains the literal `or`, `-` or `"京都` |
| `|` / `""` / spaces only | No filtering |

## 2. Added parameters

Added to both endpoints:

| Name | Type | Default | Meaning |
| --- | --- | --- | --- |
| `watch` | `all` \| `unwatched` \| `inProgress` \| `watched` | `all` | Filters by watch state. Defined in [data-model.md §6](../data-model.md#6-deriving-watch-state) |
| `playable` | boolean | `false` | When `true`, only videos with `playable = true` |
| `seed` | integer (0 to 2147483647) | `0` | Decides the order for `sort=random`. Ignored for other sorts |

Added to `listFolderVideos` only:

| Name | Type | Default | Meaning |
| --- | --- | --- | --- |
| `query` | string (max 100 characters) | empty | Filters with the syntax in §1 |
| `scope` | `direct` \| `subtree` | `direct` | `direct` covers only locations directly in the folder; `subtree` covers locations in the folder and everything below it |

`scope` and `query` are independent. The screen uses `subtree` only when there is
a query ([list-url.md](list-url.md)). Matching (§1, item 9) covers only the
locations in that scope.

`total` is the count of all items after the query, `watch`, `playable` and the
scope are applied (requirement 13).

## 3. `VideoSort` values

The existing `addedDesc` and `titleAsc` keep their names, and the following
values are added. The default stays `addedDesc`.

| Sort | Ascending | Descending | Value used |
| --- | --- | --- | --- |
| Date added | `addedAsc` | `addedDesc` | `videos.added_at` |
| Date modified | `modifiedAsc` | `modifiedDesc` | `mtime` of the listed location (§4) |
| Title | `titleAsc` | `titleDesc` | `title_key` of the listed location (natural order) |
| Duration | `durationAsc` | `durationDesc` | `videos.duration_ms`. Videos without it go last in either direction |
| File size | `sizeAsc` | `sizeDesc` | `size_bytes` of the listed location |
| Recently played | `playedAsc` | `playedDesc` | `playback_progress.updated_at`. Videos without a record go last in either direction |
| Random | `random` | — | A value made from `seed` and `id` |

The meaning of `titleAsc` changes from byte order to natural order
(requirement 14). Every sort breaks ties on `id` (ascending `id` for ascending
sorts, descending for descending sorts, and ascending `id` for `random`).

The `random` value depends only on `seed` and `id`. With the same `seed`, the
same video never appears twice, across pages and even when a scan adds or
removes rows (requirement 15, Edge Case "random order and scans").

## 4. Listed location and `Video.folder`

For one video, the list item's `title`, `sizeBytes` and sort value come from the
first location, in ascending path order, among the locations that:

- are in the scope (under a registered media folder for the library; within the
  `scope` range for a folder)
- satisfy the whole §1 expression, when there is a query

In the library list without a query, this is the same as today's "first
location under a registered folder". With a query, the title of the matching
location is shown.

The following optional field is added to the items of both endpoints. It is also
set on `GET /api/videos/{id}` as the folder of the representative location (for
the breadcrumb in the player screen heading; only there it also carries
`rootName`, the display name of the registered folder).

```yaml
VideoFolder:
  type: object
  required: [rootId, path]
  properties:
    rootId: { type: integer, format: int64 }   # registered media folder that contains the location
    path:   { type: string }                    # `/`-separated relative path from there to the folder; empty when directly in it
    rootName: { type: string }                  # display name of the registered folder; GET /api/videos/{id} only
```

`Video.folder` points at the folder that holds the location above. The folder
screen builds the location relative to the open folder from it
(requirement 19).

## 5. Cursor and errors

- The guarantee when a scan runs while paging stays within what the current
  keyset approach gives. A video that matched the conditions when the first page
  was fetched, and whose sort value does not change, is neither duplicated nor
  missed, even when other rows are added or removed. A video that starts matching
  midway (a newly scanned video, or an existing video that gains a matching
  location) appears in a later page if it sorts after the cursor, and does not
  appear if it sorts before. A video whose value changes midway may appear again
  on the next page, or on no page at all; for example, when a location with an
  earlier path is added and changes the title, size or modification time, or when
  analysis finishes and fills in the duration. Duplicates are dropped by `id` when
  the screen's `useVideos` appends the next page. Missed videos show up on the
  next search or reload (Edge Case "during a scan"). The values for `random`,
  `addedAsc` and `addedDesc` never change midway.
- The cursor wraps the sort name, the sort value (including a marker for a
  missing value) and `id`. Passing a cursor made with a different sort or a
  different `seed` returns `400` (`ErrInvalidCursor`).
- Unknown values of `watch`, `scope` and `sort`, and an out-of-range `seed`, are
  checked at the `internal/httpapi` entry and return `400`, as `sort` does today.
  The screen resets them to the defaults before sending
  ([list-url.md](list-url.md)).
- When the folder does not exist, `listFolderVideos` returns the same `404` as
  today (`そのフォルダは見つかりません`), regardless of `scope` and `query`.
