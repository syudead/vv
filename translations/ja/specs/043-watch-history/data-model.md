---
source: specs/043-watch-history/data-model.md
sourceHash: d34ca90b5e9560ab786bb329e20c4d452338dfd6a114a0b8b045805490513b1e
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

このファイルは、この機能が加えるテーブル、値、読み書きの規則だけを扱う。*改訂* と記した節は要件 12 から 18 とともに加えた ([research.md、Revision](research.md#revision-filter-search-date-jump-and-resume-actions))。

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

`00035_watch_history_title_key.sql` (*改訂*。実装するときに番号を確かめる) は [R-10](research.md#r-10-title-search-uses-the-librarys-query-syntax-on-the-title-alone) の検索用の列を加える:

```sql
alter table watch_history add column title_key text;
```

SQL はタイトルを畳み込めない ([013 データモデル、`search_key` rules](../013-library-search/data-model.md#search_key-rules)) ので、マイグレーションは列を null のままにし、起動時に、マイグレーションの直後でリスナーを開く前、`cmd/mdm` がすでに場所とタグの検索キーを更新する箇所で、`PlaybackStore.RefreshWatchHistoryTitleKeys` が null のすべての行を `domain.FoldForMatch(title)` で埋める。Down は列を削除する。

## `watch_history` {#watch_history}

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `id` | integer | 不可 | エントリの id。削除の対象であり、順序で同じ時刻を決める |
| `content_key` | text | 不可 | 再生した内容。決して空ではない |
| `playback_id` | text | 可 | [R-2](research.md#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id) の再生 id。一意なので、同じ id での 2 回目の保存は何も加えない |
| `title` | text | 不可 | エントリを書いたときの実際のタイトルのスナップショット。内容がライブラリにないときに表示する |
| `played_at` | integer | 不可 | Unix ミリ秒。エントリの最初の保存でのサーバーの時計 |
| `title_key` | text | 可 | *改訂。* `FoldForMatch(title)`。動画のないエントリの検索対象。null になるのはマイグレーションと起動時の埋め込みの間だけである |

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
| `WatchHistoryFilter` (*改訂*) | `all`、`inProgress`、`watched`。`WatchHistoryFilter` スキーマの値である。`ParseWatchHistoryFilter` はそれ以外をすべて拒む |
| `WatchHistoryQuery` (*改訂*) | 1 回の一覧または日付の読み取りの条件: `Filter WatchHistoryFilter`、`Search SearchExpr` (`ParseSearchQuery` から)、`Before time.Time` (`date` が与えられないときはゼロ) |
| `WatchHistoryPeriod` (*改訂*) | `date` パラメーター: `YYYY-MM-DD` または `YYYY-MM`。`ParseWatchHistoryPeriod` はこの 2 つの形を受け付け、それ以外は何も受け付けない。`End(loc)` は `loc` でのその日または月の後の最初の時点である |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 再生 id のない保存はエントリを書かない。id のある保存は id ごとに多くとも 1 つのエントリを書く | `PlaybackStore.SaveProgress` (`insert or ignore`)、`watch_history.playback_id unique` |
| 動画の `content_key` が空の保存はエントリを書かない | `PlaybackStore.SaveProgress`、`check (content_key <> '')` |
| エントリの時刻とタイトルは最初の保存で決まる | `insert or ignore`: その後の保存は何も変えない |
| エントリを削除しても他のどのテーブルの行も変わらない (要件 9) | `PlaybackStore.DeleteWatchHistoryEntry`、`ClearWatchHistory` |
| エントリが動画に解決するのは、その内容の鍵を持つ動画が閲覧者の開ける場所を持つときだけである | `PlaybackStore.ListWatchHistory` (与えられた `domain.Audience` での `visibleVideoCondition`。動画を返すストアの読み取りはすべてそれを受け取る: ARCHITECTURE.md、「Every read knows its viewer」) |
| *改訂。* `inProgress` と `watched` は、`userKeyExpr` で結合した `playback_progress` の行が `watchCondition` を満たす動画を持つエントリだけを残す。`all` はすべてのエントリを残す ([R-9](research.md#r-9-the-state-filter-reads-the-videos-current-watch-state-with-the-librarys-rule)) | `PlaybackStore.ListWatchHistory`、`ListWatchHistoryDays` |
| *改訂。* 検索語が動画を持つエントリに一致するのは、その登録された場所のいずれかの `search_key` のタイトルの行か表示名の行が、畳み込んだ語を含むときである。動画のないエントリに一致するのは、`title_key` がそれを含むときである。AND、OR、NOT はライブラリと同じである ([R-10](research.md#r-10-title-search-uses-the-librarys-query-syntax-on-the-title-alone)) | `PlaybackStore.ListWatchHistory`、`ListWatchHistoryDays` |
| *改訂。* `date` は、`played_at` が要求のタイムゾーンでの `WatchHistoryPeriod.End` より前のエントリを残す ([R-11](research.md#r-11-the-date-list-and-the-jump-are-computed-on-the-server-in-the-viewers-time-zone)) | `PlaybackStore.ListWatchHistory` |
| *改訂。* `title_key` はエントリとともに書かれ、決して更新されない。起動時の埋め込みは null の行だけに触れる | `PlaybackStore.SaveProgress`、`RefreshWatchHistoryTitleKeys` |

## ストアの操作 (`PlaybackStore`) {#store-operations-playbackstore}

`PlaybackStore` は引き続き共有の SQLite 接続だけを持つ。動画は、すべての役割が使うパッケージ内の列のヘルパー (`videoColumns`、`scanVideo`) を通じて読む。履歴がこの役割に残るのは、エントリが位置のトランザクションの中で書かれ、1 つのテーブルを 2 つの役割の下に置くと 1 つの業務操作が分かれるからである。

| 操作 | 振る舞い |
| --- | --- |
| `SaveProgress(ctx, userKey, progress, play *domain.Play)` | 今と同じ。加えて、`play` が nil でなく `play.ContentKey` が空でないとき、同じトランザクションの中で、今の時刻のミリ秒と `FoldForMatch(play.Title)` で `insert or ignore into watch_history (content_key, playback_id, title, title_key, played_at)` する |
| `ListWatchHistory(ctx, audience domain.Audience, query domain.WatchHistoryQuery, cursor string, limit int)` | `query` を満たすエントリの中で、`(played_at desc, id desc)` の順で `cursor` の後のページを `limit` 項目返す (*改訂*: 絞り込み、検索、`Before` は、件数の上限より前に同じ SQL 文の中で適用し、動画は `visibleVideoCondition` の下で内容の鍵により、その進み具合は `userKeyExpr` により結合する)。各項目は、あるときは `audience` が開ける `Video` を持つ (ハンドラーは境界が分類した audience を渡し、所有者専用のこれらのルートではそれは所有者である)。さらに行があるときは `NextCursor` を設定する |
| `ListWatchHistoryDays(ctx, audience domain.Audience, query domain.WatchHistoryQuery, loc *time.Location) ([]string, error)` (*改訂*) | `query` を満たすエントリ (その `Before` は無視する) の `loc` での重複のない暦日を、`YYYY-MM-DD` として新しい順に返す。一致する行の `played_at` を読み、Go でまとめる ([R-11](research.md#r-11-the-date-list-and-the-jump-are-computed-on-the-server-in-the-viewers-time-zone)) |
| `RefreshWatchHistoryTitleKeys(ctx) (int, error)` (*改訂*) | null であるすべての行の `title_key` を埋め、その数を返す。起動時に `cmd/mdm` が呼ぶ |
| `DeleteWatchHistoryEntry(ctx, id int64) (bool, error)` | 行を削除する。なかったときは false |
| `ClearWatchHistory(ctx) error` | すべての行を削除する |

ドメインイベントは発行しない ([research.md R-6](research.md#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads))。

## 変わらないもの {#what-does-not-change}

`playback_progress`、そのキー、`EvaluateProgress`、`ResumePosition`、視聴状態、`lastPlayedAt`、`played*` の並べ替えは `watch_history` から何も読まないので、履歴を削除してもカードの進み具合のバー、その視聴状態、「Last played」での位置はそのままである (要件 9)。保存の `Progress` 応答は変わらない。改訂はエントリに視聴状態も位置も保存しない: 絞り込みと位置のバーは、要求の時点での動画の今の `playback_progress` の行を読む ([R-9](research.md#r-9-the-state-filter-reads-the-videos-current-watch-state-with-the-librarys-rule)、[R-13](research.md#r-13-the-resume-and-restart-actions-open-the-video-page-with-autoplay-and-the-existing-resume-rule-decides-the-position))。
