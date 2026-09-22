# Implementation Plan: 動画シーク時のサムネイルプレビュー

**Branch**: `codex/seek-thumbnail-preview-feature` | **Date**: 2026-09-22 | **Spec**: [spec.md](spec.md)

## Summary

再生画面のシークバーが示す論理時刻について、要求時に元動画から静止画を抽出する版付き画像経路を
追加する。プレイヤーは時刻を即時表示し、1秒単位の画像要求を短く遅延・中断・再利用しながら、
ポインターとタッチの移動へ追従する。

要求は [spec.md](spec.md)、方式選択は [research.md](research.md)、HTTP 差分は
[contracts/seek-thumbnail.md](contracts/seek-thumbnail.md)、検証手順は [quickstart.md](quickstart.md)
を正本とする。永続化する新しいエンティティや状態はないため `data-model.md` は作成しない。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- 技術選定と FFmpeg 利用: [tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
- API スキーマ: [api/openapi.yaml](../../api/openapi.yaml)
- 既存サムネイルとキャッシュ契約:
  [002 HTTP 契約](../002-core-video-library/contracts/http-routes.md)
- ライブ変換の論理時間軸: [008 live playback contract](../008-live-mp4-playback/contracts/live-playback.md)
- Web の所有境界と視覚規則:
  [ARCHITECTURE.md](../../ARCHITECTURE.md#web-layer)・
  [library-ui-design-system.md](../../docs/design-docs/library-ui-design-system.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)

**Feature-specific context**:

- `GET /api/videos/{id}/seek-thumbnail` を OpenAPI に追加し、元動画の論理時刻 `positionMs` に対応する
  JPEG を返す。動画詳細は content-derived version を含む `seekThumbnailUrl` を返す。
- 抽出処理は request context と短い上限時間に従い、生成画像を永続化しない。新しい SQLite 列、
  background job、data directory は追加しない。
- client は対象時刻を最寄りの1秒へまとめ、150ms の debounce、前要求の中断、最新位置の照合で
  連続操作を制御する。同じ URL は immutable browser cache で再利用する。
- 既存の直接配信／ライブ変換の source ではなく、元動画全体の `durationMs` とシークバー位置から
  `positionMs` を求めるため、再生経路切り替え後も同じ時間軸を使う。
- [spec.md](spec.md) の表示1秒、時刻誤差1秒、100回移動、360px／768px／1280pxを contract、unit、
  browser test と視覚レビューの基準にする。

## Constitution Check

- **依存方向**: 合格。`internal/httpapi` が静止画抽出の interface を所有し、`internal/media` が
  外部 process 実装を提供し、`cmd/mdm` だけが組み立てる。`internal/domain` へ HTTP や process
  依存を追加しない。
- **API の正本**: 合格。新経路と `Video.seekThumbnailUrl` は `api/openapi.yaml` から Go/TypeScript
  を生成する。
- **データ保護**: 合格。検証済み current location を読み取り専用で開き、元動画、SQLite、
  playback progress、data directory を変更しない。
- **UI の正本**: 合格。既存の役割トークンとプレイヤー構造を使い、仕様の5観点は Design と
  360px／768px／1280pxの画像レビューで判定する。
- **検証可能性**: 合格。時刻正規化、古い応答の拒否、process cancellation、HTTP status、
  ポインター／タッチ操作を境界別に自動検証できる。
- **文書の近接性**: 合格。本機能固有の判断、契約、検証を feature directory に置く。

Phase 1 後も判定は同じで、例外や Complexity Tracking を必要とする違反はない。

## Structural Decisions

- **要求時に1枚だけ抽出する**: 位置ごとの静止画を request-scoped process で生成し、応答後は残さない。
  全尺のスプライトを取り込み時に生成する案は、見られない長尺動画にも保存量と走査時間を先払いし、
  既存の直列 job を長時間占有するため採用しない。各フレームを永続キャッシュする案も、利用された
  時刻に比例する回収対象を新設するため採用しない。
- **1秒単位の版付き URL を再利用する**: client は最寄りの1秒を同じ URL にまとめ、150ms 静止した
  対象だけ取得する。移動時は前要求を中断し、完了時に対象時刻を再照合する。生の pointer move ごとに
  process を起動する案は負荷と古い応答競合を増やし、粗い5秒以上の区間は仕様の1秒精度を満たさない。
- **静止画専用経路を既存 thumbnail から分ける**: 一覧用の生成済み代表画像と、任意時刻を入力にする
  一時画像で status・lifecycle・cache key が異なるため、既存 `/thumbnail` の optional query へ
  多重化しない。ライブ変換経路からフレームを抜く案は source offset と再生経路に依存し、直接配信と
  ライブ変換で意味が変わるため採用しない。
- **プレイヤー内の補助表示として所有する**: Video.js の進捗操作へイベントと表示を追加し、React の
  再生ページや一覧へ状態を持ち上げない。ページ全体で pointer を監視する案はシークバー外の操作まで
  対象にし、既存コントロールの所有境界を崩すため採用しない。

## Project Structure

### Documentation (this feature)

```text
specs/009-seek-thumbnail-preview/
├── spec.md
├── plan.md
├── research.md
├── quickstart.md
└── contracts/
    └── seek-thumbnail.md
```

`research.md` は抽出・再利用・境界の判断、contract は新しい binary response、quickstart は
時刻精度、連続操作、縮退、視覚確認を持つ。永続データ差分がないため `data-model.md` は作成しない。
この workflow は `tasks.md` を作らず、下の Implementation Work を子 Issue 化する。

### Source Code

**Affected boundaries**:

- `api/openapi.yaml`: 任意時刻の JPEG 経路、query、response、`Video.seekThumbnailUrl`
- `internal/media/`: 1 request 分の静止画抽出、上限時間、process cancellation
- `internal/httpapi/`: current location 検証、抽出 interface、binary response と cache/error
- `cmd/mdm/`: media adapter の注入
- `web/src/api/`・`web/src/player/`: 版付き URL、時刻正規化、debounce、中断、表示 lifecycle
- `web/e2e/`: ポインター／タッチ、直接配信／ライブ変換、失敗時縮退、viewport 検証

**New paths**:

- `internal/media/seek_thumbnail.go` と対応 test
- `internal/httpapi/seek_thumbnail.go` と対応 test
- `web/src/player/seekPreview.ts` と対応 test
- `web/e2e/seek-preview.e2e.ts`

## Implementation Work

### 任意時刻のシークサムネイル生成・配信 API

**Scope**: [seek thumbnail contract](contracts/seek-thumbnail.md) に従い、OpenAPI と生成物、
`Video.seekThumbnailUrl`、安全な current location からの request-scoped JPEG 抽出、版付き cache、
中断・期限、status/error mapping、unit/contract testを追加する。既存の一覧サムネイル、動画配信、
ライブ変換、DB schema、job queueは変更しない。

**Dependencies**: なし。

**Acceptance**: 有効な動画と時刻へ JPEG と immutable cache が返り、入力不正は400、動画・実体なしは404、
解析未完了・尺なし・フレーム取得不能は409、process開始不能は500になる。先頭、中央、末尾の画像が
要求時刻から1秒以内で、request中断時に抽出processが終了する。元動画、SQLite、data directoryに変更が
なく、`task check` が成功する。

### シークバーのサムネイルプレビュー UI

**Scope**: [spec UI acceptance](spec.md#ui-acceptance-criteria) と [quickstart](quickstart.md) に従い、
シークバーのpointer hover／drag／touchへ追従する静止画と時刻、1秒正規化、150ms debounce、前要求中断、
古い応答拒否、取得失敗時の時刻のみ表示、破棄時cleanupを既存プレイヤーへ追加する。frontend unit testと
browser testを追加し、一覧画面は変更しない。

**Dependencies**: 任意時刻のシークサムネイル生成・配信 API。

**Acceptance**: 直接配信とライブ変換で同じ論理時刻の画像を表示し、10秒間100回の移動後も最新位置へ
1秒以内に収束して古い画像へ戻らない。画像取得失敗時も時刻、再生、シークが使え、画面離脱後に表示・
要求・object URLが残らない。360px、768px、1280pxの実装画像、5観点のvisual review、pointer／touch／
keyboard／支援技術の確認結果をPRへ添付し、`task check` と対象browser testが成功する。
