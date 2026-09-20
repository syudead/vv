# Implementation Plan: 設定画面

**Branch**: `codex/revise-settings-folder-picker` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

## Summary

設定画面にメディアフォルダ1項目を追加し、複数のサーバーディレクトリを個別resourceとして管理する。
folder pickerから1件ずつ追加・変更・削除し、一覧全体の保存は行わない。新規追加では既存ライブラリを
維持し、既存folderの変更・削除時だけ対象folder由来のDBデータをtransaction内で削除する。
`MDM_MEDIA_DIR` と起動時自動取り込みは廃止する。

## Feature Behavior

- メディアフォルダは0件以上の重複しないresource
- pathは読み取り専用表示とし、文字列入力では変更しない
- folder pickerから1件を選び、追加または既存1件の変更を即時実行する
- 各行の削除も1件の独立した操作として即時実行する
- 一覧全体のdraft、bulk PUT、保存buttonは持たない
- 同一pathと祖先・子孫で探索範囲が重なるpathは登録しない
- 新規追加では既存ライブラリDBへ書き込まない
- 既存folderの変更・削除では対象folder由来のデータだけをatomicに削除する
- folder操作だけでは取り込まず、次の明示走査が登録済み全rootを処理する
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
- 既存folder 1件の更新と、そのfolder由来データの削除を1つのSQLite transactionにする
- 新規追加transactionから既存ライブラリtableへ書き込まない
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

### メディアフォルダ個別操作・対象別無効化・安全な走査

**Scope**:

- ID、path、行単位versionを持つ0件以上のmedia folder rowsをSQLiteへ追加する
- video pathのroot所属判定を共通化し、変更・削除とstream・scannerで同じ境界規則を使う
- 追加、既存path変更、削除を別々のstore transactionとして実装する
- 追加時は既存videos、FTS、jobs、scans、playback progressを変更しない
- 変更・削除時は旧root配下のvideos、FTS、jobs、playback progressだけを削除する
- videos schemaと既存video rowsをmigrationや新規追加で変更しない
- 他folderのデータと完了済みscan履歴を維持する
- 対象thumbnail filesをcommit後にcleanupし、失敗を安全に記録する
- 0件でscanを開始せず、走査は開始時の全root snapshotを使う
- root/directory/file単位のI/O失敗を記録して可能な範囲を継続する
- goroutine境界でpanicを回収し、必ずscanをdone/failedへ確定してrunningを残さない
- `MDM_MEDIA_DIR`、`MDM_SCAN_ON_START`、起動時走査をコード・設定・文書から削除する

**Dependencies**: なし。

**Acceptance**: 新規追加後も既存ライブラリが変わらず、既存1件の変更・削除後はそのfolder由来の
DBデータだけが消える。次の手動走査が全rootを処理し、一部I/O失敗またはpanicでもprocess crashと
永続running scanを残さない。

### メディアフォルダ・ディレクトリ選択 API

**Scope**:

- `GET/POST /api/media-folders`で一覧取得と1件追加を提供する
- `PUT/DELETE /api/media-folders/{id}`で既存1件の変更・削除を提供する
- 一覧全体のPUTを提供しない
- PUT/DELETEは行単位versionで同時変更を検出する
- directory APIはroot/drive、現在位置、親、子directoryだけを返す
- 無効directory、重複・包含、走査中、対象消失、版競合を機械可読errorへ変換する
- OpenAPIからGo/TypeScriptを再生成し、Web API clientとcontract testsを追加する

**Dependencies**: メディアフォルダ個別操作・対象別無効化・安全な走査。

**Acceptance**: folderを1件ずつ取得・追加・変更・削除できる。追加応答後は既存ライブラリが維持され、
変更・削除応答後は対象データだけが消える。すべてのerrorが契約どおり区別される。

### 設定画面と複数サーバーフォルダ選択 UI

**Scope**:

- `/settings` とSidebar navigationを追加する
- 0件以上の登録済みroot一覧と、追加・変更・削除の行単位操作を表示する
- 一覧全体の編集状態と保存buttonを置かない
- folder picker dialogでroot/drive、親、子directoryをkeyboardとpointerで移動する
- 追加は選択確定時に1件POSTし、既存ライブラリが維持されたことを反映する
- 変更・削除は対象データが消えることを事前に示し、対象行だけをPUT/DELETEする
- loading、directory失敗、0件、各行処理中、成功、走査中、版競合を扱う
- Vitest/Testing Library、360px・768px・1280pxの画像、visual reviewで検証する

**Dependencies**: メディアフォルダ・ディレクトリ選択API。

**Acceptance**: 利用者が文字列を入力せず複数rootを1件ずつ管理できる。追加は既存動画を維持し、
変更・削除は対象folderの動画だけを非表示にする。操作からscanは開始しない。
