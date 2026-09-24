# Issue・PR単位のLLMコスト可視化（検討）

状態: 提案。未実装。対象はClaude CodeとCodexに限る。

## Decision

agentが作業で使ったtokenを、agent自身のsession logから集計し、作業先のPR（PRを
作らない作業ではIssue）に一件の台帳commentとして記録する。金額はrepositoryに置いた
単価表でtokenから計算する「API定価換算」とし、実際の請求額とは扱わない。親Issue単位の
合計は、そのIssueに紐づくPRとIssueの台帳を後から合算して出す。

```text
agent session log ──scripts/llm-usage──> 台帳comment（PR / Issue）
                                           │
                        親Issue合計 <──合算──┘（stage PR、子IssueのPR、integration PR）
```

## Why

- **tokenを正本にする。** Claude CodeもCodexも定額プランで使う場合はtoken単位の請求が
  存在せず、Codexのlogには金額がそもそも無い。両方に共通して得られるのはtoken数なので、
  それを記録し、金額は単価表から導出する。単価改定や換算方法の変更は再計算で済む。
- **agent自身が報告する。** CIはagentのsession logを読めない。
  [Issue handoff SDD](sdd-loop-harness.md)では一回のagent実行が一工程を担い一つのPRを
  開くので、「sessionとPRの対応」を最も確実に知っているのはそのagentである。
- **記録先はGitHub上のcommentにする。** repositoryのfileに書くと長寿命feature branchで
  毎回conflictし、コード履歴にも混ざる。外部DBやOpenTelemetry collectorは運用する
  基盤を増やす。PR/Issue commentなら人がその場で読め、機械でも集められる。

## Data sources

どちらもlocal fileのJSONLで、agentが自分のsessionのfileを読める。

### Claude Code

`~/.claude/projects/<cwdをエスケープした名前>/<sessionId>.jsonl`。subagentの記録は
`<sessionId>/subagents/`配下に別fileとして置かれるので合算対象に含める。

- `type: "assistant"`の行の`message.usage`に`input_tokens`、`output_tokens`、
  `cache_read_input_tokens`、`cache_creation_input_tokens`があり、`cache_creation`で
  5分/1時間のcache書き込みが分かれる。`message.model`がmodel名を持つ。
- **一つのAPI応答がcontent blockごとに複数行へ分かれ、同じ`usage`を繰り返す。**
  `message.id`（または`requestId`）で重複を除かないと数倍に数える
  （このdocument作成時のsessionでも4応答が6行に記録されていた）。
- 各行に`sessionId`、`gitBranch`、`timestamp`がある。

hookの入力にも`transcript_path`が渡るので、同じ集計器をhookから呼ぶこともできる。

### Codex

`~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`。

- `session_meta`がsession idとgit情報、`turn_context`がmodelを持つ。
- `event_msg`の`token_count`が`input_tokens`、`cached_input_tokens`、`output_tokens`、
  `reasoning_output_tokens`の**累積値**を持つ。差分ではないので、期間の使用量は
  「期間末の累積 − 期間初めの累積」で求める。
- `codex exec --json`では`turn.completed`がturn単位の`usage`を出す。

Codexのlog形式は版によって変わってきたため、実装時に対象版の実物で確認する。

## Ledger format

一つのPR（またはIssue）に台帳commentを一件だけ置き、`<!-- llm-usage:v1 -->`で識別して
上書き更新する。人向けの表と、機械向けのJSONを同じcommentに持つ。

```markdown
<!-- llm-usage:v1 -->
### LLM usage（API定価換算）

| tool | model | session | input | cache read | cache write | output | USD |
|---|---|---|---:|---:|---:|---:|---:|
| claude | claude-opus-… | f21e339a | 1.2k | 3.4M | 210k | 48k | 7.91 |
| codex | gpt-… | 0199ab… | 800k | 2.1M | – | 35k | 2.40 |
| **合計** | | | | | | | **10.31** |

<details><summary>raw</summary>

<!-- llm-usage-data
{"version":1,"prices":"2026-09-24","rows":[{"tool":"claude","session":"…","model":"…",
 "from":"…","to":"…","input":…,"cache_read":…,"cache_write_5m":…,"cache_write_1h":…,"output":…}]}
-->
</details>
```

- 行の一意キーは`(tool, session, model, from)`。一つの期間の中でmodelが切り替わった場合
  （model変更、subagentが別modelを使う場合）は、応答・turnごとのmodelで分けてmodel別の行を
  作る。単価はmodelごとに違うため、一行に複数modelのtokenを混ぜない。
- 同じsessionが同じPRへ再報告したら同じキーの行を置き換え、
  別sessionなら行を足す。review修正で別agentが同じPRに積んだ作業も別行として残る。
