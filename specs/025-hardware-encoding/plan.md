# Implementation Plan: ライブ変換でハードウェアエンコードを使えるようにする

**Branch**: `feature/025-hardware-encoding` | **Parent Issue**: #370

**Input**: The parent Issue. It is this feature's specification.

## Summary

ライブ変換（`GET /api/videos/{id}/transcode.mp4`）の映像エンコードを、`libx264` だけでなく
サーバーのハードウェアエンコーダー（NVENC・Quick Sync・VAAPI・VideoToolbox）でも行えるようにする。
所有者が設定画面の新しい区画「動画の変換」で方式（ソフトウェア・各ハードウェア・自動）を選び、
SQLite に保存し、再起動なしに次の変換の要求から効かせる。

- **方式の決定**: 保存した選択と起動時の確認結果から、実際に使う方式を `internal/domain` の
  純粋関数が決め、`internal/app` の `TranscodeSettings` がメモリに持つ
  （[research.md R-3](research.md#r-3-実際に使う方式はドメインの純粋関数が決めapp-がメモリに持つ)、
  [R-4](research.md#r-4-保存先は汎用の-settings-表key-value)）。
- **起動時の確認**: 方式ごとに短い実エンコードを並行して試し、HTTP の待ち受けを待たせない
  （[R-2](research.md#r-2-起動時の確認はエンコーダーごとに短い実エンコードを並行して走らせる)）。
- **変換**: `internal/media` が方式ごとの符号化器の引数を組み立て、共通のフィルターと
  キーフレームの指定で出力の約束を守る。ハードウェアが最初のデータを出す前に失敗したら、同じ
  要求の中でソフトウェアに切り替える（[R-6](research.md#r-6-要求の中での切り替えはエンコードの段でハードウェア--ソフトウェアの順に試す)、
  [R-7](research.md#r-7-エンコード引数は方式ごとの符号化器の指定だけを差し替え出力の約束は共通の引数で守る)）。
- **API と画面**: `GET`/`PUT /api/settings/transcoding`
  （[contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)）と、設定画面の
  区画。見た目と操作の基準は親 Issue の「UI品質」がそのまま仕様である（`ui` ラベルは無く、
  design 段階は無い）。
- **同梱イメージと文書**: 同梱の Docker イメージは `main` と同じ Alpine のままソフトウェア
  エンコードだけとし、コンテナの中では起動時の確認がハードウェアの方式をすべて使えないと報告する。
  ハードウェアエンコードはホスト（Windows・Linux・macOS）に直接入れた VVMDM で使い、その前提と
  手順を文書に書く（[R-1](research.md#r-1-同梱イメージは-alpine-のままにしソフトウェアエンコードだけにする)、
  [R-9](research.md#r-9-ハードウェアエンコードはホストへの直接インストールで使い文書に前提と手順を書く)）。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、設定のデータの区分、認証の境界: [ARCHITECTURE.md](../../ARCHITECTURE.md)
  （「Intended dependency direction」「Rebuildable and user data」、認証の段落）
- 今のライブ変換: [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)、
  [docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md)、
  [internal/media/transcode.go](../../internal/media/transcode.go)（`Start` の切り替えの梯子、
  `buildTranscodeArgs`、`videoEncodeArgs`）、[internal/httpapi/transcode.go](../../internal/httpapi/transcode.go)、
  [internal/domain/live_transcode.go](../../internal/domain/live_transcode.go)
- 起動の順序と配線: [cmd/mdm/main.go](../../cmd/mdm/main.go)、
  [internal/media/preflight.go](../../internal/media/preflight.go)
- 今の設定の作り: [internal/store/media_folders.go](../../internal/store/media_folders.go)
  （`SettingsStore`）、[internal/app/media_folders.go](../../internal/app/media_folders.go)、
  [internal/httpapi/media_folders.go](../../internal/httpapi/media_folders.go)、
  [web/src/settings/SettingsPage.tsx](../../web/src/settings/SettingsPage.tsx)、
  [web/src/settings/ScanStatusSection.tsx](../../web/src/settings/ScanStatusSection.tsx)
- API の正本とエラーの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)、
  画面の文言: [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- 同梱イメージと運用の文書: [Dockerfile](../../Dockerfile)、[compose.yaml](../../compose.yaml)、
  [compose.hosting.yaml](../../compose.hosting.yaml)、
  [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md)、
  [docs/how-to/hosting-vv.md](../../docs/how-to/hosting-vv.md)、
  [.github/workflows/ci.yml](../../.github/workflows/ci.yml)（`linux/amd64,linux/arm64` の公開）
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- Go と npm の依存は足さない。`Dockerfile` と compose のファイルは `main` から変えない（R-1）。
  ハードウェアエンコードの前提（ドライバー、ハードウェアエンコーダーを含む ffmpeg）は、直接
  インストールするホストの側で利用者が用意する（R-9）。
- SQLite は表を 1 つ足す（[data-model.md](data-model.md)）。移行は `internal/store/migrations` の
  次の番号を使う。
- ハードウェアエンコーダーは CI に無い。実機の確認は GPU のあるホストに直接入れて
  [quickstart.md](quickstart.md) で行い、
  自動テストは ffmpeg を差し替える（R-10）。
- 親 Issue の要件 10 は #371（`main` に入っている）の「出力の時刻で 2 秒以下」のキーフレーム間隔を
  含む。`-force_key_frames` の指定は方式に依らず共通なので、そのまま守る（R-7）。
- #371 が決めたコピーの経路（`videoCanCopy`、`CopySeekAllowance`）は変えない。方式の設定は
  エンコードする要求にだけ効く（要件 12）。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: 方式の値、選択の解釈、実際に使う方式を決める規則、OS ごとの確認対象。
    純粋関数と列挙だけで、`os/exec` も `runtime` の値も受け取る引数にする。
  - `internal/media`: 方式ごとの引数、確認用の実エンコード、要求の中の切り替え。store も
    logger も持たず、切り替えの事実は値で返す（R-6）。
  - `internal/app`: 保存値と確認結果を持ち、確認を走らせ、ログを書く。store と checker は
    自分が宣言する interface で受け取る。
  - `internal/store`: `settings` 表の読み書きだけ。
  - `internal/httpapi`: 要求の解釈と `gen` の型への変換だけ。方式の決定は app に聞く。
  - `cmd/mdm`: 配線と、起動時の確認の goroutine の開始。兄弟のパッケージを互いに import しない
    （depguard の規則は変えない）。
- **API の正本**（ARCHITECTURE.md）: 合格。`api/openapi.yaml` を変えて `task generate` し、生成物は
  手で直さない（AGENTS.md）。
- **認証の境界**（ARCHITECTURE.md の認証の段落、要件 13）: 合格。新しい経路は `accessRoutes` に
  足さず所有者だけになり、`openapi_routes_test.go` が `security` との一致を確かめる。設定画面は
  既に所有者だけの経路なので、ゲストに区画は出ない。
- **索引と利用者データの区別**（ARCHITECTURE.md「Rebuildable and user data」）: 合格。`settings` は
  設定のデータで、一覧に足す。
- **ドメインイベント**（ARCHITECTURE.md のイベントの段落）: 該当なし。方式の変更はイベントを出さない
  （R-5）。
- **サーバーの出力は英語**（`.golangci.yml` の gosmopolitan、023 の方針）: 合格。ログ・`message`・
  理由のコードは英語で、画面の文言はカタログが持つ。
- **文書は変更と同じ PR で直す**（core-beliefs.md、AGENTS.md）: 合格。各単位が ARCHITECTURE.md・
  設計文書・how-to を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/025-hardware-encoding/
├── plan.md                              # This file
│                                        # No spec.md — the parent Issue is the specification
├── research.md                          # 実行環境、確認、決定の規則、保存先、切り替え、引数、API、文書、検査、文書への案内
├── data-model.md                        # settings 表とメモリに持つ値
├── quickstart.md                        # GPU のあるホストでの実機の確認
└── contracts/
    └── transcoding-settings-api.md      # GET/PUT /api/settings/transcoding
```

`ui-design.md` は作らない（`ui` ラベルが無い）。画面の基準は親 Issue の「UI品質」にある。

### Source Code

**Affected boundaries**:

- `internal/domain`: `VideoEncoder`・`EncoderChoice`・`EncoderAvailability`・`TranscodeEncoding`、
  `ParseEncoderChoice`・`ResolveVideoEncoder`・`HardwareEncoderCandidates`、
  `LiveTranscodeRequest.VideoEncoder`、`LiveTranscode` に使った方式と切り替えの事実
- `internal/store`: 移行、`SettingsStore` の読み書き
- `internal/app`: `TranscodeSettings`
- `internal/media`: 方式ごとの引数、`EncoderCheck`、`Start` の梯子
- `api/openapi.yaml`・`internal/httpapi`（新しい経路、`transcode.go`、`requiresJSONBody`）・
  `cmd/mdm/main.go`
- `web/src/api`・`web/src/settings`・`web/src/i18n`
- `docs/how-to/running-vv.md`・`docs/how-to/hosting-vv.md`・`ARCHITECTURE.md`・`docs/design-docs/`
  （`Dockerfile`・`compose.yaml`・`compose.hosting.yaml` は `main` のままで、この feature の差分に
  含めない）

**New paths**:

- `internal/store/migrations/000NN_settings.sql`（実装の時点の次の番号）
- `internal/domain/video_encoder.go`
- `internal/app/transcode_settings.go`
- `internal/media/encoder_check.go`
- `internal/httpapi/transcoding_settings.go`
- `web/src/settings/TranscodingSection.tsx`
- `docs/design-docs/hardware-encoding.md`（方式の決定・確認・切り替え・引数の現行設計。
  `docs/design-docs/index.md` に載せる）

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。方式の決定と
確認結果の保持は `internal/app` の `TranscodeSettings` にした。値がメモリ（確認結果）と SQLite
（選択）にまたがり、`MediaFolders` と同じく設定画面の操作の使用例だからである。httpapi が store を
直接読んで決める案は、決定の規則と確認結果の置き場が HTTP の層に入るので採らない。media が
store を読む案は依存方向に反する。

## Implementation Work

### ライブ変換の映像エンコード方式を保存し、起動時の確認結果から実際に使う方式を決める

**Scope**: 方式の値と決定の規則、保存、app の状態。
- `internal/domain`: [data-model.md](data-model.md) §2・§3 の値と、`ParseEncoderChoice`・
  `ResolveVideoEncoder`・`HardwareEncoderCandidates`
  （[R-3](research.md#r-3-実際に使う方式はドメインの純粋関数が決めapp-がメモリに持つ)）。
- `internal/store`: [data-model.md](data-model.md) §1 の移行と、`SettingsStore` の
  `TranscodeEncoderChoice`・`SaveTranscodeEncoderChoice`
  （[R-4](research.md#r-4-保存先は汎用の-settings-表key-value)）。
- `internal/app`: `TranscodeSettings`（保存値の読み込み、checker interface による確認の並行実行と
  エンコーダーごとの上限時間、`Current`、`Select`、起動時と変更時のログ。
  [R-2](research.md#r-2-起動時の確認はエンコーダーごとに短い実エンコードを並行して走らせる)・R-3）。
  `Select` は使えない方式に `domain.ErrEncoderUnavailable` を返す。
- 文書: ARCHITECTURE.md の「Rebuildable and user data」に `settings` を、`internal/app` と
  `SettingsStore` の説明に `TranscodeSettings` と設定の読み書きを足す。

**Dependencies**: None.

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通る。
- `internal/domain` のテスト: 未選択・未知の値が `software` になる。`auto` は使える方式を
  `nvenc`・`qsv`・`vaapi`・`videotoolbox` の順で選び、無ければ `software` で `fallbackReason` が無い。
  ハードウェアの方式は使えなければ `software` と `selected_unavailable`、確認中なら `checking`。
  OS ごとの確認対象（linux・windows・darwin・その他）。
- `internal/store` のテスト: 行が無ければ未選択。保存して読み戻せる。2 度保存すると後の値が残る。
- `internal/app` のテスト（fake の checker）: 確認中は `Current()` が `checking` で
  `software`。確認が終わると結果が反映され、ログに選択・実際の方式・理由が出る。1 つの確認が
  上限時間を超えても他の結果は出て、その方式は `timed_out`。`Select` は使えない方式を拒み、
  保存値を変えない。

### ffmpeg のハードウェアエンコーダーで変換し、確認用の短いエンコードとソフトウェアへの切り替えを行う

**Scope**: `internal/media` の変更。
- `videoEncodeArgs` を方式で分岐させる。`software` の引数は変えない
  （[R-7](research.md#r-7-エンコード引数は方式ごとの符号化器の指定だけを差し替え出力の約束は共通の引数で守る)）。
- `LiveTranscodeRequest.VideoEncoder` を受け取り、`Start` の梯子のエンコードの段を
  「ハードウェア → ソフトウェア」の 2 段にする。使った方式と、切り替えたときのハードウェアの誤りを
  `LiveTranscode` に載せる（[R-6](research.md#r-6-要求の中での切り替えはエンコードの段でハードウェア--ソフトウェアの順に試す)）。
- `EncoderCheck`: 上限時間つきの `-encoders` の読み取りと、方式ごとの短い実エンコード
  （[R-2](research.md#r-2-起動時の確認はエンコーダーごとに短い実エンコードを並行して走らせる)）。
  前の単位の checker interface を満たす。

**Dependencies**: `ライブ変換の映像エンコード方式を保存し、起動時の確認結果から実際に使う方式を決める`
（`domain.VideoEncoder`・`EncoderAvailability` と checker interface を使う）。

**Acceptance**: 次の検査があり、`task check` が通る。
- 引数のテスト: `software` の引数が今のテストの期待と 1 文字も変わらない。各ハードウェアの方式で
  `-c:v h264_<方式>`、High・Level 5.1、4:2:0 8bit への変換、`-force_key_frames` の指定、VAAPI の
  `hwupload`、MOV の二入力が今までどおりある。コピーできる要求では方式に依らず `-c:v copy`。
- helper process のテスト: ハードウェアが最初のデータを出さずに終わると、同じ要求の中で `libx264`
  で始まり、`LiveTranscode` に切り替えの事実が載る。期限切れと取り消しでは切り替えない。最初の
  データを出したあとの失敗では切り替えない。
- `EncoderCheck` のテスト: `-encoders` に無い方式は実行せずに `encoder_missing`。`-encoders` が
  上限時間を超えて固まると、確認対象がすべて `timed_out` になり結果が返る。実エンコードの
  失敗は `check_failed`、固まる helper は上限時間で `timed_out`、成功は `available`。
- ffmpeg 付きの既存のテスト（回転・縦横比・4K・キーフレーム間隔・MOV・切断時の後始末）が
  そのまま通る。

### 設定 API とライブ変換の経路をつなぎ、方式の変更を再起動なしで効かせる

**Scope**: API・経路・配線・設計文書。
- `api/openapi.yaml` に [contracts/transcoding-settings-api.md](contracts/transcoding-settings-api.md)
  の型と経路を足し、`task generate` する。エラーの reason に `encoder_unavailable` を足す。
- `internal/httpapi`: `getTranscodingSettings`・`updateTranscodingSettings`、`requiresJSONBody`。
  `transcode.go` は要求ごとに `TranscodeSettings.Current()` の実際の方式を要求に載せ、切り替えが
  起きたら `Warn`（動画、方式、ffmpeg の誤りの末尾）を記録する。
- `cmd/mdm/main.go`: `TranscodeSettings` を作り、保存値を読み、待ち受けを待たせずに確認の goroutine
  を始め、停止時に止める。
- 文書: `docs/design-docs/hardware-encoding.md` を書き、`docs/design-docs/index.md` と
  ARCHITECTURE.md（ライブ変換の段落、設定 API）を直す。

**Dependencies**:
- `ライブ変換の映像エンコード方式を保存し、起動時の確認結果から実際に使う方式を決める`
- `ffmpeg のハードウェアエンコーダーで変換し、確認用の短いエンコードとソフトウェアへの切り替えを行う`

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通り、`task generate` で差分が
出ない。
- `internal/httpapi` のテスト: `GET` が契約の形を返す。`PUT` で `software`・`auto`・使える方式が
  保存され応答に反映される。使えない方式は 409 `encoder_unavailable`、列挙に無い値は 400。
  ゲストは 401／403 で、`openapi_routes_test.go` が通る。
- `internal/httpapi/transcode_test.go`（helper process）: 方式を変えたあとの要求から
  `-c:v h264_<方式>` で始まる。変える前に始まった要求はそのまま続く。ハードウェアの失敗を注入した
  要求が 200 で本文を返し、`Warn` のログが出る（受け入れ条件 8）。
- `cmd/mdm` のテスト: 確認が終わる前に `/api/health` が応答する。起動ログに方式の行がある。

### 設定画面に「動画の変換」区画を足し、方式の選択と使える方式の表示を行う

**Scope**: 設定画面の区画。基準は親 Issue の「UI品質」と「要件 1・6・8」。
- `web/src/api/client.ts`: `getTranscodingSettings`・`updateTranscodingSettings`。
- `web/src/settings/TranscodingSection.tsx`: 見出し・説明（公開文書サイトの
  `docs/how-to/running-vv.md`「Hardware encoding」への外部リンク。
  [R-11](research.md#r-11-設定画面の説明文は公開文書サイトの節を指す)）・「今使われている方式」・
  選択肢（radio、1 行に 1 つ、方式名と状態）。使えない方式は選べず理由を添える。確認中は「確認中」を出し、終わるまで数秒ごとに
  読み直す（[R-5](research.md#r-5-変更の知らせは出さず画面は表示時と保存の応答で合わせる)）。
  選んだ時点で保存し、保存中は「保存中…」を出して二重の変更を防ぐ。失敗したら選択を元に戻し、
  区画内に理由を出す。`fallbackReason` の警告は `text-warning` で選択肢より上に出す。
  `SettingsPage` に `ScanStatusSection`・メディアフォルダと同じ余白（`mt-8`、見出し下の区切り線）
  で置く。
- `web/src/i18n/en.ts`: 区画の文言、方式名、理由（`unsupported_os`・`encoder_missing`・
  `check_failed`・`timed_out`・`checking`）、reason `encoder_unavailable` の文。

**Dependencies**: `設定 API とライブ変換の経路をつなぎ、方式の変更を再起動なしで効かせる`。

**Acceptance**: 画面が変わる単位なので、360px・768px・1280px 幅で見た目と操作を確認する。次の検査が
あり、`task check` が通る。
- `web/src/settings` の単体テストで、次を確かめる。
  - 未選択の応答でソフトウェアが選ばれている。
  - 使えない方式の radio が disabled で、理由の文が添えられている。
  - 選ぶと `PUT` が送られ、「保存中…」の間は他の選択肢が操作できず、応答で「今使われている
    方式」が変わる。
  - `PUT` の失敗で選択が元に戻り、区画内に `role="alert"` の理由が出る（受け入れ条件 10）。
  - `fallbackReason: selected_unavailable` で警告が選択肢より上に出る。
  - 説明文のリンクの `href` が R-11 の URL で、新しいタブで開く（`target="_blank"`・
    `rel="noreferrer"`）。
  - `checking: true` で確認中の表示になり、`false` を返す応答で置き換わる。
  - 疑似ロケールの検査（`expectCatalogTextOnly`）を通り、ゲストの画面に区画が無い
    （`/settings` が所有者だけであることを既存のテストで確かめる）。

### Docker イメージはソフトウェアエンコードのままにし、ハードウェアエンコードを直接インストールで使う手順を書く

**Scope**: 同梱イメージの範囲の確定と利用者向けの文書。
- `Dockerfile`・`compose.yaml`・`compose.hosting.yaml`: `main` と同じにする（実行段は Alpine で
  `ffmpeg` だけを入れる。GPU のドライバーも GPU を渡す設定も足さない。
  [R-1](research.md#r-1-同梱イメージは-alpine-のままにしソフトウェアエンコードだけにする)）。feature ブランチにこれと違う
  変更があれば `main` の内容に戻す。
- `docs/how-to/running-vv.md`: 「Hardware encoding」の節（ハードウェアエンコードにはホストへの直接
  インストールが要ること、同梱の Docker イメージはソフトウェアエンコードだけで、コンテナの中では
  ハードウェアの方式がすべて使えないと表示されること、方式ごとの前提（ドライバー、デバイス、OS、
  ハードウェアエンコーダーを含む ffmpeg）、直接インストールの手順への案内、設定画面での有効化）。
  GPU をコンテナに渡す override の例は置かない
  （[R-9](research.md#r-9-ハードウェアエンコードはホストへの直接インストールで使い文書に前提と手順を書く)）。
  `docs/how-to/hosting-vv.md` から節を指す。
- [quickstart.md](quickstart.md) を GPU のあるホストに直接入れて実行し、結果を PR の本文に残す。

**Dependencies**: `設定画面に「動画の変換」区画を足し、方式の選択と使える方式の表示を行う`。

**Acceptance**: `task check-docs` が通る。`Dockerfile` と compose のファイルに `main` との差分が無く、
CI の `Docker image` のビルド（`linux/amd64,linux/arm64`）が通る。同梱イメージで起動すると、設定画面で
ハードウェアの方式がすべて使えない状態で表示され、ライブ変換はソフトウェアで動く。文書を読んで、
直接インストールしたホストで前提を満たし、設定画面からハードウェアエンコードを有効にできる
（受け入れ条件 11）。quickstart の各手順の結果が PR の本文にある。
