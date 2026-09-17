# 契約: `sdd-ui-classify.sh`（implement ループ判定）

**Feature**: [spec.md](../spec.md) | **Design**: [設計文書 6 章](../../../docs/design-docs/sdd-loop-harness.md)

場所: `.claude/skills/sdd-next/scripts/sdd-ui-classify.sh`

## 呼び出し

実装前のループ選択:

```bash
sdd-ui-classify.sh --feature <feature_dir> --phase <N>
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| `--root` | カレントディレクトリ | リポジトリ root |
| `--feature` | 必須 | 対象機能。`specs/NNN-name` の相対パス |
| `--phase` | 必須 | 対象 Phase の 1 以上の整数 |

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

## 出力

標準出力に JSON 1 行を返す。

```json
{"ui_change":true,"source":"phase-domains","domains":["frontend-ui","backend"]}
```

`source` は常に `phase-domains`。変更パスや拡張子は入力にも判定にも使用しない。

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた |
| 2 | 引数、root、feature の誤り |
| 3 | `tasks.md` または対象 Phase の分類がない、分類行が複数、未知または重複した domain |

先頭・末尾・連続するカンマは空の domain として終了コード 3 にする。

## テスト

`.claude/skills/sdd-next/tests/run.sh` が Phase の UI / 非 UI 分類と分類契約違反を検証する。
