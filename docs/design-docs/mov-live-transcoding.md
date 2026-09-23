# MOVライブ変換のtrack分離入力

- ステータス: 採用
- スコープ: request-scoped fragmented MP4へのライブ変換

## Context

動画ファイルはローカルディスクだけでなくネットワークドライブにも置かれる。
MOV demuxerが映像と音声のpacketを時刻順に返すためにtrack間を細かくseekすると、
ネットワークドライブでは初期出力と再生中の変換が著しく遅くなることがある。

FFmpegのMOV demuxerには`interleaved_read`があるが、これを無効にするとfile位置順の
読み出しが一方のtrackだけを長時間先行させ得る。映像と音声が離れて格納されたMOVでは、
不足するtrackをmuxerが待って初期出力が期限を超えるか、再生が途中で停止する。そのため、
全MOVでinterleaveを無効にする方法は採用しない。

## Decision

`ffprobe`が返すformat名に`mov`を含み、選択対象の音声streamがある場合だけ、FFmpegに
同じpathを二つのinputとして渡す。

- input 0は`-an`で音声を無効にし、選択した映像streamだけをmapする。
- input 1は`-vn`で映像を無効にし、選択した音声streamだけをmapする。
- seek再開時は同じ`-ss`を両inputへ指定し、映像と音声の論理開始位置を揃える。
- `interleaved_read`は指定せず、MOV demuxerの既定動作を維持する。

各demuxerが一方のtrackだけを追うため、映像と音声の間を往復するseekを避けられる。
非MOV、または音声のないMOVは単一inputのままとする。複数の映像・音声がある場合も、
request時probeが選んだ最初の非添付映像と最初の音声だけを出力する既存規則は変えない。
実装は`internal/media/transcode.go`の`transcodeArgs`に閉じる。

## Trade-offs

- FFmpegは音声付きMOVごとに同じファイルを二つのdemuxerで開く。単一inputよりfile handle、
  demux処理、container metadataの読込みが増える。
- track配置とOS cacheによっては、二つのinputが同じ領域を読み、総読込量が増える。
  この方式が保証するのはtrack間の往復seek削減であり、すべてのMOVでI/O量が減ることではない。
- 負荷は同時ライブ変換request数に比例する。想定運用は単一ユーザー、同時視聴1から2 sessionであり、
  現時点では共有cacheや変換workerを追加しない。
- 二つのinputは一つのFFmpeg process内にあり、request contextのcancelでまとめて停止する。
  出力はrequest-scopedで、local fileやdatabaseへ永続化しない。

この負荷増加は、ネットワークドライブ上のMOVで実用的な初期出力を得ながら、track配置に依存する
再生停止を避けるための代償として受け入れる。同時変換数を増やす場合は、file handle上限、
ネットワーク帯域、server CPUを再計測してこの判断を見直す。

## Alternatives

### `interleaved_read=0`

track間のseekは減るが、非interleave読込みにより一方のtrackが長時間先行し得る。また、古い
FFmpegにはoption自体がない。再生の正しさとhost FFmpeg互換性を損なうため採用しない。

### local一時fileへのcopy

既定のinterleaveを保ったままネットワークseekを避けられるが、再生開始前に動画全体のcopyが必要になり、
長尺動画の初期待ち時間とlocal storage管理が増える。request-scopedで永続物を持たない現在の設計にも
合わないため採用しない。

### すべてのformatを二入力にする

問題が確認されたのはMOV demuxerのtrack間seekであり、他formatへfile handleとdemux負荷を広げる
根拠がない。MOVかつ映像・音声の両方がある場合に限定する。

## Validation

- 通常配置の映像・音声MOVを生成し、二入力変換後のMP4に両streamがあることを確認する。
- 実MOVをネットワークドライブから変換し、初期出力、映像・音声の開始時刻、durationを確認する。
- 初期データ取得後にtranscoderをcancelし、二入力のFFmpeg processが短時間で終了することを確認する。
- browser E2EでMOVを含むformat matrixの再生開始を確認する。

自動検証は`internal/media/transcode_test.go`と`web/e2e/playback.e2e.ts`に置く。
