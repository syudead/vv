# Implementation Plan: 代表サムネイルがシーク用サムネイルの生成を待たないようにする

**Branch**: `feature/020-seek-thumbnail-stage` | **Parent Issue**: #388

**Input**: The parent Issue. It is this feature's specification.

## Summary

今は 1 つの `thumbnail` ジョブが代表 JPEG を 1 枚作ってからシーク用サムネイル（全編デコード）を
作るので、スキャンで後ろに並んだ動画の代表 JPEG は前の全動画のシーク用サムネイルを待つ
（要件 1）。これを、シーク用サムネイルを取り込みの独立した段階にすることで解く。

- 新しいジョブの種類 `seek_thumbnail` と、それ専用のワーカー（他の段階と同じく 1 本・直列）を
  足す。`thumbnail` ジョブは代表 JPEG だけを作る（要件 1・3）。
- `videos` に `seek_thumbnail_state`（`pending` / `done` / `failed`）を足し、代表サムネイルと同じ
  形で成功・終端失敗・再試行を記録する。走査と読み取りのやり直しは、足りない方だけを積む
  （要件 2・5、[data-model.md](data-model.md)）。
- シーク用サムネイルのワーカーは、解析が終わり、かつ取り出せる代表サムネイルの仕事が残って
  いない間だけ仕事を取り出す。代表サムネイルが常に先に終わり、重い ffmpeg が同時に走る数は
  今と同じ 2 本（シーク用とホバープレビュー）を超えない（要件 1・3）。
- 生成物の置き場・再利用・削除・一時ファイルの掃除（`internal/artifacts`・`internal/app` の
  内容ごとの錠）は変えない（要件 4）。
- `GET /api/processing` と `/api/events` の `processing` に `seekThumbnail` を足し、処理状況の
  表示を 4 段階にする（要件 6、[contracts/processing-api.md](contracts/processing-api.md)）。

## Technical Context

**Canonical definitions**:

- 段階ごとのワーカー・待ち行列・取り出しの条件・イベントによる起床・生成物の所有と削除:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)（「Intended topology」の `internal/jobs`・
  `internal/artifacts`・domain events の段落、「Intended dependency direction」）
- 今の取り込み: `Ingest.Thumbnail`（[internal/app/ingest.go](../../internal/app/ingest.go)）、
  取り出しの条件 `domain.ClaimConditionFor`（[internal/domain/job.go](../../internal/domain/job.go)）、
  待ち行列と終端失敗の記録（[internal/store/jobs.go](../../internal/store/jobs.go)）、
  結果の反映と読み取りのやり直し（[internal/store/ingest_results.go](../../internal/store/ingest_results.go)）、
  走査が積む仕事（[internal/scanner/scanner.go](../../internal/scanner/scanner.go) の
  `ensurePendingJobs`・`enqueue`）、購読の登録（[cmd/mdm/events.go](../../cmd/mdm/events.go)）
- シーク用サムネイルの状態の導出: `Catalog.SeekThumbnailState`
  （[internal/app/catalog.go](../../internal/app/catalog.go)）と
  `Video.seekThumbnailState`（[api/openapi.yaml](../../api/openapi.yaml)）
- 前例となる移行: [00005_seek_thumbnail_cache.sql](../../internal/store/migrations/00005_seek_thumbnail_cache.sql)
  （既存動画への積み直し）、[00006_hover_preview.sql](../../internal/store/migrations/00006_hover_preview.sql)
  （状態列の追加と `jobs.kind` の CHECK の作り直し）
- 処理状況の表示: [web/src/shell/ProcessingBreakdown.tsx](../../web/src/shell/ProcessingBreakdown.tsx)、
  [specs/012-scan-progress/ui-design.md](../012-scan-progress/ui-design.md)
