---
source: specs/017-folder-groups/contracts/folder-groups-api.md
sourceHash: 2216ac5ab64179fb8829be8895072fb61a24f166c7f37923c923f2d289395ea4
---

# 契約: フォルダのグループ化、動画ページのグループ、タグの応答 {#contract-folder-grouping-groups-on-the-video-page-and-tag-responses}

正本は `api/openapi.yaml` である。この文書は、追加するルートと変わるスキーマだけを扱う。フォルダは、既存の `FolderRootId` と `FolderPath` パラメータと同じ方法 (`rootId` と `path`) で指定する。

## 1. フォルダのグループ化 {#1-folder-grouping}

```yaml
FolderGrouping:
  required: [mode, grouped, taggable]
  properties:
    mode: { type: string, enum: [auto, ungroup, groupDirect] }  # auto = no override
    grouped: { type: boolean }   # whether this folder's direct videos are a group now
    taggable: { type: boolean }  # a group, and not the registered folder itself (§2)
```

- `FolderSummary` に `grouping: FolderGrouping` が加わる。所有者への応答には常にあり、ゲストへの応答では省く (省略可能なフィールド)。フォルダ画面のメニューはこれから項目を決め、ゲストには表示しない。
- この節と §2 で状態を変えるルートは所有者だけが使える (既定の `security`)。
- `PUT /api/folders/{rootId}/grouping?path=…`、本文 `{ "mode": "auto" | "ungroup" | "groupDirect" }`:

| ステータス | 条件 |
| --- | --- |
| 200 | 変更後の `FolderGrouping`。`auto` は上書き設定を取り除く。同じ値を再び設定しても 200 |
| 400 | パスまたは `mode` が不正 |
| 404 | フォルダが存在しない (`getFolder` と同じ確認) |

- 上書き設定の保存と索引の再構築は 1 つのトランザクションの中で起きる ([data-model.md §3](../data-model.md#3-when-the-index-is-rebuilt))。後のリクエストが勝つ (Edge Case `同時操作`)。

## 2. グループをタグに変える {#2-turning-a-group-into-a-tag}

`POST /api/folders/{rootId}/grouping/tag?path=…` (本文なし)

| ステータス | `code` | 条件 |
| --- | --- | --- |
| 200 | — | `{ "tag": TagRef, "created": boolean, "grouping": FolderGrouping }`。タグを新しく作ったとき `created` は true |
| 400 | `invalid_request` | フォルダ名がタグ名の規則 (`NormalizeTagName`) に反する。タグも上書き設定も作らない |
| 404 | — | フォルダが存在しない |
| 409 | `conflict` | フォルダが今はグループでない、または登録フォルダそのものである (登録フォルダの名前はフォルダ由来のタグとして照合しない、[data-model.md §4](../data-model.md#4-folder-derived-tags)) |

- `FolderGrouping` に `taggable: boolean` (必須) が加わる。画面は、これが true のときだけ `グループをタグに変える` を表示する。
- タグの検索または作成、`ungroup` の保存、再構築は 1 つのトランザクションの中で起きる ([data-model.md §4](../data-model.md#4-folder-derived-tags))。

## 3. 動画のグループと関連動画 {#3-groups-of-videos-and-related-videos}

```yaml
VideoGroupRef:          # Video.group in GET /api/videos/{id} (only for members)
  required: [folder, name, position, count]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    position: { type: integer }   # 1-based
    count: { type: integer }

RelatedGroup:           # RelatedVideos.group (only for members)
  required: [folder, name, items, offset, total]
  properties:
    folder: { $ref: VideoFolder }
    name: { type: string }
    items: { type: array, maxItems: 100, items: { $ref: Video } }   # consecutive members around the current video, in order
    offset: { type: integer }   # 0-based position of items[0] in the group's order
    total: { type: integer }    # number of members

GroupMemberPage:        # GET /api/videos/{id}/group-members?offset=&limit=
  required: [items, offset, total]
  properties:
    items: { type: array, maxItems: 200, items: { $ref: Video } }
    offset: { type: integer }
    total: { type: integer }
```

- `Video.group` は (`location` と同じく) `GET /api/videos/{id}` の応答にだけあり、一覧の要素には決してない。
- ゲストには、`position`、`count`、`group.items`、`group.total`、前後の動画を公開されたメンバーだけから作る。公開されたメンバーが 1 本だけのときは `group` を省き、応答はどのグループにも属さない動画と同じになる ([data-model.md §7](../data-model.md#7-visibility-per-audience))。
- メンバーに対する `GET /api/videos/{id}/related`:
  - `group` を含む。`items` の 20 件の上限はグループには適用しない (Edge Case `大きなグループ`)。`group.items` は、現在の動画を中央に置いた最大 100 本の連続したメンバーの窓で、グループの両端では内側に寄せる。そのため応答はグループとともに大きくならない (issue 674)。`offset` と `total` は、グループ全体の中での窓の位置を示す。
  - `nextId` と `prevId` は、グループの順序での次と前のメンバーである。最後のメンバーには `nextId` がなく、最初のメンバーには `prevId` がない。
  - `items` (関連動画) は、`domain.OrderRelated` の入力 (同じフォルダの動画と追加時刻の近い動画) から同じグループのメンバーを**先に**取り除き、それから今と同じ順に並べて作る。20 件の上限はその後に適用するので、20 本を超えるグループでも関連動画は残る。`VideosAddedNear` に渡す件数は、取り除いたメンバーの分だけ増える。
- `GET /api/videos/{id}/group-members` は、グループの順序で 0 始まりの `offset` (既定 0) から最大 `limit` (1 から 200、既定 100) 本のメンバーを、`group.items` と同じ形と同じ閲覧者の規則で返す。`offset` が `total` 以上なら空の `items` を返す。メンバーでない動画、またはグループが閲覧者に表示されない動画は `404` である。`offset` が 0 未満、または `limit` が範囲外なら `400` である。動画ページは、最初の窓の外のメンバーをこれで読む。
- `GET /api/videos/{id}` は、`position` と `count` を埋めるためにメンバーの ID だけを読む。
- グループ外の動画への応答は変わらない (要件 27 の後半、受け入れ条件 17)。

## 4. タグの応答の変更 {#4-changes-to-tag-responses}

```yaml
VideoTag:               # Element of Video.tags and LibraryGroup.tags (replaces TagRef)
  required: [id, name, manual, fromFolder]
  properties:
    id: { type: integer, format: int64 }
    name: { type: string }        # the original name
    manual: { type: boolean }     # has a part attached by hand
    fromFolder: { type: boolean } # attached from an ancestor folder's name
```

- ゲストには、`Video.tags` と `LibraryGroup.tags` は空の配列のままで、フォルダ由来のタグも含まない。
- `VideoTagsSummaryItem` に `manualCount` (必須、そのタグを手で付けた動画の数) が加わる。`count` は、どちらの経路であれそのタグを持つ動画の数である。
- `Tag.videoCount`、`tag` フィルタ、検索ボックスでのタグ名の照合は、フォルダ由来のタグを含む ([data-model.md §4](../data-model.md#4-folder-derived-tags))。
- `POST /api/video-tags` の `remove` は、手で付けたタグだけを取り除く。応答 (`tag`、`applied`) は変わらない。
