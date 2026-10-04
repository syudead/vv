# Data model: タグ管理画面のページ読み・まとめての操作と画面の側の状態

親 Issue: #651。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、タグの表と名前の
規則・書き換えの規則は [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、仮のタグと
却下した名前は [specs/031-tentative-tags/data-model.md](../031-tentative-tags/data-model.md) にある。
ここには `domain` に足す値、保存層に足す・変える操作、画面の側で持つ状態の規則だけを書く。書いていない
操作（1 件の作成・改名・削除・確定・却下・シノニム、付け外し、本数）は変えない。

§1〜§3 のまとめての操作（`BatchTags`・`MergeTags`・`TagImpact`）は feature branch に merge 済みで、
親 Issue の改訂で変わらない。§0 の移行、§1 のページの値、§2 の `ListTags`・`ListRejectedTagNames`・
鍵の書き、§4 は改訂で足した・書き直した部分である。

## 0. マイグレーション

1 つ足す（`00030_tag_sort_keys.sql`。[research.md R-10](research.md#r-10-名前の自然順の鍵-sort_key-を-tag_names-と-rejected_tag_names-に持ち起動時の鍵の埋め直しで作る)）。
表の意味も既存の列も変えない。

| 表 | 足す列 | 規則 |
| --- | --- | --- |
| `tag_names` | `sort_key text not null default ''` | `domain.NaturalSortKey(name)`。名前の行を書く操作（作成・シノニム登録・付与での作成・改名）が `search_key` と同じ取引で書く。統合で `tag_id` を付け替えても変わらない |
| `rejected_tag_names` | `sort_key text not null default ''`、`search_version integer not null default 0` | `domain.NaturalSortKey(name)`。却下が書く。`search_version` は `tag_names.search_version` と同じ意味（`domain.SearchKeyVersion`） |

移行は `update tag_names set search_version = 0` を行い、既存の行の `sort_key` は起動時の
`TagStore.RefreshSearchKeys` が `search_key` と一緒に埋める（§2）。`rejected_tag_names` の既存の行は
`search_version` の既定が 0 なので同じ埋め直しに入る。鍵は作り直せる派生の値で、`SearchKeyVersion` を
上げたときも同じ仕組みで作り直される。

名前の自然順の定義: `sort_key` のバイト順、同じなら `tags.id`（却下した名前では `name` のバイト順）。
`NaturalSortKey` は照合形（`FoldForMatch`）に掛けるので、全角・半角・かなが同じ名前は同順位になり、
`id` で決着する（R-10「名前の順の定義が変わる点」）。

## 1. `domain` に足す値

merge 済み（まとめての操作）:

| 値 | 中身 |
| --- | --- |
| `Tag.CreatedAt time.Time` | `tags.created_at`（Unix 秒）。`Tag` を返す操作はすべて載せる。API では `Tag.createdAt`（[research.md R-8](research.md#r-8-作った日は-tagscreated_at-を-tagcreatedat-として載せ同じ秒のタグは名前の順にする)） |
| `TagBatchAction` | `TagBatchConfirm`・`TagBatchReject`・`TagBatchDelete` の 3 値。`Valid()` を持つ |
| `TagBatchOutcome{AppliedIDs, NotFoundIDs, NotApplicableIDs []int64}` | まとめての操作の結果。どの配列も `ids` に現れた順で、空なら空の配列（`nil` にしない） |
| `TagMergeOutcome{Tag Tag, NotFoundIDs []int64}` | 統合の結果。`Tag` は統合後の統合先 |
| `TagImpactAction` | `TagImpactReject`・`TagImpactDelete`・`TagImpactMerge` の 3 値。確認をとる操作（要件 11）で、`Valid()` を持つ |
| `TagImpact{TagCount, VideoCount int}` | 確認に出す数。`TagCount` は `ids` のうち今あり、その操作が働くタグの数（`TagImpactApplies`）、`VideoCount` はそのどれかが付いた動画の本数（重複なし） |
| `MaxTagBatch = 20000` | `ids`・`sourceIds` の上限。`MaxVideoTagsSelection` と同じ理由（[R-4](research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かない無いタグは数えて飛ばす)） |

「働く種類」の規則は純関数 `TagBatchApplies(action TagBatchAction, tentative bool) bool`: `confirm`・`reject` は
`tentative` が真のとき、`delete` は偽のとき真。画面の 1 行ずつの規則（[031 の ui-design.md「Row」](../031-tentative-tags/ui-design.md#row)）と
同じで、要件 8 の「今の 1 行ずつの操作の規則は変えない」をサーバーの側でも守る。確認の数の規則は
`TagImpactApplies(action TagImpactAction, tentative bool) bool`: `reject`・`delete` は `TagBatchApplies` の同じ
名前の操作と同じ値、`merge` は種類によらず真。

改訂で足す（ページ読み。[R-1](research.md#r-1-一覧はサーバーのページで受け検索絞り込み並び順はサーバーが全部のタグに掛ける)）:

| 値 | 中身 |
| --- | --- |
| `TagSort` | `TagSortName`（既定）・`TagSortCountDesc`・`TagSortCountAsc`・`TagSortCreatedDesc`・`TagSortCreatedAsc`。API の `TagSort` と同じ文字列（`name`・`countDesc`・`countAsc`・`createdDesc`・`createdAsc`）。`Valid()` を持つ |
| `TagListQuery{Search string; TentativeOnly, UnusedOnly bool; Sort TagSort; Cursor string; Limit int}` | 一覧の条件。`Search` は呼び手が受けた生の文字列で、店が `FoldForMatch` を掛けて前後の空白を落とす（空なら絞らない）。`Limit` が 0 なら全件（カーソルは無視し、`NextCursor` は空）。`Sort` が空なら `TagSortName` |
| `TagPage{Items []Tag; Total, TotalAll int; NextCursor string}` | 1 ページ。`Total` は条件（検索・絞り込み）に合うタグの数、`TotalAll` は全部のタグの数。`NextCursor` は続きが無ければ空 |
| `RejectedTagNamePage{Items []string; Total int; NextCursor string}` | 却下した名前の 1 ページ |
| `MaxTagPageLimit = 200` | `limit` の上限。`GET /api/library` と同じ |

`ErrInvalidCursor` は 013 のものを使う（別の並び順で作ったカーソル、解釈できないカーソル）。

## 2. 保存層の操作

`TagStore` に足す・変える。すべて共有する SQLite 接続だけを使い、ドメインイベントは発行しない（タグの
変更は副作用を持たない。ARCHITECTURE.md の `TagStore` の段落のまま）。`ids` は `json_each` に 1 つの
引数で渡す（`external_video_tags.go` と同じ）。

merge 済み（まとめての操作）:

| 操作 | 1 つの取引で行うこと |
| --- | --- |
| `BatchTags(ctx, action, ids) (TagBatchOutcome, error)` | `ids` の重複を除き、`tags` を結んで今あるものと `tentative` を読む。無い id は `NotFoundIDs`、`TagBatchApplies` が偽の id は `NotApplicableIDs`。残りに対して: `confirm` は `update tags set tentative = 0`、`reject` は各タグの元の名前を `rejected_tag_names` に `insert or ignore` してから `delete from tags`、`delete` は `delete from tags`。`AppliedIDs` はその id |
| `MergeTags(ctx, targetID, sourceIDs) (TagMergeOutcome, error)` | 統合先が無ければ `ErrTagNotFound`。`sourceIDs` の重複を除き、無い id は `NotFoundIDs`。残り（統合先の id は除く）を `mergeTagsInto`（統合元の数によらない文の数で付与の写し・名前の移動・統合元の削除を行う）に渡し、そのあと `tagByID` で統合先を 1 回だけ読む |
| `TagImpact(ctx, action, ids) (TagImpact, error)` | `ids` の重複を除き、今あるものと `tentative` を読み、`TagImpactApplies` が真の id だけを残す。`TagCount` はその数。`VideoCount` は `taggedVideosSQL` に残した id の条件を付け、`video_id` を `distinct` に数える |

改訂で変える・足す:

| 操作 | 行うこと |
| --- | --- |
| `ListTags(ctx, query TagListQuery) (TagPage, error)` | 今の `ListTags(ctx)` を置き換える。1 つの問い合わせで、本数を `taggedVideosSQL("")` の集計（CTE）から全タグに付け、条件（`Search` → `exists (select 1 from tag_names tn where tn.tag_id = t.id and instr(tn.search_key, ?) > 0)`、`TentativeOnly` → `t.tentative = 1`、`UnusedOnly` → 本数が 0）を掛け、並び順（下の表）とカーソルの条件を付けて `Limit + 1` 件読む。`Limit + 1` 件目があれば `Limit` 件目から `NextCursor` を作る。ページのタグのシノニムは `tag_id in (json_each)` で 1 回読む。`Total` は同じ条件の `count(*)`、`TotalAll` は `tags` の `count(*)`。`Limit` が 0 なら条件と並び順だけを掛けて全件を返す（外部連携 API の `listTags` と候補の全件は `TagListQuery{}` で呼ぶ） |
| `ListRejectedTagNames(ctx, cursor string, limit int) (RejectedTagNamePage, error)` | 今の全件を返す形を置き換える。`sort_key, name` の順で `limit + 1` 件読み、`Total` は `count(*)`。カーソルは `sort_key` と `name` を包む |
| `RefreshSearchKeys(ctx) (int, error)` | `tag_names` の `search_version` が現在の版より小さい行の `search_key` と `sort_key` を書き直し、`rejected_tag_names` の同じ条件の行の `sort_key` を書き直す。戻り値は書き直した行の数（両方の合計） |
| 名前の行を書く操作（`insertTagName` を通る作成・シノニム登録・付与での作成・改名、`RejectTag`・`BatchTags(reject)` の `rejected_tag_names` への挿入） | `sort_key = domain.NaturalSortKey(name)` を同じ文で書く |

並び順と keyset の条件（`listOrder` の `orderBy`・`after` と同じ考え。値の向きと名前の向きが違うので、
行値の比較 1 つではなく展開した形で書く）:

| `TagSort` | `order by` | カーソルが包むもの | 「カーソルより後」の条件 |
| --- | --- | --- | --- |
| `name` | `tn.sort_key asc, t.id asc` | `sort_key`, `id` | `(tn.sort_key, t.id) > (?, ?)` |
| `countDesc` / `countAsc` | `video_count desc/asc, tn.sort_key asc, t.id asc` | `video_count`, `sort_key`, `id` | `video_count < ?`（asc は `>`）`or (video_count = ? and (tn.sort_key, t.id) > (?, ?))` |
| `createdDesc` / `createdAsc` | `t.created_at desc/asc, tn.sort_key asc, t.id asc` | `created_at`, `sort_key`, `id` | 本数と同じ形 |

カーソルは並び順の名前・値・`sort_key`・`id` を包んだ不透明な文字列で、別の並び順のカーソルや
解釈できないものは `ErrInvalidCursor`（`listing.go` の `encodeCursor`・`decodeCursor` と同じ形。
共有できる部分は共有する）。

不変条件（[031 の data-model.md §1](../031-tentative-tags/data-model.md#1-マイグレーション) の 2 つ）に、
「`tag_names` と `rejected_tag_names` の `search_version` が現在の版の行は、`sort_key` が
`NaturalSortKey(name)` と一致する」を足し、`invariants_test.go` の検査に入れる。

`internal/httpapi` が宣言する `Tags` の interface（`router.go`）の `ListTags`・`ListRejectedTagNames` の
署名が変わる。配線は `cmd/mdm`（変更なし）。

## 3. 書き換えの規則の対応

014 §4・031 §3 の表に新しい種類の書き換えは無い。まとめての操作は、1 件の操作を同じ取引の中で
複数回行ったものと同じ結果になる（merge 済み）。

| まとめての操作 | 1 件の操作との関係 |
| --- | --- |
| 確定 | `ConfirmTag` を各 id に行ったのと同じ。既に確定したタグは飛ばして数える（1 件では何も変えずに 200 を返す点が違う） |
| 却下 | `RejectTag` を各 id に行ったのと同じ。確定したタグは飛ばして数える（1 件では `409 tag_not_tentative`） |
| 削除 | `DeleteTag` を各 id に行ったのと同じ。仮のタグは飛ばして数える（1 件の経路は仮のタグも消すが、画面は仮の行に削除を出さない。031 §3） |
| 統合 | 1 件の統合を各統合元に順に行ったのと同じ結果を、`mergeTagsInto` が統合元の数によらない文の数で作る。統合先は 1 回で確定になる |

`sort_key` は名前の行と一緒に書かれ、名前の行が消えれば消える。書き換えの規則に影響しない。

## 4. 画面の側で持つ状態

タグ管理画面（`web/src/tags/TagsPage.tsx`）が持つ状態と、その規則。条件（検索語・絞り込み・並び順）と
タブは URL のクエリに載せ（`web/src/tags/tagListUrl.ts`、[ui-design.md「URL state」](ui-design.md#url-state)）、
`localStorage` には並び順だけを残す（[R-7](research.md#r-7-並び順はこの画面の端末の設定として-localstorage-に持ち絞り込みは今までどおり画面の状態に留める) の追記）。
画面は共有の保持（`web/src/api/tags.ts` の `getTags`・`subscribeTags`）を使わない（[R-1](research.md#r-1-一覧はサーバーのページで受け検索絞り込み並び順はサーバーが全部のタグに掛ける)）。

| 状態 | 規則 |
| --- | --- |
| 条件 `query` | 検索語、「Tentative only」、「Unused only」、並び順 `TagSort`（URL の `sort`。無ければ `tagListPreferences.ts` に保存した値、読めない・壊れているときは `name`）。どれかが変わると: 進行中の要求を打ち切り、世代の番号を進め、選択を空にし、先頭のページを要求する（Edge Case「選んでいる間に…」「続きを読んでいる最中に…」）。届くまで前の行は残す（空の一瞬を作らない） |
| ページ `rows` | 読み込んだ行（`Tag[]`）、`total`、`totalAll`、`nextCursor`、続きの読み込み中か、続きの失敗。続きは末尾に `id` の重複を捨てて足す（[R-11](research.md#r-11-続きは画面の末尾に近づいたら-100-件ずつ読みid-で重複を捨て件数が食い違えば知らせて取り直させる)）。1 ページは 100 件 |
| 続きを読むきっかけ | 仮想化が描く最後の行の位置が `rows.length - overscan` 以上で、`nextCursor` があり、続きを読んでいなければ 1 回要求する。失敗したら読み込んだ行を残し、末尾の失敗の行の「Retry」が同じカーソルで読み直す（Edge Case「続きの読み込みに失敗したとき」） |
| 食い違い `inconsistent` | 続きの応答の `totalAll` が画面の値と違うとき真。行は残し、続きは止め、「一覧が変わった」の 1 行と「取り直す」を出す。取り直しは先頭から読み直して選択を空にする（Edge Case「続きを読むあいだに別のタブで…」） |
| 読み込み失敗 | 先頭のページの失敗: 一覧を持っていなければ今の失敗の表示と「Retry」、持っていればその一覧を残して理由を控える（Edge Case「一覧の読み込みに失敗したとき」） |
| 見えている行 `visibleRows` | `rows` そのまま。改名中の行が `rows` に無ければ（条件を変えて先頭から読み直したとき）`sort` の位置に差し込んで残す（Edge Case「改名中の行」）。差し込む位置は `naturalSortKey` と並び順の値で決める（[R-12](research.md#r-12-操作のあとの反映は読み込んだ行の中で行い並びの位置は-naturalsortkey-の移植で決める)） |
| 見出しの件数 | 検索・絞り込みのどちらかが効いているときは「〈`total`〉 of 〈`totalAll`〉」（受け入れ条件 8）、効いていなければ「〈`totalAll`〉」。読み込んだ数は出さない。差し込んだ改名中の行は数えない。先頭のページが届く前は読み込み中 |
| タブ | 「Tags」か「Rejected names」（URL の `tab`）。切り替えると選択・作成・改名を閉じる。「Rejected names」の間は一覧の続きを読まない |
| 選択 `Set<number>` | `rows` の id の部分集合。`rows` から消えた id、改名中の行の id を外す。「読み込んだものをすべて選ぶ」は `rows` の id 全部（要件 10。読み込んでいないタグは選ばれない）。上限は送る id の数に掛ける（[R-4](research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かない無いタグは数えて飛ばす)）: `rows.length` が `maxTagBatch` を超えると先頭のチェック（読み込んだものをすべて選ぶ）だけを disabled にし、まとめての操作は選択の数が `maxTagBatch` を超えるときだけ disabled にする（1 行ずつ選んだ選択は、何行読み込んでいても操作できる。要件 8・9） |
| 操作のあとの反映 | 1 件の操作とまとめての操作のどちらも `rows` の中で行う（R-12）: 確定は `tentative: false` に差し替え（「Tentative only」が効いていれば取り除く）、却下・削除・統合元は取り除き `total`・`totalAll` を減らす、統合先は応答の `tag` で書き換える（`rows` にあれば差し替えて位置を直し、無ければ（統合の窓がサーバーから引いた読み込んでいないタグ）位置に差し込む。統合の前の `Tag`（行か窓の候補）と応答の `tag` をそれぞれ今の条件に照らし、合っていたのに合わなくなれば取り除いて `total` を減らし、合っていなかったのに合うようになれば `total` を増やす。統合元の名前がシノニムに移るので検索語に合うようになりうる）、改名は名前を差し替えて位置を直し、今の条件に合わなくなれば取り除く、作成は条件に合えば位置に差し込み `total`・`totalAll` を増やす（合わなければ `totalAll` だけ増やす）。条件に合うかは `foldForMatch` の部分一致・`tentative`・`videoCount === 0` で決める（R-3）。位置を直す・差し込むは `naturalSortKey` と並び順の値で決めた位置に置くが、その位置が読み込んだ範囲の外（最後の行より後ろで `nextCursor` がある）なら `rows` に入れない（入っていれば取り除く）。続きのカーソルは最後の行の鍵なので、その後ろの行は続きのページが返し、前に動いた行（`countDesc` で本数が増えた統合先など）は返さないため、前に動いた行は画面が置かなければ一覧から抜ける（R-12）。`notFoundIds` が空でなければ先頭から取り直す。失敗したら何も変えず、選択は残る |
| 描く行 | `visibleRows` のうち仮想化が表示域と前後に入ると決めた行と、フォーカスを持つ行・改名中の行（[R-2](research.md#r-2-行の仮想化は-tanstackreact-virtual-の-usewindowvirtualizer-で行う)、merge 済み） |
| 却下した名前 | 先頭の 1 ページ（`items`・`total`・`nextCursor`）を画面を開いたときと 031 のきっかけで取り直す。「Rejected names」のタブで並びの末尾までスクロールしたら続きを足す。「Allow again」で外した名前は局所で取り除き `total` を減らす（[R-13](research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)） |
| 統合の窓の候補 | 入力の照合形で `GET /api/tags?q=…&limit=…` を呼んで候補にする。行から開いたときは統合元を除き、選んだ中から開いたとき（`fromSelection`）は選んだタグも候補に残す（選んだ中から統合先を選ぶと統合元から外す。要件 9・Edge Case、merge 済みの形）。入力が変わったら進行中の要求を打ち切る（[R-14](research.md#r-14-統合の窓の統合先の候補は-get-apitagsqlimit-で引く)） |

共有の保持（`web/src/api/tags.ts`）は候補・絞り込みの確かめのために残る。1 件とまとめての操作の関数が
成功のあとに呼ぶ `afterTagChanged` は、購読者がいれば今までどおり取り直し、いなければ `held` を捨てる
（R-12）。動画一覧の控え（`clearListSnapshot`）の扱いは変えない。
