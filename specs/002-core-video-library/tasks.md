# Tasks: 絞られたコア機能（動画ライブラリの中核）

**Input**: Design documents from `/specs/002-core-video-library/`

**Prerequisites**: [plan.md](./plan.md)（必須）、[spec.md](./spec.md)（ユーザーストーリー）、[research.md](./research.md)、[data-model.md](./data-model.md)、[contracts/](./contracts/)

**Tests**: 本機能はテストを**含む**。[plan.md](./plan.md) の Technical Context が検証手段を
`go test`（`domain` の判定規則、`scanner` の走査規則、`store` の問い合わせと検索2経路）と
`net/http/httptest`（一覧・詳細・Range 配信・進捗）と定めており、
[quickstart.md](./quickstart.md) 末尾の「自動検証との対応」表が各シナリオの担保先を指定しているため。

**Organization**: タスクはユーザーストーリー単位にまとめてある。各ストーリーは独立して実装・
検証でき、そこで止めても価値が残る。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 並行実行可（別ファイル・依存なし）
- **[Story]**: 対応するユーザーストーリー（US1 / US2 / US3）
- 説明には必ず対象ファイルのパスを書く

## Path Conventions

本機能は **web-service**（単一 Go バイナリ + 埋め込み React SPA）。パスはすべてリポジトリ
root からの相対で、[plan.md](./plan.md) の "Source Code" の配置に従う。**新しいパッケージは
作らない**。001 が `doc.go` で宣言だけしていた `scanner`・`jobs`・`media` を、その宣言どおりに埋める。

- Go: `cmd/mdm/`、`internal/{domain,httpapi,store,media,scanner,jobs}/`
- 契約: `api/openapi.yaml`（Go／TS 双方の生成元、唯一の真実）
- Web: `web/src/`
- 生成物（`internal/httpapi/gen/`、`web/src/api/gen/`）は手編集しない

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 増える依存（2つだけ）と、API 契約の更新・生成物の再生成を済ませる

- [X] T001 `golang.org/x/text` を依存に追加する（Unicode NFC 正規化、[R-107](./research.md)）。`go get golang.org/x/text` を実行し、`go.mod` と `go.sum` の差分をコミットする。本機能で増やす Go の依存はこれ1つだけである（[plan.md](./plan.md) Primary Dependencies）
- [X] T002 [P] `react-router` v7 を追加する（[R-113](./research.md)）。`web/package.json` の `dependencies` に `"react-router": "^7"` を加え、`npm --prefix web install` で `web/package-lock.json` を更新する。データ取得ライブラリ（TanStack Query 等）は**入れない**
- [X] T003 `api/openapi.yaml` を [contracts/openapi.yaml](./contracts/openapi.yaml) の内容で更新する。`info.version` は `0.2.0`、追加する経路は `/api/videos`・`/api/videos/{id}`・`/api/videos/{id}/stream`・`/api/videos/{id}/thumbnail`・`/api/videos/{id}/progress`・`/api/scans`・`/api/scans/current`、追加するスキーマは `VideoSort`・`VideoPage`・`Video`・`ProgressUpdate`・`Progress`・`Scan`。以後もこのファイルが唯一の真実である
- [X] T004 `make generate` を実行して `internal/httpapi/gen/api.gen.go` と `web/src/api/gen/openapi.ts` を再生成する（T003 に依存）。この時点で `internal/httpapi/router.go` の `server` 型が `gen.ServerInterface` を満たさずコンパイルエラーになることを確認する（未実装の経路に気付ける設計であることの確認、[plan.md](./plan.md)）

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: すべてのストーリーが載るスキーマ・設定・判定規則を置く

**⚠️ CRITICAL**: このフェーズが終わるまで、どのユーザーストーリーの実装も始められない

- [X] T005 `internal/store/migrations/00002_core.sql` を作成する（goose の Up / Down 両方）。[data-model.md](./data-model.md) の表を、制約をそのまま写して定義する:
  - `videos` へ列を追加: `content_key` text **not null**（`unique`）、`duration_ms` integer（尺（ミリ秒）。`ffprobe` 取得前は `null`）、`width` integer、`height` integer、`container` text、`video_codec` text、`audio_codec` text、`playable` integer **not null 既定 0**（解析前は「再生できない」側に倒す）、`unplayable_reason` text（`container` / `video_codec` / `audio_codec` のいずれか）、`probe_state` text **not null**（`pending` / `done` / `failed`）、`probe_error` text、`thumbnail_state` text **not null**（`pending` / `done` / `failed`）、`updated_at` integer **not null**（Unix 秒）
  - `playback_progress`（利用者データ）: `content_key` text **主鍵**（`videos` への外部キーは張らない — 動画が消えても残す）、`position_ms` integer not null（`position_ms >= 0` の検査制約）、`duration_ms` integer、`completed` integer not null、`updated_at` integer not null
  - `jobs`: `id` integer 主鍵、`kind` text not null（`probe` / `thumbnail`）、`video_id` integer not null（`videos` 削除時に**連鎖削除**）、`state` text not null（`queued` / `running` / `done` / `failed`）、`attempts` integer not null **既定 0**、`last_error` text、`created_at` / `updated_at` integer not null
  - `scans`: `id` integer 主鍵、`state` text not null（`running` / `done` / `failed`）、`started_at` / `finished_at` integer、`total` / `completed` / `failed` integer not null、`error` text
  - 索引: `videos(added_at desc, id desc)`、`videos(title asc, id asc)`、`videos(content_key)` unique、`jobs(state, id)`、同じ `(kind, video_id)` の未完了ジョブを1件に制限する**部分ユニーク索引**（`where state in ('queued','running')`）、`scans` の `running` を1件に制限する部分ユニーク索引
  - `videos_fts` と同期トリガは 001 のまま変更しない
