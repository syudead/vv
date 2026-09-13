# Tasks: SDD ループハーネス（spec 以降の段階を自動で回す）

**Input**: Design documents from `/specs/003-sdd-loop-harness/`

**Prerequisites**: [plan.md](./plan.md)（必須）、[spec.md](./spec.md)（ユーザーストーリー）、[research.md](./research.md)、[data-model.md](./data-model.md)、[contracts/](./contracts/)、[quickstart.md](./quickstart.md)

**Tests**: 本機能はテストを**含む**。spec が FR-012／SC-003 で「状態判定はフィクスチャによる
自動テストを持ち、リポジトリの自動検証の一部として実行される」ことを要件として求めており、
US3 はテストの存在そのものが成果物であるため。テスト対象は `sdd-state.sh`（フィクスチャ 6 組）と
`sdd-guard.sh`（`gh` 不在時の挙動のみ）。REST を伴う判定とスキル全体は
[quickstart.md](./quickstart.md) の S3〜S8 で実機確認する（自動化しない）。

**Organization**: タスクはユーザーストーリー単位にまとめてある。判定の核（3 本のスクリプトと
そのテスト）はすべてのストーリーが依存するので Foundational に置き、各ストーリーは
`SKILL.md` の手順・文書・配線を足していく。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 並行実行可（別ファイル・依存なし）
- **[Story]**: 対応するユーザーストーリー（US1 / US2 / US3 / US4）
- 説明には必ず対象ファイルのパスを書く

## Path Conventions

本機能は**開発ワークフローの自動化**（bash スクリプト + スキル + 文書）。製品コード
（`cmd/`、`internal/`、`web/`）には触れない。パスはすべてリポジトリ root からの相対で、
[plan.md](./plan.md) の "Source Code" の配置に従う。

- スキル一式: `.claude/skills/sdd-next/`（`SKILL.md`、`scripts/`、`tests/`）。`.specify/scripts/` には置かない
- 配線: `Makefile`（`test-sdd`）、`.github/workflows/ci.yml`（Go ジョブに 1 ステップ）
- 文書: `docs/references/sdd-routine.md`、`docs/exec-plans/active/003-sdd-loop-harness.md`、`AGENTS.md`
- スクリプトの依存: `sdd-state.sh` と `tests/run.sh` は bash・awk・grep・sed のみ（`jq` 不可）。`sdd-guard.sh` だけ `gh`・`jq` を使う（[R-007](./research.md)）
- 手元は Windows の Git Bash（`core.autocrlf=true`）でも動くこと。読み込む行は末尾の `\r` を落としてから解釈する

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 実行計画とスキルの置き場所を用意する

- [X] T001 実行計画 `docs/exec-plans/active/003-sdd-loop-harness.md` を作成する。書式は [docs/exec-plans/completed/002-core-video-library.md](../../docs/exec-plans/completed/002-core-video-library.md) に合わせ、ステータス（進行中）・最終更新日・対象・目的・一次資料の表（spec／plan／research／data-model／contracts／quickstart へのリンク）・検証の方針（quickstart S1〜S8 で完了を判定する）・進捗（フェーズごとの空欄）・決定の記録を置く。routine の作成（[contracts/routine.md](./contracts/routine.md)）は保守者が claude.ai で行うリポジトリ外の作業なので、本 tasks.md には含めず、ここに「S3 の前に保守者が行う」と明記する
- [X] T002 [P] スキルの骨組みを作る: `.claude/skills/sdd-next/SKILL.md`（frontmatter のみ。`name: "sdd-next"`、`description: "Spec Kit の次の 1 段階（plan / tasks / implement の 1 フェーズ）を実行し、sdd ラベル付き PR を開く。routine と手動の両方から同じ手順で動く"`、`argument-hint: "--dry-run（判定だけ表示して終わる）"`、`user-invocable: true`、`disable-model-invocation: false` — routine のプロンプトからモデルが Skill ツールで呼ぶため `true` にしてはならない。書式は [.claude/skills/speckit-plan/SKILL.md](../../.claude/skills/speckit-plan/SKILL.md) の frontmatter に合わせる）、`.claude/skills/sdd-next/scripts/.gitkeep`、`.claude/skills/sdd-next/tests/fixtures/.gitkeep`。本文は US1 以降で書く
- [X] T003 [P] `.gitattributes` を作成し `*.sh text eol=lf` を書く。手元（Windows、`core.autocrlf=true`）でチェックアウトしたスクリプトが CRLF になると Git Bash で `$'\r': command not found` になり US3（手元で検算）が満たせないため。フィクスチャ（`.md`／`.json`）には適用せず、手元では CRLF のまま読ませてスクリプト側の `\r` 除去を検証する

**Checkpoint**: `ls .claude/skills/sdd-next/` に `SKILL.md`・`scripts/`・`tests/` がある

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 判定の核（状態判定・ガード）とそのテスト。フィクスチャと runner を先に書き、
スクリプトはそれが通るまで実装する（テスト先行）

**⚠️ CRITICAL**: `SKILL.md` の手順はすべてこの 2 本のスクリプトの出力を真実として動くので、
ここが終わるまでユーザーストーリーの作業は始められない

### フィクスチャ（[contracts/sdd-state.md](./contracts/sdd-state.md) の表に対応）

