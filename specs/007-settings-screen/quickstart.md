# Quickstart: 設定画面を検証する

共通コマンドは [Makefile](../../Makefile)、UI確認は
[UI画像手順](../../docs/how-to/ui-change-screenshots.md)を使う。

## Automated

```bash
make check
```

## Acceptance Scenarios

1. 初回起動でfolder一覧が0件になり、環境変数は初期値へ影響しない。
2. folder pickerから複数rootを1件ずつ追加・変更・削除でき、bulk保存操作がない。
3. 同一pathと祖先・子孫で重なるpathを登録できない。
4. 新規folderを追加しても既存videos、FTS、jobs、scans、playback progressが変わらない。
5. videos schemaと既存video rowsはmigrationと新規folder追加で変更されない。
6. 既存folderを変更すると、path更新と旧root配下データの削除が同じtransactionで完了する。
7. 既存folderを削除すると、旧root配下データだけが消え、他folderのデータとscan履歴が残る。
8. 変更・削除transactionを失敗させると対象folderとライブラリDBの双方が元のまま残る。
9. folder操作の直後にscanは自動開始されず、次の手動scanが登録済み全rootを処理する。
10. 1rootまたは一部entryを走査中に削除・読取不能にしても、失敗を記録して残りを処理する。
11. scannerでpanicを発生させてもserver processが継続し、scanがfailedになりrunningを残さない。
12. 360px・768px・1280pxで複数folder一覧とpickerをkeyboard操作できる。

UI実装PRに0件、複数件、picker、重複error、追加成功、変更・削除警告、対象データ削除後、
走査中の画像とvisual reviewを記録する。