- 長尺の入力の作り方: [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- 追加する依存は無い。ffmpeg の引数（`internal/media`）も変えない。
- シーク用サムネイルの状態は今 DB に無く、置き場の有無と `thumbnail` ジョブの行から導いている。
  要件 2・5 の「独立に失敗・再試行し、足りない方だけ積む」は、走査（ファイルシステムの生成物を
  見ない `internal/store`・`internal/scanner`）が判断できる形で状態が要る。列で持つ
  （Structural Decisions 2）。
- 既存のデータでは `thumbnail_state = done` がシーク用サムネイルの完成を意味しない
  （代表 JPEG の後でシーク用だけ失敗した動画も `done` のまま）。移行は既存の全動画を
  `seek_thumbnail_state = pending` として扱い、生成物の再利用（置き場があれば書かない）で
  作り直しを避ける（Structural Decisions 5）。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。取り出しの条件は
  `internal/domain` の純粋関数に足し、`internal/store` はそれを SQL へ写す。`internal/app` は
  自分の宣言した interface に操作を足すだけで、adapter を import しない。段階の間の起床は
  `cmd/mdm/events.go` の購読に足す。
- **生成物の所有**（ARCHITECTURE.md「Generated files have one owner」）: 合格。置き場・公開・
  削除は `internal/artifacts` のまま。新しいワーカーは今と同じ `PublishSeekThumbnails` を、
  同じ内容ごとの錠の中で呼ぶ。
- **索引と利用者データの区別**（ARCHITECTURE.md「Two kinds of data」）: 合格。足す列と
  ジョブの種類は作り直せる索引で、利用者データには触れない。
- **API の正本**（ARCHITECTURE.md）: 合格。`Processing` の変更は `api/openapi.yaml` から生成する。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 合格。ARCHITECTURE.md のワーカー・取り出し
  条件・起床の段落は、その挙動を変える単位が直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/020-seek-thumbnail-stage/
├── plan.md                        # This file
│                                  # No spec.md — the parent Issue is the specification
├── data-model.md                  # seek_thumbnail_state 列、seek_thumbnail ジョブ、状態遷移、取り出しの条件、移行
└── contracts/
    └── processing-api.md          # Processing.seekThumbnail、seekThumbnailState と reprobeVideo の意味の変更
```

`research.md` は作らない。決定はどれも既存の仕組み（段階ごとのワーカー、状態列、取り出しの
条件、移行の前例）の当てはめで、調べて確かめる未知の事項が無い。代案と却下の理由は下の
Structural Decisions に置く（P-2）。`quickstart.md` は作らない。確かめ方は既存の検査と実装単位の
受け入れに書いた手順（長尺の入力は `docs/how-to/preview-benchmark.md` のもの）で足りる。

### Source Code

**Affected boundaries**:

- `internal/domain`: ジョブの種類 `JobSeekThumbnail`、`JobKinds`、`ClaimConditionFor` と
  `JobClaimCondition`、`Processing.SeekThumbnail`、`Video` / `IndexedVideo` / `UpsertResult` の
  シーク用サムネイルの状態。`SeekThumbnailState` は保存する値になる。
- `internal/store`: 移行 `00014`、取り出しの条件の SQL、段階ごとの残りの集計、終端失敗の記録、
  状態の反映、置き場を失った `done` の積み直し、読み取りのやり直し、走査の `Needs*` 判定。
  `ThumbnailJobActive` は要らなくなるので消す。
- `internal/scanner`: 足りない仕事に `seek_thumbnail` を足す。
- `internal/app`: `Ingest.Thumbnail` を代表 JPEG だけにし、`Ingest.SeekThumbnails` を足す。
  `Catalog.SeekThumbnailState` を列から導く。`Catalog.RetryProbe` から置き場の有無の引数を外す。
- `cmd/mdm`: ワーカーは `domain.JobKinds` から作られるので追加は無い。`events.go` に
  シーク用ワーカーの起床を足す。
- `api/openapi.yaml`・`internal/httpapi`・`web/src/shell`: `Processing.seekThumbnail` と 4 列の内訳。
- `ARCHITECTURE.md`: ワーカーの段階、取り出しの条件、起床の段落。

**New paths**: `internal/store/migrations/00014_seek_thumbnail_stage.sql`。

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。

## Structural Decisions

1. **シーク用サムネイルを独立したジョブの種類 `seek_thumbnail` にし、専用のワーカー（1 本・直列）
   で処理する**。要件 1 は「どの動画のシーク用サムネイルの生成も待たない」なので、1 本のワーカーが
   `thumbnail` を優先して取り出す案では、走っている最中のシーク用生成（Linux で 84 秒）を
   待つことになり採らない。`thumbnail` ジョブの中でシーク用を goroutine に逃がす案は、ジョブの
   成否の記録・再試行・中断時の巻き戻し（`internal/jobs`）の外で生成が走ることになり採らない。
2. **`videos.seek_thumbnail_state` 列で状態を持つ**（[data-model.md §1](data-model.md#1-videosseek_thumbnail_state)）。
   今の導出（置き場の有無と `thumbnail` ジョブの行）を続ける案は、走査と `internal/store` が
   ファイルシステムを見ないので「足りない方だけ積む」（要件 5）を判断できず、また
   `Catalog.SeekThumbnailState` 自身が「失敗した行が消えると読めない」と注記している弱さを
   残すので採らない。`done` は置き場の有無で裏づけ、置き場が無ければホバープレビューと同じく
   `pending` に戻して 1 回だけ積み直す（[data-model.md §3](data-model.md#3-状態遷移)）。
3. **`seek_thumbnail` の取り出しは、その動画の解析が終わり、かつ取り出せる `thumbnail` の仕事が
   1 件も無いときに限る**（[data-model.md §4](data-model.md#4-取り出しの条件)）。条件を付けず
   `thumbnail` と並走させる案は、要件 1 は満たすが、スキャン中は代表 JPEG・シーク用・ホバー
   プレビューの ffmpeg が 3 本同時に走り、要件 3（負荷を今より増やさない）に反するので採らない。
   解析を待つのは `thumbnail` と同じ理由（解析が失敗した動画の全編デコードを先に走らせない）である。
   条件が変わったときの起床は、`thumbnail` が解析の完了で起きるのと同じ購読（`cmd/mdm/events.go`）
   で行う: `seek_thumbnail` が積まれたとき、解析または代表サムネイルの 1 件の成否が記録された
   とき、動画の行が消えたとき。
4. **完了の記録は代表サムネイルと同じ専有時の同一性（内容鍵・所在・所在の世代）で行う**。
   ホバープレビューのように内容鍵だけで完了を記録する案（`CompletePreviewForContent`）は、
   生成が長いあいだの所在だけの変化を許すための追加の操作だが、シーク用サムネイルは
   `PublishSeekThumbnails` が置き場があれば書かないので、同一性が変わって queued へ戻っても
   やり直しは置き場の確認 1 回で済む。追加の操作を持たない。
5. **移行は既存の全動画を `seek_thumbnail_state = pending` とし、解析が終わり所在のある動画に
   `seek_thumbnail` ジョブを積む**（[data-model.md §5](data-model.md#5-移行)）。
   `thumbnail_state = done` を `seek_thumbnail_state = done` に写す案は、代表 JPEG の後でシーク用
   だけ失敗した動画（`00005` 以降 `done` のまま残る）を完成扱いにするので採らない。移行では
   積まず次の走査に任せる案は、`00006` の前例と違い、走査するまで処理状況に残りが出ないので
   採らない。完成済みの動画のジョブは置き場の確認だけで終わり、作り直さない（要件 5）。

## Implementation Work

### シーク用サムネイルを独立した状態とジョブの種類として保存層に持たせる

**Scope**: [data-model.md](data-model.md) の全部。`internal/domain`（`JobSeekThumbnail`、
`ClaimConditionFor` / `JobClaimCondition` の追加条件、`Processing.SeekThumbnail`、
`Video` / `IndexedVideo` / `UpsertResult` の状態）、移行 `00014_seek_thumbnail_stage.sql`、
`internal/store`（取り出しの条件の SQL、`Processing` の集計、`recordTerminalFailure`、
`SetSeekThumbnailStateForJob`、`RequeueMissingSeekThumbnails`、`RetryProbe` が積む仕事、
`EnsureJob` の列、走査の `Needs*`）、`internal/scanner` の `ensurePendingJobs` / `enqueue`。
この単位では `domain.JobKinds`（ワーカーを作る一覧）に `seek_thumbnail` を足さず、
`RetryProbe` の引数と `ThumbnailJobActive` も残す（`internal/app` はまだ使う）: 積まれたジョブは
次の単位でワーカーが付くまで待ち、`Ingest.Thumbnail` は今のまま両方を作る。この段階の
feature branch で起動しても退行しない。

**Dependencies**: None.

**Acceptance**: `internal/store` のテストが、`seek_thumbnail` の取り出しが解析の完了と
`thumbnail` の残りが無いことの両方を待つこと、`Processing` が `seek_thumbnail` を別に数えること、
終端失敗が `seek_thumbnail_state` だけを `failed` にし `thumbnail_state` を変えないこと、
`RetryProbe` が `failed` のシーク用を `pending` に戻して `seek_thumbnail` を積むこと、
`RequeueMissingSeekThumbnails` が `done` のときだけ 1 回積むこと、`00014` が既存の動画に
`seek_thumbnail` を積み `up` / `down` が通ること（`task migrations-check`）を検査する。走査の
テストが、`seek_thumbnail_state = pending` の既存動画に `seek_thumbnail` だけを積み直すことを
検査する。`task check` が通る。

### 代表サムネイルとシーク用サムネイルを別々のワーカーで生成する

**Scope**: `internal/app` の `Ingest.Thumbnail` を代表 JPEG だけにし、`Ingest.SeekThumbnails`
（専有時の同一性の確認、元動画の確認、内容ごとの錠の中で `PublishSeekThumbnails` と
`SetSeekThumbnailStateForJob`、動画が消えていたときの生成物の削除）と `Handler` の対応を足す。
`Catalog.SeekThumbnailState` を列から導き、`done` で置き場が無ければ `RequeueMissingSeekThumbnails`
で `pending` に戻す（[data-model.md §3](data-model.md#3-状態遷移)）。`RetryProbe` から置き場の
有無の引数を外し（`internal/store`・`internal/app`）、`ThumbnailJobActive` を消す。
`domain.JobKinds` に `JobSeekThumbnail` を足してワーカーを立て、`cmd/mdm/events.go` に
Structural Decisions 3 の起床を足す。`api/openapi.yaml` の `reprobeVideo` と
`Video.seekThumbnailState` の説明を [contracts/processing-api.md §2](contracts/processing-api.md#2-seekthumbnailstate-の意味)
に合わせる（schema は変えない）。ARCHITECTURE.md のワーカー・取り出し条件・起床の段落を直す。

**Dependencies**: `シーク用サムネイルを独立した状態とジョブの種類として保存層に持たせる`。

**Acceptance**: `internal/app` のテストが、`thumbnail` ジョブが `SeekThumbnails` を呼ばないこと、
`seek_thumbnail` ジョブが `PublishSeekThumbnails` の後に `done` を記録し、失敗しても
`thumbnail_state` に触れないこと、生成中に動画が消えたら生成物を消すこと、`done` で置き場の
無い動画を `pending` として返し積み直しを 1 回だけ頼むことを検査する。`cmd/mdm` のテストが、
解析と代表サムネイルの成否と動画の削除でシーク用ワーカーが起きることを検査する。
`task preview` を [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) の
2 時間の入力を 3 本以上置いて起動し直すと、全動画の代表サムネイルが一覧に並んだ時点で
シーク用サムネイルの残りがまだあり（`GET /api/processing`）、その後それぞれの再生画面で
シークプレビューが出る。代表サムネイルとシーク用サムネイルが揃った動画を再スキャンしても
ffmpeg が起動しない（サーバーのログにジョブの失敗が無く、`thumbnails/` の更新時刻が変わらない）。
`task check` と `task check-docs` が通る。

### 処理状況の表示でシーク用サムネイルの残りを代表サムネイルと分けて示す

**Scope**: [contracts/processing-api.md §1](contracts/processing-api.md#1-processingseekthumbnail)
に従い `api/openapi.yaml` の `Processing` に `seekThumbnail` を足して `task generate`、
`internal/httpapi` の `GetProcessing` と `/api/events` の `processing` の写し、
`web/src/shell/ScanProvider.tsx` の `processingRemaining`、`ProcessingBreakdown.tsx` の
4 列（解析・サムネイル・シーク用・プレビュー）とそのテスト。フローティング表示
（`ScanProgressIndicator`）と設定画面（`ScanStatusSection`）の両方に出る。

**Dependencies**: `シーク用サムネイルを独立した状態とジョブの種類として保存層に持たせる`。

**Acceptance**: `GET /api/processing` と `/api/events` の `processing` に `seekThumbnail` が入り、
`internal/httpapi` のテストが `seek_thumbnail` の残りをそこへ写すことを検査する。web のテストが、
`seekThumbnail` だけが残っているとき「準備中」の状態と 4 列の内訳が出ることを検査する。画面が
変わるので、360px・768px・1280px でフローティング表示と設定画面の内訳が 4 列で崩れず、
スクリーンリーダーで「準備の残り」の各段階と件数が読めることを確かめる。`task check` が通る。
