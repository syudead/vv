# Research: ライブ変換でハードウェアエンコードを使えるようにする

技術スタック、ライブ変換の現行の作り（解析情報の再利用、コピーとエンコードの切り替え、
キーフレームの間隔）、設定画面と API エラーの作りは正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)・
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)・
[docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md)・
[internal/media/transcode.go](../../internal/media/transcode.go)・
[specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)）。
ここにはこの feature が足す決定だけを書く。ffmpeg のエンコーダーの有無と選択肢は、開発コンテナの
ffmpeg 6.1.1（Ubuntu 24.04 のパッケージ。`--enable-libvpl`、NVENC・VAAPI 有効）の `-encoders` と
`-h encoder=…` で確かめた。

## R-1: 同梱イメージの実行環境を Alpine から Debian に変える

- Decision: `Dockerfile` の実行段（3 段目）を `debian:<stable>-slim` にし、`ffmpeg` は Debian の
  パッケージを入れる。`linux/amd64` では Intel の VAAPI ドライバー（`intel-media-va-driver-non-free`、
  `non-free` を有効にする）、Quick Sync の実行時ライブラリ（`libmfx-gen1.2`）、AMD の VAAPI ドライバー
  （`mesa-va-drivers`）を足す。`linux/arm64` では ffmpeg だけを入れる（Intel のパッケージは amd64 に
  しか無い）。NVENC の実行時ライブラリ（`libnvidia-encode.so.1`）はイメージに入れず、ホストの
  NVIDIA Container Toolkit がコンテナへ渡す。SPA とバイナリのビルド段は変えない。
- Rationale: 親 Issue 要件 14 は同梱イメージで VAAPI・Quick Sync・NVENC を使えることを求める。
  NVIDIA Container Toolkit が渡すドライバーのライブラリは glibc に結合されていて、musl の Alpine では
  読み込めない（Jellyfin・Plex の公式イメージが Debian/Ubuntu 系なのはこのため）。Debian の
  `ffmpeg` パッケージは `h264_nvenc`・`h264_qsv`（libvpl）・`h264_vaapi` をすべて含む（Ubuntu 24.04 の
  同系のパッケージで確かめた）ので、ffmpeg 自体を別に用意する必要が無い。
- Alternatives considered: Alpine のまま（NVENC が成り立たない）。`jellyfin-ffmpeg` の deb を
  入れる（ハードウェア対応が最も広く小さいが、第三者の apt リポジトリと鍵を Dockerfile に持ち込み、
  依存の更新の運用（Renovate）の外になる）。ハードウェア対応版を別タグの 2 つ目のイメージにする
  （要件 14 は同梱イメージ 1 つを指しており、利用者に選ばせる理由が無い）。

## R-2: 起動時の確認は、エンコーダーごとに短い実エンコードを並行して走らせる

