# routine `sdd-next (syudead/vv)` の設定

出典: [specs/003-sdd-loop-harness/contracts/routine.md](../../specs/003-sdd-loop-harness/contracts/routine.md)、写した日: 2026-09-13

claude.ai 側に作る routine の設定の写しである。**以後はこのファイルを真実とする**
（FR-021）。routine そのものは claude.ai/code/routines にあり、リポジトリからは
変更できない。設定を変えたら、このファイルも同じ変更単位で更新する。

routine には判定ロジックを一切置かない。しきい値も対象機能の決定も
[`.claude/skills/sdd-next/`](../../.claude/skills/sdd-next/SKILL.md) の側にある。

## 設定値

| 項目 | 値 |
| --- | --- |
| 名前 | `sdd-next (syudead/vv)` |
| リポジトリ | `syudead/vv`（既定ブランチ `main` から clone） |
| 環境 | Default（Trusted network）。[R-005](../../specs/003-sdd-loop-harness/research.md) の結果、SessionStart フックが走らなければ setup script に `make setup` を置く |
| モデル | 既定（保守者が選ぶ。implement 段階があるので最上位を推奨） |
| コネクタ | 無し |
| プロンプト | 下記 |

## プロンプト（全文）

```text
リポジトリの `/sdd-next` スキルを実行する。それ以外の作業はしない。
routine-fire-payload に PR 番号が含まれていれば、対象機能の確認に使ってよいが、
対象機能の決定とレビュー指摘への対応はスキルの手順に従う。
```

## トリガー

| # | 種類 | 設定 |
| --- | --- | --- |
| 1 | GitHub event | リポジトリ `syudead/vv`、イベント `pull_request` / アクション `closed`。フィルタ: Labels is one of `sdd`、Is merged equals `true`（Base branch フィルタは設定しない） |
| 2 | Schedule | 毎日 1 回、03:00 JST（見送り・取りこぼし・open PR のレビュー指摘対応の再開用。FR-019） |

トリガー 1 には `opened` / `labeled` / `synchronize` を**含めない**。含めると、ハーネス
自身が開く `sdd` ラベル付き PR で自分が起きてしまう。
レビュー指摘は GitHub event では起動せず、日次 schedule か保守者の Run now で拾う。
`/sdd-next` 側が open な `sdd` PR の review thread を見て、必要なら同じ PR の head に
修正 commit を積む。

> **移行時の必須作業**: claude.ai の routine 編集画面で既存の `Base branch equals main` を
> 削除する。段階 PR は `claude/sdd-NNN-feature` にマージされるため、このフィルタが残ると
> plan 承認後に連鎖が止まる。変更後、plan PR の test merge で新セッションが起動することを確認する。

## 前提

- Claude GitHub App が `syudead/vv` にインストールされている（GitHub トリガーに必要）
- リポジトリに `sdd` ラベルがある（説明: 「マージすると /sdd-next が次の段階を回す」、色 `0e8a16`）
  — **作成済み（2026-09-13）**

ラベルを作り直す場合の手順を残す。**ラベルが無いと連鎖が始まらない。**

推奨は GitHub の画面から作る方法である。Issues → Labels → New label で次を入れる。

| 項目 | 値 |
| --- | --- |
| Name | `sdd` |
| Description | `マージすると /sdd-next が次の段階を回す` |
| Color | `0e8a16` |

API で作る場合は GitHub App、REST client、または保守者の手元で使える認証済みツールから
`POST /repos/syudead/vv/labels` を呼ぶ。`gh` は使える環境だけの例であり、cloud セッションでは
前提にしない。

```bash
gh api -X POST /repos/syudead/vv/labels \
  -f name=sdd \
  -f description='マージすると /sdd-next が次の段階を回す' \
  -f color=0e8a16
gh api /repos/syudead/vv/labels/sdd   # 作成を確認する
```

## 作成手順

1. claude.ai/code/routines → New routine。上の設定値を入れる。トリガーは Schedule を
   先に付けて保存する
2. 保存後に Edit routine → Add another trigger → GitHub event。フィルタを 2 条件で入れる
   （CLI から付ける場合は `RemoteTrigger` の `create_webhook_trigger` で同じ内容）
3. 「Run now」で 1 回実行し、`/sdd-next --dry-run` の結果とプローブ項目
   （[R-004〜R-006](../../specs/003-sdd-loop-harness/research.md)）を transcript で確認する
   （[quickstart.md](../../specs/003-sdd-loop-harness/quickstart.md) S3）
4. 下の「作成の記録」を埋める

## 停止

| 目的 | 方法 |
| --- | --- |
| 一時停止 | routine の Repeats トグルを off にする。GitHub トリガーも止まる（FR-018） |
| 恒久停止 | routine を削除する。リポジトリ側は何も変えなくてよい |

どちらもリポジトリの変更を伴わない。ハーネスを止めるのに PR は要らない。

## 作成の記録

保守者が routine を作ったあとに埋める。

| 項目 | 値 |
| --- | --- |
| routine ID | `trig_01Bxsy8v6ogCron7BY6hrUkY` |
| URL | <https://claude.ai/code/routines/trig_01Bxsy8v6ogCron7BY6hrUkY> |
| 作成日 | 2026-09-13（環境 `env_014pXxfbDJcVpmSABD6rgVJf`、モデル `claude-opus-5`、cron `0 18 * * *` = 03:00 JST） |
| GitHub トリガー ID | `f9033214-135a-42f0-93dd-6c2a794b50d9`（`create_webhook_trigger` で作成。API の応答にフィルタが含まれないため、Labels / Is merged の 2 条件を画面で確認し、Base branch 条件があれば削除する） |
| プローブ（S3）の結果 | 2026-09-13 実施（session `cse_011ZErYWwbMCiPrnW6Msyka7`）。**`sdd-guard.sh` が `gh-unavailable` を返して見送り** — cloud 環境に `gh` CLI が無い（`gh: command not found`）。R-005: SessionStart フックは発火した（init まで約 110 秒、hook イベント多数。setup script は不要）。R-004 案 a: `/tmp/sdd-rate-limits.json` は無く、statusLine は cloud では走らない。案 b（curl）と `GH_TOKEN` は未確認 — Run now の添え文は `<routine-fire-payload>` として届き、routine プロンプトが「それ以外の作業はしない」なので (1)(3)(5) の報告は行われなかった。cloud セッションでは `mcp__github__*` ツールは使えた。対処: `sdd-guard.sh` に `--github-dir` を足し、スキルが組み込みの GitHub ツールで取った PR 一覧を渡す形にした（cloud には `gh` を入れない） |
