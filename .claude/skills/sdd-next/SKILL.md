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
implement PR は、PR checks が green になってから作成したセッションがマージする。

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

routine payload のマージ済み PR、または closed PR 一覧から選んだ直近の **git 履歴にある**
マージ済み段階 `sdd` PR の `base.ref` が `claude/sdd-NNN-feature` なら、次を行ってその
feature branch を作業 base にする。closed PR 一覧は `updated_at` 順なので、未マージ PR や
コメント更新だけで先頭に来た PR を作業 base の根拠にしてはならない。

日次実行では、open な `head.ref = claude/sdd-NNN-feature`, `base.ref = main` の最終 PR も
候補にする。複数ある場合は、更新日時に頼らず、対象候補を `head.ref` の昇順で 1 件だけ選ぶ。
同時に複数機能を進めることは本機能の対象外なので、選ばなかった候補は触らない。

```bash
git fetch origin <feature-branch>
git switch -C <feature-branch> origin/<feature-branch>
```

それ以外（spec PR が `main` に入った最初の実行）は `main` のままにする。段階間の状態は
`main` ではなく feature branch にあるため、この復元を省略してはならない。guard が返した
`state.feature_branch` が現在 branch と違い、remote に存在する場合は、その branch に切り替えて
PR 一覧の取得、`before`、guard をやり直す。

## 0.5 使用量ゲート

`${TMPDIR:-/tmp}/sdd-rate-limits.json` から
`rate_limits.seven_day.used_percentage` と `rate_limits.five_hour.used_percentage` を読めた場合、
前者が 80 以上または後者が 70 以上なら次を表示し、何も作らず終了する。読めない場合は
ゲートを飛ばす。

```text
今回は見送り: 7 日窓 NN% / 5 時間窓 NN%
```

## 1. 判定

組み込み GitHub ツールでは `state=closed` と `state=open` の PR を **base で絞らず**、
`per_page=100` 相当で必要なページを続けて取得し、JSON 配列を
`${TMPDIR:-/tmp}/sdd-github/pulls-{closed,open}.json` に置く。
ツールの応答は書き換えずにそのまま保存する。各要素に `number`, `head.ref`, `base.ref`,
`labels` が必要で、`labels` は `["sdd"]` と `[{"name":"sdd"}]` のどちらでもよい。
open 一覧で同じ head prefix の要素に `base.ref` が無い場合は、段階 PR と最終 PR を区別できないので
`gh-unavailable` として止める。open な段階 PR は label 付与に失敗していても同じ head/base なら塞ぐ。
`fields` で絞ると `labels` や `base.ref` が落ちることがあるので付けない。手元では `--github-dir` を
省略した場合にだけ、スクリプトが任意フォールバックとして `gh api` を試す。

cloud:

```bash
before=$(.claude/skills/sdd-next/scripts/sdd-state.sh)
guard=$(printf '%s\n' "$before" | \
  .claude/skills/sdd-next/scripts/sdd-guard.sh --github-dir "${TMPDIR:-/tmp}/sdd-github")
before=$(printf '%s\n' "$guard" | jq -c '.state')
```

手元:

```bash
before=$(.claude/skills/sdd-next/scripts/sdd-state.sh)
guard=$(printf '%s\n' "$before" | .claude/skills/sdd-next/scripts/sdd-guard.sh)
before=$(printf '%s\n' "$guard" | jq -c '.state')
```

`guard.state` を以後の真実にし、`before` も必ず `guard.state` に置き換える。ガードが直近の
マージ済み `sdd` PR から対象機能を差し替えることがあるため、初回の `sdd-state.sh` 出力を
前進確認に使ってはいけない。`guard.go=false` は次の通り扱う。

