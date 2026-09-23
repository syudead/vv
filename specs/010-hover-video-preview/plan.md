# Implementation Plan: 一覧画面の hover 動画プレビュー

**Branch**: `codex/hover-video-preview` | **Stage branch**: `codex/plan-hover-video-preview` | **Parent Issue**: [#134](https://github.com/syudead/vv/issues/134)

**Input**: The parent Issue. It is this feature's specification.

## Summary

グリッド表示の動画カードで、hover できるポインターが一定時間留まったときだけ、サムネイル領域をミュート済みの直接ストリーム再生へ差し替える。プレビューは一覧カードの補助表示として `web/src/library/VideoCard.tsx` に閉じ、本再生・進捗保存・行ビュー・タッチ向け長押しを増やさない。

要求は GitHub Issue #134、方式選択は [research.md](research.md)、UI contract は [contracts/library-hover-preview.md](contracts/library-hover-preview.md)、検証手順は [quickstart.md](quickstart.md) を正本とする。永続化する新しいエンティティや状態はないため `data-model.md` は作成しない。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- Web 所有境界と一覧 UI の視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・[library-ui.md](../../docs/design-docs/library-ui.md)
- API スキーマと生成型: [api/openapi.yaml](../../api/openapi.yaml)・[web/src/api/client.ts](../../web/src/api/client.ts)
- 一覧カードと選択操作: [web/src/library/VideoCard.tsx](../../web/src/library/VideoCard.tsx)・[web/src/library/LibraryPage.tsx](../../web/src/library/LibraryPage.tsx)
- 再生画面の直接配信 URL と進捗保存: [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx)・[web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)

**Feature-specific context**:

- 対象はグリッド表示のカードだけで、`VideoRow` には追加しない。親 Issue の対象外に従い、行ビューの hover preview は作らない。
- プレビュー開始条件は `video.playable === true`、`video.probeState === "done"`、再生に必要な既存 `Video` 情報が揃っていること、かつ `pointerenter` を発生させた入力の `pointerType` が `mouse` であることとする。`thumbnailUrl` は poster と復帰先に使える場合だけ使い、preview eligibility にはしない。
- プレビューの取得先は既存の `GET /api/videos/{id}/stream` とし、ライブ変換 `transcode.mp4` は使わない。ブラウザ直接再生できない動画を hover のために変換しないという親 Issue の対象外に合わせる。
- プレビュー要素は `muted`、`playsInline`、`preload="metadata"` または実装上同等の軽い初期取得を使い、音声有効化 UI を持たない。`play()` rejection や media error はサムネイルへ戻す。

## Constitution Check

- **依存方向**: 合格。Go/domain/store/API contract を変えず、既存 `Video` contract と stream route を web layer が読むだけにする。
- **API の正本**: 合格。OpenAPI の schema や生成物を変更しないため、`task generate` は差分なしで通る必要がある。
- **データ保護**: 合格。プレビューは `saveProgress`、`beaconProgress`、`PUT /api/videos/{id}/progress` を呼ばず、`playback_progress` を変更しない。
- **UI の正本**: 合格。親 Issue の `UI品質とアクセシビリティ` が視覚的階層、情報密度、余白、タイポグラフィ、操作優先順位を定め、カード外形、グリッド幅、カード高さ、選択チェック、進捗バー、視聴済み表示、再生不可表示は [library-ui.md](../../docs/design-docs/library-ui.md) の一覧構成を保つ。次の design stage で `ui-design.md` に観測可能な判定基準を具体化し、実装 PR はその承認後に 360px / 768px / 1280px の画像を証拠として添付する。
- **検証可能性**: 合格。hover delay、単一 active preview、cleanup、progress API 不使用、keyboard/touch 非起動、reduced motion を web unit test と手順確認で観測できる。
- **文書の近接性**: 合格。feature 固有の判断、UI contract、検証はこの feature directory に置く。

Phase 1 後も判定は同じで、例外や Complexity Tracking を必要とする違反はない。

## Structural Decisions

- **カードローカルな preview component/hook として実装する**: `VideoCard` のサムネイル領域が開始待ち、video element、error fallback、cleanup を所有する。`LibraryPage` で active id を集中管理する案は、一覧の取得・選択・スクロール復元の責務に preview playback lifecycle を混ぜるため採用しない。
- **イベントを発生させた mouse pointer だけで開始する**: `pointerenter`/`pointerleave` を使い、開始時の `event.pointerType === "mouse"` で mouse/trackpad に限定する。主入力だけを表す `(hover: hover) and (pointer: fine)` を eligibility 判定に使う案は、タッチ主体端末へ接続した mouse を除外するため採用しない。focus や touch contact を開始条件にする案も、親 Issue がキーボードと touch の既存操作を優先すると定めているため採用しない。
- **直接 stream だけを使う**: `video.playable` が true のカードだけ `streamUrl(video.id)` を再生する。hover 用に `transcode.mp4` へ fallback する案は、一覧 hover が本再生より重いライブ変換を起動し、対象外のブラウザ非対応動画 preview を実質的に追加するため採用しない。

## Project Structure

### Documentation (this feature)

```text
specs/010-hover-video-preview/
├── plan.md
├── research.md
├── quickstart.md
└── contracts/
    └── library-hover-preview.md
```

`research.md` は pointer event gating と stream 選択の判断、contract はカードが利用者へ見せる状態遷移、quickstart は入力方式・進捗非保存・画面幅確認を持つ。UI の5観点は親 Issue を入力として次の design stage の `ui-design.md` で具体化するため、この Plan PR では作成しない。永続データ差分がないため `data-model.md` は作成しない。この workflow は `tasks.md` を作らず、下の Implementation Work を子 Issue 化する。

### Source Code

**Affected boundaries**:

- `web/src/library/VideoCard.tsx`: グリッドカードのサムネイル領域に hover preview lifecycle と表示状態を追加する。`VideoRow` は対象外のままにする。
- `web/src/api/client.ts`: 既存 `streamUrl` helper を library preview から再利用する。API request helper や progress helper は変更しない。
- `web/src/library/LibraryPage.test.tsx` または新しい library unit test: hover delay、cleanup、selection mode、keyboard/touch 非起動、progress API 不使用を検証する。
- `web/src/index.css`: 必要な場合だけ、既存 token と `motion-reduce:` に沿って preview 中の transition を調整する。

**New paths**:

- `web/src/library/hoverPreview.ts` または `web/src/library/HoverPreview.tsx`。実装時に、`VideoCard.tsx` 内へ収めた方が短く明確なら新規 path は作らない。

## Implementation Work

### グリッドカードの hover 動画プレビュー

**Scope**: 承認済みの `ui-design.md`、[UI contract](contracts/library-hover-preview.md)、[research](research.md) に従い、グリッド `VideoCard` のサムネイル領域へ mouse pointer による遅延開始、muted/playsInline video、cleanup、play rejection fallback、pointer leave/navigation/unmount 停止を追加する。選択チェック、進捗バー、視聴済み・再生不可表示、focus ring、selection mode、keyboard/touch 操作、reduced motion を既存の優先順位で保つ。行ビュー、本再生、progress 保存、OpenAPI は変更しない。

**Dependencies**: Design stage で `ui-design.md` が承認済みであること。

**Acceptance**: 直接再生できるグリッドカードへ mouse/trackpad pointer を一定時間置くと muted preview がサムネイル領域内で始まり、pointer leave、別カード hover、クリック遷移、一覧再描画または unmount で停止して元の thumbnail/placeholder へ戻る。`play()` が拒否された場合と media load/playback error の場合も元の表示へ戻り、再度 hover できる。タッチ主体端末へ接続した mouse でも開始し、touch contact と keyboard focus では開始しない。解析中、解析失敗、再生情報不足、`playable=false` のカードは既存表示を保ち、selection mode と checkbox 操作を妨げない。`fetch`/`PUT /api/videos/{id}/progress`、`saveProgress`、`beaconProgress` は呼ばれない。実装 PR に `ui-design.md` の判定基準に沿う 360px、768px、1280px の画像と視覚・操作・支援技術確認を添付し、`task check` と、delay/cancel、cleanup、入力方式、ineligible state、selection、progress 非保存、`play()` rejection、media error recovery を扱う対象 web test が成功する。
