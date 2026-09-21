# Live Playback HTTP Contract

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md)

JSON/binary responseのschema正本は [api/openapi.yaml](../../../api/openapi.yaml) とする。本書はOpenAPIに
書き切れない再生経路、stream lifecycle、codec選択を定める。既存の直接配信、Range、path安全性は
[002 HTTP契約](../../002-core-video-library/contracts/http-routes.md)を変更せず継承する。

## Playback route order

| Condition | First route | On media error | Final outcome |
| --- | --- | --- | --- |
| `probeState=done`, `playable=true` | `/stream` | 同じ論理位置の`/transcode.mp4`へ1回切替 | transcode errorを表示 |
| `probeState=done`, `playable=false` | `/transcode.mp4` | 別経路を試さない | errorを表示 |
| probe未完了・失敗、映像なし、尺なし | 再生要求を送らない | なし | errorを表示 |

playerはdirectへ戻らず、transcodeを再帰的に再試行しない。source切替時に既存の再生位置保存を継続する。

## `GET /api/videos/{id}/transcode.mp4`

元動画を指定位置からfragmented MP4へ変換しながら返す。

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `startMs` | int64 | no | default 0、`0 <= startMs < durationMs` |

Range headerはこの経路のseek入力ではない。付いていてもserverはRange responseを作らず`200`を返す。
seekは新しい`startMs` requestで行う。

### Success

- Status: `200 OK`
- `Content-Type: video/mp4`
- `Cache-Control: no-store`
- `Accept-Ranges`と`Content-Length`は付けない
- Body: `ftyp`/`moov` initializationに続く`moof`/`mdat` fragments
- `Content-Disposition`は付けず、download filenameを示さない

FFmpeg outputはresponseへ直接流し、data directoryやmedia folderへfileとして書かない。

### Errors before body starts

| Condition | Status | Error code |
| --- | --- | --- |
| `id`または`startMs`が不正 | `400` | `invalid_request` |
| 動画、current location、実体がない／安全に開けない | `404` | `not_found` |
| 取込probeまたはrequest時互換probe失敗、尺なし、映像streamなし | `409` | `conflict` |
| FFmpegを起動できない | `500` | `internal` |

Error responseは既存`Error` JSONと`Cache-Control: no-store`を使い、filesystem pathやFFmpeg command全文を
返さない。body開始後のFFmpeg failureはstatusを変更できないためstreamを終了し、playerのmedia errorを
最終errorへ変換する。原因と上限付きstderr tailはserver logへ残す。

## Codec selection

最初の映像streamと、存在する場合は最初の音声streamだけを出力し、subtitle/data streamは含めない。
resolution renditionは追加しない。transcode request開始時にffprobeで下記属性を取得し、判定結果は
永続化しない。属性が欠落・未知・範囲外ならcopyせずencodeする。

### Selective mode

取り込み時に`playable=false`と判定された動画に使う。

| Input stream | Output action |
| --- | --- |
| H.264 Baseline/Constrained Baseline/Main/High、level 5.1以下、8-bit `yuv420p` | copy |
| 上記を全て確認できないvideo | H.264 (`libx264`, `yuv420p`, `preset=veryfast`, `crf=23`) |
| AAC-LC、1〜2 channel、8〜48 kHz | copy |
| 上記を全て確認できないaudio | AAC-LC stereo 192kbps |
| audioなし | audioを出力しない |

H.264 encode時は`pad=ceil(iw/2)*2:ceil(ih/2)*2,format=yuv420p`を適用する。奇数の幅・高さだけ右端・
下端へ最大1px補い、元の映像内容は拡大・縮小しない。

### Normalize mode

`playable=true`と判定済みの動画がdirect media error後にこの経路へ来た場合、または`startMs>0`の
場合に使う。映像を上表のH.264設定、音声があればAAC設定へ変換する。前者では取り込み時判定が
見落としたbrowser差を持ち越さず、後者では要求位置より前のframe/sampleを正確に捨てる。

## FFmpeg and request lifecycle

- `startMs>0`ではinput-side seekと既定のaccurate seekを使い、映像・音声をencodeして要求位置より前を
  捨てる。`sourceOffsetMs`は要求した`startMs`と一致する
- outputは`frag_keyframe+empty_moov+default_base_moof`のMP4をstdoutへ出す
- HTTP request contextをFFmpeg command contextに渡す
- seek、source切替、画面離脱、接続切断、server shutdownで旧contextをcancelする
- stderrをstdoutと同時に上限付きでdrainし、pipe詰まりを起こさない
- cancel後10秒以内にprocess、pipe、drain goroutineを終了する
- 同じ動画を含む別requestのprocessを停止しない

## Cache behavior

既存`/stream`の`private, max-age=0, must-revalidate`は変更しない。`/transcode.mp4`は毎回のrequestと
processが1対1であり、browser・proxy・serverの再利用cacheを持たせず`no-store`とする。
