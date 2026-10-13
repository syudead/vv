---
source: docs/how-to/external-api.md
sourceHash: bdf48c1a061523d62fdccbbfd3da5ae9a7196411a305c41caa7ef56d5b2c64d7
---

# 外部 API を使う {#use-the-external-api}

API トークンがあれば、スクレイパーなどの外部ツールは `/api/v1` または `/mcp` の MCP サーバーを通じて、動画一覧を読み、スキャンを開始し、タグ、表示名、カバーサムネイル、作品情報を設定できる。判断の理由は [specs/026-external-api/research.md](../../specs/026-external-api/research.md) にある。

外部ツールは `/api/v1` と `/mcp` だけを使う。画面用の `/api/*` は予告なく変わる。

```mermaid
flowchart LR
  tool[外部ツール] -->|Bearer トークン| v1["/api/v1"]
  mcp[MCP クライアント] -->|Bearer トークン| mcpep["/mcp"]
  screens[画面] -->|cookie| screenapi["/api/*"]
  v1 --> vv[VVMDM]
  mcpep --> vv
  screenapi --> vv
```

## 契約 {#contract}

[api/external-v1.yaml](../../api/external-v1.yaml)（OpenAPI 3.1）が、操作、フィールド、エラーの `code` と `reason` の値の正となる情報源である。サーバーはこのファイルを配信しない。画面用の契約は [api/openapi.yaml](../../api/openapi.yaml) である。

## 互換性の方針 {#compatibility-policy}

| 変更 | 反映先 |
| --- | --- |
| 新しいフィールド、操作、`code` または `reason` | `v1` に追加する。クライアントは知らない値を読み飛ばす |
| 意味、型、必須かどうかの変更、または操作の削除 | 新しい `v2`。`v1` は変えない |

## トークンを作成する {#create-a-token}

1. 所有者としてサインインし、Settings ページの "API tokens" セクションを開き、トークンの用途を表す名前を入力してトークンを作成する。
2. 平文のトークン（`vvt_` で始まる 47 文字）をその場でコピーする。表示されるのは一度だけである。失くした場合は失効させ、新しく作成する。
3. 使わなくなったトークンは同じセクションで失効させる。ユーザー名またはパスワードを変更する（`mdm account`）と、すべてのトークンが失効する。

トークンは非公開の動画の読み取りを含め、所有者の権限を持つ。パスワードと同じように保管する。

## API を呼び出す {#call-the-api}

すべてのリクエストに `Authorization: Bearer <token>` を付ける。cookie は読まない。

```sh
BASE=http://localhost:8080
TOKEN=vvt_...

curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/tags"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=100"
curl -X POST -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans/current"
```

| 場合 | 挙動 |
| --- | --- |
| トークンがない、形式が不正、無効、または失効済み | `WWW-Authenticate: Bearer` 付きの `401`（`code: unauthenticated`）。原因は区別しない |
| リクエストが `Origin` を送る | 受け付ける。同一オリジンの検査はない |
| 操作が本文を取る | `Content-Type: application/json` が必要 |
| リクエスト中にトークンが失効する | リクエストは打ち切られる |

