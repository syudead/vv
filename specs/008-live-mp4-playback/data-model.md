# Runtime Model: MP4 ライブ変換による動画再生

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

本機能はSQLite entityを追加せず、既存の動画・location・playback progress schemaも変更しない。
以下は1画面または1 HTTP requestの寿命だけを持つ実行時modelである。

## Playback Attempt

1つの再生画面が1動画について持つ有限state machine。

| Field | Meaning | Validation |
| --- | --- | --- |
| `videoId` | 既存動画ID | 1以上の整数 |
| `durationMs` | 解析済みの元動画尺 | transcode開始には1以上が必要 |
| `route` | `direct` / `transcode` | 同時に1つだけ |
| `state` | `loading` / `ready` / `playing` / `failed` / `disposed` | 下記遷移だけを許可 |
| `fallbackTried` | directからtranscodeへ切り替え済みか | `false`から`true`への一方向 |
| `logicalPositionMs` | 元動画全体での現在位置 | `0..durationMs`へclamp |
| `sourceOffsetMs` | 現sourceが元動画のどこから始まるか | directは0、transcodeは正確に捨てたseek位置 |
| `playIntended` | failure前に再生中または再生要求済みだったか | source切替後のautoplay可否だけに使う |

### Initial route

- `probeState=done`かつ`playable=true`: `direct`
- `probeState=done`かつ`playable=false`: `transcode`
- 解析が完了していない、または尺・映像streamが得られない: 再生を始めず`failed`

### Transitions

```text
loading -> ready(direct | transcode)
ready -> playing
direct ready/playing -- decode or source error --> transcode ready, fallbackTried=true
transcode ready/playing -- error --> failed
failed -> disposed
ready/playing -> disposed
```

`fallbackTried=true`から`direct`へ戻る遷移と、`transcode`から別の自動再生経路への遷移は存在しない。
directからの切替時は`logicalPositionMs`と`playIntended`を保持する。

## Transcode Request

1つの`GET /api/videos/{id}/transcode.mp4`に対応する値。

| Field | Meaning | Validation |
| --- | --- | --- |
| `videoId` | 対象動画 | current locationが1件以上必要 |
| `sourcePath` | root・symlink・regular-file検証済みlocation | 応答やlogへpathを公開しない |
| `startMs` | 変換開始位置 | 省略時0、`0 <= startMs < durationMs` |
| `videoStreamIndex` | 最初の非添付video streamの絶対index | `attached_pic=0`、存在しなければrequest失敗 |
| `audioStreamIndex` | 最初のaudio streamの絶対index | 音声なしは`none` |
| `mode` | `selective` / `normalize` | playable判定と`startMs`からserverが決定 |
| `videoAction` | `copy` / `h264` | 映像streamが無い場合はrequest失敗 |
| `audioAction` | `none` / `copy` / `aac` | 音声なしは`none` |

`selective`は既知の非対応動画を`startMs=0`から再生する場合だけ使う。request時probeでprofile、level、
pixel format、bit depth、寸法、AAC profile、sample rate、channel数、stream index、attached-pic dispositionを
取得する。最初の非添付videoを本編として選び、互換性をすべて確認できたstreamだけをcopyする。結果は
request終了時に破棄する。

`normalize`はdirect再生可能と判定済みの動画、または`startMs>0`のrequestに使う。映像と存在する音声を
互換設定へ変換し、正確なseekのため要求位置より前のframe/sampleを捨てる。H.264 encode時は奇数の幅・
高さだけ右端・下端へ最大1px paddingし、映像内容はscaleしない。

## Transcode Process

Transcode Requestから起動され、同じHTTP requestだけが所有する。

| Field | Meaning |
| --- | --- |
| `command` | 引数配列として組み立てたFFmpeg command |
| `stdout` | fragmented MP4 response source |
| `stderrTail` | 記録用の上限付き末尾buffer |
| `state` | `starting` / `streaming` / `stopping` / `stopped` |
| `result` | 正常終了、request cancellation、FFmpeg failure |

### Lifecycle invariants

- process開始後はstdoutとstderrを並行してdrainする。
- response write failureまたはrequest cancellationで`stopping`へ進み、child processを停止する。
- server lifetime cancellationでも同じ`stopping`へ進む。停止指示はHTTP shutdownの待機前に発火する。
- `Wait`は1回だけ呼び、pipeを閉じて`stopped`へ進む。
- cancel後5秒で終了しなければprocessをkillし、HTTP shutdownの10秒猶予を使い切らない。
- `stopped`後にfile、process、goroutineを残さない。
- process間でbuffer、offset、cancel functionを共有しない。

## Existing relationships

- Playback Attemptのposition保存は既存`PUT /api/videos/{id}/progress`だけを使い、routeを保存しない。
- Transcode Requestは既存のcurrent video location選択とpath安全性規則を使う。
- 元動画、thumbnail、playback progress、scan/job rowsはTranscode Processから書き換えない。