- [X] T006 `internal/store/migrate_test.go` を拡張し、`00002` 適用後のスキーマを検証する: 追加した列が存在すること、`playback_progress` が `videos` への外部キーを**持たない**こと、部分ユニーク索引が同じ `(kind, video_id)` の2件目の `queued` を拒否すること、`running` な `scans` が同時に1件しか作れないこと、Down で 001 の状態へ戻ること
- [X] T007 [P] `internal/domain/video_test.go` を作成する（実装前に失敗することを確認する）。再生可否の判定を表駆動で検証する（[R-103](./research.md) / S3 / SC-007）: コンテナは `.mp4` `.m4v` `.webm` のみ許可、映像コーデックは `h264` `vp8` `vp9` `av1` のみ許可、音声コーデックは `aac` `mp3` `opus` `vorbis` **または音声なし**のみ許可。3つすべてを満たすときだけ `playable = 1`。満たさない場合に `unplayable_reason` が `container` / `video_codec` / `audio_codec` のどれになるかも検証する。外部プロセスに依存しないこと（`os/exec` を import しない）
- [X] T008 [P] `internal/domain/video.go` を作成する。`Video`（`videos` の行と 1 対 1）、`Probe`（`ffprobe` から取り出した事実: 尺・解像度・映像/音声コーデック）、`ProbeState` / `ThumbnailState` / `UnplayableReason` の定数、および許可リストによる再生可否判定の純粋関数を置く。`net/http`・`database/sql`・`os/exec` を import してはならない（depguard が `.golangci.yml` で強制している）
- [X] T009 [P] `internal/domain/scan.go` を作成する。`ScanResult`（走査1回の集計: 総数・追加・更新・移動・削除・失敗）を置く。`scans` 行の元になる値で、永続化の手段は知らない
- [X] T010 `cmd/mdm/config_test.go` を拡張し、`MDM_SCAN_ON_START` の検証を足す（[contracts/configuration.md](./contracts/configuration.md)）: 未設定なら既定値 `true`、`true` / `false` は受理、それ以外の値は**起動中止**の誤りになること、誤りが他の設定の誤りと**まとめて**列挙されること
- [X] T011 `cmd/mdm/config.go` を更新する。`MDM_SCAN_ON_START`（既定 `true`）を `Config` に足し、`LogAttrs` に含める。`MDM_DATA_DIR/thumbnails` のパスを導出する関数を置き（設定項目にはしない）、`verifyProblems` に「`MDM_DATA_DIR/thumbnails` を作成できること」の確認を追加して、他の確認結果とまとめて列挙する
- [X] T012 `internal/httpapi/router.go` に、応答の共通部品を足す。[contracts/http-routes.md](./contracts/http-routes.md) の「キャッシュ」表どおりに `Cache-Control` を付けるヘルパ（`/api/videos`・`/api/videos/{id}`・`/api/scans*` は `no-store`、ストリームは `private, max-age=0, must-revalidate`、`v` 付きサムネイルは `public, max-age=31536000, immutable`）と、`code`（`not_found` / `invalid_request` / `internal`）と日本語 `message` を返す `writeError` を追加する

**Checkpoint**: スキーマ・設定・判定規則が揃った。ここから各ストーリーを並行して始められる

---

## Phase 3: User Story 1 - 置いた動画が自動で一覧に並ぶ (Priority: P1) 🎯 MVP

**Goal**: 動画を置いた場所を教えるだけで、静止画・題名・長さ付きの一覧が並ぶ。あとから
追加した動画も、手作業の登録なしに取り込みで一覧へ現れる。

**Independent Test**: 動画を何本か置いたフォルダを指定して起動し、一覧を開く。手作業の登録
なしに、置いた本数と同じ件数が静止画・題名・長さ付きで並べば成功。フォルダに1本追加して
取り込みを促すと、追加分だけが増えることも確認する（[quickstart.md](./quickstart.md) S1・S2・S7・S8）。

### Tests for User Story 1 ⚠️

> **NOTE: 先にこれらを書き、実装前に失敗することを確認する**