各フィクスチャは `specs/` の書式を縮小した最小のファイル群と `expected.json`（引数なしで
`--root <fixture>` を渡したときの出力、JSON 1 行）を持つ。`--feature` の検証が要るものは
さらに `feature.txt`（`--feature` に渡す相対パス 1 行）と `expected-feature.json` を持つ。
`spec.md`／`plan.md` の中身は見出し 1 行でよい（存在だけを見る）。

- [X] T004 [P] `.claude/skills/sdd-next/tests/fixtures/01-before-plan/` を作る: `specs/010-a/spec.md` のみ。`expected.json` は `{"feature_dir":"specs/010-a","feature":"010","stage":"plan","branch":"claude/sdd-010-plan"}`（`plan.md` 無し → `plan`、`branch` の規則を検証）
- [X] T005 [P] `.claude/skills/sdd-next/tests/fixtures/02-before-tasks/` を作る: `specs/010-a/spec.md` と `specs/010-a/plan.md`。`expected.json` は `{"feature_dir":"specs/010-a","feature":"010","stage":"tasks","branch":"claude/sdd-010-tasks"}`
- [X] T006 [P] `.claude/skills/sdd-next/tests/fixtures/03-implement-mid/` を作る: `specs/010-a/{spec,plan,tasks}.md`。`tasks.md` は `## Phase 1: Setup`（`- [x]` 1 件、全完了）、`## Phase 2: Foundational (Blocking Prerequisites)`（`- [X]` 1 件 + `- [ ]` 2 件 = total 3 / remaining 2。加えて**インデントされた** `  - [ ] ` 行を 1 つ置き、数えないことを検証する）、`## Phase 3: User Story 1 - 例 (Priority: P1) 🎯 MVP`（`- [ ]` 2 件）の 3 フェーズ。フェーズ見出しの間に `**Purpose**:` などの地の文と `## Dependencies` のような Phase 以外の `## ` 見出しも置く。`expected.json` は `{"feature_dir":"specs/010-a","feature":"010","stage":"implement","phase":2,"phase_title":"Foundational (Blocking Prerequisites)","remaining":2,"total":3,"phases":3,"branch":"claude/sdd-010-implement-p2"}`（未完了を含む最初のフェーズの選択、`remaining`/`total`/`phases`、題名の trim を検証）
- [X] T007 [P] `.claude/skills/sdd-next/tests/fixtures/04-done/` を作る: `specs/010-a/{spec,plan,tasks}.md`。`tasks.md` は 2 フェーズで、チェックは `- [x]` と `- [X]` を**混在**させ、未完了は 0 件。`expected.json` は `{"feature_dir":"specs/010-a","feature":"010","stage":"done","phases":2}`（`done` には `branch` を含めず `phases` を含める）
- [X] T008 [P] `.claude/skills/sdd-next/tests/fixtures/05-no-spec/` を作る: `specs/010-a/plan.md` のみ（`spec.md` 無し）。`expected.json` は `{"stage":"none"}`（自動選択で飛ばされ、対象が無い）。`feature.txt` は `specs/010-a`、`expected-feature.json` は `{"feature_dir":"specs/010-a","feature":"010","stage":"none"}`（`--feature` で明示すれば `none` のまま判定する）
- [X] T009 [P] `.claude/skills/sdd-next/tests/fixtures/06-multi-feature/` を作る: `specs/010-a/{spec,plan,tasks}.md`（`tasks.md` は 1 フェーズ・全完了 = `done`）と `specs/011-b/spec.md`（plan 前）。`expected.json` は `{"feature_dir":"specs/011-b","feature":"011","stage":"plan","branch":"claude/sdd-011-plan"}`（`done` の機能を飛ばして番号順で次を選ぶ）。`feature.txt` は `specs/010-a`、`expected-feature.json` は `{"feature_dir":"specs/010-a","feature":"010","stage":"done","phases":1}`

### テストランナー

- [X] T010 `.claude/skills/sdd-next/tests/run.sh` を書く（bash のみ、`jq` 不要、`set -u`）。自分の場所から `SCRIPTS=$dir/../scripts` を解決し、次を順に検査して `PASS <名前>` / `FAIL <名前>`（FAIL は expected と actual を並べて表示）を出し、最後に件数を出して失敗が 1 つでもあれば終了コード 1: (1) `fixtures/*/` ごとに `sdd-state.sh --root <fixture>` を実行し、stdout（末尾 `\r` と改行を落とした 1 行）が `expected.json`（同じく正規化）と**文字列として完全一致**、終了コード 0、stderr が空であること (2) `feature.txt` があるフィクスチャは `--feature $(cat feature.txt)` でも同様に `expected-feature.json` と比較 (3) 決定性: `03-implement-mid` を 2 回実行して出力が同一 (4) 異常系: `--root <存在しないパス>` が終了コード 2、`--root fixtures/01-before-plan --feature specs/999-x` が終了コード 2、未知の引数が終了コード 2 (5) ガード: 一時ディレクトリに `gh` の代役（`#!/usr/bin/env bash` + `exit 1`、実行属性付き）を置いて `PATH="$tmp:$PATH"` で先頭に足し、`01-before-plan/expected.json` の内容を stdin に与えて `sdd-guard.sh --repo example/repo --root fixtures/01-before-plan` を実行し、stdout が `{"go":false,"reason":"gh-unavailable","state":<stdin の JSON そのまま>}`、終了コード 0 であること。この時点では scripts が無いので全件 FAIL する（それが正しい）

