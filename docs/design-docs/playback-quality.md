# 再生の画質

- ステータス: 採用
- スコープ: ライブ変換（`GET /api/videos/{id}/transcode.mp4`）の画質ごとの縮小とビットレートの上限
- 経緯: [specs/027-playback-quality/](../../specs/027-playback-quality/plan.md)（親 Issue #521）

ライブ変換の開始（解析情報の用意、コピーとエンコードの切り替え）は
[live-transcode-seek.md](live-transcode-seek.md) に、映像の符号化器の選択と方式ごとの引数は
[hardware-encoding.md](hardware-encoding.md) に書いてある。この文書は、画質を指定したライブ変換が
何を変えるかを扱う。

## 変換

### Context

回線の遅い環境では、元の画質のままのライブ変換（映像をコピーできればコピーし、エンコードしても
一定品質）は回線の速さを超え、再生が途切れる。見る人が画質を選んだときに、映像の寸法と
ビットレート、音声のビットレートを合わせて軽くする必要がある。

### Decision

画質は `domain.TranscodeQuality`（`1080p`・`720p`・`480p`・`360p`）で、`internal/domain/transcode_quality.go`
の純粋関数が値を持つ。知らない文字列は `ParseTranscodeQuality` で解釈できない。

| 画質 | 表示の短辺 | 映像の上限（`-maxrate`） | `-bufsize` | 音声（AAC） |
| --- | --- | --- | --- | --- |
| `1080p` | 1080 | 5000 kbps | 10000 kbps | 128 kbps |
| `720p` | 720 | 2500 kbps | 5000 kbps | 128 kbps |
| `480p` | 480 | 1200 kbps | 2400 kbps | 96 kbps |
| `360p` | 360 | 700 kbps | 1400 kbps | 64 kbps |

画質が動画に使えるのは、その短辺が動画の表示の短辺（回転を反映済み）より小さいときだけで、
寸法の無い動画にはどれも使えない（`TranscodeQuality.Available`）。拡大や同じ寸法への変換は重く
するだけだからである。

`domain.LiveTranscodeRequest.Quality` が空なら、変換の引数は画質を足す前と 1 文字も変わらない。
画質があるとき、`internal/media` の変換は次のようにする。

- 映像はコピーできる動画でも必ずエンコードする。コピーを試さないので、途中からの変換も指定位置
  そのものから始まる。
- 寸法は表示の寸法（`displayGeometry`）から `qualityDimensions` で決め、`scale=W:H` を出す。縮める比は
  「画質の短辺への比」と「今の変換の枠（長辺 3840・短辺 2160）への比」の小さい方で、幅・高さとも
  最も近い偶数に丸める。短辺で決めるので、縦長の 1080×1920 の `480p` は 480×854 になり、横長と
  同じ重さになる。長辺が枠を超える極端に細長い動画だけは短辺が画質より小さくなる
  （1200×12000 の `1080p`・`720p`・`480p` は 384×3840、`360p` は 360×3600）。既存の `setsar` の
  扱いはそのまま通し、表示の縦横比を保つ。
