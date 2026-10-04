# Data model: Tentative tags and rejected names

Parent Issue: #589.

The rest of the model is unchanged. Table definitions are in
[internal/store/migrations/](../../internal/store/migrations/); the tag tables,
name rules and write rules are in
[specs/014-video-tags/data-model.md](../014-video-tags/data-model.md);
folder-derived tags are in
[specs/017-folder-groups/data-model.md, Folder-derived tags](../017-folder-groups/data-model.md#folder-derived-tags);
the data categories are in [ARCHITECTURE.md](../../ARCHITECTURE.md) "Rebuildable
and user data". This file records only the column and table this feature adds and
the rules that read and write them. Tables not named here do not change
(`tag_names` and `video_tags` stay as they are, and the SQL for attaching,
detaching, filtering, search and counts does not change).

## Migration

Added as `00022_tentative_tags.sql` (the last migration on `main` is
`00021_video_overrides.sql`).

```sql
-- Tentative tags (specs/031-tentative-tags/research.md R-1). 1 for a tag newly created by
-- automatic tagging; 0 for a tag created by hand or before this migration. Confirming only
-- rewrites it to 0 and changes neither names nor assignments.
alter table tags add column tentative integer not null default 0 check (tentative in (0, 1));

-- Rejected names (R-2). Rejecting a tentative tag remembers its canonical name, and later
-- tentative creates skip it. User data that is neither a tag nor an assignment; names match
-- exactly, like tag_names.name.
create table rejected_tag_names (
    name       text    primary key,
    created_at integer not null
) without rowid;
```

Down drops `rejected_tag_names` and drops `tags.tentative` with
`alter table … drop column`.

Invariants (added to `internal/store/invariants_test.go`):

| Invariant | Source |
| --- | --- |
| A tag with `tentative = 1` has no `tag_names` row with `canonical = 0` (a tentative tag has no synonyms) | [R-4](research.md#r-4-edits-to-a-tentative-tag-rename-synonym-merge-target-confirm-it-in-the-same-transaction) |
| No value appears in both `rejected_tag_names.name` and `tag_names.name` | [R-3](research.md#r-3-a-transaction-that-writes-a-name-to-tag_names-removes-it-from-rejected_tag_names) |

Category: both are user data that cannot be rebuilt. Add `rejected_tag_names` to
the list of tag tables in ARCHITECTURE.md.

## `rejected_tag_names`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `name` | text | No | One rejected name, in the form `domain.NormalizeTagName` produces (a rejected tag's canonical name is always stored in that form) |
| `created_at` | integer | No | When the name was rejected |

- Only two readers: a create with `tentative` true ([Write rules](#write-rules)) and the tag management
  page's list ([contracts/screen-api.md, Rejected names](contracts/screen-api.md#rejected-names)).
  The table has no matching key (`search_key`) and is not a target of name search.
- The list is in natural name order (`domain.SortTagNames`).

## Write rules

These rows extend the table in [014 data-model.md, Write rules](../014-video-tags/data-model.md#write-rules). Every operation runs in one transaction
and leaves nothing behind if it fails partway.

| Operation | Writes |
| --- | --- |
| Tentative create (`add` or `replace` with `tentative` true, when the name matches no tag's name or synonym) | If the name is in `rejected_tag_names`, skip it (do not create, do not attach, return it as a skipped name). Otherwise one row in `tags` with `tentative = 1` and one row in `tag_names` with `canonical = 1`. When one request attaches the same name to several videos, only one tag is created (edge case) |
| Manual create (create in the screen, create during attach-by-name, the API with `tentative` false, group-to-tag conversion) | As today (`tentative = 0`). Removes the name from `rejected_tag_names` in the same transaction |
| Attaching a name that matches an existing tag (with `tentative` true or false) | As today: attaches that tag. Does not change `tentative` (requirements 2 and 3; a manual attach of a tentative tag leaves it tentative) |
| Rename | As today. When the name changes, sets `tentative = 0` and removes the new name from `rejected_tag_names` in the same transaction. A rename to the current name changes nothing |
| Synonym registration (name n on tag T) | As today. When n is added (including with an accepted merge), sets T to `tentative = 0` and removes n from `rejected_tag_names`. If n is already a synonym of T, nothing changes |
| Merge (source X → target Y) | As today. Sets Y to `tentative = 0`. X is deleted, tentative or confirmed |
| Confirm (`ConfirmTag`) | `tentative = 0`. If already 0, nothing changes. `tag_names` and `video_tags` do not change |
| Reject (`RejectTag`) | `domain.ErrTagNotTentative` unless `tentative = 1`. Reads the canonical name, deletes the `tags` row (`tag_names` and `video_tags` cascade), and inserts the canonical name into `rejected_tag_names` with `insert or ignore` |
| Delete (`DeleteTag`) | As today. Remembers no name, tentative or confirmed (the screen does not offer delete for tentative tags; requirement 10) |
| Removing a rejected name (`ForgetRejectedTagName`) | `delete from rejected_tag_names where name = ?`. Nothing changes if the name is absent (edge case) |

- The tentative-create decision and the rejected-name check run in the same
  transaction as the name lookup (`lookupTagName`). Write transactions run one at
  a time, so they cannot race a reject
  ([R-6](research.md#r-6-rejection-and-a-tentative-attach-of-the-same-name-rely-on-sqlite-write-serialization)).
- The two entry points that write a name to `tag_names` (`insertTagName` and the
  rename `update`) carry the delete from `rejected_tag_names`. A tentative create
  also passes through them, but after the skip check, so there is nothing to
  delete (R-3).
- `remove` does nothing for a name that matches no tag, as today. It accepts
  `tentative` but does not read it.

## Values added to `domain`

| Value | Content |
| --- | --- |
| `TagRef.Tentative` | `bool`. Also carried by `VideoTag` (which embeds `TagRef`) and `TagSummaryItem.Tag` |
| `Tag.Tentative` | `bool`. One tag on the tag management page |
| `ErrTagNotTentative` | An attempt to reject a tag that is not tentative. `tag_not_tentative` (409) in the API |
| `VideoTagsOutcome{Items []VideoTagsResult, SkippedNames []string}` | Result of a bulk operation. `SkippedNames` are the names skipped because they matched a rejected name (normalized, in `tags` order, without duplicates; an empty array when none) |

`NormalizeTagName` and `SortTag*` do not change. Tentative tags follow the same
name rules as confirmed tags.

## Store operations

Added to `TagStore`. All of them use only the shared SQLite connection and publish
no domain events (tag changes have no side effects, as the `TagStore` paragraph
in ARCHITECTURE.md states).

| Operation | What one transaction does |
| --- | --- |
| `ApplyVideoTags(ctx, videos, action, names, tentative bool) (VideoTagsOutcome, error)` | Adds `tentative` to the current `ApplyVideoTags`. `resolveTagNames` receives `tentative`, creates each missing name by the "tentative create" or "manual create" rule in [Write rules](#write-rules), and collects skipped names. Skipped names do not enter the replacement set (edge case "`replace` and `tentative`") |
| `ConfirmTag(ctx, id) (Tag, error)` | Confirm in [Write rules](#write-rules). `ErrTagNotFound` when absent |
| `RejectTag(ctx, id) (name string, error)` | Reject in [Write rules](#write-rules). `ErrTagNotFound` when absent, `ErrTagNotTentative` when not tentative |
| `ListRejectedTagNames(ctx) ([]string, error)` | Natural name order |
| `ForgetRejectedTagName(ctx, name) error` | Removal in [Write rules](#write-rules). `name` is normalized with `NormalizeTagName` before matching; input that cannot be normalized is matched as is (the same handling as `RemoveSynonym`) |

Changes to existing operations:

- `ListTags`, `tagByID`, `lookupTagName` (`nameLookup` gains the tentative flag),
  `AttachTagByID`, `DetachTag` and `AttachTagByName` (which use
  `canonicalNameByTagID`), `TagsByContentKeys` (joins `tags`), `Summary`, and the
  external list (shares `ListTags`) read `tentative` and set `TagRef.Tentative`
  and `Tag.Tentative`.
- `RenameTag`, `AddSynonym` and `mergeTagInto` perform the confirm in [Write rules](#write-rules).
- `insertTagName` and the `update` in `RenameTag` perform the delete from
  `rejected_tag_names` in [Write rules](#write-rules). Group-to-tag conversion in `FolderGroupStore` goes
  through `findOrCreateTag` → `insertTag` → `insertTagName`, so it follows the
  manual-create rule with no change.
- The `Tags` interface declared by `internal/httpapi` (`router.go`) gains the
  operations above, and the signature of `ApplyVideoTags` changes. The wiring in
  `cmd/mdm` does not change (the same `store.TagStore` satisfies it).

## What does not change

- Attaching and detaching tags on videos (`AttachTagByID`, `DetachTag`,
  `applyManualTags`), tag filtering ([014 data-model.md, Tag filter](../014-video-tags/data-model.md#tag-filter)), tag-name matching in the search
  field ([014 data-model.md, Matching tag names in the search box](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box)), counts ([014 data-model.md, Video counts](../014-video-tags/data-model.md#video-counts)), and folder-derived tags ([017 data-model.md, Folder-derived tags](../017-folder-groups/data-model.md#folder-derived-tags)). None of them
  reads `tags.tentative` (requirement 4).
- Responses to guests (`Video.tags` stays an empty array;
  [guest-api.md](../016-single-account-auth/contracts/guest-api.md)).
- `SearchKeyVersion`. `rejected_tag_names` has no matching key.