- [X] T013 [P] [US1] `internal/scanner/content_key_test.go` を作成する（[R-101](./research.md)）。`content_key` = `sha256(先頭 1MiB ‖ 末尾 1MiB) + ":" + ファイルサイズ` の 16 進表現であること、**2MiB 以下のファイルは全体を1度だけ読んで同じ式に当てる**こと、内容が同じならパスが違っても同じ鍵になること、末尾だけが違うファイルで鍵が変わることを、一時ディレクトリ上の固定バイト列で検証する
- [X] T014 [P] [US1] `internal/scanner/scanner_test.go` を作成する（[R-107](./research.md) / S1・S7・S8）。一時ディレクトリを使い、走査規則を検証する: 対象拡張子は `.mp4` `.m4v` `.webm` `.mkv` `.mov` `.avi` `.wmv` `.flv` `.ts` `.mpg` `.mpeg`（再生できない形式も**取り込む**）、除外は `.` 始まりのファイル・ディレクトリ／`@eaDir`／`#recycle`／`lost+found`／`.part` `.crdownload` `.tmp`、パスは保存前に Unicode **NFC** へ正規化、`size_bytes` と `mtime` が変わらない既存行は何もしない（`content_key` を再計算しない）、消えたパスと新しいパスの `content_key` が一致したら**パスの更新**（移動・改名）として扱い重複を作らない、一致しなければ行を削除する
- [X] T015 [P] [US1] `internal/media/probe_test.go` を作成する（[R-102](./research.md)）。`ffprobe` を起動せず、固定の JSON 文字列から `format.duration`・`format.format_name`・`codec_type=video` の先頭の `codec_name`/`width`/`height`・`codec_type=audio` の先頭の `codec_name` を取り出せることを検証する。終了コードが非 0／JSON が解析できない／`duration` が取れない場合に、取り込み全体を止めずその1件を失敗として返すことも検証する
- [X] T016 [P] [US1] `internal/media/thumbnail_test.go` を作成する（[R-104](./research.md)）。抽出位置が「尺の 10%、ただし下限 1 秒・上限 60 秒」に丸められること、出力先が `<MDM_DATA_DIR>/thumbnails/<content_key の先頭2文字>/<content_key>.jpg` になること、組み立てる引数が `-ss` を `-i` の**前**に置き `-frames:v 1 -vf scale=640:-2 -q:v 4` を含むことを検証する（実際の実行は統合テスト側）
- [X] T017 [P] [US1] `internal/store/videos_test.go` を作成する（[R-109](./research.md)）。取り込みの upsert（新規・更新・移動によるパス更新）、一覧のカーソル方式ページング（既定 `limit=60`、上限 200）、並び順 `added_at desc, id desc` と `title asc, id asc`、`total` が絞り込み後の総件数であること、不正なカーソルが誤りとして返ることを検証する
- [X] T018 [P] [US1] `internal/store/jobs_test.go` を作成する（[R-106](./research.md)）。状態遷移 `queued` → `running` → `done`、失敗時は `attempts` を +1 して `queued` に戻し **3 回で `failed`** で止まること、`begin immediate` で1件を専有し二重取得が起きないこと、起動時に `running` の行が `queued` へ戻ること、同じ `(kind, video_id)` の未完了ジョブが1件だけになることを検証する
- [X] T019 [P] [US1] `internal/store/scans_test.go` を作成する。`running` は同時に1件だけであること、`total` / `completed` / `failed` が更新できること、直近のスキャン（実行中があればそれ、無ければ最後に終わったもの）を取得できることを検証する
- [X] T020 [P] [US1] `internal/jobs/worker_test.go` を作成する。ワーカーが**直列（並列度1）**で処理すること、失敗したジョブを再試行し3回で止めること、`context` の取り消しで安全に止まること、起動時に `running` を巻き戻してから処理を始めることを、偽の実行関数で検証する
- [ ] T021 [P] [US1] `internal/httpapi/videos_test.go` を作成する（`net/http/httptest`）。`GET /api/videos` が `items`・`total`・`nextCursor` を返すこと、`limit` の既定 60・上限 200、`sort=addedDesc|titleAsc`、壊れた `cursor` が **`400` + `invalid_request`**（黙って先頭から返さない）になること、`Cache-Control: no-store` が付くこと、`GET /api/videos/{id}` が存在しない id に `404` + `not_found` を返すことを検証する
- [ ] T022 [P] [US1] `internal/httpapi/thumbnail_test.go` を作成する（[R-112](./research.md)）。`GET /api/videos/{id}/thumbnail?v=…` が `image/jpeg` を返すこと、`v` 付きの要求に `Cache-Control: public, max-age=31536000, immutable` が付き、`v` の無い要求には長期キャッシュを付けないこと、未生成なら `404` を返すことを検証する
- [ ] T023 [P] [US1] `internal/httpapi/scans_test.go` を作成する（[R-108](./research.md)）。`POST /api/scans` が `202` とスキャンを返すこと、**実行中なら新しく始めず実行中のものを返す**（`409` にしない）こと、`GET /api/scans/current` が `state`・`total`・`completed`・`failed` を返し、一度もスキャンしていなければ `404` を返すことを検証する

