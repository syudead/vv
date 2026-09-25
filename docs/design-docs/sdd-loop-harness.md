# Issue handoff SDD

## Decision

SDDの工程継続をClaude RoutineやPRイベントに任せない。保守者が親Issueまたはnative
sub-issueを任意のコーディングエージェントへ明示的に渡し、エージェントは一工程だけを実行して
PRを作り、そこで終了する。

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

## Ownership

### Parent Issue

親Issueは仕様そのものとSDD成果物の到達点を持つ。要求、要件、受け入れ条件、Edge Cases、
対象外がここにあり、`.agents/skills/issue-spec`が書く。PR、branch、子Issue一覧、retry、
agent情報は書かない。

```markdown
## SDD

- [x] Plan: `specs/006-search/plan.md`
```

UI IssueだけはPlanの後にDesignを持つ。`Next`は
`plan | design | plan-to-issues`のいずれかで、子Issue作成後に削除する。
ただしSDD節は人向けの進捗表示であり、指定された工程を許可または禁止する状態機械ではない。

### Feature artifacts

共通skillは指定されたIssue、PR、branch、現在のcheckoutと、関連する成果物をそのまま読む。
featureディレクトリは`plan.md`を必ず持ち、`ui`Issueでは`ui-design.md`を加える。
Planが判断の根拠や契約を持つ場合は`research.md`・`data-model.md`・`contracts/`・
`quickstart.md`がそれに伴う（P-2: 書くことが無い成果物は作らない）。`spec.md`は
作らない。要求の正本は親Issueである。

既存の後続成果物が後から改訂された前段成果物を取り込んでいるかは自動推測しない。改訂時は保守者が
親IssueのSDD summaryを戻し、影響する工程をreviewed PRとして再実行する。

### GitHub

- stage PRは親Issueを`Refs`で通常参照し、feature branchをbaseにする。
- implementation PRは一つの子Issueを`Refs`で参照し、feature branchをbaseにする。
- integration PRだけが`main`をbaseにし、親Issueを`Closes`で参照する。
- childはimplementation PRがfeatureへmergeされた時点で人（[Autopilot](#autopilot)ではorchestrator）が
  completedとして閉じる。
- parentはintegration PRが`main`へmergeされた時点でGitHub標準動作により閉じる。

子Issueのcloseは「feature branchへ実装済み」、親Issueのcloseは「mainへ統合済み」を表す。

## Context

指定されたIssue、PR、branchと現在のcheckoutを作業文脈として使う。GitHub標準のIssue/PR参照や
native sub-issue関係は必要に応じて読むが、親からintegration PR、feature branch、feature directoryを
順番に復元するrepository固有の手順は一工程の実行には設けない（[Autopilot](#autopilot)は次の
行動を決めるためにだけこれらの事実を読む）。複数のPRや成果物を照合して一意性を判定しない。
要求された変更に本当に必要な情報が得られない場合だけユーザーへ確認する。

## Stage transitions

通常Issueは`plan -> plan-to-issues`、UI Issueは`plan -> design -> plan-to-issues`で進む。
仕様は親Issueとして先に書かれているので、工程には含めない。成果物stageは任意名sub-branchから
feature branch向けPRを一件作って終了し、人がreview、merge、親IssueのSDD節更新を行う
（[Autopilot](#autopilot)ではmergeとSDD節更新をorchestratorが行う）。

`plan-to-issues`は承認済みPlanから実装作業を直接native sub-issueとして作る。子Issueを作る直前に親の既存
sub-issuesを確認し、同じ作業が既にあれば作成しない。既存childの更新やcloseは対象Issueが明示された
場合だけ行う。

## Integration

Plan merge直後にfeature branchから`main`へのintegration PRを作り、featureの生存中は同じPRを使う。
全child解決後、最新`main`を長寿命feature branchへ直接mergeしてintegration PRを更新する。
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
`subissue-implementer`へ委譲し、self-reviewは実装とは別文脈の`self-reviewer`へ委譲する。
branch、全体検証、review指摘の修正、push、PRは親agentが所有する。この内部委譲は次工程を
起動せず、handoff stateも追加しない。対応しないhostは同等のbounded workerを使い、なければ
同じ作業を親agent自身で実行する。

## Autopilot

保守者が親Issueを明示して`.agents/skills/sdd-autopilot`を起動した場合に限り、上の工程を
統合PRのmerge直前まで連続して進める。これはClaude RoutineやPRイベント購読のworkflowでは
なく、保守者が開始した一つのsessionであり、終われば何も残らない。

- orchestratorは工程の作業をしない。各工程、self-review、review指摘の一巡ごとに新しい
  worker（`sdd-stage-worker`、`self-reviewer`、`pr-review-fixer`）を起動し、Issue番号・
  branch・pathだけを渡して、固定形式の短い結果だけを受け取る。長い会話でもorchestratorの
  文脈にはdiff、CI log、review本文、Issue本文が溜まらない。
- 次の行動は毎回GitHubとrepositoryの事実（feature branch、integration PR、`ui-design.md`の
  有無、native sub-issues、feature branch向けのopen PR）から決める。session stateやledgerは
  持たないので、会話の要約や再開で失うものがない。`## SDD`節は人向けの表示として更新するが、
  判断の入力にはしない。
- feature branch向けPRのmerge、`## SDD`節の更新、integration PRの作成、子Issueのclose、
  最新`main`のmergeは、この起動によって保守者から委ねられる。merge条件はCIがheadで成功、
  review botがheadをreview済み、その後のreview対応workerが変更なしを返したこと。
- PRイベントの購読や定期的な確認は、この起動したsessionが待つための手段としてだけ使う。
  上のAutomation boundaryが除くのは、repositoryに置いて人の起動なしにagentを動かす仕組みであり、
  それは引き続き使わない。
- integration PRはmergeしない。merge可能になった時点で保守者へ報告して止まる。
- 要求者の判断が要る質問、承認済み成果物の変更が要る指摘、収束しないreview、決定表に
  当てはまらない状態では止まる。再度起動すれば、GitHub上の事実から続きを進める。

手順は`.agents/skills/sdd-autopilot/references/loop.md`に置く。

## Failure behavior

親IssueのSDD節、artifact、GitHub relationshipは作業入力として利用できるが、相互の一致を実行条件に
しない。review修正は指定されたPR headへ積む。必要な対象が依頼にも現在のcheckoutにも存在しない場合は
推測せずユーザーへ確認する。

## Verification

- Agent Skill validator: frontmatter、skill名、参照先の整合性
- `task check`: repository全体
- GitHub実機確認: timeline参照、native sub-issues、parent取得、
  feature向けPRでのCI、default branch merge時のparent close
