# 動くプレビューの生成を測る

動くプレビュー（一覧カードの hover 再生）の生成方式を変える PR で、変更前と変更後の
壁時計時間とピークメモリを同じ入力・同じ環境で測り、PR に残す手順。測るのは本番の
生成コード（`internal/media` の `GeneratePreview`）そのもので、`scripts/previewbench`
がそれを呼ぶ。理由は [specs/019-preview-input-seek/research.md R-4](../../specs/019-preview-input-seek/research.md#r-4-計測の方法)。

## 入力を作る

`ffmpeg` の `testsrc2` から作る。置き場所はリポジトリの外でも `.local/` の下でもよい
（`.local/` は git に入らない）。ここでは `.local/bench/` に置く。

```sh
mkdir -p .local/bench

# 2 時間・640×360・30fps・H.264（長尺。数分かかる）
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=7200 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p .local/bench/long-2h.mp4

# 2 分・1280×720・H.264/AAC（短尺）
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=1280x720:rate=30:duration=120 \
  -f lavfi -i sine=frequency=440:duration=120 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -shortest .local/bench/short-2m-720p.mp4

# hover 確認用: 回転情報（90 度）を持つ縦長の入力（1 分）
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=60 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p .local/bench/landscape.mp4
ffmpeg -nostdin -v error -display_rotation 90 -i .local/bench/landscape.mp4 \
  -c copy .local/bench/portrait-rotated.mp4
```

`ffprobe .local/bench/portrait-rotated.mp4` の出力に `rotation of 90.00 degrees`
（displaymatrix）が出ていれば、回転情報が入っている。

## 測る

```sh
go run ./scripts/previewbench [-runs N] <video>
```

入力ごとに生成を `-runs` 回（既定 2）走らせ、回ごとの壁時計時間と、Linux と macOS
では ffmpeg のピークメモリ（全回の最大）を表示する。Windows では壁時計時間だけを表示する。
存在しない入力や ffmpeg の失敗では、理由を出して 0 以外の終了コードで終わる。

ピークメモリは終了した子プロセス全体の最大なので、**入力ごとに実行を分ける**。
1 回の実行に複数の入力を渡すと、後の入力に前の入力の値が残る。

変更前と変更後を同じ環境で続けて測る。`scripts/previewbench` が無いコミット
（変更前）を測るときは、変更後のプログラムを worktree へ写して走らせる。

```sh
# 変更後（PR のブランチ）
go run ./scripts/previewbench .local/bench/long-2h.mp4
go run ./scripts/previewbench .local/bench/short-2m-720p.mp4

# 変更前（PR の base のコミット）
bench="$PWD/.local/bench"
git worktree add ../vv-before <base のコミット>
cp -r scripts/previewbench ../vv-before/scripts/   # base に無いときだけ
(cd ../vv-before && go run ./scripts/previewbench "$bench/long-2h.mp4")
(cd ../vv-before && go run ./scripts/previewbench "$bench/short-2m-720p.mp4")
git worktree remove --force ../vv-before
```

時間はディスクのキャッシュで変わるので、`-runs` の 1 回目と 2 回目を分けて載せる。

## PR に残す形

環境（OS、CPU、ffmpeg の版）と、入力ごとの値を表にする。値は各回の壁時計時間と
ピークメモリ。

```markdown
環境: Linux x86_64, <CPU>, ffmpeg <版>

| 入力 | 変更前 1 回目 | 変更前 2 回目 | 変更前ピーク | 変更後 1 回目 | 変更後 2 回目 | 変更後ピーク |
| --- | --- | --- | --- | --- | --- | --- |
| 2 時間・640×360・H.264 | 00.000 秒 | 00.000 秒 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0.0 MiB |
| 2 分・1280×720・H.264/AAC | 00.000 秒 | 00.000 秒 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0.0 MiB |
```

変更前が失敗した（メモリ不足で落ちたなど）ときは、その旨と表示された理由を表の欄に書く。
Windows では「ピーク」の欄を「測らない」とする。

## hover で確かめる

`task preview` の組み込みサンプルは 20 秒以下なので、長尺の確認には使わない。作った入力を
`.local/preview/media/` に置いて `task preview` を起動し直す。`task preview` は起動のたびに
そのフォルダを取り込む（[Codespaces で PR を確かめる](codespaces-preview.md)）。

```sh
mkdir -p .local/preview/media
cp .local/bench/long-2h.mp4 .local/bench/portrait-rotated.mp4 .local/preview/media/
task preview   # 動いていれば Ctrl+C で止めてから起動し直す
```

プレビューの生成が終わってから、一覧のカードに hover して次を確かめる。

- 2 時間の入力: 冒頭から末尾まで分散した場面（`testsrc2` の時刻表示で分かる）が、
  時系列順に約 9 秒で、無音でループして流れる。
- 縦長の入力: 縦長のまま流れ、縦横比が崩れない。
