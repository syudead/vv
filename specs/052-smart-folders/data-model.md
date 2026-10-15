# Data model: Smart folders whose contents follow a saved condition

Parent Issue: #1139. The rest of the model is unchanged: existing tables are in
[internal/store/migrations/](../../internal/store/migrations/), groups in
[048 data-model](../048-groups/data-model.md).

## Migration

One migration, `000NN_smart_folders.sql`, takes the next free number on
`main` when the unit lands (`00047` is the last today, and #1136 plans
`00048`).

```sql
-- User data (research.md R-1). No foreign key to videos.
create table smart_folders (
    id         integer primary key autoincrement,
    parent_id  integer references smart_folders (id) on delete cascade,
    name       text    not null check (length(name) between 1 and 100),
    title_key  text    not null,          -- domain.NaturalSortKey(name)
    rule       text    not null,          -- JSON, domain.SmartRule
    sort       text    not null,          -- a VideoSort other than random
    owner_kept integer not null default 1 check (owner_kept in (0, 1)),
    created_at integer not null,          -- Unix ms
    updated_at integer not null,          -- Unix ms
    check (parent_id is null or parent_id <> id)
);
create index smart_folders_parent_idx on smart_folders (parent_id);

create table smart_folder_favorites (
    smart_folder_id integer primary key references smart_folders (id) on delete cascade,
    favorited_at    integer not null
);

-- playlist_items is rebuilt with smart_folder_id integer references smart_folders (id)
-- on delete cascade; exactly one of content_key, group_id and smart_folder_id is set.
```

`smart_folders` and `smart_folder_favorites` are user data: ARCHITECTURE.md
("User data survives a rebuild") and the table in running-vv.md gain them.
Down drops both and rebuilds `playlist_items` without its smart folder rows.

## `smart_folders.rule`

A JSON object. Every member is optional; an absent member does not constrain,
and an empty object matches every video. `domain.SmartRule` parses and
validates it ([R-3](research.md#r-3-a-rule-is-the-library-filters-fields-contents-are-the-filtered-library)).

| Member | Type | Library filter field it equals |
| --- | --- | --- |
| `q` | string, at most 500 characters | Search text, owner semantics ([R-11](research.md#r-11-the-rule-reads-the-owners-facts-visibility-is-the-viewers)) |
| `watch` | `unwatched`, `inProgress`, `watched` | Watch state |
| `playable` | `true` | Playable only |
| `favorite` | `true` | Favorites only |
| `tags` | array of tag ids, 1 to 16 | Every tag attached (manual or from a folder) |
| `tagKind` | tag kind id | Any tag of that kind (#1136) |
| `person`, `organization` | id | Credited in any role |
| `addedWithin` | integer, 1 to 36,500 | Added within that many days ([R-4](research.md#r-4-the-library-filter-gains-added-within-and-length)) |
| `minDuration`, `maxDuration` | integer seconds, 0 to 360,000 | Length bounds, inclusive; `min` ≤ `max` |

`SmartRule.Query()` returns the `VideoQuery` whose filter fields equal the
rule's; `SmartRule.Refs()` lists its references as `(kind, id)` for the
missing-reference check and for merges.

## Contents

| Read | Rule |
| --- | --- |
| Effective rule | The folder's rule and each ancestor's; the store applies each one's filter conditions ([R-5](research.md#r-5-a-childs-effective-rule-is-every-rule-on-its-path-anded)) |
| Items | `ListLibrary` for the effective rule, in the folder's `sort`, for the viewer ([R-3](research.md#r-3-a-rule-is-the-library-filters-fields-contents-are-the-filtered-library)); on the first page, child folders by `title_key` come first |
| Missing reference | A rule on the path names an id that does not exist: no item but the child folders ([R-9](research.md#r-9-merges-rewrite-rules-a-deleted-reference-makes-the-rule-match-nothing)) |
| Play order | The items in order, a group card replaced by its play order (048), each video at its first appearance; child folders are not played |
| Card values | `LibraryGroup`'s values over the play order: count, sums, dates, watch state, previews, open video |
| Shown | To the owner always; to a guest when the play order is not empty ([R-8](research.md#r-8-an-empty-folder-stays-and-is-shown-to-the-owner-only)) |

## Library items

The library item query gains root folders (`parent_id is null`) as items of
kind `smartFolder` when the query has no search and no filter other than
favorites only; with favorites only, the favorited folders at any depth
([R-7](research.md#r-7-root-smart-folders-are-cards-in-the-unfiltered-library)).
Their sort values come from their card values as a group card's do; the
cursor carries the kind. No video or group item changes.

## Edits

| Operation | Rule |
| --- | --- |
| Create | Name (048's name rule), rule, sort (default `addedDesc`), optional parent; `owner_kept = 1` |
| Change | Any of name, rule, sort, parent; `updated_at` and `owner_kept = 1` |
| Move | A parent that is the folder or one of its descendants is `ErrSmartFolderCycle` with nothing written; no parent makes it a root |
| Delete | Deletes the folder and, through the foreign key, its descendants, their favorites and their playlist items |
| Missing target | A parent id that does not exist is `ErrSmartFolderNotFound` |

A rule naming an id that does not exist is accepted and reported as missing.

## Merges

| Change | Effect on rules |
| --- | --- |
| Tag merge (`mergeTagsInto`) | Each rule's source tag ids become the target id, deduplicated, in the merge's transaction |
| Person or organization merge (`CreditStore.Merge`) | The source id becomes the target id in the same way |
| Tag, tag kind, person or organization deleted | Rules are not written; the reference is missing |

## Notices

After commit, each write below publishes one `SmartFoldersChanged`
([R-12](research.md#r-12-writes-that-can-change-a-match-publish-smartfolderschanged)).

| Write | Folders named |
| --- | --- |
| A folder created, changed, moved or deleted, its favorite set, or a rule rewritten by a merge | The folder and its descendants (and the old and new parent for a move) |
| A tag attached to or detached from a video, by hand, in a batch or by auto-tagging | Every folder |
| A tag renamed, deleted or merged, a synonym changed, or a tag's kind set or cleared (#1136) | Every folder |
| A person or organization credited, uncredited, renamed, deleted or merged | Every folder |
| A video favorite set or cleared | Every folder |
| A progress save whose `domain.WatchState` differs from the stored one | Every folder |
| A video becomes listed or leaves the library, is bundled or unbundled, or a group changes (`GroupsChanged`) | Every folder |

Nothing is published when no folder exists.

## Rules

| Rule | Enforced in |
| --- | --- |
| `rule` parses as a valid `domain.SmartRule` | `domain.ParseSmartRule` on every write; `internal/store/invariants_test.go` |
| `sort` is a valid `VideoSort` other than `random` | `domain.ValidateSmartSort` |
| No folder is its own ancestor | `domain.ValidateSmartFolderParent` in the write; `internal/store/invariants_test.go` |

## What does not change

`groups`, `group_items` and `group_exclusions`: a smart folder takes no video
off the library's top level and holds no items
([R-1](research.md#r-1-a-smart-folder-is-its-own-entity-not-a-group)).
