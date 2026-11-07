# Contract: screen API changes for the home and the folder favorites

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), operations
`listLibrary` and `updateFavorites` and the schema `LibraryItem`. No endpoint,
parameter, schema field or error is added; the two operation descriptions and
the `LibraryItem` description change, and the behaviour of `listLibrary` with
them.

## `GET /api/library`

Lists videos and groups only, for every audience, filter and sort; no
`SmartFolderSummary` item leads the first page.

| Aspect | Before | After |
| --- | --- | --- |
| Owner, unfiltered | Root smart folders by name, then the items | The items |
| Owner, `favorite=true` | Every favorite folder by name, then the favorite items | The favorite items |
| `total` | Folders plus items | Items |
| Cursor inside the folder section | Continues after the named folder | `400 invalid_cursor`; the home shows its existing message and retry, and a reload starts from page one |
| Guest | No folders | Unchanged |

**Response**: `200` with `LibraryPage` as today; `invalid_cursor` as for any
cursor of another list.

The sentence "Without filters, the owner's smart folders come first by name in
every sort and count toward `limit` and `total`; guests get none." leaves the
description. The screens read the folders through `listSmartFolders`
([plan.md, Home band of favorite folders](../plan.md#home-band-of-favorite-folders)).

## `LibraryItem`

The schema keeps `kind: smartFolder` and its `smartFolder` field, because a
paired node on an older release still leads its library page with its root
folders and the merged page passes them through
([plan.md, Technical Context](../plan.md#technical-context)). Its description
stops promising a top-level or favorite folder from this node and says that a
`smartFolder` item comes only from such an older paired node.

## `PUT /api/favorites`

Unchanged in request, response and errors. Its description says the change
emits no events; the store raises the all-folder `smartFolder` event whenever
the request names a video, group or smart folder, because a folder rule can
read a favorite, and the band and the list page rely on it. The description
says so after this feature.
