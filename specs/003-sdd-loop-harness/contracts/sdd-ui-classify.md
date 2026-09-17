# 契約: `sdd-ui-classify.sh`（UI 変更判定）

**Feature**: [spec.md](../spec.md) | **Design**: [設計文書 6 章](../../../docs/design-docs/sdd-loop-harness.md)

場所: `.claude/skills/sdd-next/scripts/sdd-ui-classify.sh`

## 呼び出し

```bash
git diff --name-only > /tmp/sdd-ui-paths.txt
sdd-ui-classify.sh [--root <repo_root>] [--feature <feature_dir>] [--paths <file>]
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| `--root` | カレントディレクトリ | リポジトリ root |
| `--feature` | なし | 対象機能。`specs/NNN-name` の相対パス |
| `--paths` | 標準入力 | `git diff --name-only` 形式の変更パス一覧 |

## 判定規則

1. `--feature` が指定され、`spec.md` / `plan.md` / `tasks.md` のいずれかに
   `<!-- sdd-ui-change: yes -->` または `<!-- sdd-ui-change: no -->` がある場合は、その明示分類を優先する。
2. 明示分類が無い場合、次のパスが含まれていれば UI 変更とする。
   `web/index.html`、`web/tailwind.config.ts`、`web/src/**/*.tsx`、`web/src/**/*.ts`、`web/src/**/*.css`、
   `docs/screenshots/*`、`specs/*/assets/*`。
3. ただし `web/src/api/*`、`web/src/api/gen/*`、`web/src/preferences/*`、`web/src/theme/*` は、
   単独では UI 変更としない。表示が変わる API 変更など、パスだけで拾えないものは spec の明示分類を使う。

## 出力

標準出力に JSON 1 行を返す。

```json
{"ui_change":true,"source":"path","matched_path":"web/src/pages/LibraryPage.tsx"}
```

`source` は `spec-explicit` または `path`。`matched_path` は明示分類では空文字にする。

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた |
| 2 | `--root` が存在しない、`--feature` のディレクトリが存在しない、`--paths` が読めない、引数の誤り |

## テスト

`.claude/skills/sdd-next/tests/run.sh` が、明示 `yes`、明示 `no`、UI パス、非 UI パスを検証する。
