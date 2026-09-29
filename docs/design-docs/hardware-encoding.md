# ライブ変換のハードウェアエンコード

- ステータス: 採用
- スコープ: ライブ変換（`GET /api/videos/{id}/transcode.mp4`）の映像エンコード方式の選択・起動時の確認・
  要求の中の切り替え・方式ごとの引数と、設定の API（`GET`/`PUT /api/settings/transcoding`）
- 経緯: [specs/025-hardware-encoding/](../../specs/025-hardware-encoding/plan.md)（親 Issue #370）

ライブ変換の開始（解析情報の用意、コピーとエンコードの切り替え）は
[live-transcode-seek.md](live-transcode-seek.md) に、MOV の二入力は
[mov-live-transcoding.md](mov-live-transcoding.md) に書いてある。この文書は、映像をエンコードする段で
どの符号化器を使うかだけを扱う。

## 方式と、実際に使う方式の決定

### Context

ライブ変換の映像は `libx264` でエンコードしていた。サーバーに GPU があれば、NVENC・Quick Sync・
VAAPI・VideoToolbox で CPU の負荷を下げられる。ただし、ffmpeg のビルドに符号化器があっても、
デバイスやドライバーが無ければ使えない（同梱の Docker イメージの中、`/dev/dri` の権限が無い、
NVIDIA のライブラリが無い）。

### Decision

所有者が選ぶ値（`domain.EncoderChoice`）は `software`・`nvenc`・`qsv`・`vaapi`・`videotoolbox`・
`auto` で、SQLite の `settings` 表のキー `transcode.video_encoder` に文字列で保存する
（`SettingsStore`）。行が無いときと知らない文字列は `software` として扱い、保存値は書き換えない
（`domain.ParseEncoderChoice`）。

実際に使う方式（`domain.VideoEncoder`）は、選択と起動時の確認結果から純粋関数
`domain.ResolveVideoEncoder` が決める。

- `software` は software。
- `auto` は使える方式を `nvenc`・`qsv`・`vaapi`・`videotoolbox` の順で最初の 1 つ。無ければ software で、
  これは fallback ではないので理由を付けない。
- ハードウェアの方式は、使えればそれ。確認中なら software で理由 `checking`、確認して使えなければ
  software で理由 `selected_unavailable`。

選択と確認結果は `internal/app` の `TranscodeSettings` がメモリに持つ。確認結果は起動ごとに作り直す
値なので保存しない。`Current()` が今の状態（選択・実際の方式・理由・確認中か・方式ごとの結果）を返し、
`Select()` が保存してメモリを更新する。使える（確認済みで `available`）ハードウェアの方式でなければ
`Select()` は `domain.ErrEncoderUnavailable` を返し、保存値を変えない。`software` と `auto` は常に
受け付ける。確認が終わったときと `Select()` のたびに、選択・実際の方式・理由を `Info` で記録する。

ライブ変換の経路（`internal/httpapi/transcode.go`）は、要求ごとに `Current()` の実際の方式を
`domain.LiveTranscodeRequest.VideoEncoder` に載せる。方式の変更は次に始まる要求から効き、配信中の
変換は始めたときの方式で続く。再起動は要らない。映像をコピーできる要求は方式に依らずコピーする。

### Trade-offs

- 要求ごとに読むのはメモリの状態で、変換の開始に SQLite の読み出しは加わらない。
- 方式の変更は画面へ知らせない（domain event も `/api/events` の種類も無い）。別のタブや端末は、
  次に区画を表示したときか、自分の保存の応答で正しい状態になる。

## 起動時の確認

### Decision

`cmd/mdm` は保存値を読んで `TranscodeSettings` を作り、方式の行（選択・実際の方式・確認中か）を
記録してから、確認を背後の goroutine で始める。HTTP の待ち受けは確認を待たない。確認が終わるまで、
ハードウェアの方式を選んでいても実際の方式は software で、設定の API は `checking: true` を返す。
停止の指示で確認を取り消し、終わりを待ってから走査とワーカーを止める。

確認の対象は OS で決まる（`domain.HardwareEncoderCandidates`）。linux は NVENC・Quick Sync・VAAPI、
windows は NVENC・Quick Sync、darwin は VideoToolbox で、それ以外の組は `unsupported_os` である。

`internal/media` の `EncoderCheck` が 1 つの方式を確かめる。

1. `ffmpeg -encoders` を 1 回だけ読み、並行する確認で共有する。名前の無い方式は実行せずに
   `encoder_missing`。読み取りが上限時間を超えたら `timed_out`。
2. `-f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8` を、ライブ変換と同じエンコード引数で
   `-f null -` へ符号化する。終了コード 0 なら `available`、失敗は `check_failed`、上限時間の超過は
   `timed_out`。

方式ごとの確認は並行に走り、`TranscodeSettings` がそれぞれに上限時間（10 秒）を掛ける。確認が
戻らなくても上限時間で `timed_out` にし、「確認中」のまま残さない。失敗した確認の標準エラーの末尾は
`Warn` で記録し、API には出さない。確認は起動時の 1 回だけである。

### Trade-offs

- `-encoders` の有無だけではデバイスやドライバーの有無が分からないので、実際に短く符号化する。
  合成入力（`lavfi`）はどのビルドにもあり、入力ファイルが要らない。
