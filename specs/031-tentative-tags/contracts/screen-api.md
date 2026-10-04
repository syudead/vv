# Contract: Tentative tags and rejected names in the screen API

Source of truth: `api/openapi.yaml`. This document describes only the fields,
routes and errors this feature adds or changes. The error shape, the same-origin
check, JSON body parsing, and additions to `requiresJSONBody` and
`openapi_routes_test.go` stay as in
[specs/014-video-tags/contracts/tags-api.md](../../014-video-tags/contracts/tags-api.md).
The decisions are [research.md R-5, R-8 and R-9](../research.md).

## Schema changes

| Schema | Added field | Rule |
| --- | --- | --- |
| `TagRef` | `tentative: boolean` (`required`) | Appears in a video's tags, in `tag` of the `POST /api/video-tags` response, and in `items[].tag` of `summary` |
| `VideoTag` | `tentative: boolean` (`required`) | One entry of `Video.tags`. A tag present only through a folder also carries the tag's state |
| `Tag` | `tentative: boolean` (`required`) | `GET /api/tags` and the responses of create, rename, merge, synonym and confirm |

New schema:

```yaml
RejectedTagNameList:
  type: object
  required: [items]
  additionalProperties: false
  properties:
    items:
      type: array
      description: Rejected names, in natural name order
      items: { type: string }
```

`Error.code` gains `tag_not_tentative`.

## Changed existing routes

| Route | Change |
| --- | --- |
| `PATCH /api/tags/{id}` | When the name changes, a tentative tag is returned as a confirmed tag ([data-model.md, Write rules](../data-model.md#write-rules)). A rename to the current name changes nothing |
| `POST /api/tags/{id}/synonyms` | When a name is added, the tentative tag `{id}` is returned as a confirmed tag |
| `POST /api/tags/{id}/merge` | The target `{id}` is returned as a confirmed tag. The source is deleted, tentative or confirmed |
| `POST /api/tags`, `POST /api/video-tags` (when creating with `tag: { name }`) | As today: creates a confirmed tag. If the name is a rejected name, it is removed from the list (requirement 15) |
| `DELETE /api/tags/{id}` | Unchanged. Remembers no name, tentative or confirmed. The screen does not offer this operation for tentative tags (requirement 10) |

## Tentative tag operations

| Route | Body | Success | Errors |
| --- | --- | --- | --- |
| `POST /api/tags/{id}/confirm` | — | 200 `Tag` (`tentative: false`; an already confirmed tag is returned unchanged) | 404 `tag_not_found` |
| `POST /api/tags/{id}/reject` | — | 204. The tag is deleted, removed from its videos, and its canonical name enters the rejected-name list | 404 `tag_not_found`, 409 `tag_not_tentative` (a confirmed tag; nothing changes) |

- Both are owner-only (`security` matches the existing tag operations; the
  boundary's default classification returns `401`).
- The screen handles `tag_not_tentative` like `tag_not_found` (it closes the
  dialog and reloads the list; edge case "conflicting operations").
- After `reject`, the screen reloads the rejected-name list ([Rejected names](#rejected-names)). The response
  carries no name because reloading the list is enough, and the same path also
  reflects rejects made in other tabs.

## Rejected names

| Route | Body | Success | Errors |
| --- | --- | --- | --- |
| `GET /api/tags/rejected-names` | — | 200 `RejectedTagNameList` | — |
| `DELETE /api/tags/rejected-names?name=…` | — | 204 (also 204, with no change, when the name is absent; edge case) | 400 (`name` missing) |

- Owner-only.
- `name` is normalized with `domain.NormalizeTagName` before matching (the same
  normalization as at registration, and the same as
  `DELETE /api/tags/{id}/synonyms`). Input that cannot be normalized matches
  nothing and returns 204.
- `/api/tags/rejected-names` is distinguished from `/api/tags/{id}` by the
  `ServeMux` rule that literal segments take precedence. `openapi_routes_test.go`
  gains a check that `GET` and `DELETE` on this route are not captured by `{id}`
  (as with `/api/videos/ids`).
- A removed name is created again as a tentative tag the next time it is attached
  with `tentative: true` (acceptance criterion 13).

## `web/src/api` functions

Add `confirmTag(id)`, `rejectTag(id)`, `listRejectedTagNames()` and
`forgetRejectedTagName(name)` to `client.ts` (or `tags.ts`). `confirmTag` and
`rejectTag` reload the shared list, like `renameTag` and `deleteTag`. `errorText`
gains the text for `tag_not_tentative`.
