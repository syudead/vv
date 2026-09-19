# 実行計画: Issue を引き継ぎ面にしたエージェント非依存 SDD

- ステータス: リポジトリ実装済み。独立仕様レビュー、GitHub実機確認、外部Routine/label削除待ち
- 最終更新: 2026-09-19
- 対象: Claude Routine と PR マージ連鎖で動く `/sdd-next` を廃止し、親 Issue を
  通常は`specify -> plan -> tasks`、UI Issueは`specify -> plan -> design -> tasks`の工程間で
  引き継ぐ方式へ移行する

## 実装状況（2026-09-19）

リポジトリ内の共通workflow、ローカル状態判定と契約テスト、agent adapter、全PR向けCI設定、
設計・仕様・運用文書の置換、旧`/sdd-next`とusage hookの撤去は完了した。共通workflow、実行script、
契約testはそれぞれ`docs/agent-workflows/`、`scripts/issue-handoff/`、`tests/issue-handoff/`に置き、
Spec Kit管理下の`.specify/`から分離した。workflow test、local-dev test、Web format/lint/build/test
156件は成功している。

GitHub MCPで`syudead/vv`へのadmin/push権限、Issue/PRのread、PR作成能力を確認した。2026-09-19
時点でopen PRは0件、`sdd`付きopen Issueは0件である。Issue #46のnative sub-issues APIをreadし、
空配列を取得できることも確認した。一方、現在公開されているGitHub MCP toolにはsub-issueの
追加・解除mutationとrepository labelの削除操作がない。このため、通常Issueを代わりに作ることはせず、
native sub-issue作成の実機確認とlegacy `sdd` label削除は未完了としている。外部Claude Routineの
停止・削除もGitHub MCPの責務外である。local環境には`make`とGoがないため、完全な`make check`も
実行していない。

また、実装後の最終監査で既存の仕様品質規則Q-6に必要な独立reviewが未実施と判明した。要求原文と
Success Criterionの対応表はspecへ補完したが、仕様作成者とは別のreviewerによる承認は未完了である。
これは今回の実装順序上のprocess gapであり、完了扱いにはしない。

## 1. 目的

現在のハーネスは、`sdd` ラベル付き PR のマージを Claude Routine の起動契機にし、Claude
セッションが対象 feature の特定、branch の復元、次工程の決定、Spec Kit コマンドの実行、PR
作成とマージを担う。そのため、リポジトリ内に判定スクリプトがあっても、工程の継続は Claude
Routine、PR trigger、`<github-trigger-context>`、Claude 固有 skill に依存している。

移行後は、各工程を任意のコーディングエージェントへ個別に依頼する。エージェントは指定された
親 Issue と feature branch のマージ済み成果物を読み、1 工程だけ実行して feature branch 向け
PR を作り、そこで終了する。次のエージェントを自動起動しない。

長寿命 feature branch と sub-branch の topology は維持する。branch 名、Issue 番号、feature
番号の対応規則は設けない。

## 2. 役割と正本

### 2.1 親 Issue

親 Issue は機能要求と SDD 工程の到達点だけを持つ。

```markdown
## SDD

- [x] Spec: `specs/006-search/spec.md`
- [x] Plan: `specs/006-search/plan.md`
- [x] Design: `specs/006-search/ui-design.md` <!-- UI Issue のみ -->
- [ ] Tasks
- Next: `tasks`
```

親 Issue 本文へ次の情報は転記しない。

- stage PR、実装 PR、統合 PR の番号や状態
- feature branch 名
- 子 Issue 一覧や子 task の進捗
- retry 回数、agent、session 情報

stage/implement PRの通常参照はIssueのtimeline、`Closes`を持つ統合PRはDevelopment表示、子Issueは
GitHubのsub-issues、branch間の関係はPRのhead/baseを正本とする。親IssueのSDD節は工程の要約であり、
成果物そのものではない。
`Next`は未完了のSDD成果物工程がある間だけ置き、taskstoissues完了後は削除する。

