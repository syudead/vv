# Implementation Plan: SDD ループハーネス（spec 以降の段階を自動で回す）

**Branch**: `claude/sdd-loop-harness`（機能ディレクトリ: `003-sdd-loop-harness`） | **Date**: 2026-09-13 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-sdd-loop-harness/spec.md`

## Summary

Spec Kit の plan → tasks → implement を、人の PR マージを承認ゲートにして Claude Code on
the web のセッションで 1 段階ずつ自動実行する。保守者の操作は「spec PR に `sdd` ラベルを
付けてマージする」と「各段階の PR をレビューしてマージする」だけになる（SC-001）。

技術的な進め方は、判定ロジックをすべてリポジトリ内の bash スクリプト 2 本と
スキル 1 つに置き、claude.ai 側の routine は「`/sdd-next` を呼ぶ」だけにすることである。
状態は `specs/NNN-*/` の成果物から導出し（`sdd-state.sh`）、暴走防止と冪等性は cloud の
組み込み GitHub ツールが取得した PR 一覧と git 履歴で判定する（`sdd-guard.sh`）。段階の中身は既存の `/speckit-*` に委ね、ハーネスは
「どれを呼ぶか」「PR をどう開くか」「いつ止まるか」だけを持つ。

方式の選定と代替案は[設計文書](../../docs/design-docs/sdd-loop-harness.md)で確定済み。
残っていた未確定事項は [research.md](./research.md) で閉じた。実機でしか確かめられない
3 点（使用率の取得・SessionStart フック・fire payload）はプローブ実行で確定し、いずれも
US1〜US3 の受け入れには影響しない。

## Technical Context

**Language/Version**: bash 4 以上（cloud セッション、CI の ubuntu-latest、手元の Git Bash で共通）。スキルは Markdown（`SKILL.md`）

**Primary Dependencies**: cloud セッション組み込みの GitHub ツール／`jq`（`sdd-guard.sh` のみ）／`awk`・`grep`・`sed`（POSIX 範囲）／既存の Spec Kit スクリプト `.specify/scripts/bash/common.sh`／Claude Code の routine（GitHub トリガー + 日次スケジュール）。`gh` はローカル検算の任意フォールバックであり、cloud の成功条件に含めない

**Storage**: なし。状態はリポジトリの成果物と GitHub の PR／Issue から導出する（FR-009）

**Testing**: bash のテストランナー（`.claude/skills/sdd-next/tests/run.sh`）+ フィクスチャ。`make test-sdd` として `make test` に含め、CI の Go ジョブから呼ぶ。`sdd-guard.sh` は `gh` 無し環境の挙動のみ自動化。エンドツーエンドは quickstart のプローブと本番 1 回目で確認

**Target Platform**: Claude Code on the web の cloud セッション（Linux、Anthropic 管理の Default 環境）。状態判定とそのテストは Windows の Git Bash でも動く（US3）

**Project Type**: 開発ワークフローの自動化（スクリプト + スキル + 外部サービスの設定）。製品コードには触れない

**Performance Goals**: `sdd-state.sh` は 1 秒以内（SC-002）。`sdd-guard.sh` は渡された PR 一覧と git 履歴だけで数秒以内

**Constraints**: 判定に隠れた状態を持たない／`jq` 無しで状態判定が動く／GraphQL を使わない（プロキシの制限、R-002）／1 セッション = 1 段階／routine 側に判定ロジックを置かない

**Scale/Scope**: 機能は同時に 1 つ、フェーズは 10 個程度まで。追加するのはスクリプト 2 本・テストランナー 1 本・フィクスチャ 6 組・SKILL.md・文書 3 本で、合計 600 行程度を目安とする

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` は雛形のまま批准されていない（001 と同じ状況）。
ゲートは [`AGENTS.md`](../../AGENTS.md)、[core-beliefs.md](../../docs/design-docs/core-beliefs.md)、
[`ARCHITECTURE.md`](../../ARCHITECTURE.md) から導出した。

| ゲート | 根拠 | 初期判定 | Phase 1 後の再判定 |
| --- | --- | --- | --- |
| G1: 重要な制約は可能な限りテスト可能にする | core-beliefs | PASS — 状態判定はフィクスチャで自動テスト（FR-012） | PASS — 契約 [sdd-state.md](./contracts/sdd-state.md) の各規則にフィクスチャを対応させた |
| G2: 文書は変更と同じ変更単位で更新する | AGENTS.md / core-beliefs | PASS — 設計文書・routine の写し・AGENTS.md の追記を成果物に含む（FR-021／FR-022） | PASS |
| G3: エージェント向け案内は地図であり、詳細は docs/ に置く | core-beliefs | PASS — AGENTS.md には 1 行だけ足し、詳細は設計文書とスキルに置く | PASS |
| G4: 生成物は手編集せず元から生成する | AGENTS.md | PASS — 生成物は増やさない | PASS |
| G5: 依存方向（cmd → internal → domain）を壊さない | ARCHITECTURE.md | PASS — 製品コードに触れない | PASS |
| G6: push した feature ブランチには必ず PR を開く | AGENTS.md | PASS — ハーネスは push 直後に PR を開く（FR-004）。PR を開けない場合は push もしない | PASS |
| G7: 実質的な作業は `docs/exec-plans/active/` に実行計画を置く | AGENTS.md | PASS — 本機能の実行計画を置き、完了時に completed へ移す | PASS |

