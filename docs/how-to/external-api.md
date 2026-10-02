# 外部連携 API を使う

スクレイパーなどの外部ツールから、API トークンで VVMDM を操作する手順である。決定の理由は
[specs/026-external-api/research.md](../../specs/026-external-api/research.md) にある。

## 契約

- 契約の正本は [api/external-v1.yaml](../../api/external-v1.yaml)（OpenAPI 3.1）である。操作・
  項目・誤りの `code` と `reason` はこの文書を読む。サーバーからは配らない。
- 基底の経路は `/api/v1`。画面が使う `/api/*`（[api/openapi.yaml](../../api/openapi.yaml)）とは別の
  約束で、画面の API は予告なく変わる。外部ツールは `/api/v1` だけを使う。

## 互換の方針

- `v1` の中では、項目と操作の追加だけを行う。利用者は、知らない項目と知らない `code`・`reason` を
  読み飛ばすように書く。
- 既存の項目の意味・型・必須を変えるとき、操作を取り除くときは、`v1` を変えずに `v2` を足す。

## トークンを発行する

1. 所有者でログインし、設定ページの「API トークン」節で用途の名前を入れて発行する。
2. 表示された平文（`vvt_` で始まる 47 文字）をその場で控える。平文はこの一度しか表示されない。
   失くしたら失効させて発行し直す。
3. 使わなくなったトークンは同じ節で失効させる。ユーザー名かパスワードを変える
   （`mdm account`）と、すべてのトークンが失効する。

トークンは所有者と同じ権限を持つ（非公開の動画も読める）。設定ファイルやログに残すときは
パスワードと同じように扱う。

## 呼び出す

すべての要求に `Authorization: Bearer <トークン>` を付ける。Cookie は読まない。

```sh
BASE=http://localhost:8080
TOKEN=vvt_...

curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/tags"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=100"
curl -X POST -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans"
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/scans/current"
```

- トークンが無い・形式が違う・無効・失効済みのときは `401`（`code: unauthenticated`）と
  `WWW-Authenticate: Bearer` が返る。原因は区別しない。
- 同一オリジンの検査はかけないので、`Origin` を送るクライアントでも使える。本文を取る操作は
  `Content-Type: application/json` を要する。
- 誤りの本文は `{ code, message, reason?, limit?, index? }`。`message` は英語の説明で、分岐には
  `code` と `reason` を使う。
- 最終使用日時は設定ページの一覧に出る。1 分より細かくは更新しない。
- 実行中の要求も、トークンを失効させた時点で打ち切られる。

## スキャン

- `POST /api/v1/scans` は本文を取らない。新しく始めたら `201`、実行中のものがあれば新しく始めずに
  それを `200` で返す。メディアフォルダが無ければ `409`（`media_folders_not_configured`）。
- `GET /api/v1/scans/current` で直近の状態を読む。`status` が `done`・`partial`・`failed` になれば
  終わっている。`finding` のあいだは本数を数え終えていないので、`videos`・`settledVideos` は `null`
  である。一度も走査していなければ `404`（`no_scan`）。

## 動画の一覧を読む

- `GET /api/v1/videos` は、登録フォルダの下にある動画を追加日時の古い順（`addedAt`、同じなら `id`）で
  返す。非公開の動画も返る。1 ページの件数は `limit`（既定 100、1〜200。範囲外は `400`）。
- 応答は `{ items, nextCursor }`。`nextCursor` が空文字列でなければ、それをそのまま `cursor` に渡して
  続きを読む。空文字列になれば最後まで読んだ。カーソルの中身は解釈しない。解釈できないカーソルは
  `400`（`invalid_cursor`）なので、先頭から読み直す。

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

- 一覧は変更を追跡しない。新しい動画を知るには、スキャンの後に一覧を先頭から読み直す。読み通しの
  途中で動画が増える・消える・移動しても続きの要求は失敗しないが、その間の取りこぼしと重複は
  起こりうる。
- 消えた動画は一覧に出なくなるだけで、消えたことを知らせる項目は無い。手元に持っている動画が
  まだあるかは `lookup` で確かめ、`404`（`video_not_found`）なら消えている。登録フォルダの外の所在だけが
  残った動画も、消えた動画と同じに扱う。