### 2.2 リポジトリ成果物

feature branch にマージされた `spec.md`、`plan.md`、`ui-design.md`、`tasks.md` を工程状態の正本とする。
`spec.md` の `**Parent Issue**: #NNN` だけが feature と親 Issue の対応を表す。Issue 番号と feature
directory 番号は独立であり、一致や変換を仮定しない。

| feature branch の成果物 | 親 Issue の表示 | 次工程 |
| --- | --- | --- |
| `spec.md` なし | Spec 未完了 | specify |
| `spec.md` あり、`plan.md` なし | Spec 完了 | plan |
| 通常 Issueで`plan.md`あり、`tasks.md`なし | Spec / Plan 完了 | tasks |
| `ui` Issueで`plan.md`あり、`ui-design.md`なし | Spec / Plan 完了 | design |
| `ui` Issueで`ui-design.md`あり、`tasks.md`なし | Spec / Plan / Design 完了 | tasks |
| `tasks.md` あり | 通常はSpec / Plan / Tasks、UIはSpec / Plan / Design / Tasks完了 | taskstoissues |

spec が plan より後に変更された場合は Plan を未完了へ戻し、plan が tasks より後に変更された場合は
Tasks を未完了へ戻す。UI Issueではplanがui-designより後ならDesignを、ui-designがtasksより後なら
Tasksを未完了へ戻す。古い成果物は自動削除せず、後続工程のPRで更新する。`ui`ラベルはUI設計工程
を選ぶ既存のドメイン入力として維持し、SDD自動起動や状態管理には使わない。

### 2.3 GitHub の関係

- stage PR と実装 PR は feature branch を base にする。
- feature branch から `main` への統合 PR は、spec が feature branch へマージされて差分ができた
  後に作り、以後は機能全体の累積差分を表示する。
- stage PR は親Issueを`Refs #NNN`で関連付ける。
- 実装 PR は対応する子Issueを`Refs #NNN`で関連付ける。
- 統合 PR は親 Issue だけを `Closes #NNN` で関連付ける。子 Issue は対応する実装 PR が feature
  branch へマージされた時点で人が閉じ、GitHub sub-issues の進捗へ即時に反映する。
- branch 名は任意とし、対象 Issue、工程、base branch の判定に使わない。
- Spec完了後に親Issueから作業を再開するエージェントは、親Issueにリンクされた`base=main`のopenな
  統合PRを1件だけ取得し、そのheadをfeature branchとする。0件または複数件なら推測せず停止する。
- Spec未完了で統合PRがまだない場合は、親Issueを参照し、`**Parent Issue**`が一致する`spec.md`を
  追加するopenなspec PRを調べる。0件なら新規specifyを開始し、1件ならそのbaseをfeature branch、
  headを作業sub-branchとしてreview対応だけを行い、複数件なら停止する。

## 3. 完成後の利用フロー

### 3.1 Specify

1. 保守者が機能要求の親Issueを作り、SDD節を`Spec`未完了、Next=`specify`で初期化する。SDD起動・
   状態管理用の専用ラベルは不要とする。UI設計工程を選ぶ既存の`ui`ラベルは必要なIssueに付ける。
2. 任意のコーディングエージェントに親 Issue を指定する。
3. エージェントは最新`main`を取得し、`main`、全open統合PRのhead、openなspecify PRが持つspec
   directory prefixを調べ、現行の連番採番と衝突しないことを確認する。同時実行との競合を検出した
   場合は別番号へ自動で書き換えず、remoteを再取得してspecifyを最初からやり直す。
4. エージェントは`main`から長寿命feature branchを作ってremoteへpushし、そこから任意名の
   sub-branchを作ってcheckoutする。feature branch単独のpushは、統合PRを作れる差分がまだない
   specify中に限る例外としてAGENTS.mdへ明記する。
