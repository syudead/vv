# Data model: One ordered list of names

The rest of the model is unchanged:
[`00042_work_info.sql`](../../internal/store/migrations/00042_work_info.sql)
creates the two name tables, and the entity, external-ID and profile tables
stay as they are.

## Migration

`00098_credit_name_positions.sql` changes `people_names` and
`organization_names` the same way, in this order:

1. Adds `position integer not null default 0`.
2. Sets `position` to 0 on the row with `canonical = 1` and numbers the other
   rows of each entity from 1, ordered by `name collate vv_alias_order`, which
   is the order the screens show aliases today. `sort_key` cannot give that
   order: it is NFKC-folded and kana-folded, so `Ｂ` sorts before `C` by
   `sort_key` but after it on the screens.
3. Creates the unique index `<table>_position_idx (<id>, position)`.
4. Drops the partial index `<table>_canonical_idx` and the `canonical` column.

`vv_alias_order` is a SQLite collation the store registers in Go, backed by
the comparison `domain.SortAliases` makes today (`CompareNatural`, ties by
`strings.Compare`). That comparison stays in `domain` as the collation's
function after `SortAliases` goes, and the collation stays registered so that a
database migrated later gets the same order, as `vv_natural_sort_key` does for
migration 43.

Down restores `canonical` as `position = 0`, recreates the partial index and
drops `position` and its index. The search-dirty triggers of migration 62 stay
in place because the table is altered, not recreated.

## `people_names` and `organization_names`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `person_id` / `organization_id` | integer | no | The entity; cascades on delete (unchanged) |
| `name` | text | no | One spelling; the primary key with the entity id (unchanged) |
| `position` | integer | no | 0 is the display name; the rest are dense and in the owner's order (new, replaces `canonical`) |
| `language` | text | yes | BCP 47 tag on any row, including the display name (was null on the canonical row) |
| `search_key`, `sort_key`, `search_version` | text, text, integer | no | Unchanged |

**Relationships**: Unchanged. `(id, position)` is unique, so an entity has at
most one display name.

## Rules

| Rule | Enforced in |
| --- | --- |
| An entry's `name` is trimmed, not blank, at most 200 code points, without control characters | `domain.NormalizeCredit` |
| An entry's `language` is empty or BCP 47 syntax of at most 35 characters | `domain.NormalizeCredit` |
| Two entries never share a spelling; the later one is the duplicate | `domain.NormalizeCredit`; the primary key as the final guard |
| At most 50 entries, the display name counted, checked when a create or patch carries `names` and after a merge; a patch without `names` keeps a stored list over the limit | `domain.NormalizeCredit`; `checkMergedCreditLimits` after a merge |
| An organization has at least one entry; a person with none is provisional | `domain.NormalizeCredit`, given the kind |
| Positions are 0 to n-1 in the written order | `CreditStore` writes; the unique index as the final guard |
| A merge appends the sources' entries after the target's, in `sourceIds` order, skipping spellings the list holds | `CreditStore.Merge` |
| A plugin apply appends the chosen fetched names the person lacks, in fetched order, up to the limit | `domain.FillPersonItems` |
| The by-name rename replaces the spelling at position 0 and keeps its language | `CreditStore.ApplyByName` |

## What does not change

| Expected change | Why not |
| --- | --- |
| A `provisional` column or flag | It stays derived: a person without a position-0 row is provisional, as it is derived from the missing canonical row today |
| `search_key` of each row | Search, the library's name filters, the assistant's name lookup and the credit matching keep reading every row, so every spelling still matches |
| The `sort_key` the lists sort by | The listed relation joins the position-0 row where it joined the canonical row, so the name sorts and the keyset cursor are unchanged |
| The external-ID tables and `(source, value)` order | Not part of the feature |
