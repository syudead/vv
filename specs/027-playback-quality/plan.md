# Implementation Plan: 低速ネットワーク向けの画質選択と、再生が途切れるときの警告

**Branch**: `feature/027-playback-quality` | **Parent Issue**: #521

**Input**: The parent Issue. It is this feature's specification.

## Summary

再生画面の操作バーに「画質」の選択を置き、「元の画質」以外を選ぶと、直接再生できる動画でも
ライブ変換（`GET /api/videos/{id}/transcode.mp4`）で、映像の短辺をその画質に縮め、ビットレートに
上限を付けて配信する。選んだ画質はブラウザに覚え、再生中の切り替え・シーク・読み込み直しでも
保つ。回線の遅さで再生が途切れていると判断したら、プレイヤーの上に知らせるだけの警告を出す。

- **変換**: `quality` を要求のパラメータで受け、`internal/domain` の純粋関数が画質ごとの短辺と
  上限を持ち、`internal/media` が縮小と方式ごとの上限の引数を組み立てる
  （[research.md R-1](research.md#r-1-画質はライブ変換の要求の-quality-パラメータで指定し変換の経路は増やさない)・
  [R-2](research.md#r-2-画質は表示の短辺で縮めビットレートは--maxrate-bufsize-で上限を付ける)・
  [contracts/transcode-quality-api.md](contracts/transcode-quality-api.md)）。
- **選択肢と拒否**: 動画の短辺より小さい画質だけを出し、サーバーも同じ規則で 400 にする
  （[R-3](research.md#r-3-画質が使えるかは動画の短辺で決めサーバーは使えない画質を-400-で拒む)）。
- **プレイヤー**: 画質のメニューは video.js の `MenuButton` の部品にし、切り替えはプレイヤーを
  作り直さず同じ位置で source を差し替える
  （[R-4](research.md#r-4-画質のメニューは-videojs-の-menubutton-の部品にする)・
  [R-5](research.md#r-5-画質の切り替えはプレイヤーを作り直さず同じ位置で-source-を差し替える)）。
- **警告**: `waiting`／`playing` の対で数える純粋な状態機械と、状態表示とは別の層
  （[R-6](research.md#r-6-途切れの判断は-waitingplaying-の対で数え落ち着くまでの待ちは数えない)・
  [R-7](research.md#r-7-警告は状態表示の入れ物とは別の層に出し状態の層が出ている間は隠す)）。
- **画面の設計**: 親 Issue は `ui` ラベルを持つので、メニュー・表示・警告の見た目と操作は design
  段階の `ui-design.md` が決める。この Plan は構造と契約だけを決める。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、認証の境界: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- 今のライブ変換: [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)、
  [docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md)、
  [docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md)、
  [internal/media/transcode.go](../../internal/media/transcode.go)（`buildTranscodeArgs`・
  `videoEncodeArgs`・`encoderCodecArgs`・`outputDimensions`）、
  [internal/httpapi/transcode.go](../../internal/httpapi/transcode.go)、
  [internal/domain/live_transcode.go](../../internal/domain/live_transcode.go)
- 今のプレイヤー: [docs/design-docs/library-ui.md「再生画面の構成」](../../docs/design-docs/library-ui.md#8-再生画面の構成)、
  [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx)（`controlBarChildren`、
  `TranscodeIndicator`、`handleFailure` の fallback、`reload`）、
  [web/src/player/playbackAttempt.ts](../../web/src/player/playbackAttempt.ts)、
  [web/src/player/liveOffset.ts](../../web/src/player/liveOffset.ts)、
  [web/src/player/playbackRecovery.ts](../../web/src/player/playbackRecovery.ts)、
  [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx)（層の入れ物）、
  [web/src/preferences/playbackVolume.ts](../../web/src/preferences/playbackVolume.ts)（覚え方の手本）
- API の正本とエラーの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)、
  画面の文言: [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`・
  `task test-e2e`）

**Feature-specific context**:

- Go と npm の依存は足さない。SQLite の表も設定も足さない（画質はブラウザの `localStorage` だけが
  持つ。親 Issue 要件 5・対象外）。
- 通信が切れたときの読み込み直しと失敗の伝え方は #520 で `main` に入っている。この feature は
  その経路を変えず、画質の切り替えと読み込み直しがどちらも同じ `liveSource` を通るようにする。
- 「元の画質」の変換は今の引数と 1 文字も変えない（要件 2）。既存の引数のテストがそれを守る。
- ハードウェアエンコーダーは CI に無い。方式ごとの上限は software を ffmpeg 付きの Go テストで、
  ハードウェアは [quickstart.md](quickstart.md) で実機で確かめる
  （[R-8](research.md#r-8-ビットレートと寸法の検査は-ffmpeg-付きの-go-テストで行う)）。
- 回線を絞った再生は自動テストにできないので、警告は状態機械の単体テストと、
  [quickstart.md](quickstart.md) の手動の確認で確かめる。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: 画質の値、画質ごとの短辺と上限、動画の寸法に対して使えるかの規則。純粋関数だけ。
  - `internal/media`: 縮小と上限の引数。画質の意味は domain から受け取る。
  - `internal/httpapi`: `quality` の解釈と 400 の判定だけ。`LiveTranscodeRequest.Quality` に載せる。
  - `web/src/player`・`web/src/preferences`: 選択肢の規則、覚え方、切り替え、警告の判断。サーバーの
    状態には依らない。
- **API の正本**（ARCHITECTURE.md）: 合格。`api/openapi.yaml` に `quality` を足して `task generate`
  し、生成物は手で直さない（AGENTS.md）。
- **認証の境界**（ARCHITECTURE.md の認証の段落、要件 8）: 合格。経路は増えず、
  `GET /api/videos/{id}/transcode.mp4` は「ゲストも」のまま。`openapi_routes_test.go` の一致は変わらない。
- **画面の文言はカタログ**（`docs/design-docs/i18n.md`、`.golangci.yml` の gosmopolitan）: 合格。
  メニュー・表示・警告の文言は `web/src/i18n/en.ts` に置き、サーバーの `message` は英語にする。
- **設計文書は今どうなっているかを書く**（`docs/design-docs/index.md` の方針、core-beliefs.md）: 合格。
  各単位が `docs/design-docs/playback-quality.md`（新規）・`library-ui.md`・ARCHITECTURE.md を同じ PR で直す。
- **ドメインイベント**（ARCHITECTURE.md のイベントの段落）: 該当なし。画質も警告もイベントを出さない。
- **索引と利用者データの区別**（ARCHITECTURE.md「Rebuildable and user data」）: 該当なし。
  サーバーに保存するものは無い。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/027-playback-quality/
├── plan.md                              # This file
│                                        # No spec.md — the parent Issue is the specification
├── research.md                          # 要求のパラメータ、縮小と上限、拒否の規則、メニューの部品、切り替え、警告の判断と層、検査
├── quickstart.md                        # 回線を絞った再生と、ビットレート・ハードウェアの実機の確認
├── ui-design.md                         # design 段階が作る（`ui` ラベル）
└── contracts/
    └── transcode-quality-api.md         # transcodeVideo の quality と、画質ごとの約束
```

`data-model.md` は作らない。エンティティも保存する値も足さず、画質ごとの値の表は契約が持つ。

### Source Code

**Affected boundaries**:

- `internal/domain`: `TranscodeQuality` と画質ごとの値、使えるかの規則、`LiveTranscodeRequest.Quality`
- `internal/media`: `buildTranscodeArgs`・`videoEncodeArgs`・`encoderCodecArgs`・`outputDimensions` の
  画質の分岐（コピーしない、縮小、上限、音声）
- `api/openapi.yaml`・`internal/httpapi/transcode.go`（`quality` の解釈と 400）
- `web/src/api/client.ts`（`transcodeUrl` の `quality`）、`web/src/preferences`、`web/src/player`
  （`playbackAttempt.ts`・`liveOffset.ts`・`VideoPlayer.tsx`・`VideoPage.tsx`・`StatusOverlays.tsx`・
  `playerControls.ts`）、`web/src/i18n/en.ts`、`web/e2e/playback.e2e.ts`
- `docs/design-docs/`（新しい設計文書と `library-ui.md`・`index.md`）、`ARCHITECTURE.md`

**New paths**:

- `internal/domain/transcode_quality.go`
- `web/src/preferences/playbackQuality.ts`
- `web/src/player/quality.ts`（選択肢と有効な画質の純粋関数）
- `web/src/player/qualityMenu.ts`（video.js の部品）
- `web/src/player/stallMonitor.ts`（途切れの判断の状態機械）
- `web/src/player/StallWarning.tsx`
- `docs/design-docs/playback-quality.md`（画質の変換・選択・切り替え・警告の現行設計。
  `docs/design-docs/index.md` に載せる）

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。画質の意味
（短辺・上限・使えるか）は `internal/domain` に置き、`internal/media` は引数への写像だけを持つ。
httpapi が短辺や上限を知る案は、拒否の規則と引数の元が 2 か所に分かれるので採らない。プレイヤー側の
同じ規則は `web/src/player/quality.ts` の純粋関数に閉じ、`VideoPlayer.tsx` には持ち込まない。

## Implementation Work

### ライブ変換に画質を足し、選んだ画質で映像の短辺を縮めてビットレートに上限を付ける

**Scope**: `internal/domain` と `internal/media` の変更と、設計文書。
- `internal/domain/transcode_quality.go`: `TranscodeQuality`（`1080p`・`720p`・`480p`・`360p`）、
  `ParseTranscodeQuality`、画質ごとの短辺・映像の上限 kbps・音声 kbps、動画の表示の寸法に対して
  使えるかの純粋関数（[contracts/transcode-quality-api.md §2](contracts/transcode-quality-api.md#2-画質ごとの変換の約束)、
  [research.md R-2](research.md#r-2-画質は表示の短辺で縮めビットレートは--maxrate-bufsize-で上限を付ける)・
  [R-3](research.md#r-3-画質が使えるかは動画の短辺で決めサーバーは使えない画質を-400-で拒む)）。
  `LiveTranscodeRequest.Quality`。
- `internal/media/transcode.go`: 画質があれば映像をコピーせずエンコードし、表示の寸法から短辺が
  画質になる偶数の寸法を計算して `scale` を出し、方式ごとの上限（software・NVENC は一定品質に
  `-maxrate`／`-bufsize`、QSV・VAAPI・VideoToolbox は `-b:v`／`-maxrate`／`-bufsize` の VBR）を足し、
  音声を画質の kbps で AAC にする。画質が無いときの引数は変えない。
- `docs/design-docs/playback-quality.md` の変換の節を書き、`docs/design-docs/index.md` と
  `hardware-encoding.md`「方式ごとの引数」に画質のときの上限を足す。

**Dependencies**: None.

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通る。
- `internal/domain` のテスト: 4 つの画質の短辺と kbps が契約の表と一致する。知らない文字列は
  解釈できない。1920×1080 には `720p`・`480p`・`360p` だけが使え `1080p` は使えない。1080×1920
  （縦長）も同じ。640×360 にはどれも使えない。
- 引数のテスト: 画質の無い要求の引数が今のテストの期待と 1 文字も変わらない。`480p` で 1920×1080 は
  `scale=854:480`、1080×1920 は `scale=480:854`（偶数に丸める規則で確かめる）、極端に細長い
  1200×12000 は `scale=384:3840`・12000×1200 は `scale=3840:384`（今の変換の枠を超えない。R-2）、コピーできる動画でも
  `-c:v libx264`、`-maxrate 1200k -bufsize 2400k`、`-c:a aac … -b:a 96k`。各ハードウェアの方式で
  R-2 の上限の引数がある。`-force_key_frames` と `-movflags` は変わらない。
- ffmpeg 付きのテスト（R-8）: 動きの多い合成入力を `480p` で変換した出力の短辺が 480、映像の
  平均ビットレートが 1200 kbps の 1.2 倍以下、縦長の入力では幅が 480。既存の ffmpeg 付きの
  テスト（回転・縦横比・4K・キーフレーム間隔・MOV）がそのまま通る。

### ライブ変換の API に `quality` を足し、動画の短辺より小さい画質だけを受け付ける

**Scope**: 契約と経路。
- `api/openapi.yaml` の `transcodeVideo` に `quality` を足し（[contracts/transcode-quality-api.md §1](contracts/transcode-quality-api.md#1-get-apivideosidtranscodemp4-の-quality)）、
  `task generate` する。
- `internal/httpapi/transcode.go`: `quality` を解釈し、動画の `Width`／`Height` に対して使えなければ
  400 `invalid_request` にし、使えれば `LiveTranscodeRequest.Quality` に載せる。開始のログに画質を
  添える。
- ARCHITECTURE.md のライブ変換の段落と、`docs/design-docs/playback-quality.md` の API の節。

**Dependencies**: `ライブ変換に画質を足し、選んだ画質で映像の短辺を縮めてビットレートに上限を付ける`。

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通り、`task generate` で差分が出ない。
- `internal/httpapi/transcode_test.go`（helper process）: `quality=480p` の要求が `Quality` を載せて
  始まり、引数に `scale` と `-maxrate 1200k` がある。`quality` の無い要求は今までどおりコピーで
  始まる。動画の短辺以上の画質と寸法の無い動画は 400 で変換を始めない。列挙に無い値は 400。
  `startMs` と `attempt` を付けた `quality` の要求で `transcode-start` が `startMs` を返す。
- `internal/httpapi/guest_test.go`: ゲストが公開の動画を `quality` 付きで変換できる（受け入れ条件 8）。
  `openapi_routes_test.go` が通る。

### 覚えた画質でライブ変換を始め、シークや読み込み直しでも同じ画質を保つ

**Scope**: プレイヤーの経路の決め方と画質の引き継ぎ。メニューはまだ置かない。
- `web/src/preferences/playbackQuality.ts`: `localStorage` の読み書き（`playbackVolume.ts` と同じ作り。
  壊れた値・使えない保存領域は「元の画質」）。
- `web/src/player/quality.ts`: 動画の `width`／`height` から選択肢を作る規則と、覚えた画質が使えなければ
  「元の画質」にする規則（R-3、Edge Case 1・2）。
- `web/src/player/playbackAttempt.ts`: `quality` を足し、「元の画質」以外なら経路を `transcode` にする。
  `web/src/api/client.ts` の `transcodeUrl` と `liveOffset.ts` の `liveSource` に `quality` を通し、
  `reloadAt` と `VideoPlayer.tsx` の `reload` が引き継ぐ（R-5、要件 7）。
- `VideoPlayer.tsx`: 作るときに覚えた画質を読んで `createPlaybackAttempt` に渡す。`TranscodeIndicator`
  に画質を渡し、「元の画質」以外では選んだ画質で変換していることが分かる文（吹き出しの説明も
  画質のもの）にする（要件 6。見た目は `ui-design.md`）。`web/src/i18n/en.ts` に文言を足す。
- `docs/design-docs/playback-quality.md` の選択肢と覚え方の節。

**Dependencies**: `ライブ変換の API に quality を足し、動画の短辺より小さい画質だけを受け付ける`。

**Acceptance**: 画質の表示が変わる単位なので、`ui-design.md` に照らして見た目を確認する。次の検査が
あり、`task check` が通る。
- `playbackQuality.test.ts`: 保存・読み戻し、壊れた値と使えない保存領域で「元の画質」。
- `quality.test.ts`: 1920×1080 の選択肢は `720p`・`480p`・`360p`、1080×1920 も同じ、640×360 は空、
  寸法が無ければ空。1200×12000 は 4 つとも出る（要件 1 は短辺だけで決める）。覚えた `480p` は
  640×360 の動画では「元の画質」になり、保存値は変わらない。
- `playbackAttempt.test.ts`: `480p` で `playable` の動画の経路が `transcode` になり、`sourceOffsetMs`
  が位置になる。「元の画質」は今までどおり。
- `liveOffset.test.ts`: `480p` の source を未 buffer シークで作り直した URL に `quality=480p` がある。
- `VideoPlayer.test.tsx`: 覚えた `480p` で `playable` の動画を開くと最初の `src` が
  `transcode.mp4?…quality=480p` で、表示が 480p の変換であること。通信の失敗からの読み込み直しの
  URL にも `quality=480p` がある。
- `web/e2e/playback.e2e.ts`: `localStorage` に `480p` を入れて 1080p の直接再生できる動画を開くと、
  変換で始まり `videoHeight` が 480、シーク後も 480（受け入れ条件 5・7）。

### 操作バーの画質メニューで再生中に画質を切り替える

**Scope**: メニューの部品と切り替え。見た目・並び・文言は `ui-design.md`。
- `web/src/player/qualityMenu.ts`: video.js の `MenuButton`／`MenuItem` を継承した部品を登録し、
  `controlBarChildren` の再生速度の前に置く（R-4）。選択肢は `quality.ts` の規則で渡し、選択肢が
  「元の画質」だけのときは選べる画質が無いことが分かる形にする（Edge Case 1）。ボタンは今の画質を
  示す。読み上げ名とツールチップは `playerDictionary` と同じくカタログから作る。
- `VideoPlayer.tsx`: 選択で `playbackQuality.ts` に書き、論理上の位置で `attempt` を新しい画質と経路で
  作り直して source を差し替え、再生中なら再生を続け、止めていたら止めたままにする（R-5、要件 4）。
  「元の画質」に戻すと `playable` なら直接再生に戻る（要件 6）。続けて変えたら最後の画質だけを残す
  （Edge Case 4）。`playerControls.ts` の `rateMenuOpen` が画質のメニューにも効くことをテストで確かめ、
  名前を実態に合わせる。
- `docs/design-docs/library-ui.md`「再生画面の構成」の操作バーの項と、`playback-quality.md` の切り替えの節。

**Dependencies**: `覚えた画質でライブ変換を始め、シークや読み込み直しでも同じ画質を保つ`。

**Acceptance**: 画面が変わる単位なので、360px・768px・1280px 幅と縦長の動画の枠で、`ui-design.md` に
照らして見た目と操作（マウス・タッチ・キーボード、全画面）を確認する。次の検査があり、`task check`
が通る。
- `qualityMenu.test.ts`（本物の video.js）: 1080p の動画で項目が「元の画質」「720p」「480p」「360p」で
  `1080p` が無い（受け入れ条件 1）。360p 以下の動画では選べる画質が無いことが分かる。項目を選ぶと
  選択が知らされ、`rateMenuOpen`（改名後）が開閉を見分ける。
- `VideoPlayer.test.tsx`: 再生中に `480p` を選ぶと同じ位置から `quality=480p` の source で再生が続き、
  止めた状態で選ぶと止まったまま同じ位置（受け入れ条件 2・4）。「元の画質」に戻すと `playable` の
  動画は `stream` の source に戻り、メタデータが来たら切り替えた位置へシークして続く（0 から
  始め直さない。受け入れ条件 4・6）。古い source のメタデータは新しい source をシークしない。続けて 2 回変えると最後の画質の source だけが
  残る。切り替えた変換の誤りは今の経路で伝わり、再試行の位置が切り替えた位置。
- `web/e2e/playback.e2e.ts`: 再生中に操作バーから `480p` を選ぶと `videoHeight` が 480 になり、
  現在時刻が戻らない。ゲストでも同じ（受け入れ条件 8）。疑似ロケールの検査（`expectCatalogTextOnly`）
  が通る。

### 回線の遅さで再生が途切れているときに、プレイヤーの上に知らせるだけの警告を出す

**Scope**: 途切れの判断と警告。見た目・置き場所・文言は `ui-design.md`。
- `web/src/player/stallMonitor.ts`: R-6 の状態機械（60 秒の窓で 3 回、1 回 10 秒、シーク・source の
  設定・再生開始の直後と停止中・読み込み直し中は数えない）。
- `VideoPlayer.tsx`: `waiting`／`playing`／`seeking`／`loadstart`／`play`／`pause` と `recovering` を
  状態機械に渡し、10 秒のタイマーを掛け、判断を `PlayerStatus.stalled` で知らせる。
- `web/src/player/StallWarning.tsx` と `VideoPage.tsx`: 状態表示とは別の層に出し、失敗・再生終了・
  再接続中・次の予告の層が出ている間は出さず（データ待ちの読み込み中は並べて出す）、閉じた記録を動画の id ごとに持つ（R-7、要件 9・10、Edge Case 9）。画質を切り替える
  操作は置かない。`web/src/i18n/en.ts` に文言を足す。
- `docs/design-docs/library-ui.md`「再生画面の構成」に警告の層を足し、`playback-quality.md` の警告の節。

**Dependencies**: None.

**Acceptance**: 画面が変わる単位なので、360px・768px・1280px 幅と全画面で、`ui-design.md` に照らして
見た目と操作を確認し、[quickstart.md](quickstart.md) の手順 9・10 の結果を PR の本文に残す。次の検査が
あり、`task check` が通る。
- `stallMonitor.test.ts`: 60 秒の中で 3 回で判断し、2 回では判断しない。61 秒前の 1 回は数えない。
  1 回が 10 秒を超えたら判断する。シーク・source の設定・再生開始の直後の最初の待ちは数えない。
  停止中と読み込み直し中の待ちは数えず、数え直す。
- `VideoPlayer.test.tsx`: 再生中の `waiting` が 60 秒の中で 3 回来ると `onStatus` に `stalled: true` が
  届き、再生は止まらない。1 回の `waiting` が `playing` の来ないまま 10 秒続くと、その最中に
  `stalled: true` が届く。シーク直後の `waiting` は数えない。
- `VideoPage.test.tsx`: `stalled` で警告が出て、`role="status"` で、画質を切り替える操作が無い。
  閉じると消え、同じ動画では再び出ない。別の動画へ移ると閉じた記録が消える。失敗・再生終了・
  再接続中の層が出ている間は警告が出ない。データ待ちの読み込み中（`loading`）と `stalled` が
  同時のときは、読み込み中の表示と警告の両方が見える（R-7）。操作バーと中央の操作が押せる（受け入れ条件 9〜11）。