5. sub-branch上でIssue本文を入力に共通workflowのspecify工程だけを実行し、`spec.md`に
   `**Parent Issue**: #NNN`を記録する。現行コマンドはbranchを作成しないため、実行前の手順4を必須とする。
6. エージェントは sub-branch を push し、feature branch 向け spec PR を作る。PR 本文で親 Issue
   を通常参照する。PR作成後に同じdirectory prefixを追加するopenなspecify PRを再確認し、複数なら
   作成日時が最も古いPRだけを採用する。後発PRは閉じてremoteを再取得し、採番からやり直す。
   次工程は実行せず終了する。
7. 現行のspec品質規則に従って独立reviewを行い、人がspec PRをfeature branchへマージする。
8. spec のマージ後、feature branch から `main` への統合 PR を作り、本文に`Closes #<親Issue>`を
   記載する。以後この PR を再利用し、親Issueからfeature branchを見つける導線にする。
9. 親 Issue の SDD 節を Spec 完了、Next=`plan` に更新する。

手順 8 と 9 は spec PR をマージした保守者が行う。PR trigger やコーディングエージェントの自動起動
は使わない。

### 3.2 Plan

1. 保守者が同じ親 Issue を任意のコーディングエージェントに指定する。
2. エージェントは親Issueにリンクされた唯一のopenな統合PRからfeature branchを取得し、親Issueと
   feature branchの`spec.md`を読む。
3. エージェントは共通workflowのplan工程だけを実行する。
4. 任意名の sub-branch から feature branch 向け plan PR を作り、親 Issue を通常参照して終了する。
5. 人が plan PR をレビューして feature branch へマージする。
6. 親IssueのSDD節をPlan完了に更新する。通常IssueはNext=`tasks`、`ui` Issueは
   Next=`design`とする。

### 3.3 Design（UI Issueのみ）

1. 親Issueに`ui`ラベルがある場合だけ、planの次にdesign工程を実行する。
2. 任意のコーディングエージェントが既存のUI/interaction design契約に従って`ui-design.md`を作る。
3. 任意名のsub-branchからfeature branch向けdesign PRを作り、親Issueを通常参照して終了する。
4. 人がdesign PRをレビューしてfeature branchへマージする。
5. 親IssueのSDD節をDesign完了、Next=`tasks`に更新する。

### 3.4 Tasks

1. 同じ方法で共通workflowのtasks工程を実行し、その中でanalyzeまで完了する。
2. tasks PR を feature branch へマージする。
3. 親 Issue の SDD 節を Tasks 完了、Next=`taskstoissues` に更新する。
4. 共通workflowのtaskstoissues工程が`tasks.md`のtaskごとにIssueを作り、GitHubのsub-issue APIで親Issueへ
   追加する。現在のskillは通常Issueを作るだけなので、この工程の実装対象として変更する。
5. 重複判定はrepository全体の`TNNN`タイトルではなく、親Issueの既存sub-issues内のtask IDで行う。
   同じtask IDが親のsub-issuesに1件あれば再利用し、複数あれば作成せず停止する。
6. task IDは子Issue作成後に不変とし、既存taskの並べ替え、改番、別の意味への再利用を禁止する。
   task追加時は既存の最大番号より大きいIDを付ける。taskが不要になった場合は、`tasks.md`の元の行を
   `- [x] ~~TNNN 元のtask~~ (cancelled: 理由)`という解決済みの取り消し表記へ変え、対応する子Issueを
   `not planned`として閉じる。
   要件が実質的に変わる場合は既存IDを上書きせず、旧taskを取り消して新しいIDを追加する。
7. GitHub sub-issue操作は、GitHubの公式sub-issue APIを扱える各エージェントのnative GitHub
   integrationを使う。このrepositoryのClaude/Codex adapterはGitHub MCPを使い、`gh` CLIへは
   fallbackしない。対応能力がない環境では通常Issueだけを作って続行せず、変更前に停止する。
8. sub-issuesの作成・再利用が完了したら、親IssueのSDD節から`Next`行を削除する。SDD成果物工程は
   完了とし、以後の実装進捗はGitHubのsub-issuesだけで扱う。

