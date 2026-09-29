# Data model: search in the list and folder pages

Parent Issue: #195.

The source of truth for the existing table definitions is
[internal/store/migrations/](../../internal/store/migrations/). The split between the index and
user data is in [ARCHITECTURE.md](../../ARCHITECTURE.md). This document records only the columns
and tables this feature adds, and the rules that fill them. `videos`, `media_folders`,
`playback_progress` and `jobs` do not change. Everything added belongs to the rebuildable index.

## 1. Columns added to `video_locations`

| Column | Type | Meaning |
| --- | --- | --- |
| `search_key` | `text not null default ''` | Matching string for one location. Go builds it with the rules in §3 |
| `title_key` | `text not null default ''` | Key for sorting titles in natural order. Go builds it with the rules in §4 |
| `search_version` | `integer not null default 0` | Version of the rules that built `search_key` and `title_key`. See §5 |

The keys are stored per location so that Requirement 9 (a video does not match when different
terms are satisfied by different locations) is decided within one location row.

## 2. Replacing the full-text index

Drop `videos_fts` (trigram over `title` and `path`) and its sync triggers `video_locations_ai`,
`video_locations_ad` and `video_locations_au`. Create the index below instead, with only the
`search_key` column.

```sql
create virtual table location_search_fts using fts5(
    search_key, content='video_locations', content_rowid='id', tokenize='trigram'
);
```

The sync triggers copy only `search_key` (insert, delete, and update of `search_key`). A change to
the title or the path reaches the index because Go rebuilds `search_key` in the same write.

The migration is added as a new file, `00007_location_search.sql`. Existing migrations do not
change (`scripts/migrations-immutable.sh`).

- SQL alone cannot express the normalization in §3. The migration therefore only creates the
  column, the index and the triggers, and §5 fills in the key values.
- Down recreates `videos_fts` and the 3 triggers exactly as defined in 00003, refills the index
  with `'rebuild'`, and then drops the added columns.

## 3. `search_key` rules

```text
search_key = fold(title) + "\n" + fold(relative_path)
```

- `relative_path` is the path relative to the registered media folder that contains the
  location, separated by `/`, with the extension. The path of the registered media folder itself
  is not included (Requirement 8).
- Line breaks inside the title and the relative path are turned into spaces after `fold`. A line
  break remains only at the boundary between the two parts. The search term side also treats a
  line break as a space, so a match never spans the boundary.
- Whether a location is under a registered folder follows the same rule as the list. The
  separators are `/` and `\` on Windows, and only `/` on other operating systems.
- A location not contained in any registered media folder gets `search_key = ''`. That location
  never appears in the list anyway (`registeredLocationCondition`).
- `fold` is the conversion to the matching form in `internal/domain`. The same conversion is
  applied to search terms ([contracts/list-api.md §1](contracts/list-api.md#1-search-syntax)).
  It applies, in order, NFKC normalization, Unicode lower-casing, and hiragana-to-katakana
  replacement.

## 4. `title_key` rules

A function in `internal/domain` builds `title_key` from the title. Its byte order equals natural
order.

- It applies `fold`, then replaces each run of ASCII digits with a prefix plus the digits without
  leading zeros. The prefix is the count of those digits in 4 decimal digits (`2` becomes
  `00012`, `10` becomes `000210`).
- The prefix starts with a digit, so a digit run sorts against other characters as the original
  characters do.
- The sort breaks ties on equal `title_key` by `id`.

The order defined is "natural order of the title in matching form". For two different titles `a`
and `b`, the byte order of `title_key` gives the same direction as
`CompareNatural(fold(a), fold(b))`. The comparison runs after `fold`, so full-width digits that
NFKC turns into digits (`２`) and `①` compare as digits, and hiragana compares as katakana
(between `い` and `ア`, `ア` comes first). Titles that differ only in case, width or kana get the
same key and are ordered by `id`.

The folder card order (`CompareNatural` on the raw names) can differ for names that contain kana
or full-width digits. This feature does not change the folder card order.

## 5. When keys are built, and `search_version`

`domain.SearchKeyVersion` (initially 1) is the version of the rules in §3 and §4. At each point
below, the keys are rebuilt for the target locations, and `search_version` is set to the current
version.

| Point | Target |
| --- | --- |
| Startup, right after the migration and before HTTP is accepted | Every location whose `search_version` is lower than the current version |
| A scan adds or updates a location (`UpsertVideo`) | That location |
| A media folder is added or changed (`AddMediaFolder`, `ReplaceMediaFolder`) | Locations under the new registered folder |

- The startup refill runs right after `store.Migrate`, before interrupted jobs are reset, before
  the job worker starts, and before HTTP is accepted.
- It writes in transactions of 500 rows. On failure, startup stops, so search never runs with
  stale keys.
- The version is per row, so the next startup continues from where it stopped.
- The rebuild on a media folder add or change runs in the same transaction as the folder change.

The startup refill lets an existing library satisfy Requirements 7 and 8 without a rescan
(Acceptance criterion 8). To change the rules, bump the version, and the same path rebuilds every
row. A deleted location loses its whole row, so it is never a rebuild target.

## 6. Deriving the watch status

Nothing is stored. Look up `playback_progress` by `content_key` and collapse it into the 3 values
below. The definition matches `watchState` in the list UI (`web/src/lib/format.ts`). The server
holds it as a function in `internal/domain` and the matching SQL condition.

| Status | Condition |
| --- | --- |
| Unwatched `unwatched` | No record, or `completed = 0` and `position_ms = 0` |
| Watched `watched` | `completed = 1` |
| In progress `inProgress` | Anything else |

"Recently played" uses `playback_progress.updated_at`. Videos without a record go last in either
direction (Acceptance criterion 14).
