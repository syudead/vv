# Implementation Plan: 設定画面

**Branch**: `codex/plan-settings-screen` | **Date**: 2026-09-20 | **Spec**: [spec.md](spec.md)

## Summary

設定画面を追加し、メディアフォルダだけを変更可能にする。保存値は SQLite を正本とし、
走査は利用者の明示操作でのみ開始する。

## Feature Behavior

- 画面は「設定」とし、設定項目はメディアフォルダ1件だけにする。データフォルダ、待受先、
  ログレベル、表示設定などは置かない
- 現在有効なサーバー上の絶対パスを表示し、その場で編集・保存できる
- 初回値は既存の `MDM_MEDIA_DIR` を引き継ぎ、画面から保存した後は保存値を正本にする
- 保存前に、絶対パス、存在、ディレクトリ、読取可能性を検証する。失敗時は現在値を変更せず、
  入力中の値と理由を画面に残す
- 保存だけでは取り込みを開始しない。保存後に手動取り込みが必要であることを示し、次の
  手動取り込みが新しいフォルダを使う
- 走査中は変更できない。複数画面からの古い保存は競合として拒否し、新しい値を上書きしない
- 起動時の自動取り込みと `MDM_SCAN_ON_START` は廃止する
- 画面は読み込み中、保存中、保存成功、入力エラー、競合、走査中を扱う

詳細な利用者シナリオと受け入れ条件は [spec.md](spec.md) を正本とする。

## Technical Context

技術構成、依存方向、生成物、検証コマンドは既存定義を変更しない。

- 構成と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- Go / Web の依存: [go.mod](../../go.mod)、[web/package.json](../../web/package.json)
- API の正本: [api/openapi.yaml](../../api/openapi.yaml)
- 開発・検証コマンド: [Makefile](../../Makefile)

本機能固有の技術判断だけを [research.md](research.md) に記録する。

## Constitution Check

`.specify/memory/constitution.md` は未設定。既存の `AGENTS.md` と `ARCHITECTURE.md` に対して、
次を確認した。

- `cmd/mdm` で store、scanner、httpapi を組み立て、internal の兄弟依存を増やさない
- API は OpenAPI を先に変更し、生成物を手編集しない
- 設定項目をメディアフォルダ以外へ広げない
- UI 実装 PR は既存の画像確認手順に従う

設計後も違反なし。

## Project Structure

既存の所有境界を維持する。永続化は `internal/store`、実行時の調停は `cmd/mdm`、HTTP は
`internal/httpapi`、画面は `web/src/settings` に置く。新しいパッケージや外部依存は追加しない。

## Complexity Tracking

該当なし。

## Implementation Work

### メディアフォルダ設定の保存と実行時反映

**Scope**: SQLite の設定行、初回 `MDM_MEDIA_DIR` 引継ぎ、パス検証、走査開始との排他、
走査・動画配信への反映、`MDM_SCAN_ON_START` と起動時走査の削除。

**Dependencies**: なし。

**Acceptance**: 保存値が再起動後も残り、保存や起動では走査せず、次の手動走査だけが新しい
フォルダを使う。

### 設定 API

**Scope**: `GET /api/settings` と `PUT /api/settings` を OpenAPI と HTTP 実装へ追加する。

**Dependencies**: 設定の保存と実行時反映。

**Acceptance**: 有効値を取得・保存でき、無効パス、走査中、版競合を契約どおり拒否する。

### 設定画面

**Scope**: `/settings`、Sidebar のリンク、メディアフォルダ1項目の表示・編集・状態表示を追加する。

**Dependencies**: 設定 API。

**Acceptance**: 保存だけでは走査せず、入力失敗時も値を保持する。360px・768px・1280px、
キーボード、支援技術で確認し、UI 実装 PR に画像を添付する。