- 並行にするので、固まる方式が 1 つあっても待ちは最悪で `-encoders` の読み取りと合わせて 20 秒程度で
  済む。その間の要求は software で変換する。

## 要求の中の切り替え

### Decision

`LiveTranscoder.Start` の切り替えの梯子
（[live-transcode-seek.md](live-transcode-seek.md#コピーの経路と差の上限)）のうち、エンコードの段を
「要求の方式 → software」の 2 段にする。ハードウェアの FFmpeg が最初のデータを出さずに終わったら、
同じ解析情報で `libx264` で始め直す。期限は今までどおり `StartupDeadline` 1 つで、期限切れと
取り消しでは切り替えない。最初のデータを出したあとの失敗も切り替えない。

切り替えたことは `LiveTranscode.HardwareFailure`（FFmpeg の標準エラーの末尾を含む誤り）で返り、
経路が `Warn`（動画、方式、誤り）で記録する。応答は software の出力で 200 になり、形もヘッダーも
変わらない。失敗した方式は覚えず、次の要求もまた設定の方式から試す。

### Trade-offs

- ハードウェアの初期化の失敗（セッション数の上限、デバイスが無い、扱えない入力）はすぐに終了コードで
  返るので、期限を分けなくても software のやり直しに時間が残る。
- セッション数の上限は一時的なことが多いので、失敗を設定の状態（画面の表示）に反映しない。

## 方式ごとの引数

### Decision

`videoEncodeArgs` の共通の部分（縮小・pad・setsar・fps のフィルター、
`-force_key_frames expr:gte(t,n_forced*2)`）と音声・`-movflags` は方式に依らない。符号化器の指定
（`encoderCodecArgs`）だけを方式で差し替え、どれも H.264 High・Level 5.1・4:2:0 8bit・一定品質で、
強制キーフレームを IDR にする。

| 方式 | 符号化器の指定 |
| --- | --- |
| `software` | `-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23` |
| `nvenc` | `-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23 -b:v 0 -forced-idr 1` |
| `qsv` | `-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -global_quality 23 -look_ahead 0 -forced_idr 1` |
| `vaapi` | `-vaapi_device /dev/dri/renderD128`、フィルターの末尾に `format=nv12,hwupload`、`-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode CQP -qp 23` |
| `videotoolbox` | `-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -q:v 60 -realtime 1` |

画質（`domain.LiveTranscodeRequest.Quality`）のある変換では、表の指定に画質ごとの上限
（映像の上限 `<上限>`、`-bufsize` はその 2 倍）を足す。値と縮める寸法は
[playback-quality.md](playback-quality.md) にある。

| 方式 | 画質のあるときの指定 |
| --- | --- |
| `software`・`nvenc` | 一定品質（`-crf 23`／`-cq 23`）のまま、末尾に `-maxrate <上限>k -bufsize <上限×2>k` |
| `qsv` | `-global_quality 23` を `-b:v <上限>k -maxrate <上限>k -bufsize <上限×2>k` に替える |
| `vaapi` | `-rc_mode CQP -qp 23` を `-rc_mode VBR -b:v <上限>k -maxrate <上限>k -bufsize <上限×2>k` に替える |
| `videotoolbox` | `-q:v 60` を `-b:v <上限>k -maxrate <上限>k -bufsize <上限×2>k` に替える |

software と NVENC は一定品質に上限を重ねられるので、静かな場面で上限より軽くなる。QSV・VAAPI・
VideoToolbox で一定品質に上限を重ねる動作はドライバーの対応に依るので、確実に上限が効く VBR に
する。ハードウェアが最初のデータを出さずに software に切り替えたときも、同じ画質の引数で始め直す。

IDR にするのは、`frag_keyframe` が fragment を切る印にキーフレームを使うためである。非 IDR の I
フレームでは fragment が切れず、最初のデータが遅れる。デコードはどの方式でもソフトウェアで行う。

### Alternatives

- **ハードウェアデコード（`-hwaccel`）も使う**: 入力の形式ごとの対応の差が大きく、この feature の
  対象外にした。
- **`-g 60` でキーフレームを入れる**: fps フィルターで間引いたあとのフレーム数と時刻がずれる。
  時刻基準の `-force_key_frames` を方式に依らず使う。

## 設定の API

`GET /api/settings/transcoding` と `PUT /api/settings/transcoding`（本文 `{"videoEncoder": …}`）は
どちらも同じ `TranscodingSettings`（選択、実際の方式、`fallbackReason`、`checking`、
`nvenc`・`qsv`・`vaapi`・`videotoolbox` の順の 4 件の確認結果と理由）を `Cache-Control: no-store` で
返す。所有者だけの経路である。`internal/httpapi/transcoding_settings.go` は要求の解釈と契約の形への
変換だけを行い、方式の決定と拒否は `TranscodeSettings` に任せる。

- 列挙に無い値・本文が JSON でない: 400 `invalid_request`。
- 使えないハードウェアの方式（確認中を含む）: 409 `conflict`、reason `encoder_unavailable`。
  保存値は変えない。
- 保存の失敗: 500 `internal`。

契約の正本は [api/openapi.yaml](../../api/openapi.yaml)
（[specs/025-hardware-encoding/contracts/transcoding-settings-api.md](../../specs/025-hardware-encoding/contracts/transcoding-settings-api.md)）。
