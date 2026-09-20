# Implementation Plan: 設定画面

**Branch**: `codex/revise-settings-folder-picker` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

## Summary

設定画面にメディアフォルダ1項目を追加する。初回は未設定とし、利用者はサーバー上の
ディレクトリをフォルダ選択UIで辿って保存する。設定はSQLiteだけを正本とし、
`MDM_MEDIA_DIR` と起動時自動取り込みを廃止する。

## Feature Behavior

- メディアフォルダは初回未設定。設定完了まで手動取り込みを開始できない
- 現在値は読み取り専用で表示し、文字列入力では変更しない
- 「フォルダを選択」でサーバー上のroot/driveからディレクトリ階層を移動する
- 選択候補は保存時に存在・directory・readableを再検証する
- 保存だけでは取り込まず、次の明示取り込みが新しいフォルダを使う
- 走査中の変更と古いversionからの更新は拒否する
- `MDM_MEDIA_DIR` と `MDM_SCAN_ON_START` は設定、コード、文書から削除する

詳細な利用者シナリオと受け入れ条件は [spec.md](spec.md) を正本とする。

## Technical Context

共通の構成と規則は再掲しない。

- 構成と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- APIの正本: [api/openapi.yaml](../../api/openapi.yaml)
- 検証コマンド: [Makefile](../../Makefile)
- 本機能の判断: [research.md](research.md)
- データ差分: [data-model.md](data-model.md)
- API差分: [contracts/settings-api.md](contracts/settings-api.md)

新規外部依存は追加しない。OS上のdirectory列挙はGo標準ライブラリで実装する。

## Constitution Check

- `cmd/mdm` がstore、scanner、httpapi、OS filesystem操作を調停し、internalの兄弟依存を増やさない
- APIはOpenAPIを先に変更し、生成物を手編集しない
- directory APIはファイル内容を返さず、選択に必要なdirectory情報だけを公開する
- 設定項目をメディアフォルダ以外へ広げない
- UI実装PRは既存の画像確認手順に従う

設計後も違反なし。

## Project Structure

永続化は `internal/store`、設定とfilesystemの調停は `cmd/mdm`、HTTPは `internal/httpapi`、
画面とfolder pickerは `web/src/settings` に置く。既存の所有境界を維持する。

## Complexity Tracking

該当なし。

## Implementation Work

### メディアフォルダ設定の保存と実行時反映

**Scope**:

- nullableなメディアフォルダを持つSQLite singletonとversion付き更新を追加する
- 初回値を未設定にし、`MDM_MEDIA_DIR` の読取、Config field、ログ、文書、開発スクリプトを削除する
- `MDM_SCAN_ON_START` と待受開始時の自動走査も削除する
- 未設定時はscanを開始せず、設定が必要なdomain errorを返す
- 設定更新と走査開始を直列化し、走査は開始時の保存値を固定して使う
- streamは要求時の保存値を配信rootとして検証する
- migration、store、config、startup、scan、streamのテストを追加・更新する

**Dependencies**: なし。

**Acceptance**: 初回は未設定で起動し、環境変数に依存しない。保存や起動では走査せず、
次の手動走査だけが保存済みフォルダを使う。未設定時と走査中の失敗を永続状態を壊さず扱う。

### 設定・ディレクトリ選択 API

**Scope**:

- `GET /api/settings` はnullableな現在値、version、更新時刻を返す
- `PUT /api/settings` はfolder pickerで選択した絶対パスとversionを受け取る
- `GET /api/directories` はpath省略時にfilesystem root/drive、指定時に現在位置・親・子directoryを返す
- ファイルを一覧へ含めず、失われた場所、読取不能、走査中、版競合を機械可読なerrorへ変換する
- OpenAPIからGo/TypeScriptを再生成し、Web API clientを追加する
- settingsとdirectory listingのcontract testsを追加する

**Dependencies**: メディアフォルダ設定の保存と実行時反映。

**Acceptance**: 未設定から選択・保存でき、directoryだけを階層移動できる。無効場所、走査中、
版競合を区別し、生成物とOpenAPIが一致する。

### 設定画面とサーバーフォルダ選択 UI

**Scope**:

- `/settings` とSidebarの実navigationを追加する
- 現在値または未設定を表示し、text inputを置かない
- folder picker dialogでroot/drive、親、子directoryをkeyboardとpointerで移動できるようにする
- 現在表示中のdirectoryを候補として選び、設定画面で明示保存する
- loading、取得失敗、directory失敗、未設定、保存中、成功、走査中、版競合を扱う
- 未設定時はTopBarの取り込みを利用不能にし、設定への導線と理由を示す
- Vitest/Testing Library、360px・768px・1280pxの画像、visual reviewで検証する

**Dependencies**: 設定・ディレクトリ選択API。

**Acceptance**: 利用者がパス文字列を入力せず、サーバー上のdirectoryを選んで保存できる。
未設定・失敗・競合でも現在値や候補を失わず、保存だけでは取り込みを開始しない。
