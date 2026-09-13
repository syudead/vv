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
対象機能の決定はスキルの手順（sdd-guard.sh）に従う。
```

## トリガー

| # | 種類 | 設定 |
| --- | --- | --- |
| 1 | GitHub event | リポジトリ `syudead/vv`、イベント `pull_request` / アクション `closed`。フィルタ: Base branch equals `main`、Labels is one of `sdd`、Is merged equals `true` |
| 2 | Schedule | 毎日 1 回、03:00 JST（見送り・取りこぼしの再開用。FR-019） |

トリガー 1 には `opened` / `labeled` / `synchronize` を**含めない**。含めると、ハーネス
自身が開く `sdd` ラベル付き PR で自分が起きてしまう。

## 前提

- Claude GitHub App が `syudead/vv` にインストールされている（GitHub トリガーに必要）
- リポジトリに `sdd` ラベルがある（説明: 「マージすると /sdd-next が次の段階を回す」、色 `0e8a16`）

ラベルがまだ無い場合、保守者が次のいずれかで作る。**ラベルが無いと連鎖が始まらない。**

```bash
gh api -X POST /repos/syudead/vv/labels \
  -f name=sdd \
  -f description='マージすると /sdd-next が次の段階を回す' \
  -f color=0e8a16
gh api /repos/syudead/vv/labels/sdd   # 作成を確認する
```

GitHub の画面から作る場合は Issues → Labels → New label で同じ名前・説明・色を入れる。

## 作成手順

1. claude.ai/code/routines → New routine。上の設定値を入れる。トリガーは Schedule を
   先に付けて保存する
2. 保存後に Edit routine → Add another trigger → GitHub event。フィルタを 3 条件で入れる
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
| routine ID | （未作成） |
| URL | （未作成） |
| 作成日 | （未作成） |
| プローブ（S3）の結果 | （未実施） |