- エラー本文は `{ code, message, reason?, limit?, index?, field?, tagId?, tagName? }` である。`message` は英語の文である。分岐には `code` と `reason` を使う。`tagId` と `tagName` はタグ名の衝突のときだけ付く（[タグを整理する](#tidy-up-tags)）。`field` は作品情報のエラーのときだけ付く（[作品情報を設定する](#set-work-information)）。
- Settings ページのトークン一覧は最終使用時刻を表示する。この時刻の更新は最大で 1 分に 1 回である。

## スキャン {#scans}

`POST /api/v1/scans` は本文を取らない。スキャンを開始したときは `201` を、すでに実行中のスキャンがあるときはそのスキャンとともに `200` を、メディアフォルダが登録されていないときは `409`（`media_folders_not_configured`）を返す。

`GET /api/v1/scans/current` は最新のスキャンを返し、まだ一度も実行されていなければ `404`（`no_scan`）を返す。その `status` は次のように遷移する。`done`、`partial`、`failed` ではスキャンは終了している。

```mermaid
stateDiagram-v2
  finding: finding、videos と settledVideos は null
  running: running、スキャン中または動画の処理中
  done: done
  partial: partial、一部の項目が失敗
  failed: failed、スキャン自体が失敗
  [*] --> finding
  finding --> running
  running --> done
  finding --> done
  running --> partial
  finding --> partial
  finding --> failed
  running --> failed
```

## 動画一覧を読む {#read-the-video-list}

`GET /api/v1/videos` は、登録済みフォルダの下にある動画を、非公開のものも含めて、追加の古い順（`addedAt`、次に `id`）に返す。`limit` はページサイズを決める。既定は 100、範囲は 1 から 200 で、範囲外は `400` になる。

応答は `{ items, nextCursor }` である。空でない `nextCursor` は、次のページの `cursor` としてそのまま渡す。空文字列は終わりを表す。カーソルを解析しない。解析できないカーソルは `400`（`invalid_cursor`）を返す。そのときは最初からやり直す。

```sh
cursor=""
while :; do
  page=$(curl -s -G -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos" \
    --data-urlencode "cursor=$cursor")
  echo "$page" | jq -c '.items[]'
  cursor=$(echo "$page" | jq -r '.nextCursor')
  [ -z "$cursor" ] && break
done
```

| 場合 | 挙動 |
| --- | --- |
| スキャン後の新しい動画 | 追跡しない。一覧を最初から読み直す |
| ページング中に動画が追加、削除、移動された | 次のリクエストも成功する。動画が抜けたり重複したりすることがある |
| 動画が削除された | 一覧から消える。削除を伝えるフィールドはない |
| 動画が登録済みフォルダの外にしか残っていない | 削除されたものとして扱う |

保持している動画がまだ存在するか確かめるには、[その動画を検索する](#look-up-one-video)。`404`（`video_not_found`）は存在しないことを表す。

| フィールド | 意味 |
| --- | --- |
| `locations` | 登録済みフォルダの下の現在の場所。パス順に並び、最初が主な場所である |
| `title` | 設定されていれば `displayName`、なければ主な場所のファイル名から得た `fileTitle` |
| `displayName` | 表示名。未設定なら `null` |
| `thumbnailPositionMs` | カバーサムネイルの位置（ミリ秒）。未設定なら `null` |
| `durationMs` | 解析前は `null` |
| `tags.manual` | 手で付けたタグ |
| `tags.fromFolder` | 祖先フォルダの名前から得たタグ |
| `tags.tentative` | 仮のタグ（[タグを仮のタグとして付ける](#add-tags-as-tentative-tags)） |
| `updatedAt` | 表示名、タグ、公開状態、カバーサムネイルの最後の編集時刻。この API による編集も含む。一度も編集されていなければ `addedAt` と等しい。何も変えないリクエストでは変わらない |
| `fileCreatedAt` | 主な場所のファイルの作成時刻。ファイルシステムが作成時刻を持たないときは mtime |

## 1 本の動画を検索する {#look-up-one-video}

`GET /api/v1/videos/lookup` は `id`、`contentKey`、`path` のうちちょうど 1 つを取る。どれもないか 2 つ以上あると `400` を返す。応答は一覧の項目と同じ形である。

```sh
curl -G -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos/lookup" \
  --data-urlencode "path=/media/videos/clip.mp4"
```

- `path` は正規化せずにバイト単位で比較する。NFD で綴られた名前（macOS など）には、一覧の `locations[].path` をそのまま渡す。
- 存在しない動画、または登録済みフォルダの下に場所を持たない動画は `404`（`video_not_found`）を返す。

## 動画にタグを付ける {#tag-videos}

`POST /api/v1/video-tags` は、1 回のリクエストで複数の動画に対し、名前でタグを追加、削除、または置き換える。

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }, { "contentKey": "…" }, { "id": 12 }],
  "action": "add",
  "tags": ["猫", "ねこ"] }
