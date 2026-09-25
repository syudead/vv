# Contract: フォルダのまとめ方・再生画面のグループ・タグの応答

正本は `api/openapi.yaml` で、この文書は足す経路と変わるスキーマだけを書く。フォルダの指し方
（`rootId` と `path`）は既存の `FolderRootId`・`FolderPath` パラメータと同じ。

## 1. フォルダのまとめ方

```yaml
FolderGrouping:
  required: [mode, grouped]
  properties:
    mode: { type: string, enum: [auto, ungroup, groupDirect] }  # auto = 例外なし
    grouped: { type: boolean }   # いまこのフォルダの直下がグループか
```

- `FolderSummary` に `grouping: FolderGrouping` を足す（必須）。フォルダ画面のメニューはこれで出す項目を決める。
- `PUT /api/folders/{rootId}/grouping?path=…`、本文 `{ "mode": "auto" | "ungroup" | "groupDirect" }`
  - 200: 変更後の `FolderGrouping`。`auto` は例外を消す。同じ値の再設定も 200。
  - 400: パスや `mode` の誤り。404: そのフォルダが無い（`getFolder` と同じ判定）。
  - 例外の保存と索引の作り直しは1つの取引（[data-model.md §3](../data-model.md#3-作り直す時点)）。
    後の要求が勝つ（Edge Case「同時操作」）。

## 2. グループをタグに変える

`POST /api/folders/{rootId}/grouping/tag?path=…`（本文なし）

- 200: `{ "tag": TagRef, "created": boolean, "grouping": FolderGrouping }`。`created` は新しく作ったとき true。
- 400 `invalid_request`: フォルダ名がタグ名の規則（`NormalizeTagName`）に合わない。タグも例外も作らない。
- 404: フォルダが無い。409 `conflict`: そのフォルダが今グループでない。
- タグの引き当て・作成、`ungroup` の保存、作り直しは1つの取引（[data-model.md §4](../data-model.md#4-フォルダ由来のタグ)）。

## 3. 動画と関連動画のグループ

```yaml
VideoGroupRef:          # GET /api/videos/{id} の Video.group（メンバーのときだけ）
  required: [folder, name, position, count]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    position: { type: integer }   # 1 始まり
    count: { type: integer }

RelatedGroup:           # RelatedVideos.group（メンバーのときだけ）
  required: [folder, name, items]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    items: { type: array, items: { $ref: Video } }   # 全メンバー、並びの順、今の動画を含む
```

- `Video.group` は `GET /api/videos/{id}` の応答にだけ入る（`location` と同じ扱い）。一覧の項目には入らない。
- メンバーの `GET /api/videos/{id}/related`:
  - `group` を入れる。`items` の上限 20 はグループには掛けない（Edge Case「大きなグループ」）。
  - `nextId`・`prevId` はグループの中の並びの次と前。最後のメンバーに `nextId`、最初のメンバーに `prevId` は無い。
  - `items`（関連動画）は、同じグループのメンバーを `domain.OrderRelated` の入力（同じフォルダの動画と
    追加日時の近い動画）から**先に**除いてから今の並べ方で並べたもの。上限 20 件はその後に掛けるので、
    20 本を超えるグループでも関連動画が残る。`VideosAddedNear` に渡す件数も、除く本数を見込んで増やす。
- グループに属さない動画の応答は今と同じ（要件 27 の後半・受け入れ条件 17）。

## 4. タグの応答の変更

```yaml
VideoTag:               # Video.tags と LibraryGroup.tags の要素（TagRef を置き換える）
  required: [id, name, manual, fromFolder]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }        # 元の名前
    manual: { type: boolean }     # 手で付けた分がある
    fromFolder: { type: boolean } # 祖先フォルダの名前から付いている
```

- `VideoTagsSummaryItem` に `manualCount`（必須、手で付けた本数）を足す。`count` はどちらかの出所で付いている本数。
- `Tag.videoCount`、`tag` での絞り込み、検索欄のタグ名の照合は、フォルダ由来の分を含む
  （[data-model.md §4](../data-model.md#4-フォルダ由来のタグ)）。
- `POST /api/video-tags` の `remove` は手で付けた分だけを外す。応答（`tag`・`applied`）は変えない。
