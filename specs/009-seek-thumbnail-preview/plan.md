# Implementation Plan: 動画シーク時のサムネイルプレビュー

**Branch**: `codex/seek-thumbnail-preview-feature` | **Date**: 2026-09-22 | **Spec**: GitHub Issue #117

## Summary

再生画面のシークバーが示す論理時刻について、thumbnail background jobが5秒間隔のJPEGを事前生成する。
画像経路は生成済みfileを読むだけとし、再生中にFFmpegを起動しない。プレイヤーは時刻を即時表示し、
5秒単位の画像要求を即時開始し、中断・再利用しながら、ポインターとタッチの移動へ追従する。

要求はGitHub Issue #117、方式選択は [research.md](research.md)、HTTP 差分は
[contracts/seek-thumbnail.md](contracts/seek-thumbnail.md)、UI差分は [ui-design.md](ui-design.md)、
検証手順は [quickstart.md](quickstart.md) を正本とする。永続化する新しいエンティティや状態はないため
`data-model.md` は作成しない。

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

- `GET /api/videos/{id}/seek-thumbnail` は5秒間隔で事前生成したJPEGを返す。動画詳細は
  content-derived versionを含む`seekThumbnailUrl`を返す。
- 既存thumbnail jobを拡張し、生成画像を`MDM_DATA_DIR/thumbnails/seek`へ保存する。既存動画は
  migrationで同じjobへ一度だけ再投入する。
- client は対象時刻を5秒bucketへ切り下げ、前要求の中断、最新位置の照合、decode後の差し替えで
  連続操作を制御する。同じ URL は immutable browser cache で再利用する。
- 既存の直接配信／ライブ変換の source ではなく、元動画全体の `durationMs` とシークバー位置から
  `positionMs` を求めるため、再生経路切り替え後も同じ時間軸を使う。
- parent Issueの即時表示、5秒bucket、100回移動、360px／768px／1280pxをcontract、unit、
  browser test と視覚レビューの基準にする。

## Constitution Check

- **依存方向**: 合格。`internal/httpapi` が静止画抽出の interface を所有し、`internal/media` が
  外部 process 実装を提供し、`cmd/mdm` だけが組み立てる。`internal/domain` へ HTTP や process
  依存を追加しない。
- **API の正本**: 合格。新経路と `Video.seekThumbnailUrl` は `api/openapi.yaml` から Go/TypeScript
  を生成する。
- **データ保護**: 合格。検証済み current location を読み取り専用で開き、元動画、SQLite、
  playback progressを変更しない。data directoryには再構築可能なシーク画像cacheだけを追加する。
- **UI の正本**: 合格。既存の役割トークンとプレイヤー構造を使い、仕様の5観点は Design と
  360px／768px／1280pxの画像レビューで判定する。
- **検証可能性**: 合格。時刻正規化、古い応答の拒否、process cancellation、HTTP status、
  ポインター／タッチ操作を境界別に自動検証できる。
- **文書の近接性**: 合格。本機能固有の判断、契約、検証を feature directory に置く。

Phase 1 後も判定は同じで、例外や Complexity Tracking を必要とする違反はない。

## Structural Decisions

- **background jobで事前生成する**: 5秒間隔の静止画をcontent key単位で永続cacheし、HTTP requestは
  file readだけを行う。requestごとのFFmpeg起動は初回表示が数秒遅れるため廃止する。
- **5秒単位の版付き URL を再利用する**: client は同じ5秒bucketを同じ URL にまとめ、bucket変更時に
  即時取得する。移動時は前要求を中断し、decode完了時に対象時刻を再照合して一度に差し替える。
  取得中に表示済み画像を消す案は黒い面が連続操作へ挟まるため採用しない。
- **静止画専用経路を既存 thumbnail から分ける**: 一覧用の生成済み代表画像と、任意時刻を入力にする
  事前生成画像で status・lifecycle・cache key が異なるため、既存 `/thumbnail` の optional query へ
  多重化しない。ライブ変換経路からフレームを抜く案は source offset と再生経路に依存し、直接配信と
  ライブ変換で意味が変わるため採用しない。