### スクリプト

- [X] T011 `.claude/skills/sdd-next/scripts/sdd-lib.sh` を書く（`source` して使う共通関数。bash・awk・grep・sed のみ）: `sdd_json_escape`（`\` → `\\` を先に、次に `"` → `\"`。改行・タブは扱わない — [contracts/sdd-state.md](./contracts/sdd-state.md) "JSON のエスケープ"）、`sdd_list_features <root>`（`<root>/specs/` 直下で `[0-9][0-9][0-9]-*` に一致するディレクトリを `LC_ALL=C` の名前昇順で `specs/NNN-name` の相対パスとして 1 行ずつ出す）、`sdd_phases <tasks.md>`（awk 1 本で、行末の `\r` を落としてから次の規則で数え、フェーズごとに `N<TAB>title<TAB>total<TAB>remaining` を出す。規則は [R-008](./research.md) を**そのまま**: フェーズ見出しは行頭が `## Phase N:`（`N` は 1 以上の整数）、題名は最初の `:` の後ろ全体を前後の空白で trim したもの（`(Priority: P1)` や絵文字を含む）、節の範囲はその見出しから次の `## ` 見出し（または EOF）まで、タスク行は行頭が `- [ ] `（未完了）または `- [x] ` / `- [X] `（完了）で、**インデントされた行は数えない**、フェーズ数は `## Phase N:` 見出しの個数）
- [X] T012 `.claude/skills/sdd-next/scripts/sdd-state.sh` を書く（[contracts/sdd-state.md](./contracts/sdd-state.md) と [data-model.md](./data-model.md) 1.〜4. の実装。`sdd-lib.sh` を `source`）: 引数は `[--root <repo_root>] [--feature <feature_dir>]`、既定の root はカレントディレクトリ。終了コードは 0（判定できた。`none`／`done` を含む）と 2（`--root` が存在しない、`--feature` のディレクトリが存在しない、引数の誤り。このときだけ stderr にメッセージ）のみ。stage の導出は上から最初に一致: `spec.md` が無い → `none`、`plan.md` が無い → `plan`、`tasks.md` が無い → `tasks`、未完了タスクがある → `implement`、それ以外 → `done`。`implement` の対象フェーズは `remaining > 0` の最初のフェーズ。出力は stdout に JSON 1 行 + 改行で、属性の並びは stage ごとに固定: `none` = `feature_dir, feature, stage`／`plan`・`tasks` = `feature_dir, feature, stage, branch`／`implement` = `feature_dir, feature, stage, phase, phase_title, remaining, total, phases, branch`／`done` = `feature_dir, feature, stage, phases`。`feature` は basename の先頭 3 文字、`branch` は `claude/sdd-NNN-plan`／`claude/sdd-NNN-tasks`／`claude/sdd-NNN-implement-pN`。`--feature` 無しの自動選択は、候補を名前昇順に判定して stage が `none` でも `done` でもない最初の機能を返し、無ければ `{"stage":"none"}`（終了コード 0）。成功時は stderr に何も出さない。`phase_title` は `sdd_json_escape` を通す。実装後 `bash .claude/skills/sdd-next/tests/run.sh` の (1)〜(4) が PASS すること
- [X] T013 `.claude/skills/sdd-next/scripts/sdd-guard.sh` を書く（[contracts/sdd-guard.md](./contracts/sdd-guard.md) と [data-model.md](./data-model.md) 5.〜6. の実装。`gh api` の REST と `jq` のみ、`gh pr list` 等の高水準コマンドは使わない — [R-002](./research.md)）: 引数は `[--repo <owner/name>] [--root <repo_root>]`、`--repo` 省略時は `git -C <root> remote get-url origin` から `git@github.com:o/r.git` と `https://github.com/o/r(.git)` の両形式で導出。stdin が空か `{` で始まらなければ終了コード 2。**最初に** `command -v gh` と `command -v jq` の有無と `gh api /rate_limit` の疎通を見て、いずれか失敗なら `jq` を使わずに `printf` で `{"go":false,"reason":"gh-unavailable","state":<stdin をそのまま>}` を出して終了コード 0（手元で検算できるようにするため）。以降は契約の手順の順に最初に該当したものを返す: (1) 対象機能の確定 — `GET /repos/{o}/{r}/pulls?state=closed&base=main&sort=updated&direction=desc&per_page=100` から `merged_at != null` かつ `labels[].name` に `sdd` を含む先頭 1 件を取り、`GET /repos/{o}/{r}/pulls/{n}/files` の `filename` から `^specs/[0-9]{3}-[^/]+/` に一致する最初のディレクトリを抽出し、見つかれば `sdd-state.sh --root <root> --feature <dir>` で `state` を置き換える（ディレクトリが手元に無く終了コード 2 なら stdin の state のまま） (2) `state.stage` が `done` か `none` → `{"go":false,"reason":"nothing-to-do","state":...}` (3) `GET /repos/{o}/{r}/pulls?state=open&base=main&per_page=100` で `head.ref` が `claude/sdd-NNN-` で始まるものがあれば `{"go":false,"reason":"open-pr","state":...,"open_prs":[<head.ref の配列>]}` (4) `stage = implement` で、(1) の一覧のうち `head.ref = claude/sdd-NNN-implement-pN`（`N` は `state.phase`）のマージ済みが **2 件以上** → `reason:"phase-retry-limit"` (5) (1) の一覧のうち `head.ref` が `claude/sdd-NNN-` で始まるマージ済みが **`2 + phases + 2` 件以上**（`phases` は `state.phases`、無ければ 0）→ `reason:"hop-limit"` (6) それ以外 → `{"go":true,"state":...,"hops":<件数>,"phase_retries":<件数>,"open_prs":[]}`。go:false の場合も `hops`・`phase_retries` を含めてよい。実装後 `run.sh` の (5) が PASS すること

