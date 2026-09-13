# 契約: routine `sdd-next (syudead/vv)`

**Feature**: [spec.md](../spec.md) | **Research**: [R-001](../research.md), [R-005](../research.md)

claude.ai 側に作る routine の設定。実装時にこの内容を `docs/references/sdd-routine.md` に
写し、以後はそちらを真実にする（FR-021）。routine には判定ロジックを一切置かない。

## 設定値

| 項目 | 値 |
| --- | --- |
| 名前 | `sdd-next (syudead/vv)` |
| リポジトリ | `syudead/vv`（既定ブランチ `main` から clone） |
| 環境 | Default（Trusted network）。R-005 の結果で SessionStart フックが走らなければ setup script に `make setup` を置く |
| モデル | 既定（保守者が選ぶ。implement 段階があるので最上位を推奨） |
| コネクタ | 無し |
| プロンプト | 下記 |

## プロンプト（全文）

```
リポジトリの `/sdd-next` スキルを実行する。それ以外の作業はしない。
routine-fire-payload に PR 番号が含まれていれば、対象機能の確認に使ってよいが、
対象機能の決定はスキルの手順（sdd-guard.sh）に従う。
```

## トリガー

| # | 種類 | 設定 |
| --- | --- | --- |
| 1 | GitHub event | リポジトリ `syudead/vv`、イベント `pull_request` / アクション `closed`。フィルタ: Base branch equals `main`、Labels is one of `sdd`、Is merged equals `true` |
| 2 | Schedule | 毎日 1 回、03:00 JST（見送り・取りこぼしの再開用。FR-019） |

トリガー 1 は `opened` / `labeled` / `synchronize` を含めない（自分の PR で自分が起きないため）。

## 前提

- Claude GitHub App が `syudead/vv` にインストールされている（GitHub トリガーに必要）
- リポジトリに `sdd` ラベルがある（説明: 「マージすると /sdd-next が次の段階を回す」）

## 作成手順

1. claude.ai/code/routines → New routine。上の設定値を入れる。トリガーは Schedule を
   先に付けて保存する
2. 保存後に Edit routine → Add another trigger → GitHub event。フィルタを 3 条件で入れる
   （CLI から付ける場合は `RemoteTrigger` の `create_webhook_trigger` で同じ内容）
3. 「Run now」で 1 回実行し、`/sdd-next --dry-run` の結果とプローブ項目（R-004〜R-006）を
   transcript で確認する（quickstart S3）
4. 設定値を `docs/references/sdd-routine.md` に写す。routine の ID と URL も書く

## 停止

- 一時停止: routine の Repeats トグルを off にする（FR-018）。GitHub トリガーも止まる
- 恒久停止: routine を削除する。リポジトリ側は何も変えなくてよい
