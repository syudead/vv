# Implementation Plan: MP4 ライブ変換による動画再生

**Branch**: `codex/live-mp4-playback-feature` | **Date**: 2026-09-22 | **Spec**: [spec.md](spec.md)

## Summary

既存の Range 対応直接配信を第一候補のまま維持し、直接再生できない動画または実再生で失敗した
動画を、FFmpeg が生成する fragmented MP4 として HTTP 応答へ直送する。ライブ変換中のシークは
指定位置から応答を作り直し、Video.js の時間軸 middleware で元動画の尺と再生位置を維持する。

要求は [spec.md](spec.md)、技術判断は [research.md](research.md)、実行時状態は
[data-model.md](data-model.md)、HTTP 差分は [contracts/live-playback.md](contracts/live-playback.md)、
検証手順は [quickstart.md](quickstart.md) を正本とする。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- 技術選定と既存配信: [tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
- API スキーマ: [api/openapi.yaml](../../api/openapi.yaml)
- 既存 Range・キャッシュ・path 安全性契約:
  [002 HTTP 契約](../002-core-video-library/contracts/http-routes.md)
- Web 依存: [web/package.json](../../web/package.json)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)

**Feature-specific context**:

- `video.js` を Web runtime dependency に追加し、MP4 ライブ変換の仮想時間軸と source 切り替えを
  middleware で扱う。DASH/HLS player dependency は追加しない。
- `GET /api/videos/{id}/transcode.mp4` を OpenAPI に追加する。応答は保存されない fragmented MP4
  であり、`startMs` ごとに request-scoped FFmpeg process を1つ起動する。
- transcode request開始時のffprobeでstream互換属性を検査し、確認できたstreamだけをcopyする。
  `startMs>0`は正確なseekのため映像・音声をencodeする。probe結果は永続化しない。
- request時と取り込み時のprobeはattached coverを除外し、最初の非添付video streamを本編として扱う。
- server寿命contextをtranscoderへ注入し、SIGINT/SIGTERM時はHTTP shutdownを待つ前に変換をcancelする。
- SQLite schema と既存の再生位置契約は変更しない。ライブ変換 session、buffer、代替動画を永続化
  しない。
- [spec.md](spec.md) の開始3秒、シーク2秒、再開位置誤差5秒、終了10秒を E2E と process lifecycle
  test の基準にする。

## Constitution Check

- **依存方向**: 合格。`internal/httpapi` が必要最小限の transcoder interface を所有し、
  `internal/media` が `os/exec` 実装を提供し、`cmd/mdm` だけが両者を組み立てる。
  `internal/domain` へ HTTP・FFmpeg 依存を持ち込まない。
- **API の正本**: 合格。新経路は `api/openapi.yaml` を先に変更し、Go/TypeScript 生成物は
  `task generate` で更新する。
- **元ファイルと利用者データ**: 合格。検証済み location を読み取り専用 input にし、出力は
  response pipe のみとする。DB migration はなく、`playback_progress` の鍵と保存頻度も変えない。
- **検証可能性**: 合格。FFmpeg 引数、process cancellation、HTTP response、有限 fallback、
  形式 matrix を unit/contract/E2E test に分ける。
- **文書の近接性**: 合格。本機能の仕様、技術判断、HTTP 契約、検証手順を同じ feature directory
  に置く。

Phase 1 後も判定は同じであり、例外や Complexity Tracking を必要とする違反はない。

## Structural Decisions

- **変換結果は request-scoped fragmented MP4 として直送する**: FFmpeg stdout を response へ
  copy し、request context の終了で process を停止する。DASH/HLS は manifest と segment の一時
  保持・回収を要し、保存しない単一品質の要件に不要な session cache を導入するため採用しない。
- **ライブ変換の時間軸だけ Video.js middleware で補正する**: 元動画の duration、論理 currentTime、
  buffered range を source 開始 offset と合成し、未buffer位置への seek で `startMs` を変えて source
  を再読込する。native `<video>` だけに任せる案は、途中開始した fragmented MP4 の時間軸を元動画
  全体として表示できないため採用しない。MSE を直接操作する案は、今回不要な segment parser と
  buffer lifecycle を自前所有するため採用しない。
- **互換属性を確認できた stream だけを copy する**: request時probeでH.264のprofile/level/pixel
  format/bit depthとAACのprofile/sample rate/channel数を検査し、保守的allowlistをすべて満たすstream
  だけをcopyする。未知値、direct失敗後、`startMs>0`は必要なencodeへ倒す。H.264 encode時の奇数寸法は
  内容をscaleせず最大1px padする。codec名だけの判定は非互換profileを持ち越し、途中位置でのcopyは
  keyframeまで巻き戻って論理時間と実映像をずらすため採用しない。
