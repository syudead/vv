---
source: specs/057-ai-home/contracts/screen-api.md
sourceHash: 756064912b77fec5347ee33e72eed2bdf7075dd4882591dd3c4a9f0d07ac0280
---

# 契約: ホームの画面の API {#contract-screen-api-for-the-home}

親 Issue: #1140。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは、この機能が加えるもの、変えるものだけを一覧する。`task generate` は `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API と MCP は変わらない。

`GET /api/home` と `GET /api/home/shelves` は所有者とゲストのためのものなので、`accessRoutes` にそれらを加える。ほかのルートはすべて所有者だけである。所有者だけと記したフィールドは、ゲストには含めない。

## 加えるルート {#routes-added}

| ルート | 目的 |
| --- | --- |
| `GET /api/home` | 順序付きの棚の要約と整理役の状態 ([R-9](../research.md#r-9-the-home-lists-shelf-summaries-shelf-items-are-read-on-screen)) |
| `GET /api/home/shelves` | `keys` が名指す棚 (1 から 8、コンマ区切り) の最初の項目 |
| `PUT /api/home/shelves` | どのスマートフォルダを棚にするかとその順序を設定する ([R-4](../research.md#r-4-a-shelf-is-a-smart-folder-with-a-home-position)) |
| `POST /api/home/organize` | 作り直しを積む (`Reorganize`)。整理役の状態とともに `202` |
| `GET /api/settings/home` | 方針と整理役の状態 |
| `PUT /api/settings/home` | 方針を保存する。変更は作り直しを積む ([R-13](../research.md#r-13-the-policy-is-one-setting-without-ai-it-is-stored-but-unused)) |

## 加えるスキーマ {#schemas-added}

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
  required: [shelves, organizer]
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
    items: { type: array, maxItems: 24, items: { $ref: LibraryItem } }
    total: { type: integer }          # the shelf's item count; 0 drops the shelf from the screen

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

`GET /api/home/shelves` は `keys` の順に `{ shelves: HomeShelfItems[] }` を応答する。もうない棚のキーは、052 のカードの読み取りと同じく、含めない。

## エラー {#errors}

| ルート | ステータス | `code` | 条件 |
| --- | --- | --- | --- |
| `GET /api/home/shelves` | 400 | `invalid_request` | キーがない、8 を超える、または形の誤ったキー |
| `PUT /api/home/shelves` | 400 | `invalid_request` | 50 を超える id、または 2 回現れる id |
| `PUT /api/home/shelves` | 404 | `not_found` | id がどのスマートフォルダも名指さない |
| `PUT /api/settings/home` | 400 | `invalid_request` | 方針が 2,000 文字を超える |

## 変わるスキーマとパラメーター {#schemas-and-parameters-changed}

| 項目 | 変更 |
| --- | --- |
| `GET /api/library` | `unshelved` (boolean) を得る: どのホームの棚にもない動画だけ ([R-8](../research.md#r-8-unshelved-means-matching-no-home-shelf-computed-on-read))。ゲストも使える |
| `SmartFolder` (`GET /api/smart-folders/{id}` のフォルダ) | フォルダのページの加える操作と外す操作のために、`onHome` (boolean、所有者だけ) を得る |
| 画面のイベント | ペイロードのない種類 `home` を得る ([R-14](../research.md#r-14-homechanged-tells-open-screens-to-read-the-home-again))。所有者とゲストに送る |
