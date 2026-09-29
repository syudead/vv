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
- `locations` は登録フォルダの下の今の所在で、パスの順に並び、先頭が代表である（`title` は代表の
  題名）。`durationMs` は解析前は `null`。`tags` の `manual` は手で付けたタグ、`fromFolder` は
  祖先のフォルダ名から付くタグである。

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
- 応答は `{ items: [{ video: { id, contentKey }, tags }] }` で、`videos` の順に各動画の操作後の
  タグを返す。`tags` の形は一覧の項目と同じである。
- 全体を 1 つのトランザクションで行う。引けない動画が 1 つでもあれば `404`（`video_not_found`、
  `index` は `videos` の何番目か）で、何も反映しない（タグも作らない）。
- 同じ要求を繰り返しても状態は変わらず、`200` が返る。通信の失敗のあとは、そのまま送り直してよい。
- 件数の誤りは `400`（`too_many_videos`・`too_many_tags`、`limit` に上限）。名前の誤りは `400`
  （`tag_name_empty`・`tag_name_control_characters`・`tag_name_too_long`）で、`index` は `tags` の
  何番目かを指す。

## スクレイパーからの連携例

新しく取り込んだ動画を外部のサイトで調べ、見つけたタグを付ける流れの例である。

1. スキャンを始め（`POST /api/v1/scans`）、`GET /api/v1/scans/current` の `status` が `done`・`partial`・
   `failed` のどれかになるまで待つ。
2. 一覧（`GET /api/v1/videos`）を先頭から読み直し、手元に記録の無い `contentKey` を新しい動画として
   拾う。`contentKey` はファイルを移しても変わらないので、手元の記録の鍵にする。
3. 新しい動画ごとに、`locations[].path` や `title` から外部のサイトで調べる。後で状態を確かめ直す
   ときは `GET /api/v1/videos/lookup?contentKey=…` で 1 本引く（`404` なら消えている）。
4. 見つけた名前を `POST /api/v1/video-tags` で付ける。同じ名前の組を付ける動画はまとめて 1 回で送る。

```sh
# 2. 一覧から contentKey と代表のパスを拾う（ページのたどり方は「動画の一覧を読む」）。
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos?limit=200" |
  jq -r '.items[] | [.contentKey, .locations[0].path] | @tsv'

# 4. 調べた結果のタグを付ける。
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  "$BASE/api/v1/video-tags" \
  -d '{"videos":[{"contentKey":"…"}],"action":"add","tags":["猫","旅行"]}'
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

- ツールの引数は同じ操作の問い合わせ・本文と、結果（structured content）は応答の本文と同じ形である。
  操作の誤りは、ツールの結果の `isError: true` と、上の誤りの本文（`{ code, message, reason?, limit?,
  index? }`）で返る。
- 受けるのは `POST /mcp` だけで、応答は `application/json`。サーバーは会話の状態を持たない
  （`GET`・`DELETE` は `405`）。
- トークンが無い・無効なときは、MCP の処理に入る前に `401` と `WWW-Authenticate: Bearer` が返る。
  トークンを失効させると、実行中のツールも打ち切られる。
