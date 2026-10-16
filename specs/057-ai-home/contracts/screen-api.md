# Contract: Screen API for the home

Parent Issue: #1140. Source of truth: [api/openapi.yaml](../../../api/openapi.yaml).
This file lists only what this feature adds or changes. `task generate`
regenerates `internal/httpapi/gen/` and `web/src/api/gen/`. The external API
and MCP do not change.

`GET /api/home` and `GET /api/home/shelves` are for the owner and guests, so
`accessRoutes` gains them; every other route is owner only. Fields marked
owner only are left out for a guest.

## Routes added

| Route | Purpose |
| --- | --- |
| `GET /api/home` | The ordered shelf summaries and the organizer state ([R-9](../research.md#r-9-the-home-lists-shelf-summaries-shelf-items-are-read-on-screen)) |
| `GET /api/home/shelves` | The first items of the shelves `keys` names (1 to 8, comma-separated) |
| `PUT /api/home/shelves` | Set which smart folders are shelves and their order ([R-4](../research.md#r-4-a-shelf-is-a-smart-folder-with-a-home-position)) |
| `POST /api/home/organize` | Queue a rebuild (`Reorganize`); `202` with the organizer state |
| `GET /api/settings/home` | The policy and the organizer state |
| `PUT /api/settings/home` | Save the policy; a change queues a rebuild ([R-13](../research.md#r-13-the-policy-is-one-setting-without-ai-it-is-stored-but-unused)) |

## Schemas added

```yaml
HomeShelfKey:            # "folder:<id>", "unshelved", "recent" or "recommended"
  type: string

HomeShelf:
  required: [key, kind, name]
  properties:
    key:      { $ref: HomeShelfKey }
    kind:     { type: string, enum: [folder, unshelved, recent, recommended] }
    name:     { type: string }        # the folder name; empty for the fixed shelves, which the screen names
    folderId: { type: integer, format: int64 }   # folder shelves only
    byOrganizer: { type: boolean }    # owner only; true while owner_kept is 0

Home:
  required: [shelves]               # organizer is left out for a guest
  properties:
    shelves:   { type: array, items: { $ref: HomeShelf } }
    organizer: { $ref: HomeOrganizer }   # owner only
    empty:     { type: string, enum: [noVideos, noTags] }  # set when the home has nothing to show

HomeOrganizer:
  required: [organizer, running]
  properties:
    organizer: { type: string, enum: [ai, rules] }   # which one the next run uses (R-1)
    running:   { type: boolean }
    pending:   { type: string, enum: [rebuild, extend] }
    last:      { $ref: HomeOrganizeRun }

HomeOrganizeRun:         # data-model.md, Settings, home.organize.last
  required: [organizer, run, outcome, finishedAt, added, removed]

HomeShelfItems:
  required: [key, items, total]
  properties:
    key:   { $ref: HomeShelfKey }
    items: { type: array, maxItems: 24, items: { $ref: LibraryItem } }  # videos and groups only; a folder shelf leaves out its child folders (R-11)
    total: { type: integer }          # the shelf's video and group count; 0 drops the shelf from the screen

HomeShelfOrder:          # PUT /api/home/shelves
  required: [folderIds]
  properties:
    folderIds: { type: array, maxItems: 50, items: { type: integer, format: int64 } }

HomeSettings:
  required: [policy, organizer]
  properties:
    policy:    { type: string, maxLength: 2000 }
    organizer: { $ref: HomeOrganizer }
```

`GET /api/home/shelves` answers `{ shelves: HomeShelfItems[] }` in the order
of `keys`; a key for a shelf that no longer exists is left out, as the cards
read does in 052.

## Errors

| Route | Status | `code` | When |
| --- | --- | --- | --- |
| `GET /api/home/shelves` | 400 | `invalid_request` | No key, more than 8, or a malformed key |
| `PUT /api/home/shelves` | 400 | `invalid_request` | More than 50 ids, or an id twice |
| `PUT /api/home/shelves` | 404 | `not_found` | An id names no smart folder |
| `PUT /api/settings/home` | 400 | `invalid_request` | The policy is over 2,000 characters |

## Schemas and parameters changed

| Item | Change |
| --- | --- |
| `GET /api/library` | Gains `unshelved` (boolean): only videos in no home shelf ([R-8](../research.md#r-8-unshelved-means-matching-no-home-shelf-computed-on-read)); guests may use it |
| `SmartFolder` (the `GET /api/smart-folders/{id}` folder) | Gains `onHome` (boolean, owner only), for the folder page's add and remove action |
| Screen events | Gain the kind `home` with no payload ([R-14](../research.md#r-14-homechanged-tells-open-screens-to-read-the-home-again)), sent to the owner and guests |
