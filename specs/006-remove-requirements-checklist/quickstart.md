# Quickstart: requirements.md と必須承認ルールの廃止を検証する

## Prerequisites

- リポジトリルートで実行する
- PowerShell 7 と `rg` が利用できる
- 対象差分が checkout 済みである

## S1: 組み込み requirements ファイルがない

```powershell
Get-ChildItem -Path specs -Recurse -File -Filter requirements.md
```

**Expected**: 出力が 0 件。

## S2: 現行手順に組み込みファイルのライフサイクルがない

```powershell
rg -n --hidden "requirements\.md|checklists/requirements|Specification Quality Checklist" `
  .agents .specify .github docs AGENTS.md specs `
  --glob '!specs/006-remove-requirements-checklist/spec.md' `
  --glob '!specs/006-remove-requirements-checklist/{plan,research,data-model,quickstart}.md' `
  --glob '!specs/006-remove-requirements-checklist/contracts/**' `
  --glob '!docs/product-specs/index.md' `
  --glob '!.git/**'
```

**Expected**: 出力が 0 件。

## S3: 別担当者の承認を必須にする現行ルールがない

```powershell
rg -n --hidden -i `
  "independent review|independent reviewer|independent-review|独立.{0,12}(レビュー|reviewer)|spec-quality-review|Independent review required" `
  . --glob '!.git/**' `
  --glob '!specs/006-remove-requirements-checklist/**'
```

**Expected**: 出力が 0 件。

## S4: 削除した専用手順へのリンクがない

```powershell
rg -n --hidden "docs/how-to/spec-quality-review|spec-quality-review\.md" . `
  --glob '!.git/**' `
  --glob '!specs/006-remove-requirements-checklist/**'
```

**Expected**: 出力が 0 件。

## S5: カスタムチェックリスト機能が残る

```powershell
rg -n "name: \"speckit-checklist\"|custom checklist|custom checklists|custom review artifacts" `
  .agents/skills/speckit-checklist/SKILL.md `
  .agents/skills/speckit-implement/SKILL.md
```

**Expected**: `speckit-checklist` の定義と、実装前確認における custom checklist の説明が見つかる。

## Acceptance

S1〜S5 がすべて期待どおりなら、[spec.md](spec.md) の SC-001〜SC-006 を満たすための
実装・移行・残存確認が再現可能である。
