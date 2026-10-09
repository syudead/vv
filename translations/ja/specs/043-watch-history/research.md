---
source: specs/043-watch-history/research.md
sourceHash: f17c514aea771bd694762c9c1d521fff6eff308afee7f9675e0cb5863cc5e2f3
---

# 調査: 視聴履歴の画面 {#research-watch-history-screen}

親 Issue: #792。引き継ぐ決定:

| 項目 | 正本 |
| --- | --- |
| 境界、ユーザーデータと作り直せるインデックス、認証の境界 | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| 再生位置: ユーザーキーをキーにし、`PUT /api/videos/{id}/progress` が書き、視聴済みと再開の規則を持つ | [internal/store/progress.go](../../internal/store/progress.go)、[internal/domain/progress.go](../../internal/domain/progress.go)、[internal/httpapi/progress.go](../../internal/httpapi/progress.go) |
| ユーザーキーと同じパスでの継承 | [030 データモデル、User key](../030-video-versions/data-model.md#user-key)、[Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| 所有者専用の画面とルート | [016 の UI 設計、Guest degradation](../016-single-account-auth/ui-design.md#guest-degradation)、[016 の auth-api、Three access classes](../016-single-account-auth/contracts/auth-api.md#three-access-classes) |
| 不透明なキーセットのカーソル | [024 の scan-api、`GET /api/scans/current/issues`](../024-import-progress/contracts/scan-api.md#get-apiscanscurrentissues) |

このファイルは、この機能が加える決定だけを記録する。

## R-1: 内容の鍵をキーにし、タイトルのスナップショットを持つ `watch_history` テーブル {#r-1-a-watch_history-table-keyed-by-content-key-with-a-title-snapshot}

**決定**: `watch_history` を加える ([data-model.md](data-model.md#migration))。再生ごとに 1 行で、再生したバージョンの `content_key` をキーにし、その時点で表示していたタイトルを持ち、`videos` への外部キーも `playback_progress` へのつながりも持たない。同じパスでの継承は行を新しい内容の鍵に移す。まとめることとまとまりを解くことは行をそのままにする。

| 案 | 判定 |
| --- | --- |
| **独自のテーブル、`content_key`、タイトルのスナップショット** | 採用 |
| `playback_progress` から履歴を導く | 不採用: 内容ごとに 1 行で、保存のたびに上書きされるので、要件 2 と 4 (1 回の視聴に 1 つのエントリ、1 つの動画に複数のエントリ) を満たせない |
| `playback_progress` と同じく、ユーザーキー (まとまりのメンバーには `bundle:<id>`) をキーにする | 不採用: エントリはどのバージョンを再生したかを言えなくなり、まとまりを解いたときにその行をメンバーに返せない。自分の id で開いたメンバーはそのまま再生できる (`GET /api/videos/{id}` はすべてのバージョンを返す) ので、「今の動画を開く」エッジケースはまとまりのキーなしで成り立つ |
| `videos` への外部キー、削除で連鎖 | 不採用: 動画の行は最後の場所がなくなると削除されるが、エッジケースはエントリを残す |

**理由**: エントリは「どの内容を、いつ」に答える。内容の鍵は再スキャンが再現する値なので、戻ってきたファイルはエントリを再び再生できるようにし、タイトルのスナップショットはファイルがない間もエントリを読めるようにする。履歴の行を削除しても他のテーブルには触れない。これが要件 9 である。

## R-2: 1 回の再生に 1 つのエントリ。クライアントが生成する再生 id で識別する {#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id}

**決定**: `PUT /api/videos/{id}/progress` は省略できる `playbackId` を得る ([contracts/screen-api.md](contracts/screen-api.md#playbackid-on-put-apivideosidprogress))。動画のページは、動画への最初の `play` イベントで id を 1 つ作り (`crypto.randomUUID()`)、動画 id が変わるまですべての保存とビーコンでそれを送る。最初の再生の前の保存は id を持たない。サーバーは一意の `playback_id` に対する `insert or ignore` で、位置と同じトランザクションの中でエントリを挿入するので、一時停止、再開、シークは何も延ばさず、何も作らない。id は `VideoPlayer` ではなくページの再生位置を保存するフックにあるので、プレーヤーの再マウント (エラーからの回復、画質の切り替え) でもエントリは保たれる。

| 案 | 判定 |
| --- | --- |
| **既存の保存に載せるクライアントの再生 id** | 採用 |
| 保存の間の時間の空きでサーバー側でまとめる | 不採用: 同じ動画を再生する 2 つのタブが 1 つのエントリにまとまり (エッジケース)、空きより長い一時停止は 1 回の視聴を分けてしまう (受け入れ条件 3) |
| 最初の再生での別の `POST` | 不採用: 再生ごとに往復が 1 回増え、再生中に所有者がエントリを削除した後、クライアントがそれに気づいて再び送る必要がある。すべての保存に id があれば次の保存がエントリを作り直し、それがエッジケースの「新しいエントリ」である |
| `VideoPlayer` のインスタンスごとの id | 不採用: 回復と画質の切り替えはプレーヤーを再マウントするので、1 回の視聴が分かれる |

**理由**: 1 回の視聴がどこで始まりどこで終わるかを知るのはクライアント (ページ) だけであり、サーバーは識別以外の何についてもそれを信用してはならない唯一の側である: 時刻とエントリが何を指すかはサーバーが決める。ページを読み直すと新しいエントリが始まる。Issue は「しばらくしてから」の繰り返しの視聴を別のエントリとしており、読み直しはページが観察できる最も近いものである。

## R-3: エントリの時刻は、その id を持つ最初の保存のサーバー時刻である {#r-3-the-entrys-time-is-the-server-time-of-the-first-save-with-its-id}

**決定**: `played_at` は、再生 id を持つ最初の保存でのサーバーの時計である。クライアントは最初の `play` で即座に保存を送る (今は最初の保存は 5 秒のタイマーか一時停止を待つ) ので、時刻は往復 1 回の範囲で再生の開始である。一覧は `played_at` の降順、次に `id` の降順に並べ、エントリは視聴中もその位置を保つ。

| 案 | 判定 |
| --- | --- |
| **最初の保存のサーバー時刻** | 採用 |
| クライアントが知らせる開始時刻 | 不採用: サーバーが制御しない端末の時計が所有者の履歴を並べることになる |
| 最新の保存の時刻 (最後の活動) | 不採用: 長い視聴は、その間に他の 2 つの視聴があっても先頭に跳ね続ける。「いつ見たか」は始めたときである |

## R-4: 既存の再生の記録はマイグレーションが埋める {#r-4-existing-playback-records-are-backfilled-by-the-migration}

**決定**: マイグレーションは `playback_progress` の行ごとに 1 つのエントリを挿入する。`played_at` = その `updated_at`、再生 id はなし、記録の内容の鍵 (まとまりのキーはまとまりの代表に解決する)、その時点で動画のページが表示するタイトル (表示名の上書き、なければファイルのタイトル。内容がライブラリにないときは空) を持つ。キーが何にも解決しない行は飛ばす。

| 案 | 判定 |
| --- | --- |
| **マイグレーションで埋める** | 採用 |
| エントリがないとき、リクエスト時に `playback_progress` を一覧に読み込む | 不採用: 1 つの一覧に 2 つの出どころができ、そのエントリを削除するには再生位置を削除しなければならず、要件 9 に反する |
| 埋めない | 要件 10 により不採用 |

**理由**: SQL だけでこれらの行を作れ、マイグレーションは起動時にリスナーが開く前に 1 回だけ動き、それ以降の履歴の出どころは 1 つになる。

## R-5: 内容がライブラリを離れたエントリは、動画なしで残る {#r-5-an-entry-whose-content-left-the-library-stays-without-a-video}

**決定**: 一覧は各エントリを、その内容の鍵を持ち、所有者が開ける場所を持つ動画に結合する (`visibleVideoCondition`、動画ごとの条件であり、表示される代表の規則ではない)。そのような動画があるとき、エントリはその `Video` を持ち、画面はそれを開く。ないとき、エントリはスナップショットのタイトルだけを持ち、画面はそれを再生できないと示し、リンクを描かない。代表でないまとまりのメンバーもあるとみなすので、そのエントリはそのバージョンを開く。

| 案 | 判定 |
| --- | --- |
| **エントリを残し、再生できないと示す** | 採用 |
| 内容がなくなったエントリを隠す | エッジケースにより不採用: エントリは残り、内容が戻ると再び使えるようになる |
| 再生したバージョンではなく、まとまりの代表に解決する | 不採用: エントリはどのバージョンを見たかを言い、その動画のページが他のバージョンを挙げる |

## R-6: エントリの削除は単純な `DELETE` である。消えたエントリは `404` を返し、画面は読み直す {#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads}

**決定**: `DELETE /api/watch-history/{id}` は、行があったときは `204` を、なかったときは `404 not_found` を返す。`DELETE /api/watch-history` はすべての行を消し、`204` を返す。どちらもドメインイベントも `/api/events` の通知も発行しない。`404` のとき、画面は表示している一覧が古いので、エラーなしで一覧を先頭から読み直す。それ以外の失敗では行を残し、トーストを出す。

| 案 | 判定 |
| --- | --- |
| **`204`/`404`、`404` で画面が読み直す** | 採用 |
| ない行にも冪等に `204` | 不採用: 画面は自分の一覧が古いことを知れず、エッジケースは一覧を最新にすることを求めている |
| `/api/events` の `watchHistory` 通知 | 不採用: Issue が挙げるタブをまたぐ場面は古い一覧での削除だけであり、それは `404` が覆う。一覧は開いたときに取り直す |

## R-7: 視聴履歴は、既存のゲートの規則に従う所有者専用の画面とルートの 1 つである {#r-7-watch-history-is-one-more-owner-only-screen-and-route-with-the-existing-gate-rules}

**決定**: 画面は `/history` であり、所有者にだけサイドバーの上のグループに挙げ、ゲートの所有者専用のパスに加える (`/history` のゲストは `/login?next=/history` に移る)。3 つのルートは `/api/*` の既定により所有者専用なので、ゲストは他のすべての所有者専用のルートと同じ `401` を受け取る。履歴は外部 API に加えない。

| 案 | 判定 |
| --- | --- |
| **既存の所有者専用のパスとルートの規則** | 採用 |
| 隠した動画と同じく、`/history` のゲストに `404` | 不採用: 画面はすでにゲストを `/settings`、`/tags`、`/duplicates` からログインのページに送っており、受け入れ条件 9 は履歴が表示されないことだけを求めている |
