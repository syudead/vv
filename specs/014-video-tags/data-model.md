# Data model: video tags

Parent Issue: #193.

The source of truth for existing tables is
[internal/store/migrations/](../../internal/store/migrations/). The split between "rebuildable
index" and "user data that cannot be recreated" is in the header of
[00002_core.sql](../../internal/store/migrations/00002_core.sql) and in
[ARCHITECTURE.md](../../ARCHITECTURE.md). This document covers only the tables this feature adds
and the rules that change them. Existing tables do not change.

All three added tables are "user data that cannot be recreated". A rescan or a media folder change
must not delete them, so they have no foreign key to `videos`. Like `playback_progress`, they link
to a video by `content_key` (Requirement 9).

## 1. Migration

A new `00008_tags.sql`. The last migration on `main` is #195's `00007_location_search.sql`.

```sql
-- The tag itself. Its names live in tag_names.
-- autoincrement stops a deleted tag's id from being reused by another tag,
-- so an id left in an old tab or URL never points at an unrelated new tag.
create table tags (
    id         integer primary key autoincrement,
    created_at integer not null
);

-- Tag names and synonyms share one namespace.
-- name uses the default BINARY collation: two names match only when spelled exactly alike (Requirement 4).
create table tag_names (
    name      text    primary key,
    tag_id    integer not null references tags (id) on delete cascade,
    -- 1 is the displayed original name, 0 is a synonym.
    canonical integer not null check (canonical in (0, 1)),
    -- Match key for the search box. Go writes domain.FoldForMatch(name) (§7).
    search_key     text    not null default '',
    -- Version of the rule that built search_key. Same domain.SearchKeyVersion as video_locations.search_version.
    search_version integer not null default 0
) without rowid;

-- A tag has exactly one original name.
create unique index tag_names_canonical_idx on tag_names (tag_id) where canonical = 1;
create index tag_names_tag_idx on tag_names (tag_id);

-- Mapping from video content to tags. User data, so no foreign key to videos.
create table video_tags (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    created_at  integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- Used for per-tag counts and for reassigning rows on merge and delete.
create index video_tags_tag_idx on video_tags (tag_id, content_key);
```

Down drops the three tables.

Invariants (added to the checks in `internal/store/invariants_test.go`):

- Every `tags` row has exactly one `tag_names` row with `canonical = 1`.
- Every `video_tags.tag_id` and `tag_names.tag_id` points at an existing `tags` row (guaranteed
  by the foreign keys; `foreign_keys(on)` is already in the DSN).

## 2. Name rules

One function in `internal/domain` normalizes the input into a name.

