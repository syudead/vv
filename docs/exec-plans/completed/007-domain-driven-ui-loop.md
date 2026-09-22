# 実行計画: 領域分類で UI 実装ループを選択する

- ステータス: 廃止
- 対象: PR #49 の中間案

## 結論

Phase や変更ファイルから UI 変更を分類する案を検討したが、実装領域と workflow の種類を
混同していたため採用しなかった。分類用マーカーと専用分類スクリプトは削除した。

現在の設計では、親 Issue の `ui` ラベルを唯一の入力として UI workflow を選ぶ。
UI workflow は `plan → design → tasks → implement` と進む。詳細は
`009-issue-driven-ui-workflow.md` を参照する。
