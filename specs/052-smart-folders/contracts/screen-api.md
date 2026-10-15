# Contract: Screen API for smart folders

Parent Issue: #1139. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml).
This file lists only what this feature adds or changes on the group routes of
[048](../../048-groups/contracts/screen-api.md), the organize routes of
[050](../../050-drag-organize/contracts/screen-api.md) and the library list
parameters. `task generate` regenerates `internal/httpapi/gen/` and
`web/src/api/gen/`. The external API (`api/external-v1.yaml`) and MCP do not
change; the AI organizing Issue adds smart folders there.

Access follows 048: reads are for the owner and guests, writes owner only. The
fields marked owner only are left out for a guest.

## Schemas added

```yaml
SmartRule:                         # data-model.md, groups.rule
  additionalProperties: false
  properties:
    q:            { type: string, maxLength: 500 }
    watch:        { type: string, enum: [unwatched, inProgress, watched] }
    playable:     { type: boolean }
    favorite:     { type: boolean }
    tags:         { type: array, minItems: 1, maxItems: 16, items: { type: integer, format: int64 } }
    tagKind:      { type: integer, format: int64 }
    person:       { type: integer, format: int64 }
    organization: { type: integer, format: int64 }
    addedWithin:  { type: integer, minimum: 1, maximum: 36500 }
    minDuration:  { type: integer, minimum: 0, maximum: 360000 }
    maxDuration:  { type: integer, minimum: 0, maximum: 360000 }

SmartRuleRef:                      # a reference the rule names that no longer exists
  required: [kind, id]
  properties:
    kind: { type: string, enum: [tag, tagKind, person, organization] }
    id:   { type: integer, format: int64 }

SmartFolder:                       # owner only
  required: [rule, sort, missing]
  properties:
    rule:    { $ref: SmartRule }
    sort:    { $ref: VideoSort }   # random is refused
    missing: { type: array, items: { $ref: SmartRuleRef } }
```

## Schemas changed

| Schema | Change |
| --- | --- |
| `LibraryGroup` | Gains `smart: boolean`, true for a smart folder, for guests too |
| `GroupDetail` | Gains `smartFolder: SmartFolder` (owner only, smart folders only) |
| `GroupItem` | `id` becomes optional: a matched video with no item row has none. Gains `manual: boolean` (owner only, smart folders only): the video is in by an item and does not match |
| `GroupCreate` | Gains optional `rule: SmartRule` and `sort: VideoSort`; with `rule` the group is a smart folder |
| `GroupRename` (`PATCH /api/groups/{id}`) | `name` becomes optional; gains optional `rule` and `sort`; at least one of the three is required |
| `GroupItemRemove` | `itemIds` becomes optional; gains `videoIds` (0 to 20,000); at least one is non-empty |

## Routes added

| Route | Purpose |
| --- | --- |
| `GET /api/groups/{id}/excluded` | Owner only. The videos the owner removed from a smart folder, as a `VideoPage` with `cursor`; empty for a group |

## List parameters added

`listVideos`, `listLibrary` and `listLibraryIds` gain `addedWithin`,
`minDuration` and `maxDuration` with the ranges of `SmartRule`; guests may
send them ([research.md R-4](../research.md#r-4-the-library-filter-gains-added-within-and-length)).
A value out of range, or `minDuration` over `maxDuration`, is
`400 invalid_request`.

## Statuses

| Route | Status | `code` | When |
| --- | --- | --- | --- |
| `POST /api/groups`, `PATCH /api/groups/{id}` | 400 | `invalid_smart_rule` | `rule` fails `domain.SmartRule`, or `sort` is `random` |
| `PATCH /api/groups/{id}` | 409 | `group_not_smart` | `rule` or `sort` on a group that is not a smart folder |
| `POST /api/groups/{id}/items/{itemId}/move`, `POST /api/organize/reorder` | 409 | `smart_folder_sorted` | The moved item is a video in a smart folder |
| `POST /api/organize/group` | 409 | `smart_folder_sorted` | `from` is a smart folder |
| `POST /api/organize/merge` | 409 | `smart_folder_merge` | A merged item is a smart folder |
| `POST /api/groups/{id}/items/remove` | 404 | `group_target_not_found` | A `videoIds` entry is not a member of the folder; nothing is written |
| `POST /api/groups/{id}/items/remove` | 200 | none | Removing the last member of a smart folder answers the empty folder's `LibraryGroup` instead of 048's `204` for a dissolved group ([R-8](../research.md#r-8-an-empty-smart-folder-stays-and-is-shown-to-the-owner-only)) |
| `POST /api/organize/move` | 200 | none | A smart folder `from` left empty stays |

A `rule` naming an id that does not exist is accepted and reported in
`missing`, so a folder saved from a filter whose tag was just deleted is not
refused.
