# Quickstart / 受け入れ検証: SDD ループハーネス

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-13

ハーネスが「できた」と言える状態を、実行して確かめる手順。各シナリオは
[spec.md](./spec.md) の受け入れ条件と成功基準に対応する。S1〜S2 は手元、S3 以降は
claude.ai の routine と GitHub で行う。実装の詳細はここに書かない。

## 前提

| シナリオ | 必要なもの |
| --- | --- |
| S1〜S2（判定の検算） | bash（Git Bash 可）。`jq`・`gh` は不要 |
| S3〜S9（実機） | routine が [contracts/routine.md](./contracts/routine.md) どおりに作られている、`sdd` ラベルがある、Claude GitHub App が入っている |

---

## S1: 判定の自動テストが通る（US3 / FR-012 / SC-003）

```bash
make test-sdd
```

**期待**: フィクスチャ 6 組（`01-before-plan` 〜 `06-multi-feature`）がすべて PASS。
`gh` を `PATH` から外した場合と、`gh api` の PR 一覧取得が失敗した場合に
`sdd-guard.sh` が `gh-unavailable` を返すテストも PASS。
同じコマンドが CI の Go ジョブでも実行され、同じ判定になる。

---

## S2: 手元で今の状態を検算できる（US3 / FR-010 / SC-002）

```bash
time .claude/skills/sdd-next/scripts/sdd-state.sh
.claude/skills/sdd-next/scripts/sdd-state.sh --feature specs/001-initial-setup
.claude/skills/sdd-next/scripts/sdd-state.sh | .claude/skills/sdd-next/scripts/sdd-guard.sh
```

**期待**:

- 1 行目: 今日の `main`（001〜003 は `done`、`004-library-ui` は spec だけ）なら
  `{"feature_dir":"specs/004-library-ui","feature":"004","stage":"plan","branch":"claude/sdd-004-plan"}`
  が 1 秒以内に返る。3 回実行して同じ
- 2 行目: `{"feature_dir":"specs/001-initial-setup","feature":"001","stage":"done","phases":6}`
- 3 行目（手元に `gh` が無い場合）: `{"go":false,"reason":"gh-unavailable",...}`。
  `gh` がある場合は `{"go":true,...}` と `hops:0`

---

## S3: プローブ — routine のセッションで前提を確かめる（R-004〜R-006）

routine の「Run now」に次の文を添えて実行する:

```
/sdd-next --dry-run を実行したあと、次を報告する:
(1) echo $CLAUDE_CODE_REMOTE と、make setup が SessionStart フックで実行されたか
(2) /tmp/sdd-rate-limits.json の有無と内容
(3) curl -s https://api.anthropic.com/api/oauth/usage の HTTP ステータス
(4) routine-fire-payload の中身（あれば）
(5) cloud セッションで組み込み GitHub ツールから PR 一覧を取得できたか
```

**期待**: `--dry-run` の出力に `guard` の JSON が含まれ、`go:true` かつ `stage:"plan"`
（`004-library-ui`）である。(1)〜(5) の結果を [research.md](./research.md) の「未解決事項の一覧」に
書き戻し、必要なら追従 PR を出す:

| 結果 | 追従 |
| --- | --- |
| (1) フックが走らない | routine の環境に setup script `make setup` を置く |
| (2) または (3) が使える | SKILL.md の「取得手段」を埋め、使用量ゲートを有効にする |
| どちらも使えない | `docs/exec-plans/tech-debt.md` に記録し、ゲートは無効のまま |
| (5) PR 一覧を取得できない | 組み込み GitHub ツールまたは GitHub App 権限の前提が崩れる。停止して見直す |

---

## S4: 本番 1 回目 — plan の PR が自動で開く（US1 / FR-003 / FR-004）

routine の「Run now」を引数なしで実行する。

**期待**: ブランチ `claude/sdd-004-plan` に `specs/004-library-ui/plan.md` 以下が
コミットされ、`main` 向けの PR が `sdd` ラベル付きで開く。PR 本文に before／after／guard の
JSON、実行元の session ID がある。セッションの最後の報告が 3 行で、「マージすると tasks が
始まる」とある。plan 以外の段階には進んでいない。

---

## S5: マージで連鎖する（US1 / FR-001 / SC-001）

S4 の PR をレビューしてマージする（`sdd` ラベルが付いていることを確認してから）。

**期待**: 数分以内に routine の新しい run が始まり、`claude/sdd-004-tasks` の PR が開く。
それをマージすると `claude/sdd-004-implement-p1` の PR が開く。以後、フェーズごとに
1 PR ずつ進み、最後のフェーズの PR をマージした次の run は PR を作らずに終わる
（`nothing-to-do`）。この間、保守者の操作はレビューとマージだけである。

---

## S6: 二重起動しても PR が増えない（US2 / FR-013 / SC-005）

S4 の PR を open のままにして、routine の「Run now」を 3 回実行する。

**期待**: 3 回とも `{"go":false,"reason":"open-pr",...}` で終わり、ブランチも PR も
増えない。Issue も作られない。

---

## S7: マージされずに閉じても連鎖しない（US2 / FR-002）

ハーネスが開いた PR を 1 つ、マージせずに Close する。

**期待**: routine の run が始まらない（run 一覧に新しい行が無い）。次の「Run now」または
日次実行で、同じ段階の PR が改めて開く（閉じた PR はホップに数えない）。

---

## S8: 上限で止まり、Issue で知らせる（US2 / FR-015 / FR-017 / SC-006）

同じフェーズの implement PR を意図的に 2 回マージした状態を作る（1 回目の PR の
チェックを一部だけ付けてマージし、2 回目も同様にする）。3 回目の run を待つか
「Run now」で起動する。

**期待**: PR は開かず、Issue `sdd-next 停止: 004 phase-retry-limit` が 1 件立つ。もう一度
「Run now」しても同じ Issue は増えない。Issue の本文だけで、止まった理由と状態が分かる。

---

## S9: open PR のレビュー指摘を同じ PR に返す

S4〜S5 で作られた open な `sdd` PR に inline review comment を付ける。修正内容は小さく、
同じ段階の範囲で直せるものにする。その後、日次実行を待つか routine の「Run now」を実行する。

**期待**: 新しい段階 PR は作られない。対象 PR の head branch に修正 commit が push され、
該当 review thread に対応内容と検査結果の返信が付く。解決できた thread は resolved になる。
仕様判断が必要なコメントの場合は、修正 commit を作らず PR コメントに block 理由が残る。

---

## 完了の判定

| 成功基準 | 確認するシナリオ |
| --- | --- |
| SC-001 操作はレビューとマージだけ | S5 |
| SC-002 1 秒以内・決定的 | S2 |
| SC-003 フィクスチャが CI で通る | S1 |
| SC-004 自動 PR の総数が上限を超えない | S5 + S8 |
| SC-005 二重起動で PR が増えない | S6 |
| SC-006 Issue だけで停止が分かる | S8 |

S1〜S9 がすべて期待どおりなら、`docs/exec-plans/active/003-sdd-loop-harness.md` を
completed へ移す。