- Decision: `internal/media` の `EncoderCheck` が、対象のエンコーダーごとに
  `ffmpeg -f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8 <R-7 のエンコード引数> -f null -`
  を実行し、終了コード 0 なら「使える」とする。対象は OS で決める（linux: NVENC・Quick Sync・VAAPI、
  windows: NVENC・Quick Sync、darwin: VideoToolbox。それ以外の組は `unsupported_os` で「使えない」）。
  先に `ffmpeg -encoders` を 1 回読み、名前の無いエンコーダーは実行せずに `encoder_missing` とする。
  実エンコードの失敗は `check_failed`（標準エラーの末尾をログに残す）、上限時間
  （`encoderCheckTimeout`、エンコーダーごとに 10 秒）の超過は `timed_out`。確認は並行して走らせ、
  HTTP の待ち受けを待たせない（[R-3](#r-3-実際に使う方式はドメインの純粋関数が決めapp-がメモリに持つ)
  の「確認中」）。確認は起動時の 1 回だけで、画面から調べ直す操作は置かない（親 Issue 対象外）。
- Rationale: 親 Issue 要件 5 は「実際に短いエンコードを試す」ことを求める。`-encoders` の有無だけでは
  ビルドに含まれていてもデバイスやドライバーが無い場合（Docker に GPU を渡していない、`/dev/dri` の
  権限が無い、NVIDIA のライブラリが無い）を見分けられず、実エンコードだけがそれを一度に確かめる。
  `lavfi` の合成入力はどのビルドにもあり、入力ファイルを要らなくする。並行にするのは、固まった
  エンコーダーが 1 つあっても全体の待ちが 10 秒で済むようにするためである。
- Alternatives considered: `-encoders` の有無だけ（上記）。デバイスファイル（`/dev/dri/renderD128`、
  `/dev/nvidia*`）の存在で判定（ドライバーの不一致やセッション上限を見ない）。順に実行（最悪 30 秒）。
  確認が終わるまで起動を待つ（Edge Case「起動時の確認が遅い、または固まる」に反する）。

## R-3: 実際に使う方式はドメインの純粋関数が決め、app がメモリに持つ

- Decision: `internal/domain` に方式の値（`VideoEncoder`: `software`・`nvenc`・`qsv`・`vaapi`・
  `videotoolbox`、選択肢 `EncoderChoice` はそれに `auto` を足したもの）、各エンコーダーの確認結果
  （`EncoderAvailability`: `checking`／`available`／`unavailable` と理由）、実際に使う方式を決める
  純粋関数 `ResolveVideoEncoder(choice, availability)` を置く。
  - `software` → software。
  - `auto` → 使えるものを `nvenc`・`qsv`・`vaapi`・`videotoolbox` の順で最初の 1 つ。無ければ software
    （fallback ではない。要件 7）。
  - ハードウェアの方式 → 使えればそれ。使えなければ software で、`fallbackReason` を
    `selected_unavailable`（確認済みで使えない）か `checking`（確認中）にする。
  - 保存値が知らない文字列なら `software` として扱う（Edge Case「保存値が未知の値」）。
  `internal/app` の `TranscodeSettings` が保存値と確認結果をメモリに持ち、`Current()` で今の状態
  （選択・実際の方式・理由・確認中か・各エンコーダーの結果）を返し、`Select(choice)` で保存する。
  変換の経路は要求ごとに `Current()` の実際の方式を `LiveTranscodeRequest` に載せるので、変更は
  次の要求から効き、配信中の変換は始めたときの方式で続く（要件 4）。起動時の確認が終わったときと
  `Select` のたびに、app が選択・実際の方式・理由をログに記録する（要件 9）。
- Rationale: 「どれを使うか」は入力（選択と確認結果）だけで決まる規則なので、ARCHITECTURE.md の
  層の分け方に従って `internal/domain` に置き、SQLite も ffmpeg も無しにテストする。確認結果は
  起動ごとに作り直す値なので保存しない。要求ごとに読むのは保存値ではなくメモリの状態で、SQLite の
  読み出しが変換の開始に加わらない。
- Alternatives considered: 実際の方式を SQLite に書く（起動ごとに変わる値で、保存値を変えない
  要件 8 と混ざる）。経路が保存値を毎回読む（変換の開始に SQLite の読み出しが入る）。`auto` の順を
  設定にする（エンコーダーごとの細かい設定は対象外）。

## R-4: 保存先は汎用の `settings` 表（key-value）

- Decision: 表 `settings(key text primary key, value text not null, updated_at integer not null)`
  を足し、キー `transcode.video_encoder` に選択肢の文字列を保存する
  （[data-model.md](data-model.md)）。読み書きは `SettingsStore` のメソッドで、行が無ければ
  「未選択」＝ `software`。値の解釈（未知の値を `software` に倒す）は `domain.ParseEncoderChoice`
  が行い、store は文字列をそのまま返す。
- Rationale: 保存する値は文字列 1 つで、未知の値を保存したまま扱う要件（Edge Case）は型付きの列より
  文字列の方が素直である。今後の設定（同種の「所有者が設定画面で選ぶ値」）を、表とマイグレーションを
  足さずに置ける。ARCHITECTURE.md の区分では `media_folders` と同じ「利用者・設定のデータ」で、
  走査では戻らない。
- Alternatives considered: `transcode_settings` の 1 行の表（項目が増えるたびに列とマイグレーションが
  要る）。`media_folders` のような専用表（値が 1 つで表にする理由が無い）。環境変数（要件 1 で除外）。

## R-5: 変更の知らせは出さず、画面は表示時と保存の応答で合わせる

- Decision: 方式の変更で domain event も `/api/events` の種類も足さない。画面は区画を表示するときに
  `GET` し、`PUT` の応答（保存後の状態全体）で表示を置き換える。「確認中」の間だけ、確認が終わるまで
  数秒ごとに `GET` し直す。
- Rationale: 親 Issue の Edge Case「複数のタブや端末で同時に方式を変える」は「後から保存した方が
  勝ち、次に表示したとき、または変更の結果を受け取ったときに正しく表示する」で足りるとしている。
  所有者だけが使う設定 1 つのために、イベントの種類・購読・画面の購読を増やす方が重い。確認中は
  起動直後の数秒〜十数秒だけなので、その間の読み直しで十分である。
- Alternatives considered: `domain.TranscodeSettingsChanged` を足して SSE で配る（上記）。
  確認の完了も SSE で配る（同上）。

## R-6: 要求の中での切り替えは、エンコードの段でハードウェア → ソフトウェアの順に試す

- Decision: `LiveTranscoder.Start` の切り替えの梯子（live-transcode-seek.md「コピーの経路と差の
  上限」）の「エンコード」の段を 2 段にする。実際の方式がハードウェアなら、まずそのエンコーダーで
  始め、最初のデータを出さずに終わったら（`errNoInitialData`）同じ解析情報で `libx264` で始め直す。
  期限は今までどおり `StartupDeadline` 1 つで、期限切れと取り消しでは切り替えない。切り替えたことは
  `LiveTranscode` に載せて返し（使った方式と、ハードウェアの失敗の誤り）、経路が
  `Warn` でログに記録する（Edge Case「起動後にハードウェアが使えなくなる」「同時セッション数の
  上限」）。コピーで済む要求（`videoCanCopy` かつ `Normalize` でない）はこれまでどおりコピーで、
  方式の設定を見ない（要件 12）。最初のデータを出したあとの失敗は、今までどおり切り替えない。
- Rationale: 親 Issue 要件 11 の「最初のデータを出す前に失敗したら同じ要求の中でソフトウェアに
  切り替える」は、既に「コピー → エンコード → その場の解析」で使っている仕組み（プロセスが
  データを出さずに終わったら次の段へ）そのものである。ハードウェアの初期化の失敗（セッション上限、
  デバイス無し、対応しない入力）は即座に終了コードで返るので、期限を分けなくてもソフトウェアの
  やり直しに時間が残る。期限を足すと、切り替えの合計が今の上限を超える（Edge Case
  「開始時の待ち時間の上限」）。
- Alternatives considered: ハードウェアの試行に別の短い上限（3 秒など）を切る（4K の入力では
  ソフトウェアのデコードだけで最初の 2 秒分に 3 秒以上かかることがあり、成功する試行を切ってしまう。
  固まるエンコーダーはこれまでの梯子でも期限切れで失敗にしている）。ソフトウェアのやり直しに
  新しい期限を与える（合計が最大 12 秒になる）。失敗したエンコーダーを覚えて以後の要求で使わない
  （セッション上限は一時的で、次の要求では使えることが多い。要件 8 の表示も起動時の結果のままで
  よいとしている）。

## R-7: エンコード引数は、方式ごとの符号化器の指定だけを差し替え、出力の約束は共通の引数で守る

- Decision: `videoEncodeArgs` を方式で分岐させる。共通の部分（フィルター: 縮小・pad・setsar・fps、
  `-force_key_frames expr:gte(t,n_forced*2)`）と音声・`-movflags` は変えない。`software` は今の
  引数（`-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23`）を
  1 文字も変えない（受け入れ条件 1）。ハードウェアでは、H.264 High・Level 5.1・4:2:0 8bit・一定品質・
  強制キーフレームを IDR にする指定を、そのエンコーダーの綴りで与える。
  - `nvenc`: `-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23
    -b:v 0 -forced-idr 1`
  - `qsv`: `-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -global_quality 23
    -look_ahead 0 -forced_idr 1`
  - `vaapi`: `-vaapi_device /dev/dri/renderD128`、フィルターの末尾に `format=nv12,hwupload`、
    `-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode CQP -qp 23`
  - `videotoolbox`: `-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -q:v 60
    -realtime 1`（`-q:v` が効かない機種では符号化器の既定の bitrate になる）
  10bit・特殊な画素形式の入力は、`format`／`-pix_fmt` の指定で符号化器に渡す前に 8bit 4:2:0 へ
  落とす（Edge Case「ハードウェアエンコーダーが扱えない入力」）。Quick Sync と VAAPI は `nv12`
  で受けるが、出力の bitstream は 4:2:0 8bit で、ブラウザから見て `yuv420p` と同じである。
  解像度の上限を超える入力は、今と同じ `scale` で先に縮める。数値（品質、preset）はこの表を
  出発点とし、実装の PR が受け入れ条件 7 の確認（[quickstart.md](quickstart.md)）で調整してよい。
  出力の約束（プロファイル・レベル・画素形式・キーフレーム間隔）は変えない。
- Rationale: 要件 10 は出力の約束をソフトウェアと同じにすることを求め、それはフィルターと
  キーフレームの指定（既に符号化器に依らない形で書いてある）で守られる。符号化器ごとの綴り
  （レベルの書き方、`nv12` の入力、VAAPI の hwupload、強制キーフレームを IDR にする option）だけが
  差分である。IDR にするのは、`frag_keyframe` が fragment を切る印にキーフレームを使うためで、
  非 IDR の I フレームでは fragment が切れず最初のデータが遅れる。
- Alternatives considered: ハードウェアデコード（`-hwaccel`）も使う（親 Issue 対象外）。
  `-g 60` だけでキーフレームを入れる（fps フィルターで間引いたあとのフレーム数と時刻がずれる。
  #371 が時刻基準を選んでいる）。VAAPI をソフトウェアフレームのまま渡す（`h264_vaapi` は
  hw フレームしか受けない）。

## R-8: 設定の API は `/api/settings/transcoding` の GET と PUT で、状態全体を返す

- Decision: `GET /api/settings/transcoding` と `PUT /api/settings/transcoding`（本文
  `{ "videoEncoder": <選択肢> }`）を足し、どちらも同じ `TranscodingSettings`（選択、実際の方式、
  fallback の理由、確認中か、エンコーダーごとの結果と理由）を返す
  （[contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)）。所有者だけ
  （`accessRoutes` に足さない）。使えない方式を選ぶ `PUT` は 409 `conflict` に reason
  `encoder_unavailable` を添える。理由は機械可読のコードで、文言は SPA の英語カタログが持つ
  （023 の error-api の方針）。
- Rationale: 画面が要るのは「保存値」「今使われている方式」「各方式が使えるか」の 3 つで、
  `PUT` の応答が状態全体を返せば、保存後に `GET` し直さずに R-5 の表示が揃う。経路を
  `/api/settings/` の下に置くのは、同種の「所有者が設定画面で選ぶ値」を今後同じ場所に並べるため
  （`media-folders` はこの名前空間より前からある）。
- Alternatives considered: `/api/transcoding-settings`（上記）。`PATCH`（項目が 1 つで部分更新の
  意味が無い）。エンコーダーの一覧を別の経路にする（画面は必ず両方を出すので往復が増える）。

## R-9: GPU をコンテナに渡す設定は override ファイルの例として文書に置く

- Decision: `docs/how-to/running-vv.md` に「Hardware encoding」の節を足し、`compose.override.yaml`
  （`task up`）と `compose.hosting.yaml` に足す行の例を、Intel/AMD（`/dev/dri` の `devices` と
  `group_add`）と NVIDIA（NVIDIA Container Toolkit、`deploy.resources.reservations.devices` と
  `NVIDIA_DRIVER_CAPABILITIES=video,utility`）の 2 つについて書く。方式ごとの前提（ドライバー、
  デバイス、OS、VideoToolbox は Docker では使えないこと）も同じ節に置く。`compose.yaml` 自体には
  デバイスの行を入れず、節へのコメントだけを足す。設定画面の説明文はこの節を指す。
- Rationale: `devices: /dev/dri` はホストにその device が無いと起動に失敗するので、既定の
  `compose.yaml` には入れられない。Compose は `compose.override.yaml` を自動で重ねるので、利用者は
  `compose.yaml` を書き換えずに足せる。
- Alternatives considered: `compose.yaml` にコメントアウトで置く（NAS の管理画面に貼る
  `compose.hosting.yaml` と二重になる）。`compose.gpu.yaml` を同梱して `task up-gpu` を足す
  （Intel と NVIDIA で内容が違い、2 つ同梱しても組み合わせは利用者が選ぶ）。

## R-10: 検査は ffmpeg を差し替えたテストで行い、実機の確認は quickstart に置く

- Decision: CI にハードウェアエンコーダーは無いので、切り替え・確認・保存・API・画面は
  `commandContext` を差し替えた helper process（`internal/media/transcode_test.go` の既存の形）と
  fake の checker で検査する。受け入れ条件 2・3・5・6・7・11 の実機の確認は
  [quickstart.md](quickstart.md) の手順で GPU のあるホストで行い、結果を実装 PR の本文に残す。
  ソフトウェアの引数が変わらないことと、共通の出力の約束（回転・縦横比・4K の縮小・キーフレーム
  間隔）は、今の ffmpeg 付きのテストをそのまま通すことで確かめる。
- Rationale: 実機でしか確かめられないことを CI で偽って通したように見せない。既存の検査の形を
  そのまま使う。
- Alternatives considered: GPU 付きの self-hosted runner（この feature の範囲で用意できない）。
  実機の確認を省く（要件 10 の約束が符号化器の option の綴りに依るので、実機で ffprobe する必要がある）。
