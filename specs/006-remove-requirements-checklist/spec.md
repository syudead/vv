# Feature Specification: requirements.md の廃止

**Feature Branch**: `codex/remove-requirements-checklist`

**Created**: 2026-09-20

**Status**: Independent review required

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
- **RQ-008**: 「独立レビューの記録はPRレビューで確認できる」
- **RQ-009**: 「`requirements.md` を生成・参照する現行手順が残っていない」

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 仕様を一つの正本として作成する (Priority: P1)

仕様作成者は、要求と受け入れ条件を `spec.md` にまとめ、生成時の自己確認結果を
別の組み込みファイルとして残さずにレビューへ提出する。

**Why this priority**: 要求の所在を一つに限定することが、この変更の中心的な価値である。

**Independent Test**: 新しい feature directory に対して仕様作成を完了し、要求と受け入れ条件が
`spec.md` に揃い、`checklists/requirements.md` が作成されていないことを確認する。

**Acceptance Scenarios**:

1. **Given** 新しい feature directory がある、**When** 仕様作成を完了する、**Then** 要求本体と
   受け入れ条件は `spec.md` に記録され、`checklists/requirements.md` は存在しない
2. **Given** 初回の品質確認で仕様に不備がある、**When** 仕様作成者が不備を解消する、**Then**
   修正結果は `spec.md` に反映され、確認結果だけを保存する別ファイルは作られない

---

### User Story 2 - PR で独立した仕様レビューを行う (Priority: P1)

独立した reviewer は Spec PR 上で仕様品質を判定し、承認または具体的な指摘を PR レビューに
記録する。後続作業の担当者は、そのレビュー記録から承認済みかを確認できる。

**Why this priority**: 自己確認ファイルを廃止しても、独立レビューという品質ゲートを維持する
必要がある。

**Independent Test**: 独立した reviewer が Spec PR をレビューし、承認状態または未解決の指摘を
PR 上だけで確認できることを検証する。

**Acceptance Scenarios**:

1. **Given** 独立レビュー待ちの Spec PR がある、**When** reviewer が品質基準を満たすと判断する、
   **Then** 承認は PR レビューとして記録される
2. **Given** 仕様に不備がある、**When** reviewer が承認しない、**Then** 修正が必要な内容は
   PR 上の指摘として残り、承認されるまで実装へ進まない

---

### User Story 3 - requirements.md なしで後続工程を進める (Priority: P1)

保守担当者は、既存仕様の明確化や承認済み仕様の実装を開始するとき、
`checklists/requirements.md` の有無やチェック状態に依存せず、仕様と PR レビューを使って
工程を進める。

**Why this priority**: 生成だけを止めても後続工程が旧ファイルを参照すれば、廃止は完了しない。

**Independent Test**: `checklists/requirements.md` がない feature directory に対して明確化と実装開始の
各手順を実行し、旧ファイルを要求・生成・更新せずに所定の判断まで到達することを確認する。

**Acceptance Scenarios**:

1. **Given** `checklists/requirements.md` がない仕様、**When** 明確化を行う、**Then** 更新と再確認は
   `spec.md` に対して完了し、旧ファイルは作られない
2. **Given** 独立レビューで承認された仕様と実装計画がある、**When** 実装開始条件を確認する、
   **Then** `checklists/requirements.md` の有無や内容は開始可否に使われない

---

### User Story 4 - 既存資産から旧ライフサイクルを除去する (Priority: P2)

保守担当者は、現行の手順、テンプレート、既存 feature artifacts を検索したとき、
組み込み `requirements.md` の生成・更新・ゲート利用を示す記述に遭遇しない。

**Why this priority**: 古い説明や既存ファイルが残ると、利用者が廃止済みの運用を再開する恐れが
ある。

**Independent Test**: 対象パスを検索し、既存の組み込み `requirements.md` が 0 件で、現行手順と
テンプレートにそのライフサイクルへの参照が 0 件であることを確認する。

**Acceptance Scenarios**:

1. **Given** 既存の feature artifacts がある、**When** 移行を完了する、**Then**
   `specs/*/checklists/requirements.md` はすべて削除される
2. **Given** 現行のワークフロー文書とテンプレートがある、**When** 利用者が仕様品質と実装開始の
   手順を読む、**Then** PR 上の独立レビューを使う一貫した説明だけが示される