```

どの action も手で付けたタグだけを変える。`fromFolder` のタグは残る。

| `action` | 効果 |
| --- | --- |
| `add` | 指定した名前のタグを追加する。知らない名前にはタグを作成する |
| `remove` | 指定した名前のタグを削除する。知らない名前は何もしない |
| `replace` | 手で付けたタグをちょうど `tags` にする。知らない名前は作成する。空の `tags` はすべて削除する |

| 項目 | 規則 |
| --- | --- |
| `videos` の要素 | `id`、`contentKey`、`path` のうちちょうど 1 つ。[検索](#look-up-one-video) と同じように解決する |
| `videos` の数 | 1 から 20000。範囲外は `limit` 付きの `400` `too_many_videos` |
| `tags` | 名前または同義語。同じタグに解決される名前はまとめる。最大 100、`add` と `remove` では 1 以上。範囲外は `limit` 付きの `400` `too_many_tags` |
| タグ名のエラー | `400` `tag_name_empty`、`tag_name_control_characters` または `tag_name_too_long`。`index` は `tags` 内の位置 |
| 本文のサイズ | 最大 32 MiB（33554432 バイト）。超えると `400` `invalid_request`。長いパスが多いときはリクエストを分割する |
| 知らない動画 | `404` `video_not_found`。`index` は `videos` 内の位置 |

- 応答は `{ items: [{ video: { id, contentKey }, tags }], skippedTags }` である。項目は `videos` の順に動画ごとに 1 つで、操作後のタグを一覧の項目と同じ形で持つ。
- リクエストは 1 つのトランザクションで実行される。エラーがあれば何も適用せず、タグも作成しない。
- 同じリクエストを繰り返しても状態は変わらず、`200` を返す。そのため、ネットワーク障害の後はそのまま再送する。

### タグを仮のタグとして付ける {#add-tags-as-tentative-tags}

`"tentative": true` を指定すると、`add` と `replace` が新しく作成するタグは **仮のタグ** になる。画面では、ユーザーが Tags ページで確定または却下するまで仮として表示される。

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }],
  "action": "add",
  "tags": ["猫", "高画質"],
  "tentative": true }
```

`tentative` は真偽値で、既定は `false` である。それ以外の値は `400`（`invalid_request`）を返す。`remove` では何も変えない。`tags` の各名前は次のように解決される。

```mermaid
flowchart LR
  name[tags の名前] --> known{タグか同義語に一致?}
  known -->|はい| addexisting[そのタグを追加]
  known -->|いいえ| tent{tentative が true?}
  tent -->|いいえ| confirmed[確定タグを作成]
  tent -->|はい| rejected{却下済みの名前?}
  rejected -->|いいえ| tentative[仮のタグを作成]
  rejected -->|はい| skipped[飛ばして skippedTags に記録]
```