1. If the input as received contains a control character (Unicode general category Cc,
   including line feed and tab) anywhere, it is an error (`invalid_request`). A tag name with a
   line break does not fit on one card line or in a tooltip. The search side turns line breaks
   into spaces, so the name would also never match a search (§7).
   - The check runs on the input before trimming. Line feed, tab and CR are also Unicode
     White_Space, so running step 2 first would strip the control characters around `"旅行\n"`
     or `"\t旅行"`, and an input that must be rejected would become a different name. The screen
     does not take in a pasted line break and shows the reason
     ([ui-design.md "Combobox"](ui-design.md#combobox)), so the API rejects the same input.
   - Rejected: checking for control characters after trimming. Leading and trailing line breaks
     and tabs would disappear silently, and the API and the screen would handle the same input
     differently.
2. Trim leading and trailing whitespace (Unicode White_Space). Inner whitespace stays. After
   step 1, the only White_Space left is non-control characters such as the space, the
   ideographic space and the no-break space.
3. An empty result is an error (`invalid_request`).
4. More than 100 code points is an error (`invalid_request`). Without a limit, one name could
   grow the card line, the suggestions and the request bodies outside the URL without bound. The
   limit matches the search term's 100, so users remember one limit.
5. There is **no** Unicode normalization and **no** case folding. `Anime` and `anime`, and
   full-width `Ａ` and half-width `A`, are different names (Requirement 4, Acceptance
   criterion 6).

## 3. Name lookup

A tag is looked up by name with one lookup of `tag_names` by `name`. The original name and a
synonym reach the same tag (Requirement 7). The name shown on screen is always the
`canonical = 1` row.

## 4. Write rules

Every operation runs in one transaction and leaves nothing behind if it fails part way (Edge case
"Partial failure of a bulk operation or a merge").

| Operation | Writes |
| --- | --- |
| Create | One row in `tags` and one row in `tag_names` with `canonical = 1`. If the name exists, `ErrTagNameTaken` |
| Rename | Rewrite `name` in the tag's `canonical = 1` row. The current name changes nothing. If the new name exists (even as the tag's own synonym), `ErrTagNameTaken` |
| Delete | Delete the `tags` row. `tag_names` and `video_tags` cascade. Tags on videos not now in the library are also deleted |
| Merge (source X → target Y) | `insert or ignore into video_tags select content_key, Y, … from video_tags where tag_id = X`; move X's names (original name and synonyms) to `tag_id = Y`, `canonical = 0`; delete X. X's original name becomes a synonym of Y |
| Add synonym (name n to tag T) | If n does not exist, add one row with `canonical = 0`. If n is already a synonym of T, nothing changes. If n is T's original name, `ErrTagNameTaken`. If n is a synonym of another tag S, `ErrTagNameTaken` (naming S). If n is the original name of another tag S: when the approved merge source `id` is not S (including when there is none), `ErrTagMergeRequired`; when it is S, merge S → T (the merge makes n a synonym of T) |
| Remove synonym | Delete that `canonical = 0` row. If n is not a synonym of T, nothing changes |
| Add to videos | `insert or ignore` per target `content_key`. An existing tag changes nothing |
| Remove from videos | `delete` per target `content_key`. A missing tag changes nothing |

- On a merge, content that had both tags ends up with one row through `insert or ignore`
  (Acceptance criterion 12).
- The source's synonyms move to the target (Edge case "Merge and synonyms"). The source's original
  name also becomes a synonym of the target, so a merge triggered by adding a synonym writes the
  same rows as a plain merge.
- To use the source's name for another tag after a merge, remove that synonym.
- The targets of an add or remove are the video `id` values the screen sends, resolved to the
  `content_key` of videos now in the library. An `id` that does not resolve (a video removed in
  the meantime) is skipped, and the response returns the number of videos applied.
- When an add passes a name, the tag is looked up as in §3 and, if missing, created in the same
  transaction (Requirement 1).

## 5. Video counts

The count on the management page joins `video_tags` with `videos.content_key` and counts only
videos now in the library (`registeredVideoCondition`) (Edge case "Videos whose files are no
longer visible"). Tags with 0 videos are listed too (Requirement 8).

## 6. Tag filter

Each selected tag adds this condition to the list's videos with AND (Requirement 5).

```sql
exists (select 1 from video_tags vt where vt.content_key = videos.content_key and vt.tag_id = ?)
```

The primary key `(content_key, tag_id)` serves as the index for this condition. A `tag_id` that
does not exist is dropped from the conditions, and the list response reports which ones were
dropped ([contracts/tags-api.md §5](contracts/tags-api.md#5-list-filter-and-select-all)).

## 7. Tag name matching in the search box

#195's search builds a per-term condition against one location row
([013 data-model.md §3](../013-library-search/data-model.md#3-search_key-rules),
`searchExprCondition` in `internal/store/search.go`). The condition for one term widens to this OR
(Requirement 10).

```text
(current location condition: MATCH on location_search_fts, or instr on search_key)
or exists (select 1 from video_tags vt join tag_names tn on tn.tag_id = vt.tag_id
           where vt.content_key = <content_key of that location's video>
             and instr(tn.search_key, ?) > 0)
```

- An excluded term negates the whole OR. Only videos that contain the term in none of title,
  location and tag names remain.
- The tag name side uses `instr` for every term length. Tag names are expected to number a few
  hundred at most and a video has a few tags, so no full-text index is added.
- The match form is `domain.FoldForMatch`, the same as for titles. Both original-name and synonym
  rows are checked, so a synonym also matches.
- `tag_names.search_key` is written in the same transaction when a name row is added (create, add
  synonym, create on add) and when a rename rewrites `name`. Moving a row's `tag_id` on a merge
  does not change the key.
- At startup, at the same point as `RefreshSearchKeys`, `tag_names` rows whose `search_version`
  is below the current version are rebuilt. When the match-form rule changes and the version goes
  up, the tag name keys are rebuilt together with the location keys.
- Tags belong to the video, so #195's rule "no match when the terms are satisfied by different
  locations" does not apply. The same tag name matches from any location row.
