# Technical debt

Track deliberate compromises that need follow-up. Each entry should identify
the affected area, impact, intended resolution, and an owner or trigger for
reconsideration.

## 記録

### TD-001: trigram では2文字以下の検索語に `MATCH` が一致しない

- 影響範囲: 検索（`internal/store`、Phase 2 で実装）
- 内容: FTS5 の trigram トークナイザは3文字単位で索引を作るため、`旅行` のような
  2文字の検索語は `MATCH` で一致しない。日本語では2文字の検索語が多い。
- 当面の対処: 3文字以上は `MATCH`、1〜2文字は FTS5 表への `LIKE '%…%'`（trigram 索引で
  処理される）に振り分ける。経路が2つになる分、検索の実装と試験が複雑になる。
- 見直しの契機: 件数が増えて `LIKE` 経路の応答が実用的でなくなったとき。その時点で
  形態素解析ベースのトークナイザ（外部拡張）か、別の検索基盤を再検討する。
- 一次資料: [research.md R-001](../../specs/001-initial-setup/research.md)