違反なし。Complexity Tracking に記載する逸脱はない。

## Project Structure

### Documentation (this feature)

```text
specs/003-sdd-loop-harness/
├── spec.md              # 要件と受け入れ条件（設計文書から起こした）
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 未確定事項の解消
├── data-model.md        # Phase 1 output — 状態・段階・フェーズ・ホップの定義
├── quickstart.md        # Phase 1 output — 受け入れの検証手順（プローブを含む）
├── contracts/           # Phase 1 output
│   ├── sdd-state.md          # 状態判定コマンドの契約（引数・JSON・終了コード）
│   ├── sdd-guard.md          # ガードコマンドの契約
│   ├── sdd-next-skill.md     # スキルの入出力（ブランチ名・PR・Issue の形式）
│   └── routine.md            # routine の設定（docs/references/sdd-routine.md の原型）
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
.claude/
├── skills/
│   └── sdd-next/
│       ├── SKILL.md                 # /sdd-next の手順書（しきい値の定数を先頭に置く）
│       ├── scripts/
│       │   ├── sdd-state.sh         # 状態判定（ファイルのみ、jq 不要）
│       │   ├── sdd-guard.sh         # ガード（PR 一覧 JSON + git + jq。gh は任意フォールバック）
│       │   └── sdd-lib.sh           # 共通関数（JSON エスケープ、tasks.md の解析）
│       └── tests/
│           ├── run.sh               # テストランナー（bash のみ）
│           └── fixtures/
│               ├── 01-before-plan/       specs/010-a/spec.md
│               ├── 02-before-tasks/      specs/010-a/{spec,plan}.md
│               ├── 03-implement-mid/     specs/010-a/{spec,plan,tasks}.md（Phase 2 に未完了）
│               ├── 04-done/              specs/010-a/…（全部 [X]、[x] 混在）
│               ├── 05-no-spec/           specs/010-a/plan.md のみ
│               ├── 06-multi-feature/     specs/010-a（done）+ specs/011-b（plan 前）
│               └── */expected.json       各フィクスチャの期待出力
├── settings.json                    # 既存。プローブの結果次第で statusLine を追加（R-004）
└── hooks/session-start.sh           # 既存。変更なし

Makefile                             # test-sdd 目標を追加し、test に含める
.github/workflows/ci.yml             # Go ジョブに「ハーネスの判定テスト」ステップを追加

docs/
├── design-docs/sdd-loop-harness.md  # 既存（設計）。実装で変わった点があれば追従
├── references/sdd-routine.md        # routine 設定の写し（contracts/routine.md から）
├── exec-plans/active/003-sdd-loop-harness.md   # 実行計画（完了時に completed へ）
└── product-specs/index.md           # 003 の spec へのリンクを追加

AGENTS.md                            # Working agreements に 1 行追加
```

**Structure Decision**: ハーネスに関わるものは `.claude/skills/sdd-next/` の 1 箇所に
まとめる（FR-022）。スクリプトをスキルの配下に置くのは、routine のセッションで
「読む場所が 1 つ」になるようにするためで、`.specify/scripts/` には置かない（Spec Kit の
更新で上書きされる領域を避ける）。テストのフィクスチャは実際の `specs/` の書式を
そのまま縮小したもので、`specs/` 本体は使わない（本体が進むと期待値が変わるため）。

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

該当なし。

## Phase 1 の設計で決めたこと（要約）

詳細は各成果物にある。ここでは tasks に落とすときに迷わないよう判断だけを列挙する。

1. **`sdd-state.sh` は純粋関数**: 引数はリポジトリ root（省略時はカレント）と任意の
   `--feature`。標準出力に JSON 1 行、標準エラーに何も出さない。終了コードは 0 のみ
   （`none` や `done` も正常）。入力の異常（ディレクトリが無い）だけ非 0
2. **`sdd-guard.sh` は state を受けて state を返す**: 入力 JSON を stdin で受け、対象機能を
   確定し直した state を `state` に含めて返す。スキルは guard の返す `state` を以後の
   真実として使う（対象機能が変わり得るため）
3. **前進チェックはスキルが行う**: `sdd-state.sh --feature <dir>` を作業前後で実行し、
   文字列として比較する。`git status --porcelain` が空なら差分なし
4. **implement の対象指定は文言で渡す**: `/speckit-implement` に「Phase N（題名）の
   タスクだけを対象にする」と引数で伝える。`speckit-implement` は user input を
   考慮する仕様なので、フィルタの仕組みを新設しない（FR-008）
5. **PR 作成・ラベル付与・マージは cloud の組み込み GitHub ツールで行う**。`gh` は
   cloud で使えるとは限らないため通常経路に含めない。ラベルが無いと連鎖が切れるため、
   付与を検証する
6. **使用量ゲートは最初から SKILL.md に書く**が、取得関数が無ければ飛ばす。取得手段は
   プローブ後の追従 PR で足す（R-004）
7. **routine の作成はこのリポジトリの外**（claude.ai）で行う。手順と設定値は
   [contracts/routine.md](./contracts/routine.md) に書き、実装時に
   `docs/references/sdd-routine.md` へ写す
