# Issue handoff SDD

## Decision

保守者が親 Issue または native sub-issue の URL を渡したときだけ、agent が GitHub と
feature branch の状態から次の一工程を選び、その工程の PR を作る。通常の修正は focused
branch から `main` への PR とする。連続実行は保守者が `sdd-autopilot` を明示して開始する。

工程の選び方、PR の向き先、review と merge の条件は
[issue-handoff](../../.agents/skills/issue-handoff/SKILL.md) と
[sdd-autopilot](../../.agents/skills/sdd-autopilot/SKILL.md) に置く。ここに手順を複製しない。

## Why

旧方式は Claude Routine、`sdd` ラベル、`claude/sdd-*` という branch 名、session 固有の
状態に依存していた。別の agent が URL だけから作業を再開できるよう、要求は親 Issue、
進捗は feature branch の成果物、native sub-issues、merge 済み PR から読む。
親 Issue に工程のチェックリストを書き写す方式は、更新が漏れて実態と食い違ったため廃止した。
branch 名や feature directory の番号も識別子にしない。

Plan は判断を持ち、`ui` Issue では UI design を追加する。判断や検証に固有の内容がなければ
`research.md`、`data-model.md`、`contracts/`、`quickstart.md` は作らない。
要求の正本は親 Issue なので `spec.md` も作らない。成果物の要否は
[Plan 品質の規則](plan-quality.md) に従う。

## Integration PR を最後に開く理由

以前は Plan の merge 後すぐに integration PR を開いたため、未実装の範囲を review bot が
繰り返し欠陥として報告した。#135 では約 40 件の指摘のうち約 30 件が未着手範囲、PR 本文の
進捗、設計どおりの実装に対する返答で終わった。個々の変更は feature branch 向けの
stage / implementation PR で review される。そこで全 child が完了してから、feature branch
を `main` へ統合する PR を開く。統合 PR の merge は保守者が行う。