- JSONにはtoken数だけを持ち、金額は表示時に単価表から計算する。`prices`は計算に使った
  単価表の版。
- 測れなかったsession（logが見つからない等）は`"unmeasured": true`の行として残し、
  0円と区別する。

## Attribution

- 既定では1 session = 1 PRとみなし、sessionの全使用量をそのPRに付ける。handoffの
  「一工程を実行し、PRを一つ開いて終わる」という前提に合う。
- 一つのsessionで複数のPRを扱った場合は、報告時に`--since <timestamp>`で期間を切り、
  台帳行に`from`/`to`を残す。台帳行は期間の合計しか持たず、重なった区間の使用量は後から
  求められないので、**同じsessionの期間は重ねない**。次のPRへの報告は前のPRへ報告した
  `to`を`--since`に渡して始める。合算時に同じsessionの期間の重なりを見つけたら、推測で
  差し引かずに集計結果へ警告として出す。
- PRを作らない作業はそのIssueに台帳commentを置く。`issue-spec`でのIssue本文作成は
  そのIssueへ、PRを作らずに終わる`plan-to-issues`工程は親Issueへ記録する。
- 最後の報告以降のtoken（PR作成そのものや、その後の短い応答）は次の報告まで載らない。
  pushのたびに再報告するので、取りこぼしは最後の一往復程度に収まる。

## Aggregation

親Issueの合計は次を合算する。

- 親Issueを`Refs`/`Closes`するPR（Plan、Design、integration、同期PR）
- 親のnative sub-issueと、それを`Refs`する実装PR
- 親Issueと子Issue自身に置かれた台帳comment

合算は`scripts/llm-usage report --issue <n>`として手元で実行できる形を先に作る。
結果を親Issueのcommentとして常時表示したくなった段階で、GitHub Actionsの
`workflow_dispatch`とPRのmerge（`pull_request: closed`）で同じ集計を実行し、親Issueの
集計commentを上書きするworkflowを足す。このworkflowはcommentを読み書きするだけで
agentを起動しないので、[Automation boundary](sdd-loop-harness.md#automation-boundary)
に反しない。親Issueの本文には書かない（本文は仕様だけを持つ）。

## Pricing

`scripts/llm-usage/prices.json`に、modelごとの100万token当たり単価（input、cache read、
cache write 5分/1時間、output）と適用開始日を持つ。更新は人がprovider公式の価格表を
見て行い、表にないmodelは金額を「不明」とし、token数だけを表示する。

- Claude: cache書き込みは5分と1時間で単価が違うので分けて数える。
- Codex: `cached_input_tokens`は`input_tokens`の内数として扱われるため、非cache分を
  `input − cached`として計算する。`reasoning_output_tokens`は`output_tokens`の内数で、
  別途加算しない（実装時に実物で確認する）。

## Collection flow

1. agentは`.agents/skills/issue-handoff`の最後、PRを作成・更新する直前に
   `go run ./scripts/llm-usage record --tool <claude|codex> [--since …]`を実行する。
   collectorは現在のsessionのlogを見つけ、台帳comment本文を標準出力へ出す。既存の
   台帳commentの本文を`--merge`で渡すと行を統合した本文を返す。
2. agentは自分のGitHub tool（MCP、`gh`など）で台帳commentを作成または上書きする。
   collector自体はGitHubへ書き込まないので、hostのGitHub手段に依存しない。
3. `self-review`の確認項目に「台帳commentを更新したか」を加える。

Claude Codeには`Stop`/`SessionEnd` hookで自動化する余地があるが、hookはMCPの
GitHub toolを使えず、cloud環境ではsession終了後にcontainerが消える。まずはskill手順で
全hostを揃え、hookは後から補助として検討する。

## Open questions

- Codex cloud（web）のtask containerで`~/.codex/sessions`を読めるか。読めない場合は
  `unmeasured`行で記録するか、ユーザーがUIの使用量を転記する運用にするか。
- Claude Code on the webのsession logが、subagent分も含めて`~/.claude/projects`に
  揃うか（このdocument作成時のsessionでは親sessionのfileは存在した）。
- レビューbot（`Claude Code Review`等）の実行分は、このrepositoryのagent sessionでは
  ないため今回の対象外とする。

## Verification

- collectorのunit test: 重複した`message.id`、subagent file、Codexの累積値、
  `--since`による分割、期間内のmodel切り替え、未知model、`--merge`での行の置き換え、
  期間の重なりの検出。
- 実機確認: Claude Code（CLI/web）とCodex（CLI）でそれぞれ一工程を実行し、台帳の
  token数がhostの表示（Claudeの`/cost`、Codexの`/status`）と一致すること。
