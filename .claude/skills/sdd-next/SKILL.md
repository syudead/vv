---
name: "sdd-next"
description: "Spec Kit の次の 1 段階（plan / tasks / implement の 1 フェーズ）を実行し、sdd ラベル付き PR を開く。routine と手動の両方から同じ手順で動く"
argument-hint: "--dry-run（判定だけ表示して終わる）"
user-invocable: true
disable-model-invocation: false
---

# sdd-next

Spec Kit の **次の 1 段階だけ**を実行し、`sdd` ラベル付きの PR を開いて終わる。承認ゲートは
人のマージである。1 セッション = 1 段階で、2 段階以上を続けて進めることはしない（FR-003）。

routine（[docs/references/sdd-routine.md](../../../docs/references/sdd-routine.md)）から呼ばれる
場合も、保守者が web セッションで手打ちする場合も、手順は同じである。判定ロジックは
すべて `scripts/` の 2 本にあり、この手順書はその出力に従うだけである。

- 設計: [docs/design-docs/sdd-loop-harness.md](../../../docs/design-docs/sdd-loop-harness.md)
- 契約: [contracts/sdd-next-skill.md](../../../specs/003-sdd-loop-harness/contracts/sdd-next-skill.md)
- 状態の定義: [data-model.md](../../../specs/003-sdd-loop-harness/data-model.md)

## 定数

| 名前 | 値 | 用途 |
| --- | --- | --- |
| `USAGE_LIMIT_7D` | 80 | 7 日窓の使用率がこれ以上なら見送る（FR-020） |
| `USAGE_LIMIT_5H` | 70 | 5 時間窓の使用率がこれ以上なら見送る |
| `PHASE_RETRY_LIMIT` | 2 | 同じフェーズのマージがこの回数に達したら止まる。`sdd-guard.sh` と同じ値で、**ここは文書用**である。判定はスクリプト側にしかない |

---

## 手順 0: 前提確認

次の 3 つをすべて確かめる。

```bash
git branch --show-current      # main であること
git status --porcelain         # 空であること
gh api /rate_limit             # 通ること
```

1 つでも満たさなければ、**理由を書いて終了する**。ブランチも作らず、ファイルも書き換えず、
リポジトリに変更を残さない。

## 手順 0.5: 使用量ゲート

US4（T029）で埋める。それまでは何もせず手順 1 へ進む。

## 手順 1: 判定

US2（T020）で埋める。それまでは次を実行し、`guard.go` が `true` のときだけ手順 2 へ進む。

```bash
before=$(.claude/skills/sdd-next/scripts/sdd-state.sh)
guard=$(printf '%s\n' "$before" | .claude/skills/sdd-next/scripts/sdd-guard.sh)
```

## 手順 1.5: `--dry-run`

US3（T024）で埋める。

## 手順 2: 準備

```bash
git switch -c <state.branch>
export SPECIFY_FEATURE_DIRECTORY=<state.feature_dir>
```

`state.branch` と `state.feature_dir` は手順 1 の `guard.state` から取る。

`SPECIFY_FEATURE_DIRECTORY` を export すると `.specify/scripts/bash/common.sh` が
`.specify/feature.json` を書くので、以後の `/speckit-*` は環境変数なしでも同じ機能を指す
（[R-003](../../../specs/003-sdd-loop-harness/research.md)）。ブランチ名は Spec Kit の
規約（`NNN-name`）ではなくハーネスの規約（`claude/sdd-NNN-<stage>`）に従う。routine の
セッションは `claude/` 接頭辞のブランチに push することが常に許可されている。

ブランチが切れなければ終了する。

## 手順 3: 段階の実行

`guard.state.stage` に応じて**1 つだけ**実行する。

| stage | 実行 | 検証 | 通らないとき |
| --- | --- | --- | --- |
| `plan` | `/speckit-plan` | `plan.md` と `research.md` が生成されている | 生成されていなければ手順 4 で `no-progress` になる。無理に作らない |
| `tasks` | `/speckit-tasks` → `/speckit-analyze` | `tasks.md` が生成され、analyze の CRITICAL が tasks.md 側の直しで解消済み | **`spec.md` と `plan.md` には手を入れない。** 解消できない CRITICAL は PR 本文の「残課題」に書いて残す |
| `implement` | `/speckit-implement "Phase <N>（<phase_title>）のタスクだけを対象にする。他のフェーズには手を付けない"` | 完了したタスクを `tasks.md` で `[X]` にし、`make check` が通る | 直す。直せなければ PR を **draft** で開き、本文に失敗内容を書く（FR-006） |
| `done` / `none` | 何もしない | — | — |

`implement` のフィルタは**引数で伝えるだけ**で、`/speckit-implement` 側に新しい仕組みは
足さない（FR-008）。`<N>` と `<phase_title>` は `guard.state.phase` / `guard.state.phase_title`
をそのまま使う。

どの段階でも「次の 1 段階」だけを実行する。plan を終えたあとに続けて tasks を実行しては
ならない（FR-003）。次の段階は、人がこの PR をマージしたときに別のセッションで始まる。

## 手順 4: 前進確認

US2（T021）で埋める。

## 手順 5: コミット・push・PR・ラベル

### コミット

日本語の要約 1 行 + 空行 + 本文。既存の履歴に合わせる。

- プレフィックス: `plan` / `tasks` は `docs:`、`implement` は `feat:`（テストだけを足した回は `test:`）
- 末尾に `Co-Authored-By: <セッションのモデル名> <noreply@anthropic.com>` を置く

### push

```bash
git push -u origin <state.branch>
```

`main` には直接 push しない。

### PR

| 項目 | 値 |
| --- | --- |
| base | `main` |
| title（plan） | `docs: NNN の実装計画と設計成果物を追加する` |
| title（tasks） | `docs: NNN の実装タスクを分解する` |
| title（implement） | `feat: NNN Phase N（<phase_title の先頭 30 文字>）を実装する` |
| draft | `implement` で `make check` が通らなかったときだけ `true` |
| label | `sdd`（必須） |

作成手段は次の順に試す。

1. cloud セッション組み込みの GitHub ツール
2. `gh pr create`
3. `gh api -X POST /repos/{o}/{r}/pulls`

本文は次の雛形をそのまま使う。

```markdown
## 段階

<stage>（機能 NNN、Phase N のとき: Phase N <phase_title>）

## 判定

- before: `<before の JSON>`
- after: `<after の JSON>`
- guard: `<guard の JSON>`

## 実行した検査

- <make check の結果 / analyze の要約 / なし>

## 残課題

- <あれば>

## 実行元

- session: <CLAUDE_CODE_REMOTE_SESSION_ID>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

### ラベル

**ラベルが無いと連鎖が切れる**（routine の GitHub トリガーが `sdd` で絞っているため）。
付けたことを必ず検証する。

```bash
gh api -X POST /repos/{o}/{r}/issues/{n}/labels -f 'labels[]=sdd'
gh api /repos/{o}/{r}/issues/{n}/labels        # sdd が入っていることを確認する
```

付いていなければ 1 回だけ再試行する。それでも付かなければ、PR 本文の**先頭**に
`ラベル未付与: 手で `sdd` を付けてください` を追記して終了する。

## 手順 6: 報告

次の 3 行だけを出す。

```text
段階: <stage>（機能 NNN、implement なら Phase N <phase_title>）
PR: <URL>
次に起きること: マージすると <次の段階> が始まる ／ これで完了
```
