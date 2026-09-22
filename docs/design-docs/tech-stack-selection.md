# 技術選定: MDM（Media Data Management）

- ステータス: 採用（第2版 / バックエンドを Go に変更）
- スコープ: 動画ファイルを管理し、ブラウザで再生できるシステムの初期実装

## 1. 前提と非スコープ

本ドキュメントは以下の前提で技術を選定する。前提が変わった場合は、
「6. 将来の拡張ポイント」に記した差し替え箇所から見直す。

| 項目 | 決定 |
| --- | --- |
| デプロイ形態 | セルフホスト（個人／自宅の1台、NAS や小型サーバー上の Docker） |
| クライアント | Web ブラウザのみ（PC／スマートフォン） |
| 動画の扱い | 既存ファイルをそのまま配信（H.264/AAC の mp4 を主対象） |
| 利用者数 | 単一ユーザー（同時視聴は1〜2セッション程度） |
| ライブラリ規模 | 〜数万本、数 TB をローカルディスク上に想定 |

非スコープ（初版では作らない）:

- 配信用トランスコード（HLS/DASH の生成）とそのためのワーカー基盤
- マルチテナント、ロールベースの権限管理、外部 IdP 連携
- CDN 配信、署名付き URL、DRM
- デスクトップ／モバイルのネイティブアプリ

## 2. 選定の判断基準

セルフホストの単一ユーザー用途であるため、スループットよりも
**運用の単純さ**と**壊れても復旧できること**を優先する。

1. **1プロセス・1コンテナで動くこと。** 常時稼働の外部ミドルウェア
   （Redis、メッセージブローカー、別 DB サーバー）を増やさない。
2. **ファイルシステムを唯一の真実とすること。** DB はインデックスであり、
   消してもスキャンで再構築できる状態を保つ。
3. **境界を機械的に守れること。** `ARCHITECTURE.md` の原則に沿い、ドメイン層が
   HTTP・DB・ffmpeg に依存しない構造を lint で強制する。
4. **段階的に重くできること。** トランスコードやマルチユーザーが必要になった
   ときに、書き直しではなく差し替えで対応できる境界を最初から引く。

## 3. 決定事項

| レイヤ | 採用 | 主な理由 |
| --- | --- | --- |
| バックエンド言語 | Go（現行安定版、1.26 系以降を想定） | 単一バイナリで配布でき、常駐プロセスと子プロセス管理が標準ライブラリで完結する。NAS 上でのメモリ使用量も小さい |
| HTTP サーバー | 標準ライブラリ `net/http`（Go 1.22 以降の `ServeMux`） | メソッド付きルーティングとパスワイルドカードが標準で使える。`http.ServeContent` が Range 配信を正しく実装済み |
| 動画配信 | `http.ServeContent` | Range／If-Range／複数レンジ／`206` の生成が標準実装。シーク動作を自前実装しないことが最大の利点 |
| フロント | React + Vite + TanStack Router/Query + Tailwind CSS | 静的ビルドを Go バイナリに `embed` して配るため、フロントは純粋な SPA でよい |
| API 契約 | OpenAPI 3.1 を真実とし、Go は `oapi-codegen`、TS は `openapi-typescript` で生成 | 2言語構成で唯一増えるコスト（型のずれ）を機械的に防ぐ |
| DB | SQLite（`modernc.org/sqlite`、CGO 不要、WAL モード） | 静的バイナリのままクロスコンパイルでき、alpine ベースの小さいイメージに載る |
| クエリ | `database/sql` で SQL を手書き | SQL を一次資料として保てる。選定時は `sqlc` での生成も想定したが、SQLite 対応の成熟度と、FTS5 の生 SQL を書く必要から採用していない |
| マイグレーション | `goose`（`embed.FS` にマイグレーションを同梱） | 外部ツールのインストール不要でバイナリ単体で適用できる |
| 全文検索 | SQLite FTS5（`tokenize='trigram'`） | 日本語をトークナイザ追加なしで部分一致検索できる。外部検索エンジン不要 |
| メディア解析 | `ffprobe` / `ffmpeg` を `os/exec` で実行（`context` でタイムアウト） | ラッパーを挟まず引数と失敗理由が明示的になる。プロセス停止の制御も標準機能で足りる |
| 字幕 | 外部 `.srt`/`.ass` をサーバーで WebVTT に変換して配信 | ブラウザは WebVTT のみ対応するため変換は必須 |
| 認証 | パスワード1つ（`golang.org/x/crypto/argon2` の Argon2id）＋ HttpOnly Cookie セッション（セッションは SQLite に保存） | 単一ユーザーに必要十分。外部公開は Tailscale / Cloudflare Tunnel 前提 |
| 非同期処理 | SQLite のジョブテーブル + goroutine のワーカー（`context` でグレースフル停止） | 別プロセスもブローカーも不要。再起動後にジョブを再開できる |
| ログ | 標準ライブラリ `log/slog`（JSON ハンドラ） | 追加依存なしで構造化ログになる |
| テスト | Go 標準 `testing` + `net/http/httptest`（Range の検証）+ Playwright（再生の E2E） | 「実際に再生が始まる」ことは E2E でしか担保できない |
| lint | `golangci-lint`（`depguard` で層をまたぐ import を禁止） | 依存方向の制約を CI で機械的に落とせる |
| 配布 | Docker（multi-stage、alpine + ffmpeg）+ Compose | CGO 不要なので alpine でそのまま動き、イメージが小さい |

