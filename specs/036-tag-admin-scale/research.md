# Research: タグ管理画面を数千〜数万個のタグに耐えさせる

技術スタック、境界と依存方向、タグの表と名前の規則、仮のタグと却下した名前、画面の文言と書式、
一覧画面の見た目の規則は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、
[ARCHITECTURE.md](../../ARCHITECTURE.md)、
[specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
[specs/031-tentative-tags/data-model.md](../031-tentative-tags/data-model.md)、
[docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)、
[docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)）。
ここにはこの feature が足す決定だけを書く。

親 Issue #651 の改訂（画面は表示に要る分だけを読み、検索・並び順・絞り込みは読み込んでいない
タグも含めた全部に効く。規模にタグ 30,000 個を足す）で、R-1 は置き換え、R-9 は規模と場面を足し、
R-3・R-7・R-12 は役割を書き直した。R-10 以降がこの改訂で足した決定である。R-2・R-4〜R-6・R-8 は
feature branch に merge 済みの実装のとおりで変わらない。

## R-1: 一覧はサーバーのページで受け、検索・絞り込み・並び順はサーバーが全部のタグに掛ける

- Decision: `GET /api/tags` に `q`・`tentative`・`unused`・`sort`・`cursor`・`limit` を足し、画面は
  開いたときに 1 ページ（`limit` 件）だけを受け、スクロールに合わせて `nextCursor` で続きを受ける
  （[contracts/screen-api.md §5](contracts/screen-api.md#5-get-apitags-のパラメータ)）。検索・絞り込み・
  並び順は要求のパラメータで、サーバーが全部のタグに掛けて 1 ページを返す。ページは `total`
  （条件に合う数）と `totalAll`（全部の数）を持ち、件数の行はこれを出す（受け入れ条件 8）。
  カーソルは `GET /api/library` と同じ keyset 方式（並び順の値・名前の鍵・`id`）で、別の並び順で
  作ったカーソルは `400`（`ErrInvalidCursor`。[013 の list-api.md §5](../013-library-search/contracts/list-api.md#5-カーソルと誤り)）。
  `limit` を省いた要求は今までどおり全件を返し、候補（combobox）・絞り込みの確かめが使う共有の保持
  （`web/src/api/tags.ts` の `getTags`）はこの形のまま使い続ける。
- Rationale: 改訂した要件 2 は「最初に表示に要る分のタグだけを読み込み、タグの総数が増えても開くまでの
  待ちと最初に読み込む量は増えない」と求め、受け入れ条件 2 は 1,000・3,000・30,000 個で受け取る数と
  転送量が同じであることを測る。全件を 1 回で受けて画面で絞る前の R-1 では、描画は仮想化で解けても
  転送と JSON の解釈は総数に比例し、30,000 個では開くまでの待ちが規模で増える。要件 4・6・7 と
  受け入れ条件 8・9 は読み込んでいないタグにも並び順・絞り込み・検索が効くことを求めるので、
  条件はサーバーに渡すほかない。既存の経路に加えるのは、候補と絞り込みの確かめが同じタグの一覧を
  全件で要し（親 Issue の「対象外」。#674・#675 が扱う）、別の経路にすると同じ応答の形が 2 つに
  なるためである。
- Alternatives considered: 全件を 1 回で受けて画面で絞る（改訂前の R-1。要件 2・受け入れ条件 2 と
  相反する）。`GET /api/tags/page` など別の経路（応答の形と絞り込みの規則が 2 つの経路に分かれ、
  #674 が候補を同じ条件で引くときに経路を足し直すことになる）。offset ページング（`?page=N`。
  続きを読む間に別のタブでタグが増減すると行が二重に出る・抜ける。Edge Case「続きを読むあいだに
  別のタブで…」が禁じ、keyset はライブラリに前例がある）。
- 並びの決着: どの並び順でも、値が同じタグは名前の自然順（R-10 の `sort_key`）、それも同じなら
  `id` で決める（要件 4「値が同じタグどうしは名前の順」。`SortTagRefs` と同じ決着）。本数と作った日の
  並びは向きが名前と逆になりうるので、カーソルの条件は行値の比較 1 つではなく「値が前 or（値が同じ
  and（鍵, id）が後）」の形で書く（[data-model.md §2](data-model.md#2-保存層の操作)）。
- 検索の規則: `q` は `domain.FoldForMatch` を掛けて前後の空白を落とし、空でなければ
  `tag_names.search_key`（元の名前とシノニムの両方の行）への `instr` で照らす
  （[014 の data-model.md §7](../014-video-tags/data-model.md#7-検索欄でのタグ名の照合) と同じ照合形。
  要件 7）。語の分解（AND・OR・除外）は入れない。改訂前の画面の検索も 1 つの語の部分一致で、親 Issue
  が求めるのは「同じ規則で照らし合わせる」（照合形）であって検索式ではない。`q` の上限は
  `GET /api/library` の `query` と同じ 100 文字。
- 本数の数え方: 本数は今の `taggedVideosSQL` の集計（[014 の data-model.md §5](../014-video-tags/data-model.md#5-本数の数え方)）を
  1 回の問い合わせの中で全タグについて数え、並び順（本数）と絞り込み（0 本）とページの本数に使う。
  改訂前の全件の一覧も同じ集計を要求ごとに 1 回行っていたので、集計の費用は新しくない。新しいのは
  ページごとに集計が走ることで、30,000 個の規模の応答時間は quickstart.md が `GET /api/tags` の応答を
  別に出して確かめる。集計が 1 秒を占めるなら本数の持ち方（サーバーの側）を変えることになり、それは
  親 Issue の「対象外」が #674 に渡した領域である。

## R-2: 行の仮想化は `@tanstack/react-virtual` の `useWindowVirtualizer` で行う

- Decision: `web/package.json` に `@tanstack/react-virtual` を足し、タグの一覧だけで使う。スクロールの
  持ち主は今までどおり window（文書）で、行の高さは描いた要素を測って持つ（`measureElement`）。
  ページで読む形（R-1）になっても変えない。
- Rationale: 行の高さは一様でない。シノニムの行を持つタグは 1 行高く、改名中の行は失敗の文言で伸び、
  作成の行が先頭に入る。自前で窓を切るには高さの計測と累積の管理を書くことになり、そこが不具合の
  置き場になる。この依存は実行時の依存を持たず、Renovate の運用（[docs/how-to/dependency-updates.md](../../docs/how-to/dependency-updates.md)）
  に乗る。[library-ui.md §3](../../docs/design-docs/library-ui.md#3-なぜ仮想スクロールを使っていないのか) が
  仮想スクロールを避けた理由は「折り返す格子で 1 行の枚数が幅で変わる」「60 件ずつしか読まない」の
  2 つで、1 列のこの一覧には前者が当たらない。後者は R-1 でページ読みになっても当たらない: 受け入れ
  条件 4 は 30,000 個の一覧を末尾まで続きを読み込みながらスクロールすることを求め、読み込んだ行は
  30,000 行まで増える。読み込んだ分を全部描けば、改訂前に測った描画の遅さがそのまま戻る。
- Alternatives considered: 自前の窓切り（高さを 2 種類に固定すれば累積は足し算で済むが、改名の失敗の
  文言と作成の行で崩れる。崩さないために行の形を変えるのは、実装の都合で設計を狭めることになる）。
  行の高さを 1 種類に固定してシノニムを名前の行に詰める（[014 の ui-design.md「Rows」](../014-video-tags/ui-design.md#rows)
  の情報階層を変える。要求にその理由が無い）。`react-window`（固定高か高さの関数を要し、計測は自前になる）。
  ページで読むなら仮想化を外す（上の Rationale のとおり、読み込んだ行の数は総数まで増える）。
- 文書: `docs/design-docs/library-ui.md` §3 に、タグ管理画面の一覧がこれを使うことと、格子の一覧が
  今も使わない理由はそのままであることを書く（merge 済み）。R-1 の改訂で「全件を 1 回で受けて持って
  いる」の記述は「ページで受け、読み込んだ行が総数まで増える」に直す。

## R-3: 照合形 `FoldForMatch` の TypeScript 移植は、画面が読み込んだ行をその場で判定するために使う

- Decision: `web/src/lib/foldForMatch.ts`（merge 済み。NFKC → 符号位置ごとの小文字化 → ひらがなから
  カタカナ）と、Go と Vitest が共に読む `internal/domain/testdata/fold_for_match.json` は残す。役割は
  変わる: 一覧の検索はサーバーが `search_key` で行う（R-1）ので、画面は検索語の照合には使わない。
  使うのは、作成・改名・確定・却下のあとにその行が今の条件（検索語・「Tentative only」・「Unused
  only」）にまだ合うかを、サーバーへ往復せずに決めるとき（R-12）である。
- Rationale: 改訂前は画面が全件を照らすための移植だった。ページ読みになっても、操作のあとの 1 行の
  判定を往復で行うと、行の操作のたびに一覧の取り直しが要り、改訂前の「その都度固まる」の原因の 1 つ
  （操作ごとの取り直し）に戻る。サーバーと同じ照合形を画面が持っていれば、1 行の判定は同期で済む。
  2 言語に同じ規則を持つ代わりに、同じ入力の組で食い違いを機械で見つける仕組みも merge 済みである。
- Alternatives considered: 移植を捨てて操作のたびに一覧を取り直す（上の Rationale）。移植を捨てて
  操作した行は条件に合わなくても残す（検索中に改名して一致しなくなった行が、一覧を取り直すまで
  並び続け、サーバーの一覧と食い違う）。
- 小文字化と残る差（版の違いで Node に無い符号位置の組を既知の例外として除いていること）は
  merge 済みの実装と試験のとおりで、ここでは決め直さない。

## R-4: まとめての確定・却下・削除は 1 つの経路 `POST /api/tags/batch` が 1 つの取引で受け、働かない・無いタグは数えて飛ばす

- Decision: `{ action: confirm | reject | delete, ids }` を受け、`ids` のうち今あるタグで、その操作が働く
  種類（確定・却下は仮のタグ、削除は確定したタグ）だけを 1 つの取引で処理する。応答は処理した id、
  無かった id、種類が合わず飛ばした id の 3 つの配列
  （[contracts/screen-api.md §1](contracts/screen-api.md#1-post-apitagsbatch)、
  [data-model.md §2](data-model.md#2-保存層の操作)）。1 件の経路（`POST /api/tags/{id}/confirm` など）は
  今のまま残し、行の操作はそれを使い続ける（要件 8「今の 1 行ずつの操作の規則は変えない」）。
- Rationale: 仮のタグ 100 個の確定が 100 回の要求と 100 回の取り直しになるのが、親 Issue の「その都度
  固まる」の正体である。1 つの取引なら要求は 1 回、画面の反映も 1 回で済み、受け入れ条件 10 の
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
  （`limit` 付き）。上限は送る id の数に掛かるので、画面は読み込んだ行が上限を超えるときは
  「読み込んだものをすべて選ぶ」（先頭のチェック）だけを disabled にし、まとめての操作は選択の数が
  上限を超えるときだけ disabled にする（要件 10 の「読み込んである行」が対象なので、30,000 個を
  末尾まで読んだ一覧では「すべて選ぶ」は押せなくなるが、1 行ずつ選んだ選択は要件 8・9 のとおり
  操作できる。読み込んだ行の数でまとめての操作まで止めると、末尾近くまでスクロールしただけで
  数行の選択も操作できなくなる）。

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
- 統合元の数によらない文の数で行う: 1 件の `mergeTagInto` を統合元ごとに呼ぶと、そのたびに統合先の
  `tagByID`（本数の数え直しと、伸び続けるシノニムの読み）が走り、20,000 個の統合では書きの取引を長く
  握る。統合元の集合を `json_each` で渡し、付与の写し・名前の移動・統合元の削除をそれぞれ 1 文で行い、
  統合先は最後に 1 回だけ読む（[data-model.md §2](data-model.md#2-保存層の操作) の `mergeTagsInto`）。
- 統合先が統合元の中にあるときは画面が統合元から外す（Edge Case）。サーバーは今と同じく `400` で
  受け付けない。

## R-6: 確認に出す「影響を受ける動画の本数」は `POST /api/tags/impact` が重複を除いて数える

- Decision: `{ action, ids }` を受け、`ids` のうち今あり、`action`（`reject`・`delete`・`merge`）が働く
  タグの数と、そのどれかが付いた（手で付けた分とフォルダ名
  から付いている分のどちらか。[014 の data-model.md §5](../014-video-tags/data-model.md#5-本数の数え方)）
  いまライブラリにある動画の本数を、動画の `id` で重複を除いて返す
  （[contracts/screen-api.md §3](contracts/screen-api.md#3-post-apitagsimpact)）。
- Rationale: 要件 11・受け入れ条件 11・12 は「重複を数えない」本数を求める。画面が持つ `videoCount` の
  和では同じ動画が 2 つのタグに付いていれば 2 と数える。1 件の削除・統合の確認は今までどおり
  `videoCount` を使う（014 の契約の「確認のための経路は足さない」は 1 件の話で、まとめての確認は
  この feature が足す要求である）。
- `action` を受ける理由: まとめての却下・削除は仮と確定を混ぜて選べ（要件 8）、`POST /api/tags/batch` は
  働かない種類を飛ばす（R-4）。`ids` 全部を数えると、例えば 100 本に付いた仮のタグと 1 本に付いた確定した
  タグを選んで削除するとき、実際に消えるのは確定したタグだけなのに確認は 101 本と出る。確認の数は実行で
  変わるものだけにするため、処理と同じ `TagBatchApplies` の規則で数える対象を決める
  （[data-model.md §1](data-model.md#1-domain-に足す値) の `TagImpactApplies`）。
- Alternatives considered: `videoCount` の和（重複を数える。要求に合わない）。種類ごとに数を分けて返し、
  画面が選ぶ（画面が規則をもう 1 つ持つことになり、サーバーの処理と食い違いうる）。`POST /api/tags/batch` に
  `dryRun` を足す（読みと書きを 1 つの経路に混ぜる。読みの経路は `POST /api/video-tags/summary` の
  前例がある）。

## R-7: 並び順はこの画面の端末の設定として `localStorage` に持ち、絞り込みは今までどおり画面の状態に留める

- Decision: `web/src/preferences/tagListPreferences.ts` の `readTagListPreferences`・`writeTagListPreferences`
  （merge 済み）が並び順だけを保存する。`viewPreferences.ts` と同じ総関数（投げない、壊れていれば既定の
  「名前」の順）。「Tentative only」と「Unused only」と検索語は保存せず URL にも載せない。並び順の値は
  API の `TagSort`（`name`・`countDesc`・`countAsc`・`createdDesc`・`createdAsc`）と同じ 5 値で、画面は
  読んだ値をそのまま `sort` パラメータに送る（R-1）。
- Rationale: 要件 5 は並び順だけに「画面を離れてもブラウザを開き直しても残る」と求める。URL ではなく
  端末の設定にするのは、ライブラリの並び順も `viewPreferences` で端末に残していて、管理画面の URL は
  共有する用途を持たないからである。絞り込みを保存しないのは、
  [031 の research.md R-8](../031-tentative-tags/research.md#r-8-仮のタグだけの絞り込みと却下した名前の一覧は画面の側で持つ)
  が「Tentative only」を画面の状態に留めた判断を変える理由が無く、0 本の絞り込みも同じ性質だからである。
  端末の設定の値と API の値を同じにするのは、並び替えがサーバーへ移った（R-1）ことで、画面に並び順の
  写しの表を持つ理由が無くなったためである。
- Alternatives considered: URL のクエリ（ライブラリと同じ形だが、戻る・進むで並び順が変わる体験に
  なり、ブラウザを開き直すと消える）。サーバーの `settings` 表（端末ごとの見え方の好みで、
  サーバーの設定にする理由が無い）。
- 既定と保存する値: `name`（既定）、`countDesc`、`countAsc`、`createdDesc`、`createdAsc`。「名前」に
  向きは無い（今の並びと同じ）。同値の決着は R-1 のとおりサーバーが行う。

## R-8: 「作った日」は `tags.created_at` を `Tag.createdAt` として載せ、同じ秒のタグは名前の順にする

- Decision: `GET /api/tags` と `Tag` を返すすべての応答に `createdAt`（`date-time`）を足す（merge 済み）。
  値は今ある `tags.created_at`（Unix 秒）で、移行も列の変更も無い。「作った日」の並びは `created_at` で
  比べ、同じならほかの並びと同じく名前の順にする（要件 4）。
- Rationale: 列はすでにあり、`insertTag` が書いている。秒より細かくするには列の意味を変えることになり、
  既存の行と混ざる。外部連携 API の一括付与は 1 つの取引で複数のタグを作るので同じ秒に並ぶが、
  そのときの並びは要件 4 が「名前の順」と決めている。
- Alternatives considered: 同じ秒は `id` の降順（作られた順）にする（要件 4 の「値が同じタグどうしは
  名前の順」と食い違う）。`created_at` をミリ秒にする（既存の行の値と単位が混ざり、移行で写しても
  精度は戻らない）。
- 外部連携 API（`api/external-v1.yaml` の `listTags`）の応答は変えない。`domain.Tag` の欄が増えるだけで、
  外部の型への写しは `internal/httpapi/external.go` が明示的に行っている。

## R-9: 受け入れ条件の計測は、作り置きの規模のデータを `scripts/tagsbench` が作り、Playwright の計測スクリプトが本番ビルドに対して測る

- Decision: `scripts/tagsbench`（Go。merge 済み）が `.local/tagsbench/<規模>/` にデータディレクトリを
  作る。`internal/store` の役割の型（`SettingsStore.AddMediaFolder`、`ScanIndexStore.UpsertVideo`、
  `TagStore.ApplyVideoTags`）で親 Issue の規模の行を書く。動画のファイルは作らず、登録フォルダの下の
  所在の行だけを書く。同じプログラムがビルド済みの単一バイナリをそのデータで起動し、
  `web/bench/tags-admin.bench.ts`（Playwright、`web/e2e/` の試験とは別の設定で、`task test-e2e` と CI
  には入れない）が場面を測って表に出す。手順は `docs/how-to/tags-admin-benchmark.md` に書き、PR の
  本文に結果を残す（[quickstart.md](quickstart.md)）。
- 改訂で足す規模と場面: 規模にタグ 30,000 個を足す。その動画の数は 30,000 本（タグ 3,000 個の規模と
  同じ）にし、`-videos N` で動画の数を規模から切り離す（省けば今までどおり 10 倍）。場面に、開いた
  ときに `GET /api/tags` が返した件数と応答の大きさ（受け入れ条件 2）と、30,000 個の一覧を末尾まで
  続きを読み込みながらスクロールする間のフレーム時間（受け入れ条件 4）を足す。
- Rationale: 受け入れ条件は本番ビルドをヘッドレス Chromium から測るよう求めていて、数値（1 秒・
  0.2 秒・50ms・同じ転送量）は規模のデータが無いと確かめられない。`docs/how-to/preview-benchmark.md` の
  `scripts/previewbench` と同じく、測る対象は本番のコードそのもので、計測の仕掛けを製品に入れない。
  店（store）の公開の操作だけで行を書くのは、`SQL` を `internal/store` の外に出さないため
  （ARCHITECTURE.md「`store.DB` does not hand out its `*sql.DB`」）。30,000 個の規模の動画を 10 倍の
  300,000 本にしないのは、親 Issue がその規模に動画の数を添えておらず（表の見出しは「タグ 30,000 個の
  ライブラリ」）、測りたいのがタグの数による差だからである。動画を 3,000 個の規模と同じにすれば、
  3,000 個と 30,000 個の違いはタグの数だけになる。
- Alternatives considered: `web/e2e/` に入れて `task test-e2e` で測る（30,000 本の投入と計測で e2e が
  数分伸び、数値のばらつきで CI が不安定になる）。実ファイルを生成して取り込む（ffprobe で
  規模の準備に時間がかかり、測りたいのは管理画面であって取り込みではない）。Vitest の jsdom で測る
  （描画の時間が実ブラウザと違い、受け入れ条件の数値に意味が無い）。30,000 個の規模も動画を 10 倍に
  する（規模のデータの作成が 1 時間を超え、親 Issue の表に無い動画の数を測ることになる）。

## R-10: 名前の自然順の鍵 `sort_key` を `tag_names` と `rejected_tag_names` に持ち、起動時の鍵の埋め直しで作る

- Decision: `tag_names` と `rejected_tag_names` に `sort_key text not null default ''` を足し、
  `domain.NaturalSortKey(name)`（`FoldForMatch` を掛けたうえで数字の連続を桁数付きに置き換えた、
  バイト順で自然順になる鍵。[013 の data-model.md §4](../013-library-search/data-model.md#4-title_key-の規則)）
  を名前の行を書くとき（作成・シノニム登録・付与での作成・改名・却下）に同じ取引で書く。既存の行は、
  移行が `search_version` を 0 に戻し（`rejected_tag_names` には `search_version` も足す）、起動時の
  `TagStore.RefreshSearchKeys` が `search_key` と一緒に埋める（[014 の data-model.md §7](../014-video-tags/data-model.md#7-検索欄でのタグ名の照合)
  と同じ時点）。サーバーの名前の順はこの鍵のバイト順、同じなら `id`（[data-model.md §0](data-model.md#0-マイグレーション)）。
- Rationale: R-1 の keyset カーソルは「名前の順で次の行」を SQL で引く必要があり、`CompareNatural`
  （Go の関数）では引けない。ライブラリの題名順が `video_locations.title_key` に同じ鍵を持って
  `order by` と カーソルに使っている前例（`listing.go` の `sortText`）に乗る。却下した名前にも足すのは、
  要件 12 が却下した名前の一覧も表示に要る分だけ読むことを求め、その並びが名前の自然順
  （[031 の contracts/screen-api.md §3](../031-tentative-tags/contracts/screen-api.md#3-却下した名前)）だから
  である。起動時の埋め直しに乗せるのは、SQL では `NaturalSortKey` を計算できず、鍵の規則の版と作り直しの
  仕組みがすでにあるためである。
- Alternatives considered: `order by name collate nocase`（大文字小文字しか同一視せず、`2` と `10` の
  数値の順にならない。今の一覧の自然順から後退する）。鍵を保存せず Go で全件を並べてから `limit` を
  切る（毎ページ全件を読んで並べることになり、ページで読む意味が無い）。`search_key` を鍵に流用する
  （照合形は数字の連続を桁数付きにしないので、`tag 2` が `tag 10` の後ろに来る）。別の版の列
  `sort_version` を足す（`search_version` の意味は「鍵の規則の版」で、鍵が 2 つになっても版は 1 つで
  足りる）。
- 名前の順の定義が変わる点: 改訂前の `ListTags` は Go の `SortTags`（`CompareNatural` を元の名前に掛け、
  同順位は元の文字列）で並べていた。`NaturalSortKey` は照合形（全角・半角・かなを同一視）に掛けるので、
  `アニメ` と `あにめ` のように照合形が同じ名前の前後が `id` で決まるようになる。ライブラリの題名順と
  同じ定義にそろうので、契約の「名前の自然順」の文言は変えない。

## R-11: 続きは画面の末尾に近づいたら 100 件ずつ読み、`id` で重複を捨て、件数が食い違えば知らせて取り直させる

- Decision: 画面は 1 ページ 100 件（`limit=100`。`GET /api/library` の 60 件より多いのは、行が 1 列で
  軽く、1 画面に 12 行以上出るため）で、仮想化が描く最後の行が読み込んだ行の末尾から数行以内に
  入ったら `nextCursor` で次を 1 回だけ要求する（同時に 2 つの続きは送らない）。届いた行は `id` で
  重複を捨てて末尾に足す（`videosData.ts` の `appendUnique` と同じ）。続きの応答の `totalAll` が画面の
  持つ値と違えば、行はそのまま残して「一覧が変わった」の 1 行と「取り直す」を出し、続きの読み込みは
  止める（`videosData.ts` の `inconsistent` と同じ形）。続きの読み込みに失敗したら、読み込んだ行は残し、
  末尾に失敗の 1 行と「Retry」を出して同じカーソルで読み直せる。検索・絞り込み・並び順を変えたら、
  進行中の要求を `AbortController` で打ち切り、世代の番号で古い応答を捨て、先頭から読み直す
  （[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。
- Rationale: Edge Case「続きを読むあいだに別のタブでタグが増えたり消えたりしたとき: 同じタグが二重に
  出たり、行が抜けたまま気づけなかったりしない」の前半は keyset と `id` の重複捨てで、後半は
  `totalAll` の食い違いの知らせで満たす。keyset は「カーソルより前に並ぶようになった行」を続きに
  出さないので、抜けそのものは防げない（[013 の list-api.md §5](../013-library-search/contracts/list-api.md#5-カーソルと誤り)
  が同じ範囲の保証）。黙って取り直すと、選んでいる行とスクロール位置を失う。Edge Case「続きを読んで
  いる最中に検索・絞り込み・並び順を変えたとき: 古い条件の続きは一覧に混ざらない」は、打ち切りと
  世代の番号で満たす（`web/src/api/tags.ts` の `generation` と同じ考え）。
- Alternatives considered: `IntersectionObserver` の番兵（ライブラリの形。仮想化の一覧では番兵が
  描かれる位置の制御が仮想化の外に出るので、仮想化が持つ「描いた最後の行の位置」を使う方が短い）。
  食い違いを見つけたら黙って先頭から読み直す（選択とスクロール位置を失う。`useScanIssues` のように
  読んだ分まで読み直す形は、数千行を読み直すことになる）。`total` でも食い違いを見る（`total` は自分の
  操作でも変わり、画面が局所で数え直す値なので、他のタブの変化の印には `totalAll` の方が確かである。
  自分の操作も `totalAll` を局所で数え直す）。ページを 200 件にする（1 ページの応答が 3 倍近くなり、
  受け入れ条件 2 の「最初に読み込む量」を無駄に増やす）。

## R-12: 操作のあとの反映は読み込んだ行の中で行い、並びの位置は `NaturalSortKey` の移植で決める

- Decision: 1 件とまとめての操作のあと、画面はサーバーの一覧を取り直さず、読み込んだ行をその場で
  書き換える。確定は行の `tentative` を偽に、却下・削除・統合元は行を取り除き、統合先は応答の `tag` に
  差し替えて（読み込んでいない統合先は差し込んで）並びの位置を直し、改名は行の名前を差し替えて並びの
  位置を直す。位置が読み込んだ範囲の外（最後の行より後ろで `nextCursor` がある）になる行は `rows` に
  置かず、続きのページに任せる。keyset の続きは最後の行の鍵より後ろだけを返すので、範囲の外の行を
  残すと続きと二重になり、範囲の中へ動いた行（`countDesc` で本数が増えた、読み込んでいない統合先）を
  画面が置かないと、取り直すまで一覧から抜けたまま件数にだけ数えられる。作成した行は、今の条件に合えば（R-3 の
  `foldForMatch` で検索語に、`tentative`・`videoCount` で絞り込みに照らす）並びの位置に差し込み、
  合わなければ入れない。`total`・`totalAll` は局所で増減する。並びの位置は `web/src/lib/naturalSortKey.ts`
  （`NaturalSortKey` の移植。`foldForMatch` の上に数字の連続の置き換えを足す）で作った鍵を符号位置の
  順で比べ、本数・作った日の並びでは先にその値で比べる。`notFoundIds` を受けたときだけ先頭から
  取り直す（R-4）。共有の保持（`web/src/api/tags.ts`）の `afterTagChanged` は、購読者がいるときだけ
  取り直し、いなければ `held` を捨てて次の `getTags` に取らせる。
- Rationale: 操作のたびに先頭から取り直すと、末尾まで読んだ 30,000 行が 1 ページに戻り、スクロール位置と
  選択を失う。読んだ分を読み直す形は数千行の往復になる。読み込んだ行の中で書き換えれば往復は 0 で、
  受け入れ条件 3・10 の「固まらない」にも届く。位置をサーバーと同じ鍵で決めるのは、取り直したときに
  行が別の場所へ動かないためで、`compareNatural`（元の名前に掛ける）では R-10 の鍵と前後が食い違う。
  共有の保持の取り直しを購読者がいるときに限るのは、タグ管理画面には購読者が無く、操作のたびに
  30,000 個の全件を取り直すと転送量が総数に比例して戻ってしまうからである（要件 2・3。全件の
  JSON の解釈は 0.2 秒の長いタスクになりうる）。
- Alternatives considered: 操作のたびに先頭から取り直す（上の Rationale）。`compareNatural` で位置を
  決める（照合形が同じ名前の前後がサーバーと違う）。鍵を比べずに作成・改名した行を先頭に置く
  （「作った日」の新しい順では正しいが、名前の順では取り直すまで並びが崩れる）。共有の保持の取り直しを
  今のまま続ける（上の Rationale）。
- 移植の検査: `NaturalSortKey` の入力と期待の組を `fold_for_match.json` と同じ要領で
  `internal/domain/testdata/natural_sort_key.json` に置き、Go の試験と Vitest の試験が同じファイルを読む。
  鍵の比較は Go がバイト順、画面は符号位置の順で行い、どちらも同じ順になる（UTF-8 のバイト順と
  符号位置の順は一致する。UTF-16 のコード単位の順は一致しないので `<` で文字列を比べない）。

## R-13: 却下した名前は `GET /api/tags/rejected-names` のページで受け、窓の中で続きを読む

- Decision: `GET /api/tags/rejected-names` に `cursor`・`limit`（既定 100、最大 200）を足し、応答に
  `total` と `nextCursor` を足す（[contracts/screen-api.md §6](contracts/screen-api.md#6-get-apitagsrejected-names-のパラメータ)）。
  並びは `sort_key`（R-10）、同じなら `name` のバイト順。画面は開いたときに 1 ページだけ受けて入口に
  `total` を出し、窓を開いたあと窓の中身を末尾までスクロールしたら続きを受ける。× で外した名前は
  局所で取り除き `total` を 1 減らす。却下・作成・改名・シノニムの追加のあとの取り直しは先頭の
  1 ページだけを取り直す（031 のきっかけのまま）。
- Rationale: 要件 12 の後半「却下した名前の一覧も、開いたときに表示に要る分だけを読み込む」。
  却下した名前は外部連携 API が仮のタグを作るたびに増えうるので、タグと同じく総数に比例した読み込みに
  しない。既定を 100 件にするのは R-11 と同じ。
- Alternatives considered: 全件のまま（要件 12 と相反する）。入口の件数のために `total` だけ返す経路を
  足す（1 ページの応答に `total` を載せれば足りる）。

## R-14: 統合の窓の統合先の候補は `GET /api/tags?q=…&limit=…` で引く

- Decision: `MergeTagDialog` の統合先の候補は、入力の照合形で `GET /api/tags` を `q`・`limit`
  （候補の最大の行数。[ui-design.md「Merge dialog」](ui-design.md#merge-dialog) の 8 行）付きで呼んで
  作る（R-1 の経路）。行から開いたときは応答から統合元を除き、選んだ中から開いたとき（`fromSelection`）は
  除かない（要件 9「統合先は選んだ中からでも」と Edge Case「統合先は統合元から外す」。merge 済みの
  `MergeTagDialog` と同じ）。候補の並びはサーバーの名前の順のまま。入力が変わったら
  進行中の要求を打ち切り、応答を待つ間は前の候補を残す。`tags` の prop（画面が持つ全件）は外す。
- Rationale: 改訂前は画面が全件を持っていたので候補はそこから作れた。ページで読む形（R-1）では画面は
  読み込んだ行しか持たず、要件 9 は「統合先は選んでいないタグからでも選べる」と求めるので、読み込んで
  いないタグも候補に要る。共有の保持（全件の `getTags`）を窓を開くときに取る形は、30,000 個の規模で
  窓を開くたびに全件の転送と JSON の解釈（0.2 秒を超える長いタスクになりうる。要件 3 の「統合…の
  どれをしても固まらない」）が起きる。サーバーの検索は照合形（R-1）で引くので、候補の照合も
  `toLowerCase().includes` から全角・半角・かなを同一視する形にそろう。
- Alternatives considered: 共有の保持の全件から作る（上の Rationale。親 Issue の「対象外」が全件の
  候補を #674・#675 に渡したのは管理画面の外の話で、管理画面の中の窓はこの feature が持つ）。
  読み込んだ行だけから作る（要件 9 と相反する）。
- 範囲: 「タグを付ける」の候補（`web/src/library/tagChoices.tsx`・`AddTagPopover`・`VideoTags`）は
  変えない（親 Issue の「対象外」）。
