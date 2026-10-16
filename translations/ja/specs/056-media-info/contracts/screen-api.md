---
source: specs/056-media-info/contracts/screen-api.md
sourceHash: da0a3fdc06eafbab472d2a323ad2882aacd5c9b090ebdd3b33dd788725ec5861
---

# 契約: メディア情報のための画面の API {#contract-screen-api-for-media-information}

正本: [`api/openapi.yaml`](../../../api/openapi.yaml)。この文書は、この機能が加えるか変えるものだけを持つ。エラーの形は [023 error-api.md](../../023-english-i18n/contracts/error-api.md) に従う。`api/external-v1.yaml` は変わらない: 今は動画の技術的な事実を何も返さず、新しい項目での並べ替えや絞り込みは範囲外である。

## `GET /api/videos/{id}`: `mediaInfo` {#get-apivideosid-mediainfo}

`Video` に `mediaInfo` (`MediaInfo`) が加わる。この操作だけにあり、動画が今のバージョンの `video_media_info` の行を持つときだけある ([R-6](../research.md#r-6-mediainfo-is-part-of-the-video-detail-only-for-owners-and-guests-alike))。公開の動画を読むゲストは、所有者と同じ値を得る。一覧の項目、関連する動画、ライブラリの項目、所有者の `PUT` の応答はそれを省く。

`MediaInfo` とその中のオブジェクトのすべてのプロパティは省略でき、ファイルがその項目を持たないときは省く ([data-model.md](../data-model.md))。プロパティのないオブジェクトは丸ごと省く。そのため、ページがグループを示すのは、示すものがあるときだけである。

| プロパティ | 型 | 意味 |
| --- | --- | --- |
| `video` | `MediaInfoVideo` | 映像のストリームの項目 |
| `audioTracks` | `MediaInfoAudioTrack[]` | すべての音声のストリーム、ストリームの順 |
| `subtitleTracks` | `MediaInfoSubtitleTrack[]` | すべての字幕のストリーム、ストリームの順 |
| `embedded` | `MediaInfoEmbedded` | ファイルに埋め込まれたタグ |
| `broadcast` | `MediaInfoBroadcast` | TS の録画の放送局と番組 |
| `chapters` | `MediaInfoChapter[]` | 開始で並べた、長さの中のチャプター |

`MediaInfoVideo`:

| プロパティ | 型 | 意味 |
| --- | --- | --- |
| `frameRate` | number | 毎秒のフレーム数 (`avg_frame_rate`) |
| `bitDepth` | integer | サンプルごとのビット数 |
| `bitRate` | integer (int64) | 映像のストリームの毎秒のビット数 |
| `interlaced` | boolean | ファイルがプログレッシブかインターレースかを示すときだけある |
| `hdr` | `sdr` \| `hdr10` \| `hlg` \| `dolby_vision` | `video` があるときは常にある |
| `projection` | string | 360 度の動画の `equirectangular`、`cubemap`、またはほかの `ffprobe` の投影の名前 |
| `stereo3d` | string | `side by side`、`top and bottom`、またはほかの `ffprobe` のステレオの種類 |
| `closedCaptions` | boolean | 映像のストリームがクローズドキャプションを運ぶときは `true` |

`MediaInfoAudioTrack` と `MediaInfoSubtitleTrack`:

| プロパティ | 型 | 音声 | 字幕 | 意味 |
| --- | --- | --- | --- | --- |
| `index` | integer | 必須 | 必須 | ファイルの中のストリームの番号 |
| `codec` | string | あり | あり | `ffprobe` の `codec_name` (`aac`、`subrip`、`arib_caption`) |
| `channels` | integer | あり | なし | チャンネル数 |
| `channelLayout` | string | あり | なし | `stereo`、`5.1` など |
| `language` | string | あり | あり | ファイルの言語のタグ。`und` は省く ([R-8](../research.md#r-8-language-codes-are-shown-through-intldisplaynames)) |
| `title` | string | あり | あり | トラックの名前 |
| `default` | boolean | あり | あり | 既定のディスポジション |
| `forced` | boolean | なし | あり | 強制のディスポジション |
| `hearingImpaired` | boolean | なし | あり | 聴覚障害者向けのディスポジション |

`MediaInfoEmbedded`:

| プロパティ | 型 | 意味 |
| --- | --- | --- |
| `title`、`author`、`comment`、`description`、`genre` | string | ファイルのタグ |
| `date` | string | テキストとしての、ファイルの `date` のタグ |
| `recordedAt` | string (date-time) | 撮影の日時。ファイルがオフセットを持つときはそのオフセット付き |
| `device` | string | メーカーと機種 |
| `location` | `{latitude: number, longitude: number, altitude?: number}` | 撮影の場所 |
| `coverArt` | boolean | ファイルが添付の画像を持つときは `true` |

`MediaInfoBroadcast`: `station` と `program`。文字列。

`MediaInfoChapter`: `startMs` と `endMs` (integer、int64、必須)、`title` (string、省略可)。

**応答**: 今と同じく `Video` とともに `200`。今と同じく `404`。

## 値が増える列挙 {#enumerations-that-gain-a-value}

| スキーマ | 加わる値 | いつ |
| --- | --- | --- |
| `ScanActivityKind` | `metadata` | `metadata` のジョブが実行中のとき ([R-5](../research.md#r-5-imported-videos-are-re-analysed-by-a-metadata-job-that-yields-to-pending-probes)) |
| `ScanIssueKind` | `metadata_failed` | 最新のスキャンの集合にある動画で、`metadata` のジョブが試行回数の上限で失敗したとき |

## `GET /api/events`: `video` {#get-apievents-video}

新しい種類はない。終わった `metadata` のジョブは、すべての取り込みの段階と同じように、その動画の `video` の通知を送る。そのため、開いている動画ページは詳細を取得し直し、新しい `mediaInfo` を示す。

## クライアントでの使い方 {#client-use}

動画ページは、すでに取得する詳細から `mediaInfo` を読む。チャプターは `chapters` のテキストトラックとしてプレイヤーに渡る ([R-7](../research.md#r-7-chapters-are-a-chapters-text-track-with-marks-drawn-in-the-videojs-seek-bar))。言語は `Intl.DisplayNames` で名前として示す (R-8)。