- `locations` は登録フォルダの下の今の所在で、パスの順に並び、先頭が代表である。`title` は有効な
  題名で、表示名（`displayName`）があればそれ、無ければ代表のファイル名由来の題名（`fileTitle`）である。
  `displayName` と `thumbnailPositionMs`（代表サムネイルの位置、ミリ秒）は未設定なら `null`。`durationMs` は解析前は `null`。`tags` の `manual` は手で付けたタグ、`fromFolder` は
  祖先のフォルダ名から付くタグ、`tentative` は仮のタグ（「仮のタグとして付ける」）である。
- `updatedAt` は vv 上で動画の情報（表示名・タグ・公開設定・代表サムネイル）を最後に編集した日時で、
  一度も編集していなければ `addedAt` と同じ。この API の `video-tags`・`display-names` で変えたときも進み、
  何も変わらなかった要求では進まない。`fileCreatedAt` は代表の所在（`locations` の先頭）のファイルの
  作成日時で、ファイルシステムから取れないときはそのファイルの更新日時（mtime）である。

## 動画を 1 本引く

`GET /api/v1/videos/lookup` は `id`・`contentKey`・`path` のちょうど 1 つを取る（0 個・2 個以上は
`400`）。応答は一覧の項目と同じ形である。

```sh
curl -G -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos/lookup" \
  --data-urlencode "path=/media/videos/clip.mp4"
```

- `path` は正規化せず、所在のパスとバイト列で比べる。macOS などで NFD の綴りのファイル名は、
  一覧の `locations[].path` をそのまま渡す。
- 無い動画と、登録フォルダの下に所在の無い動画は `404`（`video_not_found`）。

## 動画にタグを付ける

`POST /api/v1/video-tags` は、複数の動画に名前で指定したタグを 1 回の要求でまとめて付ける・外す・
置き換える。本文は JSON で、`Content-Type: application/json` を付ける。

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }, { "contentKey": "…" }, { "id": 12 }],
  "action": "add",
  "tags": ["猫", "ねこ"] }
```

- `videos` の各要素は `id`・`contentKey`・`path` のちょうど 1 つを持つ（`lookup` と同じ引き方）。
  1〜20000 件。本文は 32 MiB（33554432 バイト）までで、超えると `400`（`invalid_request`）で断る。
  長いパスを大量に送るときは、要求を分ける。
- `action` は次のどれか。どれも手で付けたタグだけを書き換え、祖先のフォルダ名から付くタグ
  （`fromFolder`）は変えない。
  - `add`: 名前のタグを付ける。無い名前はタグを作る。
  - `remove`: 名前のタグを外す。どのタグにも当たらない名前は何もしない。
  - `replace`: 手で付けたタグをちょうど `tags` の集合にする。無い名前はタグを作る。`tags` が空なら
    手で付けたタグをすべて外す。
- `tags` はタグの名前で、シノニムも使える（シノニムは元のタグとして付く）。同じタグに当たる名前は
  1 つにまとめる。100 件まで。`add`・`remove` では 1 件以上。
- 応答は `{ items: [{ video: { id, contentKey }, tags }], skippedTags }` で、`videos` の順に各動画の操作後の
  タグを返す。`tags` の形は一覧の項目と同じである。
- 全体を 1 つのトランザクションで行う。引けない動画が 1 つでもあれば `404`（`video_not_found`、
  `index` は `videos` の何番目か）で、何も反映しない（タグも作らない）。
- 同じ要求を繰り返しても状態は変わらず、`200` が返る。通信の失敗のあとは、そのまま送り直してよい。
- 件数の誤りは `400`（`too_many_videos`・`too_many_tags`、`limit` に上限）。名前の誤りは `400`
  （`tag_name_empty`・`tag_name_control_characters`・`tag_name_too_long`）で、`index` は `tags` の
  何番目かを指す。

### 仮のタグとして付ける

本文に `"tentative": true` を足すと、`add`・`replace` で新しく作るタグを**仮のタグ**にする。仮のタグは
画面のタグ一覧に仮として出て、利用者が管理画面で確定するか却下するまで仮のままである。

```json
{ "videos": [{ "path": "/media/videos/clip.mp4" }],
  "action": "add",
  "tags": ["猫", "高画質"],
  "tentative": true }
