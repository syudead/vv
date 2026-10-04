# Data model: Search in the list and folder screens

Parent Issue: #195.

The rest of the model is unchanged: the existing table definitions are owned by
[internal/store/migrations/](../../internal/store/migrations/), and the split
between indexes and user data is in [ARCHITECTURE.md](../../ARCHITECTURE.md).
This document covers only the columns and tables this feature adds and the rules
that fill them. `videos`, `media_folders`, `playback_progress` and `jobs` do not
change. Everything added belongs to the rebuildable index side.

## 1. Columns added to `video_locations`

| Column | Type | Meaning |
| --- | --- | --- |
| `search_key` | `text not null default ''` | The match string for one location. Built in Go by the rules in §3 |
| `title_key` | `text not null default ''` | The key for sorting titles in natural order. Built in Go by the rules in §4 |
| `search_version` | `integer not null default 0` | The version of the rules that built `search_key` and `title_key`. See §5 |

The keys are stored per location so that requirement 9 (a video does not match
when different terms are satisfied by different locations) is decided within one
location row.

## 2. Replacing the full-text index

`videos_fts` (a trigram index on `title` and `path`) and the triggers that keep it
in sync, `video_locations_ai`, `video_locations_ad` and `video_locations_au`, are
dropped. In their place, the following index with only the `search_key` column is
created:

```sql
create virtual table location_search_fts using fts5(
    search_key, content='video_locations', content_rowid='id', tokenize='trigram'
);
```

The sync triggers copy only `search_key` (on insert and delete, and on updates of
`search_key`). Changes to the title or path reach the index because Go rebuilds
`search_key` in the same write.

The migration is added as `00007_location_search.sql`. Existing migrations do not
change (`scripts/migrations-immutable.sh`). SQL alone cannot express the
normalisation in §3, so the migration only creates the columns, the index and the
triggers; the key values are filled as described in §5. Down recreates
`videos_fts` and the three triggers as defined in 00003, refills the index with
`'rebuild'`, and then drops the added columns.

## 3. `search_key` rules

```text
search_key = fold(title) + "\n" + fold(relative_path)
```

- `relative_path` is the path below the registered media folder that contains
  the location, separated by `/`, including the extension. The path of the
  registered media folder itself is not included (requirement 8).
- Newlines inside the title and the relative path are turned into spaces after
  `fold`. A newline remains only at the boundary between the two, and the query
  side also treats a newline as a space, so no term matches across the boundary.
- Whether a location is under a registered folder is decided by the same rule as
  the list. The separators are `/` and `\` on Windows and only `/` on other
  operating systems.
- A location contained in no registered media folder gets `search_key = ''`.
  Such a location never appears in the list anyway
  (`registeredLocationCondition`).
- `fold` is the conversion to the match form in `internal/domain`; the same
  conversion is applied to query terms
  ([contracts/list-api.md §1](contracts/list-api.md#1-query-syntax)). It applies
  NFKC normalisation, Unicode lower-casing, and hiragana-to-katakana replacement,
  in that order.

## 4. `title_key` rules

A function in `internal/domain` builds it from the title: a string whose byte
order is natural order. After `fold`, each run of ASCII digits is replaced with
"a prefix giving the number of digits without leading zeros, as four decimal
digits, followed by the digits without leading zeros" (`2` becomes `00012`, `10`
becomes `000210`). The prefix starts with a digit, so the order between a digit
run and other characters stays the same as for the original characters. Sorting
breaks ties on `id` when `title_key` is equal.

The defined order is "natural order of the title in match form". For two
different titles `a` and `b`, the byte order of `title_key` gives the same
direction as `CompareNatural(fold(a), fold(b))`. Because the comparison happens
after `fold`, full-width digits (`２`) and `①`, which NFKC turns into digits,
compare as digits, and hiragana compares as katakana (between `い` and `ア`, `ア`
comes first). Titles that differ only in case, width or kana type get the same
key and are ordered by `id`.

The order of folder cards (`CompareNatural` on the raw names) can differ for
names that contain kana or full-width digits. This feature does not change the
order of folder cards.

## 5. When keys are built, and `search_version`

`domain.SearchKeyVersion` (1 initially) is the version of the rules in §3 and §4.
Keys are rebuilt for a location at the following points, and `search_version` is
set to the current version:

| When | Target |
| --- | --- |
| At startup, right after migrations and before accepting HTTP | All locations whose `search_version` is lower than the current version |
| When a scan adds or updates a location (`UpsertVideo`) | That location |
| When a media folder is added or changed (`AddMediaFolder`, `ReplaceMediaFolder`) | Locations under the new registered folder |

The startup refill runs right after `store.Migrate`, before interrupted jobs are
reset, before the job worker starts and before HTTP is accepted. It writes in
transactions of 500 rows, and a failure stops startup (search is never served
with old keys). The version is per row, so the next startup continues from where
it stopped. Rebuilding on a media folder add or change happens in the same
transaction as the folder change.

The startup refill lets an existing library meet requirements 7 and 8 without
rescanning (acceptance criterion 8). To change the rules, raise the version, and
the same path rebuilds every key. A deleted location loses its row, so it is not
a rebuild target.

## 6. Deriving watch state

Nothing is stored. `playback_progress` is looked up by `content_key` and reduced
to the following three values. This is the same definition as `watchState` on the
list screen (`web/src/lib/format.ts`); the server holds it as a function in
`internal/domain` and the matching SQL condition.

| State | Condition |
| --- | --- |
| Unwatched `unwatched` | No record, or `completed = 0` and `position_ms = 0` |
| Watched `watched` | `completed = 1` |
| In progress `inProgress` | Anything else |

"Recently played" uses `playback_progress.updated_at`, and videos without a
record go last in either direction (acceptance criterion 14).
