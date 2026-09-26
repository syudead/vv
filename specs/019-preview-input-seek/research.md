# Research: 動くプレビューの区間の読み方

技術スタックと生成物の所有は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)・
[ARCHITECTURE.md](../../ARCHITECTURE.md)「Generated files have one owner」）。ここにはこの feature が足す
決定だけを書く。計測値は、親 Issue #387 に載っているもの（Windows、および Linux・ffmpeg 6.1.1・4 コア・
16GB）と、この Plan を書くときに開発コンテナ（Linux・ffmpeg 6.1.1・4 コア・16GB）で `testsrc2` から作った
入力（2 分・1280×720・30fps・H.264/AAC、30 分・640×360・30fps・H.264、`-g 250`）で取ったものである。
どちらも合成した映像で、実ライブラリの動画の速度の保証ではない。

## R-1: 区間を読む方式

- Decision: 9 秒を超える動画は、`PreviewSegments` が返す 12 区間をそれぞれ
  `-ss <start> -t 0.75 -i <input>` の入力として 1 回の ffmpeg に渡し、入力ごとに
  `[i:V:0]setpts=PTS-STARTPTS,<scale>,format=yuv420p[vi]` を掛けて `concat=n=12:v=1:a=0` で連結する。
  出力側の引数（`-c:v libx264 -pix_fmt yuv420p -movflags +faststart -an`）は今のまま。
- Rationale: 入力側の `-ss` で ffmpeg は区間の直前のキーフレームまで容器の索引でシークし、そこから
  区間の終わりまでしかデコードしない。保持するフレームも区間 1 つ分なので、時間もメモリも動画の長さに
  依らない。開発コンテナの計測:

  | 入力 | 現行（1 入力・`trim`） | 12 入力・1 プロセス | 12 プロセス・concat demuxer |
  | --- | ---: | ---: | ---: |
  | 2 分・720p | 13.2〜26.9 秒、ピーク 5,010MB | 2.9〜3.2 秒、ピーク 488MB | 3.7〜3.8 秒、ピーク 126MB |
  | 30 分・640×360 | 57 秒で 13.7GB に達し OOM kill | 1.5〜1.6 秒、ピーク 270MB | 2.5 秒、ピーク 106MB |

  親 Issue の 2 時間の入力でも同じ向き（Linux: 現行は OOM、12 入力・1 プロセスは 2.1〜7.1 秒・367MB）。
  1 プロセスの案を採るのは、プロセスの起動が 1 回で済み（親 Issue の Windows の短尺計測では、13 回起動する
  案が現行 0.6 秒に対し 1.5 秒）、`internal/media` が中間ファイルの置き場を持たずに済むからである。
  ピークメモリは 12 プロセス案より大きいが、区間の数と解像度で決まり動画の長さでは増えない（要件 2）。
- Alternatives considered: 12 区間を別プロセスで抽出して concat demuxer で `-c copy` 結合する（上記。
  メモリは最小だが起動 13 回と中間ファイル）。旧方式を短尺だけに残す（要件 1 が 9 秒超の全部を対象に
  しており、2 分・720p でも 5GB を使う）。`-noaccurate_seek` で区間の開始をキーフレームに丸める
  （R-3）。区間ごとに `-ss` を出力側に置く（入力を先頭からデコードするので今と同じ費用）。

## R-2: フレームの無い区間

- Decision: 区間にフレームが無い入力（容器の長さが映像 stream より長く、末尾の区間が映像の終わりより
  後ろにある場合など）は失敗にせず、その区間が無いだけの短い出力を公開する。12 区間の全部が空で
  出力が空ファイルになった場合は、`internal/artifacts.PublishPreview` の既存の検査（空ファイルを
  公開しない）で失敗になり、ジョブの再試行と失敗記録が扱う。
- Rationale: 映像 10 秒・音声 12 秒の入力（容器の長さ 12 秒）で確かめた。ffmpeg 6.1.1 の `concat`
  フィルタは、フレームを出さずに終わった入力を空の区間として飛ばし、10 区間分（7.67 秒）の出力を
  作った。現行方式も同じ入力で 7.47 秒の出力になるので、動作は変わらない。`probe` の長さは容器のもので、
  映像 stream より数百ミリ秒長いことは珍しくないため、これを失敗にすると多くの動画のプレビューが
  作れなくなる。
- Alternatives considered: 最後の区間の開始を映像 stream の長さに合わせて前へずらす（映像 stream の
  長さを `internal/media` に渡す口を増やすことになり、区間の選び方が入力によって変わる）。空の区間を
  検出して ffmpeg をやり直す（ffmpeg が自分で飛ばすので不要）。

## R-3: シークの精度

- Decision: `-ss` は入力側に置き、ffmpeg の既定（accurate seek: 直前のキーフレームからデコードして
  指定時刻までのフレームを捨てる）のまま使う。`-noaccurate_seek` は付けない。
- Rationale: 今の `trim=start=<s>` は指定時刻からのフレームを出しており、区間と映る場面の対応
  （要件 3・受け入れ条件 3）はそこで決まっている。accurate seek なら同じ対応を保つ。捨てるフレームの
  デコード費用はキーフレーム間隔 1 つ分（x264 の既定 250 フレームで 30fps なら最大 8.3 秒分）で、
  12 区間合わせても全編のデコードより小さい（R-1 の計測に含まれる）。
- Alternatives considered: `-noaccurate_seek`（区間の開始が直前のキーフレームへずれ、区間の長さも
  `-t` の解釈が変わって伸びる。時間はわずかに縮むが要件 3 に反する）。

## R-4: 計測の方法

- Decision: `scripts/previewbench` を Go で書く。引数の動画ごとに `media.GeneratePreview` を一時
  ディレクトリの出力へ `-runs`（既定 2）回走らせ、回ごとの壁時計時間と ffmpeg のピークメモリを表示
  する。ピークメモリは Linux/macOS で `syscall.Getrusage(RUSAGE_CHILDREN)` の `Maxrss` から取り、
  入力ごとに全回の最大値として表示する（この値は終了した子プロセス全体の最大なので、複数の入力を
  1 回の実行に渡すと前の入力の値を引き継ぐ。手順書では入力ごとに実行を分ける）。Windows では取らずに
  壁時計時間だけを表示する。手順は
  `docs/how-to/preview-benchmark.md` に置き、比較用の入力を作る ffmpeg のコマンド（2 時間・640×360・
  30fps・H.264 と 2 分・1280×720・H.264/AAC を `testsrc2` から作る）、変更前のコミットと変更後で同じ
  入力を測る手順、結果を表にして PR に残す形を書く。
- Rationale: 要件 6 は「同じ入力と環境で改善前後を測れる手順」を求めており、測るのは本番の生成コード
  そのもの（`GeneratePreview`）でなければ意味が無い。Go で書けば Windows でも同じ手順で壁時計時間を
  測れる。親 Issue の OOM は Linux で起きているので、ピークメモリは Linux で測れれば受け入れ条件 1 を
  示せる。リポジトリの規則（Taskfile.yml: 込み入った処理は scripts/ の Go プログラムに置く）にも合う。
- Alternatives considered: how-to に shell と `/usr/bin/time -v` の手順だけを書く（Windows で使えず、
  ffmpeg の引数を手順書に写すことになり本番コードとずれる）。`go test -bench`（子プロセスのピーク
  メモリを報告できない）。`GeneratePreview` に計測の口を足す（本番コードに計測のためだけの引数が残る）。
