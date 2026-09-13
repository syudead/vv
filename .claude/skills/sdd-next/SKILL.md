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

使用率がしきい値以上なら、**段階を実行せずに見送る**（FR-020）。使い切ってから止まると、
中途半端な成果物がブランチに残る。再開は日次トリガーか次のマージに任せる（FR-019）ので、
見送った回は PR も Issue も作らない。

### 取得手段

`${TMPDIR:-/tmp}/sdd-rate-limits.json`（通常は `/tmp/sdd-rate-limits.json`）があれば、
そこから次の 2 つを読む。`jq -r` が使えればそれで、無ければ `grep -o` で取る。

- `rate_limits.seven_day.used_percentage`
- `rate_limits.five_hour.used_percentage`

このファイルは [`.claude/hooks/rate-limits-statusline.sh`](../../hooks/rate-limits-statusline.sh)
が書く。使用率は Claude Code がステータスラインスクリプトへの stdin JSON にしか渡さない
ため、受け取るためだけにステータスラインを 1 つ置いている（[R-004](../../../specs/003-sdd-loop-harness/research.md) の案 a）。

**ファイルが無い・読めない・値が数値でない場合は「取得不能」とし、この判定を丸ごと飛ばして
通常どおり手順 1 へ進む。** 取得できないことを理由に止まってはならない（FR-020 後段）。

### 判定

取得できたときだけ、`USAGE_LIMIT_7D` / `USAGE_LIMIT_5H` と比べる。

| 条件 | 振る舞い |
| --- | --- |
| `seven_day >= USAGE_LIMIT_7D` または `five_hour >= USAGE_LIMIT_5H` | 次の 1 行を出して終了する。PR も Issue も作らず、ブランチも切らない |
| それ以外 | 手順 1 へ進む |

```text
今回は見送り: 7 日窓 NN% / 5 時間窓 NN%
```

> **未確定**: 取得手段は R-004 の案 a（ステータスライン）と案 b
> （`curl https://api.anthropic.com/api/oauth/usage`）のどちらかに、quickstart S3 の
> プローブで確定する。確定したらこの節を書き換える。両案とも不可なら、ゲートは無効の
> まま（取得不能 = 飛ばす）とし、[tech-debt TD-007](../../../docs/exec-plans/tech-debt.md)
> に残す。

## 手順 1: 判定

```bash
before=$(.claude/skills/sdd-next/scripts/sdd-state.sh)
guard=$(printf '%s\n' "$before" | .claude/skills/sdd-next/scripts/sdd-guard.sh)
```

**以後は `guard.state` を真実として使う。`before` も `guard.state` で置き換える。**
`sdd-guard.sh` は直近のマージ済み `sdd` PR が触った機能を見て対象を確定し直すので、
`sdd-state.sh` 単独の自動選択とは別の機能になりうるためである
（[contracts/sdd-guard.md](../../../specs/003-sdd-loop-harness/contracts/sdd-guard.md) 手順 1）。

`guard.go` が `true` なら手順 2 へ進む。`false` のときは `guard.reason` で分岐する。

| `reason` | 振る舞い |
| --- | --- |
| `open-pr` | **何もせず終了する。** 正常な待ちであり、異常ではない。ブランチも PR も Issue も作らない（FR-013）。人がその PR をマージすれば次のセッションが始まる |
| `nothing-to-do` | 何もせず終了する。対象機能が `done` か `none` で、進める先が無い（FR-007） |
| `gh-unavailable` | 理由を出して終了する。リポジトリに変更を残さない（Edge Cases）。手元での検算はこの経路を通る |
| `phase-retry-limit` | 「停止通知（Issue）」を立てて終了する（FR-015・FR-017） |
| `hop-limit` | 「停止通知（Issue）」を立てて終了する（FR-016・FR-017） |

`open-pr`・`nothing-to-do`・`gh-unavailable` では Issue を作らない。作ると、正常な待ちや
手元での検算のたびに Issue が増えてしまう。

## 手順 1.5: `--dry-run`

引数に `--dry-run` があれば、**手順 0〜1 だけ**を行い、何をするつもりかを表示して終了する。
ブランチも切らず、ファイルも書き換えず、PR も Issue も作らない。

1. `guard` の JSON を整形して表示する
2. 続けて 1 行:

```text
実行するなら: ブランチ `<state.branch>` を切って `<stage>`（implement なら Phase <N> `<phase_title>`）を実行し、PR `<title>` を開く
```

`<title>` は手順 5 の PR タイトルの規則で組み立てる。`guard.go` が `false` のときは、
その `reason` と「実行しない」ことを示す。

手順 0 の前提のうち `main` 上であることと作業ツリーが clean であることは `--dry-run` でも
要求する（判定は作業ツリーの成果物から導くため）。ただし **`gh api /rate_limit` が通らない
場合は、`gh-unavailable` の `guard` をそのまま表示して終わる**。保守者の手元
（`gh` も `jq` も無い Windows の Git Bash）で検算できるようにするためである。

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

段階を実行しても何も進まなかった回で PR を開くと、レビューの手間だけが増え、
マージされればホップを 1 つ消費してしまう。push の前に前進を確かめる（FR-014）。

```bash
after=$(.claude/skills/sdd-next/scripts/sdd-state.sh --feature <state.feature_dir>)
git status --porcelain
```

