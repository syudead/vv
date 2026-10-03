# タグ管理画面を規模のデータで測る

タグ管理画面（`/tags`）を、タグ 1,000 個・動画 10,000 本と、タグ 3,000 個・動画 30,000 本の
ライブラリで本番ビルドに対してヘッドレス Chromium から測り、PR に残す手順。何を測り何を期待するかは
[specs/036-tag-admin-scale/quickstart.md](../../specs/036-tag-admin-scale/quickstart.md)、
この形にした理由は
[research.md R-9](../../specs/036-tag-admin-scale/research.md#r-9-受け入れ条件の計測は作り置きの規模のデータを-scriptstagsbench-が作りplaywright-の計測スクリプトが本番ビルドに対して測る)
にある。

`task check`・`task test-e2e`・CI には入らない。画面の描き方を変える PR で、手で走らせる。

## 前提

- `task build` が通る環境（Go・Node・`ffmpeg`・`ffprobe`。[ローカル開発](development.md)）。
  動画のファイルは作らないので、`ffmpeg` は vv の起動前確認にだけ使う。
- `web/` の Playwright の Chromium が入っている（`task test-e2e` と同じ）。入れられない環境では、
  手元の Chromium を `-chromium` で渡す。

## 測る

```sh
go run ./scripts/tagsbench -scale 1000
go run ./scripts/tagsbench -scale 3000
```

`scripts/tagsbench` は次を順に行う。

1. 規模のデータが無ければ `.local/tagsbench/<規模>/seed/` に作る。`-scale` はタグの数で、動画は
   その 10 倍。各タグには動画が付き、11 個に 1 個（約 9%）は本数 0、半数は仮のタグにする。
   行は `internal/store` の公開の操作で書き、動画のファイルは作らない（登録フォルダ
   `.local/tagsbench/<規模>/media/` の下の所在の行だけ）。初回は 1,000 で約 1 分、3,000 で数分かかる。
2. `go run ./scripts/build` で単一バイナリ `bin/mdm` を作る（`-skip-build` で省く）。
3. `seed/` を `run/` へ写し、そのデータで `bin/mdm` を空いているループバックのポートで起動する。
   計測は確定や改名で行を書き換えるので、毎回 `seed/` の写しから始める。
4. `web/bench/tags-admin.bench.ts`（設定は `web/bench/playwright.config.ts`）で場面を測り、
   表を出して `.local/tagsbench/<規模>/result.md` に書く。終わると vv を止める。

| 引数 | 意味 |
| --- | --- |
| `-scale N` | 規模（タグの数）。必須 |
| `-skip-build` | `bin/mdm` をビルドし直さずに使う |
| `-chromium PATH` | Playwright が持つものの代わりに使う Chromium の実行ファイル |

規模のデータを作り直すときは `.local/tagsbench/<規模>/` を消す。作り方（`scripts/tagsbench` の
`planTags`）を変えたときも消す。

## 表の読み方

場面は quickstart.md の 7 つで、値は場面ごとに次のとおり。

- 開いてから最初の行が出るまで: `/tags` へ移動してから、一覧の最初の行が DOM に現れるまで
  （ナビゲーションの開始から）。3 回読み込み直して各回と中央値を出す。補足の
  `GET /api/tags` の応答（Playwright の `response.timing`）が長ければサーバーの側、短ければ描画の側が
  時間を使っている（quickstart.md「内訳の切り分け」）。
- 検索の 1 文字目・Esc での取り消し・1 件の確定・1 件の改名・まとめての確定: 操作してから結果が
  見えるまでの最長のタスク（Long Task）。Long Task は 50 ms を超えるタスクだけが記録されるので、
  無ければ「なし（50 ms 以下）」。補足の「〜まで」は Playwright の往復を含む参考の値。
- スクロール: 一覧の先頭から末尾までマウスホイールで送る間の `requestAnimationFrame` の間隔。
  50 ms を超えるフレームが続いた回数と、最長のフレーム。
- まとめての確定: 「Tentative only」で見えている仮のタグをすべて選び（「Select all shown tags」）、
  選択バーの「Confirm」から「No tentative tags」が出るまで。画面にまとめて選ぶ操作が無いコミットでは
  「測れない」と出る。

値はマシンとブラウザで変わる。期待の欄と比べるより、同じ環境で測った変更前の値と並べて読む。

## 変更前と変更後を比べる

同じ環境で続けて測る。`scripts/tagsbench` や `web/bench/` が無いコミット（変更前）を測るときは、
変更後のものを worktree へ写して走らせる。作った規模のデータも写すと、作り直しを省ける。

```sh
# 変更後（PR のブランチ）
go run ./scripts/tagsbench -scale 1000
go run ./scripts/tagsbench -scale 3000

# 変更前（PR の base のコミット）
git worktree add ../vv-before <base のコミット>
cp -r scripts/tagsbench ../vv-before/scripts/   # base に無いときだけ
cp -r web/bench ../vv-before/web/               # base に無いときだけ
ln -s "$PWD/web/node_modules" ../vv-before/web/node_modules
mkdir -p ../vv-before/.local && cp -r .local/tagsbench ../vv-before/.local/
(cd ../vv-before && go run ./scripts/tagsbench -scale 1000)
(cd ../vv-before && go run ./scripts/tagsbench -scale 3000)
git worktree remove --force ../vv-before
```

## PR に残す形

環境（OS、CPU、ブラウザの版）と、規模ごとに場面の値を表にする。

```markdown
環境: Linux x86_64, <CPU>, Chromium <版>

| 場面 | 1,000 変更前 | 1,000 変更後 | 3,000 変更前 | 3,000 変更後 | 期待 |
| --- | --- | --- | --- | --- | --- |
| 開いてから最初の行が出るまで（中央値） | 0 ms | 0 ms | 0 ms | 0 ms | 1 秒以内 |
| GET /api/tags の応答 | 0 ms | 0 ms | 0 ms | 0 ms | — |
| 検索の 1 文字目（最長のタスク） | 0 ms | 0 ms | 0 ms | 0 ms | 0.2 秒を超えない |
| Esc での取り消し（最長のタスク） | 0 ms | 0 ms | 0 ms | 0 ms | 0.2 秒を超えない |
| 1 件の確定（最長のタスク） | 0 ms | 0 ms | 0 ms | 0 ms | 0.2 秒を超えない |
| 1 件の改名（最長のタスク） | 0 ms | 0 ms | 0 ms | 0 ms | 0.2 秒を超えない |
| スクロール（50 ms 超が続いた回数 / 最長のフレーム） | 0 / 0 ms | 0 / 0 ms | 0 / 0 ms | 0 / 0 ms | 2 つ続かない |
| まとめての確定（最長のタスク） | 測れない | 0 ms | 測れない | 0 ms | 0.2 秒を超えない |
```
