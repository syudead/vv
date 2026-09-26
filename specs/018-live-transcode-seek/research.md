# Research: ライブ変換のシークと解析情報の再利用

技術スタックとライブ変換の現行の作りは正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)・
[docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md)・
[internal/media/transcode.go](../../internal/media/transcode.go)）。ここにはこの feature が足す決定だけを
書く。R-1・R-6・R-7 の値は、開発コンテナの ffmpeg 6.1.1 で、キーフレーム間隔 250 フレーム・30fps・
40 秒の H.264/AAC の MOV と、それを `-c copy` で MKV にした fixture に対して `-ss 27` で確かめた。

## R-1: 実際の開始位置をどこから知るか

- Decision: コピーで途中から始める ffmpeg に `-copyts -start_at_zero` と
  `-movflags frag_keyframe+empty_moov+default_base_moof+delay_moov` を付ける。mp4 muxer は
  `delay_moov` で最初の fragment を切るまで `moov` を待ち、各 track の edit list（`elst`）の先頭に
  「空の edit」としてその track が始まる時刻（movie timescale、ffmpeg では 1000 = ミリ秒）を書く。
  サーバーはそこから実際の開始位置を読み、最も早い track を 0 にして他の track にはその差を残すよう
  `elst` を書き換えてから送る。`moof` の `tfdt` は最初の fragment で 0 から始まっており、書き換えない。
  `-start_at_zero` を付けるので、時刻は動画の先頭からの相対（今のプレイヤーの時間軸）である。
- Rationale: mp4 muxer は `empty_moov` で `moov` を先に書くと、track の最初の時刻を 0 に寄せて
  情報を捨てる（`-copyts` だけでは `tfdt` が 0 になることを確かめた）。`delay_moov` はその場合のために
  muxer 自身が用意している選択で、開始位置が `moov` に 1 回だけ、ISO BMFF の形で現れる。`moov` は
  最初の fragment と同時に出るので、今の「最初のデータを待ってから応答を始める」流れの中で読める。
  Chrome と Firefox は progressive 再生で最初の時刻を 0 に寄せるため、`-copyts` の時刻をそのまま
  出してもプレイヤーは開始位置を知れず、ブラウザごとに時間軸が変わる。edit list を書き換えれば、
  ブラウザが見る出力は今のコピーの出力と同じ形（時間軸 0 始まり、`elst` は track 間の差だけ）になる。
- Alternatives considered: `-copyts` のまま出す（上記のとおりブラウザ依存）。`moof` ごとの `tfdt` を
  書き換える（送るデータ全部を通す必要があり、`moov` 1 回で済む方を採る）。`-movflags frag_discont`
  （`tfdt` が 0 のままで、確かめた限り開始位置は現れない）。ffmpeg の `-debug_ts` や `-f framecrc` の
  標準エラー出力を読む（行の形式が版に依存し、fragment 1 つ分の全 packet を書き出す）。
  ffmpeg の前にキーフレームを調べる別プロセス（要件 8 が取り除く無駄そのもの）。

## R-2: コピーで許すキーフレームとの差

- Decision: `domain.CopySeekAllowance = 15 秒`。指定位置 − 実際の開始位置がこれ以下ならコピーで
  続け、超えたら同じ要求の中でエンコードに切り替えて指定位置から始める。
- Rationale: x264/x265 の既定のキーフレーム間隔は 250 フレームで、30fps で 8.3 秒、23.976fps で
  10.4 秒になる。親 Issue が例に挙げる「中身が H.264 の MKV」はこの既定で作られたものが多く、
  10 秒では film の frame rate の動画が全部エンコードし直しに落ちる。カメラや配信向けの動画は
  1〜10 秒で、15 秒はそれらを全部コピーで通す。場面の切り替わりにしかキーフレームが無い動画
  （数十秒〜分）は、戻り幅が「自分で選んだ位置」と読めなくなるので、要件 3 のとおりエンコードに落とす。
  切り替えの費用は ffmpeg の起動 1 回（約 0.07 秒）と demuxer の索引の読み直しで、上限を超える
  動画にだけ掛かる。
- Alternatives considered: 10 秒（23.976fps の既定を落とす）。上限なし（要件 3 に反する）。
  超えた動画を覚えておいて次から直接エンコードする（キーフレームの間隔は動画の中で場所によって
  違い、1 回の超過で決められない。プロセス 1 回分の費用を受け入れる）。

## R-3: 切り替えの順序と時間の予算

- Decision: `LiveTranscoder.Start` が同じ要求の中で次の順に試す。(1) 保存済みの解析情報でコピー
  （`videoCanCopy` かつ `normalize` でないとき。それ以外は最初からエンコード）。`startMs > 0` のコピーは
  R-1 の引数で出して実際の開始位置を解決し、`startMs = 0` のコピーは今の先頭からのコピーの引数のまま
  （`-copyts` も `delay_moov` も付けない）で、実際の開始位置は 0、R-2 の判定もしない。
  (2) コピーが最初のデータを出さずに終わった、または実際の開始位置が R-2 の上限を超えたら、同じ
  解析情報でエンコード。(3) 最初のデータが出る前に失敗し、解析情報が保存済みのものだったら、その場で
  ffprobe を実行して結果を返し、(1) からもう 1 度だけやり直す。切り替えの理由は「プロセスがデータを
  出さずに終わる」と「上限の超過」だけで、`transcodeStartupTimeout`（6 秒）の期限切れは今までどおり
  失敗にする。期限は切り替え全体で 1 つである。
