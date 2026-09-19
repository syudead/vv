# Contract: Spec artifact and workflow gates

## Specify

**Input**: 明示された親 Issue と feature directory。

**Required output**:

- `specs/<feature>/spec.md`
- `spec.md` 内の `**Parent Issue**: #NNN`
- 解消済みの仕様品質上の問題

**Forbidden output**:

- 組み込み `checklists/requirements.md`
- 自己検証結果だけを保存する別成果物
- 別担当者による approve を待つ工程状態

## Clarify

**Input**: 既存の `spec.md`。

**Required behavior**:

- 回答を `spec.md` に統合する
- 更新後の整合性と残存する曖昧さを確認する

**Forbidden behavior**:

- 組み込み `requirements.md` の存在確認、読み取り、作成、更新
- 別担当者の承認状態による完了判定

## Implement preflight

**Input**: 明示された作業単位、`spec.md`、`plan.md`、利用可能な設計成果物。

**Required behavior**:

- 明示された作業範囲と必要な成果物を確認する
- 利用者が明示的に作ったカスタムチェックリストがある場合は、その既存ルールを適用する

**Forbidden behavior**:

- 組み込み `requirements.md` の有無または checkbox 状態を開始条件にする
- 仕様作成者とは別の reviewer、agent、セッションの approve を開始条件にする

## Repository migration

- `specs/*/checklists/requirements.md` は存在してはならない。
- 現行のスキル、テンプレート、handbook、how-to、plan は組み込みファイルのライフサイクルを
  指示してはならない。
- 通常の PR レビューと明示的なカスタムチェックリストは引き続き利用できる。