**Checkpoint**: `bash .claude/skills/sdd-next/tests/run.sh` が全件 PASS。リポジトリ root で
`.claude/skills/sdd-next/scripts/sdd-state.sh` を実行すると、今日の `main` では
`{"feature_dir":"specs/003-library-ui","feature":"003","stage":"plan","branch":"claude/sdd-003-plan"}`
が 1 秒以内に返る（`001`・`002` は `done` なので飛ばされる）。ここから US1〜US4 に進める

---

## Phase 3: User Story 1 - マージすると次の段階が自動で始まる (Priority: P1) 🎯 MVP

**Goal**: `/sdd-next` が「判定 → 段階の実行 → `sdd` ラベル付き PR」を 1 段階だけ行い、
routine から呼べる状態にする。routine の設定はリポジトリに写しを置く

**Independent Test**: [quickstart.md](./quickstart.md) S4（Run now で plan の PR が開く）と
S5（マージすると tasks → implement とフェーズごとに連鎖し、最後は PR を作らずに終わる）。
手元では `/sdd-next` を web セッションで手打ちしても同じ動きになること

### Implementation for User Story 1

- [ ] T014 [US1] `.claude/skills/sdd-next/SKILL.md` に本文の骨格を書く（[contracts/sdd-next-skill.md](./contracts/sdd-next-skill.md) の順）: 冒頭に「定数」の表 `USAGE_LIMIT_7D = 80`、`USAGE_LIMIT_5H = 70`、`PHASE_RETRY_LIMIT = 2`（後者は `sdd-guard.sh` と同じ値で文書用。判定はスクリプト側）。続けて手順 0「前提確認」（`git branch --show-current` が `main`、`git status --porcelain` が空、`gh api /rate_limit` が通る。1 つでも満たさなければ理由を書いて終了し、変更を残さない）と手順 2「準備」（`git switch -c <state.branch>`、`export SPECIFY_FEATURE_DIRECTORY=<state.feature_dir>` — これで `.specify/scripts/bash/common.sh` が `.specify/feature.json` を書き、以後の `/speckit-*` が同じ機能を指す、[R-003](./research.md)）。手順 0.5／1／1.5／4 の見出しは置き「US2／US3／US4 で埋める」と書いておく
- [ ] T015 [US1] `.claude/skills/sdd-next/SKILL.md` に手順 3「段階の実行」の表を書く: `plan` → `/speckit-plan` を呼び、`plan.md` と `research.md` が生成されたことを確認／`tasks` → `/speckit-tasks` の後に `/speckit-analyze` を呼び、CRITICAL のうち tasks.md 側の直しで解消できるものだけ直す（**spec.md と plan.md には手を入れない**。解消できない CRITICAL は PR 本文の「残課題」に残す）／`implement` → `/speckit-implement "Phase <N>（<phase_title>）のタスクだけを対象にする。他のフェーズには手を付けない"` を引数付きで呼び（フィルタの仕組みは新設しない、FR-008）、完了したタスクを tasks.md で `[X]` にし、`make check` を通す。通らなければ直し、直せなければ PR を **draft** で開き本文に失敗内容を書く（FR-006）。`done`／`none` は何もしない。どの段階でも「次の 1 段階」だけを実行し、2 段階以上続けない（FR-003）
- [ ] T016 [US1] `.claude/skills/sdd-next/SKILL.md` に手順 5「コミット・push・PR・ラベル」と手順 6「報告」を書く: コミットは日本語の要約 1 行 + 空行 + 本文、末尾に `Co-Authored-By: <セッションのモデル名> <noreply@anthropic.com>`、プレフィックスは plan／tasks が `docs:`、implement が `feat:`（テストのみなら `test:`）。PR は base `main`、title は plan `docs: NNN の実装計画と設計成果物を追加する`／tasks `docs: NNN の実装タスクを分解する`／implement `feat: NNN Phase N（<phase_title の先頭 30 文字>）を実装する`、作成は cloud セッション組み込みの GitHub ツールを第一候補、`gh pr create` を代替、`gh api -X POST /repos/{o}/{r}/pulls` を最終手段。本文は契約の雛形（`## 段階`／`## 判定`（before・after・guard の JSON）／`## 実行した検査`／`## 残課題`／`## 実行元`（`CLAUDE_CODE_REMOTE_SESSION_ID`）／末尾に `🤖 Generated with [Claude Code](https://claude.com/claude-code)`）をそのまま載せる。ラベル `sdd` は `gh api -X POST /repos/{o}/{r}/issues/{n}/labels -f 'labels[]=sdd'` で付け、`GET /repos/{o}/{r}/issues/{n}/labels` で付いたことを**検証**する。失敗したら 1 回再試行し、それでも付かなければ PR 本文の先頭に「ラベル未付与: 手で `sdd` を付けてください」と追記して終了（ラベルが無いと連鎖が切れるため）。手順 6 は「段階・PR の URL・次に起きること（`マージすると <次の段階> が始まる` または `これで完了`）」を 3 行で出す
- [ ] T017 [P] [US1] `docs/references/sdd-routine.md` を作成する。[contracts/routine.md](./contracts/routine.md) の設定値・プロンプト全文・トリガー 2 つ（GitHub `pull_request` / `closed`、フィルタ Base branch equals `main`・Labels is one of `sdd`・Is merged equals `true`／Schedule 毎日 03:00 JST）・前提・作成手順・停止方法をそのまま写し、[docs/references/README.md](../../docs/references/README.md) の約束どおり冒頭に「出典: contracts/routine.md、写した日: 2026-09-13」を書く。末尾に保守者が作成後に埋める欄「routine ID」「URL」「作成日」「プローブ（S3）の結果」を空欄で置く。以後はこのファイルを真実とする（FR-021）
- [ ] T018 [P] [US1] `AGENTS.md` の "Working agreements" に 1 行だけ追加する: 「`sdd` ラベル付きの PR を `main` にマージすると `/sdd-next` が Spec Kit の次の段階を自動で回す。手順は `.claude/skills/sdd-next/SKILL.md`、設計は `docs/design-docs/sdd-loop-harness.md`」。それ以上は書かない（AGENTS.md は地図であり詳細は docs/ に置く）
- [ ] T019 [US1] GitHub リポジトリ `syudead/vv` にラベル `sdd` を作る（説明: 「マージすると /sdd-next が次の段階を回す」）。`gh api -X POST /repos/syudead/vv/labels -f name=sdd -f description='...' -f color=0e8a16` で作り、`GET /repos/syudead/vv/labels/sdd` で確認する。`gh` が使えない環境（手元の Windows）なら、`docs/references/sdd-routine.md` の「前提」に手順を残して保守者に依頼し、作成を確認してからチェックする。**本機能を入れる PR 自体には `sdd` ラベルを付けない**（設計文書 9 章）

