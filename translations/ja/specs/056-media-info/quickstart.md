---
source: specs/056-media-info/quickstart.md
sourceHash: 3425feaa0108716c331280cab58e0acdfec236205e6a9de0d8cd8d90f0f91cb6
---

# クイックスタート: 実際のファイルでのメディア情報 {#quickstart-media-information-on-real-files}

この手順は、フィクスチャに基づくテストでは示せないことを示す: 実際の HDR、スマートフォン、放送、チャプター付きのファイルが、親 Issue の受け入れ条件が挙げる項目を生むこと、そしてこの機能より前に取り込んだライブラリがそのまま残ること (既存の動画は再解析しない、R-5)。

## 前提条件 {#prerequisites}

- このブランチから動かした VVMDM ([running-vv.md](../../docs/how-to/running-vv.md))。
- 次を持つメディアフォルダ:
  - `hdr10.mp4`、`hlg.mp4`、`dv.mp4`: 短い HDR10、HLG、Dolby Vision のエンコード (カメラのサンプルのクリップか、公開のテストセット)。そして `sdr.mp4`、任意の SDR のファイル。
  - `dual.mkv`: `jpn` と `eng` のタグを付けた 2 つの音声トラックと、`jpn` のタグを付けた埋め込みの SRT の字幕トラックを持つファイル。
  - `phone.mov`: 位置情報サービスをオンにしてスマートフォンで撮ったクリップ。
  - `rec.ts`: 番組の数分前に始まり、2 つ目の音声か字幕のストリームが開始の後に現れる、日本の地上波か BS の録画。
  - `chapters.mkv`: 名前の付いたチャプターを少なくとも 3 つ持ち、そのうち 1 つがファイルの長さを越えて終わるファイル (`meta.txt` で長さを越える `END` を指定した `ffmpeg -i in.mp4 -i meta.txt -map_metadata 1 -map_chapters 1 -c copy chapters.mkv`)。
- 最後の行のために: このブランチの前のリリースで取り込んだ、同じファイルを持つデータベース。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| スキャンし、取り込みが終わるのを待ち、それぞれの HDR のファイルの動画ページを開く | Video のグループがそれぞれ HDR10、HLG、Dolby Vision を示す。`sdr.mp4` は SDR を示す | 1 |
| `dual.mkv` を開く | Audio のグループが両方のトラックを Japanese と English で並べる。Subtitles のグループが日本語のトラックを並べる。`sdr.mp4` には Subtitles のグループがない | 2、3 |
| `phone.mov` を開く | Embedded のグループが撮影の日時、機器、場所を示す。Created と Added の事実とタイトルは、この機能の前と同じである | 4 |
| `rec.ts` を開く | Broadcast のグループが放送局名と、その前の番組の名前ではなく録画した番組の名前を示す。Audio と Subtitles のグループが、開始の後に現れるストリームを並べる | 5 (要件)、7 |
| `chapters.mkv` を再生する | シークバーが、長さの中のチャプターごとに印を 1 つ示す。印にホバーするとその名前が出る。チャプターのメニューでチャプターを選ぶと、その開始から再生する。最後のチャプターはファイルの終わりで終わる。`sdr.mp4` は印もチャプターのメニューも示さない | 5 |
| 古いデータベースでこのブランチを起動し、このブランチより前に取り込んだ動画を開く | 新しいグループはなく、待ち中の `metadata` のジョブもない。ファイルが変わってスキャンした後に、グループが現れる ([R-5](research.md#r-5-media-information-is-read-only-by-the-import-probe-of-new-or-changed-files)) | 6 (再解析はメンテナーが取り除いた) |

それぞれの HDR のファイルの `ffprobe` の `color_transfer` と、TS の録画の復号した名前、その TOT の時刻、それに対して `Probe` にかかった時間を、これらの行を実行する単位の PR に記録する。TOT が先頭の 32 MiB の中にないか、`rec.ts` のプローブが 30 秒の `probeTimeout` に近づいたときは、その単位がマージされる前に、上限は [R-2](research.md#r-2-the-same-ffprobe-run-reads-chapters-and-reads-longer-for-mpeg-ts) と R-4 に戻る。`rec.ts` の名前が誤った文字か欠けた文字で復号されたときは、その単位がマージされる前に、ARIB の記号の表は [R-4](research.md#r-4-station-and-programme-names-of-a-ts-recording-come-from-a-go-si-reader-with-an-arib-std-b24-decoder) に戻る。
