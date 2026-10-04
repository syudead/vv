# Contract: Screen API changes

Parent Issue: #630.

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This file lists
only the added fields and values. `task generate` regenerates
`internal/httpapi/gen/` and `web/src/api/gen/`.

## 0. Fields added to `Video`

```yaml
Video:
  required: [..., addedAt, updatedAt, fileCreatedAt, ...]
  properties:
    updatedAt:
      type: string
      format: date-time
      description: |
        When the video's information (display name, tags, visibility, thumbnail) was last
        edited in vv. Equal to addedAt if never edited (specs/033-video-dates/data-model.md §3)
    fileCreatedAt:
      type: string
      format: date-time
      description: |
        Creation time of the listed location's file. The file's modification time (mtime) when
        the file system does not provide one (specs/033-video-dates/research.md R-4)
```

- The fields appear in every response that returns videos (lists,
  `GET /api/videos/{id}`, related, versions, the `retry` probe, and the responses
  of `PUT /api/videos/{id}/display-name` and `thumbnail-position`).
- Guest responses include them too
  ([R-7](../research.md#r-7-the-response-fields-are-updatedat-edit-time-in-vv-and-filecreatedat-location-creation-time-with-the-same-names-in-the-screen-and-external-apis)).
  The list of omitted fields in `guest-api.md` does not change.
- `LibraryGroup` (group cards) does not get them (out of scope).

## 1. Values added to `VideoSort`

```yaml
VideoSort:
  enum: [..., modifiedAsc, modifiedDesc, createdAsc, createdDesc, ...]
  description: |
    …, modified = modification time (mtime) of the listed location, created = creation time
    of the listed location (mtime when unavailable), …
```

Accepted in `sort` on `GET /api/videos`, `GET /api/folders/{rootId}/videos` and
`GET /api/library`. `GET /api/library/ids` (the owner-only "select all") has no
`sort` and decides no order, so it does not change. Guests are allowed these
values. `modifiedAsc` and `modifiedDesc` keep their names and order
(requirement 7).

## 2. Unchanged routes

No route, error shape or event is added. The responses of tag attach/detach
(`POST /api/video-tags`) and visibility toggling (`PUT /api/video-visibility`)
stay as they are. The video page reloads with `GET /api/videos/{id}` after either
succeeds
([R-8](../research.md#r-8-the-video-page-reloads-the-video-after-tag-and-visibility-changes-and-no-new-domain-event-is-added)).

## 3. `web/src/api` changes

- Add `createdAsc` and `createdDesc` to `videoSorts` (`isVideoSort` then accepts
  them).
- The screen's list criteria (the URL round trip in `listCriteria` and the device
  setting in `viewPreferences`) accept only the `videoSorts` values whose kind is
  in `sortKinds` (`isListSort`); any other value falls back to the device setting
  or the default, like an unparseable sort. Accepting a value missing from the
  menu would show the sort as Random and make the direction impossible to toggle.
  `created*` becomes accepted when the unit "Add "Date created" to the list sort
  orders" adds `created` to `sortKinds`.
- The generated `Video` type makes `updatedAt` and `fileCreatedAt` required, so
  the Vitest fixtures are updated.
