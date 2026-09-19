# Feature Specification: requirements.md と別担当者承認必須ルールの廃止

**Feature Branch**: `codex/remove-requirements-checklist`

**Created**: 2026-09-20

**Status**: Review

**Parent Issue**: #53

**Input**: User description: "https://github.com/syudead/vv/issues/53"

## Original Requirements

The following statements are preserved verbatim from GitHub Issue #53, created
by `syudead` on 2026-09-19.

- **RQ-001**: 「仕様の要求本体は `spec.md` に集約し、品質確認はPRレビューで行う。
  生成時の確認結果を `requirements.md` として永続保存しない。」
- **RQ-002**: 「`speckit-specify` から `checklists/requirements.md` の生成を削除する」
- **RQ-003**: 「`speckit-clarify` から同ファイルの更新処理を削除する」
- **RQ-004**: 「`speckit-implement` から同ファイルを使った実装開始ゲートを削除する」
- **RQ-005**: 「既存の `specs/*/checklists/requirements.md` を削除する」
- **RQ-006**: 「関連するテンプレート、仕様品質文書、手順書を更新する」
- **RQ-007**: 「要求の正本が `spec.md` に一本化されている」
- **RQ-009**: 「`requirements.md` を生成・参照する現行手順が残っていない」

The following later statements from the user conversation on 2026-09-20
supersede the earlier completion condition concerning a separate review record
and any interpretation of RQ-001 that requires a separate approval gate.

- **RQ-010**: 「そんなの要らないんだが」
- **RQ-011**: 「完全に消してくれ」
- **RQ-012**: 「手順に従わなくていいからついでに消してくれ」

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 仕様を一つの正本として作成する (Priority: P1)

仕様作成者は、要求と受け入れ条件を `spec.md` にまとめ、生成時の自己確認結果を
別の組み込みファイルとして残さずに次の工程へ渡す。

**Why this priority**: 要求の所在を一つに限定することが、この変更の中心的な価値である。

**Verification**: 新しい feature directory に対して仕様作成を完了し、要求と受け入れ条件が
`spec.md` に揃い、`checklists/requirements.md` が作成されていないことを確認する。

**Acceptance Scenarios**:

1. **Given** 新しい feature directory がある、**When** 仕様作成を完了する、**Then** 要求本体と
   受け入れ条件は `spec.md` に記録され、`checklists/requirements.md` は存在しない
2. **Given** 初回の品質確認で仕様に不備がある、**When** 仕様作成者が不備を解消する、**Then**
   修正結果は `spec.md` に反映され、確認結果だけを保存する別ファイルは作られない

---

### User Story 2 - 別担当者の承認なしで工程を進める (Priority: P1)

保守担当者は、仕様作成者とは別の reviewer や別セッションによる承認を用意しなくても、
仕様作成後の工程を進められる。通常の PR レビューは利用できるが、必須の品質ゲートではない。

**Why this priority**: 必須承認は求められておらず、作業を進めるためだけの追加セッションを
発生させるためである。

**Verification**: 仕様作成、計画、実装の各手順を確認し、別担当者による承認を開始条件として
要求する箇所がないことを確認する。

**Acceptance Scenarios**:

1. **Given** 作成者が仕様を完成させた、**When** 次の工程を開始する、**Then** 別担当者の
   approve や承認コメントを要求されない
2. **Given** Spec PR に通常のレビューコメントがある、**When** 保守担当者が対応を判断する、
   **Then** コメントは通常のレビュー情報として扱われ、専用の承認記録には変換されない

---

### User Story 3 - requirements.md なしで後続工程を進める (Priority: P1)

保守担当者は、既存仕様の明確化や実装を開始するとき、`checklists/requirements.md` の有無や
チェック状態に依存せず、`spec.md` と必要な計画成果物を使って工程を進める。

**Why this priority**: 生成だけを止めても後続工程が旧ファイルを参照すれば、廃止は完了しない。

**Verification**: `checklists/requirements.md` がない feature directory に対して明確化と実装開始の
各手順を実行し、旧ファイルを要求・生成・更新せずに所定の判断まで到達することを確認する。

**Acceptance Scenarios**:

1. **Given** `checklists/requirements.md` がない仕様、**When** 明確化を行う、**Then** 更新と再確認は
   `spec.md` に対して完了し、旧ファイルは作られない
2. **Given** 仕様と実装計画がある、**When** 実装開始条件を確認する、**Then**
   `checklists/requirements.md` の有無や内容は開始可否に使われない

---

### User Story 4 - 既存資産から旧ルールを除去する (Priority: P2)

保守担当者は、現行の手順、テンプレート、既存 feature artifacts を検索したとき、組み込み
`requirements.md` のライフサイクルや、別担当者による仕様承認の必須ルールに遭遇しない。

**Why this priority**: 古い説明や既存ファイルが残ると、廃止済みの運用が再開される恐れがある。

**Verification**: 対象パスを検索し、既存の組み込み `requirements.md` が 0 件で、現行手順と
テンプレートに旧ライフサイクルおよび必須承認ルールへの参照が 0 件であることを確認する。

**Acceptance Scenarios**:

