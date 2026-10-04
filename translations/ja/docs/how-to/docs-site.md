---
source: docs/how-to/docs-site.md
sourceHash: b7b2ca09de097ba9f4d670254c33501be5a9763593a967913c559da214182d50
---

# 文書サイトを扱う {#work-with-the-documentation-site}

`docs/`、`specs/`、`ARCHITECTURE.md` の文書は、`main` へのマージのたびに GitHub Pages に公開される。英語版は <https://syudead.github.io/vv/>、日本語版は <https://syudead.github.io/vv/ja/> 以下にある。日本語のページは `doc-translator` サブエージェントが書いた `translations/ja/` から作られる。その設計は [japanese-translation.md](../design-docs/japanese-translation.md) にある。

エージェントへの指示（`.agents/`、`.claude/`）、`AGENTS.md`、ルートの `README.md` は公開されない。

## 前提条件 {#prerequisites}

- `task setup` で `docs-site/` の依存関係がインストール済みである。

## サイトをローカルで閲覧する {#preview-the-site-locally}

1. 開発サーバーを起動する:

   ```bash
   mise exec --command "task docs"
   ```

2. 表示された URL（<http://localhost:5174/vv/>）を開く。文書の編集はすぐに反映される。

サイドバーはサーバーの起動時にディレクトリ構成から作られる。文書を追加または削除した後や、最初の見出しを変えた後は、`task docs` を再起動する。

CI と同じようにビルドするには、`task docs-build` を実行する。出力は `docs-site/.vitepress/dist/`（バージョン管理外）に出る。

日本語のページは、ビルドのたびにその前に `docs-site/translate/site.mjs` が `translations/ja/` から `ja/`（バージョン管理外）に生成する。翻訳のない文書は、「未翻訳」の注記の下に英語の本文を表示する。

## 公開の仕組み {#how-publishing-works}

`.github/workflows/docs.yml` は、公開対象のパス、`translations/`、`docs-site/` に触れるすべてのプルリクエストと `main` への push で実行される。翻訳ツールをテストし、翻訳を検査し、サイトをビルドする。`main` ではサイトもデプロイする。リポジトリの Settings → Pages で Source を "GitHub Actions" に設定しておく必要がある。

## 変更した文書を翻訳する {#translate-a-changed-document}

変更の英語が確定した後に、同じプルリクエストの中で翻訳する。

1. 変更したパスを `doc-translator` サブエージェントに渡す（例: 「`docs/how-to/docs-site.md` を翻訳して」）。サブエージェントは `translations/ja/<path>` を書き、それに `stamp` を実行する。
2. 文書を削除または名前変更したときは、両方のパスを伝える。サブエージェントが翻訳を削除または移動する。
3. 検査を実行し、翻訳を変更と一緒にコミットする:

   ```bash
   mise exec --command "task docs-test"
   ```

翻訳を手で編集してはならない。言い回しを変えるには、英語の原文か[翻訳規則](../design-docs/japanese-translation.md#translation-rules)を直し、翻訳し直す。

### コマンド {#commands}

| コマンド | 内容 |
| --- | --- |
| `node docs-site/translate/ja.mjs stamp <path>...` | 各見出しの英語のアンカーを固定し、原文のハッシュを記録し、構造を検査する |
| `node docs-site/translate/ja.mjs check` (`task docs-test`) | 壊れた翻訳や原文がなくなった翻訳があると失敗し、古い翻訳には警告を出す |
| `node docs-site/translate/ja.mjs status` (`task docs-ja-status`) | 翻訳が古いか存在しない公開文書を一覧にする |

### 読者に見えるもの {#what-readers-see}

| 状況 | 読者に見えるもの | 解消する方法 |
| --- | --- | --- |
| 翻訳の後に英語の文書が変わった | 古くなっているという注記付きの古い翻訳 | 文書を翻訳し直す |
| 翻訳のない文書 | まだ翻訳されていないという注記付きの英語の本文 | 文書を翻訳する |
| 英語の文書が削除または名前変更された | 古いパスに日本語のページはない。翻訳を削除または移動するまで `check` が失敗する | 翻訳を削除または移動する |

## 執筆上の注意 {#writing-notes}

- 文書間は相対パスでリンクする。`README.md` はそのディレクトリの索引ページ（`/docs/how-to/`）になる。
- 公開対象外のファイル（コード、設定、`ARCHITECTURE.md` 以外のルートの文書）へのリンクは、サイト上では GitHub を指す。
- 壊れたリンクがあるとビルドが失敗する。`http://localhost:…` の例は許される。
- 見出しのアンカーは GitHub の規則に従うので、同じ `#anchor` が GitHub でもサイトでも機能する。日本語のページは英語のアンカーを保つ。
- 文書は [writing-quality.md](../design-docs/writing-quality.md) に従って英語で書く。

## 設定 {#configuration}

サイトの設定は `docs-site/.vitepress/config.mts` にある。

`docs-site/package.json` は vite を 6.4.3 に上書きしている。VitePress 1.6.4 は vite 5 と esbuild 0.21 に依存しており、これらには VitePress の安定版がまだ修正していない既知の脆弱性（GHSA-4w7w-66w2-5vf9 など）がある。修正された VitePress がリリースされたら上書きを削除する。
