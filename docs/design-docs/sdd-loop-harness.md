# Issue handoff SDD

## Decision

SDDの工程継続をClaude RoutineやPRイベントに任せない。保守者が親Issueまたはnative
sub-issueのURLを任意のコーディングエージェントへ渡し、エージェントは次の工程をGitHubと
feature branchの状態から自分で選び、一工程だけを実行してPRを作り、そこで終了する。
保守者はURLを渡す以上の指示をしない。工程を明示した場合だけ、その指定が優先する。

これは保守者が明示的に開始した機能開発用のフローである。それ以外の変更は通常どおり、
focused branchから`main`向けPRを作る。機能開発では長寿命feature branchとsub-branchの
構成を使う。

```text
main
  <- feature branch                 integration PR（Closes parent）
       <- arbitrary stage branch    Plan / Design PR
       <- arbitrary task branch     implementation PR
```

branch名は識別子ではない。Issue番号とfeature directory番号にも対応規則を設けない。

## Why

旧方式は成果物から工程を導出できても、その実行開始、対象復元、retry、mergeがClaude Routine、
`<github-trigger-context>`、`sdd`ラベル、`claude/sdd-*`というbranch名に依存していた。別agentは
前sessionの状態を再現できず、repository内の判定scriptも外部Routineなしには前進できなかった。

新方式では作業の文脈を二種類から読む。

- repository state: feature branchへmerge済みの`plan.md`、`ui-design.md`
- GitHub relationships: 親Issue本文、Issue timeline、integration PRの`Closes`、PRのhead/base、native sub-issues

agent固有session、packet、独自JSON、branch命名はどちらにも含めない。

進捗と次の工程もこの二つから読む。Planの有無はfeature branch上の`plan.md`、
Designの有無は`ui-design.md`、実装の進み具合はnative sub-issuesと、それを`Refs`する
merge済みPRで分かる。これを親Issue本文へ書き写す進捗欄は持たない。

## Ownership

### Parent Issue

親Issueは仕様そのものだけを持つ。要求、要件、受け入れ条件、Edge Cases、対象外がここにあり、
`.agents/skills/issue-spec`が書く。PR、branch、子Issue一覧、retry、agent情報、工程の進捗は
書かない。

以前は本文末尾に`## SDD`節（Plan/Designのチェックリストと`Next`）を置き、stage PRのmerge
ごとに保守者が手で更新していた。実際には更新されないまま閉じる親Issueが多く、表示が実態と
食い違った。同じ情報はfeature branchの成果物とGitHub relationshipから読めるため、節ごと
廃止した。既存Issueに残る`## SDD`節は意味を持たない。

### Labels

手順上の意味を持つラベルは`ui`だけである。`ui`付きの親Issueは`## UI品質とアクセシビリティ`
節を持ち、`plan`と`plan-to-issues`の間で`design`工程を経る。ラベルは要求者が決め、agentは
推測で付けない。

`sdd`ラベルは廃止した。工程をラベルで起動・判定しないので、付いていても何も起きない。
それ以外のラベル（`enhancement`など）は人の分類用で、手順は読まない。

### Feature artifacts

共通skillは指定されたIssue、PR、branch、現在のcheckoutと、関連する成果物をそのまま読む。
featureディレクトリは`plan.md`を必ず持ち、`ui`Issueでは`ui-design.md`を加える。
Planが判断の根拠や契約を持つ場合は`research.md`・`data-model.md`・`contracts/`・
`quickstart.md`がそれに伴う（P-2: 書くことが無い成果物は作らない）。`spec.md`は
作らない。要求の正本は親Issueである。

既存の後続成果物が後から改訂された前段成果物を取り込んでいるかは自動推測しない。改訂時は保守者が
影響する工程を明示してreviewed PRとして再実行する。

### GitHub

