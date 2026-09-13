# 実行計画: SDD ループハーネス（spec 以降の段階を自動で回す）

- ステータス: 進行中（実装まで。受け入れ検証は保守者の実機作業が残る）
- 最終更新: 2026-09-13
- 対象: [設計文書](../../design-docs/sdd-loop-harness.md) の全体。Spec Kit の plan → tasks → implement を、人の PR マージを承認ゲートにして Claude Code on the web のセッションで 1 段階ずつ自動実行する仕組み

## 目的

保守者の操作を「spec の PR に `sdd` ラベルを付けてマージする」と「各段階の PR を
レビューしてマージする」だけにする。判定ロジックはすべてリポジトリ内
（`.claude/skills/sdd-next/`）に置き、claude.ai 側の routine は `/sdd-next` を呼ぶだけに
する。製品コード（`cmd/`・`internal/`・`web/`）には触れない。

## 一次資料

詳細はすべて Spec Kit の成果物にある。本書は進捗と決定の記録に徹する。

| 文書 | 内容 |
| --- | --- |
| [spec.md](../../../specs/003-sdd-loop-harness/spec.md) | 要件と成功基準（FR-001〜FR-022 / SC-001〜SC-006） |
| [plan.md](../../../specs/003-sdd-loop-harness/plan.md) | 技術的な進め方、ゲート判定、配置 |
| [research.md](../../../specs/003-sdd-loop-harness/research.md) | 未確定事項の解消（R-001〜R-009） |
| [data-model.md](../../../specs/003-sdd-loop-harness/data-model.md) | 状態・段階・フェーズ・ホップの定義 |
| [contracts/](../../../specs/003-sdd-loop-harness/contracts/) | `sdd-state.sh`・`sdd-guard.sh`・スキル・routine の契約 |
| [quickstart.md](../../../specs/003-sdd-loop-harness/quickstart.md) | 受け入れの検証手順（S1〜S8） |
| [tasks.md](../../../specs/003-sdd-loop-harness/tasks.md) | タスク分解（T001〜T035） |

## 検証の方針

`quickstart.md` の S1〜S8 をもって完了を判定する。S1〜S2（判定の検算）は手元と CI で
自動化する（`make test-sdd`）。S3〜S8 は routine と GitHub を使う実機確認で、自動化しない。

**routine の作成はリポジトリ外の作業である。** claude.ai/code/routines で保守者が行う。
手順と設定値は [contracts/routine.md](../../../specs/003-sdd-loop-harness/contracts/routine.md)
にあり、実装では写しを [docs/references/sdd-routine.md](../../references/sdd-routine.md) に
置いた。以後はそちらを真実とする（FR-021）。**S3（プローブ）の前に保守者がこの routine を
作る必要がある。** 本 tasks.md には含めていない。

## 進捗

| 区分 | 状態 |
| --- | --- |
| 設計 | 完了（2026-09-13）。[設計文書](../../design-docs/sdd-loop-harness.md) |
| 仕様 | 完了（2026-09-13） |
| 計画・設計成果物 | 完了（2026-09-13） |
| タスク分解 | 完了（2026-09-13）。T001〜T035 |
| Phase 1: Setup | 未着手 |
| Phase 2: Foundational（判定の核とテスト） | 未着手 |
| Phase 3: US1（マージで次の段階が始まる） | 未着手 |
| Phase 4: US2（暴走しない・重複しない） | 未着手 |
| Phase 5: US3（手元で検算できる） | 未着手 |
| Phase 6: US4（使用量ゲート） | 未着手 |
| Phase 7: Polish | 未着手 |
| 受け入れ検証（S1・S2） | 未着手 |
| 受け入れ検証（S3〜S8） | 未着手（保守者の実機作業） |

## 決定の記録

（実装の進行に合わせて追記する）

## 保守者に残る作業

| 作業 | 参照 |
| --- | --- |
| `sdd` ラベルの作成・確認 | [docs/references/sdd-routine.md](../../references/sdd-routine.md) の「前提」 |
| routine の作成 | 同「作成手順」。S3 の前に行う |
| S3（プローブ）の実行と結果の書き戻し | [quickstart.md](../../../specs/003-sdd-loop-harness/quickstart.md) S3 |
| S4〜S8 の実機確認 | 同 S4〜S8 |
