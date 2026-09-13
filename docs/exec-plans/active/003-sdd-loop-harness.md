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

### `04-done` フィクスチャの期待値を tasks.md の T007 から変えた

T007 は `04-done/expected.json` を
`{"feature_dir":"specs/010-a","feature":"010","stage":"done","phases":2}` としていたが、
実際には `{"stage":"none"}` にした。`expected.json` は「引数なしで `--root <fixture>` を
渡したときの出力」であり、自動選択は `done` の機能を飛ばすと契約
（[contracts/sdd-state.md](../../../specs/003-sdd-loop-harness/contracts/sdd-state.md)
「自動選択の規則」）が定めているためである。同じ規則に `06-multi-feature` の期待値も
依存している。`done` の形（`phases` を含み `branch` を含まないこと）は、`feature.txt` と
`expected-feature.json` を足して `--feature` 経由で検証する。

### 手元の検算（T028）は Linux の cloud セッションで行った

T028 は Windows の Git Bash での検算を求めているが、本実装は Claude Code on the web の
cloud セッション（Linux）で行ったため、その環境では実行できていない。代わりに次を
確認した（2026-09-13）。

| 確認 | 結果 |
| --- | --- |
| `make test-sdd` | 14 件すべて PASS |
| `sdd-state.sh` の所要時間 | 0.025〜0.040 秒（3 回とも 1 秒以内、SC-002） |
| `sdd-state.sh` の決定性 | 3 回とも同一の出力 |
| `sdd-state.sh \| sdd-guard.sh` | `{"go":false,"reason":"gh-unavailable",...}`。このセッションには `gh` が無く、実際にこの経路を通った |
| フィクスチャが CRLF の場合 | `.md`・`.json`・`feature.txt` をすべて CRLF に変換した写しでも 14 件 PASS |

CRLF の検証は Windows のチェックアウトを模したものなので、**Git Bash での実行そのものは
保守者に残る**。`.gitattributes` で `*.sh` を `eol=lf` に固定してあるため、スクリプトが
CRLF になって落ちることはない（`git ls-files --eol` で確認済み）。

## 保守者に残る作業

| 作業 | 参照 |
| --- | --- |
| `sdd` ラベルの作成・確認 | [docs/references/sdd-routine.md](../../references/sdd-routine.md) の「前提」 |
| routine の作成 | 同「作成手順」。S3 の前に行う |
| S3（プローブ）の実行と結果の書き戻し | [quickstart.md](../../../specs/003-sdd-loop-harness/quickstart.md) S3 |
| S4〜S8 の実機確認 | 同 S4〜S8 |