### 3.1 リポジトリ構成

```text
cmd/
  mdm/           # main。設定読み込みと依存の組み立て
internal/
  domain/        # ドメインモデルとユースケース（外部 I/O への依存なし）
  httpapi/       # ハンドラ、ルーティング、ストリーミング、SPA の配信
  store/         # SQLite 実装、問い合わせと検索、マイグレーション
  media/         # ffprobe/ffmpeg アダプタ、サムネイル生成
  scanner/       # ファイルスキャンと差分検出
  jobs/          # ジョブキューとワーカー
api/
  openapi.yaml   # API 契約（Go/TS 双方のコード生成元）
web/             # React SPA。ビルド結果を embed して配信
```

依存方向は `cmd → {httpapi, store, media, scanner, jobs} → domain` の一方向に
限定する。`internal/domain` が `net/http`・`database/sql`・`os/exec` を import した
時点で CI を落とすよう、`golangci-lint` の `depguard` にルールを書く。
`internal/` 配下に置くこと自体が外部からの import を防ぐので、公開 API の
境界もコンパイラが守る。

### 3.2 データモデルの要点

- `videos`: ファイルパス、サイズ、mtime、`content_key`、コンテナ／コーデック、
  尺、解像度、追加日時。
- `content_key`: `先頭1MiB と末尾1MiB の SHA-256 ハッシュ + ファイルサイズ`。
  全体ハッシュは数 TB では非現実的なため、この組み合わせで
  「リネーム・移動されただけのファイル」を同一と判定する。
  ハッシュ関数は当初 BLAKE3 を想定していたが、**標準ライブラリの SHA-256**
  に決めた（実装は `internal/scanner/content_key.go`）。読む量が1ファイル
  あたり 2MiB に固定されているため、ハッシュ関数の速度は取り込み時間を
  律速しない（律速は `ffprobe` の起動とディスク I/O）。標準ライブラリなら
  依存を1つ増やさずに済み、amd64／arm64 ではハードウェア命令が使われる。
- `tags` / `video_tags`: 分類。階層は持たせず、命名規約（`series:xxx`）で表現する。
- `playback_progress`: 再生位置と視聴済みフラグ。プレイヤーから数秒間隔で更新。
- `videos_fts`: `title` と `path` の FTS5 仮想テーブル（trigram）。
- `jobs`: 種別、対象 ID、状態、試行回数、最終エラー。