### Edge Cases

- feature directory に `checklists/` が存在しない場合も、明確化と実装開始の手順は正常に進む。
- `checklists/` に利用者が明示的に作成した別用途のチェックリストがある場合、それらは
  組み込み `requirements.md` の廃止対象として一括削除されない。
- 古い `requirements.md` が利用者の作業ブランチに残っていても、現行手順はそれを読んだり
  更新したりせず、開始条件にも使わない。
- 独立 reviewer が承認せず指摘を残した場合、ファイル上のチェック状態ではなく PR の
  未解決レビュー状態によって実装待ちであることが分かる。
- 仕様作成者の自己確認が成功しても、それを独立 reviewer の承認として扱わない。

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
- **FR-005**: 仕様作成者とは独立した reviewer による品質判定は Spec PR 上に記録し、承認前に
  実装へ進んではならない。
- **FR-006**: 仕様品質の規則とレビュー手順は、品質確認の成果を PR レビューで確認する運用を
  一貫して説明しなければならない。
- **FR-007**: 現行のスキル、テンプレート、PR 案内、および contributor 向け手順から、組み込み
  `requirements.md` の生成・更新・参照・ゲート利用を指示する記述を除去しなければならない。
- **FR-008**: 既存の `specs/*/checklists/requirements.md` をすべて削除しなければならない。
- **FR-009**: 既存の plan などの feature artifacts から、組み込み `requirements.md` を現行成果物と
  して示す記述を除去しなければならない。
- **FR-010**: 利用者が明示的に要求して作る個別目的のチェックリストは引き続き利用でき、
  組み込み `requirements.md` の廃止を理由に削除または自動承認してはならない。
- **FR-011**: 仕様作成時の自己確認結果は一時的な作業情報として扱い、独立レビューの証跡または
  後続工程の入力として永続化してはならない。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 作業完了後、`specs/*/checklists/requirements.md` に一致するファイルが 0 件である。
  (RQ-005, RQ-007)
- **SC-002**: 現行の仕様作成・明確化・実装手順と関連テンプレートに、組み込み
  `requirements.md` の生成・更新・参照・ゲート利用を求める記述が 0 件である。
  (RQ-002, RQ-003, RQ-004, RQ-006, RQ-009)
- **SC-003**: `checklists/requirements.md` がない feature directory を使った仕様作成、明確化、
  実装開始の各検証が 100% 成功し、いずれも同ファイルを新規作成しない。
  (RQ-002, RQ-003, RQ-004, RQ-009)
- **SC-004**: 新規仕様の要求と受け入れ条件の 100% が `spec.md` から確認でき、生成時の
  自己確認だけを保存する別成果物が 0 件である。(RQ-001, RQ-007)
- **SC-005**: 独立 reviewer の判定 1 件ごとに、承認または未解決の指摘を Spec PR 上で確認でき、
  仕様作成者本人の自己確認だけで承認済みとなるケースが 0 件である。(RQ-001, RQ-008)
- **SC-006**: 組み込みチェックリスト以外の利用者作成チェックリストを含む検証用 feature で、
  対象外ファイルの削除件数が 0 件である。(RQ-005, RQ-006)

## Scope Boundaries

- この変更は仕様品質の基準そのものを弱めず、確認結果の保存場所と後続工程の参照先を変える。
- PR レビュー機能や新しいレビュー管理システムは作らない。
- 利用者が明示的に依頼する個別目的のチェックリスト機能は廃止しない。
- Git 履歴、closed PR、Issue 本文などの履歴記録から過去の語句を消去することは対象外とする。
- 本仕様自身と Issue #53 が廃止対象を説明するために `requirements.md` を記載することは、
  現行手順からの参照には数えない。

## Assumptions

- Spec PR の review と approve は、独立レビューの判定と証跡を残す既存の仕組みとして利用する。
- 「既存の `specs/*/checklists/requirements.md`」は `main` に存在するすべての一致ファイルを指す。
- 「関連するテンプレート、仕様品質文書、手順書」は、現行の仕様作成・明確化・実装開始・
  チェックリスト作成・PR レビューを案内するリポジトリ管理下の文書を指す。
- 移行期間中に本 Spec を作成する現行手順が生成する品質チェックファイルは、この feature の
  実装時に他の既存ファイルとともに削除される。
