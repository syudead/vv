# 技術選定: MDM（Media Data Management）

- スコープ: 動画ファイルを管理し、ブラウザで再生できるシステムの技術選定

## 1. 選定の前提

以下は現在の選定理由である。現行の機能とデータの境界は
[`ARCHITECTURE.md`](../../ARCHITECTURE.md) に記す。

| 項目 | 決定 |
| --- | --- |
| デプロイ形態 | セルフホスト（個人／自宅の1台、NAS や小型サーバー上の Docker） |
| クライアント | Web ブラウザのみ（PC／スマートフォン） |
| 動画の扱い | 対応形式は元ファイルを配信し、非対応形式はリクエスト中にライブ変換する |
| 利用者数 | 単一アカウント。公開動画はゲストも閲覧できる |
| ライブラリ規模 | 〜数万本、数 TB をローカルディスク上に想定 |

## 2. 選定の判断基準

セルフホストの単一ユーザー用途であるため、スループットよりも
**運用の単純さ**と**壊れても復旧できること**を優先する。

1. **1プロセス・1コンテナで動くこと。** 常時稼働の外部ミドルウェア
   （Redis、メッセージブローカー、別 DB サーバー）を増やさない。
2. **再構築できる索引と利用者データを分けること。** 動画ファイルはスキャンで
   索引に戻せるが、再生位置や認証情報などは戻せない
   （[データの区別](../../ARCHITECTURE.md#rebuildable-and-user-data)）。
3. **境界を機械的に守れること。** `ARCHITECTURE.md` の原則に沿い、ドメイン層が
   HTTP・DB・ffmpeg に依存しない構造を lint で強制する。
4. **重い処理を境界の内側に置くこと。** `ffmpeg` を使う処理はアダプタに閉じ、
   HTTP や保存層の責務と混ぜない。

## 3. 決定事項

| レイヤ | 採用 | 主な理由 |
| --- | --- | --- |
| バックエンド言語 | Go（使用版は `go.mod`） | 単一バイナリで配布でき、常駐プロセスと子プロセス管理が標準ライブラリで完結する。NAS 上でのメモリ使用量も小さい |
| HTTP サーバー | 標準ライブラリ `net/http`（Go 1.22 以降の `ServeMux`） | メソッド付きルーティングとパスワイルドカードが標準で使える。`http.ServeContent` が Range 配信を正しく実装済み |
| 動画配信 | 対応形式は `http.ServeContent`、非対応形式はリクエスト単位の fragmented MP4 ライブ変換 | 元ファイルの Range 配信を標準実装に任せ、変換結果は保存しない（[ライブ変換のシーク](live-transcode-seek.md)） |
| フロント | React + Vite + React Router + Tailwind CSS | 静的ビルドを Go バイナリに `embed` して配る SPA。画面遷移は React Router で管理する |
| API 契約 | OpenAPI 3.1 を真実とし、Go は `oapi-codegen`、TS は `openapi-typescript` で生成 | 2言語構成で唯一増えるコスト（型のずれ）を機械的に防ぐ |
| DB | SQLite（`modernc.org/sqlite`、CGO 不要、WAL モード） | 静的バイナリのままクロスコンパイルでき、alpine ベースの小さいイメージに載る |
| クエリ | `database/sql` で SQL を手書き | FTS5 を含む SQL を一次資料として保てる |
| マイグレーション | `goose`（`embed.FS` にマイグレーションを同梱） | 外部ツールのインストール不要でバイナリ単体で適用できる |
| 全文検索 | SQLite FTS5（`tokenize='trigram'`） | 日本語をトークナイザ追加なしで部分一致検索できる。外部検索エンジン不要 |
| メディア解析 | `ffprobe` / `ffmpeg` を `os/exec` で実行（`context` でタイムアウト） | ラッパーを挟まず引数と失敗理由が明示的になる。プロセス停止の制御も標準機能で足りる |
| 認証 | ユーザー名とパスワードの組（`golang.org/x/crypto/argon2` の Argon2id）＋ HttpOnly Cookie セッション（セッションは SQLite に保存） | 単一ユーザーに必要十分。外部公開は HTTPS の逆プロキシを必須とする |
| 非同期処理 | SQLite のジョブテーブル + goroutine のワーカー（`context` でグレースフル停止） | 別プロセスもブローカーも不要。再起動後にジョブを再開できる |
| ログ | 標準ライブラリ `log/slog`（JSON ハンドラ） | 追加依存なしで構造化ログになる |
| テスト | Go 標準 `testing` + `net/http/httptest`（Range の検証）+ Playwright（再生の E2E） | 「実際に再生が始まる」ことは E2E でしか担保できない |
| lint | `golangci-lint`（`depguard` で層をまたぐ import を禁止） | 依存方向の制約を CI で機械的に落とせる |
| 配布 | Docker（multi-stage、alpine + ffmpeg）+ Compose | CGO 不要なので alpine でそのまま動き、イメージが小さい |

依存方向とパッケージの責務は [ARCHITECTURE.md](../../ARCHITECTURE.md#intended-dependency-direction) に記す。
SQLite には再構築できる索引と利用者データが共存するため、復旧時の区別は
[同文書のデータ分類](../../ARCHITECTURE.md#rebuildable-and-user-data) を正本とする。
ファイルの移動・改名後も同じ動画として扱う content key は、先頭と末尾の各 1 MiB
とファイルサイズから作る。ファイル全体を読まずに識別でき、標準ライブラリの
SHA-256 だけで実装できるためである。

## 4. 採用しなかった選択肢

| 候補 | 却下理由 |
| --- | --- |
| TypeScript / Node.js バックエンド | Range 配信とプロセス管理を自前で書く必要があり、ネイティブ依存（`better-sqlite3`）の再ビルド運用も抱える |
| Python + FastAPI | ライブラリは豊富だが、配布が重く、常駐ワーカーと依存管理の運用コストがセルフホスト用途に合わない |
| Echo / Gin / Fiber | 標準 `ServeMux` で足りる規模であり、ルーティングのために依存を増やす理由がない。Fiber は `net/http` 互換でないため `ServeContent` の利点も失う |
| GORM / ent | FTS5 を含む SQL を一次資料として保てるため、ORM の抽象より手書きの SQL を選んだ |
| `sqlc` | FTS5 を含む SQL をそのまま管理する方針で採用しなかった |
| `mattn/go-sqlite3` | CGO が必要で、クロスコンパイルと alpine ビルドの運用が増える |
| PostgreSQL | 単一アカウントの運用に別コンテナと別のバックアップ手順を増やしたくない |
| Meilisearch / Elasticsearch | 検索品質は上だが常駐プロセスが増える。FTS5 trigram で数万件なら実用的 |
| SvelteKit | 軽量で有力。React を選んだのはエコシステムと将来の人手確保の観点のみで、技術的な優劣ではない |
| 起動時の HLS 一括トランスコード | ストレージと CPU を大量に消費するため採用せず、リクエスト単位のライブ変換を使う |
| Redis + 外部ジョブキュー | 解析・サムネイル・プレビューは段階ごとに1件ずつ処理する。SQLite のジョブテーブルと goroutine で足りる |
| S3 / MinIO | ローカルディスクが真実である前提と合わず、Range 配信に中継が増える |
| Jellyfin / Plex の採用 | 既製品で要件は満たせるが、本リポジトリは自作を前提とする。機能比較の参照先としてのみ扱う |

## 5. 既知のリスクと対処

- **フロントとバックエンドの2言語化。** 型のずれと二重のビルド／CI が増える。
  API は `api/openapi.yaml` を唯一の真実とし、Go と TypeScript の両方を
  生成物にすることで、ずれをコンパイルエラーとして検出する。ビルドは
  `Taskfile.yml` の単一タスク（`task build` で SPA ビルド → embed → Go build）に
  まとめる。
- **FTS5 の trigram トークナイザ。** `modernc.org/sqlite` で作成と検索を検証済み。
  trigram は2文字以下の検索語に `MATCH` が一致しないため、検索は
  「3文字以上は `MATCH`、1〜2文字は照合用の鍵 `search_key` への `instr`」の2経路にする
  （振り分けは `internal/store/search.go` の `termUsesMatch`、検証は
  `internal/store/fts_test.go`）。
- **ファイル名の Unicode 正規化。** 実在パスはファイルシステムが返した綴りを保持する。
  表示名と検索用文字列だけをNFCへ正規化する（`golang.org/x/text/unicode/norm`）。
  実在パスの正規化は、Linux等で別の存在しないentryを指し得るため行わない。
- **大量スキャン時の I/O 飽和。** 解析・サムネイル・プレビューは段階ごとのワーカーが
  それぞれ1件ずつ処理する（段階内の並列度は1）。進捗は `jobs` テーブルから数え、
  `/api/events` で UI へ送る。
