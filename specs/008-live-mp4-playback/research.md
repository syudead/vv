# Research: MP4 ライブ変換による動画再生

既存の技術境界は [tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) と
[ARCHITECTURE.md](../../ARCHITECTURE.md) を継承する。ここでは本機能が追加する判断だけを記録する。

## R-201: 変換 transport は fragmented MP4 の HTTP 直送

**Decision**: FFmpeg を request ごとに起動し、`frag_keyframe+empty_moov+default_base_moof` の
fragmented MP4 を stdout から HTTP response へ直送する。変換結果をdiskへ保存せず、responseは
`Cache-Control: no-store` とする。

**Rationale**: FFmpeg の MP4 muxerはfragmented outputを生成でき、W3CのISO BMFF byte-stream定義も
初期化segmentと`moof`/`mdat` media segmentを逐次処理できる形を定めている。StashもMP4ライブ変換で
同じ`frag_keyframe+empty_moov`と`pipe:`を使い、requestへ`io.Copy`している。

**Alternatives considered**:

- DASH: FFmpeg DASH muxerはMPDとsegment filesを生成する。短命でもsegment保持・回収が必要になり、
  単一品質かつ保存しない今回の要件より大きい仕組みになるため不採用。
- 通常MP4: 完了まで`moov`を確定できず、変換しながら再生を始められないため不採用。
- HLS: Apple互換も複数renditionも今回の要求になく、playlist/segment管理だけが増えるため不採用。

**Primary sources**:

