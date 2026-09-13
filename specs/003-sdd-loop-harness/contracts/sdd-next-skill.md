# 契約: `/sdd-next` スキル

**Feature**: [spec.md](../spec.md) | **Design**: [設計文書 6 章](../../../docs/design-docs/sdd-loop-harness.md)

場所: `.claude/skills/sdd-next/SKILL.md`。routine のプロンプトと、保守者が web セッションで
手打ちする場合の両方から同じ手順で動く。

## 呼び出し

```
/sdd-next            # 次の 1 段階を実行して PR を開く
/sdd-next --dry-run  # 手順 0〜1 だけ実行し、何をするつもりかを表示して終わる
```

## 定数（SKILL.md の先頭に置く）

| 名前 | 値 | 用途 |
| --- | --- | --- |
| `USAGE_LIMIT_7D` | 80 | 7 日窓の使用率がこれ以上なら見送る（FR-020） |
| `USAGE_LIMIT_5H` | 70 | 5 時間窓の使用率がこれ以上なら見送る |
| `PHASE_RETRY_LIMIT` | 2 | `sdd-guard.sh` と同じ値。文書用（判定はスクリプト側） |

## 手順と、各手順の出力

| # | 手順 | 成功の条件 | 失敗時 |
| --- | --- | --- | --- |
| 0 | 前提確認: 作業ツリーが clean。必要に応じて feature branch を復元できる | 満たす | 理由を書いて終了。変更を残さない |
| 0.5 | 使用量ゲート: 使用率を取得し、しきい値と比較 | しきい値未満、または取得不能 | 「見送り」と書いて終了（PR・Issue 無し） |
| 1 | 判定: cloud では組み込み GitHub ツールで PR 一覧を `${TMPDIR:-/tmp}/sdd-github/` に置き、`before=$(sdd-state.sh)`、`guard=$(echo "$before" \| sdd-guard.sh --github-dir ...)`。以後の `before` は必ず `guard.state` で置き換える | `guard.go = true` | `reason` が `phase-retry-limit` / `hop-limit` なら Issue を立てて終了。それ以外は理由を書いて終了 |
| 1.5 | レビュー対応: open な `sdd` PR に未解決レビュー指摘があれば、対象 PR の head に修正 commit を積み、返信・resolve して終了 | 対象 PR が無い、または対応完了 | 判断待ちなら PR コメントで block 理由を書いて終了。新しい段階 PR は作らない |
| 1.6 | `--dry-run` なら `guard` を整形して表示して終了 | — | — |
| 2 | 準備: `git switch -c <state.branch>`、`export SPECIFY_FEATURE_DIRECTORY=<state.feature_dir>` | ブランチが切れる | 終了 |
| 3 | 段階の実行（下表） | 段階ごとの条件 | 段階ごとの扱い |
| 4 | 前進確認: `after=$(sdd-state.sh --feature <dir>)`、`before` と比較、`git status --porcelain` が非空 | 異なる かつ 差分あり | Issue（`no-progress`）を立て、ブランチを捨てて終了 |
| 5 | コミット・push・PR 作成・ラベル付与（下記） | PR の URL が得られ、ラベル `sdd` が付いている | cloud の組み込み GitHub ツールで 1 回再試行。それでも失敗なら PR 本文の先頭に「ラベル未付与」と書いて終了 |
| 6 | 報告: 段階・PR URL・次に起きること（「マージすると `<次の段階>` が始まる」または「これで完了」）を 3 行で出す | — | — |

## 段階の実行

| stage | 実行 | 検証 | 通らないとき |
| --- | --- | --- | --- |
| `plan` | `/speckit-plan` | `plan.md` と `research.md` が生成されている | 生成されていなければ手順 4 で `no-progress` になる |
| `tasks` | `/speckit-tasks` → `/speckit-analyze` | `tasks.md` が生成され、analyze の CRITICAL が tasks.md の範囲で解消済み | spec／plan に手を入れない。解消できない CRITICAL は PR 本文に残す |
| `implement` | `/speckit-implement "Phase <N>（<phase_title>）のタスクだけを対象にする。他のフェーズには手を付けない"` → 完了タスクを `[X]` に → `make check` | `make check` が通る | 直す。直せなければ draft PR にして本文に失敗内容を書く（FR-006） |

## ブランチ・コミット・PR・Issue の形式

**ブランチ**: `state.branch`（[data-model.md 4.](../data-model.md)）

**コミットメッセージ**: 既存の慣習に合わせて日本語の要約 1 行 + 空行 + 本文。末尾に
`Co-Authored-By: <セッションのモデル名> <noreply@anthropic.com>`（既存履歴は
`Claude Opus 5`）。プレフィックスは plan／tasks が `docs:`、
implement が `feat:`（テストのみなら `test:`）

**PR**:

| 項目 | 値 |
| --- | --- |
| base | `state.base_branch` |
| title | plan: `docs: NNN の実装計画と設計成果物を追加する` / tasks: `docs: NNN の実装タスクを分解する` / implement: `feat: NNN Phase N（<phase_title の先頭 30 文字>）を実装する` |
| label | `sdd`（必須） |
| draft | implement で `make check` が通らなかったときだけ `true` |
| body | 下の雛形 |

段階 PR を自動マージできたら remote の `state.branch` を削除する。同じ phase が続いたときでも
次回は最新 feature branch から同名 head を作り直す。削除できない場合は stale branch として
次回の実行を停止し、`--force` で上書きしない。

```markdown
## 段階

<stage>（機能 NNN、Phase N のとき: Phase N <phase_title>）

## 判定

- before: `<before の JSON>`
- after: `<after の JSON>`
- guard: `<guard の JSON>`

## 実行した検査

- <make check の結果 / analyze の要約 / なし>

## 残課題

- <あれば>

## 実行元

- session: <CLAUDE_CODE_REMOTE_SESSION_ID>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

**Issue**（停止通知、[data-model.md 7.](../data-model.md)）:

- title: `sdd-next 停止: NNN <reason>`
- body: 理由の説明、`state` と `guard` の JSON、session ID
- 作る前に cloud の組み込み GitHub ツールで open Issue を確認し、同じ title が無いことを確認する

## 使用率の取得（手順 0.5）

取得関数は `SKILL.md` に「取得手段」として 1 節設け、プローブの結果で埋める（R-004）。
実装前の状態では「`/tmp/sdd-rate-limits.json` があれば `rate_limits.five_hour.used_percentage`
と `rate_limits.seven_day.used_percentage` を読む。無ければ取得不能として飛ばす」とする。

## しないこと

- 2 段階以上を続けて進めない（FR-003）
- spec.md を書き換えない（tasks 段階の analyze でも）
- `main` に直接 push しない
- routine 側にしきい値や判定を持たせない
- cloud セッションで `gh` が使えることを前提にしない
