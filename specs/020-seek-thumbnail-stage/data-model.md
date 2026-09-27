# Data model: シーク用サムネイルの段階

親 Issue #388 の Plan（[plan.md](plan.md)）の一部。既存の索引（`videos`・`jobs`）と生成物の
置き場は [ARCHITECTURE.md](../../ARCHITECTURE.md) と
[internal/artifacts/store.go](../../internal/artifacts/store.go) のままで、ここには足す列と
ジョブの種類、その状態遷移、取り出しの条件、移行だけを書く。

## 1. `videos.seek_thumbnail_state`

| 列 | 型 | 値 | 意味 |
| --- | --- | --- | --- |
| `seek_thumbnail_state` | `text not null default 'pending'` | `pending` / `done` / `failed` | シーク用サムネイルの生成の状態。`thumbnail_state`・`preview_state` と同じ形 |

`domain.Video`・`domain.IndexedVideo` に `SeekThumbnailState`（既存の型
`domain.SeekThumbnailState`。値は保存する値になる）、`domain.UpsertResult` に
`NeedsSeekThumbnail`（`seek_thumbnail_state <> 'done'`）を足す。`thumbnail_state` の意味は
代表 JPEG だけになる。

作り直せる索引である（ARCHITECTURE.md「Two kinds of data」）。

## 2. `jobs.kind = 'seek_thumbnail'`

`domain.JobSeekThumbnail = "seek_thumbnail"`。`jobs.kind` の CHECK に加える（SQLite は CHECK を
その場で変えられないので、`00006` と同じく表を作り直す）。部分ユニーク索引
`jobs_pending_kind_video_idx`（未完了は `(kind, video_id)` に 1 行）はそのまま効く。

`domain.JobKinds` の順は `probe, thumbnail, seek_thumbnail, preview`。ワーカーは段階ごとに 1 本で、
`seek_thumbnail` も 1 本・直列である。

`domain.Processing` に `SeekThumbnail` を足し、`Remaining()` に含める。`IngestStore.Processing`
は既存の `group by kind` に 1 つ case を足すだけである。

## 3. 状態遷移

| 起点 | 操作 | `seek_thumbnail_state` | 積む仕事 |
| --- | --- | --- | --- |
| 新しい内容の取り込み（`UpsertVideo`） | 行の挿入 | `pending`（既定値） | `probe`・`thumbnail`・`seek_thumbnail`（`NeedsSeekThumbnail`） |
| 既存の動画の再走査（`ensurePendingJobs`） | `pending` なら | 変えない | `seek_thumbnail`（`EnsureJob`。`failed` は積み直さない） |
| `seek_thumbnail` ジョブの成功 | `SetSeekThumbnailStateForJob(done)` | `done`（専有時の内容鍵・所在・所在の世代が今も同じときだけ） | なし |
| `seek_thumbnail` ジョブの終端失敗 | `recordTerminalFailure` | `failed`（`<> 'done'` かつ同一性が同じときだけ） | なし |
| `thumbnail` ジョブの終端失敗 | `recordTerminalFailure` | 変えない | なし |
| 読み取りのやり直し（`RetryProbe`） | `failed` なら | `pending` | `probe`、`thumbnail`（`thumbnail_state` を戻したとき）、`seek_thumbnail`（戻したとき） |
| `done` なのに置き場が無い（`Catalog.SeekThumbnailState`） | `RequeueMissingSeekThumbnails(id, contentKey)` | `pending`（`done` かつ内容鍵が同じときだけ、1 つの取引で） | `seek_thumbnail` |
| 動画の行の削除 | 連鎖 | 行ごと消える | `ContentUnreferenced` で生成物を消す（既存） |

`Catalog.SeekThumbnailState` の導出: 置き場があれば `done`。無ければ列が `done` のときは
`RequeueMissingSeekThumbnails` を頼んで `pending`（積めなくても応答は `pending`。ホバー
プレビューの `RequeueMissingPreview` と同じ）、そうでなければ列の値。`ThumbnailJobActive` は
使わないので消す。

`RetryProbe` は置き場の有無の引数を持たなくなる。`done` で置き場が無い動画は、再生画面を
開いた `GET /api/videos/{id}` が上の行で積み直す。

