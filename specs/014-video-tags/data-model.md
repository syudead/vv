# Data model: Video tags

Parent Issue: #193.

The existing tables are defined in
[internal/store/migrations/](../../internal/store/migrations/). The distinction
between a rebuildable index and user data that cannot be recreated is stated at
the top of [00002_core.sql](../../internal/store/migrations/00002_core.sql) and
in [ARCHITECTURE.md](../../ARCHITECTURE.md). This document covers only the
tables this feature adds and the rules for writing them. Existing tables do not
change.

All three new tables are user data that cannot be recreated. A rescan or a
media folder change must not delete them, so they have no foreign key to
`videos` and link to a video by `content_key`, as `playback_progress` does
(requirement 9).

## 1. Migration

New file `00008_tags.sql`. The last migration on `main` is #195's
`00007_location_search.sql`.

```sql
-- The tag itself. Names live in tag_names.
-- autoincrement keeps a deleted tag's id from being reused by another tag,
-- so an id left in an old tab or URL never points at an unrelated new tag.
create table tags (
    id         integer primary key autoincrement,
    created_at integer not null
);

-- Tag names and synonyms in one namespace.
-- name uses the default BINARY collation: two names are the same only when
-- spelled exactly alike (requirement 4).
create table tag_names (
    name      text    primary key,
    tag_id    integer not null references tags (id) on delete cascade,
    -- 1 is the original name shown on screen, 0 is a synonym.
    canonical integer not null check (canonical in (0, 1)),
    -- Key for search box matching. Go writes domain.FoldForMatch(name) (§7).
    search_key     text    not null default '',
    -- Version of the rule that built search_key. The same domain.SearchKeyVersion as video_locations.search_version.
    search_version integer not null default 0
) without rowid;

-- One original name per tag.
create unique index tag_names_canonical_idx on tag_names (tag_id) where canonical = 1;
create index tag_names_tag_idx on tag_names (tag_id);

-- Video content to tag mapping. User data, so no foreign key to videos.
create table video_tags (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    created_at  integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- Used for per-tag counts and for reassignment on merge and delete.
create index video_tags_tag_idx on video_tags (tag_id, content_key);
```

Down drops the three tables.

Invariants (added to the checks in `internal/store/invariants_test.go`):

| Rule | Enforced in |
| --- | --- |
| Every `tags` row has exactly one `tag_names` row with `canonical = 1` | `invariants_test.go` |
| Every `video_tags.tag_id` and `tag_names.tag_id` points at an existing `tags` row | Foreign keys (`foreign_keys(on)` is already in the DSN) |

## 2. Name rules

One function in `internal/domain` normalizes input into a name.

