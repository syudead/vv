# 文書サイト

`docs/` と `specs/` の文書は、`main` に入るたびに GitHub Pages へ公開される。

- 公開先: <https://syudead.github.io/vv/>
- 公開するのは `docs/` と `specs/` だけで、リポジトリ直下の文書や `.agents/`・`.claude/`
  などのエージェント向けの文書は出さない。

## 手元で見る

```bash
mise exec --command "task docs"
```

表示された URL（<http://localhost:5174/vv/>）を開く。文書を直すと、その場で表示が
変わる。初回は `task setup`（`docs-site/` の依存を入れる）が必要である。

CI と同じ作り方で確かめるときは `task docs-build` を使う。出力は
`docs-site/.vitepress/dist/` に出る（版管理の外）。

## 公開の流れ

`.github/workflows/docs.yml` が行う。

- PR: `docs/`・`specs/`・`docs-site/` を変えたときに、サイトを作れることだけを確かめる。
  公開はしない。
- `main` への push: サイトを作り、GitHub Pages へ公開する。
- 公開には、リポジトリの Settings → Pages で Source を「GitHub Actions」にしておく
  必要がある。

## 書き方の注意

- 文書どうしのリンクは今までどおり相対パスで書く。`README.md` はそのディレクトリの
  入口ページ（`/docs/how-to/` など）になる。
- 公開範囲の外（ソース、設定ファイル、リポジトリ直下の文書）へのリンクは、サイトでは
  GitHub の表示へ向く。
- リンク先が見つからないとサイトの作成が失敗する。`http://localhost:…` の例は例外として
  許している。
- 見出しの anchor は GitHub と同じ規則で作るので、`#r-3-…` のような日本語の見出しへの
  リンクも GitHub とサイトの両方で通る。
- サイドバーはディレクトリの構成から自動で作る。項目の名前は各文書の見出し1である。
  サイドバーは `task docs` の起動時に作るので、起動中に文書を足したり消したり、
  見出し1を変えたりしたときは、`task docs` を起動し直すと一覧に反映される（本文の
  変更はその場で反映される）。

設定は `docs-site/.vitepress/config.mts` にある。

`docs-site/package.json` の `overrides` で vite を 6.4.3 に上げている。VitePress 1.6.4 が
依存する vite 5 系と esbuild 0.21 には既知の脆弱性（GHSA-4w7w-66w2-5vf9 など）があり、
VitePress の安定版では直っていないためである。VitePress の修正版が出たら外す。
