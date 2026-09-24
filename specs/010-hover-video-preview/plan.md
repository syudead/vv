# Implementation Plan: 一覧画面の hover 動画プレビュー

**Branch**: `codex/hover-video-preview` | **Stage branch**: `codex/plan-hover-video-preview` | **Parent Issue**: [#134](https://github.com/syudead/vv/issues/134)

**Input**: The parent Issue. It is this feature's specification.

## Summary

probe 完了後の background job で、動画全体から短い区間を等間隔に抽出した低解像度・無音の MP4 preview を生成し、content key に紐づく再構築可能な成果物として保存する。グリッドカードの mouse hover は専用 endpoint から生成済み preview だけを再生し、元動画の stream や live transcode を開始しない。

方式選択は [research.md](research.md)、状態と所有権は [data-model.md](data-model.md)、API は [contracts/hover-preview-api.md](contracts/hover-preview-api.md)、UI は [contracts/library-hover-preview.md](contracts/library-hover-preview.md)、検証は [quickstart.md](quickstart.md) を正本とする。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- job と video model: [internal/domain/job.go](../../internal/domain/job.go)・[internal/domain/video.go](../../internal/domain/video.go)・[internal/jobs/worker.go](../../internal/jobs/worker.go)
- scan と生成処理の既存パターン: [internal/scanner/scanner.go](../../internal/scanner/scanner.go)・[internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)・[internal/app/ingest.go](../../internal/app/ingest.go)（旧 `cmd/mdm/library.go`）
- API 正本と配信実装: [api/openapi.yaml](../../api/openapi.yaml)・[internal/httpapi/thumbnail.go](../../internal/httpapi/thumbnail.go)・[internal/httpapi/seek_thumbnail.go](../../internal/httpapi/seek_thumbnail.go)
- 一覧 UI: [library-ui.md](../../docs/design-docs/library-ui.md)・[web/src/library/VideoCard.tsx](../../web/src/library/VideoCard.tsx)・[web/src/library/LibraryPage.tsx](../../web/src/library/LibraryPage.tsx)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)

**Feature-specific context**:

- preview generation は duration を必要とするため probe 成功後に enqueue する。migration は既存の probe 完了動画を `pending` にして backfill し、scan/startup reconciliation は DB state と生成ファイルの欠落・破損を修復する。
- 9 秒を超える動画は全尺から等間隔に 12 区間を選び、各 0.75 秒を時系列に連結する。9 秒以下は全体を一度だけ変換する。出力は最大幅 640px、H.264、`yuv420p`、無音、fast-start MP4 とし、設定 UI は追加しない。
- 生成中は sibling temporary path を使い、MP4 と size/SHA-256 integrity manifest を個別に atomic rename してから `done` にする。完了判定は source location の generation と asset の content identity を分け、current video が同じ content key を参照する限り location-only change 後も成果物を利用する。scan/startup reconciliation は manifest と MP4 の size/digest を照合し、欠落・破損を再生成へ戻す。
- `Video` API は required な `previewState` と、`done` のときだけ optional な `previewUrl` を返す。`GET /api/videos/{id}/preview` は保存済み MP4 を Range 対応で配信し、source stream や live transcode へ fallback しない。
- UI 対象はグリッドカードだけ。開始イベント自身の `pointerType === "mouse"` を確認し、生成済み `previewUrl` だけを source にする。

## Constitution Check

- **依存方向**: 合格。preview policy と生成は `internal/media`、永続状態と job claim は domain/store、配信は httpapi、hover lifecycle は web に置き、sibling package は interface と `cmd/mdm` wiring を介して連携する。
- **API の正本**: 合格。schema と route は `api/openapi.yaml` を先に変更し、`task generate` で server/client generated code を更新する。生成物は手編集しない。
- **データ保護**: 合格。preview file と `preview_state` は thumbnail と同じ再構築可能データであり、`playback_progress` や原本を変更しない。location だけの変更では同じ content key の成果物を再利用する。
- **失敗と再開**: 合格。中断された job は既存 worker 規約で再 queue し、temporary output は完成品として公開しない。最大 retry 後の failure は preview だけを `failed` にし、import、thumbnail、一覧表示を妨げない。
- **UI の正本**: 合格。次の design stage が情報密度、余白、タイポグラフィ、視覚的階層、操作優先順位を具体化する。実装は既存カード外形と overlay を保ち、preview をサムネイル面の一時的な置換に限定する。
- **検証可能性**: 合格。生成 codec/区間、再試行、backfill、Range/cache、source URL、入力方式、cleanup、progress 非保存を自動 test と network/manual evidence で観測できる。

Phase 1 後も例外や Complexity Tracking を必要とする違反はない。

## Structural Decisions