DB は「再構築可能なインデックス」に限定する。タグ・再生位置などの
ユーザー入力データのみが再構築不能なので、この2種類はテーブル単位で分け、
バックアップ対象と復旧手順を区別する。

### 3.3 配信パスの設計

- `GET /api/videos/{id}/stream` は、権限確認のあと `os.Open` したファイルを
  `http.ServeContent` に渡すだけにする。Range の解釈・`206 Partial Content`・
  `Content-Range`・`Accept-Ranges` は標準実装に任せ、自前で書かない。
- それでも「シークが壊れていないこと」はリグレッションしやすいので、
  `httptest` で先頭・途中・末尾・不正範囲のテストを置く（標準実装の再テストでは
  なく、ハンドラが `ServeContent` に正しい `io.ReadSeeker` と ModTime を渡して
  いることの確認）。
- Linux では `http.ServeContent` からの `io.Copy` が `sendfile` に落ちるため、
  大きなファイルでもユーザー空間のコピーが発生しない。Node 構成で検討して
  いたリバースプロキシへの委譲（`X-Accel-Redirect`）は初版では不要。
- ブラウザの `<video>` は Cookie を送るため、ストリームも通常のセッション認証で
  保護できる（署名付き URL は不要）。

## 4. 採用しなかった選択肢

| 候補 | 却下理由 |
| --- | --- |
| TypeScript / Node.js バックエンド | 第1版では言語統一のため採用していたが撤回（「7. 決定の変更履歴」）。Range 配信とプロセス管理を自前で書く必要があり、ネイティブ依存（`better-sqlite3`）の再ビルド運用も抱える |
| Python + FastAPI | ライブラリは豊富だが、配布が重く、常駐ワーカーと依存管理の運用コストがセルフホスト用途に合わない |
| Echo / Gin / Fiber | 標準 `ServeMux` で足りる規模であり、ルーティングのために依存を増やす理由がない。Fiber は `net/http` 互換でないため `ServeContent` の利点も失う |
| GORM / ent | スキーマが小さく、FTS5 の生 SQL を書く必要がある。ORM の抽象より手書きの SQL のほうが読める |
| `mattn/go-sqlite3` | 成熟しているが CGO が必要で、クロスコンパイルと alpine ビルドが面倒になる。FTS5 にもビルドタグが必要。`modernc.org/sqlite` で FTS5 が使えない場合の代替として残す |
| PostgreSQL | 単一ユーザーには過剰。別コンテナとバックアップ運用が増える。マルチユーザー化時に再検討する |
| Meilisearch / Elasticsearch | 検索品質は上だが常駐プロセスが増える。FTS5 trigram で数万件なら実用的 |
| SvelteKit | 軽量で有力。React を選んだのはエコシステムと将来の人手確保の観点のみで、技術的な優劣ではない |
| 起動時の HLS 一括トランスコード | ストレージと CPU を大量に消費し、初版の要件（既存 mp4 の再生）に不要 |
| Redis + 外部ジョブキュー | ジョブはサムネイル生成程度で並列度1で足りる。SQLite のジョブテーブルと goroutine で代替できる |
| S3 / MinIO | ローカルディスクが真実である前提と合わず、Range 配信に中継が増える |
| Jellyfin / Plex の採用 | 既製品で要件は満たせるが、本リポジトリは自作を前提とする。機能比較の参照先としてのみ扱う |

## 5. 既知のリスクと対処

- **フロントとバックエンドの2言語化。** 型のずれと二重のビルド／CI が増える。
  API は `api/openapi.yaml` を唯一の真実とし、Go と TypeScript の両方を
  生成物にすることで、ずれをコンパイルエラーとして検出する。ビルドは
  `Taskfile.yml` の単一タスク（`task build` で SPA ビルド → embed → Go build）に
  まとめる。