### Implementation for User Story 1

- [X] T024 [P] [US1] `internal/scanner/content_key.go` を実装する（[R-101](./research.md)）。`io.ReaderAt` で先頭・末尾を各 1MiB 読み、`crypto/sha256` で `sha256(先頭 ‖ 末尾) + ":" + サイズ` を作る。2MiB 以下のファイルは全体を1回だけ読む。BLAKE3 は使わない（依存を増やさない）
- [X] T025 [P] [US1] `internal/media/probe.go` を実装する（[R-102](./research.md)）。`ffprobe -v error -print_format json -show_format -show_streams -- <path>` を `exec.CommandContext` で**30 秒**のタイムアウト付きに1回だけ実行し、JSON を `domain.Probe` へ写す。`os/exec` を `internal/media` の外へ漏らさない
- [X] T026 [P] [US1] `internal/media/thumbnail.go` を実装する（[R-104](./research.md)）。`ffmpeg -nostdin -v error -ss <秒> -i <path> -frames:v 1 -vf scale=640:-2 -q:v 4 -y <出力>` を実行し、`<MDM_DATA_DIR>/thumbnails/<先頭2文字>/<content_key>.jpg` へ JPEG を1枚出す。`-ss` は `-i` の前（キーフレーム単位の高速シーク）
- [X] T027 [P] [US1] `internal/store/videos.go` を実装する（[R-109](./research.md)）。取り込みの upsert（`content_key` 一致でパス更新、`path` 一致で属性更新）、`probe` / `thumbnail` の結果反映（`playable = 1` は `probe_state = done` のときだけ取り得る／`duration_ms` は 0 以上で、取得できないものは `null` のままにし 0 で代用しない）、行の削除、カーソル（並び順の値 + id を base64 で包んだ不透明な文字列）による一覧、`total` の取得を置く。SQL は手書きにする（`sqlc` は入れない、[R-115](./research.md)）
- [X] T028 [P] [US1] `internal/store/jobs.go` を実装する（[R-106](./research.md)）。`begin immediate` で1件を `running` にして専有する取得、完了・失敗（`attempts` +1 で `queued` に戻し 3 回で `failed`）、起動時の巻き戻し（`running` → `queued`）、`(kind, video_id)` 単位の投入を置く
- [X] T029 [P] [US1] `internal/store/scans.go` を実装する。スキャンの開始（`running` が既にあればそれを返す）、`total` / `completed` / `failed` の更新、終了（`done` / `failed` と `finished_at`）、直近のスキャンの取得を置く
- [X] T030 [US1] `internal/scanner/scanner.go` を実装する（[R-107](./research.md)、T024・T027・T029 に依存）。`filepath.WalkDir` で `MDM_MEDIA_DIR` 以下を再帰的に走り、対象拡張子・除外規則・NFC 正規化・`size_bytes`/`mtime` による差分判定・移動検出を行って `videos` を実際のファイルに合わせ、`probe` と `thumbnail` のジョブを積む。動画ファイルは**読み取りのみ**で、変更・移動・削除をしてはならない（FR-009）。個別のファイルの失敗で処理全体を中止せず、`scans.failed` と理由を記録して次へ進む（FR-008）
- [X] T031 [US1] `internal/scanner/scanner.go` に、スキャン完了時の孤児サムネイル掃除を足す（[data-model.md](./data-model.md) 2 節、T030 と同じファイルなので順次）。`videos` のどの `content_key` からも参照されなくなった `MDM_DATA_DIR/thumbnails/` 配下の画像を削除する
- [X] T032 [US1] `internal/jobs/worker.go` を実装する（[R-106](./research.md)）。プロセス内の goroutine **1本**で `jobs` を直列に処理し、`probe` は `media.Probe` → `domain` の再生可否判定 → `store` へ反映、`thumbnail` は `media` の1枚抽出 → `thumbnail_state` の更新を行う。起動時に `running` を巻き戻し、`context` の取り消しで停止する
- [ ] T033 [US1] `internal/httpapi/videos.go` を実装する。`GET /api/videos`（`sort`・`cursor`・`limit`）と `GET /api/videos/{id}` を、[contracts/openapi.yaml](./contracts/openapi.yaml) の `VideoPage` / `Video` に合わせて返す。`thumbnailUrl` は `thumbnailState = done` のときだけ入れ、`?v=<content_key の先頭 12 文字>` を付ける。`durationMs`・`width`・`height` は未取得なら省略する
- [ ] T034 [US1] `internal/httpapi/thumbnail.go` を実装する（[R-112](./research.md)）。`MDM_DATA_DIR/thumbnails/` 配下の画像を `http.ServeContent` で返し、`v` 付きの要求にだけ `public, max-age=31536000, immutable` を付ける。未生成は `404`
- [ ] T035 [US1] `internal/httpapi/scans.go` を実装する（[R-108](./research.md)）。`POST /api/scans` は実行中ならそれを返して `202`、無ければ背後で開始して即座に返す。`GET /api/scans/current` は直近の状態を返す。スキャン中でも一覧・再生の経路が通常どおり応答すること（FR-007）を壊さない
- [ ] T036 [US1] `internal/httpapi/router.go` を更新する。`Options` に動画・スキャンの問い合わせ先、`MediaDir`、サムネイルの置き場所を足し、追加した経路を `gen.HandlerWithOptions` 経由で登録する。`/api/*` の未定義経路が JSON の `404` を返し、SPA の `index.html` を返さない性質を維持する
- [ ] T037 [US1] `cmd/mdm/main.go` を更新する。マイグレーション後にジョブワーカーを起動し、`MDM_SCAN_ON_START` が `true` なら起動直後に1回スキャンを背後で実行する。停止時は HTTP の猶予待ちと合わせてワーカーとスキャンの `context` を取り消し、処理中のジョブを `queued` に残して次の起動で再開できる状態で終える
- [ ] T038 [P] [US1] `web/src/api/client.ts` を作成する。`web/src/api/gen/openapi.ts` の生成型を使う薄い `fetch` ラッパと、一覧を1ページずつ読むフックを置く。データ取得ライブラリは入れない（[R-113](./research.md)）
- [ ] T039 [P] [US1] `web/src/components/VideoCard.tsx` を作成する。サムネイル・題名・長さを表示する。サムネイルは `loading="lazy"` と**固定アスペクト比（16:9）の枠**で描き、未生成（`thumbnailState !== "done"`）でも枠だけを出してレイアウトが動かないようにする（[R-114](./research.md) / FR-010）
- [ ] T040 [P] [US1] `web/src/components/ScanStatus.tsx` を作成する。`GET /api/scans/current` の `state`・`total`・`completed`・`failed` を、進行中であることと残りの規模が分かる形で表示し、再取り込み（`POST /api/scans`）を起動できるようにする（FR-006）
- [ ] T041 [US1] `web/src/pages/LibraryPage.tsx` を作成する。`IntersectionObserver` による無限スクロール（初回表示は最初の1ページ 60 件だけを待つ）、並び順の切り替え（追加が新しい順／題名順）、総件数の表示を置く（FR-011〜FR-014 / SC-003）
- [ ] T042 [US1] `web/src/App.tsx` を `react-router` v7 の宣言的モードに置き換え、`/` に `LibraryPage` を割り当てる。001 の稼働状態表示は一覧画面から到達できる位置へ移すか、`ScanStatus` に統合する

