---
source: specs/052-smart-folders/contracts/screen-api.md
sourceHash: 0b4807fe08bde705097a27e9871263537e7f789a1de51ebf6d93698577867098
---

# 契約: スマートフォルダの画面の API {#contract-screen-api-for-smart-folders}

親 Issue: #1139。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは、[048](../../048-groups/contracts/screen-api.md) のグループのルート、[050](../../050-drag-organize/contracts/screen-api.md) の整理のルート、ライブラリの一覧のパラメーターについて、この機能が加えるか変えるものだけを挙げる。`task generate` が `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API (`api/external-v1.yaml`) と MCP は変わらない。そこには AI による整理の Issue がスマートフォルダを加える。

アクセスは 048 に従う: 読み取りは所有者とゲスト、書き込みは所有者だけ。所有者だけと記した項目は、ゲストには含めない。

## 加えるスキーマ {#schemas-added}

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

## 変えるスキーマ {#schemas-changed}

| スキーマ | 変更 |
| --- | --- |
| `LibraryGroup` | `smart: boolean` が加わる。スマートフォルダでは true で、ゲストにも示す |
| `GroupDetail` | `smartFolder: SmartFolder` が加わる (所有者だけ、スマートフォルダだけ) |
| `GroupItem` | `id` は省略できるようになる: 項目の行のない条件に合う動画には id がない。`manual: boolean` が加わる (所有者だけ、スマートフォルダだけ): 動画は項目によって中にあり、条件に合わない |
| `GroupCreate` | 省略できる `rule: SmartRule` と `sort: VideoSort` が加わる。`rule` があるとグループはスマートフォルダになる |
| `GroupRename` (`PATCH /api/groups/{id}`) | `name` は省略できるようになる。省略できる `rule` と `sort` が加わる。3 つのうち少なくとも 1 つが必須である |
| `GroupItemRemove` | `itemIds` は省略できるようになる。`videoIds` (0 から 20,000) が加わる。少なくとも 1 つが空でない |

## 加えるルート {#routes-added}

| ルート | 目的 |
| --- | --- |
| `GET /api/groups/{id}/excluded` | 所有者だけ。所有者がスマートフォルダから取り除いた動画を、`cursor` 付きの `VideoPage` として返す。グループでは空 |

## 加える一覧のパラメーター {#list-parameters-added}

`listVideos`、`listLibrary`、`listLibraryIds` に、`SmartRule` の範囲を持つ `addedWithin`、`minDuration`、`maxDuration` が加わる。ゲストもそれらを送れる ([research.md R-4](../research.md#r-4-the-library-filter-gains-added-within-and-length))。範囲外の値、または `minDuration` が `maxDuration` を超えることは `400 invalid_request` である。

## ステータス {#statuses}

| ルート | ステータス | `code` | 条件 |
| --- | --- | --- | --- |
| `POST /api/groups`、`PATCH /api/groups/{id}` | 400 | `invalid_smart_rule` | `rule` が `domain.SmartRule` に通らないか、`sort` が `random` である |
| `PATCH /api/groups/{id}` | 409 | `group_not_smart` | スマートフォルダでないグループへの `rule` か `sort` |
| `POST /api/groups/{id}/items/{itemId}/move`、`POST /api/organize/reorder` | 409 | `smart_folder_sorted` | 移動した項目がスマートフォルダの中の動画である |
| `POST /api/organize/group` | 409 | `smart_folder_sorted` | `from` がスマートフォルダである |
| `POST /api/organize/merge` | 409 | `smart_folder_merge` | 統合した項目がスマートフォルダである |
| `POST /api/groups/{id}/items/remove` | 404 | `group_target_not_found` | `videoIds` の要素がフォルダのメンバーでない。何も書かない |
| `POST /api/groups/{id}/items/remove` | 200 | なし | スマートフォルダの最後のメンバーを取り除くと、解散したグループへの 048 の `204` の代わりに、空のフォルダの `LibraryGroup` で応答する ([R-8](../research.md#r-8-an-empty-smart-folder-stays-and-is-shown-to-the-owner-only)) |
| `POST /api/organize/move` | 200 | なし | 空になったスマートフォルダの `from` は残る |

存在しない id を挙げる `rule` は受け付け、`missing` で報告する。そのため、タグが削除された直後の絞り込みから保存したフォルダは拒まれない。
