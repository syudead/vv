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
