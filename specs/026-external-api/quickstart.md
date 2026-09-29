# Quickstart: 外部連携 API と MCP を端から端まで確かめる

親 Issue #493 の受け入れ条件を、動いているサーバーで確かめる手順である。各単位の自動テストは
`task check` が走らせる。この手順は、画面・実際の MCP クライアント・スキャンを通す確認で、
最後の単位（「`/mcp` で外部連携の操作を MCP のツールとして提供する」）の実装 PR の本文に結果を残す。

## 前提

- [docs/how-to/development.md](../../docs/how-to/development.md) のとおり `task dev` で起動し、所有者で
  ログインし、メディアフォルダを 1 つ登録してスキャンを終えている。以下 `BASE=http://localhost:8080`。
- 非公開の動画が 1 本以上ある（既定は非公開）。
- MCP の確認に Claude Code（`claude` コマンド）。

## 手順

1. 設定ページの「API トークン」節で名前を入れて発行する。平文が一度だけ表示されコピーできる。
   再読み込みすると、一覧に名前と日時だけが残る（受け入れ条件 1）。以下その平文を `TOKEN` とする。
2. `curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos"` が非公開の動画を含めて返る。
   設定ページの一覧で最終使用日時が入る（受け入れ条件 2）。
3. `nextCursor` が空になるまで `cursor` をたどり、全件が返る。メディアフォルダに動画を 1 本足して
   スキャンし、先頭から読み直すと、その 1 本が含まれる（受け入れ条件 3）。
4. `GET /api/v1/videos/lookup?path=<その動画の絶対パス>` で引く。`POST /api/v1/video-tags` に
   `{"videos":[{"path":"…"}],"action":"add","tags":["新しい名前","<既存のタグのシノニム>"]}` を送る。
   新しいタグが作られ、シノニムは元のタグとして付く。画面の動画詳細とタグの絞り込みに出る
   （受け入れ条件 4）。同じ要求をもう一度送っても `200` で、タグは変わらない（受け入れ条件 5）。
5. 画面で失効したトークンで呼ぶと `401`。新しく発行したトークンのあと
   `mdm account set-password` を実行し、そのトークンで呼ぶと `401` で、一覧からも消えている
   （受け入れ条件 6）。
6. ブラウザのセッション Cookie だけを付けて `GET /api/v1/videos` を呼ぶと `401`。Bearer だけで
   `GET /api/videos` を呼ぶと、応答の `X-VV-Audience` が `guest` で、非公開の動画は出ない。
   Bearer だけで `GET /api/api-tokens` と `POST /api/api-tokens` を呼ぶと `401`（受け入れ条件 7・8）。
7. `claude mcp add --transport http vv "$BASE/mcp" --header "Authorization: Bearer $TOKEN"` で追加し、
   `/mcp` の一覧に [contracts/mcp.md](contracts/mcp.md) の 6 つのツールが並ぶ。`update_video_tags` で
   タグを付けると画面に出る。`curl -X POST "$BASE/mcp"`（トークン無し）と無効なトークンでは `401`
   （受け入れ条件 9）。

## 期待する結果

上のそれぞれが成り立つ。