- 符号化器には表の上限を付ける。software と NVENC は一定品質に上限を重ね、QSV・VAAPI・
  VideoToolbox は上限の VBR にする（[方式ごとの引数](hardware-encoding.md#方式ごとの引数)）。
  ハードウェアが使えず software に切り替えたときも、同じ寸法と上限で変換する。
- 音声はコピーせず、常に AAC（`-ac 2 -ar 48000`）で表の kbps にエンコードする。コピーだと元の
  256〜320 kbps が残る。
- H.264 High・Level 5.1・4:2:0 8bit、`-force_key_frames`（出力の時刻で 2 秒ごと）、`-movflags`、
  fps の扱いは画質の無い変換と同じである。

上限が効いていることは ffmpeg 付きの Go テスト（`TestTranscodeQualityCapsBitrateWithFFmpeg`）が、
一面のノイズの入力を `480p` で変換し、出力の短辺が 480、映像の平均ビットレート（パケットの大きさの
合計を最初と最後の `pts_time` の差で割る）が 1200 kbps の 1.2 倍以下であることで確かめる。
ハードウェアの方式の上限は CI では確かめられないので、
[quickstart.md](../../specs/027-playback-quality/quickstart.md) の手順で実機で確かめる。

### Alternatives

- **長辺で縮める**: 縦長の動画で `720p` が 405×720 になり、横長の `720p` より軽くなる。
- **`-b:v` だけの平均ビットレート**: 瞬間の上限が無く、動きの多い場面で回線を超える。
- **`scale=-2:480` で寸法の計算を ffmpeg に任せる**: 縦長の向きの判定を ffmpeg 側にも持つことになり、
  Go の計算とテストが二重になる。
- **画質の短辺をそのまま守って枠を超える**: 1080×10800 は H.264 Level 5.1 の 1 フレームの上限を超える。

## API

### Context

画質は見る人が選び、再生の途中でも切り替える。経路を増やしたりサーバーに画質を覚えさせたりすると、
シークや再開の要求と画質の対応を別に管理することになる。

### Decision

`GET /api/videos/{id}/transcode.mp4` に任意の `quality`（`1080p`・`720p`・`480p`・`360p`）を足す
（[contracts/transcode-quality-api.md §1](../../specs/027-playback-quality/contracts/transcode-quality-api.md#1-get-apivideosidtranscodemp4-の-quality)）。
サーバーは画質を覚えず、要求ごとに `quality` から決める。

- `quality` が無ければ元の画質で、変換は今までと同じである。
- `internal/httpapi/transcode.go` は値を `domain.ParseTranscodeQuality` で解釈し、動画の `Width`・
  `Height`（表示の寸法）で `TranscodeQuality.Available` を確かめる。列挙に無い値、動画の短辺以上の
  画質、寸法の無い動画は 400 `invalid_request`（英語の `message`）で、変換を始めない。
- 使える画質は `LiveTranscodeRequest.Quality` に載せ、開始のログ（Debug）に `quality` を添える。
  画質の無い要求は `quality=original` と書く。
- `startMs`・`attempt` は今までどおり組み合わせられる。画質のある変換は映像をエンコードして
  `startMs` の位置そのものから始まるので、`transcode-start` は `startMs` と同じ値を返す。
- 応答の形（fragmented MP4、`Cache-Control: no-store`）とエラーの形は変えない。境界はゲストも
  使える経路のまま（公開の動画だけ）である。

### Alternatives

- **画質ごとの経路**: 開始位置の台帳や公開の境界を経路ごとに重ねることになる。
- **サーバーが見る人ごとに画質を覚える**: ゲストには見る人の識別が無く、要求の URL だけでは
  出力が決まらなくなる。
- **使えない画質を黙って元の画質か最大の画質に直す**: 画面の選択と実際の画質がずれる。

## 選択肢と覚え方

### Context

画質は見る人の回線に合わせて選ぶもので、動画ごとではなくブラウザごとに決まる。一度選んだら
次に開く動画でも、シークや通信の失敗からの読み込み直しでも同じ画質で再生し続ける必要がある。
一方で、動画の短辺以上の画質はサーバーが 400 で拒む（上の「API」）。

### Decision

- 選んだ画質は `web/src/preferences/playbackQuality.ts` が `localStorage` の
  `vv.playback-quality.v1` に JSON の文字列（`"480p"`・`"original"`）で持つ。音量
  （`playbackVolume.ts`）と同じ作りで、保存値が無い・壊れている・列挙に無い・保存領域が
  使えないときは「元の画質」（`"original"`）で再生する。サーバーには送らない。
- 選択肢は `web/src/player/quality.ts` の `qualityOptions` が動画の `width`・`height` から作る。
  短辺より小さい画質だけを大きい順に出し、寸法の無い動画は空にする。規則はサーバーの
  `TranscodeQuality.Available` と同じ短辺の比較だけで、変換の枠による縮小（1200×12000 など）は
  考えない。
- 覚えている画質が選択肢に無い動画（360p の動画での `480p` など）は、`effectiveQuality` が
  「元の画質」で再生すると決める。覚えている値は書き換えず、次に大きい動画を開けばまた
  その画質で再生する。
- プレイヤー（`VideoPlayer.tsx`）は作るときに 1 回だけ覚えた画質を読み、`createPlaybackAttempt`
  に渡す。`PlaybackAttempt.quality` が「元の画質」以外なら経路は直接再生できる動画でも変換で、
  `sourceOffsetMs` は再開する位置になる。「元の画質」なら今までどおり、直接再生できる動画だけを
  直接再生する。
- 画質は変換の source が持ち続ける。`transcodeUrl` と `liveSource` は `quality` を URL と
  source の `vvQuality` に載せ、未 buffer のシーク（`liveOffset.ts` の `reloadAt`）は source の
  画質で作り直し、通信の失敗からの読み込み直し（`VideoPlayer.tsx` の `reload`）は attempt の
  画質で作り直す。どちらも画質の無い要求に戻らない。
- 「変換して再生中」は、元の画質なら今までどおりの文言、選んだ画質なら `Converting to 480p` と
  縮めていることと戻し方の説明にする（[ui-design.md「Control bar: transcode indicator」](../../specs/027-playback-quality/ui-design.md#control-bar-transcode-indicator)）。
  画質の名前は翻訳しない。

### Alternatives

- **サーバーに画質を覚えさせる**: ゲストには見る人の識別が無い（上の「API」）。
- **使えない画質を覚えた値ごと「元の画質」に書き換える**: 小さい動画を 1 本開いただけで、
  回線に合わせて選んだ画質が失われる。
- **選択肢をサーバーの応答に載せる**: 規則が寸法の比較だけなのに `Video` の形と往復が増える。
