# Quickstart: 設定画面を検証する

共通コマンドは [Makefile](../../Makefile)、UI確認は
[UI画像手順](../../docs/how-to/ui-change-screenshots.md)を使う。

## Automated

```bash
make check
```

## Acceptance Scenarios

1. 初回起動でfolder一覧が0件になり、環境変数は初期値へ影響しない。
2. folder pickerを繰り返し使い、複数rootを追加・削除できる。
3. 同一pathと祖先・子孫で重なるpathを同時保存できない。
4. folder集合の保存と同じtransactionでvideos、FTS、jobs、scans、playback progressが消える。
5. 保存直後の一覧は空で、scanは自動開始されない。
6. 次の手動scanが保存済み全rootを処理する。
7. 1rootまたは一部entryを走査中に削除・読取不能にしても、失敗を記録して残りを処理する。
8. scannerでpanicを発生させてもserver processが継続し、scanがfailedになりrunningを残さない。
9. settings transactionを失敗させるとfolder集合と旧ライブラリDBの両方が元のまま残る。
10. 360px・768px・1280pxで複数folder一覧とpickerをkeyboard操作できる。

UI実装PRに0件、複数件、picker、重複error、保存後の空一覧、走査中の画像とvisual reviewを記録する。