**Checkpoint**: `SKILL.md` に手順 0・2・3・5・6 がそろい、web セッションで `/sdd-next` を
手打ちすると（guard が `go:true` の機能に対して）1 段階だけ進んで `sdd` ラベル付き PR が
開く。quickstart S4／S5 を実機で確認できる

---

## Phase 4: User Story 2 - 暴走しない・重複しない (Priority: P1)

**Goal**: guard が `go:false` を返したとき、open PR なら黙って終わり、上限や無進捗なら
Issue を 1 件だけ立てて終わる。保守者は routine を止めれば全体が止まる

**Independent Test**: [quickstart.md](./quickstart.md) S6（open PR があるまま Run now を
3 回しても PR・ブランチ・Issue が増えない）、S7（マージせず閉じても連鎖しない）、
S8（同じフェーズを 2 回マージすると Issue `sdd-next 停止: NNN phase-retry-limit` が 1 件だけ立つ）

### Implementation for User Story 2

- [ ] T020 [US2] `.claude/skills/sdd-next/SKILL.md` の手順 1「判定」を書く: `before=$(.claude/skills/sdd-next/scripts/sdd-state.sh)`、`guard=$(printf '%s\n' "$before" | .claude/skills/sdd-next/scripts/sdd-guard.sh)` を実行し、以後は **`guard.state` を真実**として使う（対象機能が (1) で変わり得るため。`before` も `guard.state` で置き換える）。`guard.go` が `false` のときの分岐表: `open-pr` → 何もせず終了（正常な待ち。ブランチも PR も Issue も作らない、FR-013）／`nothing-to-do` → 何もせず終了（FR-007）／`gh-unavailable` → 理由を出して終了、リポジトリに変更を残さない（Edge Cases）／`phase-retry-limit`・`hop-limit` → 停止通知の Issue（T022）を立てて終了（FR-015〜FR-017）
- [ ] T021 [US2] `.claude/skills/sdd-next/SKILL.md` の手順 4「前進確認」を書く: `after=$(sdd-state.sh --feature <state.feature_dir>)` を実行し、`before`（= `guard.state`）と**文字列として**比較する。同一、または `git status --porcelain` が空なら、`no-progress` の Issue（T022）を立て、`git switch main` → `git branch -D <state.branch>` でブランチを捨て、push も PR もせずに終了する（FR-014）。異なり差分もあれば手順 5 へ。`implement` では `remaining` が減っていれば前進とみなす（[data-model.md](./data-model.md) 4.「前進の定義」）
- [ ] T022 [US2] `.claude/skills/sdd-next/SKILL.md` に「停止通知（Issue）」の節を書く（[data-model.md](./data-model.md) 7.、[R-009](./research.md)）: title は `sdd-next 停止: NNN <reason>` の固定形式（reason は `no-progress`／`phase-retry-limit`／`hop-limit` の 3 つだけ。`open-pr`・`gh-unavailable`・`nothing-to-do` では作らない）。body は理由の説明、`state` と `guard` の JSON、`CLAUDE_CODE_REMOTE_SESSION_ID`。ラベルは付けない。作る前に `GET /repos/{o}/{r}/issues?state=open&per_page=100` を引き、`pull_request` キーの無いもののうち **title が完全一致**する open Issue があれば作らない（FR-017）。作成は組み込みの GitHub ツール → `gh issue create` → `gh api -X POST /repos/{o}/{r}/issues` の順で試す
- [ ] T023 [US2] `.claude/skills/sdd-next/SKILL.md` に「しないこと」と「止め方」の節を書く: 2 段階以上を続けて進めない（FR-003）／spec.md を書き換えない（tasks 段階の analyze でも）／`main` に直接 push しない／routine 側にしきい値や判定を持たせない／同じイベントで 2 セッションが同時に起動して両方が PR を開いた場合は保守者が片方を閉じる（閉じた PR はマージされていないので連鎖しない、Edge Cases）。止め方は「routine の Repeats トグルを off（一時停止、GitHub トリガーも止まる）」「routine を削除（恒久停止、リポジトリ側は変えない）」で `docs/references/sdd-routine.md` の「停止」へリンクする（FR-018）

