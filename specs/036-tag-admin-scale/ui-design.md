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
に合わせて直している。8 とその影響（「読み込んだ行」を単位にする箇所）が、この改訂で足した部分
である。

1. 操作の行と件数の行を、一覧をスクロールしても上部バーの下に**留める**（要件 13）
2. 操作の行に**並び順**（名前・本数・作った日）と **0 本のタグだけ**の絞り込みを足す（要件 4〜6）
3. 行に**チェック**を足し、**選んでいる間だけ**出る選択バーから、まとめて確定・却下・削除・統合する
   （要件 8〜11）
4. まとめての却下・削除・統合の**確認の窓**（要件 11）と、複数の統合元を持つ**統合の窓**（要件 9・14）
5. **却下した名前**の入口を件数の行に移し、中身を窓で開く（要件 12）
6. 行の操作を、タッチの端末と狭い幅では**文字を持つメニュー**にまとめる（UI品質「行の操作」）
7. 一覧の行を見えている分だけ描く（要件 1・3）。**見た目は変えない**
8. 一覧を、**表示に要る分だけサーバーから読み、スクロールで続きを読む**（要件 1〜3・7）。検索・
   絞り込み・並び順は読み込んでいないタグも含めた全部に効き、一覧の末尾に**続きの読み込み中・
   失敗・食い違い**の行が加わる。件数の行の「すべて選ぶ」は**読み込んだ行**を選ぶ（要件 10）。
   却下した名前の窓と統合の窓の候補も、表示に要る分だけ読む（要件 12・9）

行の列と書式、作成の行、改名、シノニムの窓、1 件の確定・却下・削除・統合の規則、仮の目印、
「Tentative only」の振る舞い、却下した名前の中身の規則は変えない。新しい色・半径・影の token は
足さない（下の「Colour」）。

## Why this shape

- **操作の行と件数の行を `sticky` で留め、本文のスクロールは変えない。** 一覧を別の箱でスクロール
  させる形（本文は固定、一覧の中だけが動く）は、ブラウザの戻る・進むでのスクロール位置の復元と
  `/` のキーの扱いを変え、設定画面など他の本文の画面と動きが違ってしまう。留めるのは利用者が
  スクロール中に触るもの（検索・絞り込み・並び順・件数・却下した名前の入口）だけで、`h1` と
  一覧は今までどおり本文と一緒に流れる。
- **並び順はライブラリの「Sort by」のメニューと向きの切り替えと同じ部品で、同じ位置の規則に乗せる。**
  親 Issue の UI品質がこの形を名指ししていて、利用者はライブラリで既に覚えている。同じ `Button` の
  secondary にいまの種類の名前を出すので、「今どの並びか」はボタンの文字で分かる。並び順は 1 列の
  見出し（列をクリックして並べ替える表の形）にはしない。この一覧は表ではなく行の並びで、本数の列
  以外に見出しを置く場所が無いからである。
- **「0 本のみ」は「Tentative only」と同じ押しボタンで、同じ行に並べる。** 親 Issue は「『Tentative
  only』と並べて」と求める。ライブラリの絞り込みは吹き出しの中にあり、効いている数だけをボタンに
  出すが、ここでは絞り込みが 2 つしか無く、文字を持つ押しボタンで「何で絞っているか」がそのまま
  読める。片付けの作業（絞る → 全部選ぶ → 確定）が 1 回の押下で始まることも、吹き出しより
  押しボタンの方が短い。
- **検索・絞り込み・並び順は、見えている行の見た目を変えずにサーバーへ渡す。** 利用者から見える
  違いは、結果が「読み込んだ行の中」ではなく「全部のタグ」から出ることだけで（要件 4・6・7）、
  部品も位置も変えない。条件を変えたとき、新しい先頭のページが届くまで**前の行と件数を残す**
  （ライブラリの検索と同じ。空の一瞬や `Skeleton` の点滅を作らない）。失敗したら前の行を残して
  トーストで理由を出す（Edge Case「一覧の読み込みに失敗したときは…すでに一覧を持っていれば、その
  一覧を残す」）。
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
  集合」より狭いので、読み上げ名とバーの件数で**選んだのは読み込んだ分だけ**だと分かるようにする
  （下の「Count line」）。一覧の総数と選んだ数の違い（「500 of 1,000 tags」に対して「100 tags
  selected」）が、残りを読み込んでいないことの印になる。
- **選択はチェックで、まとめての操作はライブラリと同じ画面下部の選択バーに置く。** 選択バーは
  この製品で「選んでいる間だけ前に出る」形として既に決まっている（library-ui.md §6）。留めた
  操作の行の中に操作を差し込む案は、選んでいる間だけ操作の行の高さが変わり、一覧が上下に
  動いてチェックの位置がずれる。バーは `fixed` なので、要件 13 の「スクロールしても手が届く」も
  自然に満たす。チェックを常に薄く見せるのは、ライブラリのリスト表示と同じ規則で、hover できない
  端末でも選択の入口が見えるためである。
- **まとめての操作の中で前に出すのは「確定」だけ。却下・削除・統合は「More」のメニューに入れる。**
  親 Issue の UI品質は「片付けの主の操作は、選ぶことと、まとめての確定」「削除・統合・却下は今と
  同じく作成・確定より目立たせない」と求める。行では 014・031 がその 3 つを「その他の操作」の
  メニューに置いたので、バーでも同じ関係にする。確定は確認をとらず（要件 11）、却下・削除・統合は
  確認の窓を挟む。
- **「読み込んだものをすべて選ぶ」は件数の行の先頭のチェック。** 件数の行は「90 of 1,000 tags」と
  いう「今の条件に合う集合」を言う行で、その先頭のチェックは「この集合（のうち読み込んだ分）を
  選ぶ」と読める（メールの一覧の見出しのチェックと同じ形）。選択バーの「すべて選択」（ライブラリ）を
  ここにも置く案は、バーは 1 件選んだあとにしか出ないので、最初の 1 件を選ぶ手が別に要る。先頭の
  チェックなら、絞って 1 回押すだけで読み込んだ全部が選べる。
