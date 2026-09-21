# Quickstart: 設定画面を検証する

共通コマンドは [Taskfile.yml](../../Taskfile.yml)、UI確認は
[UI画像手順](../../docs/how-to/ui-change-screenshots.md)を使う。

## Automated

```bash
task check
task test-e2e
```

`task test-e2e`はGoサーバー、Vite proxy、Chromiumを起動し、設定画面から実際に
メディアフォルダを追加してsame-origin境界を含むbrowser-to-API経路を検証する。

## Acceptance Scenarios

1. 初回起動でfolder一覧が0件になり、環境変数は初期値へ影響しない。
2. folder pickerから複数rootを1件ずつ追加・変更・削除でき、bulk保存操作がない。
3. 同一pathと祖先・子孫で重なるpathを登録できない。
4. filesystem/drive rootはpickerで確定して登録でき、symlinkはpickerとAPIで拒否される。
5. 新規folderを追加しても既存videos、FTS、jobs、scans、playback progressが変わらない。
6. migrationで既存videoのpath、title、size、mtimeが1件のlocationへ移り、video ID、content key、
   probe結果、jobs、playback progressが維持される。
7. 既存folderを変更すると、path更新と旧root配下のlocations削除が同じtransactionで完了し、
   locationが0件になったvideos、FTS、jobsだけが削除される。
8. 同じ内容をroot Aとroot Bに置いて取り込んだあとroot Bを変更・削除しても、root A側のlocation、
   video、job、thumbnailが残り、一覧表示とstreamを継続できる。
9. 変更・削除transactionを失敗させると対象folderとライブラリDBの双方が元のまま残る。
10. root A/BのうちBをjob処理中に削除しても、B由来のI/O失敗を残存videoへ書き込まず、Aへ再試行する。
11. folder操作の直後にscanは自動開始されず、次の手動scanが登録済み全rootを処理する。
12. rootまたはsubtreeを走査中に読取不能にしても、その範囲の既存videoを保持して残りを処理する。
13. scannerでpanicを発生させてもserver processが継続し、scanがfailedになりrunningを残さない。
14. cross-origin mutationとJSON以外のPOST/PUTを拒否し、CORS responseを返さない。
15. 360px・768px・1280pxで複数folder一覧とpickerをkeyboard操作できる。
16. 複数locationのtitle/path検索結果はvideo単位で重複せず、利用可能なlocationから再生できる。
17. orphan video削除直後に同じcontentを別rootから取り込んでも、保持または再生成したthumbnailが
    folder操作によって削除されず、thumbnail APIが404にならない。

UI実装PRに0件、複数件、picker、重複error、追加成功、変更・削除警告、対象データ削除後、
走査中の画像とvisual reviewを記録する。
