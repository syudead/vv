# Data Model: 初期セットアップ（Phase 0 骨組み）

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-12

Phase 0 が扱うデータは、**起動経路を通すために必要な最小限**に限る。
利用者データ（タグ・再生位置）は持たない。したがってこの時点のデータベースは
全体が「消してもスキャンで再構築できるインデックス」であり、
バックアップ対象は存在しない（技術選定文書の判断基準2）。

列の全体像は技術選定文書
[3.2 データモデルの要点](../../docs/design-docs/tech-stack-selection.md)にあるが、
Phase 0 ではその部分集合だけを作り、Phase 1 で拡張する。

---

## 1. プロセス内の値（永続化しない）

### アプリケーション設定 `Config`

`cmd/mdm` が環境変数から組み立てる不変の値。既定値のみで起動できる。
項目と既定値の契約は [contracts/configuration.md](./contracts/configuration.md)。

| フィールド | 型 | 既定値 | 検証規則 |
| --- | --- | --- | --- |
| `Addr` | string | `:8080` | `net.SplitHostPort` で解釈できること |
| `DataDir` | string | `/data` | 絶対パスであること。存在しなければ作成する（作成失敗は起動中止） |
| `LogLevel` | string | `info` | `debug` / `info` / `warn` / `error` のいずれか |

- 導出値: データベースのパスは `DataDir/mdm.db` に固定し、設定項目にしない。
  置き場所が1箇所に決まっていれば、バックアップと削除の手順が単純になる。
- 不正な値は**起動時にまとめて検証**し、どの環境変数のどの値が不正かを列挙して終了する。
  1つ見つけて即終了しない（設定を1回直すごとに再起動する往復を避ける）。

### 稼働情報 `Health`

`internal/domain` が持つ値。外部 I/O に依存しない。

| フィールド | 型 | 説明 |
| --- | --- | --- |
| `Status` | enum(`ok`, `degraded`) | データベースへの疎通が取れていれば `ok`、取れなければ `degraded` |
| `Version` | string | リリース名。既定は `dev`（[R-007](./research.md)） |
| `Commit` | string | `vcs.revision`。取得できない場合は空 |
| `BuiltAt` | time | `vcs.time`。取得できない場合はゼロ値 |

- 状態遷移: `ok` ⇄ `degraded` の2値のみ。判定は要求のたびに行い、状態を保持しない。
- 表現形式は [contracts/openapi.yaml](./contracts/openapi.yaml) の `Health` スキーマに従う。

---

## 2. 永続化する構造（SQLite）

### `goose_db_version`（goose が管理）

適用済みマイグレーションの版。アプリケーションは読み取り専用で扱い、
直接書き換えない。

| 列 | 型 | 説明 |
| --- | --- | --- |
| `id` | integer pk | 連番 |
| `version_id` | integer | マイグレーション番号 |
| `is_applied` | boolean | 適用済みか |
| `tstamp` | timestamp | 適用時刻 |

**起動時の規則**

1. 未適用のマイグレーションがあれば適用する（FR-005）。
2. データベースが**アプリケーションの知らない将来の版**を持っていた場合は、
   何も書き換えずに起動を中止する（ダウングレードによる破壊を防ぐ）。
3. データベースファイルが存在しない場合は作成して 1. を行う（SC-006）。
4. 接続時に `journal_mode=WAL`、`busy_timeout`、`foreign_keys=ON` を設定する。

### `videos`（Phase 0 は最小列のみ）

Phase 0 でこの表を作る理由は、日本語の部分一致検索の実証（FR-014／SC-007）を
実際に適用されたスキーマの上で行うためである
（[plan.md](./plan.md) の Complexity Tracking に記載）。

| 列 | 型 | 制約 | 説明 |
| --- | --- | --- | --- |
| `id` | integer | primary key | 行 ID。FTS5 の `rowid` と対応させる |
| `path` | text | not null, unique | ファイルの絶対パス。保存前に Unicode NFC へ正規化する |
| `title` | text | not null | 表示名。Phase 0 では拡張子を除いたファイル名 |
| `size_bytes` | integer | not null | ファイルサイズ |
| `mtime` | integer | not null | ファイルの更新時刻（Unix 秒） |
| `added_at` | integer | not null, default | 登録時刻（Unix 秒） |

Phase 1 で `content_key`・コンテナ／コーデック・尺・解像度を追加する。
Phase 0 ではスキャナが動かないため、この表に行を入れるのは自動テストだけである。

### `videos_fts`（FTS5 仮想表）

```text
fts5(title, path, content='videos', content_rowid='id', tokenize='trigram')
```

- 外部コンテンツ方式にして本文の二重保持を避ける。`videos` への
  `INSERT`／`UPDATE`／`DELETE` に対応するトリガで同期する。
- **検索の規則（[R-001](./research.md) の実測に基づく）**

  | 検索語の長さ | 経路 | 根拠 |
  | --- | --- | --- |
  | 3文字以上 | `videos_fts MATCH ?` | trigram の索引が効く |
  | 1〜2文字 | `videos_fts` に対する `LIKE '%…%'` | trigram では `MATCH` が一致しない。`LIKE` は trigram 索引で処理される |

  この分岐の実装自体は Phase 2（検索機能）だが、**両経路が成立することの証明**は
  Phase 0 のテストで固定する。「2文字は `MATCH` で0件になる」も期待値として書く。

### 一貫性と再構築

- `videos_fts` は `insert into videos_fts(videos_fts) values ('rebuild')` で
  いつでも再構築できる。トリガの取りこぼしが疑われたときの復旧手段とする。
- データベースファイルを削除した場合、次の起動でスキーマが再作成される。
  Phase 0 では利用者データが無いため、これで完全に復旧する。