```

- `tentative` は真偽値で、省くと `false`。真偽値でなければ `400`（`invalid_request`）。
- 既存のタグの名前かシノニムに当たる名前は、`tentative` を問わずそのタグを付け、仮か確定かの状態は
  変えない。
- どのタグにも当たらない名前は、却下した名前（綴りの完全一致）でなければ仮のタグとして作る。
  却下した名前はタグを作らず付けずに飛ばし、応答の `skippedTags` に整えた名前を `tags` の順で 1 回ずつ
  返す。残りの名前は付き、要求は `200` で成功する。`replace` では飛ばした名前は置き換え後の集合に
  入らない。
- `remove` では `tentative` は何も変えない。
- `tentative` を省く（`false`）と、無い名前は確定したタグとして作る。その名前が却下した名前なら、
  却下した名前の一覧から消える。
- 応答の `skippedTags` は常にあり、飛ばした名前が無ければ空の配列である。`tags` の各要素と
  `GET /api/v1/tags` の各タグは、仮のタグかどうかを `tentative` で返す。
- 仮のタグの確定・却下と、却下した名前の一覧を見る・消すのは画面の操作で、この API には無い。

## 表示名を付ける

`POST /api/v1/video-display-names` は、複数の動画の表示名をまとめて設定・解除する。表示名は画面と
この API の `title` に出て、並び替えと検索にも効く。元のファイルは変えない。

```json
{ "items": [{ "video": { "path": "/media/videos/clip.mp4" }, "displayName": "旅行 2024 夏" },
            { "video": { "id": 12 }, "displayName": null }] }
```

- `items` の各要素の `video` は `id`・`contentKey`・`path` のちょうど 1 つを持つ（`video-tags` と同じ）。
  `displayName` は必須で、`null` か、前後の空白を除いて空の文字列なら解除する（ファイル名由来の題名に
  戻る）。1〜20000 件、本文は 32 MiB まで。
- 表示名は内容（`contentKey`）に結ぶ。ファイルを移しても、同じ内容の別の所在にも同じ名前が出る。
  他の動画と同じ名前も付けられる。
- 応答は `{ items: [{ video: { id, contentKey }, title, fileTitle, displayName }] }` で、`items` の順に
  反映後の値を返す。
- 全体を 1 つのトランザクションで行う。引けない動画は `404`（`video_not_found`）、制御文字を含む名前は
  `400`（`display_name_control_characters`）、200 文字を超える名前は `400`（`display_name_too_long`、
  `limit: 200`）で、どれも `index` が `items` の何番目かを指し、何も反映しない。

## 代表サムネイルの位置を変える

`POST /api/v1/video-thumbnails` は、複数の動画の代表サムネイルを指定の場面で作り直す。

```json
{ "items": [{ "video": { "contentKey": "…" }, "positionMs": 12500 },
            { "video": { "id": 12 }, "positionMs": null }] }
