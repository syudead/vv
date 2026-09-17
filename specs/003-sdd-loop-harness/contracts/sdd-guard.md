# 契約: `sdd-guard.sh`（ガード）

**Feature**: [spec.md](../spec.md) | **Data model**: [data-model.md](../data-model.md) | **Research**: [R-002](../research.md)

場所: `.claude/skills/sdd-next/scripts/sdd-guard.sh`

## 呼び出し

```bash
sdd-state.sh | sdd-guard.sh [--root <repo_root>] [--remote <name>]
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| stdin | 必須 | `sdd-state.sh` の出力（確定済みの対象 feature） |
| `--root` | カレントディレクトリ | 判定に使う git リポジトリ |
| `--remote` | `origin` | 段階 branch と feature branch の有無を問い合わせる remote |

### 判定に使うデータ（git だけ）

GitHub API は使わない。2026-09-17 までは組み込み GitHub ツールで取った PR 一覧を
`--github-dir` で渡していたが、応答が大きいとツール結果が `~/.claude/projects/` 配下に
スプールされ、それを Bash で触った時点で sandbox の権限プロンプトに掛かって routine が
止まった（2026-09-16、session_01E9LfqSkmUk9ke8owLbm9Nh）。要る情報はすべて git にある。

| 欲しいもの | 取り方 |
| --- | --- |
| マージ済み段階 PR の head 名（hop、phase retry） | `--root` の HEAD の first-parent にある merge commit の件名 `Merge pull request #N from <owner>/<head.ref>` |
| 進行中の段階 PR（冪等） | `git ls-remote --heads <remote> 'refs/heads/claude/sdd-NNN-*'` に残る branch（feature branch を除く）。リポジトリは delete_branch_on_merge なので、残っている = まだマージされていない |
| feature branch の有無（作業 base の確認） | 同じ ls-remote の結果 |

squash / rebase マージ（件名 `... (#N)`）は head 名が分からないので hop に数えない。段階 PR
は merge commit でマージする（[sdd-next-skill.md](sdd-next-skill.md) 5.）。人が plan PR を
squash しても hop 上限が少し緩くなるだけで、判定は壊れない。

## 出力

標準出力に JSON 1 行（[data-model.md 6.](../data-model.md)）。`state` は stdin で渡された
確定済み State を変更せず返し、呼び出し側はこちらを以後の真実として使う。

依存: bash、git、`jq`。`gh` も GitHub API も不要。

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた（`go` の真偽を問わない） |
| 2 | stdin が JSON でない、引数の誤り、`jq` または git が無い |

`git ls-remote` が失敗した場合は終了コード 0 で `{"go":false,"reason":"remote-unavailable",...}`
を返す（fail-closed）。

## 手順（この順で最初に該当したものを返す）

| # | 判定 | データ | 結果 |
| --- | --- | --- | --- |
| 1 | `state.stage` が `done` または `none` | — | `go:false`, `reason:"nothing-to-do"` |
| 2 | remote に問い合わせられない | `git ls-remote` の失敗 | `go:false`, `reason:"remote-unavailable"` |
| 3 | **作業 base**: `claude/sdd-NNN-feature` が remote にあるのに、現在 branch がそれではない | ls-remote と `git branch --show-current` | `go:false`, `reason:"wrong-base"`, `feature_branch`, `current_branch` |
| 4 | **冪等**: remote に `claude/sdd-NNN-*`（feature を除く）の branch が残っている。label の有無に依らない | ls-remote | `go:false`, `reason:"open-pr"`, `open_prs:[...]` |
| 5 | **フェーズ別リトライ**: `stage = implement` で、head が `claude/sdd-NNN-implement-pN` の merge commit が 2 件以上 | first-parent の件名 | `go:false`, `reason:"phase-retry-limit"` |
| 6 | **ホップ上限**: standard は `2 + phases + 2`、UI workflow は design 分を加えた `3 + phases + 2` 件以上 | 同上 | `go:false`, `reason:"hop-limit"` |
| 7 | 上記に該当しない | — | `go:true` |

`phases` は `state.phases`（無ければ 0）。

## 注意

- git 履歴は `--root` の HEAD を読む。段階 PR の履歴は feature branch にしかないので、
  スキルは `<github-trigger-context>` の base から feature branch を復元してから呼ぶ。
  復元し忘れは手順 3 が `wrong-base` で止める（main 上では plan.md が無いので、止めないと
  「plan からやり直せ」に見える）
- shallow clone で件名が読めない範囲のマージは数えない
- 出力の `open_prs` は remote に残る段階 branch 名の配列。空なら `[]`
- 最終 PR（feature → main）は feature branch そのものが head なので、手順 4 には掛からない

## 例

```bash
$ sdd-state.sh | sdd-guard.sh
{"go":true,"state":{"feature_dir":"specs/002-core-video-library","feature":"002","stage":"plan",...},"hops":0,"phase_retries":0,"open_prs":[]}

$ sdd-state.sh | sdd-guard.sh     # remote が無い作業ツリー
{"go":false,"reason":"remote-unavailable","state":{...}}

$ sdd-state.sh | sdd-guard.sh     # main 上で、feature branch は remote にある
{"go":false,"reason":"wrong-base","state":{...},"feature_branch":"claude/sdd-002-feature","current_branch":"main"}
```

## テスト

自動テスト（`tests/run.sh`）は、フィクスチャを写した一時 git リポジトリと bare の origin を
作って次を確認する。

- remote が無い → `remote-unavailable` で `state` を素通し
- feature branch がまだ無い初回は main 上で `go`（hops 0）
- plan マージ済みで `go`（hops 1）。UI workflow でも state を維持
- feature branch が remote にあるのに main 上 → `wrong-base`
- remote に段階 branch が残っていれば `open-pr`。他機能の branch は塞がない
- squash マージの件名は hop に数えない
- 同じフェーズの merge commit 2 件で `phase-retry-limit`
- 渡された対象機能を差し替えない
- 未知の引数は終了コード 2

`jq` と git が要り、無ければ FAIL になる（`mise install` で揃う。CI では走る）。

## 2026-09-13 feature branch 方式への変更

段階 PR は feature branch を base にする。冪等判定は remote に残る段階 branch で行い、
feature branch → main の最終 PR は段階実行を塞がない。ガードは対象 feature branch を
checkout した状態で呼び、first-parent 履歴から段階 hop を数える。

## 2026-09-17 git 専用化

PR 一覧の取得（`--github-dir`、`gh api` フォールバック、`gh-unavailable`）を廃止し、
`wrong-base` と `remote-unavailable` を追加した。`sdd-target.sh` は `--pr <番号>` で
起動した PR を受ける。
