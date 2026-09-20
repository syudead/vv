# Quickstart: 設定画面を検証する

共通コマンドは [Makefile](../../Makefile)、UI確認は
[UI画像手順](../../docs/how-to/ui-change-screenshots.md)を使う。

## Automated

```bash
make check
```

## Acceptance Scenarios

1. `MDM_MEDIA_DIR` なしで初回起動し、設定画面が未設定を表示する。
2. `MDM_MEDIA_DIR` に任意の値を置いても、初回値と実行時動作に影響しない。
3. フォルダ選択UIを開き、root/drive、親、子directoryを辿れる。ファイルは表示されない。
4. 選択した空directoryを保存し、再読み込み・再起動後も維持される。
5. 保存前に候補を削除または読取不能にすると、現在値を変えず選び直しを促す。
6. 未設定時は手動scanを開始できず、保存時と起動時にもscanが作られない。
7. 保存後の手動scanだけが選択済みdirectoryを処理する。
8. running scan中の保存と古いversionからの保存を409で拒否する。
9. 360px・768px・1280pxでpickerをkeyboard操作でき、重なりや横scrollがない。

UI実装PRに通常、未設定、directory取得失敗、走査中の画像とvisual reviewを記録する。
