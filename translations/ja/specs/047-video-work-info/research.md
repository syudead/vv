---
source: specs/047-video-work-info/research.md
sourceHash: 0f5189cebb84e4c0d86c87ea66d8d166c76c5b1fbcfdec78d42cd0b6aeb71543
---

# 調査: 動画の作品情報 {#research-work-information-on-videos}

親 Issue: #967。引き継ぐ決定:

| 話題 | 正本 |
| --- | --- |
| 境界、依存の方向、索引とユーザーデータ、コミット後のイベント | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| ユーザーキー (まとまりのメンバーなら `bundle:<id>`、それ以外は `content_key`)、まとめるときのコピーと復元、同じパスでの継承 | [specs/030-video-versions/data-model.md、ユーザーキー](../030-video-versions/data-model.md#user-key)、[ストアの操作 (`VersionStore`)](../030-video-versions/data-model.md#store-operations-versionstore)、[同じパスでの内容の引き継ぎ](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| 表示名のルール (制御文字、トリム、空で消去、コードポイントの上限) | [specs/029-video-overrides/research.md R-10](../029-video-overrides/research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit) |
| 検索で照合する形 (`FoldForMatch`) と、タグ名が検索ボックスに加わる方法 | [specs/013-library-search/data-model.md、`search_key` のルール](../013-library-search/data-model.md#search_key-rules)、[specs/014-video-tags/data-model.md、検索ボックスでのタグ名の照合](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box) |
| 一覧の API とライブラリの URL のタグの絞り込み | [specs/014-video-tags/contracts/tags-api.md、一覧の絞り込みとすべて選択](../014-video-tags/contracts/tags-api.md#list-filter-and-select-all)、[list-url.md](../014-video-tags/contracts/list-url.md) |
| ゲストに見えるものと、ゲストが使えない条件 | [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| 編集時刻 (`updatedAt`) | [specs/033-video-dates/research.md](../033-video-dates/research.md) |
| 外部 API の慣習、まとめての操作、MCP のツール | [specs/026-external-api/contracts/external-api.md、共通のルール](../026-external-api/contracts/external-api.md#common-rules)、[specs/029-video-overrides/research.md R-9](../029-video-overrides/research.md#r-9-the-external-api-adds-two-bulk-operations-and-mcp-the-same-two-tools) |

このファイルは、この機能が加える決定だけを記録する。

## R-1: 6 つの固定のフィールド。出演者とメーカーは一覧を持つ {#r-1-six-fixed-fields-performers-and-the-maker-get-lists}

**決定**: 作品情報は `code`、`releaseDate`、`maker`、`performers` (名前の順序つきの一覧)、`description`、`sourceUrl` の固定の組である ([data-model.md、フィールド](data-model.md#fields))。同じ値を持つ動画の一覧 (要件 5) は `performers` と `maker` にだけある。

| フィールド | 値ごとの一覧 | 理由 |
| --- | --- | --- |
| `performers` | あり | 名前は作品をまたいで繰り返し現れる。「この出演者のすべての動画」が要件の挙げる一覧である |
| `maker` | あり | 同じ。1 つのメーカーに多くの作品がある |
| `code` | なし | 1 つの品番は 1 つの作品である。同じ品番のファイルはそのバージョンで、まとまりに属する ([030](../030-video-versions/plan.md)) |
| `releaseDate` | なし | 同じ日付は偶然に繰り返し現れる。日付は範囲や並べ替えであり、それは別の仕組みで、求められていない |
| `description`、`sourceUrl` | なし | 自由な文章とリンクである。どちらも動画をまとめない |

| 案 | 判定 |
| --- | --- |
| **6 つの固定のフィールド、うち 2 つが一覧を持つ** | 採用 |
| 所有者が定義するフィールド | 不採用: 親 Issue の `対象外` |
| 1 つの自由な文章のブロック | 不採用: 要件 4 と 5 は、品番と名前を照合でき一覧にできる値として必要とする |

**理由**: 親 Issue はこの 6 つを例として挙げ、選択を計画に委ねている。これらはスクレイパー (#968) が公開された作品について埋められるフィールドであり、それぞれが異なるルール (日付、URL、一覧) を持つので、項目ごとに型のある 1 つのフィールドにすると、検証と表示をフィールドごとに保てる。

## R-2: ユーザーキーをキーにする 2 つのユーザーデータのテーブル。まとめる経路と継承の経路がコピーする {#r-2-two-user-data-tables-keyed-by-the-user-key-copied-by-the-bundle-and-succession-paths}

**決定**: `video_work_info` (キーごとに 1 行、5 つの単一値のフィールド) と `video_work_performers` (名前と位置ごとに 1 行) は、ユーザーキーをキーにするユーザーデータのテーブルで、キーは `video_tags` や `video_favorites` と同じである ([data-model.md、マイグレーション](data-model.md#migration))。両方が `userDataTables` (まとめると代表の行をまとまりのキーにコピーし、外されたメンバーは自身の行を取り戻す) と `moveUserData` (同じパスでの継承。`video_work_info` のキーを付け替え、出演者は `on update cascade` に運ばせる) に加わる。どちらのテーブルも `videos` への外部キーを持たない。

| 案 | 判定 |
| --- | --- |
| **タグやお気に入りと同じくユーザーキー** | 採用 |
| `video_overrides` と同じく内容の鍵 | 不採用: Edge Case はまとまりごとに 1 つの単位を求める。情報は作品を説明し、作品はすべてのバージョンが共有する。内容ごとの行は、まとめるときにすべてのメンバーへコピーしなければならなくなる |
| オブジェクト全体を持つ JSON の列 | 不採用: 出演者の一覧 (R-4) と検索の条件 (R-3) は 1 行につき 1 つの名前を照合する。すべての候補の行に `json_each` を使うと、一覧のページごとに動画ごとに 1 回解析が走る |
| `videos` の列 | 不採用: 索引はスキャンで再構築される (要件 6) |

**理由**: ユーザーキーをキーにすると、要件 6 と 3 つの Edge Case (同じ内容が複数の場所にある、まとまり、移動と名前の変更) が、既存の経路がすでに与える性質になる。再スキャンは内容の鍵を再現し、まとまりは 1 つのキーを持ち、`userDataTables` と `moveUserData` は、まとめる操作と継承の操作がユーザーデータをコピーする唯一の場所である。

## R-3: 検索ボックスは、キーごとの検索キーを通じて品番、メーカー、出演者名を照合する {#r-3-the-search-box-matches-the-code-the-maker-and-the-performer-names-through-a-per-key-search-key}

**決定**: `video_work_info.search_key` は、品番、メーカー、すべての出演者名の `FoldForMatch` を改行でつないだものを持ち、値と同じトランザクションで書き、`search_version` が `domain.SearchKeyVersion` より遅れているときは起動時に埋め直す (`tag_names.search_key` と同じ)。検索の条件は、所有者に対して、場所の動画のユーザーキーの行での `instr(search_key, <term>) > 0` を、`tagNameMatchCondition` が組み立てる OR の 3 つ目の分岐として得る ([data-model.md、検索](data-model.md#search))。ゲストはこれに一致しない (R-7)。説明、日付、URL はキーに入らない。

| 案 | 判定 |
| --- | --- |
| **ユーザーキーごとのキーを、タグ名と同じく `instr` で照合する** | 採用 |
| 値を各場所の `search_key` に畳み込む (表示名と同じ) | 不採用: 値はユーザーキーに属するので、まとまりの値がすべてのメンバーの場所に書かれ、まとめる、まとまりを解く、継承するたびに場所のキーを更新しなければならなくなる |
| 値に対する FTS5 の trigram のテーブル | 不採用: trigram の索引は 3 文字を必要とし、このデータでは `AB-12` のような品番や 2 文字の名前がよくある場合である。タグ名は同じ理由ですでに `instr` を使っている |
| 説明を含める | 不採用: 長い説明は一般的な語に一致し、検索ボックスをメモの全文検索に変えてしまう。要件 4 はそれを求めていない |

**理由**: 検索ボックスは品番や出演者名で動画を見つけなければならない (受け入れ条件 3)。タグ名は、場所のキーと並べて照合する動画ごとの値の先例であり、その形を共有すれば、所有者の OR に分岐が 1 つ加わるだけで、ほかは何も変わらない。

## R-4: `performer` と `maker` の一覧のパラメーター。畳み込んだ値で照合し、ライブラリだけ {#r-4-performer-and-maker-list-parameters-matched-by-folded-value-library-only}

**決定**: `GET /api/videos`、`GET /api/library`、`GET /api/library/ids` は `performer` と `maker` を得る。それぞれ 1 つの値で、ほかのすべての条件と AND になる。値は、その `FoldForMatch` が保存された `name_key` または `maker_key` と等しいときに一致する ([contracts/screen-api.md、一覧のパラメーター](contracts/screen-api.md#list-parameters-performer-and-maker))。ライブラリの URL は `performer=<name>` と `maker=<name>` を持つ。動画のページで出演者やメーカーを押すと `/?performer=…` または `/?maker=…` を開く。`listFolderVideos` は、`tag` を受けないのと同じく、これらを受けない。ゲストは `400` を受ける (R-7)。

| 案 | 判定 |
| --- | --- |
| **一覧の API とライブラリの URL の値のパラメーター** | 採用 |
| タグと同じく、出演者とメーカーを id を持つ実体にする | 不採用: 親 Issue にはそれらを管理するものがない (改名、統合、同義語がない) ので、id のテーブルは所有者向けの操作のない 2 つ目の登録簿になる |
| 専用のページ (`/performers/<name>`) | 不採用: ライブラリにはすでに URL、スナップショット、条件の仕組みがあり、絞り込みは検索ボックスやタグと組み合わさる。別のページではそれを作り直さなければならない |
| そのままの綴りで照合する | 不採用: `山田 花子` と `山田　花子` が 2 人の出演者になる。検索ボックスはすでに畳み込んだ形で照合している |

**理由**: 受け入れ条件 6 は「値を押すと、同じ値を持つ動画が見える」である。URL の値でちょうどそれができる。値を改名すると URL が変わるのは許容できる。値があるページ以外にそこへリンクするものはないからである。

## R-5: オブジェクト全体を置き換える 1 つの `PUT`。消去したフィールドは、ない、`null`、または空 {#r-5-one-put-that-replaces-the-whole-object-a-cleared-field-is-absent-null-or-empty}

**決定**: `PUT /api/videos/{id}/work-info` はオブジェクト全体を受けて置き換える。ない、`null`、またはトリム後に空のフィールドは消去され、すべてのフィールドが消去された行は削除される。応答は `Video` である。`domain.NormalizeWorkInfo` が [data-model.md、ルール](data-model.md#rules) のルールを適用し、フィールドと問題を挙げる 1 つの `InvalidWorkInfoError` を返す ([contracts/screen-api.md、`PUT /api/videos/{id}/work-info`](contracts/screen-api.md#put-apivideosidwork-info))。値を変える書き込みは、タグと同じく、ユーザーキーの下のすべての内容の鍵の編集時刻を進める。何も変えない書き込みはそのままにする。

| 案 | 判定 |
| --- | --- |
| **オブジェクト全体を置き換える** | 採用 |
| フィールドごとに 1 つの `PUT` (`/work-info/code`、…) | 不採用: ページはフォームを保存し、外部のまとめての操作 (R-6) はオブジェクト全体を送る。フィールドごとのルートは、ルートとエラーの対応づけを 6 倍にする |
| マージの意味の `PATCH` | 不採用: すると「このフィールドを消去する」に「そのままにする」と区別される番兵が必要になるが、画面がそれを必要とすることはない。マージしたいクライアントは先にオブジェクトを読む |

**理由**: ページはフィールドをまとめて編集し、スクレイパーはまとめて書くので、変更の単位はオブジェクトである。置き換えは冪等なので、ネットワークの障害の後の再送は安全である。

## R-6: 外部 API は 1 つのまとめての操作を、MCP は 1 つのツールを得る。`workInfo` が `ExternalVideo` に加わる {#r-6-the-external-api-gets-one-bulk-operation-and-mcp-one-tool-workinfo-joins-externalvideo}

**決定**: `POST /api/v1/video-work-info` は `{ video, workInfo }` の `items` (1 から 20000、1 つのトランザクション、何かのエラーがあれば何も適用しない) を、R-5 と同じ置き換えの意味で受け、`ExternalVideo` は必須のフィールド `workInfo` を得る ([contracts/external-api.md](contracts/external-api.md))。MCP は `update_video_work_info` を得る。`list_videos` と `get_video` は新しいフィールドを含む。エラーはエラー本文に `field` を加えるので、クライアントはどの項目 (`index`) のどのフィールドが拒まれたかを知れる。

| 案 | 判定 |
| --- | --- |
| **1 つのまとめての置き換え、エラー本文に `field`** | 採用 |
| フィールドと問題ごとに 1 つの理由 (`code_too_long`、`maker_too_long`、…) | 不採用: 6 つのフィールドと 3 つの問題で 18 の列挙値になり、画面のカタログにはそれぞれのメッセージが必要になる |
| `replace` と並ぶ `merge` の操作 | 不採用: この機能が対象とする唯一の書き手 (#968) はオブジェクト全体を持つ。`v1` は後でクライアントを壊さずに `action` を得られる |

**理由**: `POST /api/v1/video-display-names` は外部の書き手がすでに使っている形である。それを保てば、#968 のスクレイパーは 1 つの呼び出しを加えるだけで済む。`field` のプロパティは互換性の方針のもとでの追加であり、同じプロパティが画面のエラーの行にも役立つ。

## R-7: ゲストは応答、検索、絞り込みのいずれでも作品情報を受けない {#r-7-guests-receive-no-work-information-in-responses-search-or-filters}

**決定**: ゲストへの応答は `workInfo` を省き (`displayName` と空の `tags` を省くのと同じ)、ゲストの検索の条件には作品情報の分岐がなく、ゲストからの `performer` または `maker` は、`Audience.CheckVideoQuery` を通じて `400` `guest_filter_not_allowed` を返す。

| 案 | 判定 |
| --- | --- |
| **タグと同じく所有者だけ** | 採用 |
| 公開された動画で表示し、ゲストに検索と絞り込みを許す | 不採用: ゲストの契約はタイトル以外の所有者が入力したすべてのフィールドを除く。#968 はこれらのフィールドを第三者から埋める。所有者が公開するのは動画であり、動画についてのメモではない |

**理由**: 親 Issue のすべての要件は所有者を挙げており、ゲストの契約のルールは、所有者のデータに依存する条件を提供しないことである。そのルールにフィールドを加える費用は `CheckVideoQuery` の 1 行と `forAudience` の 1 行であり、ゲストに見える面を広げるのは Issue が下さない決定である。

## R-8: 変更は `domain.VideoWorkInfoChanged` を発行し、画面の `video` 通知に対応づける {#r-8-a-change-publishes-domainvideoworkinfochanged-mapped-to-the-screens-video-notification}

**決定**: `WorkInfoStore` はコミット後に `VideoWorkInfoChanged{VideoIDs}` を書き込みごとに 1 つ発行し、`httpapi.Events.Handle` は、`VideoBundleChanged` を対応づけるのと同じく、それを各 id の `video` 通知に対応づける。ワーカーはこれを購読しない。

| 案 | 判定 |
| --- | --- |
| **id を持つ新しいイベント** | 採用 |
| `VideoOverrideChanged` を再利用する | 不採用: 上書きは内容ごとで 1 つの id を持つ。まとまりへの書き込みはすべてのメンバーに影響し、イベントを読む側が、その名前が 2 つのテーブルを扱うと知る必要があるべきではない |
| イベントなし。ページは自身の `PUT` の後に取得し直す | 不採用: 外部 API はページが開いている間に書き (受け入れ条件 5)、ページが再読み込みなしに値を表示できるのは通知を通じてだけである |

**理由**: 動画のページと一覧はすでに `video` で動画を読み直しているので、対応づけの 1 行で受け入れ条件 5 がそのまま得られる。