### 3.5 Implement

1. 子 Issue を任意のコーディングエージェントへ個別に指定する。
2. エージェントは対応 task だけを実装し、feature branch 向け PR を作って終了する。
   同じPRで`tasks.md`の対応taskだけを完了にし、他taskのmarkerを変更しない。
   UI変更を含む場合は既存手順どおりスクリーンショットとvisual / interaction reviewをPRへ載せる。
3. 人が実装 PR をレビューして feature branch へマージする。
4. マージ後、人が対応する子Issueをcompletedとして閉じる。子Issueのcloseは「feature branchへ
   実装済み」を表し、親Issueのcloseは「`main`へ統合済み」を表す。
5. spec、plan、design、tasksを変更する必要が生じた場合は新規実装を止め、該当stageから順に
   feature branch向けPRを作り直す。改訂開始時に保守者が親Issueの該当stage以降を未完了へ戻し、
   `Next`を最初のstale stageに設定する。各stageのmerge後は通常フローどおりSDD節を進める。既存task
   IDを保ったままtasksを更新し、taskstoissuesを再実行して、新規taskの子Issue追加、説明だけ変わった
   既存子Issueの本文更新、不要taskの`not planned` closeを行い、完了後に`Next`を再び削除する。

### 3.6 Integrate

1. 全 task が `tasks.md` で完了し、対応する実装 PR が feature branch へマージ済みであることを
   人が確認する。
2. 最新`main`がfeature branchに含まれていない場合、feature branchから同期用sub-branchを作り、
   最新`main`をmergeして競合を解消し、feature branch向けPRとしてレビュー・マージする。feature
   branchをrebaseやforce-pushで書き換えない。
3. 最新`main`を含むfeature branch全体で`make check`と必要なUI reviewを実行する。
4. 全sub-issueがcompletedまたはnot plannedで閉じていることを確認し、人が既存の統合PRをレビューして
   `main`へマージする。
5. GitHubが統合PRの`Closes`に従い、親Issueを閉じる。

完成判定、統合 PR の merge、Issue の close を独自 workflow で制御しない。

### 3.7 中止と再開

- spec PRを未マージで却下して機能自体を中止する場合、保守者はspec sub-branchとまだ成果物を持たない
  feature branchを削除し、親Issueを理由付きで閉じる。
- spec PRを維持したまま修正する場合は同じPRのheadへ積む。誤って閉じた場合はbranchを削除せず
  同じPRをreopenする。feature branchのpush後、spec PR作成前に処理が止まった場合、そのbranchは
  親Issueから一意に再発見できないため再利用せず、保守者が孤立branchを確認して削除してから
  specifyをやり直す。
- plan、design、tasks、implement PRを未マージで閉じた場合、そのhead branchは削除してよい。
  親Issueとfeature branchは維持し、後続エージェントは最新feature branchから新しい任意名branchで
  同じ工程をやり直す。
- 統合PRはfeatureの生存中に常に1件だけopenに保つ。誤って閉じた場合は同じPRをreopenし、新しい
  統合PRを重複作成しない。機能を中止する場合だけ統合PRと親Issueを理由付きで閉じる。
- これらのcleanupとreopenは保守者が行い、自動retryやbranch名によるattempt管理は導入しない。

## 4. エージェント共通契約

エージェントは次を行う。

- 明示された親 Issue または子 Issue を読む。
- 開始前に、repositoryのread/push、Issueのread、PRのread/createに必要なGitHub接続があることを
  確認する。taskstoissuesではIssueのwriteとsub-issue操作、review修正では既存PR headへのpushも
  必須とし、不足していればrepositoryを変更する前に停止する。
- Spec完了後の親Issueでは唯一のopenな統合PR、子Issueでは親sub-issueを経由して統合PRを取得し、
  そのheadをfeature branchとして扱う。Spec未完了で統合PRがない場合だけは、親Issueを参照する
  openなspec PRが0件なら新規specify、1件ならreview対応、複数件なら停止する。