- stage PRは親Issueを`Refs`で通常参照し、feature branchをbaseにする。
- implementation PRは一つの子Issueを`Refs`で参照し、feature branchをbaseにする。
- integration PRだけが`main`をbaseにし、親Issueを`Closes`で参照する。
- childはimplementation PRがfeatureへmergeされた時点で人（[Autopilot](#autopilot)ではorchestrator）が
  completedとして閉じる。
  閉じ忘れても工程選択は止まらない。
- parentはintegration PRが`main`へmergeされた時点でGitHub標準動作により閉じる。

子Issueのcloseは「feature branchへ実装済み」、親Issueのcloseは「mainへ統合済み」を表す。

## Context

渡されたIssueから、GitHub標準の関係だけを辿ってfeatureを特定する。

- feature branch: 親を`Refs`し`main`以外を向くmerge済みPR（Plan PR以降のstage PR）のbase。
  そこにある`specs/<dir>/plan.md`がfeature directoryである。
- integration PR: `integrate`が開いた後は、親を`Closes`する`main`向けのopen PR。そのheadは同じfeature branch。

候補が複数ある場合は推測せずユーザーへ確認する。branch名やdirectory番号には頼らない。

以前はこの復元手順を設けず、工程も保守者が毎回指定していた。指定の手間が運用上の負担で
あり、状態はGitHubから一意に読めるため、復元と工程選択をskillへ移した。

## Stage transitions

通常Issueは`plan -> plan-to-issues -> implement* -> integrate`、UI Issueは`plan`の後に
`design`を挟む。仕様は親Issueとして先に書かれているので、工程には含めない。

親Issueを渡されたagentは次の順で最初に当てはまるものを実行する。規則の正本は
`.agents/skills/issue-handoff/references/README.md`の「Selecting the stage」である。

1. 親を`Refs`するopenなstage PRがある: review指摘が未対応ならそのPR headで直す。
   なければmerge待ちと報告して終了する。
2. feature branchが無い: `plan`。
3. `ui`ラベルがあり`ui-design.md`が無い: `design`。
4. native sub-issueが無い: `plan-to-issues`。
5. 未完了の子がある: open PRが無く前提作業が完了済みの最初の子を`implement`。
6. すべての子が完了: `integrate`（integration PRが無ければここで開く）。

子の「完了」は、completedでcloseされているか、それを`Refs`するPRがfeature branchへ
merge済みであることを指す。子Issueのcloseは保守者の操作のまま残すが、工程選択はcloseを
待たない。子Issueを渡された場合はその子を`implement`する。

選んだ工程と根拠はrunの冒頭とPR本文に書き、誤選択をreviewで見つけられるようにする。
成果物が既にある工程の再実行（改訂）は自動選択しない。保守者が工程を指定する。

[Autopilot](#autopilot)は同じ規則を繰り返し適用し、規則1のstage PRをmerge待ちで止めずに
自らmergeまで進める。

`plan-to-issues`は承認済みPlanから実装作業を直接native sub-issueとして作る。子Issueを作る直前に親の既存
sub-issuesを確認し、同じ作業が既にあれば作成しない。既存childの更新やcloseは対象Issueが明示された
場合だけ行う。

## Integration

全child完了後のrunが`integrate`として、最新`main`を長寿命feature branchへ直接mergeし、
feature branchから`main`へのintegration PRを開く（既にあれば更新する）。

以前はPlan merge後の最初のrunでintegration PRを開いていた。するとfeature branchへのmergeの
たびに作りかけの全体差分がreview botにreviewされ、未着手の子Issueの範囲を欠陥として報告し
続けた（#135では統合PRへの指摘約40件のうち約30件が「未着手の範囲」「PR本文の進捗」
「設計どおり」への返答だけで終わった）。個々の変更は子のPRでreviewされるので、統合PRは
全体が揃ってから開く。

integration PRのreviewでは、必須checkの失敗、code scanningの警告、人の指摘、botが
bugまたは高重大度のsecurityとした指摘（Devin Reviewの🔴・🟥）だけを直す。それ以外の
botの指摘は検証して返答・resolveし、本当の欠陥はPR本文の残るリスクに載せて保守者の判断に
委ねる。全体差分はpushのたびに再reviewされ、botは毎回数件の新しい指摘を出すので、全部を
直す条件は収束しない。同じ理由で、`main`の再取り込みは`main`との衝突か`main`起因の
check失敗があるときだけ行う。
既存PRのbranch更新に別のPRは作らない。feature全体の`task check`と必要なUI reviewを再実行してから、
人がintegration PRをmergeする。rebaseやforce-pushで長寿命feature branchを書き換えない。

## Automation boundary

CIはすべてのPRで検証するが、agentや次工程を起動しない。使用しないものは次のとおり。

- Claude Routine
- stage PR mergeを購読するworkflow
- `sdd` automation label
- schedule reconcile
- hop/retry state
- agent packet、result JSON、session state
- branch名によるfeature/stage判定

正規手順は`.agents/skills/issue-handoff/`に置く。保守者が明示的に起動する連続実行は
[Autopilot](#autopilot)を参照。

一工程を担当する親agentは、hostが対応している場合、その工程内の境界が確定した作業を
project-scoped subagentへ委譲する。repositoryはCodex向けに`.codex/agents/`、Claude向けに
`.claude/agents/`の同じ役割のworkerを持つ。子Issueの実装とfocused testは
`subissue-implementer`へ委譲する。
branch、全体検証、review指摘の修正、push、PRは親agentが所有する。この内部委譲は次工程を
起動せず、handoff stateも追加しない。対応しないhostは同等のbounded workerを使い、なければ
同じ作業を親agent自身で実行する。[Autopilot](#autopilot)では例外として、`sdd-stage-worker`と
`pr-review-fixer`がpushとPR作成まで行う。

## Autopilot

保守者が親Issueを明示して`.agents/skills/sdd-autopilot`を起動した場合に限り、上の工程を
統合PRのmerge直前まで連続して進める。これはClaude RoutineやPRイベント購読のworkflowでは
なく、保守者が開始した一つのsessionであり、終われば何も残らない。

- orchestratorは工程の作業をしない。各工程とreview指摘の一巡ごとに新しい
  worker（`sdd-stage-worker`、`pr-review-fixer`）を起動し、Issue番号・
  branch・pathだけを渡して、固定形式の短い結果だけを受け取る。長い会話でもorchestratorの
  文脈にはdiff、CI log、review本文、Issue本文が溜まらない。
- 次の工程は上の工程選択の規則をそのまま毎回適用して決める。session stateやledgerは持たない
  ので、会話の要約や再開で失うものがない。orchestratorはIssue本文やPR本文を読まず、本文が要る
  判断（PRがこのfeatureのものか、子の前提作業、子が既に実装済みか）はworkerが返す。
- stage PRと実装PRのfeature branchへのmerge、merge直後の次工程の開始、子Issueのcloseは、
  この起動によって保守者から委ねられる。merge条件の正本は`loop.md`の§4にある
  （headのcheckがすべて通過、PR作者以外のreviewerのreview、その後のreview対応workerが変更なしを返したこと）。
- PRイベントの購読や定期的な確認は、この起動したsessionが待つための手段としてだけ使う。
  上のAutomation boundaryが除くのは、repositoryに置いて人の起動なしにagentを動かす仕組みであり、
  それは引き続き使わない。
- integration PRはmergeしない。merge可能になった時点で保守者へ報告して止まる。
- integration PRの修正PRは、integration PRを開いてから3本まで、`main`の再取り込みは2回までとし、
  再取り込みで数え直さない。上限に達したら、その時点の状態を報告して止まる。
- 要求者の判断が要る質問、承認済み成果物の変更が要る指摘、収束しないreview、工程選択で
  一意に決まらない状態では止まる。再度起動すれば、GitHub上の事実から続きを進める。

手順は`.agents/skills/sdd-autopilot/references/loop.md`に置く。

## Failure behavior

feature特定で候補が複数ある、Issueが仕様として書かれていない、など工程を一意に選べない場合は
推測せずユーザーへ確認する。review修正は対象PRのheadへ積む。同じ親を同時に渡された二つの
runが同じ子を選ぶことはあり得るが、後から出たPRをreviewで閉じる。

## Verification

- Agent Skill validator: frontmatter、skill名、参照先の整合性
- `task check`: repository全体
- GitHub実機確認: timeline参照、native sub-issues、parent取得、
  feature向けPRでのCI、default branch merge時のparent close