**Checkpoint**: 置いた動画が自動で一覧に並ぶ。ここで止めても「フォルダを開かずに手持ちを見渡せる」価値が単独で成立する（**MVP**）

---

## Phase 4: User Story 2 - ブラウザで再生し、続きから見られる (Priority: P2)

**Goal**: 一覧から選んだ動画をブラウザ上で再生でき、任意の位置へ飛べ、途中でやめた動画は
次に開いたときに続きから再生できる。

**Independent Test**: 一覧から動画を選んで再生し、任意の位置へ飛べることを確認する。途中で
画面を閉じ、再度同じ動画を開いて続きから始まることを確認する（[quickstart.md](./quickstart.md) S4・S5）。

### Tests for User Story 2 ⚠️

- [ ] T043 [P] [US2] `internal/domain/progress_test.go` を作成する（[R-111](./research.md) / S5）。完了判定が `position_ms >= duration_ms - 15000` **または** `position_ms / duration_ms >= 0.95` で `completed = 1` になること、再開位置が `completed` のとき、または 5 秒未満のときは先頭に戻ること、`duration_ms` が既知なら `position_ms <= duration_ms` に丸められること、`position_ms >= 0` であることを表駆動で検証する
- [ ] T044 [P] [US2] `internal/store/progress_test.go` を作成する。鍵が `content_key`（`videos.id` ではない）であること、`upsert` で最後の書き込みが残ること、**動画の行を削除しても `playback_progress` が残る**こと（FR-025）、同じ内容のファイルを別パスに置き直しても同じ再生位置が引き継がれることを検証する
- [ ] T045 [P] [US2] `internal/httpapi/stream_test.go` を作成する（[R-105](./research.md) / S4）。`httptest` で先頭・途中・末尾の `Range`（`206` と `Content-Range`・`Accept-Ranges: bytes`）、範囲外の `416`、`If-Range` を確認する。標準実装の再テストではなく、**ハンドラが正しい `io.ReadSeeker` と ModTime を渡していること**の確認と位置づける。あわせて安全性を検証する: `filepath.Clean` 後に `MDM_MEDIA_DIR` + 区切り文字で始まらないパス、`filepath.EvalSymlinks` 後に外へ出るパス、通常ファイルでないものは `403` ではなく **`404`**（存在を漏らさない）。`Content-Type` は拡張子から決まること（`.mp4`/`.m4v` → `video/mp4`、`.webm` → `video/webm`、それ以外 → `application/octet-stream`）、`Cache-Control: private, max-age=0, must-revalidate` が付くことも確認する
- [ ] T046 [P] [US2] `internal/httpapi/progress_test.go` を作成する。`PUT /api/videos/{id}/progress` が `positionMs` を受けて `Progress`（`positionMs`・`completed`・`updatedAt`）を返すこと、**視聴済みの判定はサーバー側で行いクライアントの申告は採らない**こと、本文の `Content-Type` が `application/json` と `text/plain;charset=UTF-8` の**双方**で受理されること（`navigator.sendBeacon` 対応）、負の値や壊れた本文が `400` + `invalid_request`、存在しない id が `404` になることを検証する

