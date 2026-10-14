---
source: specs/048-groups/contracts/screen-api.md
sourceHash: 39756437432bcd0f3d4f2b0daffcb9dcf81170039f2a20057171a5ee610a6139
---

# 契約: グループの画面の API {#contract-screen-api-for-groups}

親 Issue: #1040。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは、この機能が加えるか変えるルートとスキーマだけを挙げる。`task generate` が `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API (`api/external-v1.yaml`) と MCP はグループを出さず、変わらない。

読み取り (`GET`) は `GET /api/library` と同じく所有者とゲストのためのもの (`security` が `{}` を含む) なので、`accessRoutes` にそれらが加わる。書き込みはすべて `/api/*` の既定により所有者だけである。`POST` と `PATCH` のルートは `requiresJSONBody` に加わる。

## 加えるルート {#routes-added}

| ルート | 目的 |
| --- | --- |
| `GET /api/groups` | 閲覧者に見せるすべてのグループ。「Add to group」のピッカー用 |
| `POST /api/groups` | 動画とグループからグループを作る |
| `GET /api/groups/{id}` | 1 つのグループ、その経路、項目の最初のページ |
| `GET /api/groups/{id}/items` | 項目の後のページ |
| `PATCH /api/groups/{id}` | 名前を変える |
| `DELETE /api/groups/{id}` | 解散する |
| `POST /api/groups/{id}/items` | 動画とグループを加える |
| `POST /api/groups/{id}/items/remove` | 項目を取り除く |
| `POST /api/groups/{id}/items/{itemId}/move` | 1 つの項目を並べ替える |
| `POST /api/groups/{id}/tag` | グループをタグに変える |

## 加えるスキーマ {#schemas-added}

```yaml
GroupMembers:                     # body of create and add
  properties:
    videoIds: { type: array, maxItems: 20000, items: { type: integer, format: int64 } }
    groupIds: { type: array, maxItems: 20000, items: { type: integer, format: int64 } }

GroupCreate:
  allOf: [GroupMembers]
  required: [name]
  properties:
    name: { type: string }        # 1 to 100 characters after trimming

GroupItem:
  required: [id, kind]
  properties:
    id: { type: integer, format: int64 }   # the item, not the video or group
    kind: { type: string, enum: [video, group] }
    video: { $ref: Video }                 # kind video
    group: { $ref: LibraryGroup }          # kind group

GroupRef:
  required: [id, name]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }

GroupDetail:
  required: [group, trail, items]
  properties:
    group: { $ref: LibraryGroup }
    trail: { type: array, items: { $ref: GroupRef } }   # the valid prefix of ?trail, outermost first
    items: { type: array, items: { $ref: GroupItem } }  # first page, position order, shown items only
    nextCursor: { type: string }

GroupItemPage:
  required: [items]
  properties:
    items: { type: array, items: { $ref: GroupItem } }
    nextCursor: { type: string }

GroupListPage:                    # GET /api/groups
  required: [groups]
  properties:
    groups: { type: array, items: { $ref: LibraryGroup } }   # natural name order, at every depth
    nextCursor: { type: string }

GroupItemMove:
  properties:
    after: { type: integer, format: int64 }   # item id; absent moves to the top

GroupItemRemove:
  required: [itemIds]
  properties:
    itemIds: { type: array, minItems: 1, maxItems: 20000, items: { type: integer, format: int64 } }
```

## ステータス {#statuses}