**Checkpoint**: `SKILL.md` の手順 0〜6 がすべて埋まり、guard の各 `reason` に対する
振る舞いが表で決まっている。quickstart S6〜S8 を実機で確認できる

---

## Phase 5: User Story 3 - 手元で判定を検算できる (Priority: P2)

**Goal**: 判定のテストが `make test-sdd` と CI で走り、保守者が手元（Git Bash、`jq`・`gh`
無し）で状態判定と `--dry-run` を実行して結果を検算できる

**Independent Test**: [quickstart.md](./quickstart.md) S1（`make test-sdd` が全件 PASS、CI の
Go ジョブでも同じ）と S2（`sdd-state.sh` が 1 秒以内に決定的な JSON を返し、`gh` 無しの
`sdd-guard.sh` が `gh-unavailable` を返す）

### Implementation for User Story 3

- [ ] T024 [US3] `.claude/skills/sdd-next/SKILL.md` に手順 1.5「`--dry-run`」を書く: 引数に `--dry-run` があれば手順 0〜1 だけ行い、`guard` の JSON を整形して表示し、「実行するなら: ブランチ `<state.branch>` を切って `<stage>`（implement なら Phase N `<phase_title>`）を実行し、PR `<title>` を開く」を出して終了する。ブランチも変更も作らない。手順 0 の前提のうち `main` 上・clean は `--dry-run` でも要求するが、`gh api /rate_limit` が通らない場合は `gh-unavailable` の guard をそのまま表示して終わる（手元の検算用）
- [ ] T025 [P] [US3] `Makefile` に `test-sdd` 目標を追加する: `test-sdd: ## SDD ハーネスの判定テスト（bash のみ）` として `bash .claude/skills/sdd-next/tests/run.sh` を呼ぶ。`.PHONY` に加え、`test: test-go test-web test-sdd` に含める（`make check` 経由で走る）。Web ジョブでは呼ばない
- [ ] T026 [P] [US3] `.github/workflows/ci.yml` の `go` ジョブに、`テスト` ステップの直後に `- name: ハーネスの判定テスト` / `run: make test-sdd` を追加する（設計文書 8 章。手元の `make test` と同じ目標を呼び、判定が一致する状態を保つ）
- [ ] T027 [P] [US3] `.claude/skills/sdd-next/SKILL.md` に「手元での検算」の節を書く: quickstart S2 の 3 コマンド（`sdd-state.sh`、`sdd-state.sh --feature specs/001-initial-setup`、`sdd-state.sh | sdd-guard.sh`）と `bash .claude/skills/sdd-next/tests/run.sh`、`/sdd-next --dry-run` を列挙し、それぞれの期待（`gh` 無しでは guard が `gh-unavailable` を返す）を書く。判定は成果物だけから決まり隠れた状態を持たないこと（FR-009）、保守者が手で plan.md 等を書き進めた場合は次のセッションがその続きから進むこと（US3 シナリオ 3）を明記する
- [ ] T028 [US3] 手元（Windows の Git Bash）で検算する: `make test-sdd` が全件 PASS、`time .claude/skills/sdd-next/scripts/sdd-state.sh` が 1 秒以内で 3 回同じ出力、`.claude/skills/sdd-next/scripts/sdd-state.sh | .claude/skills/sdd-next/scripts/sdd-guard.sh` が `gh-unavailable` を返すことを確認する。フィクスチャが CRLF でチェックアウトされていても通ること（通らなければ `sdd-lib.sh` の `\r` 除去を直す）。結果（所要時間・環境）を `docs/exec-plans/active/003-sdd-loop-harness.md` の進捗に記録する

**Checkpoint**: `make test` と CI の Go ジョブで判定テストが走り、手元の Git Bash でも
同じ結果になる。quickstart S1／S2 が期待どおり

---

## Phase 6: User Story 4 - 使用量が乏しいときは見送り、あとで再開する (Priority: P3)

**Goal**: 使用率が取得できてしきい値以上なら段階を実行せずに見送り、取得できなければ
ゲートを飛ばす。取得手段はプローブ（quickstart S3）で確定するので、ここでは「案 a」の
配線と「取得不能なら飛ばす」経路までを入れる

**Independent Test**: [quickstart.md](./quickstart.md) S3 の (2)（`/tmp/sdd-rate-limits.json`
の有無）と、`SKILL.md` の定数を一時的に 0 にして `/sdd-next` を起動すると「見送り」で終わる
（PR も Issue も作らない）こと。日次実行が open PR／完了済みの状態で何もせず終わること

### Implementation for User Story 4

