# ライブ変換のシークと解析情報の再利用

- ステータス: 採用
- スコープ: `GET /api/videos/{id}/transcode.mp4` の開始（解析情報の用意と FFmpeg の起動）
- 経緯: 親 Issue #371、[Plan](../../specs/018-live-transcode-seek/plan.md)・
  [data-model.md](../../specs/018-live-transcode-seek/data-model.md)

## 解析情報の再利用

### Context

ライブ変換の ffmpeg の引数（映像・音声の stream の選び方、コピーできるか、寸法・回転・fps の
扱い、MOV の二入力）は ffprobe の事実で決まる。以前は要求ごとに ffprobe を起動しており、
シークのたびに同じファイルを解析し直していた。ネットワークドライブ上の動画では、この解析が
最初のデータまでの時間の大きな部分を占める。

### Decision

取り込みの解析（ffprobe）が、ライブ変換に要る値 `domain.TranscodeProbe` を表
`video_transcode_probes` に保存する。行は値の JSON とその版、解析したファイルの大きさと
更新時刻（ナノ秒）を持つ。ライブ変換は次の順で解析情報を用意する。

1. 経路（`internal/httpapi/transcode.go`）が `LibraryStore.TranscodeProbe` で保存値を読み、
   `domain.TranscodeProbeUsable` で使ってよいかを決める。版が今の
   `domain.TranscodeProbeVersion` と同じで、JSON が読め、大きさと更新時刻が変換で実際に開いた
   ファイルの `Stat` と一致するときだけ使う。同じ内容の別の所在があっても、比べるのは開いた所在
   である。
2. 経路は使える解析情報か nil を `domain.LiveTranscodeRequest` に載せて
   `LiveTranscoder.Start`（`internal/media`）に渡す。media は解析情報があれば ffprobe を起動
   せず、無ければその場で ffprobe を実行する。
3. media は FFmpeg の最初のデータが出たところで `Start` から返る。保存値で始めた FFmpeg が
   データを出さずに終わったら、同じ要求の中でその場の ffprobe を実行し、1 回だけやり直す。
   その場の解析で始めた FFmpeg の失敗、要求の取り消し、`transcodeStartupTimeout`（6 秒）の
   期限切れではやり直さない。期限は解析とやり直しを含めて 1 つである。経路は `Start` が
   戻ったあとに同じ期限をかけ直さず、要求の取り消しだけで打ち切る。
4. media がその場で解析したときは、結果を `LiveTranscode.Probed` に載せて返し、経路が
   `IngestStore.SaveTranscodeProbe` で開いたファイルの印とともに保存する。保存は最初のデータを
   読んだあとに配信と並べて行い、書き込みの待ちで開始の期限を使わない。保存には配信と独立した
   期限（`transcodeProbeSaveTimeout`、10 秒）があり、要求が取り消されても期限の中で済ませる。
   次の変換はこれを使う。解析のあとにファイルの印が開いたときと違っていたら、media は結果を変換には使うが
   保存用には返さない。途中で打ち切られた ffprobe は誤りで終わるので保存されない。

保存は upsert 1 文で、取り込みと変換が同時に書いても後に書いた行が残る。表は索引で、消えても
次の変換か再解析で埋まる。起動時に既存の動画をまとめて埋める処理は無く、取り込み済みで行の無い
動画は最初の変換で埋まる。

### Trade-offs

- 保存値が合うかの判定は大きさと更新時刻だけで、内容は比べない。大きさと更新時刻を保ったまま
  書き換えられたファイルでは古い解析情報で変換する。その変換が最初のデータの前に失敗すれば、
  やり直しの解析で保存が置き換わる。
- media が最初のデータを待つようになり、経路はその後の出力を読むだけになった。切り替えの判断を
  プロセスの起動と出力の読み取りの側に置き、httpapi に ffmpeg の引数の知識を持ち込まない。
- media は store を呼ばない。保存値を使ってよいかの判定と保存は経路が行い、ffmpeg を持たない
  単体テストに SQLite が要らない。

### Alternatives

- **media が store を読む**: adapter が adapter に依存し、依存の向き（ARCHITECTURE.md）に反する。
- **ffprobe の生の JSON を保存する**: 要求時の解釈が保存時の ffprobe の版に依存し、数十 KB の
  文字列を動画ごとに持つ。parser を通した値を版つきで保存する。
- **`videos` に型付きの列を足す**: 一覧の読み出しが使わない列で行が太り、項目が増えるたびに
  マイグレーションが要る。

### Validation

- `internal/media/transcode_test.go`: `commandContext` を差し替え、保存値があれば ffprobe を
  起動しないこと、無ければ 1 回だけ起動して結果を返すこと、保存値の失敗で 1 回だけやり直すこと、
  期限切れと取り消しでやり直さず結果も返さないことを確かめる。
- `internal/httpapi/transcode_test.go`: 本物の保存層で、取り込み済みの動画は 1 回目もシーク後も
  解析しないこと、行の無い動画は 1 回目だけ解析して保存すること、大きさか更新時刻が違えば解析して
  保存を置き換えることを確かめる。
