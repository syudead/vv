# 実行計画: Issue 種別で UI 設計ワークフローを選択する

- ステータス: 完了
- 最終更新: 2026-09-17
- 対象: GitHub Issue #42、PR #49

## 目的

UI 変更の判定をタスクやファイルから推測せず、親 Issue の種類で SDD の状態遷移を決める。
UI 型 Issue では実装前に UI/interaction design を挟み、通常 Issue は従来の軽量な経路を保つ。

## 状態遷移

- 通常: `plan → tasks → implement → done`
- UI: `plan → design → tasks → implement → done`

UI 型は親 Issue の `ui` ラベルで判定する。design の成果物は feature directory の
`ui-design.md` とする。

## 境界

design は既存要件を画面・操作へ具体化する。レイアウト、視覚階層、レスポンシブ、状態、
interaction、accessibility、視覚評価基準を含む。ユーザー調査、課題探索、要件再定義、
情報設計全体の再構築は含めない。

## 完了条件

- `sdd-state.sh --workflow ui` が design 段階を返せる
- 通常 workflow の既存出力が変わらない
- `/sdd-next` が親 Issue の `ui` ラベルから workflow を選ぶ
- UI design と実装後の visual / interaction review が手順化される
- Phase marker とファイルパス判定が残らない
- 自動テストと契約文書が新しい状態遷移を検証する

## 結果

- 親 Issue の `ui` ラベルを workflow 選択の唯一の入力にした
- `sdd-state.sh --workflow ui` に design 段階を追加した
- design 成果物を `ui-design.md` とし、UI/interaction design の境界を明文化した
- Phase marker、領域分類スクリプト、変更パス判定を削除した
- standard workflow の既存出力を維持した
