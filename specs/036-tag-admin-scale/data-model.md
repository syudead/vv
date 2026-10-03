# Data model: タグ管理画面のまとめての操作と画面の側の状態

親 Issue: #651。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、タグの表と名前の
規則・書き換えの規則は [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、仮のタグと
却下した名前は [specs/031-tentative-tags/data-model.md](../031-tentative-tags/data-model.md) にある。
**この feature は移行を足さず、どの表の定義も変えない。** ここには `domain` に足す値、保存層に足す操作、
画面の側で持つ状態の規則だけを書く。書いていない操作（1 件の作成・改名・削除・確定・却下・シノニム、
付け外し、絞り込み、検索、本数）は変えない。

## 1. `domain` に足す値

| 値 | 中身 |
| --- | --- |
| `Tag.CreatedAt time.Time` | `tags.created_at`（Unix 秒）。`ListTags`・`tagByID` が読む。API では `Tag.createdAt`（[research.md R-8](research.md#r-8-作った日は-tagscreated_at-を-tagcreatedat-として載せ同じ秒のタグは名前の順にする)） |
| `TagBatchAction` | `TagBatchConfirm`・`TagBatchReject`・`TagBatchDelete` の 3 値。`Valid()` を持つ |
| `TagBatchOutcome{AppliedIDs, NotFoundIDs, NotApplicableIDs []int64}` | まとめての操作の結果。どの配列も `ids` に現れた順で、空なら空の配列（`nil` にしない） |
| `TagMergeOutcome{Tag Tag, NotFoundIDs []int64}` | 統合の結果。`Tag` は統合後の統合先 |
| `TagImpact{TagCount, VideoCount int}` | 確認に出す数。`TagCount` は `ids` のうち今あるタグの数、`VideoCount` はそのどれかが付いた動画の本数（重複なし） |
| `MaxTagBatch = 20000` | `ids`・`sourceIds` の上限。`MaxVideoTagsSelection` と同じ理由（[R-4](research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かないないタグは数えて飛ばす)） |

「働く種類」の規則は純関数 `TagBatchApplies(action TagBatchAction, tentative bool) bool` として `domain` に
置く: `confirm`・`reject` は `tentative` が真のとき、`delete` は偽のとき真。画面の 1 行ずつの規則
（仮の行に確定・却下、確定した行に削除。[031 の ui-design.md「Row」](../031-tentative-tags/ui-design.md#row)）と
同じで、要件 7 の「今の 1 行ずつの操作の規則は変えない」をサーバーの側でも守る。

## 2. 保存層の操作

`TagStore` に足す。すべて共有する SQLite 接続だけを使い、ドメインイベントは発行しない（タグの変更は
副作用を持たない。ARCHITECTURE.md の `TagStore` の段落のまま）。`ids` は `json_each` に 1 つの引数で渡す
（`external_video_tags.go` と同じ）。

| 操作 | 1 つの取引で行うこと |
| --- | --- |
| `BatchTags(ctx, action, ids) (TagBatchOutcome, error)` | `ids` の重複を除き、`tags` を結んで今あるものと `tentative` を読む。無い id は `NotFoundIDs`、`TagBatchApplies` が偽の id は `NotApplicableIDs`。残りに対して: `confirm` は `update tags set tentative = 0`（031 §3 の確定）、`reject` は各タグの元の名前を `rejected_tag_names` に `insert or ignore` してから `delete from tags`（031 §3 の却下を複数に）、`delete` は `delete from tags`（014 §4 の削除）。`AppliedIDs` はその id |
| `MergeTags(ctx, targetID, sourceIDs) (TagMergeOutcome, error)` | 統合先が無ければ `ErrTagNotFound`。`sourceIDs` の重複を除き、無い id は `NotFoundIDs`。残りの各 id について今の `mergeTagInto` を順に呼ぶ（付与の写し、名前の移動、統合元の削除、統合先の確定）。1 件の `MergeTag` はこれで置き換える |
| `TagImpact(ctx, ids) (TagImpact, error)` | `TagCount` は `tags` にある id の数。`VideoCount` は `taggedVideosSQL` に `tag_id in (json_each)` の条件を付け、`video_id` を `distinct` に数える（手で付けた分とフォルダ名から付いている分のどちらでも 1 本。014 §5） |

既存の操作の変更:

- `listCanonicalTags`・`tagByID` は `t.created_at` を読んで `Tag.CreatedAt` に載せる。
- `MergeTag(ctx, targetID, sourceID)` は消し、呼び手（`internal/httpapi`）は `MergeTags` を使う。
  `AddSynonym` の承諾した統合は `mergeTagInto` を直接使っていて変わらない。
- `internal/httpapi` が宣言する `Tags` の interface（`router.go`）に `BatchTags`・`MergeTags`・`TagImpact` を
  足し、`MergeTag` を外す。配線は `cmd/mdm`（変更なし。同じ `store.TagStore` が満たす）。

不変条件（[031 の data-model.md §1](../031-tentative-tags/data-model.md#1-マイグレーション) の 2 つ）は、
まとめての却下・統合のあとも成り立つ。`invariants_test.go` の既存の検査を、まとめての操作のあとにも
走らせる試験を店の試験に足す。

## 3. 書き換えの規則の対応

014 §4・031 §3 の表に新しい種類の書き換えは無い。まとめての操作は、1 件の操作を同じ取引の中で
複数回行ったものと同じ結果になる。

| まとめての操作 | 1 件の操作との関係 |
| --- | --- |
| 確定 | `ConfirmTag` を各 id に行ったのと同じ。既に確定したタグは飛ばして数える（1 件では何も変えずに 200 を返す点が違う） |
| 却下 | `RejectTag` を各 id に行ったのと同じ。確定したタグは飛ばして数える（1 件では `409 tag_not_tentative`） |
| 削除 | `DeleteTag` を各 id に行ったのと同じ。仮のタグは飛ばして数える（1 件の経路は仮のタグも消すが、画面は仮の行に削除を出さない。031 §3） |
| 統合 | `mergeTagInto` を各統合元に行ったのと同じ。統合先は 1 回で確定になる |

## 4. 画面の側で持つ状態

タグ管理画面（`web/src/tags/TagsPage.tsx`）が持つ状態と、その規則。並び順以外はこの画面の状態で、
URL にも `localStorage` にも載せない（[R-7](research.md#r-7-並び順はこの画面の端末の設定として-localstorage-に持ち絞り込みは今までどおり画面の状態に留める)）。

| 状態 | 規則 |
| --- | --- |
| 並び順 `TagListSort` | `name`（既定）・`countDesc`・`countAsc`・`createdDesc`・`createdAsc`。同値は `compareTagRefs`（名前の自然順）。`web/src/preferences/tagListPreferences.ts` に保存し、読めない・壊れているときは `name` |
| 絞り込み | 「Tentative only」（今のまま）と「0 本のみ」（`videoCount === 0`）。両方オンなら両方を満たす行 |
| 検索 | 検索語と、各タグの名前・シノニムの照合形（`foldForMatch`、[R-3](research.md#r-3-一覧の検索は-domainfoldformatch-を-typescript-に移植して照合し同じ入力の組で両方を検査する)）の部分一致。照合形はタグの配列が変わったときに作り直し、打鍵ごとには作らない |
| 見えている行 `visibleRows` | 全件 → 絞り込み → 検索 → 並び替え。改名中の行は一致しなくても残す（今と同じ。Edge Case「改名中の行」） |
| 件数の行 | 絞り込み・検索のどちらかが効いているときは「〈見えている数〉 of 〈全体〉」（受け入れ条件 7）。改名中の残した行は数えない（今と同じ） |
| 選択 `Set<number>` | 見えている行の id の部分集合。`visibleRows` が変わるたびに、見えなくなった id を外す（Edge Case「選んでいる間に検索・絞り込み・並び順を変えたとき」）。「見えているものをすべて選ぶ」は `visibleRows` の id 全部（要件 9） |
| まとめての操作の結果 | `appliedIds` を選択から外し、一覧へその場で反映する（確定は `tentative: false` に差し替え、却下・削除・統合元は取り除き、統合先は応答の `tag` に差し替え）。`notFoundIds` が空でなければ `reload` で一覧を取り直す（Edge Case「別のタブで対象の一部が消えたとき」）。失敗したら何も変えず、選択は残る（Edge Case「途中で失敗したとき」） |
| 描く行 | `visibleRows` のうち仮想化が表示域と前後に入ると決めた行だけ（[R-2](research.md#r-2-行の仮想化は-tanstackreact-virtual-の-usewindowvirtualizer-で行う)）。フォーカスを行へ移すときは、その行の位置へスクロールしてから移す |

共有の保持（`web/src/api/tags.ts`）の規則は変えない。まとめての操作の関数も、1 件の操作と同じく成功の
あとに `afterTagChanged`（一覧の控えの破棄と共有の保持の取り直し）を 1 回呼ぶ。
