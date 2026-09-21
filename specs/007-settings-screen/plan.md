# Implementation Plan: 設定画面

**Branch**: `codex/revise-settings-folder-picker` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

## Summary

設定画面にメディアフォルダ1項目を追加し、複数のサーバーディレクトリを個別resourceとして管理する。
folder pickerから1件ずつ追加・変更・削除し、一覧全体の保存は行わない。新規追加では既存ライブラリを
維持し、既存folderの変更・削除時だけ対象root配下のvideo locationsをtransaction内で削除する。
`MDM_MEDIA_DIR` と起動時自動取り込みは廃止する。

既存構成と依存方向は [ARCHITECTURE.md](../../ARCHITECTURE.md)、要求は [spec.md](spec.md)、
技術判断は [research.md](research.md)、データ差分は [data-model.md](data-model.md)、API差分は
[contracts/settings-api.md](contracts/settings-api.md) を正本とする。実装進捗と検証結果は
[docs/exec-plans/completed/013-settings-screen.md](../../docs/exec-plans/completed/013-settings-screen.md) に記録する。

## Structural Decisions

- **folderは個別resourceにする**: POST/PUT/DELETEを1件ずつatomicに実行する。一覧全体のdraftと
  bulk PUTは、無関係なfolderまで競合・削除対象にし、今回の操作単位と一致しないため採用しない。
- **動画と実在場所を分離する**: `videos`をcontent単位、`video_locations`をpath単位にする。videoの
  単一pathを書き換える案は、同じcontentが複数rootにある事実を失い、片方の削除で未変更root側の
  動画まで消すため採用しない。
- **利用者データをfolder操作から分離する**: folder変更・削除で消すのは対象locationsと、locationが
  0件になった動画の再構築可能データだけとする。`playback_progress`まで消す案は再scanで復元できない
  利用者操作を失うため採用しない。
- **jobを処理元locationへ結び付ける**: video/contentに加えてlocation ID/version/pathを検証する。
  video identityだけを検証する案は、削除済みlocationのI/O失敗を残存videoへ書き込めるため採用しない。
- **thumbnail cacheをfolder操作で削除しない**: orphan fileは将来の参照確認付きGCへ任せる。commit後の
  best-effort削除は、同じcontentの再登録が生成した同名fileを消せるため採用しない。
- **既存のtrusted-network境界を維持する**: directory APIとmutationはsame-originかつJSONに限定する。
  このfeatureだけに認証を新設する案は既存APIと異なる境界を作るため採用せず、認証導入は別featureとする。

## Implementation Work

### メディアフォルダ個別操作・location model・安全な走査

**Scope**:

- ID、path、行単位versionを持つ0件以上のmedia folder rowsをSQLiteへ追加する
- `videos`からpath、title、size、mtimeを`video_locations`へ移すmigrationを追加し、既存video ID、
  content key、probe結果、jobs、playback progressを保持する
- location pathのroot所属判定を共通化し、folder変更・削除、stream、scannerで同じ境界規則を使う
- 追加、既存path変更、削除を別々のstore transactionとして実装する
- 追加時は既存videos、locations、FTS、jobs、scans、playback progressを変更しない
- 変更・削除時は旧root配下のlocationsを削除し、locationが0件になったvideos、FTS、jobsだけを削除する
- 別rootのlocationが残るvideo、job、thumbnailと、全`playback_progress`、scan履歴を維持する
- APIでは登録root内のlocationをpath順で選び、streamは同順で最初の利用可能なfileを使う
- locationのtitle/pathを検索し、結果をvideo単位で重複排除する
- location IDを再利用せず、content・所属・file fingerprint変更時にlocation versionを加算する
- workerが選んだvideo/content/location ID/version/pathの全identityをwrite-back前に再確認する
- 消失・変更locationの結果を破棄して残存locationへ再queueし、location固有I/O失敗をvideo失敗にしない
- content key名のthumbnail fileはfolder操作で削除せず、参照確認付きGCを別機能へ分離する
- scannerは同じcontentの別pathを既存locationの移動ではなく追加locationとしてupsertする
- 0件でscanを開始せず、走査は開始時の全root snapshotを使う
- root/directory/file単位のI/O失敗を記録し、列挙失敗範囲をmissing location削除から除外する
- goroutine境界でpanicを回収し、必ずscanをdone/failedへ確定してrunningを残さない
- `MDM_MEDIA_DIR`、`MDM_SCAN_ON_START`、起動時走査をコード・設定・現行文書から削除する

**Dependencies**: なし。

**Acceptance**: migrationで既存データが維持され、新規追加後も既存ライブラリが変わらない。既存1件の
変更・削除後は対象locationsとorphan動画だけが消える。同じcontentが別rootに残る場合は動画を引き続き
一覧・検索・再生できる。削除locationのjob結果とfolder操作が残存video・thumbnailを壊さない。手動走査は
一部I/O失敗で既存locationを誤削除せず、panicでもprocess crashと永続running scanを残さない。

### メディアフォルダ・ディレクトリ選択 API

**Scope**:

- `GET/POST /api/media-folders`で一覧取得と1件追加を提供する
- `PUT/DELETE /api/media-folders/{id}`で既存1件の変更・削除を提供し、一覧全体のPUTは提供しない
- PUT/DELETEは行単位versionで同時変更を検出する
- directory APIはroot/drive、現在位置、親、子directoryだけを返す
- directory listingとPOST/PUTはsymlinkを拒否し、filesystem/drive rootは登録可能にする
- 無効directory、symlink、重複・包含、走査中、対象消失、版競合を機械可読errorへ変換する
- 全mutationをsame-originに限定し、POST/PUTはJSONだけを受理してCORS responseを追加しない
- OpenAPIを先に変更し、Go/TypeScriptを再生成してhandler、Web API client、contract testsを追加する

**Dependencies**: メディアフォルダ個別操作・location model・安全な走査。

**Acceptance**: folderを1件ずつ取得・追加・変更・削除できる。追加応答後は既存ライブラリが維持され、
変更・削除応答後は対象locationsだけが消え、別rootのlocationが残る動画は利用できる。すべてのerrorが
契約どおり区別される。

### 設定画面と複数サーバーフォルダ選択 UI

**Scope**:

- `/settings` とSidebar navigationを追加する
- 0件以上の登録済みroot一覧と、追加・変更・削除の行単位操作を表示する
- 一覧全体の編集状態と保存buttonを置かない
- folder picker dialogでroot/drive、親、子directoryをkeyboardとpointerで移動する
- 追加は選択確定時に1件POSTし、既存ライブラリを維持する
- 変更・削除は対象rootのlocationが消えることを事前に示し、対象行だけをPUT/DELETEする
- loading、directory失敗、0件、各行処理中、成功、走査中、版競合を扱う
- Vitest/Testing Library、360px・768px・1280pxの画像、visual reviewで検証する

**Dependencies**: メディアフォルダ・ディレクトリ選択API。

**Acceptance**: 利用者が文字列を入力せず複数rootを1件ずつ管理できる。追加は既存動画を維持し、
変更・削除は対象rootのlocationだけを非表示にする。別rootのlocationが残る動画は表示を維持し、
操作からscanは開始しない。
