# UI Design: タグ管理画面を数千〜数万個のタグでも片付けられる形にする

**Feature**: [parent Issue #651](https://github.com/syudead/vv/issues/651) ・
[plan.md](plan.md) ・ [research.md](research.md)（R-1〜R-7・R-11〜R-14）・
[data-model.md §4](data-model.md#4-画面の側で持つ状態) ・
[contracts/screen-api.md](contracts/screen-api.md)（§5・§6）

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・一覧の構成・選択バーの形:
  [ライブラリ UI](../../docs/design-docs/library-ui.md)（§1・§3・§4・§6）
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`。値は写さず、名前で呼ぶ
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- タグ管理画面の骨格（本文の幅、行の列と書式、作成と改名、シノニムの窓、削除の窓、状態の表）:
  [specs/014-video-tags/ui-design.md「Tag management page」](../014-video-tags/ui-design.md#tag-management-page)
  と今の [`web/src/tags/`](../../web/src/tags/)
- 仮の目印、「Tentative only」、行の「確定する」「却下する…」、却下の窓、却下した名前の**中身**
  （説明・チップ・×・読み込み・取り直し・空）:
  [specs/031-tentative-tags/ui-design.md「Tag management page」](../031-tentative-tags/ui-design.md#tag-management-page)
- ライブラリのツールバーの並べ替え（メニューと向きの切り替え、`md` 未満の「表示と並び順」のまとめ）:
  [specs/013-library-search/ui-design.md「Sort and direction」](../013-library-search/ui-design.md#sort-and-direction)、
  [specs/033-video-dates/ui-design.md「Sort and direction」](../033-video-dates/ui-design.md#sort-and-direction)、
  今の [`web/src/videoList/SortControls.tsx`](../../web/src/videoList/SortControls.tsx)・
  [`FilterMenu.tsx`](../../web/src/videoList/FilterMenu.tsx)・
  [`web/src/library/LibraryToolbar.tsx`](../../web/src/library/LibraryToolbar.tsx)
- ライブラリの選択（リスト表示のチェックの見せ方、選択バーの箱・段の折り返し・上限の扱い）:
  [specs/014-video-tags/ui-design.md「Selection bar」](../014-video-tags/ui-design.md#selection-bar)、
  今の [`web/src/library/SelectionBar.tsx`](../../web/src/library/SelectionBar.tsx)・
  [`web/src/videoList/VideoCard.tsx`](../../web/src/videoList/VideoCard.tsx)（リスト表示の行）
- ライブラリの続きの読み込み（読み込み中の `Skeleton`、続きの失敗の箱と「Retry」）:
  [specs/013-library-search/ui-design.md](../013-library-search/ui-design.md)、
  今の [`web/src/videoList/states.tsx`](../../web/src/videoList/states.tsx)（`LoadMoreFailed`）・
  [`web/src/library/LibraryPage.tsx`](../../web/src/library/LibraryPage.tsx)
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足す・変えるのは、所有者だけのタグ管理画面（`/tags`）の次の 8 つである。
1〜7 は feature branch に merge 済みで（子 Issue #678〜#687）、本書はそれらを改訂後の親 Issue と Plan
に合わせて直している。8 とその影響（「読み込んだ行」を単位にする箇所）が、その改訂で足した部分
である。

そのあとの見た目のレビューで、本文の中に作った操作の行（独自の検索欄・2 つの押しボタン・並び順）と、
件数・すべて選ぶ・却下した名前の入口を詰めた件数の行が、ライブラリの画面と揃わず浮いていると
指摘された。本書はその直し（レビューで承認された「提案 1」）に合わせてある。振る舞いとサーバーの
API は変えない（100 件ずつのページ、`q`・`tentative`・`unused`・`sort` はサーバー、「すべて選ぶ」は
読み込んだ行だけ）。直したのは次の見た目の置き場で、下の各節がその形である。

- 検索・絞り込み・並び順は、ライブラリと同じく**共通トップバー**に置く（「Top bar」）
- 本文の先頭は**見出しの行**（「Tags」・件数・「New tag」）、その下に**タブ**「Tags | Rejected names」、
  効いている**絞り込みのチップ**、**列の見出し**（「Band」）
- 選んでいる間は見出しの行を**選択の行**に入れ替え、まとめての操作を本物のボタンで並べる（「Selection bar」）
- 却下した名前は窓ではなく**タブ**の本文に並べる（「Rejected names tab」）
- 統合の窓の候補は入力の下の**高さを決めた一覧**に並べる（「Merge dialog」）
- 条件とタブは **URL** に載せる（「URL state」）

1. 見出し・タブ・列の見出しを、一覧をスクロールしても上部バーの下に**留める**（要件 13）
2. **並び順**（名前・本数・作った日）と **0 本のタグだけ**の絞り込みを足す（要件 4〜6）
3. 行に**チェック**を足し、**選んでいる間だけ**出る選択の行から、まとめて確定・却下・削除・統合する
   （要件 8〜11）
4. まとめての却下・削除・統合の**確認の窓**（要件 11）と、複数の統合元を持つ**統合の窓**（要件 9・14）
5. **却下した名前**を見出しの下のタブに移し、本文で開く（要件 12）
6. 行の操作を、タッチの端末と狭い幅では**文字を持つメニュー**にまとめる（UI品質「行の操作」）
7. 一覧の行を見えている分だけ描く（要件 1・3）。**見た目は変えない**
8. 一覧を、**表示に要る分だけサーバーから読み、スクロールで続きを読む**（要件 1〜3・7）。検索・
   絞り込み・並び順は読み込んでいないタグも含めた全部に効き、一覧の末尾に**続きの読み込み中・
   失敗・食い違い**の行が加わる。列の見出しの「すべて選ぶ」は**読み込んだ行**を選ぶ（要件 10）。
   却下した名前のタブと統合の窓の候補も、表示に要る分だけ読む（要件 12・9）

行の列と書式、作成の行、改名、シノニムの窓、1 件の確定・却下・削除・統合の規則、仮の目印、
「Tentative only」の振る舞い、却下した名前の中身の規則は変えない。新しい色・半径・影の token は
足さない（下の「Colour」）。

## Why this shape

- **ページの操作はライブラリと同じく共通トップバーに置く。** 検索・絞り込み・並び順は
  `TopBarPortal` で上部バーの中央に入れ、部品もライブラリの `SearchBox`・`FilterMenu` の枠
  （`FilterPopover`）・`SortMenu` の形（`SortMenuView`・`CompactSortView`）を使う。本文の中に独自の
  操作の行を作った最初の形は、ライブラリと見た目が揃わず浮いていた（レビュー）。上部バーは
  いつも見えているので、スクロール中に手が届く（要件 13）ことも満たす。
- **見出し・タブ・列の見出しを `sticky` の帯で留め、本文のスクロールは変えない。** 一覧を別の箱で
  スクロールさせる形（本文は固定、一覧の中だけが動く）は、ブラウザの戻る・進むでのスクロール位置の
  復元と `/` のキーの扱いを変え、設定画面など他の本文の画面と動きが違ってしまう。帯に入れるのは
  見出しの行（選んでいる間は選択の行）・タブ・絞り込みのチップ・列の見出しで、行は本文と一緒に流れる。
  選択の行が帯の中にあるので、どこまでスクロールしても選んだ行にまとめての操作が届く。
- **並び順はライブラリの「Sort by」のメニューと向きの切り替えと同じ部品で、同じ位置に置く。**
  親 Issue の UI品質がこの形を名指ししていて、利用者はライブラリで既に覚えている。同じ `Button` の
  secondary にいまの種類の名前を出すので、「今どの並びか」はボタンの文字で分かる。並び順は列の
  見出しのクリックにはしない。列の見出しは「どの列が何か」を言うだけの細い行で、並び順の状態は
  トップバーのボタンが言う。
- **「Tentative only」「Unused only」はライブラリと同じ「Filter」の吹き出しの中のチェックにする。**
  最初の形は 2 つの押しボタンを本文の操作の行に並べたが、ライブラリの絞り込みと形が違い、行が
  混み合った（レビュー）。効いている絞り込みは「Filter」のボタンの数（ライブラリと同じ
  `bg-accent-soft text-link` と数）と、見出しの下の**チップ**（ライブラリのタグの絞り込みの
  `ActiveTagFilters` と同じ `FilterChip`）で読めるので、吹き出しを開かなくても「何で絞っているか」が
  分かり、チップの × で 1 回で外せる。
- **検索・絞り込み・並び順は、見えている行の見た目を変えずにサーバーへ渡す。** 利用者から見える
  違いは、結果が「読み込んだ行の中」ではなく「全部のタグ」から出ることだけで（要件 4・6・7）、
  部品も位置も変えない。条件を変えたとき、新しい先頭のページが届くまで**前の行と件数を残す**
  （ライブラリの検索と同じ。空の一瞬や `Skeleton` の点滅を作らない）。失敗したら前の行を残し、
  帯の中に失敗の箱と「Retry」を**残し続けて**、残した行が今の条件の結果ではないことを示す（Edge Case
  「一覧の読み込みに失敗したときは、今と同じ失敗の表示と『Retry』を出す。すでに一覧を持っていれば、
  その一覧を残す」）。トーストだけにすると、消えたあとに条件と合わない行が今の結果に見え続ける。
- **続きは、スクロールが読み込んだ行の末尾に近づいたら自動で読み、「もっと見る」のボタンは置かない。**
  ライブラリと同じ形で、親 Issue も「スクロールに合わせて続きを読み込む」と求める。続きの読み込み中・
  失敗・食い違いは**一覧の末尾の行**として出し、トーストにしない。読み込んだ行の下に続くものなので、
  「ここで一覧が途切れている」という位置に見せるのが読みやすく、スクロールを戻れば前の行はそのまま
  残っていることが分かる。失敗の行はライブラリの `LoadMoreFailed` と同じ箱で、同じカーソルから
  読み直す「Retry」を持つ。
- **食い違い（別のタブでタグが増減した）は黙って取り直さず、行を残して知らせる。** 続きの応答の
  `totalAll` が画面の値と違ったら、読み込んだ行と選択を残し、末尾に「一覧が変わった」の 1 行と
  「Reload」を出して続きを止める（[research.md R-11](research.md#r-11-続きは画面の末尾に近づいたら-100-件ずつ読みid-で重複を捨て件数が食い違えば知らせて取り直させる)）。
  黙って先頭から読み直すと、数千行スクロールした位置と選んでいる行を失う。利用者が「Reload」を
  押したときだけ先頭から読み直す（選択は空になる）。食い違いの行は失敗ではないので danger の色は
  使わず、面を持つ中立の箱にする。
- **「すべて選ぶ」は読み込んだ行だけを選び、読み込んでいないタグには届かない。** 親 Issue の要件 10
  と「対象外」（条件に当てはまる全部へのまとめての操作）がこう決めている。選べる範囲が「見えている
  集合」より狭いので、読み上げ名と選択の行の件数で**選んだのは読み込んだ分だけ**だと分かるようにする
  （下の「Column header」）。一覧の総数と選んだ数の違い（「500 of 1,000 tags」に対して「100 tags
  selected」）が、残りを読み込んでいないことの印になる。件数に読み込んだ数（ページの区切り）は
  添えない。読み込みはスクロールで自動に続き、利用者が知る必要のない仕組みの数だからである（レビュー）。
- **選択はチェックで、選んでいる間は見出しの行を選択の行に入れ替える。** 最初の形は画面下部に
  浮かぶバーで、最後の行に重なった（レビュー）。見出しの行は帯の中で留まるので、そこを入れ替えれば
  スクロールしても手が届き（要件 13）、行に重ならない。入れ替えは同じ高さ（`min-h-10`）で行い、
  選んだ瞬間に一覧が上下に動かない。チェックを常に薄く見せるのは、ライブラリのリスト表示と同じ
  規則で、hover できない端末でも選択の入口が見えるためである。
- **まとめての操作は本物のボタンで並べ、働かない操作は出さない。** 最初の形は「Confirm」だけを前に
  出し、残りを「More」のメニューに隠した。選択の行には場所があるので、「Confirm」（primary。片付けの
  主の操作）・「Merge into one tag…」・「Reject…」・「Delete…」（secondary、文字とアイコンは danger の色）を
  並べる。選んだタグに働かない操作（仮のタグが無いときの「Confirm」「Reject…」、確定したタグが無い
  ときの「Delete…」）は薄くせず、出さない（押せない理由を読ませるより、押せるものだけを並べる方が
  短い）。確定は確認をとらず（要件 11）、却下・削除・統合は確認の窓を挟む。
- **「読み込んだものをすべて選ぶ」は列の見出しの先頭のチェック。** 一覧の上の細い列の見出し
  （チェック・「Name」・右寄せの「Videos」）の先頭にあり、メールの一覧の見出しのチェックと同じく
  「この一覧（のうち読み込んだ分）を選ぶ」と読める。選択の行の「すべて選択」（ライブラリ）を置く
  案は、選択の行は 1 件選んだあとにしか出ないので、最初の 1 件を選ぶ手が別に要る。先頭のチェック
  なら、絞って 1 回押すだけで読み込んだ全部が選べる。
- **上限を超えて読み込んだ一覧で止めるのは、先頭のチェックだけ。** まとめての操作の上限
  （`maxTagBatch`、20,000 件）は送る id の数に掛かる（[research.md R-4](research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かない無いタグは数えて飛ばす)、
  Plan「まとめての操作」）。読み込んだ行が上限を超えても、1 行ずつ選んだ数行のまとめての操作は
  上限に当たらないので、選択の行の操作を止める理由は無い。止めるのは「読み込んだものを
  すべて選ぶ」だけで、理由は「読み込んだ数が多すぎる」と言い、絞り込みへ誘う。選択の行の操作が
  止まるのは選んだ数そのものが上限を超えたときだけである（1 行ずつでは実質起きない）。
- **却下した名前は見出しの下のタブ「Rejected names」で開き、本文で続きを読む。** 031 が一覧の下に
  置いた折りたたみは、数千行の下では届かない（要件 12）。最初の形は件数の行の右端の小さな入口から
  窓で開いたが、件数の行が混み合い、「Rejected names 0」が何の数か読みにくかった（レビュー）。
  タブなら「Tags 300 | Rejected names 4」と、2 つの一覧とその数が同じ形で並び、帯に留まるので先頭から
  届く。中身は本文に並べ、本文のスクロールで続きを読む（要件 12 の後半）。タブの数は応答の `total` で、
  読み込んだ数ではない。
- **統合の窓の統合先の候補は、入力のたびにサーバーから引く。** 画面が持つのは読み込んだ行だけで、
  要件 9 は「統合先は…選んでいないタグからでも選べる」と求める
  （[research.md R-14](research.md#r-14-統合の窓の統合先の候補は-get-apitagsqlimit-で引く)）。候補の照合も
  一覧の検索と同じ照合形になり、全角・半角やかなの違いが同一視される。利用者から見える違いは、
  入力の右端に読み込み中の小さな回転と、候補が入力より一拍遅れて変わりうることだけである。届く
  まで前の候補を残し、候補の一覧を空にして点滅させない。候補は入力の上に重ねて開く一覧ではなく、
  入力の下の**高さを決めた箱**に常に並べる。重ねる形は窓の中身より長く開いて窓のボタンを覆った
  （レビュー）。
- **行の操作は、タッチの端末と `sm` 未満の幅では、文字を持つ 1 つのメニューにまとめる。** 親 Issue の
  UI品質は「触るまで意味の分からないアイコンだけにならない」と求める。アイコンの横に文字を出す
  案は、行の幅を 4 つの操作の文字に取られ、名前の列が無くなる。メニューの各項目は文字を持ち、
  入口の「⋯」は現行の一覧の製品が「この行の操作」に収束させている形である。行で 1 回押しの確定は
  2 回押しになるが、タッチの端末での片付けの主の経路は「チェック → 確定（選択の行）」で、こちらは
  行ごとに 1 回押しのまま残る。マウスの端末では、今の 4 つの `IconButton`（ツールチップ付き）を
  変えない。

## Words

| 場所 | 英語（案） |
| --- | --- |
| 並び順のメニューの見出し | Sort by |
| 並び順の種類 | Name ／ Video count ／ Date created |
| 並び順のボタンの読み上げ名 | Sort by: 〈種類〉 |
| 向きの切り替え（本数） | Descending (most videos first). Press for ascending ／ Ascending (fewest videos first). Press for descending |
| 向きの切り替え（作った日） | Descending (newest first). Press for ascending ／ Ascending (oldest first). Press for descending |
| 向きの `SegmentedControl` の項目（`sm` 未満のまとめ） | Most videos first ／ Fewest videos first、Newest first ／ Oldest first |
| `md` 未満の並び順のまとめのボタン | Sort |
| 検索欄（読み上げ名・placeholder） | Search tags |
| 絞り込みのボタン（ライブラリと同じ） | Filter ／ Filter (2 applied)（読み上げ名） |
| 絞り込みの吹き出しのチェック | Tentative only ／ Unused only |
| 絞り込みのチェックの補足 | Show only tags created by automatic tagging ／ Show only tags that aren't on any videos |
| 絞り込みの吹き出しの解除（ライブラリと同じ） | Clear filters |
| 絞り込みのチップの並びの読み上げ名 | Active filters |
| 絞り込みのチップの読み上げ名 | Remove the filter "Tentative only" ／ Remove the filter "Unused only" |
| 見出しの件数（絞り込み・検索のどちらかが効いている） | 90 of 1,000 tags（`total` of `totalAll`）。効いていなければ 1,000 tags。読み込んだ数は添えない |
| タブの並びの読み上げ名 | Tag lists |
| タブ | Tags 〈`totalAll`〉 ／ Rejected names 〈`total`〉 |
| 列の見出し | Name ／ Videos |
| 列の見出しの先頭のチェックの読み上げ名 | Select all 100 loaded tags ／ Clear selection（全部選んでいるとき） |
| 先頭のチェックが押せない理由（読み込んだ数が上限を超える） | Too many tags are loaded to select them all at once (limit {limit}). Narrow the list with search or a filter. |
| 行のチェックの読み上げ名 | Select "〈名〉" |
| 続きを読み込んでいる間（読み上げのみ） | Loading more tags… |
| 続きの読み込みに失敗した行 | Couldn't load more: {理由} ／ Retry |
| 先頭のページを読めなかったとき（一覧を持っている。帯の箱） | Couldn't load tags: {理由}. The list below may not match the current search, filters and sort. ／ Retry |
| 一覧が変わった行 | Tags were added or removed elsewhere, so the rest of this list may be out of date. ／ Reload |
| 一致が無いときの見出し | No unused tags ／ No unused tentative tags ／ No unused tags match "〈入力〉" ／ No unused tentative tags match "〈入力〉" |
| 一致が無いときの説明（「Unused only」だけ、検索なし） | Every tag is on at least one video. |
| 一致が無いときのボタン | Show all tags（今の形） |
| 選択の行の読み上げ名（`region`） | Selected tags |
| 選択の行の件数 | 1 tag selected ／ 12 tags selected |
| 選択の行の操作 | Clear selection（×）／ Confirm ／ Merge into one tag… ／ Reject… ／ Delete… |
| 選んだ数が上限を超えたとき（選択の行） | Too many tags are selected to act on them together (limit {limit}). Clear some of the selection. |
| まとめての確定のトースト | Confirmed 8 tags ／ Confirmed 8 tags. 4 were already confirmed. |
| まとめての却下の窓の見出し | Reject selected tags |
| まとめての却下の本文（全部に働く） | The 8 selected tags will be removed from 120 videos, and automatic tagging won't create their names again. You can allow a name again from Rejected names. |
| まとめての却下の本文（一部に働く） | 8 of the 12 selected tags are tentative. They will be removed from 120 videos, and automatic tagging won't create their names again. The 4 confirmed tags are left as they are. You can allow a name again from Rejected names. |
| まとめての削除の窓の見出し | Delete selected tags |
| まとめての削除の本文（全部に働く） | The 8 selected tags will be removed from 120 videos. This can't be undone. |
| まとめての削除の本文（一部に働く） | 8 of the 12 selected tags are confirmed. They will be removed from 120 videos. This can't be undone. The 4 tentative tags are left as they are; reject them instead. |
| 0 本のとき（却下・削除・統合に共通の差し替え） | …aren't on any videos… の形（「removed from 0 videos」とは言わない） |
| 数を数えている間 | Counting the affected videos… |
| 数えられなかったとき | Couldn't count the affected videos: {理由} ／ Retry |
| まとめての却下・削除のボタン | Cancel ／ Reject（送信中 Rejecting…）／ Delete（送信中 Deleting…） |
| まとめての却下・削除のトースト | Rejected 8 tags ／ Rejected 8 tags. 4 confirmed tags were skipped. ／ Deleted 8 tags ／ Deleted 8 tags. 4 tentative tags were skipped. |
| 統合の窓の見出し（複数） | Merge 4 tags |
| 統合の窓の統合元の見出し | Tags to merge |
| 統合の窓の統合先の入力（見える名札と読み上げ名） | Tag to merge into |
| 統合先の候補が無いとき（一覧の中） | No matching tags |
| 統合の窓の下端（統合元 → 統合先） | Alpha → Action ／ 3 tags → Action |
| 統合先の候補を引けなかったとき（入力の下） | Couldn't search tags: {理由} |
| 統合の窓の確認（複数。数は統合先を外した統合元） | The 120 videos tagged with these 4 tags get the tag "Action". Their names and synonyms become synonyms of "Action", and the 4 tags leave the tag list. This can't be undone. |
| 統合先を選んだ中から選んだとき | "Action" is kept and the other 3 tags merge into it. |
| 統合元が無くなったとき | Choose another tag to merge into: "Action" is the only tag selected. |
| 統合のトースト（複数。数は実際に統合した数） | Merged 4 tags into "Action" |
| 対象の一部がもう無かったとき | Some of the tags no longer existed, so the list was reloaded |
| 却下した名前のタブの説明 | Automatic tagging won't create these names. Allow a name again to let it be created. |
| 却下した名前の行のボタン（見える文字・読み上げ名） | Allow again ／ Allow "〈名〉" again |
| 却下した名前のタブで続きを読めなかったとき | Couldn't load more rejected names ／ Retry |
| 行の操作のまとめの入口（タッチ・`sm` 未満） | Actions |

- 「Unused」は 0 本のタグの呼び名で、文言の中で「tags that aren't on any videos」と説明する。
  「empty」「orphan」は使わない。
- 「loaded」は「画面がここまでに受け取った」の意味で、読み込んでいない残りがあることを含む。
  「shown」「visible」は使わない（見えている行は描いている行のことで、読み込んだ行とは別の集合）。
  「loaded」は先頭のチェックの読み上げ名と押せない理由だけに使い、件数やページの区切りには出さない。
- 数はすべて `formatNumber`、件数の形はカタログの `tagCount`・`videos` を使う。タグの名前は利用者の
  データで、翻訳せずに埋め込む（i18n.md）。

## Top bar

検索・絞り込み・並び順は、ライブラリと同じく共通トップバーの中央（`TopBarPortal`）に置く
（[`web/src/tags/TagToolbar.tsx`](../../web/src/tags/TagToolbar.tsx)）。並びと見た目は
[`LibraryToolbar`](../../web/src/library/LibraryToolbar.tsx) と同じで、Tab の順も同じく 検索欄 →
「Filter」→ 並び順 → 向き。本文の中に操作の行は置かない。「Rejected names」のタブを開いている間は
トップバーに何も置かない（タグの検索・絞り込み・並び順は却下した名前に効かない。031 と同じ）。

検索・「Tentative only」・「Unused only」・並び順のどれを変えても、画面はその条件で**先頭のページを
サーバーから読み直す**（[data-model.md §4](data-model.md#4-画面の側で持つ状態)「条件」）。届くまで前の
行と件数を残し、`Skeleton` に戻さない。届いたら行と件数を差し替え、スクロール位置は先頭へ戻す
（条件が変わったので前の位置に意味は無い）。進行中の続きの読み込みは打ち切り、古い条件の行は
混ざらない（Edge Case）。**どの変更でも選択は空になる**（Edge Case「選んでいる間に検索・絞り込み・
並び順を変えたとき: 選択は外す」。並び順の変更も含む。読み直したあとの行が同じ id を持っていても
選び直しはしない）。改名中の行は条件を変えても残る（下の「Row checkbox」）。先頭のページの読み込みに
失敗したときは下の「Stale list」。

- **検索**: ライブラリの `SearchBox`（`syntaxHelp={false}`。動画の検索構文の手引きは出さない）。
  読み上げ名・placeholder は「Search tags」、`/` でフォーカス、Esc で消してフォーカスを外す（IME の
  変換中の Esc は無視する）。打鍵ごとに引き直す（`debounceMs={0}`。前の要求は打ち切る。014 からの
  振る舞い）。照合はサーバーが全部のタグの名前とシノニムに掛け、ライブラリでタグを探すときと同じ規則で
  全角・半角やかなの違いを同一視する（要件 7、[contracts/screen-api.md §5](contracts/screen-api.md#5-get-apitags-のパラメータ) の `q`）。
  入力は 100 文字（符号位置）で止める（`q` の上限と同じ。ライブラリの検索欄と同じ数え方）。
  タグが 1 つも無いときは `disabled`。
- **「Filter」**: ライブラリの `FilterMenu` と同じボタンと吹き出し（`FilterPopover`。`Button` の
  secondary、`ListFilter`、`xl` 以上で文字「Filter」）。吹き出しの中は 2 つのチェック（`FilterCheckbox`）
  「Tentative only」（補足「Show only tags created by automatic tagging」）と「Unused only」（補足
  「Show only tags that aren't on any videos」）。どちらかが効いている間は、ボタンをライブラリと同じく
  `bg-accent-soft text-link` にして効いている数（1 か 2）を添え、読み上げ名を「Filter (N applied)」にし、
  吹き出しの末尾に「Clear filters」（両方を外す。検索と並び順は残す）を出す。チェックを押すと
  `tentative=true`・`unused=true` で読み直し、吹き出しは開いたまま（ライブラリと同じ）。両方が効いて
  いれば両方を満たすタグ（要件 6）。読み込んでいないタグも含めた全部に効く。`disabled` は先頭の
  ページをまだ一度も受けていない間・一覧を持たないまま失敗したとき・何も効いていない間に
  `totalAll` が 0 のとき。**効いている間は `total` が 0 になっても `disabled` にしない**（絞り込みを
  外す手であり、フォーカスの行き先でもある）。
- **並び順**: `md` 以上はライブラリの `SortMenu` と同じ形（`SortMenuView`）。`Button` の secondary に
  いまの種類の名前（「Name」「Video count」「Date created」）と `ChevronDown`、読み上げ名「Sort by:
  〈種類〉」。メニューは見出し「Sort by」と 3 つのラジオ項目。アイコンは Name が `ArrowDownAZ`
  （ライブラリの「Title」と同じ、名前の順の意味）、Video count が `Hash`（数）、Date created が
  `CalendarPlus`（タグができた日。ライブラリの「Date created」の `FileClock` はファイルの作成日で別の
  ものなので、同じ絵にしない）。
  - 種類を選ぶと、その種類の既定の向きになる: Video count は多い順（`countDesc`）、Date created は
    新しい順（`createdDesc`）。Name に向きは無い（要件 4）。
  - **向きの切り替え**は、ライブラリと同じくメニューのボタンの右に接する `Button`
    （`rounded-l-none px-2.5`）で、`ArrowDownWideNarrow`（降順）／`ArrowUpNarrowWide`（昇順）、
    読み上げ名とツールチップは上の「Words」。**Name のときは向きのボタンを出さない**（メニューの
    ボタンは全周の角丸に戻る）。
  - `md` 未満は、ライブラリの「表示と並び順」と同じく `SlidersHorizontal` のボタン（読み上げ名「Sort」）
    を押すと吹き出し（`PopoverContent`、`align="end"`、`w-72`）が開き、中身は `CompactSortControls` と
    同じ形（`CompactSortView`）: 見出し「Sort by」、2 列のラジオ、その下に向きの `SegmentedControl`
    （Name のときは出さない）。ボタン自体の見た目は並び順で変えない（ライブラリと同じ）。
  - 並び順は読み込んでいないタグも含めた全部に効く（要件 4。サーバーが並べ、画面は並べ直さない）。
    同じ値のタグどうしは名前の自然順（R-10）。選んだ並び順は URL に載り（下の「URL state」）、
    `localStorage` にも残る（R-7）。URL に並び順が無いときは残した並び順で始まる。壊れていれば Name。
  - `disabled` はタグが 1 つも無いとき（先頭のページを受ける前を含む）。

## URL state

検索語・絞り込み・並び順・タブは、ライブラリの一覧の条件（[specs/013-library-search/contracts/list-url.md](../013-library-search/contracts/list-url.md)）
と同じく URL のクエリに載せ、再読み込み・戻る・進むで残す
（[`web/src/tags/tagListUrl.ts`](../../web/src/tags/tagListUrl.ts)）。

- パラメータは `q`（検索語。`normalizeQuery` の形）、`tentative=1`、`unused=1`、`sort`（`name`・
  `countDesc`・`countAsc`・`createdDesc`・`createdAsc`）、`tab=rejected`。偽の絞り込みと「Tags」の
  タブは書かない。`sort` は書くときはいつも書く（省くと「端末に残した並び順」の意味になり、戻るで
  前の並びに戻れない。list-url.md §1 と同じ）。解釈できない値は既定として扱う。
- 絞り込み・並び順・タブ・チップの × は履歴を 1 つ増やす（push）。検索の入力はライブラリと同じく、
  フォーカスが入ってから外れるまでの一続きで最初の確定だけが push で、残りは置き換え。
- 「Tentative only」「Unused only」と検索語は R-7 で URL に載せないとしていたが、ライブラリの条件と
  揃えるために載せる（レビュー）。`localStorage` には今までどおり並び順だけを残す。

## Band

本文の先頭を 1 つの帯にし、上部バーの下に留める
（[`web/src/tags/TagsPage.tsx`](../../web/src/tags/TagsPage.tsx)）。帯の中は上から、**見出しの行**
（選んでいる間は**選択の行**）→ **タブ** → **絞り込みのチップ**（効いているときだけ）→ 「Stale list」の
箱（あるときだけ）→ **列の見出し**。

- 帯は `position: sticky`、`top` は上部バーの高さの token（`top-navbar`）、面は `bg-bg`（不透明。
  下を流れる行が透けない）、`z-20`（上部バーの `z-40` より下）。中は `flex flex-col gap-3`、上に
  `pt-3`（留まったときに見出しが上部バーに接しない）。本文の幅は今の `max-w-4xl`。
- 帯の高さは幅と中身で変わる（チップ・箱・選択の行の折り返し）。`ResizeObserver` で測り直し、仮想化の
  スクロール位置の計算と `scroll-padding-top` は帯の高さを差し引く（行へフォーカスを移すときに、
  その行が帯の下に隠れない）。
- 「新しいタグ」を押したとき、一覧の先頭が帯の下に見えていなければ、先頭までスクロールしてから
  作成の行を差し込み、入力へフォーカスを移す（作成の行はいつも一覧の先頭で、見えない位置に
  入力を作らない）。

### Header

見出しの行は `flex min-h-10 items-center gap-3`。左に `h1`「Tags」（ライブラリの見出しと同じ
`text-xl font-semibold tracking-tight sm:text-2xl`）、右にライブラリの「N items」と同じ書式の件数
（`text-xs text-fg-muted tabular-nums sm:text-sm`、`role="status"`・`aria-live="polite"`）、その右に
primary の「New tag」（`Plus`）。

- **件数**: 応答の `total`・`totalAll` で出す（[contracts/screen-api.md §5](contracts/screen-api.md#5-get-apitags-のパラメータ)）。
  「1,000 tags」、絞り込み・検索のどちらかが効いていれば「90 of 1,000 tags」（受け入れ条件 8。読み込んで
  いないタグも数えた数）。**読み込んだ数・ページの区切りはどこにも出さない**。改名中の残した行は
  数えない。操作のあとの増減は局所で数え直す（[data-model.md §4](data-model.md#4-画面の側で持つ状態)
  「操作のあとの反映」）。先頭のページが届く前は「Loading…」。
- 「Rejected names」のタブの間は、件数と「New tag」を出さない（`h1` だけ）。
- 1 件でも選ぶと、この行は同じ高さの**選択の行**に入れ替わる（下の「Selection bar」）。

### Tabs

見出しの行の下に、下線のタブ（`ui/Tabs`。`role="tablist"`、読み上げ名「Tag lists」）を 2 つ置く:
「Tags 〈`totalAll`〉」と「Rejected names 〈却下した名前の `total`〉」。数は名前の後ろに `font-normal
text-fg-subtle tabular-nums` で添え、取れるまでは出さない。選んでいるタブは `text-fg` と
`border-accent` の 2px の下線、選んでいないタブは `text-fg-muted`（hover で `text-fg`）。並びの下端に
`border-b border-border`。

- 押すか、左右の矢印・Home・End で切り替える（選んでいるタブだけが Tab の順に入る）。パネルは
  `role="tabpanel"` と `aria-labelledby`。
- タブは URL の `tab` に載る（「URL state」）。切り替えると、選択・作成の行・改名を閉じる（どれも
  「Tags」のタブの中のもの）。作成・改名の送信中は切り替えない。
- 「Tags」のタブを離れても読み込んだ行と件数は残り、戻れば同じ一覧が出る（「Rejected names」の間は
  続きを読まない）。

### Active filters

「Tentative only」「Unused only」のどちらかが効いている間だけ、タブの下に効いている絞り込みの
チップを並べる（`ul`、読み上げ名「Active filters」、`flex flex-wrap gap-1.5`）。チップはライブラリの
タグの絞り込み（`ActiveTagFilters`）と同じ `FilterChip`（`h-6`・`rounded-sm`・`bg-accent-soft`・
`text-xs text-link`、末尾に `X`）で、先頭のアイコンは「Tentative only」が `CircleDashed`（仮の目印と
同じ絵）、「Unused only」が `VideoOff`。読み上げ名は「Remove the filter "〈名〉"」。押すとその絞り込み
だけを外し、フォーカスは残るチップへ、無ければ「Filter」へ（タグが 0 なら「New tag」へ）移す。

### Column header

一覧の上の細い行（`h-9`、`border-b border-border`、`text-xs text-fg-muted`）。行と同じ `px-2`・
`gap-2 sm:gap-3` で、左から**先頭のチェック** →「Name」（`flex-1`）→ 右寄せの「Videos」（行の本数の列と
同じ `w-16 sm:w-20`）→ 行の操作の列と同じ幅の空き（マウスの端末で `sm` 以上は 4 つの `IconButton` の
幅、タッチ・`sm` 未満は「Actions」1 つの幅）。行の操作の列の幅を揃えるため、確定したタグの行も
「確定する」の分の幅を空けておく（本数の列が全部の行で「Videos」の下に揃う）。列の見出しは並べ替えの
操作を持たない。一覧に行が無い（空の状態）ときは出さない。

- **先頭のチェック**: `Checkbox`（`size-5`）を `size-8` の包みに置く。見え方は行のチェックと同じ規則
  （下の「Row checkbox」。選んでいない間は `opacity-40`、列の見出しに hover するか選んでいる間は
  `opacity-100`）。対象は**読み込んだ行**（`rows`）のうち**選べる行**で、改名中の行は入らない。
  読み込んでいないタグは選ばれない（要件 10）。状態は 3 つ: 選べる行を 1 つも選んでいなければ空、
  一部なら中間（lucide `Minus`）、全部なら選択。空と中間で押すと選べる行をすべて選び、全部のときに
  押すと選択を解く。読み上げ名は空・中間で「Select all 100 loaded tags」（数は読み込んだ選べる行の数）、
  全部で「Clear selection」。続きが届いて読み込んだ行が増えると、全部の状態は中間に戻る。選べる行が
  無いとき・読み込んだ行の数が上限（`maxTagBatch`）を超えるときは `disabled`。上限のときは、理由
  「Too many tags are loaded to select them all at once…」を包みの `title` と `aria-describedby` の
  `sr-only` で添える（ライブラリの上限と同じ形）。この理由は選択の行には出さない（下の「Enabled and
  disabled」）。

### Stale list

一覧を持っている間に**先頭のページ**の読み込みが失敗したとき（条件の変更、「Reload」、`notFoundIds`
のあとの取り直しのどれでも。[data-model.md §4](data-model.md#4-画面の側で持つ状態)「読み込み失敗」）の形。

- 前の行と件数はそのまま残し（Edge Case）、帯の中の列の見出しの上に、ライブラリの
  `LoadMoreFailed` と同じ箱（`rounded-md border border-danger bg-danger-soft px-3 py-2 text-sm
  text-danger`、`AlertCircle`、`role="alert"`）で「Couldn't load tags: {理由}. The list below may not
  match the current search, filters and sort.」と `Button` の `sm`「Retry」を出す。トーストは出さない
  （箱が同じことを言い、消えない）。帯の中に置くのは、スクロールした位置でも見えるためである
  （残した行は条件と合わないので、どこまでスクロールしても知らせが要る）。
- 箱は、「Retry」か条件の変更で次の先頭のページが**届くまで**残る。「Retry」は今の条件で先頭の
  ページを読み直す（送信中は「Retry」を `disabled`。届いたら行と件数を差し替え、スクロール位置は
  先頭、箱は消える）。また失敗すれば箱は残り、理由だけが変わる。
- 箱がある間は**続きを読まない**（持っているカーソルは前の条件のもので、今の条件の続きにならない。
  Edge Case「古い条件の行は混ざらない」）。末尾の「Loading more」の状態も出さない。残した行の
  チェックと操作は押せる（どれも実在するタグへの操作で、反映は今までどおり読み込んだ行の中で行う）。
- 帯の高さは箱の分だけ伸びる（`sm` 未満では文字とボタンが `flex-wrap` で折り返す）。まれな失敗の
  間だけで、仮想化のスクロール位置の計算は今の帯の高さを差し引く（上の「Band」）。

## Rows

行の列・書式・高さ（`py-2`）・名前のリンク・シノニムの行・本数・仮の目印・改名の入力は 014・031 の
まま。足すのは先頭のチェックと、タッチ・狭い幅での操作のまとめだけで、見えている行だけを描く
仕組み（R-2）とページで読む仕組み（R-1・R-11）は行の見た目を変えない。

### Loading more

一覧の末尾（読み込んだ最後の行の下）に、続きの状態を 1 つだけ出す。行と同じ `px-2`、幅は行と同じ、
`divide-y` の線の下に置く。件数には数えず、選べず、仮想化の描く範囲にも入れない（読み込んだ行の
後ろに置く通常の要素）。

- **読み込み中**: 行の `Skeleton` を 3 つ（初回の読み込み中と同じ形・同じ高さ。`aria-hidden`）。
  一覧の包みを `aria-busy`、`sr-only` の `role="status"` に「Loading more tags…」。きっかけは
  仮想化が描く最後の行が読み込んだ行の末尾から数行以内に入ったときで（[data-model.md §4](data-model.md#4-画面の側で持つ状態)
  「続きを読むきっかけ」）、利用者が末尾に届く前に読み始める。**操作で読み込んだ行が 1 つも残らず**
  （`rows` が空）、`nextCursor` があるときは、描く行が無いので仮想化のきっかけは働かない。この
  ときは操作の反映と同時にその場で続きを 1 回要求し、`total` が 0 でなければ空の状態は出さずに
  この `Skeleton` を出す（「Tentative only」で読み込んだ仮のタグを全部確定した、「Unused only」で
  読み込んだ行を全部削除した、など）。届いた行は末尾に足し、`Skeleton` は
  消える。**出ている行の操作とスクロールは止まらない**（UI品質）。読み込みの間に行を 1 件操作しても、
  その結果は読み込んだ行の中で反映する（R-12）。
- **失敗**: ライブラリの `LoadMoreFailed` と同じ箱（`rounded-md border border-danger bg-danger-soft
  px-3 py-2 text-sm text-danger`、`AlertCircle`、`role="alert"`）に「Couldn't load more: {理由}」と
  `Button` の `sm`「Retry」。「Retry」は同じカーソルで読み直す。読み込んだ行はそのまま残る（Edge Case
  「続きの読み込みに失敗したとき」）。箱の上下は `my-3`。
- **一覧が変わった**（続きの応答の `totalAll` が画面の値と違う）: 同じ大きさの中立の箱
  （`rounded-md border border-border-strong bg-elevated px-3 py-2 text-sm text-fg`、lucide
  `RefreshCw`、`role="status"`）に「Tags were added or removed elsewhere, so the rest of this list may be
  out of date.」と `Button` の secondary・`sm`「Reload」。続きの読み込みは止め、読み込んだ行と
  選択は残す。「Reload」で先頭から読み直し（選択は空、スクロール位置は先頭）、箱は消える
  （Edge Case「続きを読むあいだに別のタブで…」、R-11）。danger にしないのは、失敗ではなく
  知らせだからである。
- **末尾まで読んだ**（`nextCursor` が無い）: 何も出さない。「すべて読み込みました」の行は置かない
  （続きの `Skeleton` が出なくなることが印で、読み込んだ数は出さない）。

末尾の状態は同時に 1 つだけで、読み込み中 → 失敗、読み込み中 → 一覧が変わった、のどちらかに
変わる。条件を変えて先頭から読み直すと、どの状態も消える。

### Row checkbox

- 名前の列の**左**に `Checkbox`（`size-5`）を `gap-2`（`sm:gap-3`）で置く。押す範囲は `size-8` の
  正方形（チェックを中央に置いた包み）で、タッチでも外さない。チェックを押しても名前のリンクへは
  移らない。
- 見え方はライブラリのリスト表示の規則: 1 件も選んでいない間は `opacity-40`、その行に hover
  するかフォーカスが入ると `opacity-100`、1 件でも選んでいる間はすべての行で `opacity-100`。
  hover できない端末では `opacity-40` のまま押せる（薄くても見えている）。
- 選んだ行は面を `bg-accent/10`（アクセントの 10% の薄い色）にする。最初の形の `bg-accent-soft` は
  塗りつぶしの濃いティールで、選んだ行が続くと一覧が重く見えた（レビュー）。名前（`text-fg`）と本数・
  シノニム（`text-fg-muted`）の色は変えない。改名中の行は改名の面（`bg-elevated` と
  `ring-control-border`）が勝ち、その行のチェックは `disabled`（改名の確定で行が並び順の別の位置へ
  動きうるため、改名の間は選ばない）。選んでいる行の改名を始めると、その行を選択から外す（選択の行の
  件数が減り、空になれば見出しの行に戻る）。まとめての操作が改名中のタグに働くことは無い。
- 選択は**読み込んだ行**の部分集合で、検索・絞り込み・**並び順**のどれを変えても空になる（Edge
  Case、[data-model.md §4](data-model.md#4-画面の側で持つ状態)「選択」）。操作や取り直しで `rows` から
  消えた id も外れる。読み込んだ行が上限を超えても、行のチェックは押せる（上の「Why this shape」）。
- 改名中の行は、条件を変えて先頭から読み直したあとも、新しい `rows` に無ければ並び順の位置に
  差し込んで残し、入力中の名前を失わない（Edge Case「改名中の行は…」。今の検索と同じ）。改名を
  確定・取り消しすると、その行は今の条件に合わなければ消える。
- Shift を押しながらの範囲選択は入れない（親 Issue に無く、先頭のチェックで読み込んだ全部を選べる）。

### Actions on touch and narrow widths

- マウスの端末（`pointer: fine`）で `sm` 以上の幅では、行の操作は今のまま（「確定する」「改名」
  「シノニム」の `IconButton` と「その他の操作」のメニュー。031「Row」）。
- **タッチの端末（`pointer: coarse`）または `sm` 未満の幅**では、行の右端を 1 つの `IconButton`
  （`Ellipsis`、読み上げ名「Actions」、ツールチップなし）にし、メニューに文字を持つ項目を並べる:
  「Confirm」（`Check`、仮の行だけ）→「Rename」（`Pencil`）→「Synonyms」（`Tags`）→
  「Merge into another tag…」（`Merge`）→ 区切り線 →「Reject…」（`Ban`、danger。仮の行）または
  「Delete…」（`Trash2`、danger。確定した行）。項目の文言は今の `IconButton` の読み上げ名と
  メニューの項目をそのまま使う。出し分けは CSS（`[@media(pointer:coarse)]` と `max-sm:`）で行い、
  `matchMedia` は読まない（library-ui.md §4、`TouchControls` と同じ）。
- まとめたメニューの「Confirm」は行の「確定する」と同じ振る舞い（確認なし、送信中は入口の
  `IconButton` を `aria-busy` にし次の押下を無視）。改名・シノニム・統合・却下・削除は、それぞれの
  `IconButton`・項目を押したときと同じ。フォーカスの行き先の規則（014・031）で「改名」「その他の
  操作」を指すものは、まとめている間は入口の `IconButton` を指す。
- 行の幅の配分（360px）: 本文の `px-4` と行の `px-2` を引いた 312px から、チェックの包み
  `size-8`（32px）・間 `gap-2` ×3（24px）・本数の列 `w-16`（64px）・入口の `IconButton`（32px）を
  引いて、名前の列は約 160px になる（031 の 92px より広い）。横スクロールは出ない。
- このまとめは merge 済み（#683）。

## Selection bar

1 件でも選ぶと、帯の先頭の見出しの行が同じ高さ（`min-h-10`）の**選択の行**に入れ替わる
（[`web/src/tags/TagSelectionBar.tsx`](../../web/src/tags/TagSelectionBar.tsx)）。`role="region"`、読み上げ名
「Selected tags」。選択が空になると見出しの行に戻る。画面下部に浮かぶバーは置かない（最後の行に
重なっていた。レビュー）。帯ごと留まるので、どこまでスクロールしても手が届く（要件 13）。

### Layout

左から:

1. **×**（`IconButton`、読み上げ名「Clear selection」）。押すと選択を解き、フォーカスを列の見出しの
   先頭のチェックへ移す
2. 件数「12 tags selected」（`text-lg font-semibold text-fg tabular-nums`、`role="status"`・
   `aria-live="polite"`。見出しの位置にあるので見出しに近い大きさ）。数は選んだ id の数で、読み込んだ
   行の部分集合である。先頭のチェックで選んだときは読み込んだ選べる行の数になり（「100 tags
   selected」）、条件に合う全部の数（タブの「Tags 1,000」、見出しに戻ったときの「500 of 1,000 tags」）
   より少なければ、残りを読み込んでいないことがそのまま読める
3. 右寄せで、まとめての操作を本物のボタン（`Button` の `md`）で並べる:
   - **「Confirm」**（primary、lucide `Check`）— 選んだ中に仮のタグがあるときだけ
   - **「Merge into one tag…」**（secondary、`Merge`）— いつも（統合元 1 件の統合は行の統合と同じ結果）
   - **「Reject…」**（secondary、`Ban`。文字とアイコンは `text-danger`）— 選んだ中に仮のタグがあるときだけ
   - **「Delete…」**（secondary、`Trash2`。文字とアイコンは `text-danger`）— 選んだ中に確定したタグが
     あるときだけ

幅が足りないとき（`sm` 未満など）は、ボタンの並びが件数の下の行へ折り返す（`flex-wrap`）。
ライブラリの「すべて選択」はここに置かない（列の見出しの先頭のチェックがその役）。

### Enabled and disabled

- 選んだタグに**働かない操作は出さない**（薄くしない）。仮と確定が混ざっていれば「Confirm」「Reject…」
  「Delete…」が全部出て、それぞれ働く分だけ処理し、外した数を伝える（Edge Case）。
- 上限は**選んだ数**に掛ける: 選んだ id の数が `maxTagBatch` を超えるときだけ、出ている操作を
  `disabled` にし、ライブラリの上限と同じ文言の理由（「Too many tags are selected to act on them
  together…」）を `title`・`aria-describedby` と、選択の行の下の `text-xs text-danger` の 1 行で添える。
  **読み込んだ行の数が上限を超えていても、選んだ数が上限の中なら操作は押せる**（1 行ずつ選んだ選択は、
  何行読み込んでいても操作できる。Plan「まとめての操作」、要件 8・9）。先頭のチェックは読み込んだ数で
  止まるので（「Column header」）、この状態に入るのは行のチェックで上限を超えて選んだときだけで、
  実質起きない。
- まとめての操作の送信中は、選択の行のすべてのボタンを `disabled` にし、「Confirm」の送信中は
  そのアイコンを `LoaderCircle`（`animate-spin motion-reduce:animate-none`）にする。続きの読み込み中は
  止めない（送る id は選んだ行で決まっていて、続きの到着に左右されない）。

### Bulk confirm

- 「Confirm」を押すとすぐ `POST /api/tags/batch`（`confirm`、選んだ id 全部）を 1 回送る。確認の窓は
  無い（要件 11）。
- 応答で、`appliedIds` の行を**読み込んだ行の中で** `tentative: false` に差し替え（目印と「確定する」が
  消える。一覧は取り直さない。R-12）、選択から外す。`notApplicableIds`（既に確定していたもの）は
  選んだまま残す。トースト「Confirmed 8 tags」、外した数があれば「Confirmed 8 tags. 4 were already
  confirmed.」。`notFoundIds` が空でなければ一覧を**先頭から**取り直し（スクロール位置は先頭、
  取り直しで `rows` に無くなった id は選択から外れる）、トースト「Some of the tags no longer existed,
  so the list was reloaded」。
- 「Tentative only」を押している間は確定した行が一覧から外れ、`total` が減る。読み込んでいない
  仮のタグは仮のまま残り、続きを読めば出てくる（受け入れ条件 10）。読み込んだ行が全部外れて
  `total` も 0 なら 031 の「No tentative tags」の空の状態になり、フォーカスは「Tentative only」へ
  （031「絞り込みから外れた行のフォーカス」。押しボタンは「Filter」の中に移ったので「Filter」へ）。
  読み込んだ行が全部外れたが `total` が 0 でない
  （読み込んでいない仮のタグが残っている）ときは、空の状態にせず、確定の反映と同時に `nextCursor` で
  続きを 1 回要求して次の行を出す（描く行が無いので仮想化のきっかけは働かず、明示して要求する。
  「Loading more」。カーソルは最後に読んだ行の位置なので、確定した行が消えても続きの位置は変わらない）。
  このとき選択の行は消えていて先頭のチェックも `disabled` なので、フォーカスは「Filter」へ移す。
  そうでなければ
  フォーカスは、選択が残っていて「Confirm」が出ていれば「Confirm」に、選択が残っていても「Confirm」が
  出なくなった（仮と確定を混ぜて選び、残った選択が確定したタグだけになった）ら「Merge into one tag…」に、
  選択が空になったら列の見出しの先頭のチェックへ移す。押せないボタンへフォーカスを置かない。
- 失敗（`5xx`・通信）はトーストで `errorText` を出し、何も変えず、選択は残る（Edge Case「途中で
  失敗したとき」）。

### Bulk reject and delete

選択の行の「Reject…」「Delete…」は `ModalFrame` の窓を開く。骨格は 014 の削除の窓と同じ（本文の
段落、secondary「Cancel」（最初のフォーカス）と danger「Reject」／「Delete」）。

- 開くと同時に `POST /api/tags/impact`（`reject` または `delete`、選んだ id 全部）を送る。届くまで
  本文は「Counting the affected videos…」の 1 行（`text-sm text-fg-muted`）と、その横に
  `LoaderCircle`。**届くまで danger のボタンは `disabled`**（数の無い確認で実行させない。
  [contracts/screen-api.md §3](contracts/screen-api.md#3-post-apitagsimpact)）。
- 届いたら本文を、`tagCount` が選んだ数と同じなら「全部に働く」の文、少なければ「一部に働く」の文
  （上の「Words」。選んだ数・働く数・外れる数・`videoCount` を埋める）にする。`videoCount` が 0 なら
  「aren't on any videos」の形で言い、「removed from 0 videos」とは言わない。本文の段落は 014 と同じ
  `border-l-2 border-danger-strong pl-3`。
- 数えられなかったとき（失敗）は本文に `text-sm text-danger`（`role="alert"`）の「Couldn't count the
  affected videos: {理由}」と `Button` の ghost・`sm`「Retry」を出し、danger のボタンは `disabled` の
  まま。
- 実行中は両方のボタンを `disabled`、danger のボタンに `LoaderCircle`（削除の窓と同じ）。
- `200` で窓を閉じ、`appliedIds` の行を読み込んだ行から消し（`total`・`totalAll` を減らす。一覧は
  取り直さない）、選択から外す。`notApplicableIds` は選んだまま残す。トースト「Rejected 8 tags」
  「Deleted 8 tags」、外した数があれば「… 4 confirmed tags were skipped.」「… 4 tentative tags were
  skipped.」。却下のあとは却下した名前の先頭の 1 ページを取り直す（タブの件数が増える）。
  `notFoundIds` が空でなければ一覧を先頭から取り直し、上と同じトースト。フォーカスは、消えた行の
  位置の次の行の「改名」（まとめている間は入口の `IconButton`）、無ければ前の行、1 つも無ければ
  「Tentative only」が効いていれば「Filter」、効いていなければ列の見出しの先頭のチェック（行が 1 つも
  残らず `disabled` なら「New tag」）へ移す（014・031 の削除・却下の規則を、まとめての操作に当てたもの）。
  消した分だけ末尾が上がって描く範囲が読み込んだ行の末尾に届けば、続きの読み込みが普通に始まる。
  読み込んだ行が 1 つも残らず `nextCursor` があれば、反映と同時に続きを要求する（「Loading more」）。
- 失敗は窓の中に `text-sm text-danger`（`role="alert"`）の 1 行で `errorText` を出し、窓は開いたまま、
  選択も残る。「Cancel」・Esc は何も変えずに閉じ、フォーカスを窓を開いたボタン（「Reject…」
  「Delete…」）へ戻す。

## Merge dialog

`MergeTagDialog` を、統合元を 1 件以上持つ 1 つの窓にする。行の「別のタグへ統合…」（統合元 1 件）と
選択の行の「Merge into one tag…」（統合元が選んだタグ）が同じ窓を開く。

### Width

- 窓は `ModalFrame` の `sm:max-w-lg`（`sm` 未満は全幅）。統合先の入力の枠と候補の一覧は窓の内側の幅
  いっぱい（`frameClassName="w-full"`。要件 14）。1 件の統合でも同じ。

### Target field and list

- 入力の上に**見える名札**「Tag to merge into」（`label`、`text-sm font-medium text-fg`）を置き、入力の
  枠の左端に lucide `Search`（`text-fg-subtle`）を置く。枠は本文の検索欄と同じ高さ（`h-9`・`text-sm`・
  `rounded-md`）。
- 候補は入力の上に重ねて開く一覧ではなく、入力の下の**高さを決めた箱**（`h-60 max-h-[40vh]`、
  `rounded-md border border-border p-1`、中を縦にスクロール）に**いつも**並べる（`Combobox` の
  `inline`）。箱の高さは候補の数によらず同じで、窓の下端のボタンを覆わない。候補の行は `min-h-9`・
  `rounded-md`、hover・矢印で選んでいる行は `bg-hover-wash`。
- **選んだ統合先は一覧の中で目立たせる**（`bg-accent-soft text-link`、名前を `font-medium`）。
- 候補が 1 つも無いとき（読み込み中でも失敗でもない）は、箱の中に `text-sm text-fg-muted` の
  「No matching tags」。
- 窓の下端（ボタンの行）の左に「**統合元 → 統合先**」を出す（統合先を選んだあとだけ。統合元が 1 件なら
  その名前、複数なら「3 tags」、`text-sm text-fg-muted`、統合先は `font-medium text-fg`、間は
  `ArrowRight`）。
- 「Merge」は **primary** の `Button`。統合先を選び、数が届くまでは `disabled`（primary の `disabled`、
  `opacity-50`）。押せない「Merge」を danger の赤で見せない（レビュー）。破壊的な操作であることは
  本文の `border-l-2 border-danger-strong` の段落が言う（014 と同じ）。
- Esc は候補の箱が開いたままでも窓を閉じる（箱は窓の本文の一部で、閉じる段は無い）。

### Sources

- 統合元が 1 件（行から）のときは今の形: 見出し「Merge "X"」、統合元の並びは出さず、確認の文言と
  本数（`videoCount`）は 014 のまま。
- 統合元が複数（選択の行から）のときは、見出し「Merge 4 tags」、入力の上に「Tags to merge」の
  見出し（`text-xs font-semibold text-fg-muted uppercase`。吹き出しの `legend` と同じ書式）と、
  統合元の名前の並び（`ul`、`flex flex-wrap gap-1.5`、各名前は 014 のシノニムの窓と同じ `bg-bg` の
  `Chip`、`h-6`・`text-xs`、× なし）。並びは `max-h-32 overflow-y-auto` で、数十個を超えても窓を
  押し広げない。仮のタグには行と同じ目印（`size-3`）を名前の後ろに付ける。
- **選んだ中のタグを統合先に選ぶと、そのタグは統合元から外れ**（Edge Case）、並びのそのチップに
  `text-fg-muted` の「kept」を添えて残し（消すと「選んだのに無い」と見える）、確認の文言の上に
  `text-sm text-fg-muted` の 1 行「"Action" is kept and the other 3 tags merge into it.」を出す。
- 統合元が統合先だけになったとき（選んだのが 1 件で、それを統合先に選んだとき）は、確認の文言の
  代わりに `text-sm text-fg-muted` の「Choose another tag to merge into: "Action" is the only tag
  selected.」を出し、「Merge」は `disabled`。

### Target candidates

統合先の候補は**全部のタグ**から出す（選んだ中からも、選んでいないタグからも、読み込んでいない
タグからも。要件 9）。候補は入力のたびに `GET /api/tags?q=〈入力〉&limit=8` で引く
（[research.md R-14](research.md#r-14-統合の窓の統合先の候補は-get-apitagsqlimit-で引く)、
[data-model.md §4](data-model.md#4-画面の側で持つ状態)「統合の窓の候補」）。

- **候補の行の中身は今の `Combobox` のまま**（名前、シノニムで当たったときの「Synonym: …」の補足、
  右端の本数、入力と完全に一致する名前・シノニムの `exactOption`、8 行まで）。並びはサーバーの
  名前の自然順で、画面では並べ直さない。行から開いたときは応答から統合元を除き、選んだ中から
  開いたときは選んだタグも候補に残す（merge 済みの形）。
- **窓を開いた直後**（入力が空）も同じ経路で先頭の 8 件を引き、届いたら箱に並べる（空の入力で候補が
  出る今の振る舞いを変えない）。
- **読み込み中**: `Combobox` の `busy`（枠の右端の `LoaderCircle`、`size-3`、`text-fg-muted`、
  `aria-busy`）。**前の候補はそのまま残し**、届いたら差し替える。候補の一覧を空にしたり閉じたり
  しない（入力のたびに一覧が消えて現れると、候補を目で追えない）。「Searching…」の行は置かない
  （回転で足りる。読み込みは 1 往復で、普段は入力と同時に見える）。進行中の要求は次の入力で
  打ち切り、最後の応答だけを候補にする。
- **候補が無い**（応答の `items` が空で、`exactOption` も無い）: 箱の中に「No matching tags」。
- **引けなかった**（失敗）: 入力の下に `Combobox` の `reason` と同じ書式（`mt-1 text-xs text-danger`）で
  「Couldn't search tags: {理由}」。最後に届いた候補は残し、次の入力で消して引き直す。「Retry」は
  置かない（入力を変えれば引き直すので、押すものを増やさない）。「Merge」は候補を選んでいなければ
  今までどおり押せない。
- 候補を選んだあとの振る舞い（数え直し、確認の文言）は下の「Confirmation」。入力の照合は一覧の検索と
  同じ照合形なので、「ａｃｔ」で「Action」が、読み込んでいなくても候補に出る（要件 9・7）。

### Confirmation

- 統合先を選ぶと、統合先を外した統合元について `POST /api/tags/impact`（`merge`）を送り、届くまで
  「Counting the affected videos…」と `LoaderCircle`、「Merge」は `disabled`。届いたら 014 と同じ
  `border-l-2 border-danger-strong` の段落に「The 120 videos tagged with these 4 tags get the tag
  "Action". …」を出す。文中のタグの数（「these 4 tags」「the 4 tags leave」）は**統合先を外した統合元の
  数**で、選んだ数ではない（4 個を選んでそのうち「Action」を統合先にすれば「these 3 tags」）。動画の
  本数は応答の `videoCount`。統合元が 1 件になったら 014 の 1 件の文言にする。数えられなかったときは
  まとめての却下と同じ「Couldn't count…」と「Retry」。統合先を選び直すたびに数え直す。
- 実行は `POST /api/tags/{id}/merge`（`sourceIds` = 統合先を外した統合元）。`200` で窓を閉じ、
  統合元の行を読み込んだ行から消し、統合先を応答の `tag` で書き換える（本数が合算に、仮なら確定に。
  一覧は取り直さない）。統合先が読み込んだ行に**あれば**その行を差し替えて並び順の位置へ動かし、
  **無ければ**（窓の候補から選んだ、読み込んでいないタグ）並び順の位置が読み込んだ範囲の中なら
  そこへ差し込み、範囲の外なら差し込まない（続きのページが返す。R-12）。選択は空にする。
  トーストの数は**実際に統合した数**（`sourceIds` の数から `notFoundIds` の数を引いたもの）で、
  「Merged 4 tags into "Action"」（1 件なら 014 の「Merged "X" into "Action"」）。フォーカスは統合先の
  行が読み込んだ行にあればその名前へ（014 と同じ）、無ければ（読み込んだ範囲の外、または
  「Tentative only」中で統合先が確定になった）031 の規則で、消えた最初の統合元の位置の次の行、
  無ければ列の見出しの先頭のチェック（行が 1 つも残らず `disabled` なら「New tag」）へ。読み込んだ行が
  1 つも残らず `nextCursor` があれば、反映と同時に続きを要求する（「Loading more」）。
- `notFoundIds` が空でないときは一覧を先頭から取り直す。統合元が**すべて**もう無かった
  （`notFoundIds` が `sourceIds` と同じ。応答の `tag` は変わっていない）ときは、統合のトーストを
  出さず、今の `tag_not_found` と同じ扱いにする（窓を閉じ、トースト「Some of the tags no longer
  existed, so the list was reloaded」、取り直し）。一部だけ無かったときは、実際に統合した数の
  トーストのあとに同じ取り直しのトーストを出す。
- 失敗・「Cancel」・Esc は 014 のまま（窓は開いたまま失敗を出す。閉じたらフォーカスは開いた元の
  「その他の操作」または選択の行の「Merge into one tag…」へ）。

## Rejected names tab

見出しの下のタブ「Rejected names」（上の「Tabs」）を選ぶと、本文（`role="tabpanel"`）に却下した名前を
並べる（[`web/src/tags/RejectedNames.tsx`](../../web/src/tags/RejectedNames.tsx)）。窓は開かない。中身は
`GET /api/tags/rejected-names` の**ページ**で受け、本文のスクロールで続きを読む
（[research.md R-13](research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)、
[contracts/screen-api.md §6](contracts/screen-api.md#6-get-apitagsrejected-names-のパラメータ)）。

- **中身の形**: 説明の 1 行（`text-sm text-fg-muted`「Automatic tagging won't create these names. Allow a
  name again to let it be created.」）と、名前の行の並び（`ul`、読み上げ名「Rejected names」、
  `divide-y divide-border`）。行は `min-h-12`・`px-2`、名前（`text-sm font-medium`、省略は `truncate` と
  `title`）と右端の secondary・`sm` の「Allow again」（読み上げ名「Allow "〈名〉" again」）。押すと即時に
  取り外す（確認なし、送信中は `disabled`、トーストなし）。フォーカスは次の行の「Allow again」→ 前の行
  → 無ければ「Rejected names」のタブ。取り外しの失敗は並びの下の `role="alert"` の 1 行。初回の読み込み中は
  行の高さの `Skeleton`（`h-10`）×4、初回に取れなかったときは「Couldn't load the rejected names」と
  「Retry」、無いときは中央に「No rejected names」。規則は 031「Rejected names」のまま（× のチップを
  行のボタンにしただけ）。
- **読み込み**: 画面を開いたときに先頭の 1 ページ（100 件）をタグの一覧と一緒に取り、タブの件数は
  応答の `total`。タブを開いたときはその 1 ページを出す（開くために読み直さない。受け入れ条件 13）。
- **続き**: 並びの末尾が表示域に近づくと（表示域を根にした番兵）、`nextCursor` で次の 100 件を引いて
  並びの末尾に足す（#730 の規則のまま。行は仮想化しない）。読み込み中は並びの下に行の高さの
  `Skeleton` ×3（`aria-hidden`）、並びの `ul` を `aria-busy`。同時に 1 つだけ送り、届くまで行のボタンは
  普通に押せる。取り直しの間に番兵が見えても、取り直しのあとで見張り直して続きを読む。
- **続きの失敗**: 並びの下に `text-sm text-danger`（`role="alert"`）の「Couldn't load more rejected
  names」と `Button` の ghost・`sm`「Retry」。読み込んだ名前は残し、「Retry」は同じカーソルで読み直す。
  読み込んだ名前をすべて外しても続きが残っていれば、空の文言ではなく続き（失敗の間は「Retry」）を出す。
- **取り外し**: `204` でその行を消し、タブの件数（`total`）を 1 減らす。一覧は取り直さない。取り外しの
  送信中に先頭のページの取り直しが重なったら、その応答は捨てて取り外しのあとで取り直す。
- **取り直し**のきっかけ（却下・作成・改名・シノニムの追加・まとめての却下のあと）は 031 のまま。
  取り直すのは**先頭の 1 ページだけ**。
- 検索・絞り込み・並び順は却下した名前に効かない（タグではない。031 と同じ）。このタブの間はトップバー
  に何も置かず、検索も置かない（サーバーの `GET /api/tags/rejected-names` は検索を持たず、足すのは
  この直しの範囲の外）。

## States

014・031 の「States」の表に足す・変える。

| 状態 | 見え方 |
| --- | --- |
| 条件（検索・絞り込み・並び順）を変えて先頭のページを待つ | 前の行と件数を残す。選択は空になり見出しの行に戻る。`Skeleton` には戻さない。届いたら差し替え、スクロール位置は先頭 |
| 「Unused only」で一致が無い（検索は空、「Tentative only」はオフ） | `EmptyState`（`VideoOff`）「No unused tags」、説明「Every tag is on at least one video.」、`Button`「Show all tags」。押すと絞り込みを外し、フォーカスを「Filter」へ移す。判定は応答の `total` が 0 |
| 「Unused only」と「Tentative only」の両方で一致が無い | `EmptyState`（`VideoOff`）「No unused tentative tags」、説明なし、`Button`「Show all tags」。押すと両方を外し、フォーカスを「Filter」へ |
| 「Tentative only」で一致が無い（031） | 031 の「No tentative tags」。「Show all tags」で外し、フォーカスを「Filter」へ（タグが 0 なら「New tag」へ） |
| 絞り込みと検索で一致が無い | `EmptyState`（`SearchX`）「No unused tags match "〈入力〉"」「No unused tentative tags match "〈入力〉"」、`Button`「Show all tags」。押すと絞り込みと検索の両方を外し、フォーカスを検索の入力へ |
| 続きを読み込んでいる | 一覧の末尾に行の `Skeleton` ×3、読み上げ「Loading more tags…」。出ている行の操作とスクロールは止まらない |
| 続きの読み込みに失敗した | 末尾に danger の箱「Couldn't load more: {理由}」と「Retry」。読み込んだ行は残る |
| 続きの応答で一覧が変わっていた（`totalAll` の食い違い） | 末尾に中立の箱「Tags were added or removed elsewhere…」と「Reload」。読み込んだ行と選択は残り、続きは止まる。「Reload」で先頭から読み直し、選択は空 |
| 末尾まで読み込んだ | 末尾に何も出さない |
| まとめての確定の送信中 | 選択の行のボタンがすべて `disabled`、「Confirm」のアイコンが `LoaderCircle` |
| まとめての却下・削除・統合の確認で数を待つ | 本文「Counting the affected videos…」と `LoaderCircle`、danger のボタンは `disabled` |
| 数えられなかった | 本文に `role="alert"` の「Couldn't count the affected videos: {理由}」と「Retry」、danger のボタンは `disabled` |
| 働かない種類を含めて実行した | 窓の本文で「8 of the 12 selected tags are …」と先に言い、実行後のトーストで「4 … were skipped.」。外した分は選んだまま残る |
| 対象の一部がもう無かった（`notFoundIds`） | 残りは処理し、トースト「Some of the tags no longer existed, so the list was reloaded」、一覧を先頭から取り直す。取り直しで `rows` に無くなった id の選択は外れる |
| まとめての操作が失敗した | 選択の行からの確定はトースト、窓からの操作は窓の中の 1 行。一覧と選択は変えない |
| 読み込んだ行の数が上限を超える | 先頭のチェックだけが `disabled`、理由を `title` と `sr-only` で。行のチェックと選択の行の操作は押せる |
| 選んだ数が上限を超える | 選択の行の操作が `disabled`、理由を `title`・`aria-describedby` と選択の行の下の 1 行で |
| 統合の窓で候補を引いている | 入力の右端に `LoaderCircle`、前の候補は残る |
| 統合の窓で候補を引けなかった | 入力の下に `text-xs text-danger`「Couldn't search tags: {理由}」、最後の候補は残る |
| 統合の窓で候補が無い | 候補の箱の中に「No matching tags」 |
| 却下した名前のタブで続きを読んでいる | 並びの下に行の `Skeleton` ×3 |
| 却下した名前のタブで続きを読めなかった | 並びの下に「Couldn't load more rejected names」と「Retry」、読み込んだ名前は残る |
| 読み込み失敗（一覧を持っている） | 今の一覧と件数を残し、帯の列の見出しの上に danger の箱「Couldn't load tags: {理由}…」と「Retry」。届くまで箱は残り、続きは読まない（「Stale list」、Edge Case）。一覧をまだ持っていなければ今の `EmptyState`（danger）と「Retry」 |

初回の読み込み中の `Skeleton`、「タグはまだありません」、「Tentative only」の空の状態、改名・作成・
1 件の操作の状態は 014・031 のまま。1 件の操作のあとも一覧は取り直さず、読み込んだ行の中で
書き換える（確定・却下・削除・改名・作成・統合。[data-model.md §4](data-model.md#4-画面の側で持つ状態)
「操作のあとの反映」）。利用者から見える違いは、操作のあとに `Skeleton` や行の点滅が無く、
スクロール位置が動かないことだけである。並び順・「Filter」・先頭のチェックは、先頭のページをまだ
受けていない間と一覧を持たないままの失敗で `disabled`。タブはいつも出し、件数は取れてから添える。

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md §4）。判定する幅は
360px（390px の端末を含む）・768px・1280px。トップバーの出し分けはライブラリの `LibraryToolbar` と
同じ。

| 幅 | トップバー | 帯 | 行 |
| --- | --- | --- | --- |
| 1280px | 検索（`sm:max-w-md`）→「Filter」（`xl` 以上で文字も）→ 並び順（メニュー＋向き） | 見出しの行（「Tags」・件数・「New tag」）→ タブ →（チップ）→ 列の見出し | チェック → 名前の列 → 本数 → `IconButton` 4 つ分の列（マウス）または「Actions」1 つ（タッチ） |
| 768px（`md` 以上） | 同上（「Filter」はアイコンと数だけ） | 同上 | 同上 |
| 360px（`md` 未満） | 検索（`flex-1`）→「Filter」（アイコンと数）→ 並び順のまとめ（`SlidersHorizontal`、読み上げ名「Sort」） | 同上。選択の行は、件数の下にボタンの並びが折り返す | チェック → 名前の列（約 160px）→ 本数（`w-16`）→「Actions」1 つ。横スクロールは出ない |

- `md` 未満の並び順のまとめは、ライブラリの「表示と並び順」と同じ吹き出し（`PopoverContent`、
  `align="end"`、`w-72`）で、中身は `CompactSortControls` と同じ形（上の「Top bar」）。
- 見出しの行は 360px でも 1 行（「Tags」・「1,000 of 1,000 tags」・「New tag」で約 300px）。件数は
  `truncate` で、折り返さない。タブ「Tags 1,000 | Rejected names 1,000」も 1 行（約 250px）。
- 帯の高さの見積り（1280px、チップなし）: `pt-3` ＋ 見出しの行 `h-10` ＋ タブ `h-10` ＋ 列の見出し `h-9`
  ＋ 間 `gap-3` ×2 で約 150px。1280×800 で上部バーと帯を引いた一覧の高さは約 600px で、シノニムの行を
  持たないタグなら約 12 行が見える。1 ページ 100 件はこの約 8 画面分で、開いた直後に続きを読む必要は無い。
- 窓（確認・統合）は `ModalFrame` の今の幅の扱い（`sm` 未満で全幅）。

## Review criteria

判定は実機で見て行う（library-ui.md §5）。「ある」だけでは満たさない（Q-4）。幅は 1280×800 を主に、
768px と 360px（タッチの端末またはデベロッパーツールのタッチの模擬）で確かめる。規模は
`tagsbench` の 1,000 個・3,000 個・30,000 個（[quickstart.md](quickstart.md)）で、見た目の判定は
30,000 個でも同じである。

- **ライブラリとの揃い**: トップバーの検索欄・「Filter」・並び順のボタンが、ライブラリの画面と同じ
  位置・同じ高さ・同じ見た目で並ぶ。見出しの「Tags」と右の件数はライブラリの「Library」と「N items」と
  同じ書式。本文の中に独自の操作の行は無い。
- **視覚的階層**: 1280×800 で管理画面を開いたとき、目が行く順は 行の名前 → 見出しと「New tag」→
  本数・シノニム・仮の目印 で、チェック（`opacity-40`）はそのあとに気づく程度である。1 件選ぶと、
  見出しの行が選択の行に替わり、選んだ行の薄い面（`accent/10`）と全行のチェックが前に出るが、名前の
  色・大きさは変わらない。×で解くと元に戻る。選択の行では「Confirm」だけが primary で、統合・却下・
  削除は secondary（却下・削除は danger の色の文字）。働かない操作は出ない。絞り込みが効いている間は
  「Filter」のボタンが `accent-soft` の面と数を持ち、見出しの下にチップが並ぶ。末尾の続きの状態
  （`Skeleton`・失敗・一覧が変わった）は行より前に出ず、失敗の箱だけが danger の色を持つ（UI品質
  「視覚的階層」「操作の優先順位」）。
- **情報密度**: 1280×800 で、シノニムの行を持たないタグなら帯の下に約 12 行が見える。行の高さは
  031 と同じ（チェックは `size-5` で `py-2` の行に収まり、行を伸ばさない）。選択の行は見出しの行と
  同じ高さで、選んでも一覧の行数は変わらない。件数にページの区切りは出ない。
- **余白のリズム**: 帯の中の見出しの行・タブ・チップ・列の見出しの間は `gap-3`。列の見出しの下の
  線は行の間の線と同じ色・太さ。チェックと名前の列の間は列どうしの `gap-2`（`sm:gap-3`）と同じで、
  列の見出しのチェック・「Name」・「Videos」が行のチェック・名前・本数の真上に揃う。スクロールして帯が
  留まったとき、帯の下を流れる行は帯に透けない。続きの `Skeleton` は行と同じ高さ・同じ `px-2` で、
  読み込んだ行の並びがそのまま続いているように見える。
- **タイポグラフィ**: 見出しと件数はライブラリと同じ書式。選択の行の件数は `text-lg font-semibold`、
  列の見出しとタブの数は `text-xs`／`text-sm` の `fg-muted`・`fg-subtle`。並び順のボタンの文字は種類の
  名前だけで、向きはアイコンで示す。
- **操作の優先順位**: 「Filter」→「Tentative only」、列の見出しの先頭のチェック、選択の行の「Confirm」の
  4 回で、読み込んだ仮のタグ全部が片付く（受け入れ条件 10。URL に `tentative=1` があれば 2 回）。
  却下は「Reject…」→ 数が出るのを待って「Reject」の 2 回で、確定より窓の分だけ多い。行の 1 件の操作は
  マウスの端末で今のまま 1 回で届く。タッチの端末では行の操作は「Actions」→ 項目の 2 回だが、各項目は
  文字で読める（UI品質「行の操作」）。
- **読み込んだ分と全部の見分け**: 「Tentative only」で `total` が 500 の一覧（「500 of 1,000 tags」）で
  先頭のチェックを押すと、選択の行は「100 tags selected」になり、先頭のチェックは選択の状態、読み上げ名は
  「Select all 100 loaded tags」→「Clear selection」。「Confirm」のあと、読み込んだ 100 行から仮の目印が
  消え、見出しの件数が「400 of 1,000 tags」になり、スクロールして続きを読むと残りの仮のタグが仮の
  まま出る（受け入れ条件 10、要件 10）。この間に画面が固まらない。
- **続きの読み込み**: 30,000 個の一覧を末尾へスクロールすると、読み込んだ最後の行の下に行の
  `Skeleton` が出てすぐ行に置き換わり、その間も帯は動かず、行のチェックと操作が押せる。同じタグが
  二重に出ない。
  続きの失敗では読み込んだ行が残り、danger の箱の「Retry」で続きが読める。別のタブでタグを作って
  から続きを読むと、中立の箱「Tags were added or removed elsewhere…」が出て続きが止まり、
  「Reload」で先頭から読み直す（Edge Case）。
- **並びと絞り込みの可視性**: 並び順のボタンの文字で今の種類が、その右の矢印で向きが分かる。
  「Unused only」をオンにすると見出しの件数が「90 of 1,000 tags」になり、チップ「Unused only」が出て、
  出る行の本数がすべて「0 videos」
  で、この数は読み込んでいないタグも含む（受け入れ条件 8）。「Video count」の多い順で先頭が最多、
  末尾まで読むと 0 本（受け入れ条件 5）。「Date created」の新しい順で「新しいタグ」を作ると、その行が
  一覧の先頭に入る（受け入れ条件 6）。並び順・絞り込み・検索・タブを変えて再読み込みしても、同じ
  条件で開く（受け入れ条件 7、「URL state」）。検索に「ＡＣＴＩＯＮ」と入れると、読み込まれていなかった「action」が
  出る（受け入れ条件 9）。条件を変えた瞬間に一覧が空や `Skeleton` にならず、前の行から新しい行へ
  置き換わる。
- **スクロール中の手の届き方**: 30,000 個の一覧の末尾までスクロールしても、トップバーの検索・
  「Filter」・並び順と、帯の見出し（選んでいれば選択の行）・タブ・列の見出しが見える（要件 13）。
  1,000 個の先頭でスクロールせずに「Rejected names」のタブを押すと本文に並ぶ（受け入れ条件 13）。
- **確認の数**: 仮 8 個と確定 4 個を選んで「Delete…」を開くと、「8 of the 12 selected tags are
  confirmed」ではなく「4 of the 12 selected tags are confirmed」と、確定した 4 個のどれかが付いた
  動画の本数（重複なし）が出る（受け入れ条件 12）。4 個を選んで「Action」へ統合すると、窓に 4 個の
  チップと統合先の入力が窓の幅いっぱいに出て、実行後に 4 個が消え、「Action」の本数が合算
  （重複なし）になる（受け入れ条件 11、要件 14）。統合先の入力に「ａｃｔ」と入れると、読み込んで
  いない「Action」が入力の下の箱に出て、入力中は右端に小さな回転が見え、候補が消えて現れたりしない
  （要件 9）。箱は窓の下端のボタンを覆わず、選んだ統合先が目立ち、下端に「統合元 → 統合先」が出る。
  統合先を選ぶまで「Merge」は primary の押せない形で、赤くない。
- **却下した名前のタブ**: 1,000 個の却下した名前でタブに「1,000」が出て、タブを開くと先頭の 100 個が
  行で並び、末尾までスクロールすると行の `Skeleton` が出て次の 100 個が足される。「Allow again」で外すと
  行が消え、タブの数が 1 減る（要件 12）。
- **キーボード**: Tab は トップバーの検索 →「Filter」→ 並び順のメニュー → 向き → 本文の「New tag」
  （選んでいれば選択の行の × → 操作）→ タブ → チップ →（「Stale list」の箱があればその「Retry」）→
  列の見出しの先頭のチェック → 行のチェック → 行の名前 → 行の操作 → … と進み、読み込んだ最後の行の
  あとは末尾の箱のボタン（「Retry」「Reload」があるとき）。`/` で検索へ移る。窓の Esc は窓だけを
  閉じる。1,000 個の一覧で、行の中を Tab で進め続けると描いている範囲の端を越えても
  次の行へ進み（帯や本文の外へ飛ばない）、Shift+Tab でも同じく前の行へ戻る。Tab で読み込んだ
  最後の行に届いたとき、続きがあれば（その行が描かれた時点で）続きの読み込みが始まっている
  （下の「Keyboard across virtualized rows」）。
- **要求を満たしたことにならない例**（UI品質）: 速くなっても、読み込んだタグをまとめて選んで
  確定する手が無い。選んでいないときからチェックや操作のボタンが名前より先に目に入る。絞り込みが
  効いているのに「Filter」のボタンにもチップにも出ず、今の状態が読めない。却下・削除・統合が確定と
  同じ primary の重さで見える。働かない操作が薄いボタンで並ぶ。選択の操作が最後の行に重なる。
  件数にページの区切り（読み込んだ数）が出る。却下した名前が一覧の下にあり、先頭から届かない。タッチの
  端末で行の操作がアイコンだけで、押すまで意味が分からない。1280×800 で 12 行に届かない。統合の
  窓の入力が窓の幅より明らかに狭い。条件を変えるたびに一覧が `Skeleton` に戻って点滅する。
  続きの読み込み中にスクロールや行の操作が止まる。先頭のチェックで選んだあと、選んだのが
  読み込んだ分だけだと件数から読めない。別のタブの変更で、黙って先頭に戻されて選択を失う。

## Keyboard across virtualized rows

見えている分だけ描く（R-2）と、描いている範囲の外の行は DOM に無いので、ブラウザの既定の Tab では
最後に描いた行から一覧の外へ飛ぶ。次の 2 つで、Tab の順を読み込んだ行の順のまま保つ（merge 済み）。

- **フォーカスのある行は描き続ける**: 仮想化の描く範囲（`rangeExtractor`）に、フォーカスを持つ行の
  位置を常に足す。スクロールで画面の外へ出ても、その行は外されず、フォーカスが `body` へ落ちない。
- **範囲の端の Tab を次の行へ渡す**: 一覧の包みの `keydown` で、Tab がその行の最後のフォーカスできる
  要素から押され、次の行が描かれていない（読み込んだ最後の行ではない）ときは、既定の動きを止め、
  次の行の位置へスクロールし（`scrollToIndex`。留めた帯の高さを差し引く。「Band」）、描かれたらその
  行のチェックへフォーカスを移す。Shift+Tab がその行の最初のフォーカスできる要素から押され、前の行が
  描かれていないときは、前の行へスクロールして、その行の最後のフォーカスできる要素（行の操作の
  入口）へ移す。読み込んだ最後の行から Tab、最初の行から Shift+Tab は既定のまま一覧の外（末尾の
  箱のボタン、帯の列の見出しの先頭のチェック）へ進む。
- **続きとの関係**: 読み込んだ最後の行へ Tab で届くと、その行が描かれるので続きの読み込みが
  始まる（「Loading more」のきっかけと同じ）。届いた行は読み込んだ最後の行の後ろに足されるので、
  利用者は続けて Tab で進める。読み込みが間に合わずに一覧の外へ出たら、Shift+Tab で戻る。

## Colour

- 新しい token は足さない。選んだ行の面は `accent` の 10%（`bg-accent/10`。`bg` の上に重ねるので、
  名前 `fg`・本数とシノニム `fg-muted` の対比は `bg` の上とほぼ同じ）。
- 「Filter」が効いている形とチップ（`link` on `accent-soft`）、選んだ統合先の候補（同じ組）、選択の行の
  文字（`fg` on `bg`）、選択の行の「Reject…」「Delete…」の文字（`danger` on `elevated`）、窓の中の失敗の
  行（`danger` on `elevated`）、帯の文字（`fg-muted` on `bg`）、続きの失敗の箱と帯の「Stale list」の箱
  （`danger` on `danger-soft`。ライブラリの `LoadMoreFailed` と同じ）、一覧が変わった箱（`fg` on
  `elevated`）は `pairs` に既にある。
- 仮の目印の `fg-subtle` はアイコンだけに使う（031 と同じ）。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前と役割だけを決める（014・031 と同じ範囲）。

- 行のチェック: 読み上げ名「Select "〈名〉"」。先頭のチェック: 「Select all 100 loaded tags」／「Clear
  selection」、中間の状態は `aria-checked="mixed"`、押せない理由は `aria-describedby`。
- 「Filter」: 読み上げ名「Filter」／「Filter (N applied)」、吹き出しの中は文字を持つ `checkbox`。チップは
  `ul`「Active filters」の中のボタン「Remove the filter "〈名〉"」。並び順のメニュー: `aria-label`
  「Sort by: 〈種類〉」、向きのボタンは上の「Words」。まとめのボタン: 「Sort」。
- タブ: `role="tablist"`「Tag lists」、`role="tab"` と `aria-selected`・`aria-controls`、パネルは
  `role="tabpanel"` と `aria-labelledby`。見出しの件数は `role="status"`。
- 続きの状態: 読み込み中は一覧の包みの `aria-busy` と `sr-only` の `role="status"`「Loading more
  tags…」、失敗の箱は `role="alert"`、一覧が変わった箱は `role="status"`。帯の「Stale list」の箱は
  `role="alert"`。
- 選択の行: `role="region"`「Selected tags」、件数は `role="status"`（`polite`）。上限で押せない理由は
  `title` と `aria-describedby`。
- 窓: 見出しは `ModalFrame` の `title`。数えている間の 1 行は `aria-busy`、失敗の行は `role="alert"`。
  統合の窓の入力は見える `label` を持ち、候補の読み込み中は `Combobox` の `aria-busy`。候補の箱は
  `role="listbox"`（いつも開いているので `aria-expanded="true"`）。
- 却下した名前のタブ: 並びの `ul`「Rejected names」、続きを読んでいる間は `aria-busy`。行のボタンの
  読み上げ名は「Allow "〈名〉" again」。
- 「Actions」のメニュー: 入口は `aria-label`「Actions」、項目は文字を持つ。