- Rationale: 親 Issue の Edge Cases（キーフレームの位置が取れない動画、保存値で失敗する動画）は
  どちらも「最初のデータが出る前なら同じ要求の中でやり直す」と決めている。コピーが遅いのは I/O で、
  同じファイルを読むエンコードはさらに CPU が要るので、コピーを早めに諦める予算に意味が無い。
  ffprobe のやり直しを 1 回に限るのは、壊れたファイルで ffprobe と ffmpeg を繰り返さないためである。
- Alternatives considered: 試行ごとの期限（上記）。切り替えを httpapi で行う（プロセスの起動と
  出力の読み取りは media の責務で、httpapi に ffmpeg の引数の知識を持ち込むことになる）。

## R-4: 実際の開始位置をプレイヤーへ伝える経路

- Decision: プレイヤーが変換の URL に `attempt`（要求ごとの乱数）を付け、同時に
  `GET /api/videos/{id}/transcode-start?attempt=…` を呼ぶ。サーバーは変換の要求が始まったときに
  台帳へ `attempt` を載せ、実際の開始位置が決まったら（応答を書き始める前に）記録する。報告の経路は
  記録が無ければ載るまで、載っていて未決なら決まるまで、`transcodeStartupTimeout` を上限に待って
  `{ "startMs": … }` を返し、上限までに現れなければ 404 を返す。変換の要求が終わった `attempt` は
  60 秒残してから消す（[contracts/transcode-start-api.md](contracts/transcode-start-api.md)）。
- Rationale: `<video src>` の再生は応答ヘッダーも本文の構造も JavaScript に見せない。ブラウザは
  動画の要求と報告の要求をどちらを先に送るとも限らないので、報告の側が現れるまで待つ形にすると
  順序を気にせずに済む。報告が届く前は今までどおり指定位置を表示するので、届かない場合も現行と同じ
  表示に留まる。
- Alternatives considered: `/api/events` の SSE（ゲストの購読と順序の扱いが増える）。先に開始位置を
  返す経路が ffmpeg を起動し、動画の要求が接続する（プロセスの寿命が 2 つの要求にまたがる）。
  MSE で本文を自分で取る（配信の作り直し、対象外）。応答の redirect で URL に開始位置を載せる
  （`<video>` は redirect 後の URL を見せない）。

## R-5: 解析情報の保存の形

- Decision: 新しい表 `video_transcode_probes` に、`domain.TranscodeProbe` の JSON、その版
  （`domain.TranscodeProbeVersion`）、解析したファイルの大きさと更新時刻（`os.Stat` の値、ナノ秒）を
  1 行で持つ（[data-model.md](data-model.md)）。読み出し側は、版が違う行と JSON が読めない行を
  「無い」と扱う。
- Rationale: 保存する項目は ffmpeg の引数に要るものだけで（要件 7 が列挙）、一覧や検索は使わない。
  1 列の JSON なら項目が増えたときに版を上げるだけで済み、古い行は要件 10 の経路（最初の変換で
  その場で解析して保存）で自然に埋まる。大きさと更新時刻を `video_locations` の値（スキャン時、秒）
  ではなく解析時の `os.Stat` で別に持つのは、要求時に開いたファイルの `Stat` と同じ精度で比べるため
  （単位が違うと常に不一致になる）。
- Alternatives considered: `videos` の型付きの列（一覧の行を太らせ、項目の追加ごとにマイグレーション）。
  ffprobe の生の JSON（要求時の解釈が保存時の ffprobe の版に依存し、大きい）。

## R-6: MOV の二入力で音声を実際の開始位置に揃える

- Decision: 二入力は今のまま（両方の入力に同じ `-ss`）にし、追加の揃え方は持たない。
- Rationale: 音声側の入力は `-vn` で映像 stream を捨てているが、MOV demuxer の seek は既定の
  stream（映像）のキーフレームへ行い、他の stream をその時刻へ合わせる。確かめた出力では、
  二入力でも単一入力でも音声は映像のキーフレーム時刻から始まった（`elst` は映像 25.000 秒、
  音声 24.981 秒で、単一入力と同じ）。R-1 の書き換えは track 間の差を残すので、音ずれは起きない。
- Alternatives considered: コピーの seek だけ単一入力にする（二入力の理由であるネットワークドライブの
  track 間 seek を、確かめた限り解決済みの問題のために戻すことになる）。

## R-7: コピーの最初のデータが出るまでの時間

- Decision: 変えない。コピーの fragment は元動画のキーフレームで区切る（要件 6 の「コピーする場合は
  元のキーフレームのまま」）ので、最初の fragment はキーフレーム 1 区間分を読み終えてから出る。
  `delay_moov` で `moov` も同じ時点に出るが、再生はどのみち最初の fragment を待つ。
- Rationale: 先頭からのコピー（`startMs = 0`）が今もこの性質を持ち、親 Issue はこれを無駄に数えて
  いない。fragment を時間で切る（`-frag_duration`）とキーフレームで始まらない fragment になり、
  ブラウザの progressive 再生での扱いを確かめる範囲が広がる。
- Alternatives considered: `-frag_duration 2000000` を足す（上記）。
