# 契約: `sdd-guard.sh`（ガード）

**Feature**: [spec.md](../spec.md) | **Data model**: [data-model.md](../data-model.md) | **Research**: [R-002](../research.md)

場所: `.claude/skills/sdd-next/scripts/sdd-guard.sh`

## 呼び出し

```bash
sdd-state.sh | sdd-guard.sh [--repo <owner/name>] [--root <repo_root>] [--github-dir <dir>]
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| stdin | 必須 | `sdd-state.sh` の出力（自動選択の結果） |
| `--github-dir` | 無し | PR 一覧を `<dir>/pulls-closed.json` と `<dir>/pulls-open.json` から読む。cloud セッションでは必ず指定する |
| `--repo` | `git remote get-url origin` から導出 | GitHub の `owner/name`。`--github-dir` 無しのローカル検算フォールバックでだけ使う |
| `--root` | カレントディレクトリ | git 履歴を読み、`sdd-state.sh --feature` を呼び直すときに渡す |

### PR 一覧の取得元

cloud セッションには `gh` が無い（2026-09-13 のプローブで確認）。そこでスキルが組み込みの
GitHub ツールで一覧を取り、ファイルに置いてから `--github-dir` で渡す。`gh api` は手元の
検算用フォールバックであり、cloud の通常経路では使わない。どちらの経路でも判定は同じである。

| ファイル | 中身 | 使う項目 |
| --- | --- | --- |
| `pulls-closed.json` | base を限定しない closed PR の配列（更新日時の降順、必要なページをすべて連結） | `number`、`head.ref`、`labels`（`["sdd"]` と `[{"name":"sdd"}]` の両方を受ける） |
| `pulls-open.json` | base を限定しない open PR の配列（必要なページをすべて連結） | `head.ref`、`base.ref`、`labels` |

**マージ済みかどうかは API の `merged_at` を見ない。** `--root` の git 履歴（HEAD の
first-parent）に `Merge pull request #N` か `(#N)` の件名があれば、PR `N` はマージ済みと
みなす。変更ファイルも同じコミットの差分から取る。取得元によって PR オブジェクトの
項目が違っても判定が変わらないようにするためで、`GET /pulls/{n}/files` は呼ばない。

## 出力

標準出力に JSON 1 行（[data-model.md 6.](../data-model.md)）。`state` は対象機能を確定し直した
後の State で、呼び出し側はこちらを以後の真実として使う。

依存: bash、git、`jq`、同じディレクトリの `sdd-state.sh`。cloud の通常経路では `gh` は不要。
`--github-dir` が無いローカル検算フォールバックでだけ `gh`（`gh api` のみ）を使う。

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた（`go` の真偽を問わない） |
| 2 | stdin が JSON でない、引数の誤り、`--github-dir` のファイルが無い・配列でない |

`jq` が無い、または（`--github-dir` 無しで）`gh` が無い・REST が失敗した場合も終了コード 0 で
`{"go":false,"reason":"gh-unavailable",...}` を返す（手元で検算できるようにするため。spec の
Edge Cases）。

## 手順（この順で最初に該当したものを返す）

