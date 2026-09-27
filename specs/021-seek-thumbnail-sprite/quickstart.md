# Validation Quickstart: シーク用サムネイルのスプライトシート

共通の検査は [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task test-e2e`）で、
ここにはこの feature だけの入力と手順を書く。入力は
[docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) と同じく `testsrc2`
（時刻が画面に描かれる）から作り、`.local/bench/` に置く。私的な動画やファイル名は使わない。

## 1. 改善前後の生成時間・ファイル数・合計サイズ（受け入れ条件 1・2）

`docs/how-to/preview-benchmark.md` の 2 時間の入力 `long-2h.mp4` と 2 分の入力 `short-2m-720p.mp4`
を使い、`go run ./scripts/previewbench -kind seek <video>` を変更前と変更後の両方で走らせる。
変更前は、生成を変える前のコミット（previewbench にシーク用の種類を足した直後）で測る。

PR には環境（OS、CPU、ffmpeg の版）と次の表を載せる。ファイル数と合計は previewbench の出力
ディレクトリ（`GenerateSeekThumbnailSet` が書くもの）を全部数える。変更後はシートだけで、
2 時間は 6、2 分は 1 になる（`sprite.json` は保存の側が書くので含まない）。

```markdown
| 入力 | 変更前 1 回目 | 変更前 2 回目 | 変更前ピークメモリ | 変更前ファイル数 | 変更前合計 | 変更後 1 回目 | 変更後 2 回目 | 変更後ピークメモリ | 変更後ファイル数 | 変更後合計 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2 時間・640×360・H.264 | 00.000 秒 | 00.000 秒 | 0 MiB | 1440 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0 MiB | 6 | 0.0 MiB |
| 2 分・1280×720・H.264/AAC | 00.000 秒 | 00.000 秒 | 0 MiB | 24 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0 MiB | 1 | 0.0 MiB |
```

ピークメモリが取れない OS では「取得不可」と書く。2 分の入力で 5 秒間隔、2 時間の入力で 12 秒間隔・
600 コマになることは `internal/domain` のテストが検査する。

## 2. 再生画面での確認（受け入れ条件 3〜6）

追加の入力を作る。縦長は `preview-benchmark.md` の `portrait-rotated.mp4`、ライブ変換用は同じ
内容を MKV（ブラウザで再生できない容器）にしたもの。

```sh
ffmpeg -nostdin -v error -i .local/bench/long-2h.mp4 -c copy .local/bench/long-2h.mkv
mkdir -p .local/preview/media
cp .local/bench/long-2h.mp4 .local/bench/long-2h.mkv .local/bench/portrait-rotated.mp4 .local/preview/media/
task preview   # 動いていれば Ctrl+C で止めてから起動し直す
```

処理状況のシーク用サムネイルの残りが 0 になってから、各動画の再生画面で確かめる。

| Case | 操作 | Expected evidence |
| --- | --- | --- |
| 先頭・中間・末尾（3） | 2 時間の MP4 でシークバーの先頭、中央、末尾近くをポイント | 表示されるコマに描かれた時刻が、その位置の区間 `[k*12s, (k+1)*12s)` の中にある |
| 追従（4） | ブラウザの開発者ツールのネットワーク記録を開き、シークバー上でポインターを端から端まで連続して動かす | 配置情報 1 回と、通過したシートそれぞれ 1 回の要求だけがあり、同じシートの再要求が無い |
| 縦長（5） | `portrait-rotated.mp4` でポイント | コマが縦長のまま枠に収まり、隣のコマの一部が見えない |
| ライブ変換（6） | `long-2h.mkv` と `long-2h.mp4` で同じ位置（たとえば中央）をポイント | 同じ時刻のコマが出る |
| 短い動画 | `task preview` の組み込みサンプル（20 秒以下） | 1 シート・数コマで、末尾でも黒いコマが出ない |

360px・768px・1280px のそれぞれで、先頭・中央・末尾の表示を撮影して PR に載せる。プレビューの
大きさ・位置・時刻表示が変更前と同じであることも比べる。

## 3. 中断・差し替え・削除と既存の JPEG の回収（受け入れ条件 7・8）

- 生成中（処理状況にシーク用サムネイルの残りがある間）に `task preview` を Ctrl+C で止めて起動し
  直すと、`MDM_DATA_DIR/thumbnails/.tmp/` が空になり、その動画の置き場に `sprite.json` の無い
  ディレクトリが残らない。
- 生成中に `.local/preview/media/` の入力を別の内容の同名ファイルに置き換えて再スキャンすると、
  古い内容のスプライトが配信されず（`GET /api/videos/{id}/seek-thumbnail` が新しい版で 409 のあと
  200 になる）、新しい内容で作り直される。
- 変更前のデータディレクトリ（`seek/<p>/<s>/000000.jpg` … の個別 JPEG）で起動し直すと、処理状況に
  シーク用サムネイルの残りが出る。終わるとその置き場にシートと `sprite.json` だけが残る。残りが
  ある間も、その動画の再生画面で再生・シーク・時刻の表示が使える。