`after` を `before`（= `guard.state`）と**文字列として**比較する。属性の並びは stage ごとに
固定してあるので、文字列が同じなら状態は同じである
（[data-model.md 4.](../../../specs/003-sdd-loop-harness/data-model.md)）。

| 結果 | 次にすること |
| --- | --- |
| `after` が `before` と異なり、`git status --porcelain` も非空 | 前進した。手順 5 へ |
| `after` が `before` と同一 | **前進していない。** 下の後始末をする |
| `git status --porcelain` が空 | **差分が無い。** 下の後始末をする |

`implement` では、同じフェーズに留まっていても `remaining` が減っていれば `after` の文字列が
変わるので、前進とみなされる（「一部だけ進んだ回」を切り捨てない）。

前進していないときの後始末:

1. 「停止通知（Issue）」を `no-progress` で立てる
2. `git switch main`
3. `git branch -D <state.branch>`
4. push も PR もせずに終了する

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

---

## 停止通知（Issue）

止まったことを保守者に知らせる唯一の手段である。Issue の本文だけで、止まった理由と
そのときの状態が分かるようにする（SC-006）。

| 項目 | 値 |
| --- | --- |
| title | `sdd-next 停止: NNN <reason>`（固定形式。例 `sdd-next 停止: 002 phase-retry-limit`） |
| `<reason>` | `no-progress` / `phase-retry-limit` / `hop-limit` の **3 つだけ** |
| body | 理由の説明、`state` の JSON、`guard` の JSON、`CLAUDE_CODE_REMOTE_SESSION_ID` |
| ラベル | **付けない。** `sdd` は PR 専用である（Issue に付けても routine は反応しないが、意味を混ぜない） |

`open-pr`・`gh-unavailable`・`nothing-to-do` では Issue を作らない。

### 重複を作らない（FR-017）

作る前に必ず既存の open Issue を引く。

```bash
gh api '/repos/{o}/{r}/issues?state=open&per_page=100'
```

応答には PR も混ざるので、**`pull_request` キーを持たないもの**だけを見る。その中に
**title が完全一致**する Issue があれば、作らずに終了する。同じ理由で何度止まっても
Issue は 1 件のままになる。

作成手段は次の順に試す。

1. cloud セッション組み込みの GitHub ツール
2. `gh issue create`
3. `gh api -X POST /repos/{o}/{r}/issues`

---

## しないこと

- **2 段階以上を続けて進めない**（FR-003）。plan を終えたら PR を開いて終わる
- **`spec.md` を書き換えない。** `tasks` 段階の `/speckit-analyze` が CRITICAL を出しても、
  直すのは `tasks.md` の側だけである。解消できないものは PR 本文の「残課題」に残す
- **`main` に直接 push しない**
- **routine 側にしきい値や判定を持たせない。** 定数も分岐もこの手順書とスクリプトにある
- 同じイベントで 2 セッションが同時に起動し、両方が PR を開いた場合は、保守者が片方を
  閉じる。閉じた PR はマージされていないので連鎖しない（Edge Cases）

## 止め方

ハーネスを止めるのに PR は要らない。リポジトリ側は何も変えなくてよい（FR-018）。

| 目的 | 方法 |
| --- | --- |
| 一時停止 | routine の Repeats トグルを off にする。GitHub トリガーも止まる |
| 恒久停止 | routine を削除する |

手順は [docs/references/sdd-routine.md](../../../docs/references/sdd-routine.md) の「停止」にある。

---

## 手元での検算

判定は成果物だけから決まり、隠れた状態を持たない（FR-009）。したがって保守者は手元で
同じコマンドを実行し、セッションが何をするつもりかを先に確かめられる。`jq` も `gh` も
要らない（`sdd-guard.sh` だけが使い、無ければ `gh-unavailable` を返す）。

| コマンド | 期待 |
| --- | --- |
| `.claude/skills/sdd-next/scripts/sdd-state.sh` | 自動選択された機能の State が JSON 1 行で 1 秒以内に返る。何度実行しても同じ |
| `.claude/skills/sdd-next/scripts/sdd-state.sh --feature specs/001-initial-setup` | その機能を明示して判定する。`done` や `none` でもそのまま返る |
| `.claude/skills/sdd-next/scripts/sdd-state.sh \| .claude/skills/sdd-next/scripts/sdd-guard.sh` | `gh` か `jq` が無ければ `{"go":false,"reason":"gh-unavailable","state":...}`。両方あれば `go` の真偽と `hops` / `phase_retries` |
| `bash .claude/skills/sdd-next/tests/run.sh` | フィクスチャ 6 組とガードのテストが全件 PASS（`make test-sdd` と同じ） |
| `/sdd-next --dry-run` | 手順 1.5 の表示。ブランチも PR も作らない |

詳しい手順と期待値は
[quickstart.md](../../../specs/003-sdd-loop-harness/quickstart.md) の S1・S2 にある。

**保守者が手で段階を進めてもよい。** たとえば `plan.md` を自分で書いて `main` にマージ
すれば、次のセッションの判定は `tasks` になる。ハーネスは自分が何をしたかを覚えて
おらず、毎回その時点の成果物だけを見るので、人の作業とそのまま噛み合う。
