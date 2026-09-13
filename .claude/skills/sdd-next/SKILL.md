---
name: "sdd-next"
description: "Spec Kit の次の 1 段階を feature branch 上で実行する。plan と最終マージだけを人の承認ゲートにする"
argument-hint: "--dry-run（判定だけ表示して終わる）"
user-invocable: true
disable-model-invocation: false
---

# sdd-next

1 spec に 1 本の長寿命 feature branch を作り、その branch を base にした段階 PR で
`plan → tasks → implement（フェーズごと）` を進める。**人が判断するのは plan PR と、完成した
feature branch を `main` へ入れる最終 PR の 2 回だけ**である。tasks と検査に成功した
implement PR は作成したセッションが直ちにマージする。

- 設計: [docs/design-docs/sdd-loop-harness.md](../../../docs/design-docs/sdd-loop-harness.md)
- 状態: [data-model.md](../../../specs/003-sdd-loop-harness/data-model.md)
- ガード: [contracts/sdd-guard.md](../../../specs/003-sdd-loop-harness/contracts/sdd-guard.md)

## 定数

| 名前 | 値 |
| --- | --- |
| `USAGE_LIMIT_7D` | 80 |
| `USAGE_LIMIT_5H` | 70 |
| `PHASE_RETRY_LIMIT` | 2（判定値は `sdd-guard.sh` が真実） |

## 0. 前提と作業 base の復元

作業ツリーが clean であることを確認する。cloud セッションに `gh` があるとは限らないため、
前提確認で `gh api /rate_limit` を要求してはならない。GitHub 情報が要る箇所では cloud の
組み込み GitHub ツールを使い、失敗したら理由を書いて変更を残さず終了する。

routine payload のマージ済み PR、または closed PR 一覧の直近のマージ済み `sdd` PR の
`base.ref` が `claude/sdd-NNN-feature` なら、次を行ってその feature branch を作業 base にする。
日次実行では open な `head.ref = claude/sdd-NNN-feature`, `base.ref = main` の最終 PR も候補にする。

```bash
git fetch origin <feature-branch>
git switch -C <feature-branch> origin/<feature-branch>
```

それ以外（spec PR が `main` に入った最初の実行）は `main` のままにする。段階間の状態は
`main` ではなく feature branch にあるため、この復元を省略してはならない。

## 0.5 使用量ゲート

`${TMPDIR:-/tmp}/sdd-rate-limits.json` から
`rate_limits.seven_day.used_percentage` と `rate_limits.five_hour.used_percentage` を読めた場合、
前者が 80 以上または後者が 70 以上なら次を表示し、何も作らず終了する。読めない場合は
ゲートを飛ばす。

```text
今回は見送り: 7 日窓 NN% / 5 時間窓 NN%
```

## 1. 判定

組み込み GitHub ツールでは `state=closed` と `state=open` の PR を **base で絞らず**各 100 件
取得し、JSON 配列を `${TMPDIR:-/tmp}/sdd-github/pulls-{closed,open}.json` に置く。
ツールの応答は書き換えずにそのまま保存する。各要素に `number`, `head.ref`, `base.ref`,
`labels` が必要で、`labels` は `["sdd"]` と `[{"name":"sdd"}]` のどちらでもよい。
`fields` で絞ると `labels` が落ちることがあるので付けない。手元では `--github-dir` を
省略した場合にだけ、スクリプトが任意フォールバックとして `gh api` を試す。

```bash
before=$(.claude/skills/sdd-next/scripts/sdd-state.sh)
guard=$(printf '%s\n' "$before" | \
  .claude/skills/sdd-next/scripts/sdd-guard.sh --github-dir "${TMPDIR:-/tmp}/sdd-github")
```

`guard.state` を以後の真実にする。`guard.go=false` は次の通り扱う。

| reason | 振る舞い |
| --- | --- |
| `open-pr` | 同じ feature branch 向けの段階 PR があるので終了 |
| `nothing-to-do` | `main` 上なら終了。feature branch 上なら手順 5 の最終 PR へ進む |
| `gh-unavailable` | 理由を表示し、変更を残さず終了 |
| `phase-retry-limit` / `hop-limit` | 同名の open Issue が無ければ停止通知を作る |

`--dry-run` ならここで guard、作業 base、次に作る head/base、plan なら手動マージ、その他なら
自動マージ、done なら最終 PR を表示して終了する。ファイル・branch・PR を変更しない。

