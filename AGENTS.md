# Agent guide

この文書は、このリポジトリで作業する人とコーディングエージェントのための短い地図である。
細かい知識はこの文書に足さず、`docs/` の該当する文書か、コードのそばに置く。

## まず読むもの

1. [ARCHITECTURE.md](ARCHITECTURE.md) — 層の境界と依存の向き
2. [README.md](README.md) — 起動方法、設定、開発コマンド
3. [docs/design-docs/](docs/design-docs/) — 結論だけでは分からない技術判断の経緯

## 進め方

- GitHub Issue が要求の置き場である。仕様を別ファイルに書き写さない。
- 変更は焦点を絞り、レビューできる大きさにする。
- `task check` が通ること。CI も同じタスクを呼ぶので、手元で再現できる。
- feature branch へ push したら `main` への pull request を開く。

## 文書の扱い

- 文書はコードのそばに置き、振る舞いを変える変更と**同じ変更単位で**更新する。
  古い文書は、無い文書より害が大きい。
- 判断の経緯を残す価値があるものだけ `docs/design-docs/` に足し、
  [docs/design-docs/index.md](docs/design-docs/index.md) にリンクを足す。
- 生成物（`internal/httpapi/gen/`・`web/src/api/gen/`）は手編集しない。
  `api/openapi.yaml` を直して `task generate` で作り直す。
- 画面の見た目や振る舞いが変わる変更では、結果の画像を pull request に添える。
  変わらないときは「UI 変更なし」と書く。撮り方は
  [docs/how-to/ui-change-screenshots.md](docs/how-to/ui-change-screenshots.md)。