| # | 判定 | データ | 結果 |
| --- | --- | --- | --- |
| 0 | PR 一覧が取れるか | cloud は `--github-dir` の 2 ファイル。ローカル検算フォールバックだけ `GET /rate_limit` → `GET /repos/{o}/{r}/pulls?state=closed&sort=updated&direction=desc&per_page=100&page=N` と `GET /repos/{o}/{r}/pulls?state=open&sort=updated&direction=desc&per_page=100&page=N` を 100 件未満のページまで取得 | `jq` 無し・`gh` 無し・REST 失敗・配列でない応答 → `gh-unavailable` |
| 1 | **対象機能の確定**: closed 一覧のうち label `sdd` で、かつ git 履歴にマージされている PR を新しい順に並べ、先頭の変更ファイルから `specs/NNN-*/` を抽出。見つかれば `sdd-state.sh --feature` で再判定 | `git log --first-parent HEAD` の件名（`Merge pull request #N` / `(#N)`）と `git diff --name-only <c>^1 <c>` | `state` を置き換える。見つからなければ stdin の state のまま |
| 2 | `state.stage` が `done` または `none` | — | `go:false`, `reason:"nothing-to-do"` |
| 3 | **冪等**: open PR のうち `head.ref` が `claude/sdd-NNN-` で始まり、`base.ref == claude/sdd-NNN-feature` のものがある。label 付与失敗で `sdd` が無くても塞ぐ。同 prefix で `base.ref` が無い要素は段階 PR と最終 PR を区別できないため `gh-unavailable` | open 一覧 | `go:false`, `reason:"open-pr"`, `open_prs:[...]` |
| 4 | **フェーズ別リトライ**: `stage = implement` で、`head.ref = claude/sdd-NNN-implement-pN` のマージ済み `sdd` PR が 2 件以上 | 1 で作った一覧を再利用 | `go:false`, `reason:"phase-retry-limit"` |
| 5 | **ホップ上限**: standard は `2 + phases + 2`、UI workflow は design 分を加えた `3 + phases + 2` 件以上 | 同上 | `go:false`, `reason:"hop-limit"` |
| 6 | 上記に該当しない | — | `go:true` |

`phases` は `state.phases`（無ければ 0）。

## 注意

- cloud セッションでは `gh` を前提にしない。高水準の `gh pr list` / `gh issue list` は
  GraphQL や認証の差で失敗しうるため使わない（R-002）
- `per_page=100` を超える通常 PR があっても、cloud 側はページを連結してから渡し、ローカル
  フォールバックも 100 件未満のページまで取得する
- git 履歴は `--root` の HEAD を読む。スキルは feature branch を作業 base に復元してから呼ぶため、段階 PR の履歴は feature branch の first-parent で数える。
  shallow clone で件名が読めない範囲のマージは数えない
- 出力の `open_prs` は `head.ref` の配列。空なら `[]`

## 例

```bash
$ sdd-state.sh | sdd-guard.sh
{"go":true,"state":{"feature_dir":"specs/002-core-video-library","feature":"002","stage":"plan","branch":"claude/sdd-002-plan"},"hops":0,"phase_retries":0,"open_prs":[]}

$ sdd-state.sh | sdd-guard.sh     # 手元（gh 無し）
{"go":false,"reason":"gh-unavailable","state":{"feature_dir":"specs/002-core-video-library","feature":"002","stage":"plan","branch":"claude/sdd-002-plan"}}
```

## テスト

自動テスト（`tests/run.sh`）は次を確認する。

- `PATH` から `gh` を外した状態、または `gh api` の一覧取得が失敗した状態で
  `gh-unavailable` を返し、`state` を素通しする
- `--github-dir` で一覧を渡し、フィクスチャを写した一時 git リポジトリにマージコミットを
  積んで、`go:true`（ホップ数）・`open-pr`・ラベル無し open PR も塞ぐこと・`base.ref`
  欠落時に fail-closed すること・
  `phase-retry-limit`・対象機能の確定
  （`nothing-to-do`）・未マージやラベル無しを数えないこと・ファイル無しの終了コード 2

`--github-dir` のテストは `jq` と git が要り、無ければ SKIP になる（CI では走る）。
組み込み GitHub ツールの応答の形は quickstart の S3〜S6 で実機確認する。

## 2026-09-13 feature branch 方式への変更

PR 一覧は base を限定せず取得する。段階 PR の冪等判定は `head.ref` の prefix に加えて
`base.ref == claude/sdd-NNN-feature` を要求する。これにより、同じ head prefix を持つ
feature branch → main の最終 PR は段階実行を塞がない。`base.ref` が無い open PR は
区別不能なので fail-closed にする。ガードは対象 feature branch を checkout した状態で呼び、
first-parent 履歴から段階 hop を数える。