## 2. feature branch と段階 branch

最初の `plan` で `feature_branch` が remote に無い場合だけ、現在の `main` から作って push する。
以後の段階は必ず最新の feature branch から作る。

```bash
git switch -c <state.feature_branch>       # 初回だけ
git push -u origin <state.feature_branch>  # 初回だけ
git switch -c <state.branch>
export SPECIFY_FEATURE_DIRECTORY=<state.feature_dir>
```

branch のトポロジーは `main ← claude/sdd-NNN-feature ← claude/sdd-NNN-<stage>` である。
Git の ref は同名 prefix と子 ref を同時に持てないため、名前に `/` は使わず、PR の base で
親子関係を表す。

## 3. 次の 1 段階だけを実行

| stage | 実行 | 検証 |
| --- | --- | --- |
| `plan` | `/speckit-plan` | `plan.md` と `research.md` が生成済み |
| `tasks` | `/speckit-tasks` → `/speckit-analyze` | `tasks.md` があり、tasks 側で直せる CRITICAL は解消済み |
| `implement` | `/speckit-implement "Phase N（title）のタスクだけ。他フェーズに触れない"` | 完了を `[X]` にし `make check` 成功 |

1 セッションで 2 段階へ進まない。tasks では `spec.md` / `plan.md` を直さない。implement の
検査を直せなければ draft PR にして自動マージしない。

## 4. 前進確認

```bash
after=$(.claude/skills/sdd-next/scripts/sdd-state.sh --feature <state.feature_dir>)
git status --porcelain
```

`after != before` かつ差分が非空のときだけ進む。同じなら `no-progress` の停止通知を作り、
段階 branch を削除して終わる。同じ phase でも `remaining` が減れば前進である。

## 5. commit、PR、マージ

段階成果物を日本語の要約で commit し、`state.branch` を push する。PR は次の値で作る。

| 項目 | plan | tasks / implement |
| --- | --- | --- |
| base | `state.base_branch` | `state.base_branch` |
| head | `state.branch` | `state.branch` |
| label | `sdd` | `sdd` |
| draft | false | `make check` 失敗時だけ true |
| マージ | **人がレビューしてマージ** | non-draft なら作成者が直ちに merge |

タイトルは plan=`docs: NNN の実装計画と設計成果物を追加する`、tasks=`docs: NNN の実装タスクを
分解する`、implement=`feat: NNN Phase N（phase_title 先頭 30 文字）を実装する` とする。
本文には before/after/guard、検査、残課題、session ID、`UI 変更なし` または UI 画像を含める。
ラベルを読み直して確認してからマージする。自動マージ API が失敗したら PR は open のまま
残し、停止理由を報告する（人に通常レビューを要求するための仕様には戻さない）。

`stage=done` では新しい commit や段階 branch を作らず、`feature_branch` → `main` の最終 PR を
1 件だけ開く（既存なら再利用）。タイトルは `feat: NNN を完成する`、label は `sdd`、draft は
false。**これは人が全体をレビューしてマージする。** final PR の open 状態は段階 PR の
`open-pr` ガードには含めない。

PR 作成・label 付与・label 検証・merge は cloud の組み込み GitHub ツールを使う。`gh` は
cloud では使わない。手元で実行する場合のみ、組み込み GitHub ツールの代替として `gh api`
などを使ってよい。
`main` や feature branch へ `git push` で成果物を直接上書きしてはならない。

## 6. 報告

段階、PR URL、自動/手動マージ結果、次に起きる段階を簡潔に報告する。

## 停止通知

`no-progress` / `phase-retry-limit` / `hop-limit` のときだけ
`sdd-next 停止: NNN <reason>` という open Issue を重複なく作る。本文に state、guard、session
ID を含め、label は付けない。作成と重複確認は cloud の組み込み GitHub ツールを使う。
`open-pr` / `nothing-to-do` / `gh-unavailable` では作らない。

## 手元での検算

```bash
.claude/skills/sdd-next/scripts/sdd-state.sh
.claude/skills/sdd-next/scripts/sdd-state.sh --feature specs/001-initial-setup
.claude/skills/sdd-next/scripts/sdd-state.sh | .claude/skills/sdd-next/scripts/sdd-guard.sh
bash .claude/skills/sdd-next/tests/run.sh
```

状態は成果物から導出し、専用状態ファイルを持たない。feature branch 上で実行することだけが
重要である。
