---
source: specs/044-playlists/research.md
sourceHash: fcb510c17ed531fa2bcfa72e39fb28e130b7118e47311e2c7eeb3a25bb6ef34d
---

# 調査: 名前付きのプレイリストと連続再生 {#research-named-playlists-and-continuous-playback}

親 Issue: #787。引き継ぐ決定:

| 項目 | 正本 |
| --- | --- |
| 境界、ユーザーデータと作り直せるインデックス、認証の境界 | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| ユーザーキー、まとまり、同じパスでの継承 | [030 データモデル、User key](../030-video-versions/data-model.md#user-key)、[Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| フォルダのキー、グループのメンバーと順序、グループを開くときのメンバー | [017 データモデル、Assignment rules](../017-folder-groups/data-model.md#assignment-rules)、[internal/domain/library_item.go](../../internal/domain/library_item.go) の `GroupOpenIndex` |
| お気に入り: キー、まとめての書き込みの形、動画とグループの選択 | [035 調査、R-1](../035-favorites/research.md#r-1-two-tables-video_favorites-by-user-key-folder_favorites-by-folder-key)、[R-2](../035-favorites/research.md#r-2-one-owner-only-put-apifavorites-for-videos-and-folders-in-one-transaction)、[R-7](../035-favorites/research.md#r-7-selection-keeps-chosen-groups-as-groups) |
| グループの自動再生の通知と再生終了のレイヤー | [017 の UI 設計、Autoplay notice](../017-folder-groups/ui-design.md#autoplay-notice) |
| 所有者専用の画面とルート | [016 の UI 設計、Guest degradation](../016-single-account-auth/ui-design.md#guest-degradation)、[016 の auth-api、Three access classes](../016-single-account-auth/contracts/auth-api.md#three-access-classes) |
| 依存の追加 | [dependency-updates.md](../../docs/how-to/dependency-updates.md) |

このファイルは、この機能が加える決定だけを記録する。

## R-1: 2 つのテーブル。項目はお気に入りと同じようにキーを持ち、ライブラリへの外部キーを持たない {#r-1-two-tables-items-keyed-like-favorites-with-no-foreign-key-to-the-library}

**決定**: `playlists` と `playlist_items` を加える ([data-model.md、Migration](data-model.md#migration))。項目は動画のユーザーキーかグループのフォルダのキーのどちらかを持つ。これは `video_favorites` と `folder_favorites` が使うのと同じキーで、項目は `videos` にも `folder_groups` にも外部キーを持たない。`playlist_items` は `on delete cascade` で `playlists` を参照するので、プレイリストを削除するとその項目が消え、ほかには何も消えない。

| 案 | 判定 |
| --- | --- |
| **独自のテーブル、ユーザーキーとフォルダのキー** | 採用 |
| `videos.id` または `folder_groups.id` をキーにする項目 | 不採用: 動画の行は最後の場所がなくなると消え、`rebuildFolderIndex` はグループの id をすべて置き換えるので、再スキャンで項目が消える |
| 項目の種類ごとに 1 つのテーブル | 不採用: 順序は動画とグループをまたいで 1 つの並びになるので、1 つの `position` 列が両方を扱う必要がある |

**理由**: エッジケース「ファイルがなくなる / グループが解散するか名前が変わり、また戻る」は、お気に入りがこれらのキーによってすでに持つ性質である: キーが何にも解決しない項目は表示されず、キーが解決すると元の位置にまた表示される (R-4)。

## R-2: まとめることと継承は、すべてのメンバーの項目のキーを付け替え、重複をまとめる {#r-2-bundling-and-succession-re-key-every-members-items-and-merge-duplicates}

**決定**: 同じパスでの継承は `from` のキーの項目を `to` のキーに移す。まとめることは、すべてのメンバーの項目をまとまりのキーに移す。メンバーの内容の鍵のもとに残る行はない。最後から 2 番目のメンバーを取り除くと、まとまりのキーの項目は残ったメンバーに移る。他のメンバーが残る中でメンバーを取り除くと、項目はまとまりに残る。移動によって 1 つのプレイリストに同じキーの項目が 2 つできるときは、前の位置のほうが残り、プレイリストの番号を振り直す ([data-model.md、Succession and bundling](data-model.md#succession-and-bundling))。

| 案 | 判定 |
| --- | --- |
| **すべてのメンバーの項目をまとまりのキーに移す** | 採用 |
| 代表の行だけを写す `userDataTables` に `playlist_items` を加える | 不採用: 代表でないメンバーの項目は、まとめたときにプレイリストから消え、エッジケース「まとまりに引き継がれる」に反する |
| お気に入りのように、まとまりのキーに写し、メンバーの行を残す | 不採用: 隠れた写しが詰めた順序 (R-3) の位置を占め、所有者がプレイリストから取り除いた項目が、そのメンバーがまとまりを離れると戻ってくる (要件 5) |
| 重複では後ろの位置を残す | 不採用: 前の位置は所有者が最初に動画を置いた場所であり、裏での再スキャンで一覧が跳ねるのを防ぐ |

**理由**: お気に入りはまとまりごとに 1 つのフラグなので、代表のものを採るのは等しい値の間の選択である。プレイリストの項目は順序の中の場所であり、メンバーの場所を捨てると所有者がそこに置いたものが失われる。残るまとまりから取り出したメンバーは項目を失う。項目はまとめた時点からまとまりのものだからである。

## R-3: 詰めた整数の位置を「後ろへ移動」で並べ替える {#r-3-dense-integer-positions-reordered-by-move-after}

**決定**: `position` はプレイリストの中で 0、1、2、… である。並べ替えは、1 つの項目を別の項目のすぐ後ろか先頭に移す 1 つの要求 (`POST /api/playlists/{id}/items/{itemId}/move`) で、ストアはそのトランザクションの中でプレイリストの番号を振り直す。新しい項目は末尾に付く。

| 案 | 判定 |
| --- | --- |
| **1 つの項目を別の項目の後ろに移す** | 採用 |
| 順序全体を項目の id の一覧で置き換える | 不採用: R-4 で隠れた項目は画面にないので画面はそれを置けず、古いタブが別のタブの移動を取り消してしまう |
| 番号を振り直さない小数の位置 | 不採用: プレイリストが持つのは手で作った数の項目なので、移動ごとに 1 つのプレイリストの番号を振り直してもコストはなく、整数は精度が尽きない |

**理由**: 見えている隣の項目に対する相対で表した移動は、隠れた項目を隣にあった項目の隣に保つので、隠れた項目は元の場所に戻る。

## R-4: 隠れた項目は読むときに解決し、決して削除しない {#r-4-hidden-items-are-resolved-at-read-time-never-deleted}

**決定**: 動画の項目は、ライブラリの動画がそのユーザーキーを持つ間表示される。グループの項目は、そのキーを持つフォルダがグループである (`folder_groups.path_key`) 間表示される。読み取りは残りを除き、どの経路もそれを削除しない。画面の数 (「3 / 7」) は表示される項目を数える。

| 案 | 判定 |
| --- | --- |
| **読むときに除く** | 採用 |
| 内容かグループがなくなったときに項目を削除する | 不採用: エッジケースは項目がその位置に戻ることを求める |
| グレーアウトして表示する | 不採用: エッジケースは項目を表示しないと言う |

## R-5: 動画とグループを所有者だけが 1 回でまとめて加え、要求の順に末尾に付ける {#r-5-one-owner-only-bulk-add-for-videos-and-groups-appended-in-request-order}

**決定**: `POST /api/playlists/{id}/items` は `PUT /api/favorites` の形で `videoIds` と `folders` を受け取り、同じ方法で解決し、解決しないものとプレイリストがすでに持つものを飛ばし、残りを要求の順 (動画、次にフォルダ) で末尾に付ける。応答は、加えた数とすでにあった数を伝える ([contracts/screen-api.md、Add items](contracts/screen-api.md#post-apiplaylistsiditems))。

| 案 | 判定 |
| --- | --- |
| **1 つの要求、解決しないものと既存のものは飛ばす** | 採用 |
| 項目がすでにプレイリストにあるときは `409` | 不採用: 受け入れ条件 2 は繰り返しの追加が何も変えないことを求め、選択からまとめて加えると 1 つの重なりで失敗してしまう |
| 項目ごとに 1 つの要求 | 不採用: 数百の選択が、部分的に失敗しうる数百のトランザクションになる |

**理由**: 追加は、カード、選択バー、動画のページのお気に入りのトグルの隣に置く (要件 2)。これらはすでに `{videoIds, folders}` を送るので、画面はすでに持っている選択から要求を組み立てる。

## R-6: サーバーが、最新の順序からプレイリストの位置と次の動画を答える {#r-6-the-server-answers-the-playlist-position-and-the-next-video-from-the-latest-order}

**決定**: プレイリストの再生は、`?playlist=<id>&item=<itemId>` 付きの動画のページである。ページは `GET /api/playlists/{id}/playback?item=&video=` ([contracts/screen-api.md、Playback](contracts/screen-api.md#get-apiplaylistsidplayback)) を、開いたとき、動画が終わって通知を表示する前にもう一度、通知が次の動画がなくなったことに気づいたときにさらに一度読む。そのため再生できなくなった項目は飛ばされる (エッジケース)。応答は、表示される項目の中での位置、連続して再生する次の動画、前と次の項目の開始の動画を与える。順序の規則は `internal/domain` の 1 つの関数にある ([data-model.md、Playback order](data-model.md#playback-order))。

| 案 | 判定 |
| --- | --- |
| **サーバーで読み、終わりで読み直す** | 採用 |
| ページが `GET /api/playlists/{id}` から次の動画を計算する | 不採用: ページはすべてのグループのメンバーと視聴状態と飛ばす規則を必要とするが、それらはグループについてはすでにサーバーにある |
| ページを開いたときに 1 回だけ読む | 不採用: エッジケース「再生中に別の画面で並べ替える」は最新の順序を求める |
| `location.state` の文脈 | 不採用: 再読み込みや共有したタブではプレイリストから外れてしまう。クエリ文字列はどちらも越えて残る |

**理由**: 次の一歩は、グループの今のメンバー、その視聴状態、どの項目が表示されるかによる。ストアは 1 つのグループについてそのすべてをすでに読んでいる (`GroupOpenIndex`)。

## R-7: プレイリストの再生は、グループとフォルダの前後に優先する {#r-7-a-playlist-playback-overrides-the-group-and-folder-neighbours}

**決定**: ページがプレイリストの文脈を持つ間、矢印、通知、再生終了のレイヤーは、関連動画の `nextId` と `prevId` ではなくプレイリストの応答に従う。文脈がなければページは変わらない (要件 8、受け入れ条件 8)。プレイリストか今の項目がなくなったとき (`404`)、ページは文脈を捨て、今の動画の後に終わる。

| 案 | 判定 |
| --- | --- |
| **プレイリストの応答が前後を置き換える** | 採用 |
| グループの項目の中ではグループの前後、その端ではプレイリストの前後 | 不採用: 1 つの矢印に 2 つの出どころができる。プレイリストの応答はすでにグループの項目の中の次のメンバーを与える |
| 今の項目が取り除かれたら、その元の位置から続ける | 不採用: 位置は項目とともになくなっており、推測は項目を再び再生したり飛ばしたりしうる。エッジケースは削除したプレイリストでも同じように止まる |

## R-8: 既存の規則による所有者専用 {#r-8-owner-only-through-the-existing-rules}

**決定**: プレイリストのルートはすべて `/api/*` の既定により所有者専用なので、`accessRoutes` には何も加わらない。`/playlists` と `/playlists/{id}` はゲートの `ownerOnlyPath` に加わり、サイドバーの項目は `ownerOnly` で、追加の操作はお気に入りのトグルが出る場所にだけ出る。動画のページは所有者のときだけ playback の応答を読むので、公開の動画を `?playlist=` 付きで開いたゲストは今と同じように再生する。

| 案 | 判定 |
| --- | --- |
| **既存の既定とゲート** | 採用 |
| プレイリストの読み取りのためのゲストの区分 | 不採用: 要件 10 と範囲外の共有 |

## R-9: `@dnd-kit/core` と `@dnd-kit/sortable` によるドラッグでの並べ替え {#r-9-drag-reordering-with-dnd-kitcore-and-dnd-kitsortable}

**決定**: プレイリストの画面の並べ替えのため、`@dnd-kit/core` と `@dnd-kit/sortable` を、そのキーボードのセンサーとともに、Renovate の規則を通じて `web/` に加える ([dependency-updates.md](../../docs/how-to/dependency-updates.md))。ハンドルとドロップの見た目は `ui-design.md` が決める。

| 案 | 判定 |
| --- | --- |
| **dnd-kit** | 採用 |
| ネイティブの HTML のドラッグアンドドロップ | 不採用: スマートフォンでタッチに対応せず、キーボードの経路もない |
| 上へ移動と下へ移動のボタンだけ | 不採用: 長い一覧で項目を移動すると、1 つの場所ごとに 1 回押すことになる |

**理由**: dnd-kit はポインター、タッチ、キーボードで動き、移動をスクリーンリーダーに読み上げ、自前のマークアップを描かないので、行はレジストリのコンポーネントのままでいられる。

## R-10: ドメインイベントもライブの更新もない {#r-10-no-domain-event-and-no-live-update}

**決定**: プレイリストの書き込みはイベントを発行しない。プレイリストの画面は開いたときと自身の書き込みの後に読む。動画のページは R-6 のとおりに playback の応答を読む。

| 案 | 判定 |
| --- | --- |
| **イベントなし** | 採用 |
| `/api/events` の `playlist` イベント | 不採用: 別のタブのプレイリストの変更をライブで示す要件はなく、画面をまたぐ唯一の場合は動画の終わりの読み直しで扱える |

## R-11: プレイリストの名前はトリム後に 1 から 100 文字で、名前は重複してよい {#r-11-a-playlist-name-is-1-to-100-characters-after-trimming-names-may-repeat}

**決定**: ストアは名前をトリムする。空か、100 文字 (SQLite の `length()` と同じく Unicode のコードポイントで数える) より長いと `400 invalid_request` である。2 つのプレイリストは同じ名前を持てる (エッジケース)。規則は `domain.ValidatePlaylistName` が持つ。

| 案 | 判定 |
| --- | --- |
| **1 から 100、重複を許す** | 採用 |
| 長さの上限なし | 不採用: 名前は画面の見出しでありメニューの項目である。上限が両方を際限なく伸びないようにする |
| 一意の名前 | 不採用: エッジケースは同じ名前を許す |
