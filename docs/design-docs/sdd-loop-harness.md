# Issue handoff SDD

## Decision

SDDの工程継続をClaude RoutineやPRイベントに任せない。保守者が親Issueまたはnative
sub-issueを任意のコーディングエージェントへ明示的に渡し、エージェントは一工程だけを実行して
PRを作り、そこで終了する。

長寿命feature branchとsub-branchの構成は維持する。

```text
main
  <- feature branch                 integration PR（Closes parent）
       <- arbitrary stage branch    Spec / Plan / Design PR
       <- arbitrary task branch     implementation PR
       <- arbitrary sync branch     latest main synchronization PR
```

branch名は識別子ではない。Issue番号とfeature directory番号にも対応規則を設けない。

## Why

旧方式は成果物から工程を導出できても、その実行開始、対象復元、retry、mergeがClaude Routine、
`<github-trigger-context>`、`sdd`ラベル、`claude/sdd-*`というbranch名に依存していた。別agentは
前sessionの状態を再現できず、repository内の判定scriptも外部Routineなしには前進できなかった。

新方式では作業の文脈を二種類から読む。

- repository state: feature branchへmerge済みの`spec.md`、`plan.md`、`ui-design.md`
- GitHub relationships: Issue timeline、integration PRの`Closes`、PRのhead/base、native sub-issues

agent固有session、packet、独自JSON、branch命名はどちらにも含めない。

## Ownership

### Parent Issue

親Issueは要求とSDD成果物の到達点だけを持つ。PR、branch、子Issue一覧、retry、agent情報は書かない。

```markdown
## SDD

- [x] Spec: `specs/006-search/spec.md`
- [x] Plan: `specs/006-search/plan.md`
```

UI IssueだけはPlanの後にDesignを持つ。`Next`は
`specify | plan | design | plan-to-issues`のいずれかで、子Issue作成後に削除する。
ただしSDD節は人向けの進捗表示であり、指定された工程を許可または禁止する状態機械ではない。

### Feature artifacts

共通skillは指定されたIssue、PR、branch、現在のcheckoutと、関連する成果物をそのまま読む。
`spec.md`の親Issue表記は文脈の補助であり、対象を決めるための照合キーや実行条件にはしない。

既存の後続成果物が後から改訂された前段成果物を取り込んでいるかは自動推測しない。改訂時は保守者が
親IssueのSDD summaryを戻し、影響する工程をreviewed PRとして再実行する。

### GitHub

- stage PRは親Issueを`Refs`で通常参照し、feature branchをbaseにする。
- implementation PRは一つの子Issueを`Refs`で参照し、feature branchをbaseにする。
- integration PRだけが`main`をbaseにし、親Issueを`Closes`で参照する。
- childはimplementation PRがfeatureへmergeされた時点で人がcompletedとして閉じる。
- parentはintegration PRが`main`へmergeされた時点でGitHub標準動作により閉じる。

子Issueのcloseは「feature branchへ実装済み」、親Issueのcloseは「mainへ統合済み」を表す。

## Context

指定されたIssue、PR、branchと現在のcheckoutを作業文脈として使う。GitHub標準のIssue/PR参照や
native sub-issue関係は必要に応じて読むが、親からintegration PR、feature branch、feature directoryを
順番に復元するrepository固有の手順は設けない。複数のPRや成果物を照合して一意性を判定しない。
要求された変更に本当に必要な情報が得られない場合だけユーザーへ確認する。

## Stage transitions

通常Issueは`specify -> plan -> plan-to-issues`、UI Issueは
`specify -> plan -> design -> plan-to-issues`で進む。成果物stageは任意名sub-branchからfeature
branch向けPRを一件作って終了し、人がreview、merge、親IssueのSDD節更新を行う。

`plan-to-issues`は承認済みPlanから実装作業を直接native sub-issueとして作る。子Issueを作る直前に親の既存
sub-issuesを確認し、同じ作業が既にあれば作成しない。既存childの更新やcloseは対象Issueが明示された
場合だけ行う。

## Integration

Spec merge直後にfeature branchから`main`へのintegration PRを作り、featureの生存中は同じPRを使う。
全child解決後、最新`main`を同期用sub-branchへmergeし、そのPRをfeatureへmergeする。feature全体の
`task check`と必要なUI reviewを再実行してから、人がintegration PRをmergeする。rebaseやforce-pushで
長寿命feature branchを書き換えない。

## Automation boundary

CIはすべてのPRで検証するが、agentや次工程を起動しない。使用しないものは次のとおり。

- Claude Routine
- stage PR mergeを購読するworkflow
- `sdd` automation label
- schedule reconcile
- hop/retry state
- agent packet、result JSON、session state
- branch名によるfeature/stage判定

正規手順は`.agents/skills/issue-handoff/`に置く。

一工程を担当する親agentは、hostが対応している場合、その工程内の境界が確定した作業を
project-scoped subagentへ委譲してよい。Codexでは子Issueの実装とfocused testだけを
`.codex/agents/subissue-implementer.toml`のworkerへ委譲し、branch、全体検証、self-review、
push、PRは親agentが所有する。この内部委譲は次工程を起動せず、handoff stateも追加しない。
対応しないhostは同じ作業を親agent自身で実行する。

## Failure behavior

親IssueのSDD節、artifact、GitHub relationshipは作業入力として利用できるが、相互の一致を実行条件に
しない。review修正は指定されたPR headへ積む。必要な対象が依頼にも現在のcheckoutにも存在しない場合は
推測せずユーザーへ確認する。

## Verification

- Agent Skill validator: frontmatter、skill名、参照先の整合性
- `task check`: repository全体
- GitHub実機確認: timeline参照、native sub-issues、parent取得、
  feature向けPRでのCI、default branch merge時のparent close
