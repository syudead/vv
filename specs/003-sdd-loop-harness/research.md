# Phase 0 Research: SDD ループハーネス

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-13

方式の採否は[設計文書](../../docs/design-docs/sdd-loop-harness.md)で決定済みである。
本ドキュメントは、その設計を実装に落とすために残っていた未確定事項だけを扱う。
文書で閉じられるものは Claude Code の公式文書（code.claude.com/docs、2026-09-13 取得）で
閉じ、実機でしか確かめられないものは「プローブで確定」として quickstart の手順に送る。

---

## R-001: GitHub のイベントで cloud セッションを起動する手段

**Decision**: Claude Code の routine に **GitHub トリガー**（`pull_request.closed`）を付ける。
フィルタは「base branch = `main`」「labels に `sdd` を含む」「is merged = true」の 3 条件。
同じ routine に**日次のスケジュールトリガー**も付ける。

**Rationale**: routine の GitHub トリガーは Pull request と Release の 2 種類のイベントに
反応でき、PR には author／title／body／base／head／labels／is draft／is merged の
フィルタが使える（`/docs/en/routines` "Add a GitHub trigger"）。Issue イベントは無いので、
状態や合図はすべて PR で運ぶ。`opened` や `labeled` を購読しないことで、ハーネス自身が
開く `sdd` ラベル付き PR で自分が起きることを防ぐ（設計文書 5 章）。

制約として、GitHub webhook のイベントには routine ごと・アカウントごとの時間あたり上限が
あり、超えた分は捨てられる。本機能の発火は人のマージ頻度に等しいので実用上は届かない。
日次トリガーは取りこぼしの保険でもある。

**Alternatives considered**:

- GitHub Actions から routine の API トリガー（`/fire`）を叩く: トークンを Actions の
  secret に置く必要があり、動く部品が 2 系統になる。設計文書 10 章で却下済み
- Claude GitHub App の `@claude` メンション: PR コメントを起点にするので「マージ」を
  合図にできない

---

## R-002: cloud セッションからの GitHub 操作（組み込みツールと `gh`）

**Decision**: cloud セッションでは `gh` を前提にしない。スキルが cloud セッション組み込みの
GitHub ツールで PR 一覧を取得し、`${TMPDIR:-/tmp}/sdd-github/pulls-closed.json` と
`pulls-open.json` に置いてから、`sdd-guard.sh --github-dir` に渡す。PR 作成・Issue 作成・
label 付与・merge も組み込み GitHub ツールを通常経路にする。`gh api` は `--github-dir` を
省略したローカル検算フォールバックでだけ使う。

**Rationale**: 2026-09-13 のプローブで、cloud セッションには `gh` が無く
`gh: command not found` になった。一方で cloud セッションの組み込み GitHub ツールは使えた。
公式文書の「pre-installed」は実運用での利用可能性を保証しないため、cloud の通常経路から
`gh` 依存を外す。`sdd-guard.sh` は MCP ツールを直接呼べないので、PR 一覧をファイルで渡す。
高水準の `gh pr list` や `gh issue list` は、`gh` がある環境でも GraphQL や認証 proxy の
制限で失敗しうるため使わない。

使う GitHub 情報:

| 用途 | 取得元 |
| --- | --- |
| open PR の一覧 | 組み込み GitHub ツールで base を限定せず `state=open` の PR を 100 件単位で必要ページ分取得し、`pulls-open.json` に置く |
| closed PR の一覧 | 組み込み GitHub ツールで base を限定せず `state=closed` の PR を更新日時降順に 100 件単位で必要ページ分取得し、`pulls-closed.json` に置く |
| マージ済み判定 | `sdd-guard.sh` が git の first-parent 履歴から決める |
| PR が触ったファイル | `sdd-guard.sh` が該当 merge/squash commit の diff から決める |
| review threads | open な `sdd` PR に未解決レビュー指摘があるかを組み込み GitHub ツールで取得する |
| open Issue の題名 | 組み込み GitHub ツールで取得する |

`gh api` を使う場合も REST だけに限定する。ただしこれは手元での検算・保守用であり、cloud
セッションの成功条件に含めない。

**Alternatives considered**:

- `gh api` を cloud の通常経路にする: 実機で `gh` が無かったため不可
- 組み込み GitHub ツールだけを使う: スクリプトからは呼べないため、ファイル渡しにする

