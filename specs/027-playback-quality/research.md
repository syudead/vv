# Research: 低速ネットワーク向けの画質選択と、再生が途切れるときの警告

技術スタック、ライブ変換の現行の作り（解析情報の再利用、コピーとエンコードの切り替え、
キーフレームの間隔、方式ごとの符号化器の引数）、再生の誤りの分け方と読み込み直し、画面の
文言の置き場は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)・
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)・
[docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md)・
[docs/design-docs/library-ui.md「再生画面の構成」](../../docs/design-docs/library-ui.md#8-再生画面の構成)・
[docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)）。ここにはこの feature が足す決定だけを
書く。ffmpeg の引数は開発コンテナの ffmpeg 6.1.1 の `-h encoder=…` で確かめた。

## R-1: 画質はライブ変換の要求の `quality` パラメータで指定し、変換の経路は増やさない

- Decision: `GET /api/videos/{id}/transcode.mp4` に任意の `quality`（`1080p`・`720p`・`480p`・`360p`）
  を足す（[contracts/transcode-quality-api.md](contracts/transcode-quality-api.md)）。無ければ今までどおり
  （元の画質）。`quality` のある要求は、映像をコピーできる動画でも必ずエンコードし、`startMs` の位置
  そのものから始まる。`transcode-start` の報告の経路と `attempt` は変えない（エンコードなので報告は
  指定位置になる）。サーバーは画質を覚えず、要求ごとに決める。
- Rationale: 親 Issue 要件 3 は「選んだ画質のときだけ、今の変換を置き換える」ことを求める。
  同じ経路のパラメータにすると、シークの作り直し（`liveOffset.ts` の `reloadAt`）と読み込み直し
  （`playbackRecovery`）が今の URL の組み立てをそのまま使え、台帳・ゲストの境界・期限の作りが
  増えない。要求ごとに決めるので、同じ動画を別のタブ・別の見る人が別の画質で見ても互いに影響
  しない（Edge Case 7）。
- Alternatives considered: 画質ごとの経路（`/transcode-480p.mp4` など。経路が 4 本増え、`accessRoutes`
  と OpenAPI の `security` の対を増やす）。サーバー側にセッションごとの画質の設定を持つ（見る人の
  選択がブラウザに閉じるという要件 5 に反し、ゲストに設定の口が要る）。

## R-2: 画質は表示の短辺で縮め、ビットレートは `-maxrate`／`-bufsize` で上限を付ける

- Decision: `internal/domain` に `TranscodeQuality`（`1080p`・`720p`・`480p`・`360p`）と、画質ごとの
  値（短辺・映像の上限 kbps・音声の kbps）を持つ純粋関数を置く。値は親 Issue 要件 3 の目安どおり
  映像 5000／2500／1200／700 kbps、音声は 128／128／96／64 kbps とする。`internal/media` の
  `videoEncodeArgs` は、画質があるとき表示の寸法（`displayGeometry`。回転を反映済み）の短辺が
  その画質になる偶数の寸法を計算して `scale=W:H` を出し、既存の `setsar` の扱いをそのまま通す。
  縦長の動画は短辺が幅なので、1080×1920 の 720p は 720×1280 になる（Edge Case 3）。
  符号化器の引数（`encoderCodecArgs`）には方式ごとに上限を足す。
  - `software`・`nvenc`: 今の一定品質（`-crf 23`／`-cq 23`）のまま `-maxrate <上限>k -bufsize <上限×2>k`
    を足す。品質に余裕のある場面は上限より軽くなる。
  - `qsv`・`vaapi`・`videotoolbox`: 一定品質の指定（`-global_quality`／`-rc_mode CQP -qp`／`-q:v`）を
    やめ、`-b:v <上限>k -maxrate <上限>k -bufsize <上限×2>k` の VBR にする（VAAPI は `-rc_mode VBR`）。
  - 音声は画質があれば常に AAC（`-ac 2 -ar 48000`）で画質ごとの kbps にエンコードし、コピーしない。
  - フィルター・`-force_key_frames`・`-movflags` の共通部分は変えない。
- Rationale: 短辺で決めるので、横長でも縦長でも「同じ重さ」の画質になる（要件 1・Edge Case 3）。
  `-maxrate`／`-bufsize` は VBV の上限で、どの方式でも出力の平均ビットレートを上限の付近に抑える
  （受け入れ条件 3）。software と NVENC は一定品質に上限を重ねられるので、静かな場面で軽くなる
  利点を残す。QSV・VAAPI・VideoToolbox の「一定品質に上限を重ねる」動作（QVBR など）は
  ドライバーの対応に依るので、確実に上限が効く VBR にする。ハードウェアが使えず software に
  切り替わっても、縮小は共通のフィルターで、上限は software の引数で効く（Edge Case 6）。
  音声を軽くするのは要件 3 の「音声も合わせて軽くする」で、コピーだと元の 256〜320 kbps が残る。
- Alternatives considered: 長辺で縮める（縦長の動画で 720p が 405×720 になり、横長の 720p より
  軽くなる）。`-b:v` だけの平均ビットレート（瞬間の上限が無く、動きの多い場面で回線を超える）。
  ffmpeg の `scale=-2:480` に寸法の計算を任せる（縦長で向きの判定を ffmpeg 側にも持つことになり、
  Go の計算とテストが二重になる）。

## R-3: 画質が使えるかは動画の短辺で決め、サーバーは使えない画質を 400 で拒む

- Decision: 選択肢は「元の画質」と、`1080p`・`720p`・`480p`・`360p` のうち短辺が動画の表示の短辺
  （`Video.width`／`height`。回転を反映済み）より小さいもの（要件 1）。プレイヤーがこの規則で
  選択肢を作り、覚えている画質が選択肢に無ければ「元の画質」で再生し、覚えている値は
  書き換えない（Edge Case 2）。サーバーは同じ規則で確かめ、動画の短辺以上の画質と、寸法の無い
  動画への画質は 400 `invalid_request` にする。
- Rationale: 拡大や同じ寸法への変換は重くするだけで、要件 1 はその選択肢を出さないと決めている。
  サーバーも拒むのは、プレイヤーの選択肢が正本の規則とずれたときに、黙って無駄な変換を始めず
  気付けるようにするためである。規則は寸法の比較だけなので、`internal/domain` の純粋関数と
  `web/src/player/quality.ts` の純粋関数がそれぞれ持ち、どちらもテストで同じ表を確かめる。
- Alternatives considered: サーバーが黙って元の寸法に丸める（「480p」の表示のまま 360p の動画を
  480p の上限で変換し、表示と中身がずれる）。選択肢をサーバーの応答に載せる（`Video` の形が増え、
  規則が寸法だけなのに往復が要る）。

## R-4: 画質のメニューは video.js の `MenuButton` の部品にする

- Decision: `web/src/player/qualityMenu.ts` に video.js の `MenuButton`・`MenuItem` を継承した部品を
  作り、`videojs.registerComponent` して `controlBarChildren` の `playbackRateMenuButton` の前に置く。
  ボタンの文字は再生速度の `1x` と同じく今の画質を短く示し、選択肢は React 側から
  `player.trigger` や部品の `setOptions` で渡す。選んだ画質は部品がイベントで知らせ、
  `VideoPlayer.tsx` が切り替え（[R-5](#r-5-画質の切り替えはプレイヤーを作り直さず同じ位置で-source-を差し替える)）
  を行う。`playerControls.ts` の `rateMenuOpen` は `.vjs-menu.vjs-lock-showing` を探すので、そのまま
  画質のメニューにも効く（開いている間の Esc はメニューを閉じるだけ）。
- Rationale: 親 Issue の UI 品質は「再生速度と同じ重み・同じ大きさ・余白・文字」を求める。再生速度は
  video.js の `PlaybackRateMenuButton` なので、同じ基底の部品にすれば見た目（`index.css` の
  `.vjs-menu` の規則）、ポイント・押下・キーボードでの開閉、Esc の扱いが自動で同じになる。
  React の `Popover` で作ると、これらを作り直した上で video.js の部品と 1 px 単位でそろえ続ける
  ことになる。「変換して再生中」が React の吹き出しなのは、押して開く説明であってメニューでは
  ないためで、判断は変わらない。
- Alternatives considered: `web/src/ui/Menu.tsx` を操作バーへ portal で差し込む（上記）。
  `PlaybackRateMenuButton` の選択肢に画質を混ぜる（速度と画質の意味が混ざり、読み上げ名も
  付けられない）。

## R-5: 画質の切り替えはプレイヤーを作り直さず、同じ位置で source を差し替える

- Decision: `PlaybackAttempt` に `quality` を足し、`quality` が「元の画質」以外なら経路は
  `transcode`、「元の画質」なら今の規則（`playable` なら `direct`、でなければ `transcode`）にする。
  切り替えは直接再生から変換への切り替え（`handleFailure` の fallback）と同じ作りで、論理上の
  位置を読み、`attempt` を新しい画質と経路で作り直し、`player.src` を差し替え、`canplay` で
  再生中なら再生を続け、止めていたら止めたままにする（要件 4）。切り替えごとに世代を進め、
  古い `canplay` の処理と古い報告（`attempt`）を捨てる。続けて変えたときは `player.src` の
  差し替えでブラウザが前の要求を打ち切り、サーバーの変換は要求の取り消しで止まる
  （Edge Case 4・10）。`liveSource` は `vvQuality` を持ち、`liveOffset.ts` の `reloadAt` と
  `VideoPlayer.tsx` の `reload` はそれを引き継ぐので、シークと読み込み直しでも画質は変わらない
  （要件 7）。切り替えた変換の開始の失敗は、今の誤りの経路（`playbackRecovery`）がそのまま
  種類で分けて伝え、失敗した位置から再試行できる（Edge Case 5）。選んだ画質は選んだ時点で
  `web/src/preferences/playbackQuality.ts` が `localStorage` に書く（音量と同じ作り。要件 5）。
- Rationale: プレイヤーを作り直す（`VideoPage` の `attempt.key` を進める）と、ポスターに戻って
  操作バーが消え、全画面の内側の状態（吹き出しの入れ物）も作り直しになる。fallback の作りは
  すでに「位置を保って source を差し替え、再生の意図を保つ」を実装していて、同じ経路を通す
  方がテストも共有できる。
- Alternatives considered: `VideoPage` からプレイヤーを作り直す（上記）。video.js の
  `sourceset` を使って部品の側で差し替える（`attempt` の位置と意図が `VideoPlayer.tsx` に
  あるので、切り替えの判断を 2 か所に持つことになる）。

## R-6: 途切れの判断は `waiting`／`playing` の対で数え、落ち着くまでの待ちは数えない

- Decision: `web/src/player/stallMonitor.ts` に純粋な状態機械を置く。データ待ちは、再生中に届いた
  `waiting` から次の `playing` までとし、始まった時刻を持つ。60 秒の窓の中に始まったデータ待ちが
  3 回以上になったか、1 回のデータ待ちが 10 秒を超えたら「回線の遅さで途切れている」と判断する
  （要件 9）。シーク（`seeking`）・source の設定（`loadstart`。最初の読み込みと画質の切り替えを
  含む）・再生の開始（`play`）のあとは、次の `playing` が来るまでの待ちを数えない。止めている
  （`paused`）間と、通信の失敗で読み込み直している間（`recovering`）は数えず、数え直す
  （Edge Case 8）。10 秒の判定は、データ待ちが始まった時点で `VideoPlayer.tsx` が 10 秒のタイマー
  を掛け、まだ続いていれば判断する。判断は `PlayerStatus.stalled` として `VideoPage` に伝える。
  読み込み直しの `error` は途切れではなく、`recovering` に入った時点で数え直す。
- Rationale: `waiting` はシーク直後や読み込みの最初にも届くので、そのまま数えると要件が数えるな
  と言う待ちを数えてしまう。「次の `playing` まで数えない」の 1 つの規則で、シーク・開始・画質の
  切り替えの 3 つの除外を同じ形で扱える。純粋な状態機械にすると、時刻を渡すだけで窓と回数の
  テストが書ける。
- Alternatives considered: `buffered` の残りを監視して速さを推定する（回線の速さを測る作りに
  なり、対象外の自動切り替えへ向かう）。`progress` イベントの間隔で判断する（ブラウザごとの
  差が大きい）。

## R-7: 警告は状態表示の入れ物とは別の層に出し、状態の層が出ている間は隠す

- Decision: 警告は `VideoPage` の入れ物（`data-overlay-layer`）の中の状態表示（読み込み中・失敗・
  再生終了・中央の操作）とは別に、プレイヤーの上端に置く 1 つの小さな帯にする。
  `role="status"` で、閉じるボタン以外は `pointer-events-none` にして、下の操作を遮らない
  （要件 9・10）。状態表示の層（失敗・再生終了・再接続中・読み込み中・次の予告）が出ている間は
  出さず、再生終了・失敗・別の動画への移動では消える（Edge Case 9）。閉じた記録は `VideoPage` が
  動画の id ごとに持ち、同じ動画の再生の間（失敗からの再試行を含む）は出し直さず、別の動画を
  開けば消える（要件 10）。警告には画質を切り替える操作も、自動で下げる仕組みも置かない。
  文言・大きさ・狭い枠での畳み方は design 段階の `ui-design.md` が決める。
- Rationale: 今の入れ物は「同時に 1 つだけ」の層で、中央に置く前提である
  （[library-ui.md](../../docs/design-docs/library-ui.md#8-再生画面の構成)）。警告は再生を止めず
  操作もふさがないので、この入れ物に入れると中央の操作と排他になり、要件 9 に反する。別の層に
  すると、入れ物の排他の規則を変えずに済み、重なりの順（状態の層より控えめ）も CSS で決まる。
- Alternatives considered: `web/src/ui/Toast.tsx` で画面の隅に出す（全画面では見えず、
  プレイヤーの外で再生と結び付かない）。入れ物の層の 1 つにする（上記）。

## R-8: ビットレートと寸法の検査は ffmpeg 付きの Go テストで行う

- Decision: `internal/media/transcode_test.go` の ffmpeg 付きのテスト（`TestTranscodeRotated4K…` と
  同じ作り）で、動きの多い合成入力（`testsrc2` など）を `480p` で変換し、出力の映像の寸法の短辺が
  480、`ffprobe` の映像の平均ビットレートが 1200 kbps × 1.2 以下、音声が 96 kbps 付近であることを
  確かめる。縦長の入力では幅が 480 になることも確かめる。ハードウェアの方式の上限は CI に無い
  ので [quickstart.md](quickstart.md) で実機で確かめる。
- Rationale: 受け入れ条件 2・3 は出力の中身の事実で、引数の文字列の検査だけでは上限が本当に
  効いているか分からない。ffmpeg 付きの Go テストはすでに同じ形で回転・4K・キーフレーム間隔を
  確かめている。e2e は `video.videoHeight` で寸法は見られるがビットレートは見られない。
- Alternatives considered: 引数のテストだけ（上記）。e2e で応答の大きさを測る（ブラウザの
  先読みの量に左右される）。
