---
source: docs/design-docs/media-information.md
sourceHash: 05c1a8e00900973c8b54efd71b63d2e81d329e0601d867567cb202654908ceeb
---

# 取り込み時のプローブから得るメディア情報 {#media-information-from-the-import-probe}

取り込み時の 1 回の `ffprobe` 実行は、映像ストリームの詳細、すべての音声トラックと字幕トラック、埋め込みタグ、チャプターも読み、動画ごとにバージョン付きの JSON 1 行として `video_media_info` に保存する ([`internal/media/probe_media.go`](../../internal/media/probe_media.go)、[`internal/domain/media_info.go`](../../internal/domain/media_info.go)、[`internal/store/media_info.go`](../../internal/store/media_info.go))。背景: [research.md](../../specs/056-media-info/research.md)。

図は、値の各部分をどこで読み、どこで判断し、どこに保存するかを示す。

```mermaid
flowchart LR
  ffprobe[ffprobe を 1 回: format、streams、chapters] --> parser[internal/media のパーサー]
  parser -->|生の値| rules[internal/domain の規則]
  rules --> info[domain.Probe の domain.MediaInfo]
  info --> apply[ApplyProbeForJob]
  info --> again[ApplyMediaInfoForJob]
  apply -->|videos と同じトランザクションと同一性の確認| row[(video_media_info)]
  again -->|同じ同一性の確認、videos は変更しない| row
  row --> read[LibraryStore.MediaInfo]
  read --> detail["GET /api/videos/{id} の mediaInfo"]
```

値を運ぶのは動画の詳細だけで、所有者にも公開動画のゲストにも同じく運ぶ。一覧の項目、関連動画、所有者の `PUT` の応答は値を省く ([`internal/httpapi/video_media_info.go`](../../internal/httpapi/video_media_info.go))。動画ページは値を技術情報の行の下にグループとして描く ([`MediaInfoGroups.tsx`](../../web/src/player/MediaInfoGroups.tsx))。

## 1 回の実行ですべてを読み、TS ファイルは長く読む {#one-run-reads-everything-and-a-ts-file-is-read-longer}

`media.Probe` は取り込みの実行に `-show_chapters` を加え、`.ts`、`.m2ts`、`.mts` のファイルには `-analyzeduration 30M -probesize 100M` も加える。

プローブのコストの大半はプロセスの起動なので、すべての項目を、インデックスの値をすでに読んでいる実行から得る。`ffprobe` は既定では TS の解析を 5 秒か 5 MB で止め、放送の録画で後から始まる音声や字幕のストリームを見落とすが、MP4 や MKV はヘッダーにストリームを列挙するので、長い上限はそれを遅くするだけだ。ライブ変換のリクエスト時のプローブは、変換がその上限を待たないように既定の上限のままにする。

| 採用しなかった案 | 理由 |
| --- | --- |
| 項目ごとに 2 回目の `ffprobe` | 動画 10,000 本のライブラリでは、さらに 10,000 個のプロセスを起動する |
| すべてのコンテナに長い上限 | ヘッダーのあるコンテナでは何も変わらない |

## TS 録画の放送局と番組は SI から得る {#a-ts-recordings-station-and-programme-come-from-its-si}

`ffprobe` の後、`media.Probe` は `.ts`、`.m2ts`、`.mts` のファイルの先頭 32 MiB を Go で独自の 10 秒の期限のもとで読み ([`internal/media/ts_si.go`](../../internal/media/ts_si.go))、SI テーブルから `Broadcast` を埋める。`ffprobe` はこれらの名前を提供できない。復号するのは ARIB STD-B24 ではなく DVB の文字テーブルであり、EIT も解析しない。

| 項目 | 取得元 |
| --- | --- |
| パケット | 188 バイトのパケット、4 バイトの前置部を持つ 192 バイトのパケット (`.m2ts`)、204 バイトのパケット。繰り返し現れる同期バイトで見つける |
| 放送局 | SDT にある、PAT の最初のプログラムのサービス名 |
| 番組 | TOT または TDT の時刻からその時刻に再生時間を足した時刻までの録画と重なる、EIT の現在または次のイベントのうち最長のもの。同じなら次のイベントを選ぶ ([`domain.SelectBroadcastProgram`](../../internal/domain/media_broadcast.go)) |
| 文字 | [`internal/media/arib`](../../internal/media/arib/arib.go) の ARIB STD-B24 8 ビット復号器: `golang.org/x/text` による JIS X 0208、かなと JIS X 0201 の文字集合、Unicode 5.2 の符号位置の表による 85 区から 94 区の追加記号。大きさ、色、位置の制御は無視し、対応のない文字は捨てる |

放送局、両方のイベント、時刻が見つかった時点で読み取りを止める。読み取りエラーまたは期限切れのときは名前を省き、プローブを失敗させることはない。名前を復号するのはオリジナルネットワーク ID が日本のネットワークで、イベントに `jpn` のタグがあるときだけなので、DVB の録画や、`ffmpeg` が既定の SDT を付け EIT なしで書いた TS には `Broadcast` がない。

## パーサーが読み、ドメインが判断する {#the-parser-reads-the-domain-decides}

`internal/media` のパーサーは `ffprobe` の値をグループ (映像、トラック、埋め込みタグ、チャプター) ごとにコピーし、`internal/domain` の純粋関数がそれを分類し整える。

