# Data Model: 仕様成果物と工程ゲート

この feature はアプリケーションデータを追加しない。以下は SDD ワークフロー上の概念モデルである。

## Feature Specification

feature の要求と受け入れ条件を保持する唯一の永続的な正本。

| Field | Meaning | Validation |
| --- | --- | --- |
| Path | `specs/<feature>/spec.md` | feature ごとに 1 つ |
| Parent Issue | 要求元の GitHub Issue | 明示された Issue と一致する |
| Requirements | 機能要件と制約 | テスト可能で曖昧さを残さない |
| Success Criteria | 完了判定 | 元要求へ追跡できる |

## Transient Validation Result

仕様作成中に品質観点を確認した一時的な結果。リポジトリには保存しない。

| Field | Meaning | Validation |
| --- | --- | --- |
| Criterion | 確認した品質観点 | `speckit-specify` 内で定義される |
| Result | pass または修正が必要 | 修正が必要なら `spec.md` を更新する |
| Lifetime | 現在の仕様作成処理だけ | ファイル、PR 必須コメント、工程状態に変換しない |

## Custom Checklist

利用者が明示的に要求した目的別のレビュー補助。組み込みの仕様品質状態ではない。

| Field | Meaning | Validation |
| --- | --- | --- |
| Path | `specs/<feature>/checklists/<purpose>.md` | 名前と目的を利用者要求から決める |
| Items | UX、security、test などの品質質問 | 実装作業ではなく要求品質を扱う |
| Ownership | 明示的な依頼を行った利用者または reviewer | 自動生成・自動承認しない |

## Pull Request Review

通常の開発レビュー情報。SDD の必須状態としては管理しない。

| Field | Meaning | Validation |
| --- | --- | --- |
| Comment / Review | 任意の指摘または判定 | GitHub 上の通常機能として扱う |
| Approval | 任意の approve | Plan・実装開始の前提にしない |

## State Transitions

```text
Issue requirement
    -> spec.md draft
    -> transient validation
       -> correction needed: update spec.md and validate again
       -> passes: spec.md is ready for the requested next stage
```

`requirements.md` の作成、チェック状態の更新、別担当者 approve 待ちという状態遷移は存在しない。
