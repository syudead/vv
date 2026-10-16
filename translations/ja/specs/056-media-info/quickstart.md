---
source: specs/056-media-info/quickstart.md
sourceHash: f71b6207444c784653963c62cb9dc8b76c230d1a8ed16c157fa9c6eb76b06c11
---

# クイックスタート: 実際のファイルでのメディア情報 {#quickstart-media-information-on-real-files}

この手順は、フィクスチャに基づくテストでは示せないことを示す: 実際の HDR、スマートフォン、放送、チャプター付きのファイルが、親 Issue の受け入れ条件が挙げる項目を生むこと、そしてこの機能より前に取り込んだライブラリがひとりでに埋まること。

## 前提条件 {#prerequisites}

- このブランチから動かした VVMDM ([running-vv.md](../../docs/how-to/running-vv.md))。
- 次を持つメディアフォルダ:
  - `hdr10.mp4`、`hlg.mp4`、`dv.mp4`: 短い HDR10、HLG、Dolby Vision のエンコード (カメラのサンプルのクリップか、公開のテストセット)。そして `sdr.mp4`、任意の SDR のファイル。
  - `dual.mkv`: `jpn` と `eng` のタグを付けた 2 つの音声トラックと、`jpn` のタグを付けた埋め込みの SRT の字幕トラックを持つファイル。
  - `phone.mov`: 位置情報サービスをオンにしてスマートフォンで撮ったクリップ。
  - `rec.ts`: 番組より前に始まり、2 つ目の音声か字幕のストリームが開始の後に現れる、日本の地上波か BS の録画。
  - `chapters.mkv`: 名前の付いたチャプターを少なくとも 3 つ持ち、そのうち 1 つがファイルの長さを越えて終わるファイル (`meta.txt` で長さを越える `END` を指定した `ffmpeg -i in.mp4 -i meta.txt -map_metadata 1 -map_chapters 1 -c copy chapters.mkv`)。
- 最後の 2 つの行のために: このブランチの前のリリースで取り込んだ、同じファイルを持つデータベース。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| スキャンし、取り込みが終わるのを待ち、それぞれの HDR のファイルの動画ページを開く | Video のグループがそれぞれ HDR10、HLG、Dolby Vision を示す。`sdr.mp4` は SDR を示す | 1 |
| `dual.mkv` を開く | Audio のグループが両方のトラックを Japanese と English で並べる。Subtitles のグループが日本語のトラックを並べる。`sdr.mp4` には Subtitles のグループがない | 2、3 |
| `phone.mov` を開く | Embedded のグループが撮影の日時、機器、場所を示す。Created と Added の事実とタイトルは、この機能の前と同じである | 4 |
| `rec.ts` を開く | Broadcast のグループが放送局名と番組名を示す。Audio と Subtitles のグループが、開始の後に現れるストリームを並べる | 5 (要件)、7 |
| `chapters.mkv` を再生する | シークバーが、長さの中のチャプターごとに印を 1 つ示す。印にホバーするとその名前が出る。チャプターのメニューでチャプターを選ぶと、その開始から再生する。最後のチャプターはファイルの終わりで終わる。`sdr.mp4` は印もチャプターのメニューも示さない | 5 |
| 古いデータベースでこのブランチを起動し、動画ページを開く | 1 分以内に、スキャンなしでグループが現れる。ジョブが終わる前に開いた動画はひとりでに更新される | 6 |
| 再解析が動いている間に、1 つのファイルを同じ名前の別のファイルに置き換え、スキャンする | 置き換えたファイルのページは新しいファイルの情報を示し、古いファイルの情報は決して示さない | Edge Case: 再解析の間に置き換えられたファイル |
| 再解析がそのファイルに届く前に、1 つのファイルを読めなくする (`chmod 000`) | そのページは長さと解像度を保ち、新しいグループを示さない。サーバーのログが `metadata` の失敗を示す | Edge Case: 再解析の間の失敗 |

それぞれの HDR のファイルの `ffprobe` の `color_transfer` と、TS の録画の復号した名前を、これらの行を実行する単位の PR に記録する。`rec.ts` の名前が誤った文字か欠けた文字で復号されたときは、その単位がマージされる前に、ARIB の記号の表は [R-4](research.md#r-4-station-and-programme-names-of-a-ts-recording-come-from-a-go-si-reader-with-an-arib-std-b24-decoder) に戻る。
