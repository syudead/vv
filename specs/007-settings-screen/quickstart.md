# Quickstart: 設定画面を検証する

共通の検証コマンドと起動方法は [Makefile](../../Makefile) と
[UI画像手順](../../docs/how-to/ui-change-screenshots.md)を使う。

## Automated

```bash
make check
```

## Acceptance Scenarios

1. `MDM_MEDIA_DIR=A` で初回起動し、設定画面で `A` が表示される。
2. 有効な `B` を保存して再起動し、環境変数が `A` のままでも `B` が表示される。
3. 保存時と起動時に scan が作られず、手動取り込みだけが `B` を走査する。
4. 相対パス、存在しないパス、通常ファイル、読取不能な場所を保存できない。
5. running scan 中の保存と、古い version からの保存を `409` で拒否する。
6. `B` の保存直後、旧ルート `A` の動画を配信しない。手動走査後の一覧は `B` と一致する。
7. 360px・768px・1280pxで重なりや横スクロールがなく、キーボードと支援技術で保存できる。

UI 実装 PR には手順7の画像と visual review を記録する。
