# Data model: 動画の更新日時とファイルの作成日時

親 Issue: #630。既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が
正本で、データの区分は [ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」、利用者
データの鍵は [specs/030-video-versions/data-model.md §3](../030-video-versions/data-model.md#3-利用者データの鍵)
にある。ここには、この feature が足す表と列、値、読み書きの規則だけを書く。書いていない表は変えない。

## 1. マイグレーション

`00027_video_dates.sql` として足す（`main` の最後は `00026_video_version_candidates.sql`）。

```sql
-- vv 上で動画の情報を最後に編集した日時（specs/033-video-dates/research.md R-1）。
-- 作り直せない利用者データで、video_overrides と同じく内容の識別子に結び、videos への外部キーを
-- 張らない。行が無い動画の更新日時は videos.added_at である。
create table video_edits (
    content_key text    primary key,
    edited_at   integer not null
) without rowid;

-- 一覧に出す所在のファイルの作成日時（Unix 秒）。ファイルシステムが持たなければ null
-- （research.md R-4）。
alter table video_locations add column file_created_at integer;
```

Down は列と表を落とす。

不変条件（`internal/store/invariants_test.go` に足す）: `video_edits` に空の `content_key` の行は無い。

区分: `video_edits` は作り直せない利用者データで、ARCHITECTURE.md の一覧に足す。
`video_locations.file_created_at` は所在の事実で、作り直せる索引の側（`video_locations` の一部）。
生成物の片付け（`RemoveContent`）と `releaseContentIndex` は `video_edits` に触れない。

## 2. `domain` に足す値

| 値 | 中身 |
| --- | --- |
| `Video.EditedAt` | `time.Time`。vv 上の更新日時。保存層が `coalesce(video_edits.edited_at, videos.added_at)` で埋める（[R-7](research.md#r-7-応答の項目は-updatedatvv-上の更新日時と-filecreatedat所在の作成日時で画面と外部連携で同じ名前にする)）。`Video.UpdatedAt`（`videos.updated_at`）は今のまま、応答には出さない |
| `Video.FileCreatedAt` | `time.Time`。一覧に出す所在の `coalesce(file_created_at, mtime)` |
| `VideoFile.FileCreatedAt` | `time.Time`。走査が読んだ作成日時。ゼロ値は取れなかった |
| `IndexedVideo.FileCreatedAt` | `time.Time`。索引に入っている値。ゼロ値は null |
| `VideoLocation.FileCreatedAt` | `time.Time`。ゼロ値は null（`VideoLocations` の読み出し） |
| `SortCreatedAsc` / `SortCreatedDesc` | `VideoSort` の `createdAsc` / `createdDesc`。`Valid` に足す |
| `ExternalVideo` | 変えない。`Video` が増えた項目を運ぶ |

`domain` は作成日時の読み方（OS ごとの `stat`）を知らない。

## 3. 更新日時を進める規則

パッケージ内の 1 つの関数 `touchEditedAt(ctx, tx, contentKeys []string, now time.Time) error` が
`insert into video_edits (content_key, edited_at) values (?, ?) on conflict (content_key) do update set
edited_at = excluded.edited_at` を、空でない内容の識別子にだけ書く。役割の型は自分の取引の中でこれを呼び、
他の役割の公開メソッドは呼ばない。

| 操作 | 進める内容の識別子（[R-2](research.md#r-2-更新日時を進めるのは動画の情報を書く-4-種の操作だけでタグ自体集まり取り込みでは進めない)・[R-3](research.md#r-3-変わらなかった編集は進めず進める対象は書き込みが実際に変えた内容の識別子だけにする)） |
| --- | --- |
| `OverrideStore.SetDisplayName` / `SetDisplayNames` | 書く前の `display_name` と整えた名前が違う内容の識別子 |
| `OverrideStore.SetThumbnailPosition` | 書く前の `thumbnail_position_ms` と指定（nil = 解除）が違う内容の識別子 |
| `VisibilityStore.SetVideosPublic` | `insert or ignore` / `delete` が行を変えた利用者データの鍵を `contentKeysForUserKeys` で広げたもの |
| `TagStore.AttachTagByID` / `AttachTagByName` / `DetachTag` | 同上（`attachTagToVideoIDs` / `detachTagFromVideoIDs` が変えた鍵） |
| `TagStore.ApplyVideoTags`（`applyManualTags`） | `delete … returning content_key` と `insert or ignore … returning content_key` が返した鍵を広げたもの |

`now` は操作の取引を始めた時刻（同じ取引の対象はすべて同じ値）。それ以外の書き込みは `video_edits` に
触れない（R-2）。

同じパスの中身の引き継ぎ（`moveUserData`、[specs/030-video-versions/data-model.md §5](../030-video-versions/data-model.md#5-同じパスの中身の引き継ぎ)）
は、表の一覧に `video_edits` を足し、`playback_progress`・`public_videos` と同じ規則（引き継ぎ先の行を
消してから付け替える）で動かす。集まりを作る・外す（`VersionStore`）は内容の識別子に結ぶ値に触れないので、
`video_edits` も触れない。

## 4. 読み出し

動画を返す読み出し（`videoColumnsTemplate`・`listColumns`・外部連携の `readVideosByIDs` は
`videoColumns` を使う）に 2 列を足し、`scanVideo` に写す。

| 列 | 式 |
| --- | --- |
| `edited_at` | `coalesce((select e.edited_at from video_edits e where e.content_key = videos.content_key and videos.content_key <> ''), videos.added_at)` |
| `file_created_at` | 代表の所在の `coalesce(l.file_created_at, l.mtime)`（`videoColumnsTemplate` は `mtime` と同じ副問い合わせ、`listColumns` は `coalesce(loc.file_created_at, loc.mtime)`） |

並べ替え（[specs/013-library-search/contracts/list-api.md §3](../013-library-search/contracts/list-api.md#3-videosort-の値)
の表と同じ形。その文書は変えず、ここが差分を持つ）:

| 並べ替え | 昇順 | 降順 | 使う値 |
| --- | --- | --- | --- |
| 作成日 | `createdAsc` | `createdDesc` | 一覧に出す所在の `coalesce(file_created_at, mtime)` |

`listOrders` は `coalesce(loc.file_created_at, loc.mtime)`、ライブラリの項目（`libraryItemsCTE`）は
`items` に `created_at` 列を足し、動画の項目はその所在の値、グループの項目はメンバーの代表の所在の値の
`max`（`mv` に `representativeLocationValue` の `coalesce` 版で持つ）。値が同じなら `id` で決着させる
（受け入れ条件 6）。`modifiedAsc`・`modifiedDesc` の式は変えない（受け入れ条件 8）。

ゲストの一覧は `createdAsc`・`createdDesc` を許す（所有者のデータに依らない）。`guest-api.md` の
`400` の表は変わらない。

## 5. 走査と所在の書き込み

`internal/scanner` は対象ごとに `fileCreatedAt(path, info)`（[R-5](research.md#r-5-作成日時はファイルシステムのアダプタ-internalscanner-が-os-ごとに読みlinux-は-golangorgxsysunix-の-statx-を使う)）を
読む。

| 経路 | 書くこと |
| --- | --- |
| `ScanIndexStore.UpsertVideo`（中身が変わった・新しいパス） | 所在の `insert` / `update` に `file_created_at`（ゼロ値なら null）を含める |
| `ScanIndexStore.UpdateLocationCreatedAt(ctx, locationID int64, createdAt time.Time) error`（新設。`scanner.Index` に足す） | `update video_locations set file_created_at = ? where id = ?`。`updated_at`・`version` は動かさず、イベントも発行しない |
| `ScanIndexStore.IndexedVideosByPath` | `IndexedVideo.FileCreatedAt` を載せる |

走査は変わっていないファイル（大きさと mtime が同じ）について、読めた作成日時と `IndexedVideo.FileCreatedAt`
が違うときだけ `UpdateLocationCreatedAt` を呼ぶ（[R-6](research.md#r-6-登録済みの所在は変わっていないファイルでも作成日時が違えば次の走査で書き直す)）。
失敗は `ensurePendingJobs` の失敗と同じく `register_failed` として報告する。読めなかった作成日時は失敗に
しない。