- 既存のタグは、仮か確定かの状態を保つ。
- 却下済みの名前は綴りの完全一致で照合する。その名前を確定タグとして作成すると、却下済みの名前から除かれる。
- 飛ばした名前でリクエストは失敗せず（`200`）、`replace` の結果にも含まれない。`skippedTags` は飛ばした名前を正規化して `tags` の順に 1 回ずつ並べ、何も飛ばさなかったときは空の配列である。
- 応答と `GET /api/v1/tags` の各タグは `tentative` を返す。
- 仮のタグを確定または却下し、却下済みの名前を一覧または消去するには、[タグを整理する](#tidy-up-tags) に従う。

## タグを整理する {#tidy-up-tags}

### タグを一覧する {#list-tags}

`GET /api/v1/tags` は、タグ管理画面と同じ検索、絞り込み、並べ替え、ページでタグを一覧する（[specs/039-external-tag-admin/contracts/external-api.md、`GET /api/v1/tags`](../../specs/039-external-tag-admin/contracts/external-api.md#get-apiv1tags)）。

```sh
curl -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/v1/tags?tentative=true&q=selfie&sort=countDesc&limit=200"
```

| パラメータ | 意味 |
| --- | --- |
| `q` | 100 文字まで。全角半角、大文字小文字、かなの違いを無視して、名前または同義語の一部に一致する |
| `tentative` | `true` で仮のタグだけを一覧する |
| `unused` | `true` でどの動画にも付いていないタグだけを一覧する。`q` と `tentative` との AND |
| `sort` | `name`（既定、名前の自然順）、`countDesc`、`countAsc`、`createdDesc` または `createdAsc`。同順位は名前、次に `id` で並べる |
| `limit` | 1 ページあたり 1 から 200 個のタグ。**省略すると、一致するすべてのタグを** `nextCursor` なしで返す |
| `cursor` | 前回の `nextCursor`。同じ `q`、`tentative`、`unused`、`sort` とともに送る |

- 応答は `{ items, total, totalAll, nextCursor? }` である。どのページでも、`total` は絞り込みに一致するタグの数、`totalAll` はすべてのタグの数である。
- `limit` を付けたときは、`nextCursor` がなくなるまでそれを `cursor` として渡し返すと、一致するすべてのタグを 1 回ずつ読める。
- 各タグは `id`、`name`、`synonyms`、`videoCount`、`tentative`、`createdAt` を持つ。
- 5 つの値以外の `sort`、1 から 200 の範囲外の `limit`、または 100 文字を超える `q` は `400` `invalid_request` を返す。読めないカーソル、または別の `sort` で作られたカーソルは、`reason: invalid_cursor` 付きの `400` `invalid_request` を返す。そのときは最初のページから読み直す。

### タグを統合する {#merge-tags}

`POST /api/v1/tags/merge` は、1 つのトランザクションで統合元のタグを統合先のタグへ統合する（[契約、`POST /api/v1/tags/merge`](../../specs/039-external-tag-admin/contracts/external-api.md#post-apiv1tagsmerge)）。

```json
{ "targetId": 12, "sourceIds": [31, 45] }
```

- 各統合元の名前と同義語は統合先の同義語になり、その動画は統合先へ移り、統合元は削除される。統合先は確定タグになる。
- 応答は `{ tag, notFoundIds }` である。統合後の統合先と、存在せず飛ばした統合元を持つ。
- `sourceIds` は重複も数えて 1 から 20000 個の id を持つ。範囲外は `limit` 付きの `400` `too_many_tags` を返す。`targetId` を含む `sourceIds` は `400` `merge_same_tag` を、存在しない統合先は `404` `tag_not_found` を返す。どちらも何も変えない。

### タグの名前を変える {#rename-a-tag}

`{ "id": 12, "name": "自撮り" }` を付けた `POST /api/v1/tags/rename` は、タグの元の名前を変え、タグを返す（[契約、`POST /api/v1/tags/rename`](../../specs/039-external-tag-admin/contracts/external-api.md#post-apiv1tagsrename)）。

- 仮のタグは名前が変わると確定タグになる。今と同じ名前は何も変えない。
- 名前の規則に反する名前は `400` `tag_name_empty`、`tag_name_control_characters` または `tag_name_too_long` を返す。存在しない `id` は `404` `tag_not_found` を返す。
- 別のタグの名前か同義語である名前、またはこのタグの同義語の 1 つである名前は、`reason: tag_name_taken` 付きの `409` `conflict` を返し、`tagId` と `tagName` はその名前を持つタグを示す。何も変わらない。

### 同義語を編集する {#edit-synonyms}

`POST /api/v1/tags/synonyms` は、タグの同義語に名前を 1 つ追加または削除し、変更後のタグを返す（[契約、`POST /api/v1/tags/synonyms`](../../specs/039-external-tag-admin/contracts/external-api.md#post-apiv1tagssynonyms)）。

```json
{ "id": 12, "action": "add", "name": "自己撮影" }
```

| 場合 | 結果 |
| --- | --- |
| 新しい名前の `add` | `200`。名前は同義語になり、仮のタグは確定タグになる |
| すでに `id` の同義語である名前の `add` | `200`。変更なし |
| `id` 自身の名前、または別のタグの同義語の `add` | `tagId` と `tagName` 付きの `409` `tag_name_taken` |
| 別のタグの元の名前の `add` | そのタグの `tagId` と `tagName` 付きの `409` `tag_merge_required`。何も変わらない |
| 同じ `add` に `"mergeTagId": <tagId>` を付ける | `200`。そのタグは `id` へ統合される |
| `remove` | `200`。名前は同義語でなくなる。同義語でなかったときはタグは変わらない |

- `add` と `remove` 以外の `action` は `400` `invalid_request` を、存在しない `id` は `404` `tag_not_found` を返す。
- `mergeTagId` は統合を受け入れるタグを示す。再試行の前に別のクライアントがその名前を別のタグに与えた場合、見ていないタグを統合せず、呼び出しは再び `tag_merge_required` で失敗する。

### タグを確定、却下、削除する {#confirm-reject-and-delete-tags}

`POST /api/v1/tags/batch` は、1 つのトランザクションでタグを確定、却下、または削除する（[契約、`POST /api/v1/tags/batch`](../../specs/039-external-tag-admin/contracts/external-api.md#post-apiv1tagsbatch)）。1 つのタグだけを対象とする操作はない。1 つのタグには `"ids": [id]` を送る。

```json
{ "action": "reject", "ids": [31, 45, 9999] }
```

| `action` | 対象 | 効果 |
| --- | --- | --- |
| `confirm` | 仮のタグ | タグは確定タグになる |
| `reject` | 仮のタグ | タグは削除され、その元の名前は却下済みの名前に入る |
| `delete` | 確定タグ | タグは削除される。その名前は記憶されない |

- 応答は `{ appliedIds, notFoundIds, notApplicableIds }` である。3 つのリストは重ならず、合わせて送った各 id を 1 回ずつ持ち、`ids` の順を保つ。`reject` に送った確定タグのような種類の違うタグは、変更されずに残り、`notApplicableIds` に入る。
- `ids` は重複も数えて 1 から 20,000 個の id を持ち、そうでなければ `limit` 付きの `400` `too_many_tags` を返す。3 つの値以外の `action` は `400` `invalid_request` を返す。トランザクションが失敗すると `500` `internal` を返し、何も変えない。

### 却下済みの名前 {#rejected-names}

仮のタグとしての付与は、却下済みの名前を飛ばす（[タグを仮のタグとして付ける](#add-tags-as-tentative-tags)）。

- `GET /api/v1/tags/rejected-names` は名前の自然順で `{ items, total, nextCursor? }` を返す（[契約、`GET /api/v1/tags/rejected-names`](../../specs/039-external-tag-admin/contracts/external-api.md#get-apiv1tagsrejected-names)）。`limit` は 1 から 200 で、既定は 100 である。`nextCursor` がなくなるまでそれを `cursor` として渡し返す。`total` はすべての却下済みの名前の数である。範囲外の `limit` は `400` `invalid_request` を返し、読めないカーソルは `reason: invalid_cursor` を加える。
- `DELETE /api/v1/tags/rejected-names?name=…` は名前を 1 つ除き、次にその名前を仮のタグとして付けるとタグが再び作成される（[契約、`DELETE /api/v1/tags/rejected-names?name=…`](../../specs/039-external-tag-admin/contracts/external-api.md#delete-apiv1tagsrejected-namesname)）。応答は `{ name, removed }` で、照合した正規化後の名前と、名前がリストになく何も変わらなかったときの `false` である。`name` がないと `400` `invalid_request` を返す。

### 例: 表記の揺れを統合する {#example-merge-spelling-variants}

エージェントは仮のタグをページごとに読み、同じ意味の名前をまとめ、各グループを 1 つのタグへ統合する。

```mermaid
sequenceDiagram
  participant A as エージェント
  participant V as VVMDM
  A->>V: GET /api/v1/tags?tentative=true&limit=200
  A->>V: nextCursor がなくなるまで GET
  A->>V: グループごとに POST /api/v1/tags/merge
  A->>V: 残りを確定または却下する POST /api/v1/tags/batch
```

1. `tentative=true&limit=200` ですべての仮のタグを読む。`nextCursor` がなくなるまでそれを `cursor` として渡し返す。
2. `selfie`、`セルフィー`、`自撮り` のような 1 つの名前の揺れをまとめ、最も多くの動画に付いたタグなど、各グループの統合先を選ぶ。
3. `POST /api/v1/tags/merge` で各グループを統合する。統合元の名前は統合先の同義語になるので、以後その綴りで付けると統合先に届く。
4. `POST /api/v1/tags/batch` で、残すタグを確定し、残りを却下する。

```sh
# 1. List the tentative tags, most used first.
curl -s -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/v1/tags?tentative=true&sort=countDesc&limit=200" |
  jq -r '.items[] | [.id, .name, .videoCount] | @tsv'

# 3. Merge the variants into the target 12.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/tags/merge" -d '{"targetId":12,"sourceIds":[31,45]}'

# 4. Reject the names that are not tags.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/tags/batch" -d '{"action":"reject","ids":[77,78]}'
```

## 表示名を設定する {#set-display-names}

`POST /api/v1/video-display-names` は、複数の動画の表示名を設定または消去する。表示名は画面とこの API の `title` に表示され、並べ替えと検索に使われる。ファイル名は変わらない。

```json
{ "items": [{ "video": { "path": "/media/videos/clip.mp4" }, "displayName": "旅行 2024 夏" },
            { "video": { "id": 12 }, "displayName": null }] }
```

- `video` は `id`、`contentKey`、`path` のうちちょうど 1 つを持つ。`displayName` は必須である。`null` または前後の空白を除くと空になる文字列は表示名を消去し、タイトルは `fileTitle` に戻る。項目は 1 から 20000 で、本文は 32 MiB までに制限される。
- 表示名は内容（`contentKey`）に属する。移動しても残り、同じ内容を持つすべての場所に表示される。複数の動画が同じ名前を共有できる。
- 応答は `{ items: [{ video: { id, contentKey }, title, fileTitle, displayName }] }` で、変更後の値を `items` の順に持つ。
- リクエストは 1 つのトランザクションで実行される。次のどのエラーでも何も適用せず、`items` 内の `index` を返す。

  | 条件 | エラー |
  | --- | --- |
  | 動画が見つからない | `404`（`video_not_found`） |
  | 名前に制御文字が含まれる | `400`（`display_name_control_characters`） |
  | 名前が 200 文字より長い | `400`（`display_name_too_long`、`limit: 200`） |

## カバーサムネイルの位置を変える {#change-the-cover-thumbnail-position}

`POST /api/v1/video-thumbnails` は、複数の動画のカバーサムネイルを指定の位置から生成し直す。

```json
{ "items": [{ "video": { "contentKey": "…" }, "positionMs": 12500 },
            { "video": { "id": 12 }, "positionMs": null }] }
```

- `positionMs` は必須で、先頭からのミリ秒である。0 以上で、長さ未満でなければならない。`null` は位置を消去し、自動の位置で生成し直す。
- 各項目が画像を生成するため、1 回のリクエストで取れる項目は 1 から 20 である。それより多いと `400`（`too_many_videos`、`limit: 20`）を返す。それより大きな集合は 20 件ずつ順に送る。
- 応答は `{ items: [{ video: { id, contentKey }, thumbnailPositionMs }] }` である。生成の後に返り、生成には項目ごとに数秒かかることがある。

リクエストはすべての項目を検証してから、`items` の順に 1 項目ずつ生成し、最初の失敗で止まる。

```mermaid
flowchart LR
  req[リクエスト] --> valid{全項目が有効?}
  valid -->|いいえ| none[エラー、何も適用しない]
  valid -->|はい| item[次の項目]
  item --> gen{再読み込みと生成に成功?}
  gen -->|はい| more{残りの項目あり?}
  more -->|はい| item
  more -->|いいえ| ok[200]
  gen -->|いいえ| stop[index 付きのエラー]
```

| 条件 | エラー |
| --- | --- |
| 動画が見つからない | `404`（`video_not_found`） |
| 動画のどの場所も開けない | `404`（`file_unavailable`） |
| 位置が負、または長さ以上 | `400`（`thumbnail_position_out_of_range`、長さは `limit`） |
| 動画がまだ解析されていない | `409`（`duration_unknown`） |
| 画像を生成できない | `409`（`thumbnail_frame_unavailable`） |
| 生成中の予期しない失敗 | `500` |

どのエラーも `index` を持つ。生成中にスキャンが動画を変えることがあるため、各項目は生成の直前に動画と場所を読み込み直す。そのため、検証エラーでもリクエストが途中で止まることがある。止まったとき、`index` より前の項目は適用され、その項目と残りは適用されない。続けるには、`index` の項目を直すか除き、`index` 以降を再送する。適用済みの項目を再送しても、同じ位置で生成し直すだけである。

## 作品情報を設定する {#set-work-information}

動画の作品情報は、そのコード、説明、日付、人物、組織、URL、外部 ID である。人物と組織はそれ自体がエンティティであり、動画はそれを id で参照する。正本は [specs/047-video-work-info/contracts/external-api.md](../../specs/047-video-work-info/contracts/external-api.md) である。

### 動画に書き込む {#write-videos}

`POST /api/v1/video-work-info` は、複数の動画の作品情報を置き換える。

```json
{ "items": [ { "video": { "path": "/media/a.mp4" },
               "workInfo": { "code": "ABC-123",
                             "dates": [ { "kind": "release", "date": "2024-06" } ],
                             "people": [ { "role": "performer", "person": { "id": 7 } } ],
                             "organizations": [ { "role": "maker", "organization": { "id": 2 } } ],
                             "urls": [ "https://example.com/abc-123" ],
                             "externalIds": [ { "source": "example", "value": "abc123" } ] } },
             { "video": { "id": 12 }, "workInfo": {} } ] }
```

- 各 `workInfo` は保存したオブジェクトを置き換える。ない、`null`、または前後の空白を除くと空になるプロパティは消えるので、`{}` はすべてを消す。1 つのフィールドを変えるには、動画を読み、そのフィールドを変え、オブジェクト全体を送る。
- 1 つのトランザクションで 1 から 20000 項目。どのエラーでも何も適用せず、`items` の中の `index` を返す。同じ動画への後の項目が優先される。
- 応答は `items` の順の `{ items: [{ video: { id, contentKey }, workInfo }] }` である。同じリクエストを繰り返すと `200` を返し、`updatedAt` も含めて何も変えない。
- 動画のどの読み取り（`GET /api/v1/videos`、`GET /api/v1/videos/lookup`）も `workInfo` を持つ。この API では、設定していない `code` や `description` は `null` であり、空の一覧は `[]` である。

フィールドは次の規則に従う。全体の一覧は [data-model.md、フィールド](../../specs/047-video-work-info/data-model.md#fields) にある。

| フィールド | 規則 |
| --- | --- |
| `code` | 最大 100 文字 |
| `description` | 最大 10,000 文字。改行を許す |
| `dates[]` | `kind` は `release`、`recorded`、`published` のいずれかで、種類ごとに 1 つ。`date` は `YYYY`、`YYYY-MM`、`YYYY-MM-DD` のいずれか |
| `people[]` | `role` は `performer`、`director`、`photographer`、`streamer` のいずれか。`person.id` が存在する。役割ごとに最大 100 |
| `organizations[]` | `role` は `maker` か `label`。`organization.id` が存在する。役割ごとに最大 20 |
| `urls[]` | 2,048 文字以下の絶対 `http` または `https` URL を最大 20 個。重複なし |
| `externalIds[]` | 最大 20。`source` は最大 50 文字、`value` は最大 200 文字。重複なし |

どの文字列も制御文字を含んではならない。規則を破ると、`work_info_*` の理由、`field`（その項目の `workInfo` の中のパス。`people[0].person.id` など）、上限のときは `limit` を付けた `400` を返す。存在しない人物か組織の id は、`field` 付きの `404`（`person_not_found` または `organization_not_found`）を返す。見つからない動画は `404`（`video_not_found`）を返す。

### 人物と組織を管理する {#manage-people-and-organizations}

人物は `/api/v1/people`、組織は `/api/v1/organizations` の下にあり、同じ 5 つの操作を持つ。組織はさらに `parent` を持ち（根では `null`）、`parentId` を受け付ける。

| 操作 | リクエスト | 応答 |
| --- | --- | --- |
| 一覧 | `GET …?q=&sort=&cursor=&limit=`。[タグを一覧する](#list-tags) と同じ | `{ items, total, totalAll, nextCursor? }` |
| 照会 | `GET …?source=example&externalId=42` | その外部 ID を持つ 1 つのエンティティを入れた `items`、または空の `items` |
| 作成 | `{ name, aliases?, externalIds?, parentId? }` を付けた `POST …` | `200` とエンティティ |
| 更新 | `{ id, …fields }` を付けた `POST …/update` | `200` とエンティティ。ないフィールドはそのまま残り、与えた一覧は保存した一覧を置き換え、`null` は一覧を空にする |
| 統合 | `{ targetId, sourceIds }` を付けた `POST …/merge` | `{ person or organization, notFoundIds }`。統合元は統合先の別名になる |
| 削除 | `{ ids }` を付けた `POST …/delete` | `ids` の順の `{ deletedIds, notFoundIds }` |

- 外部 ID は、ある種類のエンティティを最大 1 つ指す。別のエンティティが持つ外部 ID でエンティティを作成または更新すると、`409`（`external_id_taken`、`field: externalIds[i]`）を返す。
- `sourceIds` と `ids` は 1 から 20000 の id を取る。その範囲の外は `400`（`work_info_too_many`、`field`、`limit`）である。統合元に `targetId` を含む統合は `400`（`merge_same_person` または `merge_same_organization`）である。

### 例: スクレイパーが見つけたものを書き込む {#example-write-what-a-scraper-found}

各出演者を外部サイトの ID で照会し、照会が空のときだけ人物を作成し、それから id を付けて動画を 1 回書き込む。

```mermaid
sequenceDiagram
  participant S as スクレイパー
  participant V as VVMDM
  S->>V: GET /api/v1/people?source=example&externalId=42
  alt items is empty
    S->>V: POST /api/v1/people
  end
  S->>V: 人物の id を付けて POST /api/v1/video-work-info
```

```sh
# 1. Look the performer up by the external ID.
ID=$(curl -s -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/v1/people?source=example&externalId=42" | jq '.items[0].id // empty')

# 2. Create the performer only when the lookup found none.
[ -n "$ID" ] || ID=$(curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" "$BASE/api/v1/people" \
  -d '{"name":"山田 花子","externalIds":[{"source":"example","value":"42"}]}' | jq '.id')

# 3. Write the video with the id.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-work-info" \
  -d '{"items":[{"video":{"contentKey":"…"},"workInfo":{"code":"ABC-123","people":[{"role":"performer","person":{"id":'"$ID"'}}]}}]}'
```

## 例: スクレイパーを連携させる {#example-integrate-a-scraper}

この流れは、新しくスキャンされた動画を外部サイトで調べ、見つかった情報でタグを付ける。

```mermaid
sequenceDiagram
  participant S as スクレイパー
  participant V as VVMDM
  participant E as 外部サイト
  S->>V: POST /api/v1/scans
  S->>V: 終了まで GET /api/v1/scans/current
  S->>V: 最初から GET /api/v1/videos
  S->>E: 新しい動画を検索
  S->>V: POST /api/v1/video-tags
  S->>V: POST /api/v1/video-display-names
```

1. スキャンを開始し、`status` が `done`、`partial` または `failed` になるまで待つ（[スキャン](#scans)）。
2. 一覧を最初から読み、記録にない `contentKey` を新しい動画として扱う。`contentKey` はファイルを移動しても残るため、記録のキーにする。
3. `locations[].path` または `title` で外部サイトを検索する。後で動画を確認し直すには、`GET /api/v1/videos/lookup?contentKey=…` を使う。
4. 見つかった名前を `POST /api/v1/video-tags` で追加する。同じ名前を付ける動画の集合ごとに 1 リクエストにする。LLM などが自動で生成した名前は、ユーザーが確定するように `"tentative": true` を付けて送る。却下済みの名前は再び作成されず、`skippedTags` に返る。
5. 見つかった正式なタイトルを、`POST /api/v1/video-display-names` で表示名に設定する。

```sh
# 2. Pick up contentKey and the primary path from the list (paging is in "Read the video list").
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=200" |
  jq -r '.items[] | [.contentKey, .locations[0].path] | @tsv'

# 4. Add the tags found. Names produced automatically are created as tentative tags.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-tags" \
  -d '{"videos":[{"contentKey":"…"}],"action":"add","tags":["猫","旅行"],"tentative":true}'

# 5. Set the title found as the display name.
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-display-names" \
  -d '{"items":[{"video":{"contentKey":"…"},"displayName":"旅行 2024 夏"}]}'
```

- `replace` は画面で手で付けたタグも上書きする。画面も使われるときは、`add` と `remove` で差分だけを送る。
- `404` のときは、`index` の動画を記録から除くか検索し直してから再送する。

## MCP から使う {#use-from-mcp}

`/mcp` の MCP サーバー（Streamable HTTP）は、同じ操作を同じトークンでツールとして公開する。

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

| ツール | 操作 |
| --- | --- |
| `list_videos` | `GET /api/v1/videos` |
| `get_video` | `GET /api/v1/videos/lookup` |
| `list_tags` | `GET /api/v1/tags`。ツールでは `limit` の既定は 100 |
| `update_video_tags` | `POST /api/v1/video-tags` |
| `start_scan` | `POST /api/v1/scans` |
| `get_current_scan` | `GET /api/v1/scans/current` |
| `update_video_display_names` | `POST /api/v1/video-display-names` |
| `update_video_thumbnails` | `POST /api/v1/video-thumbnails` |
| `merge_tags` | `POST /api/v1/tags/merge` |
| `rename_tag` | `POST /api/v1/tags/rename` |
| `update_tag_synonyms` | `POST /api/v1/tags/synonyms` |
| `batch_tags` | `POST /api/v1/tags/batch` |
| `list_rejected_tag_names` | `GET /api/v1/tags/rejected-names`。`limit` の既定は 100 |
| `forget_rejected_tag_name` | `DELETE /api/v1/tags/rejected-names` |
| `update_video_work_info` | `POST /api/v1/video-work-info` |
| `list_people`、`list_organizations` | `GET /api/v1/people`、`GET /api/v1/organizations`。ツールでは `limit` の既定は 100 |
| `create_person`、`create_organization` | `POST /api/v1/people`、`POST /api/v1/organizations` |
| `update_person`、`update_organization` | `POST /api/v1/people/update`、`POST /api/v1/organizations/update` |
| `merge_people`、`merge_organizations` | `POST /api/v1/people/merge`、`POST /api/v1/organizations/merge` |
| `delete_people`、`delete_organizations` | `POST /api/v1/people/delete`、`POST /api/v1/organizations/delete` |

- ツールの引数は操作のクエリと本文の形を持ち、構造化された結果は応答本文の形を持つ。操作のエラーは、`isError: true` と [エラー本文](#call-the-api) を持つツール結果になる。
- 受け付けるのは `POST /mcp` だけで、応答は `application/json` である。サーバーはセッションを保持しないため、`GET` と `DELETE` は `405` を返す。
- トークンがないか無効なときは、MCP の処理の前に `WWW-Authenticate: Bearer` 付きの `401` を返す。トークンを失効させると、実行中のツールは打ち切られる。
