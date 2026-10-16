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
| Child folders | The folder's children as summaries (`SmartFolderSummary`: id, parent id, name, favorite), by `title_key` then id; a guest gets those [shown](#contents) to it ([R-14](research.md#r-14-folder-cards-carry-a-summary-their-values-are-read-on-screen-without-group-collapsing)) |
| Missing reference | A rule on the path names an id that does not exist: no item, no matching video, but the child folders ([R-9](research.md#r-9-merges-rewrite-rules-a-deleted-reference-makes-the-rule-match-nothing)) |
| Play order | The items in order, a group card replaced by its play order (048), each video at its first appearance; child folders are not played |
| Matching videos | The viewer's shown video of each content key that matches every rule on the path, each condition tested on the video itself (watch state, playable and favorite included), with no group step. The same videos as the play order, except for a group whose videos are mixed under a watch state, playable only or favorites only rule: the item query takes or leaves it as one item (048 Library items), this set takes its matching videos |
| Card values (the folder's page) | `LibraryGroup`'s values over the play order: count, sums, dates, watch state, previews, open video |
| Card values (the cards read) | The same values over the matching videos: `videoCount` their number, `sizeBytes` and `durationMs` sums, `addedAt` and `lastPlayedAt` the latest, `watchedCount` and `watchState` per video, `previews` and `openVideoId` the first in the folder's `sort`, `tags` as `LibraryGroup`; no `videoIds` ([R-14](research.md#r-14-folder-cards-carry-a-summary-their-values-are-read-on-screen-without-group-collapsing)) |
| Shown | To the owner always; to a guest when the folder's play order is not empty, by the same test on every read (the folder's page, the library, a parent's child folders and the cards read), so a card a guest sees opens ([R-8](research.md#r-8-an-empty-folder-stays-and-is-shown-to-the-owner-only)). The matching videos do not decide it: under a watch state, playable only or favorites only rule a mixed group has matching videos and an empty play order. A guest's library, child folders and cards read test each folder with one existence query for the first item of its play order; the owner's reads skip the test |

## Library items

The library item query gains root folders (`parent_id is null`) as items of
kind `smartFolder` when the query has no search and no filter other than
favorites only; with favorites only, the favorited folders at any depth
([R-7](research.md#r-7-root-smart-folders-are-cards-in-the-unfiltered-library)).
An item carries the folder's summary (`SmartFolderSummary`) and no card
value
([R-14](research.md#r-14-folder-cards-carry-a-summary-their-values-are-read-on-screen-without-group-collapsing));
the cursor carries the kind. A guest's library leaves out a folder that is
not [shown](#contents) to the guest
([R-8](research.md#r-8-an-empty-folder-stays-and-is-shown-to-the-owner-only)).
No video or group item changes.

A folder's sort value is read by one query per folder over its matching
videos ([Contents](#contents)), with no group step:

| Sort | A folder's sort value |
| --- | --- |
| Added, modified, created, last played | The latest among its matching videos, as a group card's is among its members; none with no matching video |
| Length, size | The sums over its matching videos; none with no matching video |
| Title | The folder's `title_key` |
| Favorited | Its `smart_folder_favorites.favorited_at` |

A folder with no value (the owner's empty folder under any sort but title
and favorited) is placed after every item with a value in either direction,
with the folder id as the tie-breaker, as the list places a video without a
length or a play record (`listOrder.nullable`); the cursor carries the
absence as it does for those videos.

## Cards read

`GET /api/smart-folders/cards` takes 1 to 60 folder ids and returns the card
values of the cards read ([Contents](#contents)) for each folder that exists
and is shown to the viewer, in the order asked; an id that names no folder
or a folder a guest may not see is left out. Each folder costs one query
over its matching videos
([R-13](research.md#r-13-a-read-budget-at-20000-videos-and-50-smart-folders)).

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
| Probe results applied to a video (`ApplyProbe`: length, playable) | Every folder |
| A video's display name set, cleared or changed with its work info (`SetDisplayName`, `SetDisplayNames`, `SetDetails`), when the name changes | Every folder |
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
