# UI Design: タグ管理画面を数千個のタグでも片付けられる形にする

**Feature**: [parent Issue #651](https://github.com/syudead/vv/issues/651) ・
[plan.md](plan.md) ・ [research.md](research.md)（R-1・R-2・R-4〜R-7）・
[data-model.md §4](data-model.md#4-画面の側で持つ状態) ・
[contracts/screen-api.md](contracts/screen-api.md)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・一覧の構成・選択バーの形:
  [ライブラリ UI](../../docs/design-docs/library-ui.md)（§1・§4・§6）
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
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足す・変えるのは、所有者だけのタグ管理画面（`/tags`）の次の 7 つである。

1. 操作の行と件数の行を、一覧をスクロールしても上部バーの下に**留める**（要件 12）
2. 操作の行に**並び順**（名前・本数・作った日）と **0 本のタグだけ**の絞り込みを足す（要件 3〜5）
3. 行に**チェック**を足し、**選んでいる間だけ**出る選択バーから、まとめて確定・却下・削除・統合する
   （要件 7〜10）
4. まとめての却下・削除・統合の**確認の窓**（要件 10）と、複数の統合元を持つ**統合の窓**（要件 8・13）
5. **却下した名前**の入口を件数の行に移し、中身を窓で開く（要件 11）
6. 行の操作を、タッチの端末と狭い幅では**文字を持つメニュー**にまとめる（UI品質「行の操作」）
7. 一覧の行を見えている分だけ描く（要件 1・2）。**見た目は変えない**

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
- **選択はチェックで、まとめての操作はライブラリと同じ画面下部の選択バーに置く。** 選択バーは
  この製品で「選んでいる間だけ前に出る」形として既に決まっている（library-ui.md §6）。留めた
  操作の行の中に操作を差し込む案は、選んでいる間だけ操作の行の高さが変わり、一覧が上下に
  動いてチェックの位置がずれる。バーは `fixed` なので、要件 12 の「スクロールしても手が届く」も
  自然に満たす。チェックを常に薄く見せるのは、ライブラリのリスト表示と同じ規則で、hover できない
  端末でも選択の入口が見えるためである。
- **まとめての操作の中で前に出すのは「確定」だけ。却下・削除・統合は「More」のメニューに入れる。**
  親 Issue の UI品質は「片付けの主の操作は、選ぶことと、まとめての確定」「削除・統合・却下は今と
  同じく作成・確定より目立たせない」と求める。行では 014・031 がその 3 つを「その他の操作」の
  メニューに置いたので、バーでも同じ関係にする。確定は確認をとらず（要件 10）、却下・削除・統合は
  確認の窓を挟む。
- **「見えているものをすべて選ぶ」は件数の行の先頭のチェック。** 件数の行は「90 of 1,000 tags」と
  いう「今見えている集合」を言う行で、その先頭のチェックは「この集合を選ぶ」と読める（メールの
  一覧の見出しのチェックと同じ形）。選択バーの「すべて選択」（ライブラリ）をここにも置く案は、
  バーは 1 件選んだあとにしか出ないので、最初の 1 件を選ぶ手が別に要る。先頭のチェックなら、
  絞って 1 回押すだけで全部が選べる。
- **却下した名前は、件数の行の右端の入口から窓で開く。** 031 が一覧の下に置いた折りたたみは、
  数千行の下では届かない（要件 11）。入口を件数の行（留まる行）の右端に置くのは、片付けの途中で
  毎回見るものではない従の情報を、主の操作（検索・絞り込み・並び順）と同じ行に並べないためである。
  中身は 031 の規則のまま窓（`ModalFrame`）で開く。吹き出しにしないのは、却下した名前が数百に
  なりうるため、縦に伸びる中身を窓の中でスクロールさせたいからである。
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
| 件数の行（絞り込み・検索のどちらかが効いている） | 90 of 1,000 tags（今の形） |
| 件数の行の先頭のチェックの読み上げ名 | Select all shown tags ／ Clear selection（全部選んでいるとき） |
| 行のチェックの読み上げ名 | Select "〈名〉" |
| 一致が無いときの見出し | No unused tags ／ No unused tentative tags ／ No unused tags match "〈入力〉" ／ No unused tentative tags match "〈入力〉" |
| 一致が無いときの説明（「Unused only」だけ、検索なし） | Every tag is on at least one video. |
| 一致が無いときのボタン | Show all tags（今の形） |
| 選択バーの読み上げ名（`region`） | Selected tags |
| 選択バーの件数 | 1 tag selected ／ 12 tags selected |
| 選択バーの操作 | Confirm ／ More ／ Clear selection（×） |
| 「More」のメニュー | Merge into one tag… ／ Reject… ／ Delete… |
| 押せない理由（`title`・読み上げ） | No tentative tags are selected ／ No confirmed tags are selected |
| 見えている数が上限を超えたとき | Too many tags are shown to act on them together (limit {limit}). Narrow the list with search or a filter. |
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
| 統合の窓の確認（複数） | The 120 videos tagged with these 4 tags get the tag "Action". Their names and synonyms become synonyms of "Action", and the 4 tags leave the tag list. This can't be undone. |
| 統合先を選んだ中から選んだとき | "Action" is kept and the other 3 tags merge into it. |
| 統合元が無くなったとき | Choose another tag to merge into: "Action" is the only tag selected. |
| 統合のトースト（複数） | Merged 4 tags into "Action" |
| 対象の一部がもう無かったとき | Some of the tags no longer existed, so the list was reloaded |
| 却下した名前の入口 | Rejected names 〈件数〉（今の見出しの文字） |
| 却下した名前の窓の見出し | Rejected names |
| 行の操作のまとめの入口（タッチ・`sm` 未満） | Actions |

- 「Unused」は 0 本のタグの呼び名で、文言の中で「tags that aren't on any videos」と説明する。
  「empty」「orphan」は使わない。
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

- **「Unused only」**: 「Tentative only」と同じ部品・同じ押している形（`Button` の secondary、
  `aria-pressed`、押している間は `border-accent-active`・`bg-accent-soft`・`text-link`）。アイコンは
  lucide `VideoOff`（行の本数の列が「videos」なので、「動画が無い」を同じ語で描く。`CircleDashed` は
  仮の目印なので使わない）。文言「Unused only」、ツールチップ「Show only tags that aren't on any
  videos」。押すと一覧を `videoCount === 0` の行だけにし、「Tentative only」・検索・並び順と重ねられる
  （両方の押しボタンがオンなら両方を満たす行。要件 5）。この画面の状態で、URL・`localStorage` には
  載せない（R-7）。`disabled` の規則は「Tentative only」と同じ（読み込み中・読み込み失敗・押していない
  間にタグが 0 のとき。**押している間はタグが 0 になっても `disabled` にしない**）。
- **並び順**: ライブラリの `SortMenu` と同じ形。`Button` の secondary にいまの種類の名前
  （「Name」「Video count」「Date created」）と `ChevronDown`、読み上げ名「Sort by: 〈種類〉」。メニューは
  見出し「Sort by」と 3 つのラジオ項目。アイコンは Name が `ArrowDownAZ`（ライブラリの「Title」と
  同じ、名前の順の意味）、Video count が `Hash`（数）、Date created が `CalendarPlus`（タグが
  できた日。ライブラリの「Date created」の `FileClock` はファイルの作成日で別のものなので、同じ
  絵にしない）。
  - 種類を選ぶと、その種類の既定の向きになる: Video count は多い順（`countDesc`）、Date created は
    新しい順（`createdDesc`）。Name に向きは無い（要件 3）。
  - **向きの切り替え**は、ライブラリと同じくメニューのボタンの右に接する `Button`
    （`rounded-l-none px-2.5`）で、`ArrowDownWideNarrow`（降順）／`ArrowUpNarrowWide`（昇順）、
    読み上げ名とツールチップは上の「Words」。**Name のときは向きのボタンを出さない**（メニューの
    ボタンは全周の角丸に戻る）。`disabled` の向きのボタンを残す案は、押せないものを置く理由が
    「幅を揺らさない」だけで、Q-5 の価値にならない。
  - 同じ値のタグどうしは名前の順（R-7・R-8）。選んだ並び順は `localStorage` に残り（R-7）、画面を
    開くとその並びで始まる。壊れていれば Name。
  - `disabled` の規則は「Tentative only」と同じ（読み込み中・読み込み失敗・タグが 0）。
- **「Tentative only」「新しいタグ」「検索」**は今のまま（031・014）。

### Count line

件数の行は帯の中の最後の行で、左から、**先頭のチェック** → 件数 → 右端に**却下した名前の入口**。
高さは `h-5`（チェックの大きさ）に合わせ、文字は今の `text-xs text-fg-muted tabular-nums`。

- **先頭のチェック**: `Checkbox`（`size-5`）。行のチェックと同じ列の位置に置き（行の `px-2` と
  同じ左の余白）、縦に並ぶ。見え方は行のチェックと同じ規則（下の「Selection」の「Row checkbox」。
  選んでいない間は `opacity-40`、帯に hover するか選んでいる間は `opacity-100`）。状態は 3 つ:
  見えている行を 1 つも選んでいなければ空、一部なら中間（lucide `Minus`）、全部なら選択。空と中間で
  押すと見えている行をすべて選び（要件 9）、全部のときに押すと選択を解く。読み上げ名は空・中間で
  「Select all shown tags」、全部で「Clear selection」。見えている行が無いとき（空の状態）・読み込み中・
  読み込み失敗・見えている数が上限（`maxTagBatch`）を超えるときは `disabled`。
- **件数**: 今の形（「1,000 tags」、絞り込み・検索のどちらかが効いていれば「90 of 1,000 tags」。
  受け入れ条件 7）。改名中の残した行は数えない（今と同じ）。選んだ数はここに出さない
  （選択バーが出す）。
- **却下した名前の入口**: `Button` の ghost・`sm`（`h-8` だが帯の高さは伸ばさず、`-my-1.5` で
  件数の行に収める）、文字「Rejected names」（`text-xs`）と、後ろに `tabular-nums` の件数
  （031 の見出しの件数と同じ規則。取れるまでは出さない）。アイコンは付けない（文字が意味を持つ）。
  押すと下の「Rejected names」の窓を開く。読み込み中・読み込み失敗（タグの一覧をまだ一度も
  取れていない）のときは出さない（031 と同じ）。「タグはまだありません」のときは出す。

## Rows

行の列・書式・高さ（`py-2`）・名前のリンク・シノニムの行・本数・仮の目印・改名の入力は 014・031 の
まま。足すのは先頭のチェックと、タッチ・狭い幅での操作のまとめだけで、見えている行だけを描く
仕組み（R-2）は行の見た目を変えない。

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
  動きうるため、改名の間は選ばない）。
- 選択は見えている行の部分集合で、検索・絞り込み・並び順を変えて見えなくなった行の選択は外れる
  （Edge Case、[data-model.md §4](data-model.md#4-画面の側で持つ状態)）。並び順だけを変えたときは
  行は見えたままなので選択は残る。
- Shift を押しながらの範囲選択は入れない（親 Issue に無く、先頭のチェックで全部を選べる）。

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

## Selection bar

1 件でも選ぶと画面下部に出る。箱・位置・現れ方・段の折り返しの仕組みはライブラリの `SelectionBar`
と同じ（`fixed inset-x-0 bottom-4`、`bg-elevated`・`border-border-strong`・`rounded-md`・
`shadow-elevated`、`sm` 以上で `h-11` の 1 段、`animate-slide-up`）。`role="region"`、読み上げ名
「Selected tags」。選択が空になると消える。バーが出ている間は、本文の下端に `pb-16` を足し、最後の
行がバーの下に隠れたままにならないようにする。

### Layout

`sm` 以上は 1 段、左から:

1. 件数「12 tags selected」（`text-sm text-fg tabular-nums`、`role="status"`・`aria-live="polite"`）
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
- 見えている数が `maxTagBatch` を超えるときは「Confirm」「More」を `disabled` にし、ライブラリの
  上限と同じ形で理由（「Too many tags are shown…」）を `title` と `sr-only` で添える。先頭のチェックも
  `disabled` なので、この状態に入るのは行のチェックで選び始めたときだけである。
- まとめての操作の送信中は、バーのすべてのボタンを `disabled` にし、「Confirm」の送信中は
  そのアイコンを `LoaderCircle`（`animate-spin motion-reduce:animate-none`）にする。

### Bulk confirm

- 「Confirm」を押すとすぐ `POST /api/tags/batch`（`confirm`、選んだ id 全部）を 1 回送る。確認の窓は
  無い（要件 10）。
- 応答で、`appliedIds` の行を `tentative: false` に差し替え（目印と「確定する」が消える）、選択から
  外す。`notApplicableIds`（既に確定していたもの）は選んだまま残す。トースト「Confirmed 8 tags」、
  外した数があれば「Confirmed 8 tags. 4 were already confirmed.」。`notFoundIds` が空でなければ一覧を
  取り直し、トースト「Some of the tags no longer existed, so the list was reloaded」。
- 「Tentative only」を押している間は確定した行が一覧から外れる。全部外れて空になったら 031 の
  「No tentative tags」の空の状態になり、フォーカスは「Tentative only」へ（031「絞り込みから外れた
  行のフォーカス」）。そうでなければフォーカスは、バーが残っていれば「Confirm」に、バーが消えたら
  件数の行の先頭のチェックへ移す。
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
- `200` で窓を閉じ、`appliedIds` の行を一覧から消し、選択から外す。`notApplicableIds` は選んだまま
  残す。トースト「Rejected 8 tags」「Deleted 8 tags」、外した数があれば「… 4 confirmed tags were
  skipped.」「… 4 tentative tags were skipped.」。却下のあとは却下した名前を取り直す（入口の件数が
  増える）。`notFoundIds` が空でなければ一覧を取り直し、上と同じトースト。フォーカスは、消えた行の
  位置の次の行の「改名」（まとめている間は入口の `IconButton`）、無ければ前の行、1 つも無ければ
  「Tentative only」を押していればそのボタン、押していなければ件数の行の先頭のチェックへ移す
  （014・031 の削除・却下の規則を、まとめての操作に当てたもの）。
- 失敗は窓の中に `text-sm text-danger`（`role="alert"`）の 1 行で `errorText` を出し、窓は開いたまま、
  選択も残る。「Cancel」・Esc は何も変えずに閉じ、フォーカスを「More」へ戻す。

## Merge dialog

`MergeTagDialog` を、統合元を 1 件以上持つ 1 つの窓にする。行の「別のタグへ統合…」（統合元 1 件）と
バーの「Merge into one tag…」（統合元が選んだタグ）が同じ窓を開く。

### Width

- 統合先の `Combobox` の枠は窓の内側の幅いっぱい（`frameClassName="w-full"`。今の既定 `w-40` は
  再生画面の「タグを追加」の幅で、窓の中では極端に小さい。要件 13）。候補の一覧も同じ幅で開く。
  1 件の統合でも同じ。窓の幅は今の `ModalFrame` の既定（`sm:max-w-2xl`、`sm` 未満は全幅）。

### Sources

- 統合元が 1 件（行から）のときは今の形: 見出し「Merge "X"」、統合元の並びは出さず、統合先の候補は
  統合元を除く全タグ、確認の文言と本数（`videoCount`）は 014 のまま。
- 統合元が複数（バーから）のときは、見出し「Merge 4 tags」、入力の上に「Tags to merge」の
  見出し（`text-xs font-semibold text-fg-muted uppercase`。吹き出しの `legend` と同じ書式）と、
  統合元の名前の並び（`ul`、`flex flex-wrap gap-1.5`、各名前は 014 のシノニムの窓と同じ `bg-bg` の
  `Chip`、`h-6`・`text-xs`、× なし）。並びは `max-h-32 overflow-y-auto` で、数十個を超えても窓を
  押し広げない。仮のタグには行と同じ目印（`size-3`）を名前の後ろに付ける。
- 統合先の候補は**全タグ**（選んだ中からも、選んでいないタグからも。要件 8）。**選んだ中のタグを
  統合先に選ぶと、そのタグは統合元から外れ**（Edge Case）、並びのそのチップに `text-fg-muted` の
  「kept」を添えて残し（消すと「選んだのに無い」と見える）、確認の文言の上に `text-sm text-fg-muted`
  の 1 行「"Action" is kept and the other 3 tags merge into it.」を出す。
- 統合元が統合先だけになったとき（選んだのが 1 件で、それを統合先に選んだとき）は、確認の文言の
  代わりに `text-sm text-fg-muted` の「Choose another tag to merge into: "Action" is the only tag
  selected.」を出し、「Merge」は `disabled`。

### Confirmation

- 統合先を選ぶと、統合先を外した統合元について `POST /api/tags/impact`（`merge`）を送り、届くまで
  「Counting the affected videos…」と `LoaderCircle`、「Merge」は `disabled`。届いたら 014 と同じ
  `border-l-2 border-danger-strong` の段落に「The 120 videos tagged with these 4 tags get the tag
  "Action". …」を出す。数えられなかったときはまとめての却下と同じ「Couldn't count…」と「Retry」。
  統合先を選び直すたびに数え直す。
- 実行は `POST /api/tags/{id}/merge`（`sourceIds` = 統合先を外した統合元）。`200` で窓を閉じ、統合元の
  行を消し、統合先の行を応答の `tag` に差し替え（本数が合算に、仮なら確定に）、選択を空にし、
  トースト「Merged 4 tags into "Action"」。フォーカスは統合先の行の名前へ（014 と同じ。「Tentative
  only」中で統合先が一覧に無ければ 031 の規則）。`notFoundIds` が空でなければ一覧を取り直す。
- 失敗・「Cancel」・Esc は 014 のまま（窓は開いたまま失敗を出す。閉じたらフォーカスは開いた元の
  「その他の操作」またはバーの「More」へ）。

## Rejected names

入口は上の「Count line」。押すと `ModalFrame`（`sm:max-w-lg`）の窓「Rejected names」を開く。

- 中身は 031「Rejected names」の規則のまま: 説明の 1 行、名前の並び（`Chip` に ×。窓の面は
  `bg-elevated` なので、014 のシノニムの窓と同じく `bg-bg` の面にする）、× で即時に取り外し
  （確認なし、送信中は `disabled`、トーストなし、フォーカスは次の × → 前の × → 無ければ説明の
  上の見出し）、失敗の 1 行、読み込み中の `Skeleton`、取れなかったときの「Couldn't load the rejected
  names」と「Retry」、無いときの「No rejected names」。読み込みのきっかけ（画面を開いたとき）と
  取り直しのきっかけ（却下・作成・改名・シノニムの追加のあと。まとめての却下も）は 031 のまま。
- 窓の下端に secondary「Close」。最初のフォーカスは最初の名前の ×、無ければ「Close」。Esc と × で
  閉じ、フォーカスは入口へ戻る。開閉はこの画面の状態で、URL には載せない。
- 一覧の下の折りたたみ（031）は置かない。入口が帯にあるので、1,000 個の先頭からスクロールせずに
  開ける（受け入れ条件 12）。
- 検索・絞り込みは窓の中身に効かない（タグではない。031 と同じ）。

## States

014・031 の「States」の表に足す・変える。

| 状態 | 見え方 |
| --- | --- |
| 「Unused only」で一致が無い（検索は空、「Tentative only」はオフ） | `EmptyState`（`VideoOff`）「No unused tags」、説明「Every tag is on at least one video.」、`Button`「Show all tags」。押すと絞り込みを外し、フォーカスを「Unused only」へ移す |
| 「Unused only」と「Tentative only」の両方で一致が無い | `EmptyState`（`VideoOff`）「No unused tentative tags」、説明なし、`Button`「Show all tags」。押すと両方を外し、フォーカスを「Tentative only」へ |
| 絞り込みと検索で一致が無い | `EmptyState`（`SearchX`）「No unused tags match "〈入力〉"」「No unused tentative tags match "〈入力〉"」、`Button`「Show all tags」。押すと絞り込みと検索の両方を外し、フォーカスを検索の入力へ |
| まとめての確定の送信中 | バーのボタンがすべて `disabled`、「Confirm」のアイコンが `LoaderCircle` |
| まとめての却下・削除・統合の確認で数を待つ | 本文「Counting the affected videos…」と `LoaderCircle`、danger のボタンは `disabled` |
| 数えられなかった | 本文に `role="alert"` の「Couldn't count the affected videos: {理由}」と「Retry」、danger のボタンは `disabled` |
| 働かない種類を含めて実行した | 窓の本文で「8 of the 12 selected tags are …」と先に言い、実行後のトーストで「4 … were skipped.」。外した分は選んだまま残る |
| 対象の一部がもう無かった（`notFoundIds`） | 残りは処理し、トースト「Some of the tags no longer existed, so the list was reloaded」、一覧を取り直す。取り直しで消えた行の選択は外れる |
| まとめての操作が失敗した | バーからの確定はトースト、窓からの操作は窓の中の 1 行。一覧と選択は変えない |
| 見えている数が上限を超える | 先頭のチェック・「Confirm」・「More」が `disabled`、理由を `title` と `sr-only` で |
| 選んでいる間に検索・絞り込みを変えた | 見えなくなった行の選択が外れ、バーの件数が減る。空になればバーが消える |
| 読み込み失敗（一覧を持っている） | 今の一覧を残し、トーストで `errorText`（Edge Case）。一覧をまだ持っていなければ今の `EmptyState`（danger）と「Retry」 |

読み込み中の `Skeleton`、「タグはまだありません」、「Tentative only」の空の状態、改名・作成・
1 件の操作の状態は 014・031 のまま。並び順・「Unused only」・先頭のチェック・却下した名前の入口は、
読み込み中・読み込み失敗で `disabled`（入口は出さない）。

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md §4）。判定する幅は
360px・768px・1280px。

| 幅 | 帯（操作の行・件数の行） | 行 | 選択バー |
| --- | --- | --- | --- |
| 1280px（`lg` 以上） | 1 行: 検索（`max-w-sm`）→「Tentative only」→「Unused only」→ 並び順（メニュー＋向き）→ 右端「新しいタグ」。その下に件数の行。帯は約 75px | チェック → 名前の列 → 本数 → `IconButton` 3〜4 つ（マウス）または「Actions」1 つ（タッチ） | 1 段 |
| 768px（`sm` 以上 `lg` 未満） | 2 行: 1 行目は検索（`flex-1`、`max-w-sm`）→ 右端「新しいタグ」、2 行目は「Tentative only」→「Unused only」→ 並び順（メニュー＋向き）。その下に件数の行。帯は約 120px | 同上 | 1 段 |
| 360px（`sm` 未満） | 2 行: 1 行目は検索（`flex-1`）→「新しいタグ」、2 行目は「Tentative only」→「Unused only」（アイコンを出さず文字だけ。`max-sm:hidden`）→ 右端に並び順のまとめ（`IconButton` 相当の `Button` secondary `px-2.5`、`SlidersHorizontal`、読み上げ名「Sort」）。その下に件数の行。帯は約 120px | チェック → 名前の列（約 160px）→ 本数（`w-16`）→「Actions」1 つ。横スクロールは出ない | 2 段（上段: 件数と ×、下段: 「Confirm」「More」を半分ずつ） |

- `sm` 未満の並び順のまとめは、ライブラリの「表示と並び順」と同じ吹き出し（`PopoverContent`、
  `align="end"`、`w-72`）で、中身は `CompactSortControls` と同じ形: 見出し「Sort by」、2 列のラジオ
  （1 行目「Name」「Video count」、2 行目「Date created」）、その下に向きの `SegmentedControl`
  （Name のときは出さない）。並び順が Name 以外のとき、まとめのボタンは `bg-accent-soft text-link`
  にし（ライブラリの絞り込みのボタンが効いているときと同じ形）、閉じていても「既定の並びではない」
  と分かるようにする。
- 帯の高さの見積り（`lg` 以上）: 操作の行 `h-9` ＋ `mt-2` ＋ 件数の行 `h-5` ＋ `pb-2` ＋ 線。1280×800 で
  上部バーと帯を引いた一覧の高さは約 670px で、シノニムの行を持つタグと持たないタグが半々の
  とき（行の高さ 36px と 52px）約 15 行が見える（UI品質「情報密度」の 12 行以上）。
- `sm` 未満の 2 行目は `flex items-center gap-2`、「Tentative only」「Unused only」は `w-auto`、
  まとめのボタンは `ml-auto`。文字だけの 2 つのボタンとまとめのボタンの幅の和は約 264px で、
  360px の本文（312px）に収まる。320px でも収まる。
- 窓（確認・統合・却下した名前）は `ModalFrame` の今の幅の扱い（`sm` 未満で全幅）。選択バーの
  2 段はライブラリと同じ仕組み。

## Review criteria

判定は実機で見て行う（library-ui.md §5）。「ある」だけでは満たさない（Q-4）。幅は 1280×800 を主に、
768px と 360px（タッチの端末またはデベロッパーツールのタッチの模擬）で確かめる。

- **視覚的階層**: 1280×800 で管理画面を開いたとき、目が行く順は 行の名前 → 検索と絞り込み →
  本数・シノニム・仮の目印 で、チェック（`opacity-40`）はそのあとに気づく程度である。1 件選ぶと、
  選んだ行の面（`accent-soft`）と全行のチェック、下部のバーが前に出るが、名前の色・大きさは
  変わらない。×で解くと元に戻る。並び順のボタンは検索の入力と同じ高さ・同じ secondary の重さで、
  「Tentative only」「Unused only」は押している間だけ `accent-soft` の面になる。バーの「Confirm」と
  「More」は同じ ghost の重さで、「More」の中の却下・削除は danger の色の文字でメニューを開くまで
  見えない（UI品質「視覚的階層」「操作の優先順位」）。
- **情報密度**: 1280×800 で、シノニムの行を持つタグと持たないタグが半々のとき、帯の下に 12 行以上
  （見積りは約 15 行）が見える。行の高さは 031 と同じ（チェックは `size-5` で `py-2` の行に収まり、
  行を伸ばさない）。帯は 1 行の操作と 1 行の件数で、`h1` は留めない。選択バーが出ても一覧の行数は
  変わらない（下端に `pb-16` が足されるだけ）。
- **余白のリズム**: `h1` と帯、帯の中の操作の行と件数の行、件数の行と最初の行の間隔が、この feature
  の前の `mt-3`・`mt-2`・`mt-2` と同じ。帯の下の線は `divide-y` の線と同じ色・太さで、一覧の最初の
  線に見える。チェックと名前の列の間は列どうしの `gap-2`（`sm:gap-3`）と同じ。スクロールして帯が
  留まったとき、帯の下を流れる行は帯に透けない。
- **タイポグラフィ**: 新しい文字の大きさ・太さは、統合の窓の「Tags to merge」の見出し（吹き出しの
  `legend` と同じ書式）だけで、ほかは今ある書式（件数の行の `text-xs`、バーの `text-sm`、ボタンの
  `sm`）を使う。並び順のボタンの文字は種類の名前だけで、向きはアイコンで示す。
- **操作の優先順位**: 「Tentative only」を 1 回、件数の行の先頭のチェックを 1 回、バーの「Confirm」を
  1 回の 3 回で、見えている仮のタグ全部が片付く（受け入れ条件 9）。却下は「More」→「Reject…」→
  数が出るのを待って「Reject」の 3 回で、確定より窓の分だけ多い。行の 1 件の操作はマウスの端末で
  今のまま 1 回で届く。タッチの端末では行の操作は「Actions」→ 項目の 2 回だが、各項目は文字で
  読める（UI品質「行の操作」）。
- **並びと絞り込みの可視性**: 並び順のボタンの文字で今の種類が、その右の矢印で向きが分かる。
  「Unused only」をオンにすると件数の行が「90 of 1,000 tags」になり、出る行の本数がすべて「0 videos」
  （受け入れ条件 7）。「Video count」の多い順で先頭が最多、末尾が 0 本（受け入れ条件 4）。「Date
  created」の新しい順で「新しいタグ」を作ると、その行が一覧の先頭に入る（受け入れ条件 5）。
  並び順を変えてライブラリへ移って戻る、または再読み込みしても同じ並びで開く（受け入れ条件 6）。
- **スクロール中の手の届き方**: 1,000 個の一覧の末尾までスクロールしても、検索・「Tentative only」・
  「Unused only」・並び順・件数の行・「Rejected names」の入口が上部バーの下に見え、1 件でも選んで
  いればバーが下部に見える（要件 12）。先頭でスクロールせずに「Rejected names」を押すと窓が開く
  （受け入れ条件 12）。
- **確認の数**: 仮 8 個と確定 4 個を選んで「Delete…」を開くと、「8 of the 12 selected tags are
  confirmed」ではなく「4 of the 12 selected tags are confirmed」と、確定した 4 個のどれかが付いた
  動画の本数（重複なし）が出る（受け入れ条件 11）。4 個を選んで「Action」へ統合すると、窓に 4 個の
  チップと統合先の入力が窓の幅いっぱいに出て、実行後に 4 個が消え、「Action」の本数が合算
  （重複なし）になる（受け入れ条件 10、要件 13）。
- **キーボード**: `lg` 以上で Tab は 検索 →「Tentative only」→「Unused only」→ 並び順のメニュー →
  向き →「新しいタグ」→ 先頭のチェック →「Rejected names」→ 行のチェック → 行の名前 → 行の操作 →
  … と進み、バーが出ていれば本文のあとにバーの「Confirm」→「More」→ × が続く。`/` で検索へ移る。
  窓の Esc は窓だけを閉じる。
- **要求を満たしたことにならない例**（UI品質）: 速くなっても、見えているタグをまとめて選んで確定する
  手が無い。選んでいないときからチェックや操作のバーが名前より先に目に入る。「Unused only」や
  並び順が吹き出しの中に隠れ、ボタンの文字で今の状態が読めない。却下・削除・統合がバーに直接
  並び、確定と同じ重さで見える。却下した名前が一覧の下にあり、先頭から届かない。タッチの端末で
  行の操作がアイコンだけで、押すまで意味が分からない。1280×800 で 12 行に届かない。統合の窓の
  入力が窓の幅より明らかに狭い。

## Colour

- 新しい token は足さない。選んだ行の面 `accent-soft` の上の文字（名前 `fg`、本数・シノニム
  `fg-muted`）は、ライブラリのリスト表示の選んだ行が既に使っている組だが、`tokens.test.ts` の
  `pairs` に無い。この feature が行の文字をその面に載せるので、`fg` on `accent-soft` と `fg-muted` on
  `accent-soft` を `pairs` に足す（選択の実装の単位で）。
- 「Unused only」の押している形（`link` on `accent-soft`）、並び順のまとめのボタンが効いている形
  （同じ組）、バーの文字（`fg` on `elevated`）、窓の中の失敗の行（`danger` on `elevated`）、帯の文字
  （`fg-muted` on `bg`）は `pairs` に既にある。
- 仮の目印の `fg-subtle` はアイコンだけに使う（031 と同じ）。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前と役割だけを決める（014・031 と同じ範囲）。

- 行のチェック: 読み上げ名「Select "〈名〉"」。先頭のチェック: 「Select all shown tags」／「Clear
  selection」、中間の状態は `aria-checked="mixed"`。
- 「Unused only」: `aria-pressed`。並び順のメニュー: `aria-label`「Sort by: 〈種類〉」、向きのボタンは
  上の「Words」。まとめのボタン: 「Sort」。
- 選択バー: `role="region"`「Selected tags」、件数は `role="status"`（`polite`）。押せない理由は `title`
  と `aria-describedby`。
- 窓: 見出しは `ModalFrame` の `title`。数えている間の 1 行は `aria-busy`、失敗の行は `role="alert"`。
- 「Actions」のメニュー: 入口は `aria-label`「Actions」、項目は文字を持つ。
