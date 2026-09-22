# 実行計画: Issueを引き継ぎ面にしたエージェント非依存SDD

- ステータス: リポジトリ実装済み。GitHub実機確認と外部Routine削除待ち

## 目的

Claude Routineやmergeイベントに依存せず、明示された親Issueまたはnative
sub-issueから任意のコーディングエージェントが一工程だけ実行できるようにする。

## 成果物と工程

- 標準: `specify -> plan -> plan-to-issues -> implement`
- UI: `specify -> plan -> design -> plan-to-issues -> implement`
- SDD成果物は`spec.md`、`plan.md`、必要な場合の`ui-design.md`だけとする。
- `plan.md`に実装作業の分解を含め、`plan-to-issues`がnative sub-issueを承認済みPlanから直接作る。
- `tasks.md`、Tasks工程、Tasks-to-sub-issues工程、永続task IDは使用しない。
- 実装はユーザーまたは子Issueが明示した一作業だけを対象にする。

## BranchとPR

- 長寿命feature branchは`main`から作る。
- stage PRとimplementation PRはfeature branchをbaseにする。
- integration PRだけが`main`をbaseにし、親Issueを`Closes`で参照する。
- branch名、Issue番号、feature directory番号に対応規則を設けない。
- 同工程のPRが複数あってもよい。review対象が明示された場合はそのheadを更新する。

## 状態と引き継ぎ

- 親IssueのSDD節はSpec、Plan、任意のDesign、Nextだけを持つ。
- `Next`は人向けの進捗表示であり、工程の実行条件にしない。
- 指定されたIssue、PR、branch、現在のcheckoutをそのまま作業文脈に使う。
- GitHubの標準PR参照とnative sub-issue関係は必要に応じて読む。
- branchやfeature directoryを復元する専用手順や照合は設けない。
- 専用状態ファイル、marker、wrapper、lock、agent packetは作らない。

## 残作業

- [ ] 独立した仕様品質reviewを完了する。
- [ ] native sub-issueの作成と親への追加を実機確認する。
- [ ] 外部Claude Routineを停止・削除する。
- [ ] legacy `sdd` labelを削除する。
- [ ] 利用可能な環境で`task check`を完走する。

## 完了条件

- 共通Agent SkillだけでSpecify、Plan、任意のDesign、単一作業のImplementを実行できる。
- Planからnative sub-issueを直接作成できる。
- すべてのPRでCIが実行される。
- integration PRのmergeで親Issueだけが閉じる。
- Tasks関連Skillと既存`tasks.md`がリポジトリに存在しない。
