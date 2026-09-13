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
| 成果物の有無 | `spec.md` / `plan.md` / `tasks.md` それぞれの存在 | ファイルの存在 |

規則:

- 対象候補は `feature_dir` の昇順で走査する
- `spec.md` が無い機能は `none`（対象外）として扱い、候補から除く

## 2. 段階（Stage）

列挙: `none` / `plan` / `tasks` / `implement` / `done`

導出（上から最初に一致したもの）:

| 条件 | stage |
| --- | --- |
| `spec.md` が無い | `none` |
| `plan.md` が無い | `plan` |
| `tasks.md` が無い | `tasks` |
| tasks.md に未完了タスクがある | `implement` |
| それ以外 | `done` |

状態遷移（人のマージを挟んで 1 つずつ進む）:

```
none ──(人が spec を書く)──▶ plan ──▶ tasks ──▶ implement(p1) ──▶ … ──▶ implement(pK) ──▶ done
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
| `plan` / `tasks` | 上 + `branch` |
| `implement` | 上 + `phase`, `phase_title`, `remaining`, `total`, `phases` |
| `done` | `feature_dir`, `feature`, `stage`, `phases` |

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

ハーネスが開いてマージされた PR 1 件。GitHub の REST から導出する。

| 属性 | 導出元 |
| --- | --- |
| 機能 | `head.ref` の `claude/sdd-NNN-` 部分 |
| 段階・フェーズ | `head.ref` の残り（`plan` / `tasks` / `implement-pN`） |
| マージ済み | `merged_at != null` |
| 対象 | `labels[].name` に `sdd` を含み、`base.ref = main` |

集計（`sdd-guard.sh`）:

| 名前 | 定義 | 上限 |
| --- | --- | --- |
| `hops` | 同じ機能のマージ済みホップ数 | `2 + phases + 2` 以上で停止（FR-016） |
| `phase_retries` | 同じ `implement-pN` のマージ済みホップ数 | 2 以上で停止（FR-015） |
| open な自動 PR | 同じ機能の `claude/sdd-NNN-*` で state = open | 1 件以上で何もしない（FR-013） |

`phases` は tasks.md が無い段階（plan／tasks）では 0 として扱い、ホップ上限は `2 + 0 + 2 = 4`
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
| `open-pr` | 対象機能に open な自動 PR がある | 作らない（正常な待ち） |
| `phase-retry-limit` | 同じフェーズのマージが 2 回に達した | 作る |
| `hop-limit` | 機能のホップ上限に達した | 作る |
| `no-progress` | （スキルが作業後に判定）状態が変わらない／差分なし | 作る |
| `gh-unavailable` | `gh` が無い、または REST が失敗した | 作らない（作れない） |
| `nothing-to-do` | state が `done` または `none` | 作らない |

## 7. 停止通知（Issue）

| 属性 | 値 |
| --- | --- |
| 題名 | `sdd-next 停止: NNN <reason>`（例 `sdd-next 停止: 002 phase-retry-limit`） |
| 本文 | 理由の説明、State の JSON、Guard の JSON、`CLAUDE_CODE_REMOTE_SESSION_ID` |
| ラベル | 付けない（`sdd` ラベルは PR 専用。Issue に付けても routine は反応しないが、意味を混ぜない） |
| 重複判定 | open Issue の題名の完全一致（FR-017） |
