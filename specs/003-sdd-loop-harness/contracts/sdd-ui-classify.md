# 契約: `sdd-ui-classify.sh`（implement ループ判定）

**Feature**: [spec.md](../spec.md) | **Design**: [設計文書 6 章](../../../docs/design-docs/sdd-loop-harness.md)

場所: `.claude/skills/sdd-next/scripts/sdd-ui-classify.sh`

## 呼び出し

実装前のループ選択:

```bash
sdd-ui-classify.sh --feature <feature_dir> --phase <N>
```

実装後の分類漏れ検査:

```bash
{ git diff --name-only HEAD; git ls-files --others --exclude-standard; } | sort -u > /tmp/sdd-ui-paths.txt
sdd-ui-classify.sh --feature <feature_dir> --phase <N> --paths /tmp/sdd-ui-paths.txt
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| `--root` | カレントディレクトリ | リポジトリ root |
| `--feature` | 必須 | 対象機能。`specs/NNN-name` の相対パス |
| `--phase` | 必須 | 対象 Phase の 1 以上の整数 |
| `--paths` | なし | 実装後にだけ渡す変更パス一覧 |

## Phase 領域分類

`tasks.md` の各 `## Phase N:` 節は、次のメタデータを 1 行だけ持つ。

```markdown
<!-- sdd-domains: frontend-ui, backend -->
```

有効な値は以下とする。

| domain | 実装領域 |
| --- | --- |
| `frontend-ui` | 描画、配置、スタイル、操作などブラウザ上の見た目・挙動 |
| `frontend-non-ui` | 画面を変えないフロントエンドのロジック、生成コード、テスト |
| `backend` | サーバー、ストレージ、API、バックグラウンド処理 |
| `infrastructure` | build、CI、配布、デプロイ、実行環境設定 |
| `documentation` | 文書だけの変更 |

分類は変更予定ファイルではなく、その Phase で AI ハーネスの実装ループを変える必要がある領域を
表す。`frontend-ui` を含む場合、実装開始前から UI 専用ループを選ぶ。

## 実装後の安全網

`--paths` がある場合だけ変更パスを検査する。Phase が `frontend-ui` を含まないのに UI 実装パスを
検出した場合、`classification_mismatch=true` を返す。`*.test.ts`、`*.test.tsx`、`*.spec.ts`、
`*.spec.tsx`、`__tests__` 配下は UI 実装パスから除外する。

このパス判定は分類漏れの検出専用であり、実装前のループ選択には使用しない。

## 出力

標準出力に JSON 1 行を返す。

```json
{"ui_change":true,"source":"phase-domains","domains":["frontend-ui","backend"],"classification_mismatch":false,"matched_path":""}
```

`source` は `phase-domains` または `path-safety-net`。

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた |
| 2 | 引数、root、feature、paths の誤り |
| 3 | `tasks.md` または対象 Phase の分類がない、分類行が複数、未知または重複した domain |

## テスト

`.claude/skills/sdd-next/tests/run.sh` が Phase の UI / 非 UI 分類、分類契約違反、実装後の分類漏れ、
テスト専用パスの除外を検証する。