- feature branch のマージ済み成果物を読む。
- feature branchの`specs/*/spec.md`から`Parent Issue`が対象親Issueと一致するdirectoryを1件だけ
  解決する。0件または複数件なら停止し、親IssueのSDD節と不一致なら作業を始めず保守者へ差分を
  報告する。親Issueの修正後に改めて実行する。
- 解決したpathを`SPECIFY_FEATURE_DIRECTORY`として明示し、branch名や前回sessionの
  `.specify/feature.json`に依存せずdownstream commandを実行する。
- 現在不足している指定工程を 1 つだけ実行する。
- 任意名の sub-branch を作り、feature branch 向け PR を作る。
- PR 本文で対象 Issue を通常参照する。
- 検査結果と残課題を PR 本文へ書く。
- PR 作成後は merge を待たず、次工程を実行せず終了する。
- openなstage/implement PRにreview修正がある場合は、新しい工程やPRを始めず、そのPRのheadへ
  必要な修正をpushして終了する。別種類のエージェントへ引き継いでも同じPRを継続する。

エージェントは次を行わない。

- branch 名から Issue や工程を推測しない。
- Issue 番号から feature directory 番号を作らない。
- 専用 JSON、状態ファイル、agent packet、session 情報を commit しない。
- PR merge を契機に自分自身または別エージェントを起動しない。
- 親 Issue 本文へ PR 一覧や子 Issue 一覧を転記しない。

工程の正規手順は`docs/agent-workflows/{specify,plan,design,tasks,taskstoissues,implement}.md`へ置き、
既存templateとscriptをそこから参照する。AGENTS.mdは親/子Issueの読み方とこのdirectoryへの導線だけを
持つ。`.claude/skills/`などのエージェント固有skillを残す場合は、共通workflowを参照する薄い
adapterとする。`.specify/init-options.json`の`ai`/`integration`はSpec Kit導入・更新時のmetadataに
限定し、実行時のエージェント選択やbranch操作には使わない。

## 5. 自動化の境界

Claude Routine、工程またはエージェントを起動する`pull_request` trigger、branch名を条件に工程を
起動する`push` trigger、定期reconcileは使用しない。検証だけを行うCIの`pull_request` triggerは
全PRに対して維持する。

親 Issue の SDD 節は各 stage PR を feature branch へマージした保守者が更新する。更新内容は
成果物の有無から一意に決まり、PR や子 Issue の情報を転記しない。更新を忘れても、次のエージェント
は feature branch の成果物から不一致を検出して停止する。エージェントがIssue本文を暗黙に直して
続行せず、保守者がSDD節を修正してから再実行する。

常駐・イベント駆動の制御は置かない。各エージェントの開始時preflightではGitHubをread-onlyで
確認し、その後の状態判定はcheckoutしたfeature branchだけで行う。

- 各エージェントのnative GitHub integrationで、親Issueまたは親sub-issueから統合PRとfeature
  branchを一意に解決するread-only preflight手順
- 同じ工程のopen PRがないことのread-only確認
- feature branchの成果物から次工程を表示するローカルread-onlyコマンド
- stage ごとの変更可能ファイルと成果物の検査
- tasks と子 Issue の重複作成防止
- 全体の `make check`

GitHub preflightを行う共通repositoryコマンドは作らず、認証とAPI差異は各agent adapterに閉じ込める。
ローカル状態判定コマンドはGitHubへアクセスしない。preflightと検査はPRを作成、merge、Issueを編集、
close、reopen、またはエージェントを起動しない。
工程本体が行うPR作成、保守者が行う親IssueのSDD節更新、taskstoissuesが行うsub-issue作成は
別責務として明示する。

## 6. 実装計画

### Phase 0: CI bootstrap