### Implementation for User Story 2

- [ ] T047 [P] [US2] `internal/domain/progress.go` を実装する（[R-111](./research.md)）。完了判定（`position_ms >= duration_ms - 15000` または比率 0.95 以上）と再開位置（`completed` のとき、または 5 秒未満なら先頭）を純粋関数として置く。外部 I/O に依存しない
- [ ] T048 [P] [US2] `internal/store/progress.go` を実装する。`content_key` を鍵にした `upsert` と、一覧・詳細へ載せるための取得（複数の `content_key` をまとめて引く経路を含む）を置く。`videos` への外部キーは張らない
- [ ] T049 [US2] `internal/httpapi/stream.go` を実装する（[R-105](./research.md) / [contracts/http-routes.md](./contracts/http-routes.md)）。DB から取得したパスを**そのまま開かず**、(1) `filepath.Clean` 後に `MDM_MEDIA_DIR` + 区切り文字で始まる、(2) `filepath.EvalSymlinks` 後にも (1) が成り立つ、(3) 通常ファイルである、の3つを満たす行だけを開いて `http.ServeContent` に渡す。満たさない行は `404`。再生できない形式（`playable = false`）でも配信自体は行う。転送中の接続断は `info` ではなく `debug` で記録する
- [ ] T050 [US2] `internal/httpapi/progress.go` を実装する。`PUT /api/videos/{id}/progress` を受け、`domain` の判定で `completed` を決めて `store` へ `upsert` し、記録後の状態を返す。頻度制限はサーバー側では行わない
- [ ] T051 [US2] `internal/httpapi/videos.go` を更新し、一覧と詳細の `Video` に `progress`（`positionMs`・`completed`・`updatedAt`）を載せる。再生位置を持たない動画では省略する
- [ ] T052 [US2] `internal/httpapi/router.go` に `GET /api/videos/{id}/stream` と `PUT /api/videos/{id}/progress` を登録し、`Options` に `MediaDir` を使ったパス検証と進捗の保存先を渡す
- [ ] T053 [P] [US2] `web/src/pages/VideoPage.tsx` を作成する。`<video>` で `/api/videos/{id}/stream` を再生し、題名・長さ・解像度などの情報を同じ画面に出す。再生中**5 秒間隔**と一時停止・離脱時（`visibilitychange` で `navigator.sendBeacon`）に進捗を送る。中断位置から再開しつつ、**先頭から見直す選択肢**も出す。再生できない形式は再生を試みる前に理由を示す（FR-020）。再生中に元のファイルが失われた場合は、何が起きたかを伝えてアプリケーション全体は使い続けられるようにする
- [ ] T054 [US2] `web/src/App.tsx` に `/videos/:id` の経路を足し、一覧から選んだ動画を URL で開ける（共有・再読み込みできる）ようにする
- [ ] T055 [US2] `web/src/components/VideoCard.tsx` を更新し、視聴済みと途中まで見た動画を一覧上で区別できるようにする（FR-015）。あわせて `playable = false` の動画に、再生を試みる前に分かる表示と理由（`unplayableReason`）を出す（SC-007）

**Checkpoint**: US1 と US2 がどちらも独立して成立する。一覧で見つけた動画をその場で見られる

---

## Phase 5: User Story 3 - 題名で目当ての1本を探す (Priority: P3)

**Goal**: 数千本に育っても、覚えている語の一部（日本語の途中の語を含む）で候補を絞り込める。

**Independent Test**: 日本語を含む題名の動画を多数取り込んだ状態で、題名の途中の数文字を
入力し、該当する動画だけが残ることを確認する（[quickstart.md](./quickstart.md) S6）。

### Tests for User Story 3 ⚠️

- [ ] T056 [P] [US3] `internal/store/fts_test.go`（001 のもの）を拡張し、検索の2経路を検証する（[R-110](./research.md) / [TD-001](../../docs/exec-plans/tech-debt.md)）。**書記素が3文字以上**なら `videos_fts MATCH`、**1〜2文字**なら同じ FTS5 表への `LIKE '%…%'` に振り分かること、日本語の題名で先頭一致ではない**部分一致**が取れること（FR-022）、1文字の検索語でも取り出せること（FR-023）、検索語が NFC 正規化されること、FTS5 の特殊文字（`"` `*` `:` `^` など）が引用符で包まれて無効化されること、検索時の並び順が一覧と同じ規則（関連度を使わない）であることを検証する
- [ ] T057 [P] [US3] `internal/httpapi/videos_test.go` に検索のケースを足す。`query` を与えると絞り込まれ `total` が**絞り込み後の**件数になること、`query` の `maxLength` が 100 であること、該当が無い場合に `items` が空で `total: 0` になること、`query` とカーソルを併用してもページングが破綻しないことを検証する

