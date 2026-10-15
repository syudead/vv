# Data model: Smart folders whose videos follow a saved condition

Parent Issue: #1139. The rest of the model is unchanged: the group tables are
in [internal/store/migrations/00043_groups.sql](../../internal/store/migrations/00043_groups.sql)
and [048 data-model](../048-groups/data-model.md).

## Migration

One migration, `000NN_smart_folders.sql`, takes the next free number on
`main` when the unit lands (`00047` is the last today, and #1136 plans
`00048`).

```sql
alter table groups add column rule text;               -- null for a group; JSON for a smart folder
alter table groups add column sort text;               -- null for a group; a VideoSort other than random
alter table groups add column owner_kept integer not null default 0
    check (owner_kept in (0, 1));
update groups set owner_kept = 1 where origin_folder is null;
-- A smart folder is never automatic, and has a sort exactly when it has a rule.
-- SQLite cannot add a table check with alter table; the invariants test enforces both.
```

The down migration drops the three columns after deleting every row with a
non-null `rule`. `groups` stays user data; ARCHITECTURE.md and running-vv.md
need no change.

## `groups.rule`

A JSON object. Every member is optional; an absent member does not constrain.
An empty object matches every video. `domain.SmartRule` parses and validates
it ([R-3](research.md#r-3-the-rule-is-the-library-filter-combined-with-and-built-by-the-same-sql)).

| Member | Type | Meaning, as the library filter's field of the same meaning |
| --- | --- | --- |
| `q` | string | Search text, owner semantics ([R-11](research.md#r-11-the-rule-reads-the-owners-facts-visibility-is-the-viewers)) |
| `watch` | `unwatched`, `inProgress`, `watched` | Watch state |
| `playable` | `true` | Playable only |
| `favorite` | `true` | Favorites only |
| `tags` | array of tag ids, 1 to 16 | Every tag attached (manual or from a folder) |
| `tagKind` | tag kind id | Any tag of that kind (#1136) |
| `person` | person id | Credited in any role |
| `organization` | organization id | Credited in any role |
| `addedWithin` | integer, 1 to 36,500 | Added within that many days ([R-4](research.md#r-4-the-library-filter-gains-added-within-and-length)) |
| `minDuration`, `maxDuration` | integer seconds, 0 to 360,000 | Length bounds, inclusive; `min` ≤ `max` |

`domain.SmartRule.Query()` returns the `VideoQuery` whose filter fields equal
the rule's. `domain.SmartRule.Refs()` lists its references as `(kind, id)`
for the missing-reference check and for merges.

## Members

A smart folder's members, for a viewer
([R-5](research.md#r-5-hand-added-videos-are-items-hand-removed-videos-are-exclusions)):

| Source | Rule |
| --- | --- |
| Matched videos | Shown videos (bundle representatives) that pass `rule` with the owner's facts, whose user key has no `group_exclusions` row for the folder; none when `rule` has a missing reference ([R-9](research.md#r-9-merges-rewrite-rules-a-deleted-reference-makes-the-rule-match-nothing)) |
| Video items | `group_items` with a `content_key`, shown or not by 048's rule |
| Child groups | `group_items` with a `child_id`, by `position` |
| Viewer cut | Every video the viewer may not see is left out (048 Visibility) |

`groupDescendantsCTE` gains the matched videos as direct members of their
folder, so every read built on it (library items, card values, the group page,
`?group`, playlists, `LibraryIDs`) sees them. Play order
([R-6](research.md#r-6-child-groups-keep-their-positions-videos-follow-the-folders-sort)):
the `sort_key` of a child group item is its position as today; every video
member (matched or item) sorts after all child groups, by the folder's `sort`
with the video id as tie-breaker. A video that is both matched and an item
appears once.

The group page marks each video member `manual` when it has an item row and
the video does not match, so the owner sees what was added by hand; the
excluded videos are read through their own route
([contracts/screen-api.md](contracts/screen-api.md)).

## Edits

| Operation | Rule in a smart folder |
| --- | --- |
| Create | `name`, `rule`, `sort` (default `addedDesc`); `owner_kept = 1`; items from the request become video items or child groups |
| Change rule or sort | Validated by `domain.SmartRule`; `updated_at` and `owner_kept = 1` |
| Add a video | Deletes its exclusion; inserts an item when it does not match |
| Remove a video | Deletes its item, if any, and writes its exclusion |
| Remove the last member, or an organize move that empties it | The folder stays ([R-8](research.md#r-8-an-empty-smart-folder-stays-and-is-shown-to-the-owner-only)) |
| Move or organize reorder of a video | `409 smart_folder_sorted` |
| Organize group with a smart folder as `from` | `409 smart_folder_sorted`: the new group would need the target video's position |
| Organize merge of a smart folder into another group | `409 smart_folder_merge`: the rule would be lost; merging a group into a smart folder adds its items by hand |
| Dissolve, turn into a tag, favorite, playlist, nesting | As in a group (048) |

Every screen API write above, and every group write of 048 and 050, sets
`owner_kept = 1` on the groups it changes; `syncFolderGroups` and
`rekeyGroupItems` do not ([R-10](research.md#r-10-owner_kept-marks-every-group-the-owner-made-or-edited)).

## Merges and deletions

| Change | Effect on rules |
| --- | --- |
| Tag merge (`mergeTagsInto`) | Each rule's source tag ids become the target id, deduplicated, in the merge's transaction |
| Person or organization merge (`CreditStore.Merge`) | The source id becomes the target id in the same way |
| Tag, tag kind, person or organization deleted | Rules are not written; the reference is missing and the rule matches nothing until the owner changes it |

A rewrite publishes `GroupsChanged` for the folders it changed and their
ancestors.

## Rules

| Rule | Enforced in |
| --- | --- |
| `rule` and `sort` are both null or both set | `internal/store/invariants_test.go` |
| A group with `rule` has a null `origin_folder` | `internal/store/invariants_test.go` |
| `rule` parses as a valid `domain.SmartRule` | `domain.ParseSmartRule` on every write; `internal/store/invariants_test.go` |
| `sort` is a valid `VideoSort` other than `random` | `domain.ValidateSmartSort` |