- [ ] 新方式のfeature branchを作る前に、独立した`main`向けPRで`.github/workflows/ci.yml`の
      `pull_request.branches: [main]`を外し、base branch名に依存せず全PRでCIが動くようにする。
      `push`の`main`限定は維持する。
- [ ] このbootstrap PRは既存CIが動く`main`向けでレビュー・検証し、merge後にPhase 1へ進む。

### Phase 1: 共通契約

- [ ] 新方式の設計文書を作り、design index からリンクする。
- [ ] 親IssueのSDD節をSpec / Plan / Tasks / Nextと、UI IssueだけのDesignに限定したテンプレートを
      定義する。
- [ ] feature/sub-branch、stage PR、統合 PR、親/子 Issue の責務を契約にする。
- [ ] エージェント非依存の正規手順を`docs/agent-workflows/`へ工程別に置き、AGENTS.mdと既存Claude
      skillをそこへの導線へ縮める。
- [ ] 親IssueのNext値を`specify | plan | design | tasks | taskstoissues`に限定し、slash command名を
      保存しない。
- [ ] branch 名、Issue 番号、feature 番号を対応判定に使わないことを fixture で検証する。
- [ ] AGENTS.mdの「pushしたfeature branchにはmain向けPR」の規則へ、spec PRがmergeされて統合PRを
      作れるようになるまでの長寿命feature branchだけを例外として追加する。
- [ ] 現行`speckit-specify`がbranchを作らないことを契約へ記録し、feature/sub-branchのcheckout後に
      実行する順序をテストする。

### Phase 2: read-only 状態判定

- [ ] Spec完了後の親Issueまたは子IssueからGitHub標準のIssue/PR関係をたどり、唯一のopenな統合PRとfeature
      branchを解決するagent共通手順を作る。repository内にGitHub接続コマンドは作らず、0件・複数件は
      fail-closedにする。
- [ ] spec未マージ時だけ、親Issueを参照して対象`spec.md`を追加するopenなspec PRが0件なら新規開始、
      1件ならfeature branchと作業headを復元してreview対応、複数件なら停止することを検証する。
- [ ] feature branch上の`Parent Issue`からfeature directoryを一意に解決し、親IssueのSDD節と
      照合する。0件・複数件・path不一致をfixtureで検証する。
- [ ] 明示されたfeature branchの`spec.md`、`plan.md`、`ui-design.md`、`tasks.md`から次工程を表示する。
- [ ] spec、plan、ui-design、tasksの欠落と更新順によるstaleを検査する。
- [ ] checkout後の工程状態判定がGitHub API、Issue本文、branch名、agent情報へ依存しないことを
      検証する。GitHubを読むpreflightとは責務を分ける。
- [ ] GitHub関係のfixtureを各adapterの契約テストに渡し、repositoryの状態判定テストからnetworkと
      GitHub認証を排除する。
- [ ] 出力は人とエージェントが読める表示にし、永続状態ファイルを作らない。

### Phase 3: stage PR 運用

- [ ] specify、plan、design、tasksの各工程を1 PRに限定する共通手順を作る。
- [ ] stage PR の base が feature branch であることを検査する。
- [ ] 同じ工程の open PR がある場合は新規 PR を作らず報告して終了する。
- [ ] review修正は既存PRのheadへ積み、別エージェントへ渡しても新規PRや次工程を始めない手順を作る。
- [ ] PR 本文の通常 Issue 参照と、GitHub timeline での関連表示を実機確認する。
- [ ] branch 名を変えても検査結果が変わらないことを確認する。
- [ ] 新規specの採番時にmain、openな統合PRのhead、openなspecify PRの変更pathを照合し、prefix
      競合時は再取得してやり直すことを検証する。

### Phase 4: taskstoissues と実装

- [ ] 現行`speckit-taskstoissues`を、Issue作成後にGitHub sub-issue APIで親Issueへ追加する実装へ
      変更する。GitHub MCPに対応toolがない環境では作成前に停止する。