- **上限を超えて読み込んだ一覧で止めるのは、先頭のチェックだけ。** まとめての操作の上限
  （`maxTagBatch`、20,000 件）は送る id の数に掛かる（[research.md R-4](research.md#r-4-まとめての確定却下削除は-1-つの経路-post-apitagsbatch-が-1-つの取引で受け働かない無いタグは数えて飛ばす)、
  Plan「まとめての操作」）。読み込んだ行が上限を超えても、1 行ずつ選んだ数行のまとめての操作は
  上限に当たらないので、バーの「Confirm」「More」を止める理由は無い。止めるのは「読み込んだものを
  すべて選ぶ」だけで、理由は「読み込んだ数が多すぎる」と言い、絞り込みへ誘う。バーが止まるのは
  選んだ数そのものが上限を超えたときだけである（1 行ずつでは実質起きない）。
- **却下した名前は、件数の行の右端の入口から窓で開き、窓の中で続きを読む。** 031 が一覧の下に置いた
  折りたたみは、数千行の下では届かない（要件 12）。入口を件数の行（留まる行）の右端に置くのは、
  片付けの途中で毎回見るものではない従の情報を、主の操作（検索・絞り込み・並び順）と同じ行に
  並べないためである。中身は 031 の規則のまま窓（`ModalFrame`）で開く。吹き出しにしないのは、
  却下した名前が数千になりうるため、縦に伸びる中身を窓の中でスクロールさせ、そのスクロールで
  続きを読みたいからである（要件 12 の後半）。入口の件数は応答の `total` で、読み込んだ数ではない。
- **統合の窓の統合先の候補は、入力のたびにサーバーから引く。** 画面が持つのは読み込んだ行だけで、
  要件 9 は「統合先は…選んでいないタグからでも選べる」と求める
  （[research.md R-14](research.md#r-14-統合の窓の統合先の候補は-get-apitagsqlimit-で引く)）。候補の照合も
  一覧の検索と同じ照合形になり、全角・半角やかなの違いが同一視される。利用者から見える違いは、
  入力の右端に読み込み中の小さな回転と、候補が入力より一拍遅れて変わりうることだけで、候補の
  一覧の形は今の `Combobox` のまま。届くまで前の候補を残し、候補の一覧を空にして点滅させない。
- **行の操作は、タッチの端末と `sm` 未満の幅では、文字を持つ 1 つのメニューにまとめる。** 親 Issue の
  UI品質は「触るまで意味の分からないアイコンだけにならない」と求める。アイコンの横に文字を出す
  案は、行の幅を 4 つの操作の文字に取られ、名前の列が無くなる。メニューの各項目は文字を持ち、
  入口の「⋯」は現行の一覧の製品が「この行の操作」に収束させている形である。行で 1 回押しの確定は
  2 回押しになるが、タッチの端末での片付けの主の経路は「チェック → 確定（バー）」で、こちらは
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
| `sm` 未満の並び順のまとめのボタン | Sort |
| 0 本の絞り込みのボタン | Unused only |
| 0 本の絞り込みの補足（ツールチップ） | Show only tags that aren't on any videos |
| 件数の行（絞り込み・検索のどちらかが効いている） | 90 of 1,000 tags（今の形。`total` of `totalAll`） |
| 件数の行の読み込んだ数（続きがあるときだけ） | · 100 loaded |
| 件数の行の先頭のチェックの読み上げ名 | Select all 100 loaded tags ／ Clear selection（全部選んでいるとき） |
| 先頭のチェックが押せない理由（読み込んだ数が上限を超える） | Too many tags are loaded to select them all at once (limit {limit}). Narrow the list with search or a filter. |
| 行のチェックの読み上げ名 | Select "〈名〉" |
| 続きを読み込んでいる間（読み上げのみ） | Loading more tags… |
| 続きの読み込みに失敗した行 | Couldn't load more: {理由} ／ Retry |
| 一覧が変わった行 | Tags were added or removed elsewhere, so the rest of this list may be out of date. ／ Reload |
| 一致が無いときの見出し | No unused tags ／ No unused tentative tags ／ No unused tags match "〈入力〉" ／ No unused tentative tags match "〈入力〉" |
| 一致が無いときの説明（「Unused only」だけ、検索なし） | Every tag is on at least one video. |
| 一致が無いときのボタン | Show all tags（今の形） |
| 選択バーの読み上げ名（`region`） | Selected tags |
| 選択バーの件数 | 1 tag selected ／ 12 tags selected |
| 選択バーの操作 | Confirm ／ More ／ Clear selection（×） |
| 「More」のメニュー | Merge into one tag… ／ Reject… ／ Delete… |
| 押せない理由（`title`・読み上げ） | No tentative tags are selected ／ No confirmed tags are selected |
| 選んだ数が上限を超えたとき（バー） | Too many tags are selected to act on them together (limit {limit}). Clear some of the selection. |
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
| 統合の窓の統合先の入力（今の形） | Tag to merge into |
| 統合先の候補を引けなかったとき（入力の下） | Couldn't search tags: {理由} |
| 統合の窓の確認（複数。数は統合先を外した統合元） | The 120 videos tagged with these 4 tags get the tag "Action". Their names and synonyms become synonyms of "Action", and the 4 tags leave the tag list. This can't be undone. |
| 統合先を選んだ中から選んだとき | "Action" is kept and the other 3 tags merge into it. |
| 統合元が無くなったとき | Choose another tag to merge into: "Action" is the only tag selected. |
| 統合のトースト（複数。数は実際に統合した数） | Merged 4 tags into "Action" |
| 対象の一部がもう無かったとき | Some of the tags no longer existed, so the list was reloaded |
| 却下した名前の入口 | Rejected names 〈`total`〉（今の見出しの文字） |
| 却下した名前の窓の見出し | Rejected names |
| 却下した名前の窓で続きを読めなかったとき | Couldn't load more rejected names ／ Retry |
| 行の操作のまとめの入口（タッチ・`sm` 未満） | Actions |

- 「Unused」は 0 本のタグの呼び名で、文言の中で「tags that aren't on any videos」と説明する。
  「empty」「orphan」は使わない。
- 「loaded」は「画面がここまでに受け取った」の意味で、読み込んでいない残りがあることを含む。
  「shown」「visible」は使わない（見えている行は描いている行のことで、読み込んだ行とは別の集合）。
- 数はすべて `formatNumber`、件数の形はカタログの `tagCount`・`videos` を使う。タグの名前は利用者の
  データで、翻訳せずに埋め込む（i18n.md）。

## Toolbar

操作の行と件数の行を 1 つの帯にし、上部バーの下に留める。`h1`「Tags」は今のまま帯の上にあり、
本文と一緒に流れる。

### Band

- 帯は `position: sticky`、`top` は上部バーの高さの token（`top-navbar`）、面は `bg-bg`（不透明。
  下を流れる行が透けない）、`z-20`（選択バーの `z-30`、上部バーの `z-40` より下）。帯の下端に
  `border-b border-border` を引き、下の `divide-y` の行の並びの最初の線に見せる。帯の中の間隔は
  今の `mt-3`（`h1` との間）・`mt-2`（操作の行と件数の行の間）のまま、下に `pb-2`。
- 帯の高さは幅で変わる（下の「Responsive behaviour」）。仮想化のスクロール位置の計算は帯の高さを
  差し引く（行へフォーカスを移すときに、その行が帯の下に隠れない）。
- 「新しいタグ」を押したとき、一覧の先頭が帯の下に見えていなければ、先頭までスクロールしてから
  作成の行を差し込み、入力へフォーカスを移す（作成の行はいつも一覧の先頭で、見えない位置に
  入力を作らない）。

### Controls

操作の行（`lg` 以上。それ未満は「Responsive behaviour」）は左から、検索の入力（`flex-1`、
`sm:max-w-sm`）→ 「Tentative only」→ 「Unused only」→ 並び順（メニューと向き）→ 右端に
「新しいタグ」（primary）。本文で最初に Tab が届くのは今までどおり検索の入力で、`/` のキーも今のまま。

検索・「Tentative only」・「Unused only」・並び順のどれを変えても、画面はその条件で**先頭のページを
サーバーから読み直す**（[data-model.md §4](data-model.md#4-画面の側で持つ状態)「条件」）。届くまで前の
行と件数を残し、`Skeleton` に戻さない。届いたら行と件数を差し替え、スクロール位置は先頭へ戻す
（条件が変わったので前の位置に意味は無い）。進行中の続きの読み込みは打ち切り、古い条件の行は
混ざらない（Edge Case）。**どの変更でも選択は空になる**（Edge Case「選んでいる間に検索・絞り込み・
並び順を変えたとき: 選択は外す」。並び順の変更も含む。読み直したあとの行が同じ id を持っていても
選び直しはしない）。改名中の行は条件を変えても残る（下の「Row checkbox」）。

- **検索**: 部品・位置・`/`・Esc は今のまま（014）。照合はサーバーが全部のタグの名前とシノニムに
  掛け、ライブラリでタグを探すときと同じ規則で全角・半角やかなの違いを同一視する（要件 7、
  [contracts/screen-api.md §5](contracts/screen-api.md#5-get-apitags-のパラメータ) の `q`）。入力は
  100 文字で止める（`maxLength`。`q` の上限と同じ）。
- **「Unused only」**: 「Tentative only」と同じ部品・同じ押している形（`Button` の secondary、
  `aria-pressed`、押している間は `border-accent-active`・`bg-accent-soft`・`text-link`）。アイコンは
  lucide `VideoOff`（行の本数の列が「videos」なので、「動画が無い」を同じ語で描く。`CircleDashed` は
  仮の目印なので使わない）。文言「Unused only」、ツールチップ「Show only tags that aren't on any
  videos」。押すと `unused=true` で読み直し、本数 0 のタグだけになる。「Tentative only」・検索・
  並び順と重ねられ（両方の押しボタンがオンなら両方を満たすタグ。要件 6）、読み込んでいないタグも
  含めた全部に効く。この画面の状態で、URL・`localStorage` には載せない（R-7）。`disabled` の規則は
  「Tentative only」と同じ（先頭のページをまだ一度も受けていない間・一覧を持たないまま読み込みに
  失敗したとき・押していない間に `totalAll` が 0 のとき。**押している間は `total` が 0 になっても
  `disabled` にしない**）。
- **並び順**: ライブラリの `SortMenu` と同じ形。`Button` の secondary にいまの種類の名前
  （「Name」「Video count」「Date created」）と `ChevronDown`、読み上げ名「Sort by: 〈種類〉」。メニューは
  見出し「Sort by」と 3 つのラジオ項目。アイコンは Name が `ArrowDownAZ`（ライブラリの「Title」と
  同じ、名前の順の意味）、Video count が `Hash`（数）、Date created が `CalendarPlus`（タグが
  できた日。ライブラリの「Date created」の `FileClock` はファイルの作成日で別のものなので、同じ
  絵にしない）。
  - 種類を選ぶと、その種類の既定の向きになる: Video count は多い順（`countDesc`）、Date created は
    新しい順（`createdDesc`）。Name に向きは無い（要件 4）。
  - **向きの切り替え**は、ライブラリと同じくメニューのボタンの右に接する `Button`
    （`rounded-l-none px-2.5`）で、`ArrowDownWideNarrow`（降順）／`ArrowUpNarrowWide`（昇順）、
    読み上げ名とツールチップは上の「Words」。**Name のときは向きのボタンを出さない**（メニューの
    ボタンは全周の角丸に戻る）。`disabled` の向きのボタンを残す案は、押せないものを置く理由が
    「幅を揺らさない」だけで、Q-5 の価値にならない。
  - 並び順は読み込んでいないタグも含めた全部に効く（要件 4。サーバーが並べ、画面は並べ直さない）。
    同じ値のタグどうしは名前の自然順（R-10）。選んだ並び順は `localStorage` に残り（R-7）、画面を
    開くとその並びで始まる。壊れていれば Name。
  - `disabled` の規則は「Tentative only」と同じ。
- **「Tentative only」「新しいタグ」**は今のまま（031・014）。「Tentative only」は `tentative=true` で
  読み直す形になるだけで、見た目と `disabled` の規則は変えない。

### Count line

件数の行は帯の中の最後の行で、左から、**先頭のチェック** → 件数 → 右端に**却下した名前の入口**。
高さは `h-5`（チェックの大きさ）に合わせ、文字は今の `text-xs text-fg-muted tabular-nums`。

- **先頭のチェック**: `Checkbox`（`size-5`）。行のチェックと同じ列の位置に置き（行の `px-2` と
  同じ左の余白）、縦に並ぶ。見え方は行のチェックと同じ規則（下の「Rows」の「Row checkbox」。
  選んでいない間は `opacity-40`、帯に hover するか選んでいる間は `opacity-100`）。対象は**読み込んだ
  行**（`rows`）のうち**選べる行**で、改名中の行は入らない（その行のチェックは `disabled`。下の
  「Row checkbox」）。読み込んでいないタグは選ばれない（要件 10）。状態は 3 つ: 選べる行を 1 つも
  選んでいなければ空、一部なら中間（lucide `Minus`）、全部なら選択。空と中間で押すと選べる行を
  すべて選び、全部のときに押すと選択を解く。読み上げ名は空・中間で「Select all 100 loaded tags」
  （数は読み込んだ選べる行の数）、全部で「Clear selection」。続きが届いて読み込んだ行が増えると、
  全部の状態は中間に戻る（新しく届いた行は選んでいない。押せばそれらも選ぶ）。読み込んだ行が
  無いとき（空の状態）・先頭のページをまだ受けていない間・一覧を持たないまま失敗したとき・
  読み込んだ行の数が上限（`maxTagBatch`）を超えるときは `disabled`。上限のときは、理由
  「Too many tags are loaded to select them all at once…」を包みの `title` と `aria-describedby` の
  `sr-only` で添える（ライブラリの上限と同じ形）。この理由はバーには出さない（下の「Enabled and
  disabled」）。
- **件数**: 応答の `total`・`totalAll` で出す（[contracts/screen-api.md §5](contracts/screen-api.md#5-get-apitags-のパラメータ)）。
  形は今のまま「1,000 tags」、絞り込み・検索のどちらかが効いていれば「90 of 1,000 tags」
  （受け入れ条件 8。読み込んでいないタグも数えた数）。**続きがある**（`nextCursor` がある）ときだけ、
  その後ろに同じ書式で「· 100 loaded」（読み込んだ行の数）を添え、末尾まで読んだら消す。これで
  「今の条件に合う数」と「このページが持っている数」が同じ行で読め、先頭のチェックが選ぶ範囲も
  この数である。改名中の残した行は数えない（今と同じ）。選んだ数はここに出さない（選択バーが
  出す）。操作のあとの増減は局所で数え直す（[data-model.md §4](data-model.md#4-画面の側で持つ状態)
  「操作のあとの反映」）。先頭のページが届く前は今の読み込み中のまま。
- **却下した名前の入口**: `Button` の ghost・`sm`（`h-8` だが帯の高さは伸ばさず、`-my-1.5` で
  件数の行に収める）、文字「Rejected names」（`text-xs`）と、後ろに `tabular-nums` の件数
  （応答の `total`。031 の見出しの件数と同じ規則で、取れるまでは出さない）。アイコンは付けない
  （文字が意味を持つ）。押すと下の「Rejected names」の窓を開く。先頭のページをまだ受けていない間・
  一覧を持たないまま失敗したときは出さない（031 と同じ）。「タグはまだありません」のときは出す。

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
  「続きを読むきっかけ」）、利用者が末尾に届く前に読み始める。届いた行は末尾に足し、`Skeleton` は
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
  （件数の行の「· 100 loaded」が消えることが印になる）。

末尾の状態は同時に 1 つだけで、読み込み中 → 失敗、読み込み中 → 一覧が変わった、のどちらかに
変わる。条件を変えて先頭から読み直すと、どの状態も消える。

### Row checkbox

- 名前の列の**左**に `Checkbox`（`size-5`）を `gap-2`（`sm:gap-3`）で置く。押す範囲は `size-8` の
  正方形（チェックを中央に置いた包み）で、タッチでも外さない。チェックを押しても名前のリンクへは
  移らない。
- 見え方はライブラリのリスト表示の規則: 1 件も選んでいない間は `opacity-40`、その行に hover
  するかフォーカスが入ると `opacity-100`、1 件でも選んでいる間はすべての行で `opacity-100`。
  hover できない端末では `opacity-40` のまま押せる（薄くても見えている）。
- 選んだ行は面を `bg-accent-soft` にする（リスト表示の選んだ行と同じ）。名前（`text-fg`）と本数・
  シノニム（`text-fg-muted`）の色は変えない。改名中の行は改名の面（`bg-elevated` と
  `ring-control-border`）が勝ち、その行のチェックは `disabled`（改名の確定で行が並び順の別の位置へ
  動きうるため、改名の間は選ばない）。選んでいる行の改名を始めると、その行を選択から外す（バーの
  件数が減り、空になればバーが消える）。まとめての操作が改名中のタグに働くことは無い。
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

1 件でも選ぶと画面下部に出る。箱・位置・現れ方・段の折り返しの仕組みはライブラリの `SelectionBar`
と同じ（`fixed inset-x-0 bottom-4`、`bg-elevated`・`border-border-strong`・`rounded-md`・
`shadow-elevated`、`sm` 以上で `h-11` の 1 段、`animate-slide-up`）。`role="region"`、読み上げ名
「Selected tags」。選択が空になると消える。バーが出ている間は、本文の下端に `pb-16` を足し、最後の
行（と末尾の続きの状態）がバーの下に隠れたままにならないようにする。

### Layout

`sm` 以上は 1 段、左から:

1. 件数「12 tags selected」（`text-sm text-fg tabular-nums`、`role="status"`・`aria-live="polite"`）。
   数は選んだ id の数で、読み込んだ行の部分集合である。先頭のチェックで選んだときは読み込んだ
   選べる行の数になり（「100 tags selected」）、件数の行の `total`（「500 of 1,000 tags」）より
   少なければ、残りを読み込んでいないことがそのまま読める
2. **「Confirm」**（`Button` ghost・`sm`、lucide `Check`）
3. **「More」**（`Button` ghost・`sm`、lucide `Ellipsis`、`ChevronDown` なし。文字を持つのはタッチで
   読めるため。library-ui.md §6「操作名を短縮しない」）。メニューは「Merge into one tag…」
   （`Merge`）→ 区切り線 →「Reject…」（`Ban`、danger）→「Delete…」（`Trash2`、danger）
4. 区切り線（`h-5 w-px bg-border-strong`）
5. **×**（`IconButton` `sm`、読み上げ名「Clear selection」）

ライブラリの「すべて選択」はここに置かない（件数の行の先頭のチェックがその役）。`sm` 未満は
ライブラリと同じく 2 段で、上段に件数と ×、下段に「Confirm」と「More」を半分ずつ。

### Enabled and disabled

- 「Confirm」は選んだ中に仮のタグが 1 つも無ければ `disabled`、`title` と `aria-describedby` に
  「No tentative tags are selected」。仮と確定が混ざっていれば押せる（働く分だけ処理し、外した数を
  伝える。Edge Case）。
- 「More」の「Reject…」は同じ規則で仮が無ければ `disabled`（項目の `aria-disabled`）、「Delete…」は
  確定したタグが 1 つも無ければ `disabled`（「No confirmed tags are selected」）。「Merge into one
  tag…」は 1 件でも選んでいれば押せる（統合元 1 件の統合は行の統合と同じ結果になる）。
- 上限は**選んだ数**に掛ける: 選んだ id の数が `maxTagBatch` を超えるときだけ「Confirm」「More」を
  `disabled` にし、ライブラリの上限と同じ形で理由（「Too many tags are selected to act on them
  together…」）を `title` と `sr-only` で添える。**読み込んだ行の数が上限を超えていても、選んだ数が
  上限の中ならバーは押せる**（1 行ずつ選んだ選択は、何行読み込んでいても操作できる。Plan
  「まとめての操作」、要件 8・9）。先頭のチェックは読み込んだ数で止まるので（「Count line」）、
  この状態に入るのは行のチェックで上限を超えて選んだときだけで、実質起きない。
- まとめての操作の送信中は、バーのすべてのボタンを `disabled` にし、「Confirm」の送信中は
  そのアイコンを `LoaderCircle`（`animate-spin motion-reduce:animate-none`）にする。続きの読み込み中は
  バーを止めない（送る id は選んだ行で決まっていて、続きの到着に左右されない）。

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
  （031「絞り込みから外れた行のフォーカス」）。読み込んだ行が全部外れたが `total` が 0 でない
  （読み込んでいない仮のタグが残っている）ときは、空の状態にせず、`nextCursor` で続きを読んで
  次の行を出す（描く行が無くなれば「Loading more」のきっかけがそのまま働く。カーソルは最後に
  読んだ行の位置なので、確定した行が消えても続きの位置は変わらない）。そうでなければ
  フォーカスは、バーが残っていて「Confirm」が押せれば「Confirm」に、バーが残っていても「Confirm」が
  `disabled` になった（仮と確定を混ぜて選び、残った選択が確定したタグだけになった）ら「More」に、
  バーが消えたら件数の行の先頭のチェックへ移す。押せないボタンへフォーカスを置かない。
- 失敗（`5xx`・通信）はトーストで `errorText` を出し、何も変えず、選択は残る（Edge Case「途中で
  失敗したとき」）。

### Bulk reject and delete

「More」の「Reject…」「Delete…」は `ModalFrame` の窓を開く。骨格は 014 の削除の窓と同じ（本文の
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
  skipped.」。却下のあとは却下した名前の先頭の 1 ページを取り直す（入口の件数が増える）。
  `notFoundIds` が空でなければ一覧を先頭から取り直し、上と同じトースト。フォーカスは、消えた行の
  位置の次の行の「改名」（まとめている間は入口の `IconButton`）、無ければ前の行、1 つも無ければ
  「Tentative only」を押していればそのボタン、押していなければ件数の行の先頭のチェックへ移す
  （014・031 の削除・却下の規則を、まとめての操作に当てたもの）。消した分だけ末尾が上がって描く
  範囲が読み込んだ行の末尾に届けば、続きの読み込みが普通に始まる。
- 失敗は窓の中に `text-sm text-danger`（`role="alert"`）の 1 行で `errorText` を出し、窓は開いたまま、
  選択も残る。「Cancel」・Esc は何も変えずに閉じ、フォーカスを「More」へ戻す。

## Merge dialog

`MergeTagDialog` を、統合元を 1 件以上持つ 1 つの窓にする。行の「別のタグへ統合…」（統合元 1 件）と
バーの「Merge into one tag…」（統合元が選んだタグ）が同じ窓を開く。

### Width

- 統合先の `Combobox` の枠は窓の内側の幅いっぱい（`frameClassName="w-full"`。今の既定 `w-40` は
  再生画面の「タグを追加」の幅で、窓の中では極端に小さい。要件 14）。候補の一覧も枠と同じ幅で開く
  （`listClassName="w-full"`。包みの `relative` の幅に一覧の `absolute` を合わせる。指定しない
  呼び手（再生画面の「タグを追加」など）は今の `w-64` のまま）。1 件の統合でも同じ。窓の幅は今の
  `ModalFrame` の既定（`sm:max-w-2xl`、`sm` 未満は全幅）。

### Sources

- 統合元が 1 件（行から）のときは今の形: 見出し「Merge "X"」、統合元の並びは出さず、確認の文言と
  本数（`videoCount`）は 014 のまま。
- 統合元が複数（バーから）のときは、見出し「Merge 4 tags」、入力の上に「Tags to merge」の
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

- **候補の一覧の形は今の `Combobox` のまま**（名前、シノニムで当たったときの「Synonym: …」の補足、
  右端の本数、入力と完全に一致する名前・シノニムの `exactOption`、8 行まで）。並びはサーバーの
  名前の自然順で、画面では並べ直さない。行から開いたときは応答から統合元を除き、選んだ中から
  開いたときは選んだタグも候補に残す（merge 済みの形）。
- **窓を開いた直後**（入力が空）も同じ経路で先頭の 8 件を引き、入力にフォーカスが入れば今までどおり
  一覧を開く（空の入力で候補が出る今の振る舞いを変えない）。
- **読み込み中**: `Combobox` の `busy`（枠の右端の `LoaderCircle`、`size-3`、`text-fg-muted`、
  `aria-busy`）。**前の候補はそのまま残し**、届いたら差し替える。候補の一覧を空にしたり閉じたり
  しない（入力のたびに一覧が消えて現れると、候補を目で追えない）。「Searching…」の行は置かない
  （回転で足りる。読み込みは 1 往復で、普段は入力と同時に見える）。進行中の要求は次の入力で
  打ち切り、最後の応答だけを候補にする。
- **候補が無い**（応答の `items` が空で、`exactOption` も無い）: 今の `Combobox` と同じく一覧を出さない。
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
  無ければ件数の行の先頭のチェックへ。
- `notFoundIds` が空でないときは一覧を先頭から取り直す。統合元が**すべて**もう無かった
  （`notFoundIds` が `sourceIds` と同じ。応答の `tag` は変わっていない）ときは、統合のトーストを
  出さず、今の `tag_not_found` と同じ扱いにする（窓を閉じ、トースト「Some of the tags no longer
  existed, so the list was reloaded」、取り直し）。一部だけ無かったときは、実際に統合した数の
  トーストのあとに同じ取り直しのトーストを出す。
- 失敗・「Cancel」・Esc は 014 のまま（窓は開いたまま失敗を出す。閉じたらフォーカスは開いた元の
  「その他の操作」またはバーの「More」へ）。

## Rejected names

入口は上の「Count line」。押すと `ModalFrame`（`sm:max-w-lg`）の窓「Rejected names」を開く。
中身は `GET /api/tags/rejected-names` の**ページ**で受け、窓の中で続きを読む
（[research.md R-13](research.md#r-13-却下した名前は-get-apitagsrejected-names-のページで受け窓の中で続きを読む)、
[contracts/screen-api.md §6](contracts/screen-api.md#6-get-apitagsrejected-names-のパラメータ)）。

- **中身の形**は 031「Rejected names」の規則のまま: 説明の 1 行、名前の並び（`Chip` に ×。窓の面は
  `bg-elevated` なので、014 のシノニムの窓と同じく `bg-bg` の面にする）、× で即時に取り外し
  （確認なし、送信中は `disabled`、トーストなし、フォーカスは次の × → 前の × → 無ければ「Close」）、
  失敗の 1 行、初回の読み込み中の `Skeleton`（`h-6 w-24` ×3）、初回に取れなかったときの「Couldn't
  load the rejected names」と「Retry」、無いときの「No rejected names」。
- **読み込み**: 画面を開いたときに先頭の 1 ページ（100 件）をタグの一覧と一緒に取り、入口の件数は
  応答の `total`。窓を開いたときはその 1 ページを出す（窓を開くために読み直さない。1,000 個の先頭から
  スクロールせずにすぐ開ける。受け入れ条件 13）。
- **続き**: 窓の中身（`overflow-y-auto` の箱）を末尾までスクロールすると、`nextCursor` で次の
  100 件を引いて並びの末尾に足す（箱を根にした番兵。チップの並びは仮想化しないので、ライブラリの
  番兵の形でよい）。読み込み中は並びの末尾に `Skeleton`（`h-6 w-24`）を 3 つ（`aria-hidden`）、
  並びの `ul` を `aria-busy`。同時に 1 つだけ送り、届くまで × は普通に押せる。
- **続きの失敗**: 並びの下に `text-xs text-danger`（`role="alert"`）の「Couldn't load more rejected
  names」と `Button` の ghost・`sm`「Retry」（初回の失敗と同じ形）。読み込んだ名前は残し、「Retry」は
  同じカーソルで読み直す。
- **取り外し**: × の `204` でそのチップを消し、入口の件数（`total`）を 1 減らす。一覧は取り直さない。
- **取り直し**のきっかけ（却下・作成・改名・シノニムの追加・まとめての却下のあと）は 031 のまま。
  取り直すのは**先頭の 1 ページだけ**で、窓が開いていれば並びが先頭の 1 ページに戻る（スクロール
  位置も先頭へ。取り直しは自分の操作の直後で、窓は普段閉じている）。
- 窓の下端に secondary「Close」。最初のフォーカスは最初の名前の ×、無ければ「Close」。Esc と × で
  閉じ、フォーカスは入口へ戻る。開閉はこの画面の状態で、URL には載せない。閉じても読み込んだ
  続きは捨てず、開き直せば同じ並びが出る（次の取り直しまで）。
- 一覧の下の折りたたみ（031）は置かない。入口が帯にあるので、1,000 個の先頭からスクロールせずに
  開ける（受け入れ条件 13）。
- 検索・絞り込みは窓の中身に効かない（タグではない。031 と同じ）。窓の中に検索は置かない
  （親 Issue に無い）。

## States

014・031 の「States」の表に足す・変える。

| 状態 | 見え方 |
| --- | --- |
| 条件（検索・絞り込み・並び順）を変えて先頭のページを待つ | 前の行と件数を残す。選択は空になりバーが消える。`Skeleton` には戻さない。届いたら差し替え、スクロール位置は先頭 |
| 「Unused only」で一致が無い（検索は空、「Tentative only」はオフ） | `EmptyState`（`VideoOff`）「No unused tags」、説明「Every tag is on at least one video.」、`Button`「Show all tags」。押すと絞り込みを外し、フォーカスを「Unused only」へ移す。判定は応答の `total` が 0 |
| 「Unused only」と「Tentative only」の両方で一致が無い | `EmptyState`（`VideoOff`）「No unused tentative tags」、説明なし、`Button`「Show all tags」。押すと両方を外し、フォーカスを「Tentative only」へ |
| 絞り込みと検索で一致が無い | `EmptyState`（`SearchX`）「No unused tags match "〈入力〉"」「No unused tentative tags match "〈入力〉"」、`Button`「Show all tags」。押すと絞り込みと検索の両方を外し、フォーカスを検索の入力へ |
| 続きを読み込んでいる | 一覧の末尾に行の `Skeleton` ×3、読み上げ「Loading more tags…」。出ている行の操作とスクロールは止まらない |
| 続きの読み込みに失敗した | 末尾に danger の箱「Couldn't load more: {理由}」と「Retry」。読み込んだ行は残る |
| 続きの応答で一覧が変わっていた（`totalAll` の食い違い） | 末尾に中立の箱「Tags were added or removed elsewhere…」と「Reload」。読み込んだ行と選択は残り、続きは止まる。「Reload」で先頭から読み直し、選択は空 |
| 末尾まで読み込んだ | 末尾に何も出さない。件数の行の「· 100 loaded」が消える |
| まとめての確定の送信中 | バーのボタンがすべて `disabled`、「Confirm」のアイコンが `LoaderCircle` |
| まとめての却下・削除・統合の確認で数を待つ | 本文「Counting the affected videos…」と `LoaderCircle`、danger のボタンは `disabled` |
| 数えられなかった | 本文に `role="alert"` の「Couldn't count the affected videos: {理由}」と「Retry」、danger のボタンは `disabled` |
| 働かない種類を含めて実行した | 窓の本文で「8 of the 12 selected tags are …」と先に言い、実行後のトーストで「4 … were skipped.」。外した分は選んだまま残る |
| 対象の一部がもう無かった（`notFoundIds`） | 残りは処理し、トースト「Some of the tags no longer existed, so the list was reloaded」、一覧を先頭から取り直す。取り直しで `rows` に無くなった id の選択は外れる |
| まとめての操作が失敗した | バーからの確定はトースト、窓からの操作は窓の中の 1 行。一覧と選択は変えない |
| 読み込んだ行の数が上限を超える | 先頭のチェックだけが `disabled`、理由を `title` と `sr-only` で。行のチェックとバーは押せる |
| 選んだ数が上限を超える | 「Confirm」「More」が `disabled`、理由を `title` と `sr-only` で |
| 統合の窓で候補を引いている | 入力の右端に `LoaderCircle`、前の候補は残る |
| 統合の窓で候補を引けなかった | 入力の下に `text-xs text-danger`「Couldn't search tags: {理由}」、最後の候補は残る |
| 却下した名前の窓で続きを読んでいる | 並びの末尾にチップの `Skeleton` ×3 |
| 却下した名前の窓で続きを読めなかった | 並びの下に「Couldn't load more rejected names」と「Retry」、読み込んだ名前は残る |
| 読み込み失敗（一覧を持っている） | 今の一覧を残し、トーストで `errorText`（Edge Case）。一覧をまだ持っていなければ今の `EmptyState`（danger）と「Retry」 |

初回の読み込み中の `Skeleton`、「タグはまだありません」、「Tentative only」の空の状態、改名・作成・
1 件の操作の状態は 014・031 のまま。1 件の操作のあとも一覧は取り直さず、読み込んだ行の中で
書き換える（確定・却下・削除・改名・作成・統合。[data-model.md §4](data-model.md#4-画面の側で持つ状態)
「操作のあとの反映」）。利用者から見える違いは、操作のあとに `Skeleton` や行の点滅が無く、
スクロール位置が動かないことだけである。並び順・「Unused only」・先頭のチェック・却下した名前の
入口は、先頭のページをまだ受けていない間と一覧を持たないままの失敗で `disabled`（入口は出さない）。

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md §4）。判定する幅は
360px・768px・1280px。

| 幅 | 帯（操作の行・件数の行） | 行 | 選択バー |
| --- | --- | --- | --- |
| 1280px（`lg` 以上） | 1 行: 検索（`max-w-sm`）→「Tentative only」→「Unused only」→ 並び順（メニュー＋向き）→ 右端「新しいタグ」。その下に件数の行。帯は約 75px | チェック → 名前の列 → 本数 → `IconButton` 3〜4 つ（マウス）または「Actions」1 つ（タッチ）。末尾の続きの状態は行と同じ幅 | 1 段 |
| 768px（`sm` 以上 `lg` 未満） | 2 行: 1 行目は検索（`flex-1`、`max-w-sm`）→ 右端「新しいタグ」、2 行目は「Tentative only」→「Unused only」→ 並び順（メニュー＋向き）。その下に件数の行。帯は約 120px | 同上 | 1 段 |
| 360px（`sm` 未満） | 2 行: 1 行目は検索（`flex-1`）→「新しいタグ」、2 行目は「Tentative only」→「Unused only」（アイコンを出さず文字だけ。`max-sm:hidden`）→ 右端に並び順のまとめ（`IconButton` 相当の `Button` secondary `px-2.5`、`SlidersHorizontal`、読み上げ名「Sort」）。その下に件数の行（収まらなければ却下した名前の入口を次の行へ送る。merge 済み）。帯は約 120px | チェック → 名前の列（約 160px）→ 本数（`w-16`）→「Actions」1 つ。横スクロールは出ない。続きの失敗・食い違いの箱は文字とボタンが折り返す（`flex-wrap`） | 2 段（上段: 件数と ×、下段: 「Confirm」「More」を半分ずつ） |

- `sm` 未満の並び順のまとめは、ライブラリの「表示と並び順」と同じ吹き出し（`PopoverContent`、
  `align="end"`、`w-72`）で、中身は `CompactSortControls` と同じ形: 見出し「Sort by」、2 列のラジオ
  （1 行目「Name」「Video count」、2 行目「Date created」）、その下に向きの `SegmentedControl`
  （Name のときは出さない）。並び順が Name 以外のとき、まとめのボタンは `bg-accent-soft text-link`
  にし（ライブラリの絞り込みのボタンが効いているときと同じ形）、閉じていても「既定の並びではない」
  と分かるようにする。
- 件数の行の「· 100 loaded」は `sm` 未満でも件数と同じ行に置く（「500 of 1,000 tags · 100 loaded」で
  約 190px）。収まらないときは merge 済みの規則どおり入口を次の行へ送る（帯は約 140px になる）。
- 帯の高さの見積り（`lg` 以上）: 操作の行 `h-9` ＋ `mt-2` ＋ 件数の行 `h-5` ＋ `pb-2` ＋ 線。1280×800 で
  上部バーと帯を引いた一覧の高さは約 670px で、シノニムの行を持つタグと持たないタグが半々の
  とき（行の高さ 36px と 52px）約 15 行が見える（UI品質「情報密度」の 12 行以上）。1 ページ 100 件は
  この約 6 画面分で、開いた直後に続きを読む必要は無い。
- `sm` 未満の 2 行目は `flex items-center gap-2`、「Tentative only」「Unused only」は `w-auto`、
  まとめのボタンは `ml-auto`。文字だけの 2 つのボタンとまとめのボタンの幅の和は約 264px で、
  360px の本文（312px）に収まる。320px でも収まる。
- 窓（確認・統合・却下した名前）は `ModalFrame` の今の幅の扱い（`sm` 未満で全幅）。選択バーの
  2 段はライブラリと同じ仕組み。

## Review criteria

判定は実機で見て行う（library-ui.md §5）。「ある」だけでは満たさない（Q-4）。幅は 1280×800 を主に、
768px と 360px（タッチの端末またはデベロッパーツールのタッチの模擬）で確かめる。規模は
`tagsbench` の 1,000 個・3,000 個・30,000 個（[quickstart.md](quickstart.md)）で、見た目の判定は
30,000 個でも同じである。

- **視覚的階層**: 1280×800 で管理画面を開いたとき、目が行く順は 行の名前 → 検索と絞り込み →
  本数・シノニム・仮の目印 で、チェック（`opacity-40`）はそのあとに気づく程度である。1 件選ぶと、
  選んだ行の面（`accent-soft`）と全行のチェック、下部のバーが前に出るが、名前の色・大きさは
  変わらない。×で解くと元に戻る。並び順のボタンは検索の入力と同じ高さ・同じ secondary の重さで、
  「Tentative only」「Unused only」は押している間だけ `accent-soft` の面になる。バーの「Confirm」と
  「More」は同じ ghost の重さで、「More」の中の却下・削除は danger の色の文字でメニューを開くまで
  見えない。末尾の続きの状態（`Skeleton`・失敗・一覧が変わった）は行より前に出ず、失敗の箱だけが
  danger の色を持つ（UI品質「視覚的階層」「操作の優先順位」）。
- **情報密度**: 1280×800 で、シノニムの行を持つタグと持たないタグが半々のとき、帯の下に 12 行以上
  （見積りは約 15 行）が見える。行の高さは 031 と同じ（チェックは `size-5` で `py-2` の行に収まり、
  行を伸ばさない）。帯は 1 行の操作と 1 行の件数で、`h1` は留めない。件数の行に「· 100 loaded」が
  足されても行は 1 行のまま。選択バーが出ても一覧の行数は変わらない（下端に `pb-16` が足される
  だけ）。
- **余白のリズム**: `h1` と帯、帯の中の操作の行と件数の行、件数の行と最初の行の間隔が、この feature
  の前の `mt-3`・`mt-2`・`mt-2` と同じ。帯の下の線は `divide-y` の線と同じ色・太さで、一覧の最初の
  線に見える。チェックと名前の列の間は列どうしの `gap-2`（`sm:gap-3`）と同じ。スクロールして帯が
  留まったとき、帯の下を流れる行は帯に透けない。続きの `Skeleton` は行と同じ高さ・同じ `px-2` で、
  読み込んだ行の並びがそのまま続いているように見える。
- **タイポグラフィ**: 新しい文字の大きさ・太さは、統合の窓の「Tags to merge」の見出し（吹き出しの
  `legend` と同じ書式）だけで、ほかは今ある書式（件数の行の `text-xs`、バーの `text-sm`、続きの箱の
  `text-sm`、ボタンの `sm`）を使う。並び順のボタンの文字は種類の名前だけで、向きはアイコンで示す。
  「· 100 loaded」は件数と同じ `text-xs text-fg-muted tabular-nums`。
- **操作の優先順位**: 「Tentative only」を 1 回、件数の行の先頭のチェックを 1 回、バーの「Confirm」を
  1 回の 3 回で、読み込んだ仮のタグ全部が片付く（受け入れ条件 10）。却下は「More」→「Reject…」→
  数が出るのを待って「Reject」の 3 回で、確定より窓の分だけ多い。行の 1 件の操作はマウスの端末で
  今のまま 1 回で届く。タッチの端末では行の操作は「Actions」→ 項目の 2 回だが、各項目は文字で
  読める（UI品質「行の操作」）。
- **読み込んだ分と全部の見分け**: 「Tentative only」で `total` が 500 の一覧（「500 of 1,000 tags ·
  100 loaded」）で先頭のチェックを押すと、バーは「100 tags selected」になり、先頭のチェックは選択の
  状態、読み上げ名は「Select all 100 loaded tags」→「Clear selection」。「Confirm」のあと、読み込んだ
  100 行から仮の目印が消え、件数の行が「400 of 1,000 tags」になり、スクロールして続きを読むと
  残りの仮のタグが仮のまま出る（受け入れ条件 10、要件 10）。この間に画面が固まらない。
- **続きの読み込み**: 30,000 個の一覧を末尾へスクロールすると、読み込んだ最後の行の下に行の
  `Skeleton` が出てすぐ行に置き換わり、その間も帯とバーは動かず、行のチェックと操作が押せる。
  件数の行の「· 100 loaded」が 200、300 と増え、末尾まで読むと消える。同じタグが二重に出ない。
  続きの失敗では読み込んだ行が残り、danger の箱の「Retry」で続きが読める。別のタブでタグを作って
  から続きを読むと、中立の箱「Tags were added or removed elsewhere…」が出て続きが止まり、
  「Reload」で先頭から読み直す（Edge Case）。
- **並びと絞り込みの可視性**: 並び順のボタンの文字で今の種類が、その右の矢印で向きが分かる。
  「Unused only」をオンにすると件数の行が「90 of 1,000 tags」になり、出る行の本数がすべて「0 videos」
  で、この数は読み込んでいないタグも含む（受け入れ条件 8）。「Video count」の多い順で先頭が最多、
  末尾まで読むと 0 本（受け入れ条件 5）。「Date created」の新しい順で「新しいタグ」を作ると、その行が
  一覧の先頭に入る（受け入れ条件 6）。並び順を変えてライブラリへ移って戻る、または再読み込みしても
  同じ並びで開く（受け入れ条件 7）。検索に「ＡＣＴＩＯＮ」と入れると、読み込まれていなかった「action」が
  出る（受け入れ条件 9）。条件を変えた瞬間に一覧が空や `Skeleton` にならず、前の行から新しい行へ
  置き換わる。
- **スクロール中の手の届き方**: 30,000 個の一覧の末尾までスクロールしても、検索・「Tentative only」・
  「Unused only」・並び順・件数の行・「Rejected names」の入口が上部バーの下に見え、1 件でも選んで
  いればバーが下部に見える（要件 13）。1,000 個の先頭でスクロールせずに「Rejected names」を押すと
  窓が開く（受け入れ条件 13）。
- **確認の数**: 仮 8 個と確定 4 個を選んで「Delete…」を開くと、「8 of the 12 selected tags are
  confirmed」ではなく「4 of the 12 selected tags are confirmed」と、確定した 4 個のどれかが付いた
  動画の本数（重複なし）が出る（受け入れ条件 12）。4 個を選んで「Action」へ統合すると、窓に 4 個の
  チップと統合先の入力が窓の幅いっぱいに出て、実行後に 4 個が消え、「Action」の本数が合算
  （重複なし）になる（受け入れ条件 11、要件 14）。統合先の入力に「ａｃｔ」と入れると、読み込んで
  いない「Action」が候補に出て、入力中は右端に小さな回転が見え、候補の一覧が消えて現れたりしない
  （要件 9）。
- **却下した名前の窓**: 1,000 個の却下した名前で入口に「1,000」が出て、窓を開くと先頭の 100 個が
  チップで並び、末尾までスクロールするとチップの `Skeleton` が出て次の 100 個が足される。× で外すと
  チップが消え、入口の数が 1 減る（要件 12）。
- **キーボード**: `lg` 以上で Tab は 検索 →「Tentative only」→「Unused only」→ 並び順のメニュー →
  向き →「新しいタグ」→ 先頭のチェック →「Rejected names」→ 行のチェック → 行の名前 → 行の操作 →
  … と進み、読み込んだ最後の行のあとは末尾の箱のボタン（「Retry」「Reload」があるとき）、バーが
  出ていれば本文のあとにバーの「Confirm」→「More」→ × が続く。`/` で検索へ移る。窓の Esc は
  窓だけを閉じる。1,000 個の一覧で、行の中を Tab で進め続けると描いている範囲の端を越えても
  次の行へ進み（バーや本文の外へ飛ばない）、Shift+Tab でも同じく前の行へ戻る。Tab で読み込んだ
  最後の行に届いたとき、続きがあれば（その行が描かれた時点で）続きの読み込みが始まっている
  （下の「Keyboard across virtualized rows」）。
- **要求を満たしたことにならない例**（UI品質）: 速くなっても、読み込んだタグをまとめて選んで
  確定する手が無い。選んでいないときからチェックや操作のバーが名前より先に目に入る。「Unused
  only」や並び順が吹き出しの中に隠れ、ボタンの文字で今の状態が読めない。却下・削除・統合がバーに
  直接並び、確定と同じ重さで見える。却下した名前が一覧の下にあり、先頭から届かない。タッチの
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
  箱のボタン、バーの「Confirm」、帯の「Rejected names」）へ進む。
- **続きとの関係**: 読み込んだ最後の行へ Tab で届くと、その行が描かれるので続きの読み込みが
  始まる（「Loading more」のきっかけと同じ）。届いた行は読み込んだ最後の行の後ろに足されるので、
  利用者は続けて Tab で進める。読み込みが間に合わずに一覧の外へ出たら、Shift+Tab で戻る。

## Colour

- 新しい token は足さない。選んだ行の面 `accent-soft` の上の文字（名前 `fg`、本数・シノニム
  `fg-muted`）は、ライブラリのリスト表示の選んだ行が既に使っている組で、`tokens.test.ts` の
  `pairs` に足した（merge 済み）。
- 「Unused only」の押している形（`link` on `accent-soft`）、並び順のまとめのボタンが効いている形
  （同じ組）、バーの文字（`fg` on `elevated`）、窓の中の失敗の行（`danger` on `elevated`）、帯の文字
  （`fg-muted` on `bg`）、続きの失敗の箱（`danger` on `danger-soft`。ライブラリの `LoadMoreFailed` と
  同じ）、一覧が変わった箱（`fg` on `elevated`）は `pairs` に既にある。
- 仮の目印の `fg-subtle` はアイコンだけに使う（031 と同じ）。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前と役割だけを決める（014・031 と同じ範囲）。

- 行のチェック: 読み上げ名「Select "〈名〉"」。先頭のチェック: 「Select all 100 loaded tags」／「Clear
  selection」、中間の状態は `aria-checked="mixed"`、押せない理由は `aria-describedby`。
- 「Unused only」: `aria-pressed`。並び順のメニュー: `aria-label`「Sort by: 〈種類〉」、向きのボタンは
  上の「Words」。まとめのボタン: 「Sort」。
- 続きの状態: 読み込み中は一覧の包みの `aria-busy` と `sr-only` の `role="status"`「Loading more
  tags…」、失敗の箱は `role="alert"`、一覧が変わった箱は `role="status"`。
- 選択バー: `role="region"`「Selected tags」、件数は `role="status"`（`polite`）。押せない理由は `title`
  と `aria-describedby`。
- 窓: 見出しは `ModalFrame` の `title`。数えている間の 1 行は `aria-busy`、失敗の行は `role="alert"`。
  統合の窓の候補の読み込み中は `Combobox` の `aria-busy`。却下した名前の窓で続きを読んでいる間は
  並びの `ul` の `aria-busy`。
- 「Actions」のメニュー: 入口は `aria-label`「Actions」、項目は文字を持つ。