`Ingest.SeekThumbnails`（新しいハンドラ）は `Ingest.Thumbnail` と同じ順で進む: `GetVideo`、
`JobIdentityCurrent`、`CheckSource`、内容ごとの錠の中で `PublishSeekThumbnails`（置き場があれば
書かない）→ `SetSeekThumbnailStateForJob(done)`、反映されなければ（生成中に動画が消えた）
`removeIfUnreferencedLocked`、最後に `removeIfUnreferencedLocked`。`Ingest.Thumbnail` からは
`PublishSeekThumbnails` の呼び出しが無くなる。

## 4. 取り出しの条件

`domain.ClaimConditionFor` を次にする。`internal/store` の `claimConditionSQL` は各条件を SQL へ
写し、`Allows` と同じ判断になるように書く。

| 種類 | 登録済みの所在 | 解析が終わっている | 取り出せる `thumbnail` の仕事が無い |
| --- | --- | --- | --- |
| `probe` | 要る | — | — |
| `thumbnail` | 要る | 要る | — |
| `seek_thumbnail` | 要る | 要る | 要る |
| `preview` | 要る | — | — |

「取り出せる `thumbnail` の仕事が無い」は、`state in ('queued', 'running')` かつ登録済みの所在が
ある `thumbnail` の行が 1 件も無いこと（`IngestStore.Processing` が数える範囲と同じ）。
解析待ちで取り出せない `thumbnail` も数えるので、走査の直後は解析→代表サムネイルが全部終わる
まで `seek_thumbnail` は始まらない。これが要件 1（代表が先）と要件 3（スキャン中に代表 JPEG の
流れとシーク用の全編デコードを競わせない）を満たす。

この条件は取り出しの時点だけで判断し、走っている `seek_thumbnail` を止めない。取り出した後に
新しい `thumbnail` が積まれれば（走査やフォルダの変更）、その代表 JPEG は走っている 1 件の
シーク用と並んで走る（要件 1 は待つことを許さない）。並ぶのはその 1 件が終わるまでで、次の
`seek_thumbnail` は `thumbnail` の残りが無くなるまで取り出されない。全編を読む ffmpeg
（シーク用・ホバープレビュー）が同時に 2 本を超えることは無く、重なるのは入力側シークで
1 枚だけ取る代表 JPEG（1 本 0.1〜0.3 秒）で、今も解析の `ffprobe` が同じ形で重なっている。
実行時の制限を足さない理由は plan.md の Structural Decisions 3。

起床（`cmd/mdm/events.go`）: `seek_thumbnail` のワーカーは、`JobsQueued` にその種類があるとき
（既存の一般則）に加え、`VideoIngestChanged` の `Stage` が `probe`・`thumbnail`・空（動画の行が
消えた）のときに起きる。メディアフォルダの変更は既存どおり `JobsQueued` を全種類で発行する。

## 5. 移行

`internal/store/migrations/00014_seek_thumbnail_stage.sql`:

- Up: `videos` に `seek_thumbnail_state` を足す（既定 `pending`、CHECK）。`jobs` を `kind` の
  CHECK に `seek_thumbnail` を含めて作り直す（`00006` と同じ手順。既存の行と索引を保つ）。
  `probe_state <> 'pending'` で所在が 1 つ以上ある動画に `seek_thumbnail` を `queued` で積む
  （`00006` のプレビューの積み直しと同じ形）。`thumbnail` の行は消さない。
- Down: `seek_thumbnail` の行を消し、`jobs` を元の CHECK で作り直し、列を落とす。

既存の動画は `thumbnail_state` に関わらず全部 `pending` になる。完成済みの動画の
`seek_thumbnail` ジョブは `PublishSeekThumbnails` が置き場を見て書かずに終わるので、作り直しは
起きない（要件 5）。代表 JPEG の後でシーク用だけ失敗していた動画（`thumbnail_state = done` の
まま）は、ここで初めて独立に再試行される（親 Issue Edge Cases）。

移行の時点で `queued` / `running` の `thumbnail` ジョブは、新しい版では代表 JPEG だけを作る。
その動画のシーク用は移行が積んだ `seek_thumbnail` が作る。