- **`modernc.org/sqlite` の FTS5 と trigram トークナイザ。** 同梱 SQLite の
  ビルドオプション次第で使えない可能性がある。Phase 0 で
  「FTS5 仮想テーブルを `tokenize='trigram'` で作成して検索する」最小テストを
  置き、通らなければ `mattn/go-sqlite3` + `-tags sqlite_fts5` に切り替える
  （trigram は SQLite 3.34 以降が必要なので同梱バージョンも確認する）。
  **追記: 検証済み。** `modernc.org/sqlite` v1.58.0（同梱 SQLite 3.53.4）で
  trigram の作成と検索が動作したため、切り替えは行わない。ただし trigram は
  2文字以下の検索語に `MATCH` が一致しないことが判明したため、検索は
  「3文字以上は `MATCH`、1〜2文字は FTS5 表への `LIKE`」の2経路にする
  （振り分けは `internal/store/search.go` の `routeFor`、検証は
  `internal/store/fts_test.go`）。
- **非対応コーデックの混入。** H.265/VP9/mkv などはブラウザで再生できない。
  取り込み時に `ffprobe` で判定し、再生不可を UI に明示する。変換は
  「6. 将来の拡張ポイント」の対象。
- **ファイル名の Unicode 正規化。** 実在パスはファイルシステムが返した綴りを保持する。
  表示名と検索用文字列だけをNFCへ正規化する（`golang.org/x/text/unicode/norm`）。
  実在パスの正規化は、Linux等で別の存在しないentryを指し得るため行わない。
- **大量スキャン時の I/O 飽和。** 初回スキャンとサムネイル生成はワーカー並列度1で
  直列実行する。進捗は `jobs` テーブルから UI に出す。
- **バックアップ。** SQLite は `VACUUM INTO` でオンラインバックアップを取得する。
  動画本体はバックアップ対象外（再取得可能）とし、タグと再生位置のみを守る。

## 6. 将来の拡張ポイント

要件が増えたときに触る場所をあらかじめ決めておく。

| 将来の要件 | 差し替え箇所 |
| --- | --- |
| 非対応コーデックの再生 | `internal/media` にトランスコード実装を追加し、`stream` ハンドラを HLS プレイリスト配信に分岐。プレイヤー側は hls.js を追加 |
| 複数ユーザー・権限管理 | 認証を OIDC に差し替え、DB を PostgreSQL へ移行（SQL は `internal/store` に閉じている） |
| 外部公開・CDN | `stream` ハンドラを署名付き URL 発行に差し替え、実配信をプロキシ／CDN へ委譲 |
| ジョブの並列化 | ワーカーの goroutine 数を増やす。取得は SQLite の即時トランザクションで排他する |

## 7. 決定の変更履歴

- **第2版: バックエンドを TypeScript/Node（Hono）から Go へ変更。**
  第1版ではフロントとの言語統一を優先して Node を採用したが、方針として Go を
  採用する。技術的な利点は、単一バイナリでの配布、`http.ServeContent` による
  Range 配信の標準実装、`os/exec` と `context` による ffmpeg プロセス管理、
  CGO 不要な SQLite ドライバによる小さなコンテナイメージ。
  代償は2言語構成になることで、これは OpenAPI からのコード生成で緩和する
  （「5. 既知のリスクと対処」）。フロント（React + Vite）、SQLite + FTS5、
  ffmpeg の外部プロセス実行、ジョブを DB テーブルで持つ方針は第1版から不変。

## 8. 実装の進め方（フェーズ分け）

1. **Phase 0 — 骨組み。** Go モジュール、`internal/domain` の型、
   `golangci-lint` + depguard による境界検証、SQLite + FTS5 trigram の
   実証テスト、Docker Compose、CI。
2. **Phase 1 — 一覧と再生。** スキャナ、`ffprobe` によるメタデータ取得、
   `http.ServeContent` による配信、一覧ページ、`<video>` での再生。ここで
   「動画を管理して再生できる」を満たす。
3. **Phase 2 — 使える状態。** サムネイル（シークプレビュー用スプライト含む）、
   FTS5 検索、タグ、再生位置の保存、字幕変換。
4. **Phase 3 — 運用。** 認証、構造化ログ、バックアップ手順、E2E の整備。