- **プレイヤー内の補助表示として所有する**: Video.js の進捗操作へイベントと表示を追加し、React の
  再生ページや一覧へ状態を持ち上げない。ページ全体で pointer を監視する案はシークバー外の操作まで
  対象にし、既存コントロールの所有境界を崩すため採用しない。

## Project Structure

### Documentation (this feature)

```text
specs/009-seek-thumbnail-preview/
├── plan.md
├── research.md
├── ui-design.md
├── quickstart.md
└── contracts/
    └── seek-thumbnail.md
```

`research.md` は抽出・再利用・境界の判断、contract は新しい binary response、`ui-design.md` は
表示・状態・入力方式の差分、quickstart は時刻精度、連続操作、縮退、視覚確認を持つ。永続データ差分が
ないため `data-model.md` は作成しない。この workflow は `tasks.md` を作らず、下の
Implementation Work を子 Issue 化する。

### Source Code

**Affected boundaries**:

- `api/openapi.yaml`: 任意時刻の JPEG 経路、query、response、`Video.seekThumbnailUrl`
- `internal/media/`: background生成、disk cache、孤児cache cleanup
- `internal/httpapi/`: 生成済みJPEGのbinary responseとcache/error
- `cmd/mdm/`: thumbnail jobへの生成処理とcleanupの組み込み
- `web/src/api/`・`web/src/player/`: 版付き URL、時刻正規化、中断、decode、表示 lifecycle
- `web/e2e/`: ポインター／タッチ、直接配信／ライブ変換、失敗時縮退、viewport 検証

**New paths**:

- `internal/media/seek_thumbnail.go` と対応 test
- `internal/httpapi/seek_thumbnail.go` と対応 test
- `web/src/player/seekPreview.ts` と対応 test
- `web/e2e/seek-preview.e2e.ts`

## Implementation Work

### 任意時刻のシークサムネイル生成・配信 API

**Scope**: [seek thumbnail contract](contracts/seek-thumbnail.md) に従い、OpenAPI と生成物、
`Video.seekThumbnailUrl`、thumbnail jobによる5秒間隔JPEG生成、版付きcache、status/error mapping、
unit/contract testを追加する。既存の一覧サムネイル、動画配信、ライブ変換は変更しない。

**Dependencies**: なし。

**Acceptance**: 有効な動画と時刻へ JPEG と immutable cache が返り、入力不正は400、動画・実体なしは404、
解析未完了・尺なし・cache生成中は409、予期しないI/O失敗は500になる。先頭、中央、末尾の画像が
対応する5秒bucketから返り、HTTP request中にFFmpegを起動しない。生成途中のfileを配信せず、
`task check`が成功する。

### シークバーのサムネイルプレビュー UI

**Scope**: [UI design](ui-design.md)、[親Issue #117](https://github.com/syudead/vv/issues/117)、
[quickstart](quickstart.md) に従い、
シークバーのpointer hover／drag／touchへ追従する静止画と時刻、5秒正規化、即時要求、前要求中断、
decode後の差し替え、古い応答拒否、取得失敗時の時刻のみ表示、破棄時cleanupを既存プレイヤーへ追加する。frontend unit testと
browser testを追加し、一覧画面は変更しない。

**Dependencies**: 任意時刻のシークサムネイル生成・配信 API。

**Acceptance**: 直接配信とライブ変換で同じ論理時刻の画像を表示し、10秒間100回の移動後も最新位置へ
1秒以内に収束して古い画像へ戻らない。画像取得失敗時も時刻、再生、シークが使え、画面離脱後に表示・
要求・object URLが残らない。360px、768px、1280pxの実装画像、5観点のvisual review、pointer／touch／
keyboard／支援技術の確認結果をPRへ添付し、`task check` と対象browser testが成功する。