- [ ] 既存のopen/closed sub-issueを親Issueのsub-issues内のtask IDで再利用し、repository全体の
      同名taskと混同せず重複作成しない。
- [ ] 子 Issue の実装 PR が feature branch を base にすることを検査する。
- [ ] 実装PRが対応taskだけを`tasks.md`で完了にし、他task markerを変更しないことを検査する。
- [ ] 子 Issue 一覧を親 Issue 本文へ複製しないことを確認する。
- [ ] 実装PRのfeature branchへのmerge後に対応する子Issueをcompletedで閉じ、sub-issue進捗へ反映する
      手順を作る。
- [ ] 子Issue作成後のtask IDを固定し、追加は単調増加、不要taskは取り消し表記と`not planned` close、
      実質的な要件変更は新IDにすることをfixtureで検証する。
- [ ] SDD成果物の改訂中は新規実装を止め、taskstoissues再実行で既存sub-issuesを更新・追加・close
      できることを検証する。

### Phase 5: 統合 PR

- [ ] spec PR の feature branch へのマージ後、feature branch から `main` への統合 PR を 1 件作る。
- [ ] stage/implement PR の merge に伴い統合 PR の差分が累積することを確認する。
- [ ] 統合 PR 本文に親 Issue の `Closes` が1件存在し、子Issueの`Closes`を持たないことを検査する。
- [ ] 最新`main`を同期用sub-branch経由でfeature branchへ取り込み、競合解消後に全体検査をやり直す。
- [ ] 統合 PR の `main` への merge 時に親 Issue が閉じることを実機確認する。
- [ ] feature branch 全体の検査と UI review を統合 PR のレビュー手順へ入れる。

### Phase 6: CI 適用範囲の確認

- [ ] main向け統合PR、feature向けstage PR、feature向けimplement PRの3種類で同じ必須検査が起動する
      ことを実機確認する。
- [ ] CIのconcurrency keyがPRごとに独立し、同じfeature branchをbaseにする別PR同士が相互cancel
      されないことを確認する。

### Phase 7: 任意エージェントでの実機確認

- [ ] 1 つの親 Issue について specify を実行し、spec merge後に統合PRを作る。
- [ ] 別セッションかつ別種類のエージェントで plan を実行する。
- [ ] `ui`親Issueを使い、別実行のdesignがtasksより先に入ることを確認する。
- [ ] 前段の会話履歴、session ID、ローカル worktree、Claude Routine なしで成功することを確認する。
- [ ] 任意名 branch でも feature/sub-branch topology が PR の head/base で維持されることを確認する。

### Phase 8: 旧ハーネス撤去

- [ ] Claude Routine を停止し、実機確認後に削除する。
- [ ] `sdd` ラベル付き PR merge trigger を削除する。
- [ ] `.claude/skills/sdd-next/` の target、guard、branch 復元、hop/retry、自動 merge を削除する。
- [ ] `Makefile`の`test-sdd`と`.github/workflows/ci.yml`の「ハーネスの判定テスト」を、新しい
      read-only preflight、成果物状態判定、workflow契約のテストへ置き換える。削除済みの
      `.claude/skills/sdd-next/tests/run.sh`を参照するtargetを残さない。
- [ ] `claude/sdd-*` というエージェント固有 branch 命名を必須契約から削除する。
- [ ] `docs/references/sdd-routine.md` を廃止済み資料へ変更する。
- [ ] AGENTS.md、設計文書、仕様、契約、quickstart、tech debt を新方式へ更新する。
- [ ] 旧 active plan `003-sdd-loop-harness.md` を置換済みとして completed へ移す。

## 7. 受け入れ条件

- Claude Routine、工程起動用PR trigger、定期reconcileなしで、Issueを指定して各工程を個別に実行できる。
- specify と plan を異なるコーディングエージェントで実行できる。
- 親Issue本文には要求とSpec / Plan / Tasks / Next、UI IssueだけのDesignがあり、PR一覧と子Issue
  一覧がない。
