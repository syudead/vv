# Research: タグ管理画面を数千個のタグに耐えさせる

技術スタック、境界と依存方向、タグの表と名前の規則、仮のタグと却下した名前、画面の文言と書式、
一覧画面の見た目の規則は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、
[ARCHITECTURE.md](../../ARCHITECTURE.md)、
[specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
[specs/031-tentative-tags/data-model.md](../031-tentative-tags/data-model.md)、
[docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)、
[docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)）。
ここにはこの feature が足す決定だけを書く。

## R-1: 一覧は `GET /api/tags` の全件を今までどおり 1 回で受け、画面の側で見えている行だけを描く

- Decision: サーバーの一覧はページングしない。画面は共有の保持（`web/src/api/tags.ts`）が持つ全件を
  入力に、並び替え・絞り込み・検索を画面の側で行い、描くのは表示域に入る行と前後の少数だけにする
  （[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。
- Rationale: 親 Issue の計測で遅いのは描画である（1,000 個で約 22,000 要素を毎回描き直す）。全件の
  JSON は 3,000 個でも数百 KB で、今も候補（combobox）・絞り込みの確かめ・管理画面が同じ 1 回の取得を
  共有している。「見えているタグをまとめて選ぶ」（要件 9）と件数の行（受け入れ条件 7）は絞り込み後の
  全集合を要るので、画面がその集合を持っているのがいちばん単純である。
- Alternatives considered: サーバーでページングし検索・並び順・絞り込みをパラメータにする（候補と絞り込みの
  確かめが使う共有の保持と一覧が別の経路になり、「見えているものをすべて選ぶ」に `ids` の経路が要る。
  描画の遅さは解けるが、要らない API の面を増やす）。描画を変えずに `React.memo` と計算の記憶だけで済ます
  （行の数そのものが要素数を決めるので、3,000 行では受け入れ条件 1 の 1 秒に届かない）。

## R-2: 行の仮想化は `@tanstack/react-virtual` の `useWindowVirtualizer` で行う

- Decision: `web/package.json` に `@tanstack/react-virtual` を足し、タグの一覧だけで使う。スクロールの
  持ち主は今までどおり window（文書）で、行の高さは描いた要素を測って持つ（`measureElement`）。
- Rationale: 行の高さは一様でない。シノニムの行を持つタグは 1 行高く、改名中の行は失敗の文言で伸び、
  作成の行が先頭に入る。自前で窓を切るには高さの計測と累積の管理を書くことになり、そこが不具合の
  置き場になる。この依存は実行時の依存を持たず、Renovate の運用（[docs/how-to/dependency-updates.md](../../docs/how-to/dependency-updates.md)）
  に乗る。[library-ui.md §3](../../docs/design-docs/library-ui.md#3-なぜ仮想スクロールを使っていないのか) が
  仮想スクロールを避けた理由は「折り返す格子で 1 行の枚数が幅で変わる」「60 件ずつしか読まない」の
  2 つで、1 列・全件を一度に持つこの一覧には当たらない。同じ節が「測ってから見直す」としていて、
  親 Issue がその計測である。
- Alternatives considered: 自前の窓切り（高さを 2 種類に固定すれば累積は足し算で済むが、改名の失敗の
  文言と作成の行で崩れる。崩さないために行の形を変えるのは、実装の都合で設計を狭めることになる）。
  行の高さを 1 種類に固定してシノニムを名前の行に詰める（[014 の ui-design.md「Rows」](../014-video-tags/ui-design.md#rows)
  の情報階層を変える。要求にその理由が無い）。`react-window`（固定高か高さの関数を要し、計測は自前になる）。
- 文書: `docs/design-docs/library-ui.md` §3 に、タグ管理画面の一覧がこれを使うことと、格子の一覧が
  今も使わない理由はそのままであることを書く。

## R-3: 一覧の検索は `domain.FoldForMatch` を TypeScript に移植して照合し、同じ入力の組で両方を検査する

- Decision: `web/src/lib/foldForMatch.ts` に `foldForMatch(s)` を置く。NFKC 正規化（`String.prototype.normalize`）、
  符号位置ごとの小文字化、ひらがな（U+3041–U+3096、U+309D・U+309E）からカタカナへの `+0x60` の順で、
  Go の [`FoldForMatch`](../../internal/domain/search.go) と同じ手順にする。管理画面の検索は、各タグの
  名前とシノニムの照合形を 1 回作って覚え、検索語の照合形が部分一致するかで決める。
  入力と期待の組を `internal/domain/testdata/fold_for_match.json` に 1 つ置き、Go の試験と Vitest の
  試験が同じファイルを読む（`web/src/theme/tokens.test.ts` が `index.css` をファイルとして読むのと同じ
  やり方）。
- Rationale: 要件 6 は「ライブラリでタグを探すときと同じ規則」で、その規則は `FoldForMatch` が
  `tag_names.search_key` に掛けている照合形である（[014 の data-model.md §7](../014-video-tags/data-model.md#7-検索欄でのタグ名の照合)）。
  一覧は全件を画面が持っている（R-1）ので、照合も画面で行えば打鍵ごとの往復が無く、要件 2・
  受け入れ条件 2 の 0.2 秒に収まる。2 言語に同じ規則を持つ代わりに、同じ入力の組で食い違いを機械で
  見つける。
- Alternatives considered: `GET /api/tags?q=` を足して `search_key` で絞る（打鍵ごとの往復になり、共有の
  保持の全件と一覧が別物になる。R-1 と相反する）。`search_key` を `Tag` に載せて画面に渡す（名前と
  シノニムの数だけ応答が太り、検索語の側の照合形は画面で作る必要が残る）。
- 既知の差: Go の `unicode.ToLower` は 1 符号位置を 1 符号位置に写し、JavaScript の `toLowerCase` は
  特殊な大文字化（U+0130 など）で複数の符号位置を返すことがある。タグ名の照合は部分一致なので、この差で
  見つからなくなる組は組み合わせの試験に含めず、試験の組は両方で同じ結果になるものに限る。
- 範囲: 変えるのはタグ管理画面の一覧の検索だけ。「タグを付ける」「別のタグへ統合…」などの候補の照合
  （`web/src/library/tagChoices.tsx`・`MergeTagDialog.tsx` の `toLowerCase().includes`）は親 Issue の要件に
  無いので変えない。同じ規則にそろえるのは別の Issue で扱う。

## R-4: まとめての確定・却下・削除は 1 つの経路 `POST /api/tags/batch` が 1 つの取引で受け、働かない・無いタグは数えて飛ばす

- Decision: `{ action: confirm | reject | delete, ids }` を受け、`ids` のうち今あるタグで、その操作が働く
  種類（確定・却下は仮のタグ、削除は確定したタグ）だけを 1 つの取引で処理する。応答は処理した id、
  無かった id、種類が合わず飛ばした id の 3 つの配列
  （[contracts/screen-api.md §1](contracts/screen-api.md#1-post-apitagsbatch)、
  [data-model.md §2](data-model.md#2-保存層の操作)）。1 件の経路（`POST /api/tags/{id}/confirm` など）は
  今のまま残し、行の操作はそれを使い続ける（要件 7「今の 1 行ずつの操作の規則は変えない」）。
- Rationale: 仮のタグ 100 個の確定が 100 回の要求と 100 回の取り直しになるのが、親 Issue の「その都度
  固まる」の正体である。1 つの取引なら要求は 1 回、画面の反映も 1 回で済み、受け入れ条件 9 の
  「固まらない」に届く。Edge Case「働かない種類を含めて選んで実行したとき」「別のタブで対象の一部が
  消えたとき」は、どちらも「残りは処理し、数を伝える」なので、全部か無しかにせず、飛ばした id を
  理由別に返す。取引が失敗したときは何も反映されないので、「済んだ分と済まなかった分が分かり、
  済まなかった分は選んだまま残る」は、画面が応答の `appliedIds` だけを選択から外すことで満たす。
- Alternatives considered: 操作ごとに別の経路（`/api/tags/confirm`・`/reject`・`/delete`。本文の形と
  応答が同じで、分ける理由が無い）。画面が 1 件の経路を順に呼ぶ（要求の数が選んだ数になり、途中の
  失敗で残りの扱いが画面の都合になる。固まる原因そのもの）。無い id を `404` で全体を失敗にする
  （Edge Case が「残りは処理する」と求める）。
- 上限: `ids` は 1 件以上 20,000 件以下で、`POST /api/video-tags` の `videoIds` と同じ理由（本文の
  上限 1 MiB に収まり、`json_each` に 1 つの引数で渡す）。超えたら `400` の reason `too_many_tags`
  （`limit` 付き）。画面は見えているタグが上限を超えるときはまとめての操作を disabled にする
  （選択バーの `overLimit` と同じ扱い）。

## R-5: 統合は `POST /api/tags/{id}/merge` の本文を `sourceIds`（1 件以上）にし、1 件の統合もこれを使う

- Decision: `MergeTagRequest` を `{ sourceIds: int64[] }` にし、`sourceId` は廃止する。応答は
  `{ tag: Tag, notFoundIds: int64[] }`。統合先 `{id}` が無ければ `404 tag_not_found`、`sourceIds` に
  `{id}` が含まれれば `400`（reason `merge_same_tag`、今と同じ）。無い統合元は飛ばして `notFoundIds` に
  載せ、残りを 1 つの取引で統合する（[contracts/screen-api.md §2](contracts/screen-api.md#2-post-apitagsidmerge-の変更)）。
- Rationale: 統合の取引（付与の写し、名前の移動、統合元の削除、統合先の確定）は統合元の数だけ
  繰り返すだけで、1 件と複数で違う形を持つ理由が無い。`api/openapi.yaml` は画面との契約で、呼び手は
  `web/src/api/tags.ts` の `mergeTag` の 1 か所である。外部連携 API（`api/external-v1.yaml`）に統合は無い。
- Alternatives considered: `sourceId` を残して `sourceIds` を任意で足す（どちらか一方が要る形は
  `oneOf` の生成物の扱いを増やし、[014 の contracts/tags-api.md §1](../014-video-tags/contracts/tags-api.md#1-スキーマ)
  が避けた形になる）。まとめての統合だけ別の経路（同じ取引が 2 か所になる）。
- 統合先が統合元の中にあるときは画面が統合元から外す（Edge Case）。サーバーは今と同じく `400` で
  受け付けない。

## R-6: 確認に出す「影響を受ける動画の本数」は `POST /api/tags/impact` が重複を除いて数える

- Decision: `{ ids }` を受け、`ids` のうち今あるタグの数と、そのどれかが付いた（手で付けた分とフォルダ名
  から付いている分のどちらか。[014 の data-model.md §5](../014-video-tags/data-model.md#5-本数の数え方)）
  いまライブラリにある動画の本数を、動画の `id` で重複を除いて返す
  （[contracts/screen-api.md §3](contracts/screen-api.md#3-post-apitagsimpact)）。
- Rationale: 要件 10・受け入れ条件 10・11 は「重複を数えない」本数を求める。画面が持つ `videoCount` の
  和では同じ動画が 2 つのタグに付いていれば 2 と数える。1 件の削除・統合の確認は今までどおり
  `videoCount` を使う（014 の契約の「確認のための経路は足さない」は 1 件の話で、まとめての確認は
  この feature が足す要求である）。
- Alternatives considered: `videoCount` の和（重複を数える。要求に合わない）。`POST /api/tags/batch` に
  `dryRun` を足す（読みと書きを 1 つの経路に混ぜる。読みの経路は `POST /api/video-tags/summary` の
  前例がある）。

## R-7: 並び順はこの画面の端末の設定として `localStorage` に持ち、絞り込みは今までどおり画面の状態に留める

- Decision: `web/src/preferences/tagListPreferences.ts` に `readTagListPreferences`・`writeTagListPreferences`
  を置き、並び順（6 値）だけを保存する。`viewPreferences.ts` と同じ総関数（投げない、壊れていれば既定の
  「名前」の順）。「Tentative only」と「0 本のみ」と検索語は保存せず URL にも載せない。
- Rationale: 要件 4 は並び順だけに「画面を離れてもブラウザを開き直しても残る」と求める。URL ではなく
  端末の設定にするのは、ライブラリの並び順も `viewPreferences` で端末に残していて、管理画面の URL は
  共有する用途を持たないからである。絞り込みを保存しないのは、
  [031 の research.md R-8](../031-tentative-tags/research.md#r-8-仮のタグだけの絞り込みと却下した名前の一覧は画面の側で持つ)
  が「Tentative only」を画面の状態に留めた判断を変える理由が無く、0 本の絞り込みも同じ性質だからである。
- Alternatives considered: URL のクエリ（ライブラリと同じ形だが、戻る・進むで並び順が変わる体験に
  なり、ブラウザを開き直すと消える）。サーバーの `settings` 表（端末ごとの見え方の好みで、
  サーバーの設定にする理由が無い）。
- 既定と保存する値: `name`（既定）、`countDesc`、`countAsc`、`createdDesc`、`createdAsc`。「名前」に
  向きは無い（今の並びと同じ）。同値は名前の順（`compareTagRefs`）で決める。

## R-8: 「作った日」は `tags.created_at` を `Tag.createdAt` として載せ、同じ秒のタグは名前の順にする

- Decision: `GET /api/tags` と `Tag` を返すすべての応答に `createdAt`（`date-time`）を足す。値は今ある
  `tags.created_at`（Unix 秒）で、移行も列の変更も無い。「作った日」の並びは `createdAt` で比べ、
  同じならほかの並びと同じく名前の順にする（要件 3）。
- Rationale: 列はすでにあり、`insertTag` が書いている。秒より細かくするには列の意味を変えることになり、
  既存の行と混ざる。外部連携 API の一括付与は 1 つの取引で複数のタグを作るので同じ秒に並ぶが、
  そのときの並びは要件 3 が「名前の順」と決めている。
- Alternatives considered: 同じ秒は `id` の降順（作られた順）にする（要件 3 の「値が同じタグどうしは
  名前の順」と食い違う）。`created_at` をミリ秒にする（既存の行の値と単位が混ざり、移行で写しても
  精度は戻らない）。
- 外部連携 API（`api/external-v1.yaml` の `listTags`）の応答は変えない。`domain.Tag` の欄が増えるだけで、
  外部の型への写しは `internal/httpapi/external.go` が明示的に行っている。

## R-9: 受け入れ条件の計測は、作り置きの規模のデータを `scripts/tagsbench` が作り、Playwright の計測スクリプトが本番ビルドに対して測る

- Decision: `scripts/tagsbench`（Go）が `.local/tagsbench/<規模>/` にデータディレクトリを作る。
  `internal/store` の役割の型（`SettingsStore.AddMediaFolder`、`ScanIndexStore.UpsertVideo`、
  `TagStore.ApplyVideoTags`）で、親 Issue の 2 つの規模（タグ 1,000 個・動画 10,000 本、タグ 3,000 個・
  動画 30,000 本）の行を書く。動画のファイルは作らず、登録フォルダの下の所在の行だけを書く（一覧・
  本数・検索は所在の行から決まり、ファイルは読まない）。同じプログラムがビルド済みの単一バイナリを
  そのデータで起動し、`web/bench/tags-admin.bench.ts`（Playwright、`web/e2e/` の試験とは別の設定で、
  `task test-e2e` と CI には入れない）が、受け入れ条件 1〜3 の場面を測って表に出す。手順は
  `docs/how-to/tags-admin-benchmark.md` に書き、PR の本文に結果を残す
  （[quickstart.md](quickstart.md)）。
- Rationale: 受け入れ条件は本番ビルドをヘッドレス Chromium から測るよう求めていて、数値（1 秒・
  0.2 秒・50ms）は規模のデータが無いと確かめられない。`docs/how-to/preview-benchmark.md` の
  `scripts/previewbench` と同じく、測る対象は本番のコードそのもので、計測の仕掛けを製品に入れない。
  店（store）の公開の操作だけで行を書くのは、`SQL` を `internal/store` の外に出さないため
  （ARCHITECTURE.md「`store.DB` does not hand out its `*sql.DB`」）。
- Alternatives considered: `web/e2e/` に入れて `task test-e2e` で測る（30,000 本の投入と計測で e2e が
  数分伸び、数値のばらつきで CI が不安定になる）。実ファイルを生成して取り込む（30,000 本の ffprobe で
  規模の準備に時間がかかり、測りたいのは管理画面であって取り込みではない）。Vitest の jsdom で測る
  （描画の時間が実ブラウザと違い、受け入れ条件の数値に意味が無い）。