### Implementation for User Story 3

- [ ] T058 [US3] `internal/store/videos.go` に検索を実装する（[R-110](./research.md)）。検索語を NFC 正規化し、書記素数で `MATCH` と `LIKE` の経路を選ぶ。FTS5 の特殊文字は引用符で包んで無効化する。並び順は一覧と同じ規則を使い、関連度（bm25）にはしない（2つの経路で並びが変わると利用者から見て不可解になるため）
- [ ] T059 [US3] `internal/httpapi/videos.go` に `query` パラメータを繋ぎ、`VideoPage.total` を絞り込み後の総件数として返す
- [ ] T060 [US3] `web/src/pages/LibraryPage.tsx` に検索欄を足す。入力に応じて一覧を絞り込み、件数を出し、検索語を消すと元の一覧に戻る。該当が1本もない場合は、結果が無いことと**次に取れる操作（検索語を変える）**を明示する（FR-024）
- [ ] T061 [US3] `web/src/api/client.ts` に `query` パラメータを通す経路を足し、入力が連続したときに取りこぼしが起きないようにする（直前の要求を `AbortController` で打ち切る）

**Checkpoint**: 3つのユーザーストーリーがすべて独立して成立する

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 複数のストーリーに跨る後始末と、変更と同じ単位での文書更新（`AGENTS.md`）

- [ ] T062 [P] `internal/store/jobs.go` に完了行の掃除を足す。`done` の行は 7 日で削除する（[data-model.md](./data-model.md) 4 節。取り込み直後に最大 2万行になるため）
- [ ] T063 [P] `ARCHITECTURE.md` の "Not built yet" を更新する。byte-range streaming・scanner・job worker・`ffmpeg`/`ffprobe` アダプタが実装済みになったこと、`internal/scanner` と `internal/jobs` が宣言だけの境界ではなくなったことを反映し、残るのは認証（Phase 3）であることを書く
- [ ] T064 [P] `README.md` を更新する。`MDM_MEDIA_DIR` に動画を置けば一覧に並ぶこと、`MDM_SCAN_ON_START` の存在、外部公開を前提にしない（認証を掛けていない）ことの明記を維持する。"Repository structure" に増えた場所（`web/src/pages`・`web/src/components`・`MDM_DATA_DIR/thumbnails`）を反映する
- [ ] T065 [P] `docs/design-docs/tech-stack-selection.md` の `content_key` の記述（90〜91 行目付近）を、BLAKE3 から**標準ライブラリの SHA-256** に更新する。理由（読む量が1ファイル 2MiB に固定されており、ハッシュ関数の速度が取り込み時間を律速しない）を添える（[R-101](./research.md) / [plan.md](./plan.md) Complexity Tracking）
- [ ] T066 [P] `docs/exec-plans/tech-debt.md` を更新する。TD-001（trigram の2文字問題）に実装済みの対処（`MATCH` と `LIKE` の2経路、実装箇所）を追記し、TD-004（Web の自動テストはビルド検証のみ）の見直しの契機（Phase 1 で画面が増えた時点）に到達したことを記録する
- [ ] T067 [P] `docs/exec-plans/active/002-core-video-library.md` の「進捗」表を更新し、完了後に `docs/exec-plans/completed/002-core-video-library.md` へ移動する（`AGENTS.md`）
- [ ] T068 `make generate-check` を実行し、`api/openapi.yaml` と `internal/httpapi/gen/`・`web/src/api/gen/` に差分が無いことを確認する。差分があれば直すのは生成物ではなく `api/openapi.yaml` 側である
- [ ] T069 `Makefile` の `make check`（`fmt-check` → `lint` → `test` → `generate-check`）を通す。depguard により `internal/domain` が `net/http`・`database/sql`・`os/exec`・他の `internal` パッケージを import していないことも機械的に確認される
- [ ] T070 [quickstart.md](./quickstart.md) の S0〜S8 を実行して受け入れを確認する（検証用の動画は `ffmpeg` で生成し、実データを使わない）
- [ ] T071 [quickstart.md](./quickstart.md) の S9（1,000 本規模: 初回取り込み 15 分以内・一覧1ページ目 2 秒以内・検索 1 秒以内）と S10（`make down && docker volume rm vv_data` 後の復旧）を実行し、計測値を記録する
- [ ] T072 一覧画面と再生画面のスクリーンショットを `docs/screenshots/<YYYYMMDD>-<短い名前>.png` として撮ってコミットし、PR 本文からコミット SHA 付きの raw URL で参照する（[docs/how-to/ui-change-screenshots.md](../../docs/how-to/ui-change-screenshots.md)）。本機能はこの規約が初めて適用される変更である（[plan.md](./plan.md) ゲート G8）

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 依存なし。すぐ始められる
- **Foundational (Phase 2)**: Setup の完了に依存し、**すべてのユーザーストーリーを塞ぐ**
- **User Stories (Phase 3+)**: すべて Foundational の完了に依存する
  - 人手があれば並行して進められる
  - 逐次なら優先度順（P1 → P2 → P3）
