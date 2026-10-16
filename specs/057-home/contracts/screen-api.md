# Contract: Screen API for the home and onboarding

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
| `GET /api/home` | The ordered shelf summaries and the organizer state ([R-12](../research.md#r-12-the-home-lists-shelf-summaries-shelf-items-are-read-on-screen)) |
| `GET /api/home/shelves` | The first items of the shelves `keys` names (1 to 8, comma-separated) |
| `PUT /api/home/shelves` | Set which smart folders are shelves and their order ([R-7](../research.md#r-7-a-shelf-is-a-smart-folder-with-a-home-position)) |
| `POST /api/home/organize` | Queue a rebuild (`Reorganize`); `202` with the organizer state |
| `GET /api/onboarding` | The onboarding step ([R-3](../research.md#r-3-onboarding-progress-is-one-step-held-in-a-setting)) |
| `PUT /api/onboarding` | Set the step; answers the step |
| `GET /api/onboarding/interests` | The interest catalog, each with whether it is already a home shelf ([R-5](../research.md#r-5-the-interest-catalog-is-built-in-and-files-its-tags-under-genre)) |
| `POST /api/onboarding/interests` | Turn the named interests into tags and shelves ([R-6](../research.md#r-6-a-picked-interest-becomes-an-owner-shelf-and-auto-tagging-is-asked-again)) |

## Schemas added

```yaml
HomeShelfKey:            # "folder:<id>", "continue", "recent", "unshelved" or "recommended"
  type: string

HomeShelf:
  required: [key, kind, name]
  properties:
    key:      { $ref: HomeShelfKey }
    kind:     { type: string, enum: [folder, continue, recent, unshelved, recommended] }
    name:     { type: string }        # the folder name; empty for the fixed shelves, which the screen names
    folderId: { type: integer, format: int64 }   # folder shelves only
    byOrganizer: { type: boolean }    # owner only; true while owner_kept is 0

Home:
  required: [shelves]               # organizer is left out for a guest
  properties:
    shelves:   { type: array, items: { $ref: HomeShelf } }
    organizer: { $ref: HomeOrganizer }   # owner only
    empty:     { type: string, enum: [noVideos] }  # set when the library shows the viewer no video

HomeOrganizer:
  required: [running]
  properties:
    running: { type: boolean }
    pending: { type: string, enum: [rebuild, extend] }
    last:    { $ref: HomeOrganizeRun }

HomeOrganizeRun:         # data-model.md, Settings, home.organize.last
  required: [run, outcome, finishedAt, added, removed]

HomeShelfItems:
  required: [key, items, total]
  properties:
    key:   { $ref: HomeShelfKey }
    items: { type: array, maxItems: 24, items: { $ref: LibraryItem } }  # videos and groups only; a folder shelf leaves out its child folders (R-14)
    total: { type: integer }          # the shelf's video and group count; 0 drops the shelf from the screen

HomeShelfOrder:          # PUT /api/home/shelves
  required: [folderIds]
  properties:
    folderIds: { type: array, maxItems: 50, items: { type: integer, format: int64 } }

Onboarding:              # GET and PUT /api/onboarding
  required: [step]
  properties:
    step: { type: string, enum: [folders, scan, interests, done] }

Interest:
  required: [id, name, synonym, onHome]
  properties:
    id:      { type: string }         # data-model.md, Interest catalog
    name:    { type: string }
    synonym: { type: string }
    onHome:  { type: boolean }        # a home shelf already has this interest's tag rule

InterestPick:            # POST /api/onboarding/interests
  required: [ids]
  properties:
    ids: { type: array, minItems: 1, maxItems: 24, items: { type: string } }

InterestPickResult:
  required: [shelves, queued]
  properties:
    shelves: { type: array, items: { $ref: HomeShelf } }   # the shelves added; an interest already on the home adds none
    queued:  { type: integer }        # videos queued for auto-tagging; 0 when it is off
```

`GET /api/home/shelves` answers `{ shelves: HomeShelfItems[] }` in the order
of `keys`; a key for a shelf that no longer exists is left out, as the cards
read does in 052. `GET /api/onboarding/interests` answers
`{ interests: Interest[] }` in the catalog's order.

## Errors

| Route | Status | `code` | When |
| --- | --- | --- | --- |
| `GET /api/home/shelves` | 400 | `invalid_request` | No key, more than 8, or a malformed key |
| `PUT /api/home/shelves` | 400 | `invalid_request` | More than 50 ids, or an id twice |
| `PUT /api/home/shelves` | 404 | `not_found` | An id names no smart folder |
| `PUT /api/onboarding` | 400 | `invalid_request` | An unknown step |
| `POST /api/onboarding/interests` | 400 | `invalid_request` | No id, an unknown id, or an id twice |
| `POST /api/onboarding/interests` | 409 | `conflict` | The home would pass 50 shelves |

## Schemas and parameters changed

| Item | Change |
| --- | --- |
| `GET /api/library` | Gains `unshelved` (boolean): only videos in no home shelf ([R-11](../research.md#r-11-unshelved-means-matching-no-home-shelf-computed-on-read)); guests may use it |
| `SmartFolder` (the `GET /api/smart-folders/{id}` folder) | Gains `onHome` (boolean, owner only), for the folder page's add and remove action |
| Screen events | Gain the kind `home` with no payload ([R-16](../research.md#r-16-homechanged-tells-open-screens-to-read-the-home-again)), sent to the owner and guests |