| reason | 振る舞い |
| --- | --- |
| `open-pr` | `open_prs` の対象 PR を確認する。未解決レビュー指摘があれば手順 1.5 へ進む。無ければ、tasks / implement の non-draft PR は checks を再取得し、green なら既存 PR をマージ、失敗・pending・draft なら理由を報告して待機する。plan PR は人の承認待ちとして終了 |
| `nothing-to-do` | `main` 上なら終了。feature branch 上なら既存の最終 PR と未解決レビュー指摘を先に確認し、要対応なら手順 1.5 へ進む。無ければ手順 5 の最終 PR へ進む |
| `gh-unavailable` | 理由を表示し、変更を残さず終了 |
| `phase-retry-limit` / `hop-limit` | 同名の open Issue が無ければ停止通知を作る |

`--dry-run` ならここで guard、作業 base、次に作る head/base、plan なら手動マージ、その他なら
自動マージ、done なら最終 PR を表示して終了する。ファイル・branch・PR を変更しない。

## 1.5 レビュー対応

open な `sdd` PR に未解決のレビュー指摘がある場合は、通常の段階実行より先にレビュー対応だけを
行う。`guard.reason == "open-pr"` のときも即終了せず、`guard.open_prs` の head に対応する
PR をここで確認する。対象は `state.base_branch` 向けの段階 PR、または `main` 向けの最終 PR のうち、
組み込み GitHub ツールで取得した review thread が未解決、または最新 commit 後に
`REQUEST_CHANGES` / 修正依頼コメントが付いたものに限る。

複数ある場合は更新日時が古い 1 件だけを扱い、同じセッションで新しい段階 PR を作らない。
対象 PR の head branch を checkout し、レビュー指摘に必要な最小差分だけを入れ、該当する検査を
再実行して同じ PR に push する。push 後は各レビュー thread に「対応内容 / 検査結果 / 追加で
人の判断が必要な点」を返信し、解決できた thread は resolve する。tasks / implement の
non-draft PR は、返信後に checks が green ならマージしてよい。checks が pending / failed /
読めない場合は、次回の日次実行が open PR 分岐で再評価するため、open のまま理由を報告して終わる。
仕様判断・権限・外部情報が必要なら修正を作らず、PR コメントで block 理由を返して終了する。

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
この feature branch は SDD ハーネス専用の作業 base であり、`AGENTS.md` の「push した
feature branch には `main` 向け PR を開く」一般規則の例外である。最終 PR は `stage=done` に
なってから 1 件だけ開く。

## 3. 次の 1 段階だけを実行

| stage | 実行 | 検証 |
| --- | --- | --- |
| `plan` | `/speckit-plan` | `plan.md` と `research.md` が生成済み |
| `tasks` | `/speckit-tasks` → Phase ごとの領域分類 → `/speckit-analyze` | `tasks.md` があり、各 Phase に `sdd-domains` が 1 つあり、tasks 側で直せる CRITICAL は解消済み |
| `implement` | 対象 Phase の領域分類 → 選択したループで `/speckit-implement` | 完了を `[X]` にし、実装後の分類漏れ検査と `make check` が成功 |

1 セッションで 2 段階へ進まない。tasks では `spec.md` / `plan.md` を直さない。implement の
検査を直せなければ draft PR にして自動マージしない。

### Phase の領域分類

tasks では要件を Phase に分解した後、各 `## Phase N:` 節に、その Phase の実装で AI ハーネスの
実行ループを切り替えるための領域分類を 1 行だけ付ける。

```markdown
## Phase 2: 一覧画面と検索 API
<!-- sdd-domains: frontend-ui, backend -->
```

使用できる分類は `frontend-ui`、`frontend-non-ui`、`backend`、`infrastructure`、
`documentation`。これはファイル種別ではなく、その Phase の実装ループを変える必要がある領域を
表す。分類の欠落、未知の値、同一 Phase 内の複数行は `/speckit-analyze` までに解消する。
tasks の完了確認では `sdd_phases` が返す全 Phase に対して `sdd-ui-classify.sh --phase N` を実行し、
終了コード 0 を確認する。1 Phase でも失敗した場合は tasks 段階を完了扱いにしない。

