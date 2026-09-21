# Implementation Plan: requirements.md と別担当者承認必須ルールの廃止

**Branch**: `codex/plan-remove-requirements-checklist` | **Date**: 2026-09-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/006-remove-requirements-checklist/spec.md`

## Summary

Spec Kit の仕様作成時に生成していた組み込み `checklists/requirements.md` を廃止し、
明確化と実装開始からもその読み書き・ゲート利用を除く。既存ファイルと plan 内の参照を
削除し、仕様品質は `spec.md` 作成中の一時検証として扱う。同時に、別担当者または
別セッションによる仕様承認を SDD の開始条件にするリポジトリ独自ルールと専用手順を削除する。

## Technical Context

**Language/Version**: Markdown、Agent Skills 互換の手順文書、PowerShell 7 / POSIX shell の検証コマンド

**Primary Dependencies**: リポジトリ内の Spec Kit スキル、Issue handoff スキル、GitHub Issue / Pull Request

**Storage**: Git 管理下の Markdown とテンプレート。品質確認専用の永続ファイルは追加しない

**Testing**: `rg` とファイル列挙による残存検査、手順文書の相互参照確認

**Target Platform**: このリポジトリを扱うローカル coding agent と GitHub 上の保守者

**Project Type**: リポジトリ内ワークフローおよび文書の変更

**Performance Goals**: 対象ファイル全体の残存検査を 1 回のローカル検証で完了できること

**Constraints**: `spec.md` を要求の正本に保つ。`.specify/feature.json` を handoff の情報源にしない。
任意のカスタムチェックリストと通常の PR レビューは廃止しない

**Scale/Scope**: Spec Kit の 4 スキル、Issue handoff の Specify 手順、PR / checklist テンプレート、
仕様品質文書、既存 6 feature directory の成果物と関連 plan

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` は未設定のプレースホルダーであり、強制可能な原則はない。
代わりに `AGENTS.md` と `docs/product-specs/spec-quality.md` を確認した。

- **Repository boundaries**: PASS。変更は `.agents/skills/`、`.specify/templates/`、`docs/`、
  `.github/`、`specs/` の既存所有範囲に限定する
- **Documentation consistency**: PASS。削除する手順書への索引と参照を同時に削除する
- **Generated files**: PASS。`docs/generated/` は変更しない
- **Reviewability**: PASS。組み込みチェックリスト廃止、承認ゲート廃止、既存成果物整理を
  Implementation Work で分離する
- **Post-design re-check**: PASS。追加する contract と quickstart は新しい永続状態や外部依存を
  導入しない

## Project Structure

### Documentation (this feature)

```text
specs/006-remove-requirements-checklist/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    └── spec-workflow.md
```

### Repository Files

```text
.agents/skills/
├── speckit-specify/SKILL.md
├── speckit-clarify/SKILL.md
├── speckit-implement/SKILL.md
├── speckit-checklist/SKILL.md
└── issue-handoff/references/specify.md

.specify/templates/checklist-template.md
.github/pull_request_template.md
AGENTS.md

docs/
├── how-to/README.md
├── product-specs/
│   ├── index.md
│   └── spec-quality.md
└── exec-plans/tech-debt.md

specs/{001,002,003,004,005,006}-*/
├── plan.md                              # 該当する既存参照だけを整理
└── checklists/requirements.md           # 削除対象
```

**Structure Decision**: ランタイムコードや新しい実行基盤は追加しない。現在の規則を所有する
スキル・テンプレート・文書を直接更新し、feature artifacts には判断、契約、検証手順だけを置く。

## Complexity Tracking

該当なし。新しい抽象化、永続状態、外部依存は追加しない。

## Implementation Work

### 組み込み requirements チェックリストのライフサイクルを削除する

**Scope**: `speckit-specify` のファイル生成を一時的な自己検証へ置き換え、`speckit-clarify` の
再検証処理と `speckit-implement` の組み込みファイル説明を削除する。
`speckit-checklist` と checklist template から組み込みファイルの例外規則を外す。

**Dependencies**: なし。

**Observable acceptance evidence**: 仕様作成・明確化・実装の各手順に
`checklists/requirements.md` の生成、読み取り、更新、開始ゲート利用がなく、仕様作成の品質確認は
`spec.md` を更新して完了する。

### 別担当者による仕様承認の必須ゲートを削除する

**Scope**: `AGENTS.md`、仕様品質規則、PR テンプレート、Issue handoff の Specify 手順から
別担当者または別セッションによる承認要件を削除する。専用 how-to と対応する tech-debt 項目を
削除し、曖昧さを質問へ戻す品質規則の番号を詰める。

**Dependencies**: なし。通常の PR レビューと人によるマージ判断は既存どおり利用できる。

**Observable acceptance evidence**: 現行の運用文書とテンプレートに別担当者の approve を
工程開始条件とする記述がなく、削除した how-to へのリンクが 0 件である。

### 既存成果物を整理して残存検査を固定する

**Scope**: 既存の `specs/*/checklists/requirements.md` を削除し、plan のディレクトリツリーから
同ファイルを外す。006 の spec と product spec index を最終方針に合わせ、quickstart の検索で
削除漏れと対象外のカスタムチェックリスト保護を確認する。

**Dependencies**: 前 2 項目の文言と対象範囲が確定していること。

**Observable acceptance evidence**: 対象ファイル数、現行手順内の旧ライフサイクル参照、
別担当者承認の必須文言、削除済み how-to へのリンクがすべて 0 件である。
