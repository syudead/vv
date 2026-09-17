# 実行計画: UI 変更を専用の実装・視覚評価ループへ分離する

- ステータス: 完了
- 最終更新: 2026-09-17
- 対象: GitHub Issue #42。`/sdd-next` が UI 変更を通常の implement 経路と同じ扱いにせず、
  実ブラウザ確認と visual review を必須にする

## 目的

UI 変更を「コードの部品単位で完了」ではなく、ユーザーが見るページ単位で完成度を確認してから
PR にする。バックエンドや文書だけの変更では、現在の軽量な経路を維持する。

## 方針

- UI 変更の判定は、spec の明示分類（`<!-- sdd-ui-change: yes/no -->`）を優先し、
  明示が無い場合は対象パスから判定する
- UI 変更では、360px / 768px / 1280px のスクリーンショット、参照画像との比較、
  visual review、指摘修正と再撮影、interaction / accessibility の確認を必須にする
- 判定ロジックは `.claude/skills/sdd-next/scripts/` に置き、`make test-sdd` で検算できるようにする

## 進捗

| 項目 | 状態 |
| --- | --- |
| UI 変更分類スクリプト | 完了。`sdd-ui-classify.sh` を追加 |
| 自動テスト | 完了。`tests/run.sh` に明示分類とパス分類の検査を追加 |
| `/sdd-next` 手順 | 完了。UI 変更の専用ループと PR 本文要件を追加 |
| 文書 | 完了。設計文書、契約、スクリーンショット手順を更新 |
| 検証 | 完了。Git Bash login shell で `bash .claude/skills/sdd-next/tests/run.sh` が PASS |

## 完了条件

- `make test-sdd` が成功する
- UI 変更かどうかを spec の明示分類または対象パスから判定できる
- `/sdd-next` の手順が、UI 変更で実ブラウザ確認・比較画像・visual review・再撮影を省略できない
  形になっている
- PR 本文に `UI 変更なし` または UI 確認内容を記載する要件が明文化されている
