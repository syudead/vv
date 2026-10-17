---
source: specs/057-home/contracts/screen-api.md
sourceHash: 4c68c653e6e9a79dc0fb5fb1fed0787e4a716b16eb6121ff3352c5689750b270
---

# 契約: ホームとオンボーディングの画面の API {#contract-screen-api-for-the-home-and-onboarding}

親 Issue: #1140。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは、この機能が加えるもの、変えるものだけを一覧する。`task generate` は `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API と MCP は変わらない。

`GET /api/home` と `GET /api/home/shelves` は所有者とゲストのためのものなので、`accessRoutes` にそれらを加える。ほかのルートはすべて所有者だけである。所有者だけと記したフィールドは、ゲストには含めない。

## 加えるルート {#routes-added}

| ルート | 目的 |
| --- | --- |
| `GET /api/home` | 順序付きの棚の要約と整理役の状態 ([R-12](../research.md#r-12-the-home-lists-shelf-summaries-shelf-items-are-read-on-screen)) |
| `GET /api/home/shelves` | `keys` が名指す棚 (1 から 8、コンマ区切り) の最初の項目 |
| `PUT /api/home/shelves` | どのスマートフォルダを棚にするかとその順序を設定する ([R-7](../research.md#r-7-a-shelf-is-a-smart-folder-with-a-home-position)) |
| `POST /api/home/organize` | 作り直しを積む (`Reorganize`)。整理役の状態とともに `202` |
| `GET /api/onboarding` | オンボーディングの段階 ([R-3](../research.md#r-3-onboarding-progress-is-one-step-held-in-a-setting)) |
| `PUT /api/onboarding` | 段階を設定する。段階を応答する |
| `GET /api/onboarding/interests` | 興味の一覧。各興味がすでにホームの棚かどうかを含む ([R-5](../research.md#r-5-the-interest-catalog-is-built-in-and-files-its-tags-under-genre)) |
| `POST /api/onboarding/interests` | 名指した興味をタグと棚にする ([R-6](../research.md#r-6-a-picked-interest-becomes-an-owner-shelf-and-auto-tagging-is-asked-again)) |

## 加えるスキーマ {#schemas-added}

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
    items: { type: array, maxItems: 24, items: { $ref: HomeShelfItem } }  # videos and groups only; a folder shelf leaves out its child folders (R-14)
    total: { type: integer }          # the shelf's video and group count, capped at 25 (one more than a page; R-12); 0 drops the shelf from the screen

HomeShelfItem:           # a LibraryItem without smart folders, whose group is the card alone (R-12)
  required: [kind]
  properties:
    kind:  { type: string, enum: [video, group] }
    video: { $ref: Video }
    group: { $ref: LibraryGroupCard }  # LibraryGroup without videoIds and tags; LibraryGroup is LibraryGroupCard plus those two

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

`GET /api/home/shelves` は `keys` の順に `{ shelves: HomeShelfItems[] }` を応答する。もうない棚のキーは、052 のカードの読み取りと同じく、含めない。`GET /api/onboarding/interests` は一覧の順に `{ interests: Interest[] }` を応答する。

## エラー {#errors}

| ルート | ステータス | `code` | 条件 |
| --- | --- | --- | --- |
| `GET /api/home/shelves` | 400 | `invalid_request` | キーがない、8 を超える、または形の誤ったキー |
| `PUT /api/home/shelves` | 400 | `invalid_request` | 50 を超える id、または 2 回現れる id |
| `PUT /api/home/shelves` | 404 | `not_found` | id がどのスマートフォルダも名指さない |
| `PUT /api/onboarding` | 400 | `invalid_request` | 未知の段階 |
| `POST /api/onboarding/interests` | 400 | `invalid_request` | id がない、未知の id、または 2 回現れる id |
| `POST /api/onboarding/interests` | 409 | `conflict` | ホームの棚が 50 を超えることになる |

## 変わるスキーマとパラメーター {#schemas-and-parameters-changed}

| 項目 | 変更 |
| --- | --- |
| `GET /api/library` | `unshelved` (boolean) を得る: どのホームの棚にもない動画だけ ([R-11](../research.md#r-11-unshelved-means-matching-no-home-shelf-computed-on-read))。ゲストも使える |
| `SmartFolder` (`GET /api/smart-folders/{id}` のフォルダ) | フォルダのページの加える操作と外す操作のために、`onHome` (boolean、所有者だけ) を得る |
| 画面のイベント | ペイロードのない種類 `home` を得る ([R-16](../research.md#r-16-homechanged-tells-open-screens-to-read-the-home-again))。所有者とゲストに送る |