- **fallback は frontend の有限 state machine にする**: 解析結果が直接再生可なら direct、不可なら
  transcode から開始し、direct の decode/source error から transcode へ移るのは1回だけとする。
  server が user agent を推測して redirect する案は実際の decode failure を観測できず、同じ経路を
  循環させうるため採用しない。
- **global transcode manager を置かない**: 各 HTTP request が1 processを所有し、seek/source変更/
  画面離脱/切断で旧requestを cancelする。同じ動画のrequest共有は、一方のtabのseekや終了が他方を
  巻き込むため採用しない。server停止だけは全request共通の寿命contextをcancelし、process registryを
  作らずHTTP shutdown猶予内に終了させる。

## Project Structure

### Documentation (this feature)

```text
specs/008-live-mp4-playback/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    └── live-playback.md
```

`research.md` は本機能固有の transport/player/process 判断、`data-model.md` は永続化しない再生試行と
process の状態遷移、契約と quickstart は新しい binary response と形式別検証をそれぞれ持つ。

### Source Code

**Affected boundaries**:

- `api/openapi.yaml`: MP4 ライブ変換経路と query/response/error schema
- `internal/media/`: FFmpeg argument selection、stdout/stderr、process cancellation
- `internal/httpapi/`: 安全な location 解決、transcoder interface、binary response
- `cmd/mdm/`: media adapter、server寿命contextの注入と shutdown lifecycle
- `web/src/api/`・`web/src/player/`: source URL、有限 fallback、offset seek、既存 progress 連携
- `web/e2e/`: 形式 matrix、runtime fallback、seek/resume、process cleanup の browser test

**New paths**:

- `internal/media/transcode.go` と対応 test
- `internal/httpapi/transcode.go` と対応 test
- `web/src/player/VideoPlayer.tsx`、`web/src/player/liveOffset.ts` と対応 test
- `web/e2e/playback.e2e.ts`

## Implementation Work

### fragmented MP4 ライブ変換と配信 API

**Scope**: [live-playback contract](contracts/live-playback.md) に従い、request時のstream互換probe、
attached coverを除く本編stream選択、属性ごとのcopy/変換、奇数寸法padding、途中開始のaccurate seek、
fragmented MP4 stdout、bounded stderr、request/server cancellationを`internal/media`に追加する。既存の
取り込みprobeも同じ本編選択へ揃える。location安全性を再利用する`transcode.mp4`経路、OpenAPI/生成物、
router wiring、unit/contract testを追加する。`cmd/mdm`はserver contextをHTTP shutdown前にcancelする。
直接配信とそのRange契約は変更しない。

**Dependencies**: なし。

**Acceptance**: 互換属性をすべて確認できたstreamだけがFFmpeg引数上copyされ、未知・非対応stream、
direct失敗後、途中開始だけがencodeされる。奇数寸法を含むH.264 encodeが成功し、長いGOP内へのseekで
実映像とlogical timeが一致する。cover先行時は非添付videoをmapし、coverしかなければ409を返す。
`startMs`の200、入力不正の400、実体なしの404、起動失敗の500が契約どおり返る。requestまたはserver
cancel後5秒以内にprocessが終了し、SIGTERM時はHTTP shutdownの10秒猶予内にhandlerが返る。元ファイルと
data directoryに動画出力は作られない。

### 直接配信からライブ変換へ有限切り替えするプレイヤー

**Scope**: `video.js` と [runtime state model](data-model.md) を使う `VideoPlayer` を追加し、解析結果に
よる初期経路、direct error時の1回だけのfallback、offset付きseek、pause/play意図、再開位置、終了時
disposeを実装する。既存 `VideoPage` のprogress保存と最終error表示へ接続し、解像度選択UIは追加しない。

**Dependencies**: fragmented MP4 ライブ変換と配信 API。

**Acceptance**: direct動画は既存stream URLだけを使い、非対応動画は最初からtranscode URLを使う。
direct decode errorは同じ論理位置から1度だけtranscodeへ移り、次のerrorで停止する。ライブ変換でseek、
再開、pause、終了が既存画面内で動作し、実装PRにdesktop/mobile画像とvisual/keyboard確認結果が付く。

### 動画形式 matrix の E2E

**Scope**: [quickstart](quickstart.md) のfixture matrixをFFmpegで生成し、direct、containerのみ非対応、
videoのみ非対応、audioのみ非対応、両方非対応、無音、attached cover、codec属性境界、奇数寸法、
長いGOPの途中seek、runtime fallback、resume、error、request/server cleanupをPlaywrightとbackend
table testで検証する。

**Dependencies**: fragmented MP4 ライブ変換と配信 API、直接配信からライブ変換へ有限切り替えする
プレイヤー。

**Acceptance**: `task check` と `task test-e2e` が通り、matrix全件が3秒以内の開始、ライブ変換seekが
2秒以内、再開誤差5秒以内、最終errorが10秒以内、離脱process終了が10秒以内を観測する。
