# Quickstart: 英語化の確認

起動・取り込み・検査の手順は [Running vv](../../docs/how-to/running-vv.md) と
[Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）にある。ここには、この feature
だけの確認を書く。

## 1. 日本語の文字列が残っていないこと

```sh
task check          # 画面の lint（R-3 の no-restricted-syntax）、gosmopolitan、型検査を含む
```

期待: すべて通る。全単位のあと、`web/eslint.config.js` に英語化前のディレクトリの除外の一覧が
残っていない（[research.md R-3](research.md#r-3-訳し漏れは-lint-と型で検出する)）。

## 2. サーバーの出力が英語であること

`MDM_DATA_DIR` を空にしてサーバーを起動し、初回設定、メディアフォルダの登録、取り込み、
存在しないパスの登録、誤ったパスワードでのログインを行ったあと、標準出力の JSON ログと
失敗応答の本文を日本語で検索する。

```sh
grep -P '[\p{Hiragana}\p{Katakana}\p{Han}]' server.log   # 何も出ない
curl -s -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"x","password":"y"}'                    # code と status は変更前と同じ、message は英語
```

利用者が付けた日本語の名前（メディアフォルダのパス、動画名）がログに含まれるのは正しい。

## 3. 取り込みと解析の失敗

- 読めないメディアフォルダ（登録後に `chmod 000` するか、取り外す）で取り込むと、設定画面と
  上部の進捗表示に、そのパスを含む英語の説明が出る（`Scan.errorCode = media_folder_unreadable`）。
- 動画でないファイルを `.mp4` の名前で置いて取り込むと、再生画面の解析失敗の表示が英語の説明に
  なり、ffprobe の出力は画面に出ない（`Video.probeErrorCode = probe_failed`）。

## 4. アップグレード前の日本語の失敗理由

英語化前の版で 3 の失敗を作ったデータディレクトリを、英語化後の版で開く（または SQLite で
`update videos set probe_state='failed', probe_error='ffprobe が失敗しました' where id=…` と
`update scans set state='failed', error='取り込みの途中でアプリケーションが停止しました' where id=…`
を入れる）。

期待: 再生画面と設定画面に英語の一般的な失敗の説明が出て、日本語は画面に出ない。
`GET /api/videos/{id}` の `probeError` と `GET /api/scans/current` の `error` は保存した日本語の
ままで、`probeErrorCode`・`errorCode` は無い。

## 5. 書式と支援技術

- 一覧で 1 本と複数本の結果を出し、件数が `1 video` / `2 videos` の形になる。
- 設定画面の最後の取り込みの日時、再生画面の追加日、一覧の相対時刻が英語の書式で出る。
- スクリーンリーダー（または DevTools の Accessibility ツリー）で、プレイヤーの操作バー、検索欄、
  タグの操作、取り込みの開始ボタンの読み上げ名が英語で、見える文言と一致する。
  `document.documentElement.lang` は `en`。
