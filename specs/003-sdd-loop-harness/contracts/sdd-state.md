# 契約: `sdd-state.sh`（状態判定）

**Feature**: [spec.md](../spec.md) | **Data model**: [data-model.md](../data-model.md)

場所: `.claude/skills/sdd-next/scripts/sdd-state.sh`

## 呼び出し

```bash
sdd-state.sh [--root <repo_root>] [--feature <feature_dir>]
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| `--root` | カレントディレクトリ | `specs/` を探す起点。テストではフィクスチャのディレクトリを渡す |
| `--feature` | 自動選択 | 対象機能を固定する。`specs/NNN-name` の相対パス。指定した機能が `none` や `done` でもそのまま判定する |

## 出力

- 標準出力: JSON 1 行（末尾に改行）。属性の並びは [data-model.md 4.](../data-model.md) の順で固定
  （文字列比較で前進を判定するため）
- 標準エラー: 入力の異常のときだけメッセージ
- 依存: bash、awk、grep、sed のみ。`jq` を使わない

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた（`none` / `done` を含む） |
| 2 | `--root` が存在しない、`--feature` のディレクトリが存在しない、引数の誤り |

## 自動選択の規則

1. `<root>/specs/` 直下で `[0-9][0-9][0-9]-*` に一致するディレクトリを名前の昇順に並べる
2. 各機能を判定し、`stage` が `none` でも `done` でもない最初の機能を返す
3. 該当が無ければ `{"stage":"none"}` を返す（終了コード 0）

## JSON のエスケープ

`phase_title` に含まれる `"` と `\` はそれぞれ `\"`、`\\` にする。改行・タブは見出し行に
含まれないので扱わない。それ以外の文字（絵文字を含む）はそのまま UTF-8 で出す。

## 例

```bash
$ .claude/skills/sdd-next/scripts/sdd-state.sh
{"feature_dir":"specs/002-core-video-library","feature":"002","stage":"plan","branch":"claude/sdd-002-plan"}

$ .claude/skills/sdd-next/scripts/sdd-state.sh --feature specs/001-initial-setup
{"feature_dir":"specs/001-initial-setup","feature":"001","stage":"done","phases":6}

$ .claude/skills/sdd-next/scripts/sdd-state.sh --root .claude/skills/sdd-next/tests/fixtures/03-implement-mid
{"feature_dir":"specs/010-a","feature":"010","stage":"implement","phase":2,"phase_title":"Foundational (Blocking Prerequisites)","remaining":2,"total":3,"phases":3,"branch":"claude/sdd-010-implement-p2"}
```

## テスト

`.claude/skills/sdd-next/tests/run.sh` が `fixtures/*/` ごとに `--root` を渡して実行し、
`expected.json` と 1 行文字列として比較する。フィクスチャと検証する規則の対応:

| フィクスチャ | 検証する規則 |
| --- | --- |
| `01-before-plan` | `plan.md` 無し → `plan`、`branch` |
| `02-before-tasks` | `tasks.md` 無し → `tasks` |
| `03-implement-mid` | 未完了を含む最初のフェーズの選択、`remaining`/`total`/`phases`、題名の trim |
| `04-done` | `[x]` と `[X]` の混在をすべて完了と数える、`done` に `phases` を含む |
| `05-no-spec` | `spec.md` 無し → `none`、自動選択で飛ばされて `{"stage":"none"}` |
| `06-multi-feature` | `done` の機能を飛ばして次の機能を選ぶ、`--feature` で `done` の機能を明示できる |

## Feature branch fields（2026-09-13 改訂）

`plan` / `tasks` / `implement` は `feature_branch` と `base_branch` を追加で返し、値はいずれも
`claude/sdd-NNN-feature` とする。`done` は同じ `feature_branch` と `base_branch: main` を返す。
段階 `branch` は feature branch から作り、その branch 向け PR の head に使う。