```

- `positionMs` は必須で、動画の先頭からのミリ秒（0 以上、尺未満）。`null` なら解除し、自動の位置で
  作り直す。
- 1 件ごとに画像を作るので、1 回の要求は 1〜20 件に区切る（超えると `400` `too_many_videos`、
  `limit: 20`）。多くの動画を変えるときは 20 件ずつ順に送る。
- 先に全件を確かめ、誤りがあれば何も作らない。引けない動画は `404`（`video_not_found`）、どの所在も
  開けない動画は `404`（`file_unavailable`）、尺以上か負の位置は `400`（`thumbnail_position_out_of_range`、
  `limit` にその動画の尺）、解析が終わっていない動画は `409`（`duration_unknown`）で、どれも `index` を返す。
- 確かめたあと `items` の順に 1 件ずつ作る。途中の 1 件で画像を作れなければ `409`
  （`thumbnail_frame_unavailable`）と `index` で止まる。`index` より前の項目は反映済みで、その項目と
  後は反映されていない。続きは `index` の項目の位置を変えるか外し、`index` から後だけを送り直す
  （反映済みの項目を送り直しても同じ位置で作り直すだけである）。
- 作っている間に取り込みが動画や所在を変えることがあるので、各項目は作る直前に動画を読み直し、
  所在を決め直す。そこで上の検証の誤り（`video_not_found`・`file_unavailable` など）や想定外の失敗
  （`500`）になったときも、同じく `index` で止まり、`index` より前の項目は反映済みである。
- 応答は `{ items: [{ video: { id, contentKey }, thumbnailPositionMs }] }`。応答は画像を作り終えてから
  返るので、1 件につき数秒かかることがある。

## スクレイパーからの連携例

新しく取り込んだ動画を外部のサイトで調べ、見つけたタグを付ける流れの例である。

1. スキャンを始め（`POST /api/v1/scans`）、`GET /api/v1/scans/current` の `status` が `done`・`partial`・
   `failed` のどれかになるまで待つ。
2. 一覧（`GET /api/v1/videos`）を先頭から読み直し、手元に記録の無い `contentKey` を新しい動画として
   拾う。`contentKey` はファイルを移しても変わらないので、手元の記録の鍵にする。
3. 新しい動画ごとに、`locations[].path` や `title` から外部のサイトで調べる。後で状態を確かめ直す
   ときは `GET /api/v1/videos/lookup?contentKey=…` で 1 本引く（`404` なら消えている）。
4. 見つけた名前を `POST /api/v1/video-tags` で付ける。同じ名前の組を付ける動画はまとめて 1 回で送る。
   LLM などが自動で出した名前は `"tentative": true` で付け、新しく作られたタグを利用者が管理画面で
   確定・却下して片付ける。一度却下した名前は次からは作られず、`skippedTags` に返る。
5. 外部のサイトの正式な題名が分かれば、`POST /api/v1/video-display-names` で表示名にする。ファイル名は
   変えずに、画面での題名だけを整えられる。

```sh
# 2. 一覧から contentKey と代表のパスを拾う（ページのたどり方は「動画の一覧を読む」）。
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=200" |
  jq -r '.items[] | [.contentKey, .locations[0].path] | @tsv'

# 4. 調べた結果のタグを付ける。自動で出した名前は仮のタグとして作る。
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-tags" \
  -d '{"videos":[{"contentKey":"…"}],"action":"add","tags":["猫","旅行"],"tentative":true}'

# 5. 調べた題名を表示名にする。
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-display-names" \
  -d '{"items":[{"video":{"contentKey":"…"},"displayName":"旅行 2024 夏"}]}'
```

- 付けたタグを外部の結果にそろえ直したいときは `replace` を使う。画面で手で付けたタグも置き換わる
  ので、画面と併用するなら `add` と `remove` で差分だけを送る。
- 要求が `404` で失敗したときは、`index` の動画を記録から外すか `lookup` で引き直してから送り直す。

## MCP から使う

同じ操作を `/mcp` の MCP サーバー（Streamable HTTP）がツールとして出す。Claude Code などの MCP
クライアントから、外部連携 API と同じトークンで使う。

```sh
claude mcp add --transport http vv https://vv.example/mcp --header "Authorization: Bearer vvt_…"
```

| ツール | 対応する操作 |
| --- | --- |
| `list_videos` | `GET /api/v1/videos` |
| `get_video` | `GET /api/v1/videos/lookup` |
| `list_tags` | `GET /api/v1/tags` |
| `update_video_tags` | `POST /api/v1/video-tags` |
| `start_scan` | `POST /api/v1/scans` |
| `get_current_scan` | `GET /api/v1/scans/current` |
| `update_video_display_names` | `POST /api/v1/video-display-names` |
| `update_video_thumbnails` | `POST /api/v1/video-thumbnails` |

- ツールの引数は同じ操作の問い合わせ・本文と、結果（structured content）は応答の本文と同じ形である。
  操作の誤りは、ツールの結果の `isError: true` と、上の誤りの本文（`{ code, message, reason?, limit?,
  index? }`）で返る。
- 受けるのは `POST /mcp` だけで、応答は `application/json`。サーバーは会話の状態を持たない
  （`GET`・`DELETE` は `405`）。
- トークンが無い・無効なときは、MCP の処理に入る前に `401` と `WWW-Authenticate: Bearer` が返る。
  トークンを失効させると、実行中のツールも打ち切られる。