- [ ] T029 [US4] `.claude/skills/sdd-next/SKILL.md` に手順 0.5「使用量ゲート」と「取得手段」の節を書く: 取得手段は「`/tmp/sdd-rate-limits.json` があれば `rate_limits.seven_day.used_percentage` と `rate_limits.five_hour.used_percentage` を読む（`jq -r` か `grep -o`）。無い・読めない・数値でない場合は**取得不能**として見送りの判定を飛ばし、通常どおり手順 1 へ進む（取得できないことを理由に止まらない、FR-020 後段）」。取得できたら `seven_day >= USAGE_LIMIT_7D` または `five_hour >= USAGE_LIMIT_5H` なら「今回は見送り: 7 日窓 NN% / 5 時間窓 NN%」と書いて終了（PR も Issue も作らない。再開は日次トリガーか次のマージに任せる、FR-019）。節の末尾に「プローブ（quickstart S3）の結果で案 a／案 b のどちらかに確定し、この節を書き換える」と残す
- [ ] T030 [P] [US4] 案 a の配線を入れる（[R-004](./research.md)）: `.claude/hooks/rate-limits-statusline.sh` を作り、stdin の JSON を `${TMPDIR:-/tmp}/sdd-rate-limits.json` に**そのまま**書いて何も表示せず終了する（`CLAUDE_CODE_REMOTE` が `true` でなければ何もせず終了し、保守者の手元では副作用を持たない）。`.claude/settings.json` に `"statusLine": {"type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/rate-limits-statusline.sh"}` を追加する。既存の `hooks.SessionStart` は変えない。cloud セッションでステータスラインが実行されない場合はプローブ後に外す
- [ ] T031 [P] [US4] `docs/exec-plans/tech-debt.md` に `TD-00N: 使用量ゲートの取得手段が未確定` を追記する: 影響範囲（`.claude/skills/sdd-next/SKILL.md` 手順 0.5）、内容（使用率はステータスライン用 JSON にしか渡されず、cloud セッションで取れるかは文書に無い。取得できない間はゲートを飛ばして通常どおり進むので、使用量切れで中途半端な成果物が残り得る）、当面の対処（案 a の配線を入れてプローブで確定）、見直しの契機（quickstart S3 の結果。両案とも不可なら本項を恒久の負債として残す）

**Checkpoint**: `SKILL.md` の手順 0〜6 がすべて埋まり、使用量ゲートは「取得不能なら飛ばす」
状態で動く。プローブ後の追従 PR で取得手段を確定する

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 文書を実装に合わせ、自動検証を通し、導入に必要な状態を確認する

- [ ] T032 [P] `docs/design-docs/sdd-loop-harness.md` を実装に追従させる: 8 章「テスト」に、フィクスチャの構成（`expected.json`／`feature.txt`／`expected-feature.json`）と `sdd-guard.sh` のテスト方法（`gh` の代役で `gh-unavailable` を確認）、runner が `jq` 不要であること（4 章の記述と設計文書 8 章の「依存は bash と jq」を `bash のみ` に訂正）を書く。3 章「部品」に `sdd-lib.sh`・`.gitattributes`・`rate-limits-statusline.sh` を足す。それ以外の章は変えない
- [ ] T033 [P] `specs/003-sdd-loop-harness/quickstart.md` の S2／S4／S5 の期待値を、今日の `main` の状態に合わせて更新する: `002` は `done` になっているので自動選択の対象は `specs/003-library-ui`（`stage:"plan"`、`branch:"claude/sdd-003-plan"`）。あわせて S4 の直前に「注意: `003-library-ui` と `003-sdd-loop-harness` は同じ番号 `003` を持つため、`feature` と `branch` の規則（`claude/sdd-NNN-*`）が衝突する。本番 1 回目の前に、どちらかを別番号に改名するか、判定を機能ディレクトリ名で行うよう設計を見直すこと」を書く（本 tasks.md の Notes を参照）
- [ ] T034 スクリプトの実行属性と構文を確認する: `git update-index --chmod=+x .claude/skills/sdd-next/scripts/*.sh .claude/skills/sdd-next/tests/run.sh .claude/hooks/rate-limits-statusline.sh`（Windows では属性が付かないため index 側で立てる）、`bash -n` を各スクリプトに実行、`git ls-files --eol` で `.sh` が `eol=lf` になっていることを確認する。その後 `make check` を実行し、`test-sdd` を含めて全成功すること
- [ ] T035 `docs/exec-plans/active/003-sdd-loop-harness.md` の進捗を更新する: Phase 1〜7 の完了、手元の検算結果（T028）、保守者に残る作業（`sdd` ラベルの確認、routine の作成、quickstart S3 のプローブ、S4〜S8 の実機確認）を列挙する。S1〜S8 がすべて期待どおりになった時点で `docs/exec-plans/completed/` へ移す（本 PR では移さない）

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 依存なし。すぐ始められる
- **Foundational (Phase 2)**: Setup の後。フィクスチャ（T004〜T009）→ runner（T010）→ lib（T011）→ state（T012）→ guard（T013）の順。**すべてのユーザーストーリーを塞ぐ**
- **US1〜US4 (Phase 3〜6)**: Foundational の後。`SKILL.md` の同じファイルに節を足していくので、ストーリー間で並行させるときは節の担当を分ける。順序は P1（US1 → US2）→ P2（US3）→ P3（US4）を推奨
- **Polish (Phase 7)**: 実装するストーリーがすべて終わった後