- 親IssueのNextはエージェント固有command名ではなく、共通workflowのstage名である。
- PR との関係は GitHub timeline、子 Issue との関係は GitHub sub-issues に一元化されている。
- 親Issueまたは子Issueだけを渡された新しいエージェントが、GitHub標準の関連から統合PRとfeature
  branchを一意に取得できる。
- spec未マージ時も、親Issueにcross-referenceされたopenなspec PRからfeature branchと作業headを
  一意に取得できる。
- 長寿命 feature branch と sub-branch 運用が維持される。
- branch 名、Issue 番号、feature 番号に対応規則がない。
- 各 stage/implement PR は feature branch を base にする。
- 統合 PR だけが `main` を base にし、親 Issue だけの `Closes` を持つ。
- 子Issueは対応実装がfeature branchへマージされた時点で人が閉じ、親Issueは統合PRのmergeで閉じる。
- 全stage/implement PRで、base branch名に依存せずCIが起動する。
- task IDは子Issue作成後に改番・再利用されず、追加・変更・取り消しが既存sub-issuesと矛盾しない。
- 統合前に最新`main`をfeature branchへPR経由で取り込み、全体検査を再実行する。
- 専用状態 JSON、独自 PR マーカー、機械管理コメント、agent packet を追加しない。
- 現行`speckit-specify`を正しいbranchへ移動してから実行し、現行`speckit-taskstoissues`が作るIssueを
  親Issueのsub-issuesへ追加できる。
- review修正を別エージェントへ渡しても同じPRが更新され、次工程や重複PRが始まらない。
- 旧`sdd-next`削除後も`make check`と全PRのCIが、置換後のworkflow契約テストを含めて成功する。

## 8. 移行とロールバック

1. 旧 Routine を動かしたまま、専用の検証 Issue と任意名 branch で新方式を構築する。
2. 新方式でspecify、別エージェントのplan、UI Issueのdesign、tasks、子Issue1件の実装、統合PR
   まで検証する。
3. 実装PRのfeature branchへのmergeで子Issueが閉じ、統合PRの`main`へのmergeで親Issueが閉じることを
   確認する。
4. Claude Routine を停止し、旧方式で open な PR を完了または明示的に閉じる。
5. 旧ハーネスを撤去する。
6. 撤去後に問題が出た場合は自動化を戻さず、Issue 指定による各工程の手動実行へ戻す。

## 9. この計画で行わないこと

- 親 Issue 本文へ PR、子 Issue、branch、agent、retry 情報を同期すること。
- Issue 本文へ spec、plan、tasks の全文を保存すること。
- branch 名や Issue 番号から feature を推測すること。
- PR merge 後に次のコーディングエージェントを自動起動すること。
- GitHub イベントや schedule で Issue 本文を編集すること。
- Claude と Codex の出力形式を独自 protocol で統一すること。
- 製品コードの機能開発と同時にこの移行を行うこと。

## 10. 採用時に失うもの

- stage PRのmerge後に次工程が自動起動する連続実行。各工程は保守者がIssueを指定して開始する。
- tasksやimplement PRの自動merge。すべてのstage/implement PRを人がレビューしてmergeする。
- hop上限、retry上限、定期reconcile、停止Issue作成による無人復旧。失敗は対象IssueまたはPR上で
  人が確認し、同じ工程を再実行する。
- 子Issueのcloseを「`main`へ出荷済み」と読む運用。新方式では子Issueのcloseはfeature branchへの
  実装完了を表し、機能全体の出荷状態は親Issueのopen/closedで判断する。
- feature branchのpushからspec PR作成までの間に実行が途切れた場合のbranch自動復元。親Issueとの
  GitHub標準の関係がまだないため、孤立branchは保守者が削除してspecifyをやり直す。

代わりに、工程間の状態はfeature branchの成果物、Issue/PR/sub-issueの関係はGitHub標準機能だけで
表現され、特定エージェントのsession、Routine、branch命名、独自状態データを復元しなくてよくなる。
