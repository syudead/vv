# Data Model: SDD ループハーネス

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-13

本機能に永続化するデータは無い。ここで定義するのは、判定の入出力として扱う値と、
その導出規則である。すべてリポジトリの成果物と GitHub の PR／Issue から都度導出する
（FR-009）。

## 1. 機能（Feature）

| 属性 | 型 | 導出元 |
| --- | --- | --- |
| `feature_dir` | 文字列（例 `specs/002-core-video-library`、リポジトリ root からの相対） | `specs/[0-9][0-9][0-9]-*/` に一致するディレクトリ |
| `feature` | 文字列（3 桁の番号、例 `002`） | `feature_dir` の basename の先頭 3 文字 |
| 成果物の有無 | `spec.md` / `plan.md` / `ui-design.md` / `tasks.md` それぞれの存在 | ファイルの存在 |
| `workflow` | `standard` / `ui` | 親 Issue の `ui` ラベル。ラベル判定は `/sdd-next` が行い、スクリプトへ渡す |
| `parent_issue` | 正の整数 | `spec.md` で行全体が `**Parent Issue**: #[1-9][0-9]*` に一致する単一行。feature番号とは独立 |

規則:

- 対象候補は `feature_dir` の昇順で走査する
- `sdd-target.sh` が git 履歴から対象を確定した後、その feature の `parent_issue` を読む
- 新規または進行中 feature で Parent Issue が欠落・重複・不正なら fail-closed とする。
  この契約導入前に完了済みの legacy feature は再実行しない限り移行不要
- `spec.md` が無い機能は `none`（対象外）として扱い、候補から除く

## 2. 段階（Stage）

列挙: `none` / `plan` / `design` / `tasks` / `implement` / `done`

導出（上から最初に一致したもの）:

| 条件 | stage |
| --- | --- |
| `spec.md` が無い | `none` |
| `plan.md` が無い | `plan` |
| `workflow = ui` かつ `ui-design.md` が無い | `design` |
| `tasks.md` が無い | `tasks` |
| tasks.md に未完了タスクがある | `implement` |
| それ以外 | `done` |

状態遷移（人のマージを挟んで 1 つずつ進む）:

```
standard: none ──▶ plan ──▶ tasks ──▶ implement(p1..pK) ──▶ done
ui:       none ──▶ plan ──▶ design ──▶ tasks ──▶ implement(p1..pK) ──▶ done
```

`implement` は同じ stage のまま `phase` が進む。同じ `phase` に留まることもある
（一部だけ進んだ回）。逆方向の遷移は無い（成果物を消さない限り）。

## 3. フェーズ（Phase）

tasks.md の `## Phase N:` 節。`stage = implement` のときだけ意味を持つ。

| 属性 | 型 | 導出元 |
| --- | --- | --- |
| `phase` | 整数（1 以上） | 見出し `## Phase N:` の `N` |
| `phase_title` | 文字列 | 見出しの `:` 以降を trim したもの（`(Priority: P1)` 等を含む） |
| `total` | 整数 | 節内の行頭 `- [ ] ` + `- [x] ` + `- [X] ` の数 |
| `remaining` | 整数 | 節内の行頭 `- [ ] ` の数 |
| `phases` | 整数 | tasks.md 全体の `## Phase N:` の数 |

規則:

- 対象フェーズは「`remaining > 0` の最初のフェーズ」
- 節の範囲は、その見出しから次の `## ` 見出し（または EOF）まで
- インデントされたチェックボックスは数えない（R-008）

## 4. 状態（State）

`sdd-state.sh` の出力。1 機能に対する判定結果の全体。

```json
{
  "feature_dir": "specs/002-core-video-library",
  "feature": "002",
  "stage": "implement",
  "phase": 3,
  "phase_title": "User Story 1 - 置いた動画が自動で一覧に並ぶ (Priority: P1) 🎯 MVP",
  "remaining": 5,
  "total": 12,
  "phases": 6,
  "branch": "claude/sdd-002-implement-p3"
}
```

| stage | 含まれる属性 |
| --- | --- |
| `none` | `feature_dir`, `feature`, `stage` |
| `plan` / `design` / `tasks` | 上 + UI workflow なら `workflow`, および `feature_branch`, `base_branch`, `branch` |
| `implement` | 上 + UI workflow なら `workflow`, および `phase`, `phase_title`, `remaining`, `total`, `phases`, `feature_branch`, `base_branch`, `branch` |
| `done` | `feature_dir`, `feature`, `stage`, UI workflow なら `workflow`, および `phases`, `feature_branch`, `base_branch` |

対象機能が 1 つも無いときは `{"stage":"none"}` だけを返す。

`branch` の規則:

| stage | branch |
| --- | --- |
| `plan` | `claude/sdd-NNN-plan` |
| `tasks` | `claude/sdd-NNN-tasks` |
| `implement` | `claude/sdd-NNN-implement-pN` |

前進の定義（FR-014）: 作業前後の State を文字列として比較して異なること。`implement` では
`remaining` が減れば異なる。

## 5. ホップ（Hop）