- **Polish (Phase 6)**: 実装したいストーリーがすべて終わっていることに依存する

### User Story Dependencies

- **US1 (P1)**: Foundational 完了後に開始できる。他のストーリーに依存しない
- **US2 (P2)**: Foundational 完了後に開始できる。T051（`Video` に `progress` を載せる）と T055（一覧の視聴状態表示）は US1 の成果物に手を入れるが、US1 が無くても `stream`・`progress` の経路は単独で検証できる
- **US3 (P3)**: Foundational 完了後に開始できる。T058〜T059 は US1 の `internal/store/videos.go`・`internal/httpapi/videos.go` を拡張するため、逐次で進める場合は US1 の後に置く

### Within Each User Story

- テストを先に書き、実装前に失敗することを確認する
- ドメインの判定規則 → 保存層 → 外部プロセス → 走査・ジョブ → HTTP → Web の順
- 同じファイルを触るタスクは [P] を付けない（T030 と T031、T033 と T051、T041 と T060 など）

### Parallel Opportunities

- Setup: T002 は T001 と並行できる
- Foundational: T007〜T009（`internal/domain`）は T005〜T006（マイグレーション）と並行できる
- US1: テスト T013〜T023 は全て並行、実装のうち T024〜T029（scanner/media/store の別ファイル）も並行できる
- US2: テスト T043〜T046 は全て並行、T047（domain）と T048（store）も並行できる
- US3: T056 と T057 は別ファイルなので並行できる
- Polish: T062〜T067 は互いに独立している
- Foundational 完了後は、US1／US2／US3 を別々の担当が同時に進められる

---

## Parallel Example: User Story 1

```bash
# User Story 1 のテストをまとめて着手する:
Task: "internal/scanner/content_key_test.go を作成する"
Task: "internal/scanner/scanner_test.go を作成する"
Task: "internal/media/probe_test.go を作成する"
Task: "internal/store/videos_test.go を作成する"
Task: "internal/httpapi/videos_test.go を作成する"

# 失敗を確認したあと、別ファイルの実装をまとめて着手する:
Task: "internal/scanner/content_key.go を実装する"
Task: "internal/media/probe.go を実装する"
Task: "internal/media/thumbnail.go を実装する"
Task: "internal/store/videos.go を実装する"
Task: "internal/store/jobs.go を実装する"
Task: "internal/store/scans.go を実装する"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup（T001〜T004）
2. Phase 2: Foundational（T005〜T012 — **すべてのストーリーを塞ぐ**）
3. Phase 3: User Story 1（T013〜T042）
4. **止めて検証する**: [quickstart.md](./quickstart.md) の S1・S2・S3・S7・S8 を通す
5. ここで公開・提示してよい（置いた動画が一覧で見渡せる）

### Incremental Delivery

1. Setup + Foundational → 土台
2. US1 を足す → S1・S2・S3・S7・S8 で検証 → **MVP**
3. US2 を足す → S4・S5 で検証 → フォルダを開く習慣を置き換えられる
4. US3 を足す → S6 で検証 → 数千本から1本を取り出せる
5. Phase 6 で文書を更新し、S9・S10 で規模と復旧を確認して完了と判定する

### Parallel Team Strategy

複数人で進める場合:

1. Setup + Foundational を全員で終わらせる
2. Foundational 完了後:
   - 担当 A: User Story 1（取り込み・一覧・サムネイル）
   - 担当 B: User Story 2（Range 配信・再生位置・再生画面）
   - 担当 C: User Story 3（検索の2経路・検索欄）
3. `internal/httpapi/videos.go` と `web/src/pages/LibraryPage.tsx` は US1 が先に形を作る。
   US2 の T051・T055 と US3 の T058〜T060 は、その上への追記として調整する

---

## Notes

- [P] のタスク = 別ファイル・依存なし
- [Story] ラベルはタスクとユーザーストーリーの対応を追えるようにするためのもの
- 生成物（`internal/httpapi/gen/`、`web/src/api/gen/`）は手編集しない。直すのは `api/openapi.yaml` 側（`AGENTS.md`）
- 動画ファイルは読み取り専用で扱う。変更・移動・削除・変換を行うコードを書かない（FR-009）
- 実装前にテストが失敗することを確認する
- タスクごと、あるいは論理的なまとまりごとにコミットする
- 各 Checkpoint で止めて、そのストーリー単独で成立しているか検証してよい
