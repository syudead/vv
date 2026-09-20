# Research: 設定画面

共通の技術選定は [ARCHITECTURE.md](../../ARCHITECTURE.md) と
[技術選定文書](../../docs/design-docs/tech-stack-selection.md)を引き継ぐ。

## R-701: SQLiteだけを設定の正本にする

**Decision**: メディアフォルダは初回0件でSQLiteに保持する。`MDM_MEDIA_DIR` は移行元や初期値としても
使わない。

**Rationale**: UI設定と環境変数という2つの正本を残さないため。

## R-702: MediaFolderを個別リソースとして保存する

**Decision**: 各MediaFolderにID、path、行単位versionを持たせ、追加・変更・削除をそれぞれ1件の
atomic operationにする。一覧全体のPUT、draft、global version、並び替えは実装しない。同じ
正規化pathと、祖先・子孫で探索範囲が重なるpathは拒否する。

**Rationale**: 1件の操作に無関係なfolderを更新対象にせず、競合と失敗の範囲を対象行へ限定するため。

## R-703: DB無効化を既存folderの変更・削除だけに限定する

**Decision**:

- 新規追加はMediaFolderをinsertするだけで、既存ライブラリDBを変更しない
- 既存folderのpath変更は、そのfolder由来のDBデータ削除とpath更新を1transactionで行う
- 削除は、そのfolder由来のDBデータ削除とfolder行削除を1transactionで行う
- 他folder由来の動画、job、再生状態と完了済みscan履歴は変更しない

対象videoは、正規化済みpathが旧root配下にあるかをOS規則に従う共通helperで判定する。
`videos`へfolder IDを追加せず、migrationや新規追加で既存videoを更新しない。対象videosの削除で
FTSとjobsをcascadeし、対象content keyのplayback progressもtransaction内で削除する。
thumbnail filesはcommit後にbest-effortでcleanupする。

**Rationale**: 追加済みfolderのデータは新規rootを増やしても有効であり、全削除する理由がない。

## R-704: サーバー側directory browserを提供する

**Decision**: APIが返すserver filesystemのdirectoryを辿るfolder pickerを実装する。path省略時は
Linux rootまたはWindows driveを返し、以後は親と直下directoryを返す。ファイルを返さない。

## R-705: 選択時と操作実行時の両方で検証する

**Decision**: listing時に読取可能性を確認し、POST/PUT時にも対象pathの存在・directory・readableと
全既存folderに対する重複・包含を再検証する。API pathはOSの絶対・正規化済み表現とする。

## R-706: 非同期走査を失敗境界で閉じる

**Decision**:

- scan開始時にfolder一覧をsnapshotする
- rootを1件ずつ処理し、1rootの失敗で残りrootを止めない
- directory/file単位の消失・permission・stat/open失敗をfailed countとlogへ記録して継続する
- DB更新など継続不能なerrorはscan全体をfailedにする
- goroutine最上位でdeferによるfinalizeとpanic recoveryを行う
- panic/error/cancelの全出口でrunning scanをdoneまたはfailedへ確定し、processへpanicを伝播させない

**Rationale**: 稀なfilesystem競合や予期しないpanicでserver processと次回scanを利用不能にしないため。

## R-707: 行単位versionで同時変更を検出する

**Decision**: PUTとDELETEは対象MediaFolderのexpected versionを要求し、同じ行への古い操作を409にする。
POSTはpathのunique・overlap検証で競合を処理する。