ハーネスが開いてマージされた PR 1 件。cloud の組み込み GitHub ツールで取得した PR 一覧と
git の first-parent 履歴から導出する。

| 属性 | 導出元 |
| --- | --- |
| 機能 | `head.ref` の `claude/sdd-NNN-` 部分 |
| 段階・フェーズ | `head.ref` の残り（`plan` / `design` / `tasks` / `implement-pN`） |
| マージ済み | `git log --first-parent` の件名に PR 番号がある |
| 対象 | `labels` に `sdd` を含む（`labels[].name` と文字列配列の両方を受ける） |

集計（`sdd-guard.sh`）:

| 名前 | 定義 | 上限 |
| --- | --- | --- |
| `hops` | 同じ機能のマージ済みホップ数 | standard は `2 + phases + 2`、UI は design 分を加えた `3 + phases + 2` 以上で停止（FR-016） |
| `phase_retries` | 同じ `implement-pN` のマージ済みホップ数 | 2 以上で停止（FR-015） |
| open な自動 PR | 同じ機能の `claude/sdd-NNN-*` で state = open、`base.ref = claude/sdd-NNN-feature`。label 付与失敗時も head/base が一致すれば対象。`base.ref` が無い同 prefix の open PR は区別不能なので fail-closed | 1 件以上で新しい段階 PR は作らない（FR-013）。未解決レビューがあればレビュー対応へ渡し、無ければ design / tasks / implement の non-draft PR の checks を再評価する |

`phases` は tasks.md が無い段階（plan／design／tasks）では 0 として扱う。
になる。tasks.md ができた後は実際のフェーズ数で計算し直す。

## 6. ガード結果（Guard）

`sdd-guard.sh` の出力。

```json
{"go": true, "state": { ...State... }, "hops": 1, "phase_retries": 0, "open_prs": []}
{"go": false, "reason": "open-pr", "state": { ... }, "open_prs": ["claude/sdd-002-plan"]}
```

`reason` の列挙と、停止通知（Issue）を作るかどうか:

| reason | 意味 | Issue |
| --- | --- | --- |
| `open-pr` | 対象機能の段階 branch が remote に残っている（open な自動 PR、または閉じた PR の残骸）。`open_heads` に観測 SHA | 作らない（レビュー対応・checks 再評価・stale branch 回復の入口） |
| `phase-retry-limit` | 同じフェーズのマージが 2 回に達した | 作る |
| `hop-limit` | 機能のホップ上限に達した | 作る |
| `no-progress` | （スキルが作業後に判定）状態が変わらない／差分なし | 作る |
| `wrong-base` | feature branch が remote にあるのに、その branch 上で実行していない | 作らない（スキルが復元してやり直す） |
| `remote-unavailable` | `git ls-remote` が失敗した（remote 無し・ネットワーク不可） | 作らない（作れない） |
| `nothing-to-do` | state が `done` または `none` | 作らない |

## 7. 停止通知（Issue）

| 属性 | 値 |
| --- | --- |
| 題名 | `sdd-next 停止: NNN <reason>`（例 `sdd-next 停止: 002 phase-retry-limit`） |
| 本文 | 理由の説明、State の JSON、Guard の JSON、`CLAUDE_CODE_REMOTE_SESSION_ID` |
| ラベル | 付けない（`sdd` ラベルは PR 専用。Issue に付けても routine は反応しないが、意味を混ぜない） |
| 重複判定 | open Issue の題名の完全一致（FR-017） |

## 8. Feature branch workflow（2026-09-13 改訂）

`plan` / `design` / `tasks` / `implement` の State は `feature_branch` と `base_branch` を持つ。どちらも
`claude/sdd-NNN-feature` である。`branch` は従来どおり段階ごとに異なり、その PR を
`base_branch` へ入れる。`done` は `feature_branch` と `base_branch: main` を持ち、段階を
作らず最終 PR を開く。plan PR と最終 PR だけを人がマージし、tasks と検査成功済みの
implement PR はハーネスがマージする。

## 9. レビュー対応（Review Response）

open な `sdd` PR に紐づく未解決 review thread または最新 commit 後の修正依頼コメント。
通常の State とは別に GitHub 上の PR 状態から導出し、状態ファイルは持たない。

| 属性 | 導出元 |
| --- | --- |
| 対象 PR | open PR。段階 PR は `base.ref = claude/sdd-NNN-feature`、最終 PR は `base.ref = main`。段階 PR は label 付与失敗時も同じ head/base なら対象 |
| 対象 branch | PR の `head.ref` |
| 要対応 | unresolved review thread、または最新 commit 後の `REQUEST_CHANGES` / 修正依頼コメント |
| 完了 | 修正 commit を同じ head に push し、該当 thread へ対応内容と検査結果を返信。解決できた thread は resolve |

レビュー対応が選ばれた run では新しい段階 PR を作らない。複数 PR が該当する場合は 1 run で
1 件だけ扱い、残りは次の日次 run または手動実行に任せる。
design / tasks / implement の non-draft PR は、レビュー対応後またはレビュー指摘が無い open-pr 分岐で
checks が green なら自動マージできる。pending / failed / 読み取り不能 / draft の場合は
open のまま待つ。