---

## R-003: routine のセッションで Spec Kit のコマンドを動かす

**Decision**: スキルが `SPECIFY_FEATURE_DIRECTORY=<feature_dir>` を export してから
`/speckit-*` を呼ぶ。ブランチ名は `claude/sdd-NNN-<stage>` とし、Spec Kit の
ブランチ規約（`NNN-name`）には従わない。

**Rationale**: `.specify/scripts/bash/common.sh` の `get_feature_paths` は、機能ディレクトリを
(1) `SPECIFY_FEATURE_DIRECTORY`、(2) `.specify/feature.json`（gitignore 済み）の順に解決し、
ブランチ名は使わない。(1) を与えると `.specify/feature.json` に永続化されるので、同じ
セッション内の後続コマンドは環境変数なしでも同じ機能を指す。routine のセッションは
`claude/` 接頭辞のブランチに push することが常に許可されている（`/docs/en/routines`
"Repositories and branch permissions"）ので、ブランチ名はハーネス用の規約で決められる。

**Alternatives considered**:

- `create-new-feature.sh` でブランチを切る: 既存機能に対しては不要で、`claude/` 接頭辞
  でもないため push の許可判定で不利

---

## R-004: 使用率の取得手段

**Decision**: 実装は 2 段構えにする。まず**プローブ用の routine 実行**で取得手段を確定し、
確定するまでスキルの使用量ゲートは「取得できなければ飛ばす」（FR-020 後段）として
実装する。候補は次の 2 つで、quickstart S3 で試す。

- 案 a) リポジトリの `.claude/settings.json` に `statusLine` コマンドを置き、受け取った
  JSON をそのまま `/tmp/sdd-rate-limits.json` に書く。cloud セッションでもこのコマンドが
  実行されるなら、スキルは最初の API 応答の後にそのファイルを読める
- 案 b) `curl https://api.anthropic.com/api/oauth/usage` がプロキシ経由で通るか試す

**Rationale**: `rate_limits.five_hour.used_percentage` と `rate_limits.seven_day.used_percentage`
は、Claude Code が**ステータスラインスクリプトへの stdin JSON にだけ**渡している
（`/docs/en/statusline` "Rate limit usage"）。フックの入力にはこの項目が無く、使用量に
関するフックイベントも無い（`StopFailure` の `rate_limit` は超過後に発火する）。cloud
セッションでステータスラインが動くかは文書に無い。

両案とも不可なら、ゲートは無効のまま `docs/exec-plans/tech-debt.md` に記録する
（設計文書 6 章）。本機能の受け入れ（SC-001〜SC-006）は使用量ゲートに依存しない。

**Alternatives considered**:

- `StopFailure` フックで超過を検知して後始末する: 超過してからでは中途半端な成果物が
  残る。見送りの目的に合わない

---

## R-005: routine のセッションで SessionStart フックが走るか

**Decision**: プローブで確定する。走らなければ routine の環境の setup script に
`make setup` を置く。

**Rationale**: 既存のフック（`.claude/hooks/session-start.sh`）は `CLAUDE_CODE_REMOTE=true`
のときだけ `make setup` を呼ぶ。routine は cloud セッションとして動くのでこの変数が
立つ可能性が高いが、文書は「フックはリポジトリと組織設定から読まれる」としか言って
おらず、routine での発火は明記されていない。implement 段階では `make check` が要るので、
どちらかの経路で依存を入れておく必要がある。plan／tasks の段階には不要。

---

## R-006: GitHub トリガーの fire payload に PR 情報が含まれるか

**Decision**: 依存しない。対象機能は REST で「直近のマージ済み `sdd` PR」を引いて決める
（R-002）。payload に PR 番号があれば、スキルはそれを**確認用**に使ってよい。

**Rationale**: API トリガーの `text` は `<routine-fire-payload>` に包まれて信頼されない
データとして届くことが文書化されているが、GitHub トリガーで何が届くかは明記されて
いない。届かなくても動く設計にしておけば、プローブの結果に左右されない。

---

## R-007: スクリプトの言語と依存

