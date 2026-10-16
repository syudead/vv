---
source: specs/052-smart-folders/contracts/screen-api.md
sourceHash: 3d518d17720e06984feebb1bd99ee994a893ef74d3e2739cbc55e90c8d5a2d46
---

# 契約: スマートフォルダの画面の API {#contract-screen-api-for-smart-folders}

親 Issue: #1139。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは、この機能が加えるか変えるものだけを挙げる。`task generate` が `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API (`api/external-v1.yaml`) と MCP は変わらない。そこには AI による整理の Issue がスマートフォルダを加える。

読み取り (`GET`) は、`GET /api/groups/{id}` と同じように所有者とゲストのもの (`security` に `{}` を含む) なので、`accessRoutes` にそれらが加わる。書き込みはすべて所有者だけである。`cards` が id として読まれないように、`GET /api/smart-folders/cards` は `GET /api/smart-folders/{id}` より前に宣言する。`POST` と `PATCH` のルートは `requiresJSONBody` に加わる。所有者だけと記した項目は、ゲストには含めない。

## 加えるルート {#routes-added}

| ルート | 目的 |
| --- | --- |
| `GET /api/smart-folders` | 閲覧者に示すすべてのフォルダとその親の id。親の選択と木のため |
| `POST /api/smart-folders` | フォルダを作る |
| `GET /api/smart-folders/{id}` | 1 つのフォルダ、その祖先のパンくず、その項目の最初のページ |
| `GET /api/smart-folders/{id}/items` | 項目の後のページ |
| `GET /api/smart-folders/cards` | 画面に出ているカードのため、`ids` が挙げるフォルダ (1 から 60 個、カンマ区切り) のカードの値 ([data-model.md、Cards read](../data-model.md#cards-read)) |
| `PATCH /api/smart-folders/{id}` | 名前、規則、並び順、親を変える |
| `DELETE /api/smart-folders/{id}` | フォルダとその子孫を削除する |

## 加えるスキーマ {#schemas-added}

```yaml
SmartRule:                         # data-model.md, smart_folders.rule
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

SmartRuleRef:                      # a reference that no longer exists
  required: [kind, id]
  properties:
    kind: { type: string, enum: [tag, tagKind, person, organization] }
    id:   { type: integer, format: int64 }

SmartFolderSummary:                # a folder item in the library and on a folder's page (R-14)
  required: [id, name]
  properties:
    id:       { type: integer, format: int64 }
    parentId: { type: integer, format: int64 }
    name:     { type: string }
    favorite: { type: boolean }    # owner only

LibrarySmartFolder:                # a folder card; the value fields of LibraryGroup, without folder and videoIds
  required: [id, name, videoCount, sizeBytes, previews, tags]
  properties:
    id:       { type: integer, format: int64 }
    parentId: { type: integer, format: int64 }
    name:     { type: string }
    # videoCount, sizeBytes, addedAt, durationMs, previews, openVideoId, tags,
    # watchedCount, watchState, lastPlayedAt, favorite: as LibraryGroup; over the play order in
    # SmartFolderDetail, over the matching videos in SmartFolderCards (data-model.md, Contents)
    # (openVideoId and addedAt absent for an empty folder)

SmartFolderCards:
  required: [cards]
  properties:
    cards: { type: array, items: { $ref: LibrarySmartFolder } }   # in the order of ids; absent ids left out

SmartFolderSettings:               # owner only
  required: [rule, sort, missing]
  properties:
    rule:    { $ref: SmartRule }
    sort:    { $ref: VideoSort }
    missing: { type: array, items: { $ref: SmartRuleRef } }   # on this folder and its ancestors

SmartFolderItem:
  required: [kind]
  properties:
    kind:        { type: string, enum: [video, group, smartFolder] }
    video:       { $ref: Video }
    group:       { $ref: LibraryGroup }
    smartFolder: { $ref: SmartFolderSummary }

SmartFolderDetail:
  required: [smartFolder, trail, items]
  properties:
    smartFolder: { $ref: LibrarySmartFolder }
    settings:    { $ref: SmartFolderSettings }   # owner only
    trail:       { type: array, items: { $ref: GroupRef } }   # ancestors, outermost first
    items:       { type: array, items: { $ref: SmartFolderItem } }
    nextCursor:  { type: string }

SmartFolderCreate:
  required: [name, rule]
  properties:
    name:     { type: string }     # 1 to 100 characters after trimming
    rule:     { $ref: SmartRule }
    sort:     { $ref: VideoSort }  # default addedDesc
    parentId: { type: integer, format: int64 }

SmartFolderChange:                 # at least one member
  properties:
    name:     { type: string }
    rule:     { $ref: SmartRule }
    sort:     { $ref: VideoSort }
    parentId: { type: [integer, "null"], format: int64 }   # null makes it a root
```

## 変えるスキーマとパラメーター {#schemas-and-parameters-changed}

| スキーマかルート | 変更 |
| --- | --- |
| `LibraryItem` | `kind` に、`smartFolder: SmartFolderSummary` を伴う `smartFolder` が加わる ([data-model.md、Library items](../data-model.md#library-items)) |
| `listVideos`、`listLibrary`、`listLibraryIds` | `SmartRule` の範囲を持つ `addedWithin`、`minDuration`、`maxDuration` が加わる。ゲストもそれらを送れる。範囲外の値、または `minDuration` が `maxDuration` を超えることは `400 invalid_request` である |
| `FavoritesRequest`、`FavoritesResponse` | `smartFolderIds` が加わる |
| `PlaylistItemsAdd`、`PlaylistItem` | `smartFolderIds` と `smartFolder` の種類が加わる。再生のときのフォルダの再生の順序で再生する |
| `GET /api/videos/{id}`、`/related`、`/group-members` | `smartFolder` のクエリーが加わる: `group` と同じように (048 R-9)、フォルダの再生の順序が `Up next`、前後の動画、メンバーの窓の文脈になる。両方を送ると `group` が優先する。その動画を持たないフォルダは無視する |
| `Video` | 文脈がフォルダのとき `smartFolder: { id, name, position, count }` が加わる |
| `/api/events` | `{ ids }` を持つ種類 `smartFolder` が加わる ([R-12](../research.md#r-12-writes-that-can-change-a-match-publish-smartfolderschanged)) |

## ステータス {#statuses}

| ルート | ステータス | `code` | 条件 |
| --- | --- | --- | --- |
| `POST`、`PATCH /api/smart-folders…` | 400 | `invalid_smart_rule` | `rule` が `domain.SmartRule` に通らないか、`sort` が `random` である |
| `POST`、`PATCH /api/smart-folders…` | 400 | `invalid_request` | グループと同じように、前後の空白を除いた名前が空か 100 文字を超える |
| `POST`、`PATCH /api/smart-folders…` | 404 | `smart_folder_not_found` | `parentId` がどのフォルダも指さない |
| `PATCH /api/smart-folders/{id}` | 409 | `smart_folder_cycle` | `parentId` がフォルダ自身かその子孫である |
| `GET /api/smart-folders/cards` | 400 | `invalid_request` | `ids` がないか、空か、整数でないか、60 個を超える id を挙げる |
| すべての `/api/smart-folders/{id}` のルート | 404 | `not_found` | そのフォルダがないか、ゲストが見えるものが何もないフォルダを開く |

存在しない id を挙げる `rule` は受け付け、`missing` で報告する。そのため、タグが削除された直後の絞り込みも保存できる。
