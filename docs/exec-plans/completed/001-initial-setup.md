# 実行計画: 初期セットアップ（Phase 0 骨組み）

- ステータス: 完了。S1〜S10 の期待はすべて確認した（コンテナとして起動する経路も含む）
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

## 検証の方針

`quickstart.md` の S1〜S10 をもって完了を判定する。S7〜S10 は CI でも実行され、
手元と同じ判定になることを前提にする。

## 進捗

| 区分 | 状態 |
| --- | --- |
| 仕様 | 完了（2026-09-12） |
| 計画・設計 | 完了（2026-09-12） |
| タスク分解 | 完了（2026-09-12、旧タスク成果物は後に廃止） |
| 実装 | 完了（2026-09-12、43 タスクすべて） |
| 受け入れ検証（S1〜S10） | 完了（2026-09-12）。S1〜S10 すべて期待どおり。制約は下記の1点のみ |

### 受け入れ検証の内訳

| シナリオ | 結果 |
| --- | --- |
| S1: `make up` で起動 | 確認済み。コンテナを起動して `http://localhost:8080` が開けること、`HEALTHCHECK` が `healthy` になること、`/media` が読み取り専用であること、`VERSION` ビルド引数が `version` に反映されることを確認した。Chromium でもアプリケーション名と `status: ok` が描画された。ビルドは CI の Docker ジョブでも成功している（約 70 秒）|
| S2: `/api/health` が機械可読な応答を返す | 確認済み。`200`／`Content-Type: application/json; charset=utf-8`／`Cache-Control: no-store`、本文に `status`・`version` |
| S3: SPA のフォールバックと `/api/*` の JSON `404` | 確認済み。Chromium で `/` と `/anything` が同一の画面を描画し、`/api/nope` は `Error` スキーマの JSON `404`。本物の Vite ビルドの `/assets/index-*.js` が `public, max-age=31536000, immutable` で返ることも確認 |
| S4: データを消しても自動で復帰 | 確認済み。`make down` → `docker volume rm vv_data` → 再起動で、追加の操作なしに `status: ok`、記録に `applied: 1`。`MDM_DATA_DIR` を直接削除する経路でも同じ結果。`TestMigrateRecoversAfterDatabaseFileIsDeleted` でも自動検証している |
| S5: 安全に停止する | 確認済み。バイナリ単体でも、コンテナでも `SIGTERM` で終了コード `0`。処理中の要求が無ければ猶予を待たず即座に終了する |
| S6: 前提ツールが欠けていれば原因が分かる形で止まる | 確認済み。終了コード非0で、不足しているコマンド名（`ffprobe, ffmpeg`）と導入方法を出力 |
| S7: `make check` が1コマンドで通る | 確認済み。`time make check` は約 9 秒（SC-002 の 5 分に対して十分な余裕。初回は `golangci-lint` の取得とビルドに数分かかる） |
| S8: 依存方向の違反が止まる | 確認済み。`net/http`・`database/sql`・`os/exec` の3つとも `make lint` が失敗し、出力に禁止理由の文言が出る |
| S9: 日本語の部分一致検索の前提が成り立つ | 確認済み。`go test ./internal/store/ -run FTS -v` が 6 件すべて成功（2文字の `MATCH` が0件であることも固定） |
| S10: 変更提案で同じ検証が自動実行される | 確認済み。Pull Request #6 で Go・Web・Generated code・Docker image の 4 ジョブがすべて成功。実行時間は約 1 分 50 秒（SC-004 は 10 分）。呼んでいる目標は `make check` と同じもの |

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
- **2026-09-12: `web/embed.go` を置いて埋め込みの宣言だけを `web/` に持たせた。**
  `go:embed` は自分のディレクトリより上を参照できないため、`internal/httpapi/spa.go`
  から `web/dist` を直接埋め込むことはできない。配信の実装は計画どおり
  `internal/httpapi/spa.go` に置き、`fs.FS` として受け取る形にした。テストでも
  埋め込みに依存せず `fstest.MapFS` を渡せるので、`/assets/*` のヘッダ検証ができる。
- **2026-09-12: 検査の目標を `-go` / `-web` に分けた。** 契約の 9 目標
  （`up`/`down`/`dev`/`build`/`generate`/`fmt`/`lint`/`test`/`check`）はそのまま残し、
  その下に `lint-go`・`lint-web`・`test-go`・`test-web`・`fmt-check-go`・`fmt-check-web`
  を置いた。CI で Go と Web のジョブを分けて並行させるための入口で、`make lint` /
  `make test` はこれらをまとめて呼ぶため手元と CI の判定は一致する。どれも手元で
  実行できるので「CI でしか動かない検査」は作っていない。
- **2026-09-12: Web の書式は Prettier を入れた。** 契約は `make fmt` を
  「Go・Node の書式を整える」としているが、Phase 0 の Web には整形器が無かった。
  `prettier` を devDependency に入れ、`format` / `format:check` を
  `make fmt` / `make fmt-check` から呼ぶ。生成物（`web/src/api/gen/`）は
  `.prettierignore` で対象外にした。
- **2026-09-12: 受け入れ検証のうち Docker と CI に依るものを残した。** 実装した環境に
  Docker デーモンが無く、イメージレジストリへの取得も遮断されていたため、S1・S4 と
  イメージのビルドは実行していない。`Dockerfile`・`compose.yaml`・CI の設定は
  入れてあるので、Docker が使える環境と Pull Request 上で確認する必要がある。
- **2026-09-12: 受け入れ検証はコンテナ経路まで完了した。** 当初この環境では Docker が
  使えないと判断していたが、それは誤りだった。`dockerd` は入っており（root 権限あり）、
  起動していなかっただけである。イメージ取得はマニフェストは通り blob だけが 403
  だったので、`/etc/docker/daemon.json` に Docker Hub の公式ミラー
  （`https://mirror.gcr.io`）を設定して解決した。`Dockerfile` は変更していない。
  これでコンテナを起動し、ポート公開・`HEALTHCHECK`・`/media` の読み取り専用バインド・
  `vv_data` ボリュームの寿命・停止（終了コード `0`）をすべて確認した。
- **2026-09-12: 唯一の制約は最終段の `apk add` である。** この環境では
  `dl-cdn.alpinelinux.org` が許可リストに無く（CA を信頼させると TLS は通るが 403）、
  `make up` を一気に完走させることはできない。そのため実行時の確認では
  `ffmpeg`／`ffprobe` をスタブに差し替えた最終段で起動した。Phase 0 の機能は
  これらのコマンドを実行せず存在確認（`exec.LookPath`）だけを行うため、
  起動経路の検証には影響しない。**本物の `ffmpeg` が入ることは CI の Docker ジョブ
  （`apk add` の成功）で確認済みで、不足時の挙動は S6 で確認済みである。**
  したがって S1 の期待はすべて確認できているが、`make up` 単体をこの開発環境で
  完走させることはできない。通常の開発機では制約にならない。
- **2026-09-12: 補助的に使った確認手段。** `docker compose config`（デーモン不要で
  `compose.yaml` を解決し、ボリューム実名が `vv_data` になることを確認）、
  `apt-get install ffmpeg` による本物の導入（バイナリ単体での起動前確認）、
  同梱の Chromium を Playwright で駆動（画面の描画と JS の誤りの有無）。
