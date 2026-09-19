# Issue handoff SDD

## Decision

SDDの工程継続をClaude RoutineやPRイベントに任せない。保守者が親Issueまたはnative
sub-issueを任意のコーディングエージェントへ明示的に渡し、エージェントは一工程だけを実行して
PRを作り、そこで終了する。

長寿命feature branchとsub-branchの構成は維持する。

```text
main
  <- feature branch                 integration PR（Closes parent）
       <- arbitrary stage branch    Spec / Plan / Design / Tasks PR
       <- arbitrary task branch     implementation PR
       <- arbitrary sync branch     latest main synchronization PR
```

branch名は識別子ではない。Issue番号とfeature directory番号にも対応規則を設けない。

## Why

旧方式は成果物から工程を導出できても、その実行開始、対象復元、retry、mergeがClaude Routine、
`<github-trigger-context>`、`sdd`ラベル、`claude/sdd-*`というbranch名に依存していた。別agentは
前sessionの状態を再現できず、repository内の判定scriptも外部Routineなしには前進できなかった。

新方式では状態を二種類に分ける。

- repository state: feature branchへmerge済みの`spec.md`、`plan.md`、`ui-design.md`、`tasks.md`
- GitHub relationships: Issue timeline、integration PRの`Closes`、PRのhead/base、native sub-issues

agent固有session、packet、独自JSON、branch命名はどちらにも含めない。

## Ownership

### Parent Issue

親Issueは要求とSDD成果物の到達点だけを持つ。PR、branch、子Issue一覧、retry、agent情報は書かない。

```markdown
## SDD

- [x] Spec: `specs/006-search/spec.md`
- [x] Plan: `specs/006-search/plan.md`
- [ ] Tasks
- Next: `tasks`
```

UI IssueだけはPlanとTasksの間にDesignを持つ。`Next`は
`specify | plan | design | tasks | taskstoissues`のいずれかで、sub-issuesとの照合完了後に削除する。

### Feature artifacts

`spec.md`の`**Parent Issue**: #NNN`だけがfeature directoryと親Issueを対応させる。番号の一致は
要求しない。共通skillは明示されたdirectoryの成果物を直接読み、不足している次工程を判断する。
branch名や前回sessionの選択状態から対象を推測しない。

既存の後続成果物が後から改訂された前段成果物を取り込んでいるかは自動推測しない。改訂時は保守者が
親IssueのSDD summaryを戻し、影響する工程をreviewed PRとして再実行する。

### GitHub

- stage PRは親Issueを`Refs`で通常参照し、feature branchをbaseにする。
- implementation PRは一つの子Issueを`Refs`で参照し、feature branchをbaseにする。
- integration PRだけが`main`をbaseにし、親Issueを`Closes`で参照する。
- childはimplementation PRがfeatureへmergeされた時点で人がcompletedとして閉じる。
- parentはintegration PRが`main`へmergeされた時点でGitHub標準動作により閉じる。

子Issueのcloseは「feature branchへ実装済み」、親Issueのcloseは「mainへ統合済み」を表す。

## Discovery

Spec merge後は、親Issueを閉じるopenな`base=main` PRを一件取得し、そのheadをfeature branchとする。
子Issueから開始した場合はnative sub-issue APIで親を取得してから同じ手順を使う。0件または複数件なら
推測せず停止する。

Spec merge前だけはintegration PRが存在しない。親Issueを参照し、一致する`**Parent Issue**`を持つ
`spec.md`を追加するopen PRを調べる。0件なら新規Specify、1件ならそのPRのreview対応、複数件なら
停止する。

GitHub discoveryは各agentのnative integrationが行う。repository共通のGitHub接続commandは作らない。
ClaudeとCodexはGitHub MCPを使い、`gh`へfallbackしない。

## Stage transitions

通常Issueは`specify -> plan -> tasks -> taskstoissues`、UI Issueは
`specify -> plan -> design -> tasks -> taskstoissues`で進む。各stageは任意名sub-branchからfeature
branch向けPRを一件作って終了し、人がreview、merge、親IssueのSDD節更新を行う。

`taskstoissues`はrepositoryを変更しない。親のnative sub-issues内だけでtask IDを照合し、存在しない
childを作成して親へ追加する。task IDはchild作成後に不変で、追加は単調増加、取消は
`- [x] ~~TNNN ...~~ (cancelled: reason)`とnot-planned closeで表す。

## Integration

Spec merge直後にfeature branchから`main`へのintegration PRを作り、featureの生存中は同じPRを使う。
全child解決後、最新`main`を同期用sub-branchへmergeし、そのPRをfeatureへmergeする。feature全体の
`make check`と必要なUI reviewを再実行してから、人がintegration PRをmergeする。rebaseやforce-pushで
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

正規手順は`.agents/skills/issue-handoff/`に置く。`.claude/skills`は
`.agents/skills`全体へのsymlinkとし、agent別の複製やadapterを持たない。

## Failure behavior

親Issue、artifact、GitHub relationshipが一致しない場合はfail closedとする。agentが親Issueを暗黙に
修正して続行せず、保守者が直してから再実行する。review修正は同じPR headへ積む。

feature branch push後、Spec PR作成前に停止するとGitHub標準関係がまだないため、branchを親Issueから
復元できない。そのbranchは保守者が削除し、Specifyをやり直す。この短い非原子的区間を埋めるための
独自markerや命名規則は導入しない。

## Verification

- Agent Skill validator: frontmatter、skill名、参照先の整合性
- `make check`: repository全体
- GitHub実機確認: timeline参照、native sub-issues、parent取得、integration PRからのbranch解決、
  feature向けPRでのCI、default branch merge時のparent close