1. **preview 専用 job と state を持つ**: probe/thumbnail と失敗・再試行・修復を独立させる。thumbnail job へ同居させる案は、一方の失敗が他方の状態と retry を曖昧にするため採用しない。
2. **固定の Stash-like sampling policy を使う**: 12 区間 x 0.75 秒を全尺へ分散し、短尺だけ全体変換する。冒頭だけの clip は内容を代表しにくく、設定 UI は親 Issue の対象外なので採用しない。
3. **content key が成果物を所有する**: DB は path ではなく state を保持し、path は content key から決定する。location change で不要な再生成を起こさず、content change と orphan cleanup を明確にする。
4. **専用 endpoint だけを UI に渡す**: hover は生成済み MP4 のみを読み、`stream`/`transcode.mp4` への fallback を持たない。ブラウザ非対応の原本でも生成成功後は同じ preview を再生できる。
5. **media lifecycle は card、排他と reset は library coordinator が所有する**: timer、video element、error fallback、resource cleanup は `VideoCard` が所有する。`LibraryPage` は active card ID と reset epoch だけを渡し、別 card の開始、filter/sort/page/list追加、grid/list 切替、viewport resize で残存 card も停止させる。media element を page state に持たせない。
6. **イベント自身の pointer type で判定する**: `pointerenter` の `pointerType === "mouse"` を使う。primary input を表す media query だけでは、touch 主体端末へ接続した mouse を誤って除外する。

## Project Structure

### Documentation (this feature)

```text
specs/010-hover-video-preview/
├── plan.md
├── ui-design.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── hover-preview-api.md
    └── library-hover-preview.md
```

この workflow は `tasks.md` を作らず、下の Implementation Work を native child Issues に変換する。UI の五観点は design stage の `ui-design.md` で観測可能な基準へ確定する。

### Source Code

- `internal/domain`: preview state、job kind、claim/update contract。
- `internal/store` と `internal/store/migrations/00006_hover_preview.sql`: state persistence、backfill、queue/reconciliation query。
- `internal/media/preview.go`: sampling policy、ffmpeg invocation、temporary generation、atomic publish。
- `internal/scanner` と `cmd/mdm`: probe 後 enqueue、既存 pending repair、size/SHA-256 integrity reconciliation、orphan cleanup、dependency wiring。
- `api/openapi.yaml` と `internal/httpapi`: `previewState`/`previewUrl`、preview route、Range/cache/error semantics。
- `web/src/library`: generated preview URL だけを使う card-owned media lifecycle、library coordinator、tests。

## Implementation Work

### 一覧 hover preview の事前生成と永続化

**Scope**: migration、domain state/job kind、store query、probe 完了後 enqueue、既存 library backfill、固定 sampling policy、atomic publish、retry/restart、missing/corrupt-file repair、source claim と content identity の分離、orphan cleanup を実装する。HTTP と UI は含めない。

**Dependencies**: なし。

**Acceptance**: 新規動画は probe 成功後に一度だけ preview job が queue され、既存の probe 完了動画も再 import なしで backfill される。9 秒超は全尺から 12 x 0.75 秒、9 秒以下は全体を一度だけ変換し、最大幅 640px、H.264/`yuv420p`、無音、fast-start MP4 と size/SHA-256 manifest を content-key path へ公開し、両方が完成するまで `done` にしない。中断 job は再開可能で、retry 中は `pending`、上限到達時は `failed` となるが import/thumbnail/listing は成功を保つ。生成中に source location が変わっても current video の content key が同じなら完成 asset を採用し、content key が変わった場合は temporary output だけを破棄する。MP4 または manifest の欠落、size/digest 不一致は reconciliation が `pending` に戻して再生成し、未参照の両 file は cleanup する。対象 unit/integration tests と `task check` が成功する。

### 生成済み hover preview の API 配信

**Scope**: OpenAPI の `Video.previewState`/`previewUrl` と `GET /api/videos/{id}/preview`、generated code、mapping、保存済み MP4 の Range/cache/error handling を実装する。既存 stream/transcode route は変更しない。

**Dependencies**: `一覧 hover preview の事前生成と永続化`。

**Acceptance**: `previewState` は全動画で返り、`previewUrl` は current content の state が `done` のときだけ返る。endpoint は保存済み MP4 に 200/206 と正しい `Content-Type`/`Accept-Ranges`/`Content-Range` を返し、versioned URL は immutable cache、未生成・失敗・欠落・不正 range は契約どおり 404/416 になる。source stream や live transcode を起動しないことを tests で証明し、`task generate` と `task check` が成功する。

### グリッドカードで生成済み preview を hover 再生

**Scope**: 承認済み `ui-design.md` と UI contract に従い、グリッド `VideoCard` へ mouse pointer の遅延開始、muted/playsInline 再生、停止・error fallback・cleanup を追加する。`LibraryPage` には active card ID と reset epoch だけを持つ coordinator を置く。source は `previewUrl` のみとし、selection、keyboard、touch、row view、本再生、progress 保存を変更しない。

**Dependencies**: `生成済み hover preview の API 配信` と Design stage 完了。

**Acceptance**: `previewState=done` かつ `previewUrl` がある card だけ mouse/trackpad hover で再生し、touch contact、keyboard focus、pending/failed/missing preview は既存表示を保つ。pointer leave、別 card、navigation、filter/sort/page/list追加、grid/list 切替、viewport resize、unmount、`play()` rejection、media error で停止・resource 解放・thumbnail/placeholder 復帰する。残存 card も reset epoch の変更で停止し、同時に active な preview は1件だけになる。ブラウザ非対応の原本でも生成済み preview は再生でき、network evidence に `/stream`、`transcode.mp4`、progress update が現れない。design の五観点を 360px/768px/1280px と reduced-motion 条件で確認した画像、対象 web tests、`task check` を実装 PR に添付する。