### UI 変更の専用ループ

implement では `/speckit-implement` より前に対象 Phase を判定する。

```bash
.claude/skills/sdd-next/scripts/sdd-ui-classify.sh \
  --feature <state.feature_dir> --phase <state.phase>
```

終了コード 3 はタスク分解の契約違反なので、実装を開始せず停止理由を報告する。
`ui_change=true`、すなわち Phase が `frontend-ui` を含む場合は、`/speckit-implement` のプロンプトに
以下の専用手順を含め、通常の部品単位の完了扱いにはしない。

1. 実装前に、ユーザー要求、spec / plan / tasks、参照画像、変更前画面を確認する。
2. フェーズ内のタスクを、部品別ではなく「一覧画面を完成」「再生画面を完成」のような
   ページ単位の縦切りで実装する。途中状態の画面を PR にしない。
3. 実ブラウザで 360px、768px、1280px の viewport を描画してスクリーンショットを作る。
   ブラウザが起動できない環境では UI 変更の PR を non-draft にしない。
4. 参照画像がある場合は、参照画像と変更後スクリーンショットを横に並べた比較画像を作る。
5. 実装者の視点から離れ、仕様・参照画像・スクリーンショットだけを読んで visual review を行う。
   指摘を記録し、最低 1 回は修正して再撮影する。指摘が無い場合でも「指摘なし」として
   その評価を PR 本文に残す。
6. hover / active / keyboard / focus / tap target / reduced motion など、変更した画面に関わる
   interaction と accessibility を確認する。

実装後は staged / unstaged の tracked ファイルと未追跡ファイルをまとめ、分類漏れを検査する。

```bash
{ git diff --name-only HEAD; git ls-files --others --exclude-standard; } \
  | sort -u > "${TMPDIR:-/tmp}/sdd-ui-paths.txt"
.claude/skills/sdd-next/scripts/sdd-ui-classify.sh \
  --feature <state.feature_dir> --phase <state.phase> \
  --paths "${TMPDIR:-/tmp}/sdd-ui-paths.txt"
```

`classification_mismatch=true` は、`frontend-ui` ではない Phase で UI 実装ファイルが変更されたことを
示す。これは専用ループを実装前から適用できなかった分類漏れなので、non-draft PR を作らず停止する。
テスト専用ファイルはこの安全網の UI パス判定から除外する。

UI 変更のスクリーンショットと比較画像は [docs/how-to/ui-change-screenshots.md](../../../docs/how-to/ui-change-screenshots.md)
に従って `docs/screenshots/` に置く。visual review は同じセッション内で行ってよいが、
実装中のメモではなく、撮影後の画面成果物を入力にした別節として書き直す。

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
| draft | false | `make check` 失敗または検査 skip 時は true |
| マージ | **人がレビューしてマージ** | non-draft かつ PR checks が green なら作成者が merge |

タイトルは plan=`docs: NNN の実装計画と設計成果物を追加する`、tasks=`docs: NNN の実装タスクを
分解する`、implement=`feat: NNN Phase N（phase_title 先頭 30 文字）を実装する` とする。
本文には before/after/guard、検査、残課題、session ID を含める。UI 変更でない場合は
`UI 変更なし` と書く。UI 変更の場合は変更前後、確認した viewport（最低 360 / 768 / 1280）、
参照画像との比較画像、visual review の指摘と修正、interaction / accessibility の確認結果、
視覚上の残課題を含める。
ラベルを読み直して確認し、PR の checks が green になるまで待ってからマージする。checks を
読めない、失敗、pending のまま timeout、またはローカル検査に skip がある場合は draft/open のまま
残し、停止理由を報告する（人に通常レビューを要求するための仕様には戻さない）。
自動マージが成功した段階 PR は、同じ phase を再実行しても non-fast-forward にならないよう
remote の `state.branch` を削除する。削除に失敗した場合は次回実行で同名 branch を上書きせず、
古い remote head と open PR の有無を報告して停止する。

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
