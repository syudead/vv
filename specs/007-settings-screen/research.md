# Research: 設定画面

共通の技術選定は [ARCHITECTURE.md](../../ARCHITECTURE.md) と
[技術選定文書](../../docs/design-docs/tech-stack-selection.md)を引き継ぐ。

## R-701: SQLiteだけを設定の正本にする

**Decision**: メディアフォルダ集合は初回0件でSQLiteに保持する。`MDM_MEDIA_DIR` は移行元や
初期値としても使わない。

**Rationale**: UI設定と環境変数という2つの正本を残さないため。

## R-702: 複数rootを集合として保存する

**Decision**: settings revisionと複数のmedia folder rowsを持つ。同じ正規化pathと、祖先・子孫で
探索範囲が重なるpathは拒否する。表示順は保存するが、同一性はpathで判定する。

## R-703: 設定変更で旧ライブラリDBを削除する

**Decision**: folder集合の置換、version更新、videos、検索索引、jobs、scans、playback progressの
削除を1つのSQLite transactionで行う。設定変更後のライブラリは空で、手動走査だけが再構築する。

**Rationale**: 新しい探索範囲に対して旧DBデータを有効な情報として残さないため。

thumbnail filesはDB参照削除後にbest-effortでcleanupする。cleanup errorは記録するが、DB transactionを
巻き戻さず、参照されない旧thumbnailが画面へ出ないことを優先する。

## R-704: サーバー側directory browserを提供する

**Decision**: APIが返すserver filesystemのdirectoryを辿るfolder pickerを実装する。path省略時は
Linux rootまたはWindows driveを返し、以後は親と直下directoryを返す。ファイルを返さない。

## R-705: 選択時と保存時の両方で検証する

**Decision**: listing時に読取可能性を確認し、PUT時にも全rootの存在・directory・readableと
重複・包含を再検証する。API pathはOSの絶対・正規化済み表現とする。

## R-706: 非同期走査を失敗境界で閉じる

**Decision**:

- scan開始時にfolder集合をsnapshotする
- rootを1件ずつ処理し、1rootの失敗で残りrootを止めない
- directory/file単位の消失・permission・stat/open失敗をfailed countとlogへ記録して継続する
- DB更新など継続不能なerrorはscan全体をfailedにする
- goroutine最上位でdeferによるfinalizeとpanic recoveryを行う
- panic/error/cancelの全出口でrunning scanをdoneまたはfailedへ確定し、processへpanicを伝播させない

**Rationale**: 稀なfilesystem競合や予期しないpanicでserver processと次回scanを利用不能にしないため。

## R-707: versionで同時更新を検出する

**Decision**: 設定集合に整数versionを持たせ、PUTの条件付き更新で古い画面からの保存を409にする。