1. **Given** 既存の feature artifacts がある、**When** 移行を完了する、**Then**
   `specs/*/checklists/requirements.md` はすべて削除される
2. **Given** 現行のワークフロー文書とテンプレートがある、**When** 利用者が仕様作成と実装開始の
   手順を読む、**Then** 別担当者の承認を必須とする指示は存在しない

### Edge Cases

- feature directory に `checklists/` が存在しない場合も、明確化と実装開始の手順は正常に進む。
- `checklists/` に利用者が明示的に作成した別用途のチェックリストがある場合、それらは
  組み込み `requirements.md` の廃止対象として一括削除されない。
- 古い `requirements.md` が利用者の作業ブランチに残っていても、現行手順はそれを読んだり
  更新したりせず、開始条件にも使わない。
- 自動レビューや通常の PR レビューが実行されても、その有無や判定は SDD の工程開始条件に
  ならない。
- 過去の Git 履歴や closed PR に旧ルールの記述が残っていても、現行手順として扱わない。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 各 feature の要求本体と受け入れ条件について、`spec.md` を唯一の永続的な正本と
  しなければならない。
- **FR-002**: 仕様作成手順は、仕様の品質を確認して必要な修正を `spec.md` に反映しなければ
  ならないが、`checklists/requirements.md` を生成してはならない。
- **FR-003**: 明確化手順は、変更後の `spec.md` を再確認しなければならないが、
  `checklists/requirements.md` の存在確認、読み取り、更新、作成をしてはならない。
- **FR-004**: 実装手順は、`checklists/requirements.md` の存在またはチェック状態を実装開始の
  条件として使ってはならない。
- **FR-005**: 現行のスキル、agent 案内、仕様品質文書、PR テンプレート、および Issue handoff
  手順は、仕様作成者とは別の reviewer または別セッションによる承認を工程開始条件として
  要求してはならない。
- **FR-006**: 必須の仕様承認だけを目的とする専用レビュー手順と、その承認ゲートを説明する
  tech-debt 記録を削除しなければならない。
- **FR-007**: 仕様品質の内容規則は仕様作成時に適用できるが、その確認結果を別ファイルへ保存したり、
  別担当者の承認を待つ条件にしたりしてはならない。
- **FR-008**: 既存の `specs/*/checklists/requirements.md` をすべて削除しなければならない。
- **FR-009**: 既存の plan などの feature artifacts から、組み込み `requirements.md` を現行成果物と
  して示す記述を除去しなければならない。
- **FR-010**: 利用者が明示的に要求して作る個別目的のチェックリストは引き続き利用でき、
  組み込み `requirements.md` の廃止を理由に削除してはならない。
- **FR-011**: 仕様作成時の自己確認結果は一時的な作業情報として扱い、後続工程の入力として
  永続化してはならない。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 作業完了後、`specs/*/checklists/requirements.md` に一致するファイルが 0 件である。
  (RQ-005, RQ-007)
- **SC-002**: 現行の仕様作成・明確化・実装手順と関連テンプレートに、組み込み
  `requirements.md` の生成・更新・参照・ゲート利用を求める記述が 0 件である。
  (RQ-002, RQ-003, RQ-004, RQ-006, RQ-009)
- **SC-003**: 現行の運用文書とテンプレートに、別担当者または別セッションによる仕様承認を
  必須とする記述が 0 件であり、その専用手順書が存在しない。(RQ-010, RQ-011, RQ-012)
- **SC-004**: `checklists/requirements.md` がない feature directory を使った仕様作成、明確化、
  実装開始の各検証が 100% 成功し、いずれも同ファイルを新規作成しない。
  (RQ-002, RQ-003, RQ-004, RQ-009)
- **SC-005**: 新規仕様の要求と受け入れ条件の 100% が `spec.md` から確認でき、生成時の
  自己確認だけを保存する別成果物が 0 件である。(RQ-001, RQ-007)
- **SC-006**: 組み込みチェックリスト以外の利用者作成チェックリストを含む検証用 feature で、
  対象外ファイルの削除件数が 0 件である。(RQ-005, RQ-006)

## Scope Boundaries

- 通常のコードレビューや任意の PR レビューは禁止しない。別担当者の仕様承認を必須ゲートに
  する規則だけを廃止する。
- 要求原文の保持、成功基準との対応、UI 品質、曖昧さを質問へ戻す規則は維持する。
- 利用者が明示的に依頼する個別目的のチェックリスト機能は廃止しない。
- Git 履歴、closed PR、Issue の変更履歴から過去の語句を消去することは対象外とする。
- 本仕様と Issue #53 が廃止対象を説明するために旧ファイル名や旧ルールを記載することは、
  現行手順からの参照には数えない。

## Assumptions

- PR の作成と人によるマージ判断は既存の branch workflow として残るが、approve の有無は
  SDD の工程開始条件にしない。
- 「既存の `specs/*/checklists/requirements.md`」は作業開始時に `main` に存在したすべての
  一致ファイルと、本 feature の Spec 作成時に生成された同ファイルを指す。
- 「関連するテンプレート、仕様品質文書、手順書」は、現行の仕様作成・明確化・実装開始・
  チェックリスト作成・PR 運用を案内するリポジトリ管理下の文書を指す。
