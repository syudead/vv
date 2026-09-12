# 実行計画: 初期セットアップ（Phase 0 骨組み）

- ステータス: 計画済み（実装未着手）
- 最終更新: 2026-09-12
- 対象: [技術選定文書](../../design-docs/tech-stack-selection.md) 「8. 実装の進め方」の Phase 0

## 目的

コードが存在しないリポジトリに、以降のフェーズが差分として積める最小の骨組みを置く。
「取得して1コマンドで起動し、ブラウザで稼働を確認でき、壊れた変更は自動検証で止まる」
状態を作ることが完了条件。

## 一次資料

詳細はすべて Spec Kit の成果物にある。本書は進捗と決定の記録に徹する。

| 文書 | 内容 |
| --- | --- |
| [spec.md](../../../specs/001-initial-setup/spec.md) | 要件と成功基準 |
| [plan.md](../../../specs/001-initial-setup/plan.md) | 技術的な進め方、ゲート判定、配置 |
| [research.md](../../../specs/001-initial-setup/research.md) | 未確定事項の解消（実測を含む） |
| [data-model.md](../../../specs/001-initial-setup/data-model.md) | Phase 0 で作る最小スキーマ |
| [contracts/](../../../specs/001-initial-setup/contracts/) | API・設定・コマンドの契約 |
| [quickstart.md](../../../specs/001-initial-setup/quickstart.md) | 受け入れの検証手順（S1〜S10） |
| [tasks.md](../../../specs/001-initial-setup/tasks.md) | 実装タスクの分解と実行順 |

## 検証の方針

`quickstart.md` の S1〜S10 をもって完了を判定する。S7〜S10 は CI でも実行され、
手元と同じ判定になることを前提にする。

## 進捗

| 区分 | 状態 |
| --- | --- |
| 仕様 | 完了（2026-09-12） |
| 計画・設計 | 完了（2026-09-12） |
| タスク分解 | 完了（2026-09-12、[tasks.md](../../../specs/001-initial-setup/tasks.md) に 43 タスク） |
| 実装 | 未着手 |
| 受け入れ検証（S1〜S10） | 未着手 |

## 決定の記録

- **2026-09-12: SQLite ドライバは `modernc.org/sqlite` で確定。** 技術選定文書が
  Phase 0 の検証事項としていた「FTS5 の trigram トークナイザが使えるか」を実測し、
  使えることを確認した（同梱 SQLite 3.53.4）。`mattn/go-sqlite3` への切り替えは行わない。
- **2026-09-12: 検索は `MATCH` と `LIKE` の2経路にする。** 同じ実測で、trigram は
  2文字以下の検索語に一致しないことが分かった。日本語では2文字の検索語が多いため、
  3文字以上は `MATCH`、1〜2文字は FTS5 表への `LIKE` に振り分ける。実装は Phase 2、
  両経路が成立することの固定は Phase 0 のテストで行う。詳細は
  [research.md R-001](../../../specs/001-initial-setup/research.md)。
- **2026-09-12: TanStack Router／Query の導入は Phase 1 へ送る。** Phase 0 の画面は
  1つで、ルーティングもサーバー状態のキャッシュも必要ない。
- **2026-09-12: 憲章（`.specify/memory/constitution.md`）は雛形のまま。** 計画の
  ゲートは `ARCHITECTURE.md` と技術選定文書の判断基準から導出した。憲章を作る場合は
  この計画と矛盾しないか確認すること。
