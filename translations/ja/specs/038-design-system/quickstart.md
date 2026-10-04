---
source: specs/038-design-system/quickstart.md
sourceHash: 9b8855d4dfa7e16b99a187fe2937913afcbdd5b32760d1a668f7ec443900c6e0
---

# クイックスタート: デザインシステムを端から端まで確認する {#quickstart-check-the-design-system-end-to-end}

この手順は、親 Issue #757 の受け入れ条件 1 から 4 と 6 のうち、`task check` と `task test-e2e` だけでは示せないことを証明する。確認の記録があること、レジストリが shadcn を通じて応答すること、エージェントが `AGENTS.md` からレジストリを見つけること、違反の種類ごとに失敗すること、5 つの画面が操作を保つこと、である。条件 5 は、最終的な feature ブランチで `task check` と `task test-e2e` が通ることである。

## 前提条件 {#prerequisites}

- 最後の実装単位の後の feature ブランチをチェックアウトし、`npm --prefix web ci` を実行してあること ([docs/how-to/development.md](../../docs/how-to/development.md))。
- 手順 6 には、`task dev` と同様に、動画のフォルダ、タグ、重複のまとまりをそれぞれ 1 つ以上持つライブラリ。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| 1. feature ブランチへの基礎、コンポーネント、ページパターンの PR を開く | それぞれに保守者の承認レビューと、1440 px と 390 px の `/design-system` のスクリーンショットがある | 1 |
| 2. `web/` で `npx shadcn view ./registry/r/vv.json` を実行し、次に `npx shadcn view ./registry/r/button.json` を実行する | 1 つ目はすべてのアイテムを挙げる。2 つ目は Button のソースと、`components.md` 内の節を指す `docs` 行を出力する | 2 |
| 3. リポジトリでの Claude Code のセッションで、shadcn の MCP ツール `view_items_in_registries` を `./registry/r/vv-rules.json` で呼ぶ | 3 つの規則ファイルが返る | 2 |
| 4. 新しいエージェントのセッションを始め、ファイル名を出さずに、画面を作る前にどこを見るかを尋ねる | `AGENTS.md` から `docs/design-docs/design-system.md` を挙げ、そこからレジストリを挙げる | 3 |
| 5. `web/src/ui` の外の任意の画面ファイルに、`<button>`、`className="h-[3px]"`、`className="p-7"`、`className="text-[#fff]"` を 1 つずつ加え、そのたびに `task lint-web` と `task test-web` を実行して元に戻す | 各実行が [contracts/registry.md](contracts/registry.md#check-rules) のメッセージで失敗する | 4 |
| 6. `task dev` で、各画面の移行 PR の本文にある操作リストを順にたどる (ライブラリのものはページパターンの PR にある) | `main` で動くと記録された各操作が、引き続き動く | 6 |