| ルート | ステータス | 条件 |
| --- | --- | --- |
| `GET /api/groups?q=&cursor=&limit=` | `200` | `GroupListPage`: 閲覧者に見せるグループのうち、すべての深さで、名前が大文字と小文字を区別せずにトリムした `q` を含むもの (`q` が空ならすべてのグループ)。`limit` は 1 から 200、既定は 100 |
| | `400 invalid_cursor` | 読めないカーソル |
| `POST /api/groups` | `201` | 新しいグループの `LibraryGroup` |
| | `400 invalid_request` | 名前が空か長すぎる、またはメンバーがない |
| | `404 group_target_not_found` | 本文の動画かグループが存在しない。何も作らない |
| `GET /api/groups/{id}` | `200` | `GroupDetail` |
| | `404 not_found` | 存在しないか、閲覧者に示されない |
| `GET /api/groups/{id}/items?cursor=&limit=` | `200` | `GroupItemPage`。`limit` は 1 から 200、既定は 100 |
| | `400 invalid_cursor` | 読めない cursor |
| `PATCH /api/groups/{id}` | `200` | トリムした名前を持つ `LibraryGroup` |
| `DELETE /api/groups/{id}` | `204` | 解散した ([R-4](../research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup)) |
| `POST /api/groups/{id}/items` | `200` | 追加の後の `LibraryGroup` |
| | `404 group_target_not_found` | 本文の動画かグループが存在しない |
| | `409 group_cycle` | 本文のグループが、このグループかその祖先の 1 つである |
| `POST /api/groups/{id}/items/remove` | `200` | 項目が残るとき、取り除いた後の `LibraryGroup` |
| | `204` | 項目が残らなかったので、`DELETE` と同じようにグループを解散した ([R-4](../research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup)、[data-model.md、Edits](../data-model.md#edits))。本文はない |
| | `404 group_target_not_found` | 項目の id がグループにない。何も取り除かない |
| `POST /api/groups/{id}/items/{itemId}/move` | `204` | 移動した |
| | `400 invalid_request` | `after` が項目自身である |
| | `404 group_target_not_found` | 項目か `after` がグループにない |
| `POST /api/groups/{id}/tag` | `200` | `{ tag: TagRef, created: boolean }` |
| | `400 invalid_request` | 名前がタグの名前の規則に反する。何も変わらない |
| `{id}` を持つすべてのルート | `404 not_found` | グループが存在しない |

`GET /api/groups/{id}` の `?trail=1,5` は、画面がたどってきた `{id}` より上のグループを挙げる。答えは、親とその子の項目である組を、そうでない最初の組の手前まで保ち、`{id}` の親で終わる ([R-8](../research.md#r-8-a-group-page-at-groupsid-with-a-trail-in-the-url))。

## 変えるルートとスキーマ {#routes-and-schemas-changed}

| 対象 | 変更 |
| --- | --- |
| `LibraryGroup` | `id` (必須) を得る。`folder` は省略できるようになり、自動のグループでだけ現れるので、画面は 2 つを見分けられる |
| `LibraryGroupIds` (`GET /api/library/ids`) | `folder` が `id` になる |
| `VideoGroupRef` (`Video.group`) | `folder` が `id` になる。`LibraryGroup` と同じように省略できる `folder` を得る |
| `RelatedGroup` | `folder` が `id` になる |
| `GET /api/videos/{id}`、`GET /api/videos/{id}/related`、`GET /api/videos/{id}/group-members` | 省略できるクエリ `group` を得る ([R-9](../research.md#r-9-the-video-page-follows-the-group-it-was-opened-from))。その動画を持たない `group` は R-9 のとおり代わりのグループを使う |
| `FavoritesRequest`、`FavoritesResponse` (`PUT /api/favorites`) | `folders` が `groupIds` に、`appliedFolders` が `appliedGroups` になる |
| `PlaylistItemsAdd` (`POST /api/playlists/{id}/items`) | `folders` が `groupIds` になる |
| `FolderGrouping` | `grouped` と `taggable` が、フォルダの自動のグループである省略できる `groupId` になる |
| `PUT /api/folders/{rootId}/grouping` | `ungroup` はフォルダのグループを解散する。`auto` と `groupDirect` は同期を実行する ([R-4](../research.md#r-4-dissolving-deletes-the-group-for-an-automatic-group-it-writes-ungroup)) |
| `GET /api/folders/{rootId}/group`、`POST /api/folders/{rootId}/grouping/tag` | 取り除く。`GET /api/groups/{id}` と `POST /api/groups/{id}/tag` が代わる |
| エラーコード | `group_target_not_found`、`group_cycle` を加える。`not_folder_group`、`root_group_not_taggable`、`folder_not_group` を取り除く |

## `/api/events` {#apievents}

| 種類 | 条件 |
| --- | --- |
| `group` | グループの書き込みか、グループを変えた同期の後。それらの id とその祖先の id を運ぶ ([R-13](../research.md#r-13-a-group-event-tells-open-screens-to-read-again))。ゲストも、名前なしで受け取る |
