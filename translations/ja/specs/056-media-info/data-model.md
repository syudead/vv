---
source: specs/056-media-info/data-model.md
sourceHash: a50ad4000a9c20ccf5659192af19bebe59d775e7d5442321da542976b9de4b34
---

# データモデル: 取り込みのプローブから得るメディア情報 {#data-model-media-information-from-the-import-probe}

モデルの残りは変わらない: プローブが埋める `videos` の列、ライブ変換の行、ジョブのキューは、[018 データモデル](../018-live-transcode-seek/data-model.md)、[020 データモデル](../020-seek-thumbnail-stage/data-model.md)、[`internal/store/migrations`](../../internal/store/migrations) のマイグレーションに従う。

## マイグレーション {#migration}

2 つのマイグレーションがあり、それぞれ、その単位を書く時点の `main` の最後のものの次の番号を取る (ほかの feature ブランチもマイグレーションを加える)。

| マイグレーション | 単位 | すること |
| --- | --- | --- |
| `video_media_info` | 取り込み時にメディア情報を読んで保存する | [`video_media_info`](#video_media_info) を作る |
| `metadata_jobs` | 取り込み済みの動画をバックグラウンドで再解析する | `00025_video_fingerprints.sql` がしたように、`kind` の制約に `metadata` を入れて `jobs` を作り直し、`probe_state = done` で登録されたフォルダの下に場所を持つ動画ごとに `metadata` のジョブを 1 つ入れる。待ち中か実行中のものをすでに持つ動画は飛ばす |

どちらのテーブルも [Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery) の作り直せるインデックスである: スキャンとそれが入れるジョブが、もう一度埋める。

## `video_media_info` {#video_media_info}

動画ごとに 1 行で、プローブのジョブと `metadata` のジョブが書く ([R-1](research.md#r-1-media-information-is-one-versioned-json-row-per-video))。

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `video_id` | integer | 不可 | 主キー。`references videos (id) on delete cascade` |
| `version` | integer | 不可 | 書いた時点の `domain.MediaInfoVersion`。違う値はないものとして読まれる |
| `info` | text | 不可 | [`domain.MediaInfo`](#stored-value-domainmediainfo) の JSON |
| `updated_at` | integer | 不可 | 書いた時点の Unix 秒 |

**関係**: 行はその `videos` の行とともに消える。あるパスで内容が変わると動画の行が作り直される (`UpsertVideo`) ので、新しい行はメディア情報なしで始まり、そのプローブのジョブがそれを書く。

## 保存する値: `domain.MediaInfo` {#stored-value-domainmediainfo}

`internal/domain` の値の型で、`internal/media` が 1 回の `ffprobe` の実行 ([R-2](research.md#r-2-the-same-ffprobe-run-reads-chapters-and-reads-longer-for-mpeg-ts)) と、TS のファイルでは SI リーダー ([R-4](research.md#r-4-station-and-programme-names-of-a-ts-recording-come-from-a-go-si-reader-with-an-arib-std-b24-decoder)) から埋める。`domain.Probe` の上を `Media *MediaInfo` として運ばれる。JSON のフィールド名は Go のフィールド名である。ない項目は JSON から省く (`omitempty`、nil のポインター、空のスライス)。`MediaInfoVersion` は 1 から始まり、フィールドを加えるか、その意味が変わるときに上げる。

映像のストリームは、添付でない最初の映像のストリームであり、インデックスの列がすでに表すものである。

| フィールド | `ffprobe` での出どころ | ないとき |
| --- | --- | --- |
| `Video.FrameRate` (float64) | 映像のストリームの `avg_frame_rate` | `0/0` か解析できない |
| `Video.BitDepth` (int) | `bits_per_raw_sample`、なければ `pix_fmt` の深度 ([R-3](research.md#r-3-hdr-projection-3d-and-interlacing-are-decided-by-pure-rules-in-internaldomain)) | どちらも数を与えない |
| `Video.BitRate` (int64、ビット毎秒) | 映像のストリームの `bit_rate`、なければその `tags.BPS` (Matroska) | どちらもない |
| `Video.Interlaced` (*bool) | `field_order` | `unknown` かない |
| `Video.HDR` (`sdr`、`hdr10`、`hlg`、`dolby_vision`) | `color_transfer`、`side_data_list`、`codec_tag_string` (R-3) | 決してない: 映像のストリームは少なくとも `sdr` である |
| `Video.Projection` (string) | `side_data_list` の項目 `Spherical Mapping` の `projection` | その項目がない |
| `Video.Stereo3D` (string) | `side_data_list` の項目 `Stereo 3D` の `type` | その項目がない |
| `Video.ClosedCaptions` (bool) | 映像のストリームの `closed_captions` | `0` |
| `AudioTracks[]` | すべての `audio` のストリーム、ストリームの順 | 音声のストリームがない |
| `AudioTracks[].Index` | `index` | 決してない |
| `AudioTracks[].Codec` | `codec_name` | 空 |
| `AudioTracks[].Channels` (int)、`ChannelLayout` (string) | `channels`、`channel_layout` | `0`、空 |
| `AudioTracks[].Language`、`Title` | `tags.language`、`tags.title` | 空か `und` |
| `AudioTracks[].Default` (bool) | `disposition.default` | `0` |
| `SubtitleTracks[]` | すべての `subtitle` のストリーム、ストリームの順 | 字幕のストリームがない |
| `SubtitleTracks[].Index`、`Codec`、`Language`、`Title`、`Default` | 音声と同じ | 音声と同じ |
| `SubtitleTracks[].Forced` (bool) | `disposition.forced` | `0` |
| `SubtitleTracks[].HearingImpaired` (bool) | `disposition.hearing_impaired` | `0` |
| `Embedded.Title`、`Comment`、`Description`、`Genre`、`Date` | `format.tags` の `title`、`comment`、`description` (なければ `synopsis`)、`genre`、`date` | 空。`Date` はファイルのテキストのまま保つ |
| `Embedded.Author` | `format.tags` の `artist`、なければ `author`、なければ `album_artist` | すべて空 |
| `Embedded.RecordedAt` (RFC 3339 の文字列) | `format.tags` の `com.apple.quicktime.creationdate`、なければ `creation_time` | どちらも日付として解析できない ([規則](#rules)) |
| `Embedded.Device` | 空白でつないだ `com.apple.quicktime.make` と `com.apple.quicktime.model`、なければ `com.android.manufacturer` と `com.android.model`、なければ `make` と `model` | すべて空 |
| `Embedded.Location` (`{Latitude, Longitude, Altitude *float64}`) | ISO 6709 として解析した `com.apple.quicktime.location.ISO6709`、なければ `location`、なければ `location-eng` | どれも解析できない |
| `Embedded.CoverArt` (bool) | `disposition.attached_pic = 1` のストリーム | ない |
| `Broadcast.Station` | SI リーダーの SDT のサービス名 (R-4) | TS のファイルでないか、先頭の 32 MiB で見つからない |
| `Broadcast.Program` | [規則](#rules) が選んだ、EIT の現在か次のイベントの名前 (R-4) | TS のファイルでないか、先頭の 32 MiB に EIT か TOT/TDT がないか、どちらのイベントも録画と重ならない |
| `Chapters[]` | [規則](#rules) を適用した後の `chapters` | チャプターがない |
| `Chapters[].StartMs`、`EndMs` (int64) | ミリ秒での `start_time`、`end_time` | 決してない |
| `Chapters[].Title` | `tags.title` | 空 |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| `version` が `MediaInfoVersion` と違う行はないものとして読まれ、スキャンがその動画に `metadata` のジョブを入れる (`IndexedVideo.MediaInfoMissing`: `probe_state = done` で、今のバージョンの行がない) | `store` (読み取りとスキャンのインデックスのクエリ)、`scanner` (`ensurePendingJobs`) |
| プローブのジョブは、`videos` の更新と同じ同一性の確認 (ジョブの場所の id、バージョン、パス、内容の鍵、場所の世代) のもとで、`ApplyProbeForJob` のトランザクションの中で行を書く | `store` |
| `metadata` のジョブは、同じ同一性の確認のもとで行だけを書く。確認に失敗したときは何も書かず、ジョブは完了する | `store` (`ApplyMediaInfoForJob`)、`app` |
| `metadata` のジョブは、登録された場所を持つ動画に待ち中か実行中の `probe` のジョブがないときだけ取得できる (`JobClaimCondition.NoClaimableProbe`。`NoClaimableThumbnail` が `thumbnail` のジョブを数えるように数える) | `domain` (条件)、`store` (取得のクエリ) |
| 試行回数の上限で失敗した `metadata` のジョブは、行をそのままにする。問題の種類は `metadata_failed` である | `store` (`FailClaimedJob`) |
| すべての文字列は前後の空白を除き、改行以外の制御文字を取り除いた正しい UTF-8 でなければならず、2,000 文字で切る。空になったものはないものとする | `domain` |
| 保つのは最大 64 の音声トラック、64 の字幕トラック、1,000 のチャプターで、ストリームか時刻の順である | `domain` |
| `RecordedAt` は、オフセットの有無にかかわらず RFC 3339 と、`YYYY-MM-DD HH:MM:SS` を解析する。オフセットを持つ値はそれを保ち、持たない値は UTC とする。それ以外はないものとする | `domain` |
| `Location` は、座標の整数部が 2、4、6 桁の ISO 6709 の `±DD.DDDD±DDD.DDDD[±ALT]/` を解析する。±90 の外の緯度か ±180 の外の経度はないものとする | `domain` |
| チャプターは開始で並べる。`StartMs >= DurationMs` のチャプターは捨てる。`EndMs` は `DurationMs` までに切り、少なくとも `StartMs` にする。チャプターが 1 つでも保つ。重なるチャプターは与えられたとおりに保つ | `domain` (`NormalizeChapters`) |
| TS の録画の番組は、EIT の現在か次のイベントのうち、その範囲が録画 (TOT/TDT の時刻 `T` から、`T` にプローブした長さを足したところまで) と重なり、最も長く重なるものである。同じ長さなら次のイベントを選ぶ。長さが未定義の現在のイベントは次のイベントの開始で終わる。時刻がないか、重なりがなければ `Program` はない | `domain` (`SelectBroadcastProgram`) |
| 欠けている項目や読めない項目は省き、決してプローブを失敗させない。プローブを失敗させるのは、今と同じく、長さが欠けているときだけである | `media` (パーサー)、`domain` |

## 変わらないもの {#what-does-not-change}

| 項目 | 理由 |
| --- | --- |
| `videos.duration_ms`、`width`、`height`、`video_codec`、`audio_codec`、`playable` | インデックスの列、再生できるかどうか、シークプレビューは入力を保つ。`metadata` のジョブは決してそれらに触れない |
| `video_transcode_probes` | ライブ変換は自分の行を読む。`MediaInfo` はどの `ffmpeg` の引数の入力にもならない |
| `video_locations.file_created_at`、タイトル、表示名 | 要件 10: 埋め込まれたタイトルと `RecordedAt` はそれらの隣に示し、それらに書き込まない |
| `Video.FileTitle` の並べ替えと検索のキー | メディア情報から検索のキーは作らない (範囲外) |