| 項目 | 規則 |
| --- | --- |
| HDR の種類 | Dolby Vision (`DOVI configuration record`、または `dvh1`、`dvhe`、`dvav`、`dva1`、`dav1` のコーデックタグ)、なければ HDR10 (`smpte2084`)、なければ HLG (`arib-std-b67`)、なければ SDR |
| インターレース | `tt`、`bb`、`tb`、`bt` はインターレース、`progressive` はインターレースではない。`unknown` は省く |
| ビット深度 | `bits_per_raw_sample`、なければ `pix_fmt` の深度 (`yuv420p10le` は 10) |
| 文字列 | 不正な UTF-8 と改行以外の制御文字を除き、前後の空白を除き、2,000 文字で切る。空なら省く |
| 件数 | 音声トラックは最大 64、字幕トラックは最大 64、チャプターは最大 1,000 |
| 記録日時 | オフセットの有無を問わない RFC 3339 (Apple の `+0900` を含む)、または `YYYY-MM-DD HH:MM:SS`。オフセットは保持し、オフセットがなければ UTC とする |
| 位置 | 度、度と分、または度と分と秒で表した ISO 6709 で、高度は任意。±90 または ±180 の範囲外は省く |
| チャプター | 開始位置の順に並べる。開始が再生時間以上なら捨てる。終了は再生時間で切り、開始より前にならないようにする。重なりは保持する |

解析できない値は省き、プローブを失敗させることはなく、プローブを失敗させるのは再生時間がないときだけだ。パーサーが期待する JSON の型と異なる型のタグの値は、出力全体を失敗させずに読み飛ばす。規則をパーサーの外に置くので、各ケースをプロセスなしの単体テストにできる。

## 動画ごとにバージョン付きの 1 行 {#one-versioned-row-per-video}

プローブのジョブは `ApplyProbeForJob` のトランザクションの中で、`videos` の更新を守る同一性の確認のもとで `video_media_info` を書き、`version` が `domain.MediaInfoVersion` と異なる行は存在しないものとして読む。

そのため、プローブ中に置き換えられたファイルは古いファイルの行を残さず、動画を削除するとその行も削除される。この値を読むものはほかにないので、JSON 1 行は `video_id` による検索 1 回で済み、形が変わったときはマイグレーションを加えずにバージョンを上げる。このテーブルは再構築できるインデックスだ ([データと復旧](../how-to/running-vv.md#data-and-recovery))。

| 採用しなかった案 | 理由 |
| --- | --- |
| `videos` の列 | すべての一覧クエリが読む、NULL を許す約 30 列 |
| 項目ごとのテーブル (ストリーム、チャプター、タグ) | それらを結合やフィルタに使うクエリがない |

## 以前に取り込んだ動画は `metadata` ジョブが読み直す {#videos-imported-earlier-are-read-again-by-a-metadata-job}

`metadata` ジョブは、行ができる前にプローブした動画に同じ `media.Probe` を実行し、`ApplyMediaInfoForJob` で行だけを書く ([`internal/app/ingest.go`](../../internal/app/ingest.go)、`Metadata`)。マイグレーション `00052_metadata_jobs.sql` は、プローブが完了し、登録済みフォルダの下に場所を持つ動画ごとに 1 件を積み、スキャンは行がないか行が `domain.MediaInfoVersion` を下回る動画に 1 件を積む (`IndexedVideo.MediaInfoMissing`)。新しい取り込みには積まない。プローブのジョブが行を書くからだ。

| 規則 | 理由 |
| --- | --- |
| 登録済みの場所を持つ動画の `probe` ジョブが待機中でも実行中でもない間だけ取得する (`JobClaimCondition.NoClaimableProbe`) | 新しい取り込みの段階が先に進み、待機中または実行中のプローブと並んで `metadata` の実行が始まることはない。`seek_thumbnail` と同じく、条件は取得の時点で成り立てばよいので、実行中に積まれたプローブと重なるのはその 1 回のヘッダーの読み取りだけだ。プローブのジョブが `running` を離れたときと、プローブの動画が登録済みの場所を失ったときに `metadata` のワーカーを起こす |
| プローブのジョブと同じ同一性の確認 (ジョブの場所 ID、バージョンとパス、内容の鍵、場所の世代)。確認に失敗すると何も書かずにジョブを完了する | 実行中に置き換えられたファイルが古いファイルの行を得ることはない。新しいファイルを見つけたスキャンがもう一度積む |
| インデックスの列、再生可否、ライブ変換用のプローブは書かない | 失敗や変更されたパーサーが再生やシークのプレビューを隠すことはできない |
| 試行回数の上限での失敗は行を残し、最新のスキャンの集合に含まれる動画にだけ `metadata_failed` を記録する | その集合に含まれる動画は最新の取り込みが数える作業なので、その失敗はその取り込みの問題だ。集合の外の動画は何も記録しないので、最新の取り込みが一度も触れなかった動画の補完が、その取り込みを部分的な結果にすることはない。次のスキャンがジョブをもう一度積む |

完了したジョブは `video` 通知を送るので、開いている動画ページは再取得して新しい情報を表示する。

| 採用しなかった案 | 理由 |
| --- | --- |
| `probe` ジョブを積み直す | 上限でのプローブの失敗は動画を `failed` にし、再生を隠す |
| 起動時の補完 goroutine | 何を試したかの記録が残らず、再起動するとやり直しになる |
