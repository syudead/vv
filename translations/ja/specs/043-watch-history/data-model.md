---
source: specs/043-watch-history/data-model.md
sourceHash: 25a3d9f53e45ee6adba645f2ed3b884b50211e8f3a45fc3c00c5a283b90930be
---

# データモデル: 視聴履歴の画面 {#data-model-watch-history-screen}

親 Issue: #792。モデルの残りは変わらない。正本:

| 項目 | 出典 |
| --- | --- |
| 既存のテーブルの定義 | [internal/store/migrations/](../../internal/store/migrations/) |
| データの分類 | [running-vv.md、Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery) |
| 再生位置 | [internal/store/progress.go](../../internal/store/progress.go)、[internal/domain/progress.go](../../internal/domain/progress.go) |
| ユーザーキー、継承、まとまり | [030 データモデル、User key](../030-video-versions/data-model.md#user-key)、[Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| 実際のタイトル | [029 データモデル、Values added to `domain`](../029-video-overrides/data-model.md#values-added-to-domain) |

このファイルは、この機能が加えるテーブル、値、読み書きの規則だけを扱う。

## マイグレーション {#migration}

`00034_watch_history.sql` (`main` の最後のマイグレーションは `00033_scan_origin.sql`。実装するときに番号を確かめる) はテーブルを作り、`playback_progress` から埋める ([research.md R-1](research.md#r-1-a-watch_history-table-keyed-by-content-key-with-a-title-snapshot)、[R-4](research.md#r-4-existing-playback-records-are-backfilled-by-the-migration))。

```sql
create table watch_history (
    id          integer primary key autoincrement,
    content_key text    not null check (content_key <> ''),
    -- The client's id for one viewing (R-2). Null for backfilled rows.
    playback_id text    unique,
    -- The title the video page showed when the viewing started.
    title       text    not null,
    -- Unix milliseconds of the first save of this viewing (R-3).
    played_at   integer not null
);
create index watch_history_played_idx on watch_history (played_at desc, id desc);
create index watch_history_content_idx on watch_history (content_key);
```

埋め戻しは `playback_progress` の行ごとに 1 行を挿入する:

| 列 | 値 |
| --- | --- |
| `content_key` | 記録のキー。マイグレーションの時点で動画がそれを持つかどうかによらない。`bundle:<id>` のキーはそのまとまりの `representative_key` になる。飛ばすのは、`video_bundles` の行のない `bundle:<id>` のキーだけである ([R-4](research.md#r-4-existing-playback-records-are-backfilled-by-the-migration)) |
| `title` | マイグレーションの時点でのその内容の鍵に対する `coalesce(video_overrides.display_name, <representative location's title>, '')`。`video_overrides` は内容をキーにするので、取り除かれた動画も表示名を保ち、なければタイトルは空である |
| `played_at` | `playback_progress.updated_at * 1000` (記録は秒単位) |
| `playback_id` | Null |

Down はテーブルを削除する。

## `watch_history` {#watch_history}

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `id` | integer | 不可 | エントリの id。削除の対象であり、順序で同じ時刻を決める |
| `content_key` | text | 不可 | 再生した内容。決して空ではない |
| `playback_id` | text | 可 | [R-2](research.md#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id) の再生 id。一意なので、同じ id での 2 回目の保存は何も加えない |
| `title` | text | 不可 | エントリを書いたときの実際のタイトルのスナップショット。内容がライブラリにないときに表示する |
| `played_at` | integer | 不可 | Unix ミリ秒。エントリの最初の保存でのサーバーの時計 |

**関係**: なし。`videos` にも `playback_progress` にも外部キーはない。

分類: 作り直せないユーザーデータ。ARCHITECTURE.md (「User data survives a rebuild」) と running-vv.md (「Data and recovery」) の一覧に加える。生成ファイルの片付け、`releaseContentIndex`、`rebuildFolderIndex`、メディアフォルダの削除はこれに触れない。

継承とまとまり:

| 経路 | 変更 |
| --- | --- |
| 同じパスでの継承 (`moveUserData`) | 同じトランザクションの中で `update watch_history set content_key = <new> where content_key = <old>`。そこにある他のテーブルと違い、すでに新しいキーにある行は残る: どちらの視聴も履歴である |
| まとめる、代表を変える、外す (`userDataTables`) | 加えない。エントリは再生したバージョンの内容の鍵を保つ ([R-1](research.md#r-1-a-watch_history-table-keyed-by-content-key-with-a-title-snapshot)) |

## `domain` に加える値 {#domain-values-added}

| 値 | 内容 |
| --- | --- |
| `Play` | 再生位置の保存が視聴について伝えるもの: `PlaybackID string`、`ContentKey string`、`Title string`。`ValidatePlaybackID` は 36 文字の RFC 4122 のテキスト形式 (`8-4-4-4-12` の 16 進数) を受け付け、それ以外は何も受け付けない |
| `WatchHistoryEntry` | `ID int64`、`PlayedAt time.Time`、`Title string`、`Video *Video` (内容がライブラリにないときは nil) |
| `WatchHistoryPage` | `Items []WatchHistoryEntry`、`NextCursor string` (続きがないときは空) |
| カーソル | 最後の項目の `played_at` と `id`。スキャンの問題のカーソルと同じく符号化と復号をする。読めないカーソルは `ErrInvalidCursor` である |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 再生 id のない保存はエントリを書かない。id のある保存は id ごとに多くとも 1 つのエントリを書く | `PlaybackStore.SaveProgress` (`insert or ignore`)、`watch_history.playback_id unique` |
| 動画の `content_key` が空の保存はエントリを書かない | `PlaybackStore.SaveProgress`、`check (content_key <> '')` |
| エントリの時刻とタイトルは最初の保存で決まる | `insert or ignore`: その後の保存は何も変えない |
| エントリを削除しても他のどのテーブルの行も変わらない (要件 9) | `PlaybackStore.DeleteWatchHistoryEntry`、`ClearWatchHistory` |
| エントリが動画に解決するのは、その内容の鍵を持つ動画が閲覧者の開ける場所を持つときだけである | `PlaybackStore.ListWatchHistory` (与えられた `domain.Audience` での `visibleVideoCondition`。動画を返すストアの読み取りはすべてそれを受け取る: ARCHITECTURE.md、「Every read knows its viewer」) |

## ストアの操作 (`PlaybackStore`) {#store-operations-playbackstore}

`PlaybackStore` は引き続き共有の SQLite 接続だけを持つ。動画は、すべての役割が使うパッケージ内の列のヘルパー (`videoColumns`、`scanVideo`) を通じて読む。履歴がこの役割に残るのは、エントリが位置のトランザクションの中で書かれ、1 つのテーブルを 2 つの役割の下に置くと 1 つの業務操作が分かれるからである。

| 操作 | 振る舞い |
| --- | --- |
| `SaveProgress(ctx, userKey, progress, play *domain.Play)` | 今と同じ。加えて、`play` が nil でなく `play.ContentKey` が空でないとき、同じトランザクションの中で、今の時刻をミリ秒で `insert or ignore into watch_history (content_key, playback_id, title, played_at)` する |
| `ListWatchHistory(ctx, audience domain.Audience, cursor string, limit int)` | `(played_at desc, id desc)` の順で `cursor` の後のページを `limit` 項目返す。各項目は、あるときは `audience` が開ける `Video` を持つ (`audience` での `visibleVideoCondition`。ハンドラーは境界が分類した audience を渡し、所有者専用のこれらのルートではそれは所有者である)。さらに行があるときは `NextCursor` を設定する |
| `DeleteWatchHistoryEntry(ctx, id int64) (bool, error)` | 行を削除する。なかったときは false |
| `ClearWatchHistory(ctx) error` | すべての行を削除する |

ドメインイベントは発行しない ([research.md R-6](research.md#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads))。

## 変わらないもの {#what-does-not-change}

`playback_progress`、そのキー、`EvaluateProgress`、`ResumePosition`、視聴状態、`lastPlayedAt`、`played*` の並べ替えは `watch_history` から何も読まないので、履歴を削除してもカードの進み具合のバー、その視聴状態、「Last played」での位置はそのままである (要件 9)。保存の `Progress` 応答は変わらない。