- [FFmpeg Formats: MOV/MP4 fragmentation](https://ffmpeg.org/ffmpeg-formats.html)
- [W3C ISO BMFF Byte Stream Format](https://www.w3.org/TR/mse-byte-stream-format-isobmff/)
- [Stash MP4 transcode implementation](https://github.com/stashapp/stash/blob/develop/pkg/ffmpeg/stream_transcode.go)

## R-202: シークは source 再起動と仮想時間軸で扱う

**Decision**: ライブ変換sourceに元動画のdurationと開始offsetを持たせ、未buffer位置へのseek時は
`startMs`を変えてsourceを再読込する。Video.js middlewareがduration、currentTime、buffered rangeを
元動画の時間軸へ写し、連続scrubはdebounceする。

**Rationale**: 途中位置から始めたpipe outputの内部時刻は0へ戻るため、元動画の全体尺と論理位置を
player側で補う必要がある。Video.js middlewareは`duration`、`currentTime`、`setCurrentTime`、
`buffered`の変換を公式に提供する。Stashの実装も未buffer位置で`start`を付け直してsourceを再読込し、
offsetを加えた時間軸を提示している。

**Alternatives considered**:

- native `<video>`だけ: 内部時刻を元動画全体の時刻として上書きできず、途中再開後のseek barとprogress
  が一致しないため不採用。
- Media Source Extensionsを直接操作: timestamp、append queue、buffer eviction、codec changeを本アプリが
  所有することになり、単一pipe sourceのために必要以上のbrowser media engineを自作するため不採用。
- DASH.js: 正しいseekを提供するが、R-201で不要としたmanifest/segment lifecycleを再導入するため不採用。

**Primary sources**:

- [Video.js middleware guide](https://legacy.videojs.org/guides/middleware/)
- [Stash offset middleware](https://github.com/stashapp/stash/blob/develop/ui/v2.5/src/components/ScenePlayer/live.ts)
- [Stash transcode route start parameter](https://github.com/stashapp/stash/blob/develop/internal/api/routes_scene.go)

## R-203: codec選択は互換属性をrequest時に検査する

**Decision**: transcode requestの開始時にffprobeを実行し、codec名に加えてprofile、level、pixel format、
bit depth、寸法、AAC profile、sample rate、channel数を取得する。H.264はBaseline/Constrained Baseline/
Main/High、8-bit `yuv420p`、level 5.1以下をすべて確認できた場合だけcopyする。AACはLC、1〜2 channel、
8〜48 kHzをすべて確認できた場合だけcopyする。値が欠落・未知・範囲外なら対応streamをencodeする。
このrequest時probe結果は永続化しない。

映像変換は`libx264`/`yuv420p`/`preset veryfast`/`crf 23`とし、encode時だけ
`pad=ceil(iw/2)*2:ceil(ih/2)*2`で右端・下端を最大1px補って偶数寸法にする。映像内容を拡大・縮小
しない。音声変換はAAC-LC stereo 192kbpsとする。字幕/data streamは出力しない。

**Rationale**: codec名だけではHigh 4:4:4 Predictiveや10-bit H.264を一般的なbrowser向けH.264と
区別できない。保守的な属性allowlistなら、containerだけが問題の互換streamは品質を保ってcopyしつつ、
判定できないstreamを安全側でencodeできる。`yuv420p` encodeは奇数寸法で失敗するため、必要な場合だけ
最大1pxのpaddingを加える。

**Alternatives considered**:

- 常に映像・音声を再encode: 容器だけ非対応の動画にも品質劣化とCPU負荷を加えるため不採用。
- codec名だけでcopy: profile、pixel format、bit depth、AAC profileによる非互換性を見落とすため不採用。
- 互換属性をDBへ追加: transcode requestだけが使う値のmigrationとAPI公開を増やすため不採用。
- 解像度別rendition: 要求された選択肢ではなく、scaleとsource選択UIを増やすため不採用。

**Primary sources**:

- [FFmpeg seeking and accurate seek](https://ffmpeg.org/ffmpeg.html)
- [FFmpeg pad filter](https://ffmpeg.org/ffmpeg-filters.html#pad-1)
- [MDN Web video codec guide](https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Formats/Video_codecs)
- [Stash stream codec selection](https://github.com/stashapp/stash/blob/develop/pkg/ffmpeg/stream_transcode.go)

## R-204: processはHTTP requestが単独所有する

**Decision**: 1回のtranscode responseにつき1個の`exec.Cmd`を起動し、request contextで停止する。
stdoutはresponseへ流し、stderrは上限付きで常にdrainし、`Wait`は1か所だけが呼ぶ。seek、source切り替え、
画面離脱、切断、server shutdownのどれでもcontext cancellationから同じ終了経路へ入る。

**Rationale**: requestとprocessの寿命を一致させれば、永続session registryやcleanup timerなしで不要処理を
止められる。同じ動画を複数tabで開いてもrequest contextが別なので互いに停止しない。

**Alternatives considered**:

- 動画単位でprocessを共有: 一方のseek/終了がもう一方のoffsetと寿命へ干渉するため不採用。
- background jobへ投入: response切断をjob cancellationへ確実に伝える別registryが必要になり、既存の
  再開可能な取り込みjobとは寿命も意味も異なるため不採用。
- stderrを`io.ReadAll`する: 長時間の異常出力でmemoryを無制限に消費できるため不採用。

## R-205: fallback判定はbrowser、経路選択は有限

**Decision**: 解析結果が`playable=true`ならdirect sourceから開始し、media decode/source errorをbrowserで
観測したときだけ同じ論理位置からtranscodeへ1回切り替える。`playable=false`はtranscodeから開始する。
transcode errorは最終errorにして次のsourceを試さない。再生意図がないmetadata preload中の切り替えでは
autoplayせず、再生中のfailureだけ再生を継続する。

**Rationale**: 取り込み時判定だけでは実browserのdecoder差を完全には把握できないが、server側には実際の
decode errorが見えない。browserがfailureを通知し、attempted routeを明示状態にすれば要求順序を保ち、
循環を防げる。

**Alternatives considered**:

- server-side User-Agent判定: codec/profile/device差を正確に表せず、実際のfailureも観測できないため不採用。
- errorごとにdirectへ戻す: 同じ2経路を循環し、最終errorへ到達しないため不採用。
- すべてtranscodeから開始: 既存の高速なRange直接配信を失うため不採用。

**Primary source**:

- [Stash finite source fallback](https://github.com/stashapp/stash/blob/develop/ui/v2.5/src/components/ScenePlayer/source-selector.ts)

## R-206: 途中開始は正確さを優先して全streamをencodeする

**Decision**: `startMs=0`の既知非対応動画だけR-203の選択的copyを許可する。`startMs>0`のrequestは、
映像をH.264、存在する音声をAACへencodeし、input-side `-ss`の既定`accurate_seek`で要求位置より前の
frame/sampleを捨てる。direct media error後のfallbackも開始位置にかかわらず同じ全正規化を使う。
playerの`sourceOffsetMs`には要求した`startMs`をそのまま使う。

**Rationale**: FFmpegのinput-side seekは最寄りのseek pointへ移動する。transcode時は要求位置までの余分な
区間をdecodeして捨てるが、stream copy時はその区間を保持する。copy出力を要求位置として扱うと、映像、
表示時刻、保存progressがGOP長だけずれる。途中開始でのencodeは正確な時間軸に必要な処理であり、先頭から
のcontainer-only変換では引き続きcopyを使う。

**Alternatives considered**:

- copy出力の実際の先頭timestampをclientへ返す: body送信前に確定・伝達する追加protocolが必要で、
  音声と映像の先頭差もclientへ漏れるため不採用。
- output-side seekとcopyを併用する: 先頭から要求位置までdecode/demuxするため、長い動画の2秒以内のseekを
  満たせないため不採用。
- keyframeまでの巻き戻りを許容する: seek barと保存progressが実映像と一致せずFR-006/FR-007に反するため
  不採用。

**Primary source**:

- [FFmpeg `-ss` and accurate seek](https://ffmpeg.org/ffmpeg.html)
