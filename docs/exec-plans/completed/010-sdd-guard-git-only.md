# SDD ガードの git 専用化

- ステータス: 完了
- 対象: `.claude/skills/sdd-next/`（`sdd-guard.sh`、`sdd-target.sh`、`SKILL.md`、tests）

## Goal

feature branch 方式に切り替えて最初に走った routine セッション
（session_01E9LfqSkmUk9ke8owLbm9Nh）が、判定に入る前の
「組み込み GitHub ツールで closed PR 一覧を取り、ファイルに保存する」手順で止まった。
応答 157KB がトークン上限を超えて `~/.claude/projects/.../tool-results/` にスプールされ、
それを Bash で `cp` した時点で sandbox が「sensitive file の編集」の権限プロンプトを出し、
答える人がいない routine は ABANDONED になった。この経路を無くす。

## Plan

- [x] guard が欲しい情報（マージ済み段階 PR の head 名、進行中の段階 PR、feature branch の
      有無）が git だけで取れることを確認する。merge commit の件名に head 名が入り、
      リポジトリは delete_branch_on_merge なので remote に残る branch = 進行中
- [x] `sdd-guard.sh` を git 専用にする。`--github-dir` / `gh` フォールバック /
      `gh-unavailable` を廃止し、`remote-unavailable` と `wrong-base` を追加する
- [x] `sdd-target.sh` を `--pr <番号>`（`<github-trigger-context>` の PR）で受ける形にする
- [x] tests を bare の origin を使う git フィクスチャに置き換える（34 件 PASS）
- [x] SKILL.md 手順 0 / 1 を書き換え、`~/.claude/` 配下を Bash で触らない規則を明記する。
      段階 PR は merge commit でマージすることを明記する
- [x] 契約・データモデル・設計・routine 参照を追従させる

## Result

判定は git だけで行い、GitHub API は PR の作成・label・merge・review thread（応答が小さい
書き込み系と単一 PR の読み取り）に限った。scratch リポジトリで spec マージ → plan →
tasks → implement（部分・完了）→ done → 最終 PR マージ → 次 spec の 9 ホップを判定し、
すべて期待どおり。`wrong-base` により、main のまま判定して「plan からやり直せ」に見える
穴も塞いだ。

日次スケジュールトリガーは保守者が削除した。取りこぼしとレビュー対応は
Run now で拾う。

## 残課題

- squash / rebase マージは件名から head 名が消えるため hop に数えない（TD-013）
- routine の prompt は claude.ai 側の設定なので、`docs/references/sdd-routine.md` の
  写しと同時に手で更新する
