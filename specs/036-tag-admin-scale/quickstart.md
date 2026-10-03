# Quickstart: 規模のデータで受け入れ条件を確かめる

親 Issue #651 の受け入れ条件 1〜3 は、本番ビルドをタグ 1,000 個・動画 10,000 本と、タグ 3,000 個・
動画 30,000 本のライブラリでヘッドレス Chromium から測る数値である。`task check` と Vitest はこれを
確かめられないので、ここにその手順だけを書く。機能の正しさ（受け入れ条件 4〜12）は store・httpapi・
Vitest の試験で確かめ、ここには書かない。

計測の道具は実装の単位「規模のデータを作って測る道具を足す」が足す
（[research.md R-9](research.md#r-9-受け入れ条件の計測は作り置きの規模のデータを-scriptstagsbench-が作りplaywright-の計測スクリプトが本番ビルドに対して測る)）。
使い方の正本は、その単位が書く `docs/how-to/tags-admin-benchmark.md` で、ここは何を測り何を期待するかの
一覧である。

## 前提

- `task build` が通る環境（Go・Node・`ffmpeg`・`ffprobe`。[docs/how-to/development.md](../../docs/how-to/development.md)）。
  動画のファイルは作らないので、`ffmpeg` は起動前確認にだけ使う。
- `web/` の Playwright のブラウザが入っている（`task test-e2e` と同じ）。

## 手順

```sh
# 規模のデータを作り（初回だけ。.local/tagsbench/ に残る）、本番ビルドをそのデータで起動して測る
go run ./scripts/tagsbench -scale 1000
go run ./scripts/tagsbench -scale 3000
```

`-scale` は規模の名前（タグの数）で、動画は 10 倍。各タグには動画が付き、約 9% のタグは 0 本、半数は
仮のタグにする（親 Issue の「1,000 個のうち約 90 個」に合わせる）。計測は `web/bench/tags-admin.bench.ts`
が行い、場面ごとの値を表で出す。変更前（`main`）と変更後を同じ環境で続けて測り、PR の本文に残す
（[docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) の「PR に残す形」と同じ）。

## 場面と期待

| 場面 | 測り方 | 期待（1,000・3,000 のどちらでも） |
| --- | --- | --- |
| 開いてから最初の行が出るまで | `/tags` へ移動してから、一覧の最初の行が DOM に現れるまで | 1 秒以内（受け入れ条件 1） |
| 検索の 1 文字目 | 検索欄に 1 文字入れてから一覧が変わるまでの、最長のタスク（Long Task） | 0.2 秒を超えない（受け入れ条件 2） |
| Esc での取り消し | 検索を Esc で消してから一覧が戻るまでの最長のタスク | 0.2 秒を超えない（受け入れ条件 2） |
| 1 件の確定 | 仮の行の「確定する」を押してから行が差し替わるまでの最長のタスク | 0.2 秒を超えない（受け入れ条件 2） |
| 1 件の改名 | 改名を送ってから行が差し替わるまでの最長のタスク | 0.2 秒を超えない（受け入れ条件 2） |
| スクロール | 一覧の先頭から末尾までマウスホイールで送る間のフレーム時間 | 50ms を超えるフレームが 2 つ続かない（受け入れ条件 3） |
| まとめての確定 | 「Tentative only」で見えているタグをすべて選んで確定し、仮の目印が消えるまでの最長のタスク | 0.2 秒を超えない（受け入れ条件 9） |

Long Task とフレーム時間は `PerformanceObserver`（`longtask`）と `requestAnimationFrame` の間隔で
ページの中から読む。数値はブラウザとマシンで変わるので、変更前の値を同じ表に並べる。

## 内訳の切り分け

最初の行までの時間が長いとき、サーバーの `GET /api/tags` の応答時間（Playwright の `response.timing`）を
別に出し、描画の時間と分ける。応答が長ければ `internal/store/tag_listing.go` の本数の問い合わせが疑わしく、
描画が長ければ `web/src/tags/` の側である。この feature は描画の側を直す。応答の側が 1 秒を占めるときは、
その計測を PR の本文に書いて別の Issue にする（親 Issue の「対象外」が `GET /api/library` に同じ扱いを
している）。