**Decision**: `bash`（4 以上）で書く。`sdd-state.sh` は **`jq` に依存しない**（`awk`・`grep`・
`printf` だけで JSON を組み立てる）。`sdd-guard.sh` は `jq` に依存するが、cloud の通常経路では
`gh` に依存しない。テストランナーも `bash` で、基本テストは `jq` 無しで動く。

**Rationale**: cloud セッションには bash・git・jq がある想定だが、`gh` は実機プローブで
無かった。CI（ubuntu-latest）には jq がある。一方、保守者の手元（Windows + Git Bash）には
`jq` も `gh` も無い（本セッションで確認）。
US3「手元で判定を検算できる」を満たすには、状態判定とそのテストが `jq` 無しで動く
必要がある。`sdd-guard.sh` は GitHub を見る以上、手元では `gh` 無しで `go:false` を返す
挙動を確認するだけでよい。`jq` と git がある環境では、`--github-dir` のファイル入力で
GitHub 判定も自動テストする。

JSON の組み立てで注意するのは `phase_title` のエスケープだけである（`"` と `\` を
エスケープし、制御文字は含まない前提）。

**Alternatives considered**:

- Go で書いて `go run` する: 依存は無くなるが、スキルから呼ぶ判定に Go のビルドを
  挟むのは重い。`make test-sdd` を Go のジョブに足す構成（設計文書 8 章）とも噛み合わない
- Python: cloud には入っているが、手元と CI で版を揃える手間が増える

---

## R-008: tasks.md の構文規則

**Decision**: 次の規則で数える。

| 要素 | 規則 |
| --- | --- |
| フェーズの見出し | 行頭が `## Phase N:`（`N` は 1 以上の整数）。題名は `:` の後ろ全体（`(Priority: P1)` や絵文字を含む）を trim したもの |
| タスク行 | 行頭が `- [ ] `（未完了）または `- [x] ` / `- [X] `（完了）。インデントされた行は数えない |
| 節の範囲 | フェーズの見出しから次の `## ` 見出しまで |
| フェーズ数 | `## Phase N:` の見出しの個数 |

**Rationale**: `.specify/templates/tasks-template.md` と既存の
[001 の tasks.md](../001-initial-setup/tasks.md) はこの書式で統一されている。
テンプレートを Spec Kit が変えたら追従が要るが、フィクスチャのテストで検出できる。

---

## R-009: 停止通知（Issue）の題名と重複判定

**Decision**: 題名は `sdd-next 停止: NNN <理由コード>` の固定形式。理由コードは
`no-progress` / `phase-retry-limit` / `hop-limit` の 3 つ。本文に状態 JSON と
セッションへの参照（`CLAUDE_CODE_REMOTE_SESSION_ID`）を書く。重複判定は open Issue の
題名の完全一致。

**Rationale**: FR-017 の「同じ題名の open Issue があれば作らない」を、REST 1 回で判定
できる形にする。理由コードを固定にしておけば、同じ理由での再停止が重複しない。
`CLAUDE_CODE_REMOTE_SESSION_ID` は cloud セッションが自分の ID を読める環境変数
（`/docs/en/cloud-environments` "Link output back to the session"）。

---

## 未解決事項の一覧（プローブで確定）

| 項目 | 確定方法 | 影響範囲 | 結果（2026-09-13 プローブ、session `cse_011ZErYWwbMCiPrnW6Msyka7`） |
| --- | --- | --- | --- |
| R-004 使用率の取得手段 | quickstart S3 | 使用量ゲートの有効／無効 | 案 a 不可: `/tmp/sdd-rate-limits.json` は生成されず、statusLine は cloud で走らない。案 b は未確認 |
| R-005 SessionStart フックの発火 | quickstart S3 | routine 環境の setup script の要否 | 発火した（init まで約 110 秒）。setup script は不要 |
| R-006 fire payload の中身 | quickstart S3 | なし（確認用途のみ） | Run now の添え文は `<routine-fire-payload>` として届く。GitHub トリガー発火時の中身は未確認 |
| （新規）`gh` CLI の有無 | 同上 | **`sdd-guard.sh` が常に `gh-unavailable` を返し、連鎖が始まらない** | cloud 環境に `gh` が無い（PR 作成は組み込みの GitHub ツールで行われている）。`sdd-guard.sh` に `--github-dir` を足し、スキルが組み込みツールで取った PR 一覧を渡す形に変えた |

いずれも US1〜US3 の受け入れには影響しない。