1. Input containing a control character (Unicode general category Cc, including
   newline and tab) anywhere is an error (`invalid_request`). A tag name with a
   newline does not fit on one card line or in a tooltip, and search terms fold
   newlines into spaces, so such a name would not match in search either (§7).
   - The check runs on the input before whitespace is trimmed. Newline, tab and
     CR are also Unicode White_Space, so applying step 2 first would strip the
     leading or trailing control characters of `"旅行\n"` or `"\t旅行"` and
     create input that should be rejected as a different name. The screen
     refuses to take in a paste containing a newline and shows why
     ([ui-design.md "Combobox"](ui-design.md#combobox)), so the API rejects the
     same input.
   - Rejected: checking for control characters after trimming. Leading and
     trailing newlines and tabs would be dropped silently, and the API and the
     screen would give different results for the same input.
2. Trim leading and trailing whitespace (Unicode White_Space). Inner whitespace
   stays. The only White_Space left after step 1 is characters that are not
   control characters, such as the space, the ideographic space and the
   no-break space.
3. An empty result is an error (`invalid_request`).
4. More than 100 code points is an error (`invalid_request`). Without a limit,
   one name could grow a card line, the suggestions and the body outside the URL
   without bound. The number matches the search term limit of 100, so the user
   has one limit to remember.
5. **No** Unicode normalization and **no** case folding. `Anime` and `anime`,
   and full-width `Ａ` and half-width `A`, are different names (requirement 4,
   acceptance criterion 6).

## 3. Name lookup

A tag is looked up from a name with one `tag_names` lookup by `name`. The
original name and a synonym reach the same tag (requirement 7). The name shown
on screen is always the `canonical = 1` row.

## 4. Write rules

Every operation runs in one transaction and leaves nothing behind when it fails
partway (Edge Case `一括操作・統合の途中失敗`).

| Operation | Writes |
| --- | --- |
| Create | One `tags` row and one `tag_names` row with `canonical = 1`. `ErrTagNameTaken` when the name exists |
| Rename | Rewrite `name` in the tag's `canonical = 1` row. The same name as now changes nothing. `ErrTagNameTaken` when the new name exists (even as the tag's own synonym) |
| Delete | Delete the `tags` row. `tag_names` and `video_tags` cascade. Tags on videos not currently in the library are deleted too |
| Merge (source X → target Y) | `insert or ignore into video_tags select content_key, Y, … from video_tags where tag_id = X`, move X's names (original and synonyms) to `tag_id = Y`, `canonical = 0`, delete X. X's original name becomes a synonym of Y |
| Add synonym (name n to tag T) | When n does not exist, add one row with `canonical = 0`. When n is already a synonym of T, change nothing. When n is T's original name, `ErrTagNameTaken`. When n is a synonym of another tag S, `ErrTagNameTaken` (naming S). When n is the original name of another tag S: `ErrTagMergeRequired` unless the accepted merge source `id` is S (including when none is given); when it is S, merge S → T (n becomes a synonym of T through the merge) |
| Remove synonym | Delete that `canonical = 0` row. When n is not a synonym of T, change nothing |
| Add to videos | `insert or ignore` per target video `content_key`. A tag already attached changes nothing |
| Remove from videos | `delete` per target `content_key`. A tag not attached changes nothing |

- On merge, content that had both tags ends up as one row through
  `insert or ignore` (acceptance criterion 12).
- The source's synonyms carry over to the target (Edge Case `統合とシノニム`).
  The source's original name also becomes a synonym of the target, so the merge
  that comes with adding a synonym writes the same as an ordinary merge.
- To use the source's name for another tag after a merge, remove that synonym.
- The targets of adding and removing are the video `id`s the screen sends,
  resolved to the `content_key`s of videos currently in the library. `id`s that
  do not resolve (videos removed in the meantime) are skipped, and the response
  returns the number applied.
- When adding by name, the name is looked up as in §3 and the tag is created in
  the same transaction when missing (requirement 1).

## 5. Video counts

The count on the management page joins `video_tags` with `videos.content_key`
and counts only videos currently in the library (`registeredVideoCondition`)
(Edge Case `ファイルが見えなくなった動画`). Tags with 0 videos are listed too
(requirement 8).

## 6. Tag filter

Each selected tag adds this condition to the list's videos with AND
(requirement 5):

```sql
exists (select 1 from video_tags vt where vt.content_key = videos.content_key and vt.tag_id = ?)
```

The primary key `(content_key, tag_id)` is the index for this condition. A
nonexistent `tag_id` is dropped from the conditions, and the list response
says which were dropped
([contracts/tags-api.md §5](contracts/tags-api.md#5-list-filter-and-select-all)).

## 7. Matching tag names in the search box

#195's search builds each term's condition against one location row
([013 data-model.md §3](../013-library-search/data-model.md#3-search_key-rules),
`searchExprCondition` in `internal/store/search.go`). The condition for one term
widens to this OR (requirement 10):

```text
(current location condition: location_search_fts MATCH or instr on search_key)
or exists (select 1 from video_tags vt join tag_names tn on tn.tag_id = vt.tag_id
           where vt.content_key = <content_key of that location's video>
             and instr(tn.search_key, ?) > 0)
```

- An excluded term negates the whole OR. Only videos whose title, location and
  tag names all lack the term remain.
- The tag name side is checked with `instr` regardless of term length. Tag names
  are expected to number in the hundreds at most, and a video has a few tags, so
  no full-text index is added.
- The matching form is `domain.FoldForMatch`, the same as for titles. Both the
  original name rows and the synonym rows are checked, so synonyms match too.
- `tag_names.search_key` is written in the same transaction when a name row is
  added (create, add synonym, create on add) and when a rename rewrites
  `name`. Moving a row's `tag_id` on merge does not change the key.
- At startup, at the same point as `RefreshSearchKeys`, `tag_names` rows whose
  `search_version` is below the current version are rebuilt. When the matching
  rule changes and the version goes up, tag name keys are rebuilt along with the
  location keys.
- A tag belongs to the video, so #195's rule "do not match when terms are
  satisfied by different locations" does not apply. The same tag name matches
  from any location row.