### User Story Dependencies

- **US1 (P1)**: Foundational だけに依存。手順 1 は「`guard.go = true` なら進む」だけを書けば動く。`go:false` の扱い（US2）が無くても、guard が `go:true` を返す状況では単独で受け入れ可能
- **US2 (P1)**: Foundational と、US1 が置いた `SKILL.md` の骨格（T014）に依存。spec が「US1 の受け入れの前提」としているとおり、本番運用前に US1 と合わせて入れる
- **US3 (P2)**: Foundational に依存。`--dry-run`（T024）だけ US1 の手順 0〜1 に依存する。Makefile／CI／検算の節は独立
- **US4 (P3)**: US1 の骨格（T014）に依存。取得手段はプローブ後に確定するため、本フェーズは「配線と飛ばす経路」まで

### Within Each User Story

- フィクスチャ → runner → 実装（テスト先行。runner は最初は全件 FAIL する）
- `SKILL.md` は手順番号の順に埋める（0 → 0.5 → 1 → 1.5 → 2 → 3 → 4 → 5 → 6）
- 文書（sdd-routine.md、AGENTS.md、tech-debt.md）は同じ変更単位で更新する

### Parallel Opportunities

- Phase 1: T002 と T003 は並行可
- Phase 2: T004〜T009（フィクスチャ 6 組）は並行可。T010 以降は直列
- Phase 3: T017・T018 は T014〜T016 と並行可
- Phase 5: T025・T026・T027 は並行可
- Phase 6: T030・T031 は T029 と並行可
- Phase 7: T032・T033 は並行可

---

## Parallel Example: Foundational

```bash
# フィクスチャ 6 組を同時に作る:
Task: "01-before-plan を作る（T004）"
Task: "02-before-tasks を作る（T005）"
Task: "03-implement-mid を作る（T006）"
Task: "04-done を作る（T007）"
Task: "05-no-spec を作る（T008）"
Task: "06-multi-feature を作る（T009）"

# その後、直列に:
Task: "run.sh を書く（T010）"  → 全件 FAIL を確認
Task: "sdd-lib.sh を書く（T011）"
Task: "sdd-state.sh を書く（T012）" → (1)〜(4) PASS
Task: "sdd-guard.sh を書く（T013）" → (5) PASS
```

## Parallel Example: User Story 1

```bash
# SKILL.md の本文（直列）と並行して文書を書く:
Task: "docs/references/sdd-routine.md を作る（T017）"
Task: "AGENTS.md に 1 行足す（T018）"
```

---

## Implementation Strategy

### MVP First (Foundational + US1 + US2)

1. Phase 1: Setup
2. Phase 2: Foundational — `make test-sdd` 相当（`bash tests/run.sh`）が全件 PASS するまで
3. Phase 3: US1 — `SKILL.md` の主経路
4. Phase 4: US2 — `go:false` の各経路と Issue
5. **STOP and VALIDATE**: web セッションで `/sdd-next --dry-run`（US3 の T024 を先に入れると楽）

US1 と US2 は同じ P1 で、spec が US2 を「US1 の受け入れの前提」としているため、本番
（quickstart S4 以降）は両方が入ってから行う。

### Incremental Delivery

1. Setup + Foundational → 判定の核が手元で検算できる
2. US1 + US2 → `/sdd-next` が 1 段階進めて止まれる（MVP）
3. US3 → `make test-sdd` と CI、`--dry-run`、手元の検算手順
4. US4 → 使用量ゲート（取得不能なら飛ばす）
5. Polish → 文書の追従、`make check`、実行計画の更新
6. 設計文書 9 章のとおり、**1〜5 を 1 つの PR で入れてマージする（この PR に `sdd` ラベルは付けない）**。その後、保守者が routine を作り、S3 のプローブ → S4 の本番 1 回目へ進む

---

## Notes

- [P] tasks = 別ファイル・依存なし
- [Story] label はトレーサビリティのため。Setup／Foundational／Polish には付けない
- 各タスクの完了後、または論理的なまとまりごとにコミットする
- **`/speckit-analyze` で確認してほしい点**:
  1. `specs/003-library-ui` と `specs/003-sdd-loop-harness` が同じ番号 `003` を持つ。[data-model.md](./data-model.md) は `feature` を「basename の先頭 3 文字」と定義し、ブランチ名（`claude/sdd-NNN-*`）とホップ数の集計もこの番号でしか区別しないため、この 2 機能はガードの判定と PR のブランチが衝突する。本 tasks.md は data-model の定義どおりに実装し（T012・T013）、衝突の注意を quickstart に書く（T033）に留めた。どちらかの改名か、判定を機能ディレクトリ名で行う設計変更が要る
  2. [quickstart.md](./quickstart.md) S2／S4／S5 の期待値（`002` が `plan` 前）は spec 作成時点の `main` に基づく。今日の `main` では `002` は `done` で、自動選択の対象は `003-library-ui` になる（T033 で追従）
  3. routine の作成は claude.ai 側の手作業でリポジトリ内のタスクにできない。本 tasks.md では checkbox にせず、実行計画（T001）と `docs/references/sdd-routine.md`（T017）に手順を置いた
- 避けること: 曖昧なタスク、同一ファイルの同時編集、ストーリーの独立性を壊す相互依存
