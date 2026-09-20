# Implementation Plan: 設定画面

**Branch**: `codex/revise-settings-folder-picker` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

## Summary

設定画面にメディアフォルダ1項目を追加し、その値として複数のサーバーディレクトリを管理する。
初回は0件で、folder pickerから追加・削除する。保存時は旧ライブラリDBデータをtransaction内で
削除し、次の手動走査で全rootから再構築する。`MDM_MEDIA_DIR` と起動時自動取り込みは廃止する。

## Feature Behavior

- メディアフォルダは0件以上の重複しないroot集合
- 現在一覧は読み取り専用で表示し、文字列入力では変更しない
- folder pickerを繰り返し使ってrootを追加し、一覧から削除できる
- 同一pathと祖先・子孫で探索範囲が重なるpathは保存しない
- 一覧の変更保存時にvideos、検索索引、jobs、scans、playback progressを削除する
- 保存だけでは取り込まず、次の明示走査が保存済み全rootを処理する
- 0件では走査を開始できない
- 非同期走査のfilesystem errorは対象単位で記録し、致命的失敗やpanicでもprocessとscan状態を壊さない
- `MDM_MEDIA_DIR` と `MDM_SCAN_ON_START` はコード・設定・文書から削除する

詳細な利用者シナリオと受け入れ条件は [spec.md](spec.md) を正本とする。

## Technical Context

共通定義は再掲しない。

- 構成と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- APIの正本: [api/openapi.yaml](../../api/openapi.yaml)
- 検証コマンド: [Makefile](../../Makefile)
- 技術判断: [research.md](research.md)
- データ差分: [data-model.md](data-model.md)
- API差分: [contracts/settings-api.md](contracts/settings-api.md)

新規外部依存は追加しない。filesystem操作はGo標準ライブラリを使う。

## Constitution Check

- `cmd/mdm` がstore、scanner、httpapi、filesystemを調停し、internalの兄弟依存を増やさない
- 設定一覧の更新と旧DBデータ削除を1つのSQLite transactionにする
- APIはOpenAPIを先に変更し、生成物を手編集しない
- directory APIは選択に必要なdirectory情報だけを公開する
- 設定項目をメディアフォルダ以外へ広げない
- UI実装PRは既存の画像確認手順に従う

設計後も違反なし。

## Project Structure

永続化は `internal/store`、設定・filesystem・走査の調停は `cmd/mdm`、HTTPは
`internal/httpapi`、画面とfolder pickerは `web/src/settings` に置く。

## Complexity Tracking

該当なし。

## Implementation Work

### メディアフォルダ集合の保存・ライブラリ無効化・安全な走査

**Scope**:

- settings revisionと0件以上のmedia folder rowsをSQLiteへ追加する
- 一覧を正規化し、重複・祖先子孫の重なり・無効directoryを拒否する
- 一覧更新と同じtransactionでvideos、FTS、jobs、scans、playback progressを削除する
- 設定変更後の不要thumbnail filesを参照不能にし、cleanup失敗を安全に記録する
- 0件でscanを開始せず、走査は開始時の全root snapshotを使う
- root/directory/file単位のI/O失敗を記録して可能な範囲を継続する
- goroutine境界でpanicを回収し、必ずscanをdone/failedへ確定してrunningを残さない
- `MDM_MEDIA_DIR`、`MDM_SCAN_ON_START`、起動時走査をコード・設定・文書から削除する

**Dependencies**: なし。

**Acceptance**: 一覧変更直後に旧ライブラリDBが空になり、自動走査は始まらない。次の手動走査が
全rootを処理し、一部I/O失敗またはpanicでもprocess crashと永続running scanを残さない。

### 設定・ディレクトリ選択 API

**Scope**:

- settings APIを `mediaFolders: string[]` とversionへ変更する
- PUTは一覧全体を置換し、DB無効化結果を返す
- directory APIはroot/drive、現在位置、親、子directoryだけを返す
- 0件、無効directory、重複・包含、走査中、版競合を機械可読errorへ変換する
- OpenAPIからGo/TypeScriptを再生成し、Web API clientとcontract testsを追加する

**Dependencies**: メディアフォルダ集合の保存・ライブラリ無効化・安全な走査。

**Acceptance**: 複数root一覧を取得・置換でき、directoryだけを階層移動できる。設定変更応答後は
ライブラリが空で、自動scanはない。すべてのerrorが契約どおり区別される。

### 設定画面と複数サーバーフォルダ選択 UI

**Scope**:

- `/settings` とSidebar navigationを追加する
- 0件以上の保存済みroot一覧、追加、削除、未保存変更を表示する
- folder picker dialogでroot/drive、親、子directoryをkeyboardとpointerで移動する
- 同一・重複範囲の候補を追加できない理由を表示する
- 保存後に一覧を空状態へ更新し、手動取り込みが必要と案内する
- loading、directory失敗、0件、保存中、成功、走査中、版競合を扱う
- Vitest/Testing Library、360px・768px・1280pxの画像、visual reviewで検証する

**Dependencies**: 設定・ディレクトリ選択API。

**Acceptance**: 利用者が文字列を入力せず複数rootを管理できる。変更保存後は旧動画が表示されず、
手動走査完了後に全rootの動画が表示される。
