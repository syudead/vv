# UI Design: 仮のタグを見分け、確定・却下・統合で片付ける

**Feature**: [parent Issue #589](https://github.com/syudead/vv/issues/589) ・
[plan.md](plan.md) ・ [contracts/screen-api.md](contracts/screen-api.md) ・
[research.md R-5](research.md#r-5-却下するは仮のタグにだけ効き確定したタグには-409-tag_not_tentative-で何もしない)・
[R-8](research.md#r-8-仮のタグだけの絞り込みと却下した名前の一覧は画面の側で持つ)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・一覧と再生画面の構成:
  [ライブラリ UI](../../docs/design-docs/library-ui.md)（「6. 一覧の構成」のカード、「8. 再生画面の構成」）
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`。値は写さず、名前で呼ぶ
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- タグのチップの 3 つの形・カードのタグの行・再生画面のタグ・タグ管理画面の骨格（行・作成と改名・
  シノニムの窓・統合と削除の窓・状態）: [specs/014-video-tags/ui-design.md](../014-video-tags/ui-design.md)
  と今の [`web/src/tags/`](../../web/src/tags/)・[`web/src/library/CardTagRow.tsx`](../../web/src/library/CardTagRow.tsx)・
  [`web/src/player/VideoTags.tsx`](../../web/src/player/VideoTags.tsx)
- フォルダ由来だけのタグのチップ（破線の枠と Folder の目印）:
  [specs/017-folder-groups/ui-design.md「Folder-derived tag chip」](../017-folder-groups/ui-design.md#folder-derived-tag-chip)
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足すのは、所有者だけの次の 4 つである。

1. 動画のタグのチップ（ライブラリのカード・グループのカード・再生画面）の**仮の目印**（要件 6、
   受け入れ条件 5）
2. タグ管理画面の行の**仮の目印**と、**仮のタグだけ**の絞り込み（要件 6・7、受け入れ条件 5・6）
3. 仮のタグの行の**確定する・却下する**（統合は今の操作のまま）と、「削除…」の置き換え（要件 8・10、
   受け入れ条件 7・8・12）
4. タグ管理画面の**却下した名前**の一覧と取り外し（要件 14、受け入れ条件 13）

それ以外は変えない。仮のタグは付け外し・絞り込み・検索で確定したタグと同じに働く（要件 4）ので、
絞り込み中のタグの行、「タグを追加」「タグを付ける」「タグを外す」の候補、統合先の候補、選択バー、
リスト表示、フォルダ画面の検索結果の置き場所の行には目印を足さない。候補の行に目印を足さないのは、
候補で選ぶのは「どのタグを付けるか」であり、仮かどうかはその判断を変えないからである（付けた
結果は仮のまま。[data-model.md §3](data-model.md#3-書き換えの規則)）。ゲストにはタグが出ないので
（要件 4）、この feature の 4 つのどれも出ない。新しい色・半径・影の token は足さず、
`tokens.test.ts` の `pairs` にも足さない（下の「Colour」）。

## Why this shape

- **仮の目印は 1 つの形（lucide `CircleDashed`）で全画面に通す**。破線の丸は、現行の開発・
  課題管理の製品が「下書き・未確定」に収束させている形で、017 の Folder（出所）と別の意味
  （状態）を別の形で持たせられる。目印は名前の**前**ではなく**後ろ**に置く。前に置くと Folder の
  目印と並んで「出所が 2 つ」に読め、名前の読み始めも遅れるからである。色は `text-fg-subtle` で、
  名前（`text-fg-muted` または `text-fg`）より薄い。仮のタグも普通に使えるタグなので、警告色・
  面の色の変更・バッジ・件数は使わない（UI品質「視覚的階層」「要求を満たしたことにならないもの」）。
- **「仮のタグだけ」は検索と同じ行の押しボタン（`aria-pressed`）で、URL には載せない**。014 の
  管理画面の検索がその場の絞り込みで URL を持たないのと同じ扱いにし、片付けの作業（絞り込む →
  行を 1 つずつ片付ける → 空になる）が 1 画面で閉じるようにする。押している間は行が減っていく
  ことが結果で、件数の行が「3 of 12 tags」で残りを伝える（UI品質「仮のタグを一覧で探すのに、
  全タグを読む必要が残る形」を避ける）。
- **「確定する」は行に直接、「却下する」はメニューの中**。確定は状態を進めるだけで取り消しの
  費用が無く、片付けの主の操作なので、行の操作の一群の先頭に置いて 1 回で届くようにする。却下は
  タグの削除を伴うので、014 が「削除…」に与えた場所（「その他の操作」の区切り線の下、danger）を
  そのまま引き継ぎ、確認の窓を挟む（UI品質「操作の優先順位」）。統合は今の「別のタグへ統合…」の
  まま動かさない。
- **却下した名前は一覧の下の折りたたみ**。タグではなく「次から作らない」だけの事実で、片付けの
  途中で毎回見るものではない（UI品質「情報密度」）。設定画面や別の画面に置かないのは、却下の直後に
  「どこに入ったか」を同じ画面で確かめられ、取り消し（取り外し）もその場で済むためである。

## Words

| 場所 | 英語（案） |
| --- | --- |
| 仮の目印の視覚的に隠した文言・`title`・ツールチップ | Tentative |
| カードのチップの読み上げ名（押せる形） | Filter by 〈名〉 (tentative) |
| カードのチップの読み上げ名（フォルダ由来だけで仮） | Filter by 〈名〉 (from the folder name, tentative) |
| 再生画面の名前の部分の読み上げ名 | Filter by 〈名〉 (tentative) |
| 絞り込みのボタン | Tentative only |
| 絞り込みのボタンの補足（ツールチップ） | Show only tags created by automatic tagging |
| 件数の行（絞り込み中・検索と同じ形） | 3 of 12 tags |
| 仮のタグが無いときの空の状態の見出し | No tentative tags |
| 仮のタグが無いときの説明 | Tags created by automatic tagging appear here until you confirm or reject them. |
| 絞り込み中に検索で一致が無いとき | No tentative tags match "〈入力〉" |
| 空の状態・一致なしのボタン | Show all tags |
| 行の「確定する」（読み上げ名・ツールチップ） | Confirm |
| メニューの「却下する」 | Reject… |
| 却下の窓の見出し | Reject "〈名〉" |
| 却下の窓の本文（N 本） | This tag will be removed from N videos, and automatic tagging won't create "〈名〉" again. You can allow the name again from the rejected names below. |
| 却下の窓の本文（0 本） | This tag isn't on any videos. Automatic tagging won't create "〈名〉" again. You can allow the name again from the rejected names below. |
| 却下の窓のボタン | Cancel / Reject（送信中 Rejecting…） |
| 確定のトースト | Confirmed "〈名〉" |
| 却下のトースト | Rejected "〈名〉" |
| `tag_not_tentative` のトースト | This tag was already confirmed, so the list was reloaded |
| 却下した名前の見出し | Rejected names |
| 却下した名前の説明 | Automatic tagging won't create these tags. Remove a name to allow it again. |
| 却下した名前が無いとき | No rejected names |
| 名前の × の読み上げ名 | Allow "〈名〉" again |
| 却下した名前を取れなかったとき | Couldn't load the rejected names |
| 取り外せなかったとき | Couldn't remove "〈名〉": {理由} |
| `errorText` の `tag_not_tentative` | The tag is already confirmed. |

- 「Tentative」は目印の文言、「automatic tagging」は説明の文にだけ使う。「draft」「pending」は
  使わない（「pending」は取り込みの段階の語）。
- タグの名前は利用者のデータで、翻訳せずに埋め込む（i18n.md）。

## Tentative mark

仮のタグ（`tentative: true`）の目印は、どの画面でも同じ 1 つの形である。

- lucide `CircleDashed`、`shrink-0`、`text-fg-subtle`、`aria-hidden`。名前の**後ろ**に `gap-1` で
  置く。大きさはチップでは `size-3`（Folder の目印と同じ）、管理画面の行では `size-3.5`。
- 名前が省略される（`truncate`）ときも目印は省略されず、名前の末尾の後ろに残る（目印が
  `shrink-0`、名前が `min-w-0 truncate`）。
- 読み上げは、押せる要素ではその要素の読み上げ名に「(tentative)」を添え、押せない要素では
  視覚的に隠した「Tentative」を名前の後ろに置く。`title` を持つ要素の `title` は名前のまま
  （名前の全体を見せるためのもの）で、目印自身にツールチップは付けない。管理画面の行だけは、
  目印に `Tooltip`「Tentative」を付ける（下の「Row」）。
- 確定したタグには何も足さない。確定したタグの見た目はこの feature の前と同じである
  （受け入れ条件 5・7）。

## Video tag chips

### Library card and group card

[`CardTagRow`](../../web/src/library/CardTagRow.tsx) の各チップ（見えているチップ、「+N」の
ポップオーバーの中のチップ、測るためだけの並び）に上の目印を足す。

- 面のあるチップ（`bg-elevated`、`h-5`・`text-xs`・`text-fg-muted`）: 名前 → 目印。
- フォルダ由来だけのチップ（破線の枠）: Folder の目印 → 名前 → 仮の目印。2 つの目印は形が
  違うので並んでも混ざらない。
- チップの幅は目印の分だけ広がり、行に収まる数（「+N」の算出）はそのまま測り直した幅で決まる。
  高さは変えない（UI品質「余白のリズムとタイポグラフィ」）。
- 押したときの振る舞い（そのタグで絞り込む、選択中はカードの選択の切り替え）は仮かどうかに
  よらず同じ（要件 4、受け入れ条件 4）。
- グループのカードのタグの行は同じ部品なので同じ形になる。

### Video page

[`VideoTags`](../../web/src/player/VideoTags.tsx) のチップ（`h-6`・`text-xs`・`text-fg`）に目印を
足す。

- 面のあるチップ（名前 ＋ 縦線 ＋ ×）: 名前の部分（`Link`）の中で名前 → 目印。縦線と × は今の
  まま。× を押すとこの動画から外れ、応答を受けてチップが消える（今と同じ）。
- フォルダ由来だけのチップ（破線、× なし）: Folder の目印 → 名前 → 仮の目印。
- 名前で付けた応答（`POST /api/video-tags` の `tag`）の `tentative` をそのままチップに映す。
  「タグを追加」で無い名前を作ったときは確定したタグとして作られる（要件 16）ので目印は出ない。
  「タグを追加」の候補に仮のタグがあれば、選んで付いたチップは目印付きで並ぶ。
- 管理画面で確定・却下したあと、開いたままの再生画面のチップは、次に動画を取り直すまで古い
  ままでよい（014「Stale tags in other screens」と同じ）。押して `tag_not_found` になれば今の
  とおり取り直す。

## Tag management page

### Toolbar

操作の行（検索・「新しいタグ」）に、**仮のタグだけ**の押しボタンを足す。

- 部品は `Button` の secondary（`h-9`、検索の入力と同じ高さ）、lucide `CircleDashed`、文言
  「Tentative only」、`aria-pressed`。押している間は `IconButton` の `active` と同じ見え方
  （`border-accent-active`・`bg-accent-soft`・`text-link`）で、ツールバーの表示形式・並び順の
  選択と同じ「選んでいる」の形にする。独自の状態の色は作らない。
- 並びは `sm` 以上で、左から検索の入力（`flex-1 sm:max-w-sm`）→ 「Tentative only」→ 右端に
  「新しいタグ」（primary）。`sm` 未満では 1 行目に検索の入力、2 行目に「Tentative only」と
  「新しいタグ」を `grid grid-cols-2 gap-2` で並べる（どちらも幅いっぱい）。本文で最初に Tab が
  届くのは今までどおり検索の入力で、`/` のキーも今のまま。
- 押すと、一覧を `tentative` が真のタグだけにする。検索と重ねられる（両方で絞る）。件数の行は
  検索と同じ「3 of 12 tags」の形（分母は全タグ）。絞り込みはこの画面の状態で、URL・
  `localStorage` には載せない（Why this shape）。画面を離れると解除される。
- タグが 1 つも無いとき（「タグはまだありません」の状態）は検索の入力と同じく `disabled`。
  読み込み中・読み込み失敗でも `disabled`。
- 改名中の行は、検索と同じく、絞り込みで一致しなくなっても一覧から外さない（改名すると
  確定になり、絞り込みの外へ出る。改名の応答を受けて閉じたときに外れる）。

### Row

仮のタグの行は、名前の列と操作の一群だけが変わる。行の高さ（`py-2`）・本数の列・改名の入力は
今のまま（UI品質「情報密度」）。

- **名前の列**: 名前の `Link` の後ろに `gap-1` で仮の目印（`size-3.5`）。目印には `Tooltip`
  「Tentative」と視覚的に隠した「Tentative」を付ける。名前とシノニムの行の書式は変えない。
  仮のタグはシノニムを持たない（要件 9）ので、仮の行にシノニムの行は出ない。改名中は入力に
  置き換わるので目印も出ない。
- **操作の一群**（左から）: 「確定する」（`IconButton` ghost・`sm`、lucide `Check`）→ 「改名」
  → 「シノニム」→ 「その他の操作」。「確定する」は仮の行にだけあり、確定した行には無い
  （確定した行の操作は今と同じ 3 つ）。「確定する」を先頭に置くのは、片付けの主の操作を
  行の操作の中で最初に触れる位置にするためで、色は他の `IconButton` と同じ（アクセント色・
  面は持たない。UI品質「視覚的階層」）。
- **「その他の操作」のメニュー**（仮の行）: 「別のタグへ統合…」（`Merge`）、区切り線、
  「却下する…」（lucide `Ban`、`tone="danger"`）。「削除…」は出さない（要件 10、受け入れ条件 12）。
  確定した行のメニューは今のまま（「別のタグへ統合…」、区切り線、「削除…」）。
- 改名・シノニムの追加・統合先になったとき、応答の `Tag` は `tentative: false` で返る
  （[contracts/screen-api.md §1](contracts/screen-api.md#1-変わる既存の経路)）。行はその 1 件で
  差し替わり、目印と「確定する」が消え、メニューが確定した行の形になる（受け入れ条件 11）。
  改名で今と同じ名前を送ったときは何も変わらず、仮のままである。
- 「シノニム」の窓（014「Synonyms」）は仮のタグでも同じ窓で開く。上のチップの並びは空で、
  「シノニムを追加」で名前を足すとタグが確定する。窓の文言は変えない。

### Confirm

- 押すとすぐ `POST /api/tags/{id}/confirm` を送る。確認の窓は無い（取り消しの費用が無い。
  仮に戻す操作は要件に無いが、確定は「タグが使える」状態を変えないので取り消す理由も無い）。
- 送信中はそのボタンのアイコンを `LoaderCircle`（`animate-spin motion-reduce:animate-none`）にし
  `aria-busy`、その行の「改名」「その他の操作」を `disabled`（改名の送信中の `blockStart` と
  同じ扱い。ほかの行はそのまま）。
- `200` を受けたら、応答の `Tag` で行を差し替える（目印と「確定する」が消える。受け入れ条件 7）。
  トースト「Confirmed "〈名〉"」（削除・統合のトーストと同じ）。フォーカスは、消えた「確定する」の
  代わりに同じ行の「改名」へ移す。「Tentative only」を押している間はその行が一覧から外れるので、
  削除と同じ規則で次の行の「改名」、無ければ前の行、1 つも無ければ「Tentative only」へ移す
  （「新しいタグ」ではなく絞り込みのボタンへ移すのは、その次の操作が「絞り込みを外す」だから
  である）。
- 既に確定していた（別のタブで先に確定。応答は `200` のまま）ときも同じに扱う。
- 失敗（`tag_not_found`）は今の「タグがもう無い」と同じ: トースト「This tag no longer exists,
  so the list was reloaded」、一覧を取り直し、削除と同じ規則でフォーカスを移す。その他の失敗は
  トーストで `errorText` を出し、行は変えない（行の中に失敗の 1 行を置く場所が無い。改名中の
  入力の下の行と違い、この操作は入力を持たない）。

### Reject

- 「却下する…」は `ModalFrame` の窓「Reject "〈名〉"」を開く。014 の「削除…」の窓と同じ骨格
  （本文の段落、secondary「Cancel」（最初のフォーカス）と danger「Reject」）で、本文は上の
  「Words」の 2 つ（N 本・0 本）。本数は `GET /api/tags` の `videoCount`。「却下した名前から
  戻せる」を本文に書くのは、削除と違って取り消しの入口があることを、押す前に知らせるためである。
- 実行中は両方のボタンを `disabled`、danger のボタンに `LoaderCircle`（削除の窓と同じ）。
- `204` を受けたら窓を閉じ、行を一覧から消し、トースト「Rejected "〈名〉"」、フォーカスは削除と
  同じ規則（次の行の「改名」→ 前の行 → 「新しいタグ」。「Tentative only」中は「新しいタグ」の
  代わりに「Tentative only」）。却下した名前の一覧（下）を取り直す。
- `409 tag_not_tentative`（別のタブや API で先に確定された）は窓を閉じ、トースト「This tag was
  already confirmed, so the list was reloaded」、一覧を取り直す（Edge Case「操作の競合」、
  screen-api.md §2）。`404 tag_not_found` は今の「タグがもう無い」と同じ。その他の失敗は窓の中に
  `text-sm text-danger`（`role="alert"`）の 1 行で `errorText` を出し、窓は開いたまま（削除の窓と
  同じ）。
- 「キャンセル」・Esc は何も変えず閉じ、フォーカスをその行の「その他の操作」へ戻す（削除の窓と
  同じ）。

### Merge

- 仮の行の「別のタグへ統合…」は今の窓のまま。統合先の候補に仮のタグも出る（候補に目印は
  足さない。上の「それ以外は変えない」）。応答の `Tag`（統合先）は `tentative: false` で、
  統合先の行の目印が消える（Edge Case「仮のタグどうしの統合」、受け入れ条件 11）。
- 確認の文言は変えない。統合元が仮でも「"X" and its synonyms become synonyms」の文で不都合は
  無い（仮のタグのシノニムは空なので、その部分は空集合を言うだけ）。

### Rejected names

一覧（`divide-y` の行の並び）と空の状態の**下**に、折りたたみの一群を 1 つ置く。読み込み中・
読み込み失敗（タグの一覧をまだ一度も取れていない）のときは置かない。

- **見出し**: `h2` の中の 1 つのボタン（`text-sm font-medium text-fg`、`aria-expanded`）。左に
  lucide `ChevronRight`（`size-4`、開くと `rotate-90`、`motion-reduce:transition-none`）、
  文言「Rejected names」、その後ろに `text-xs text-fg-muted tabular-nums` で件数（「3」。0 なら
  「0」）。件数は「片付けたものがここにある」を閉じたままで伝えるためのもので、仮のタグの件数
  ではない（対象外「仮のタグの件数による通知やバッジ」には当たらない）。上の一覧との間は
  `mt-6`。閉じた状態で始まり、開閉はこの画面の状態（URL・`localStorage` には載せない）。
- **中身**（開いたとき、`mt-2`）: `text-xs text-fg-muted` の説明の 1 行「Automatic tagging won't
  create these tags. Remove a name to allow it again.」、その下に名前の並び（`ul`、
  `flex flex-wrap gap-1.5`、`mt-2`）。各名前は 014 のシノニムの窓のチップと同じ形（`Chip` の
  neutral（本文の面は `bg` なので `bg-elevated` でよい）に × を足したもの、`h-6`・`text-xs`）。
  名前は 1 行で省略し `title` に全体、× は `size-6` の正方形、lucide `X`（`size-3`）、読み上げ名
  「Allow "〈名〉" again」。並びは API の順（名前の自然順）。
- **取り外し**: × を押すとすぐ `DELETE /api/tags/rejected-names?name=…` を送る。確認は
  挟まない（UI品質「取り消しに当たるので確認を挟まずに済ませてよい」）。送信中はその × を
  `disabled`（`opacity-50`）。`204` でチップを消す（もう無かった名前でも `204` なので同じ。
  Edge Case）。トーストは出さない（シノニムの解除と同じ。チップが消えることが結果である）。
  フォーカスは、次のチップの × へ、無ければ前のチップの × へ、最後の 1 つなら見出しのボタンへ
  移す（014 の再生画面の × と同じ規則）。失敗は並びの下に `text-xs text-danger`（`role="alert"`）
  の 1 行「Couldn't remove "〈名〉": {理由}」で、チップは残す。次の取り外しで消える。
- **読み込み**: 一覧は画面を開いたときにタグの一覧と一緒に `GET /api/tags/rejected-names` で
  取る（見出しの件数のため）。取れるまで見出しの件数は出さず、開いた中身は `Skeleton`
  （`h-6 w-24`）を 3 つ並べる。失敗したら見出しの件数は出さず、開いた中身に `text-xs text-danger`
  の 1 行「Couldn't load the rejected names」と `Button` の ghost・`sm`「Retry」を出す。
- **取り直し**: 却下の `204` のあと、「新しいタグ」で作成したあと、改名のあと、シノニムの追加の
  あとに取り直す（それぞれ却下した名前を一覧から外しうる。要件 15、受け入れ条件 14）。取り直しは
  中身が開いていても閉じていても行い、開いていれば並びがその場で変わる。
- **空**: 名前が 1 つも無いときは、説明の 1 行の代わりに `text-xs text-fg-muted` の「No rejected
  names」だけを出す。見出しは出したまま（この一群がどこにあるかを、初めて却下する前から
  分かるようにする）。
- 検索・「Tentative only」はこの一群に効かない（タグではない）。「タグはまだありません」の
  空の状態のときも置く（タグが 0 でも却下した名前はありうる）。

### States

014 の「States」の表に足す・変える。

| 状態 | 見え方 |
| --- | --- |
| 「Tentative only」で仮のタグが無い（検索は空） | `EmptyState`（`CircleDashed`）「No tentative tags」、説明「Tags created by automatic tagging appear here until you confirm or reject them.」、`Button`「Show all tags」。押すと絞り込みを外し、フォーカスを「Tentative only」へ移す |
| 「Tentative only」と検索で一致が無い | `EmptyState`（`SearchX`）「No tentative tags match "〈入力〉"」、`Button`「Show all tags」。押すと絞り込みと検索の両方を外し、フォーカスを検索の入力へ移す |
| 確定の送信中 | 「確定する」が `LoaderCircle`、同じ行の「改名」「その他の操作」が `disabled` |
| 却下の送信中 | 窓のボタンが `disabled`、「Reject」に `LoaderCircle` |
| タグが既に確定していた（`tag_not_tentative`） | 窓を閉じ、トースト「This tag was already confirmed, so the list was reloaded」、一覧を取り直す |
| 却下した名前の取り外しの送信中 | その × が `disabled`（`opacity-50`） |

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md 4）。判定する幅は
360px・768px・1280px。

| 幅 | 管理画面の操作の行 | 仮の行の操作 | 却下した名前 | カードのチップ |
| --- | --- | --- | --- | --- |
| 1280px | 検索（`max-w-sm`）・「Tentative only」・右端に「新しいタグ」が 1 行 | 4 つの `IconButton` が 1 行、名前の列が残りを取る | 見出しの行と、折り返す名前の並び | 目印の分だけチップが広がり、収まる数が減れば「+N」が増える |
| 768px | 同上 | 同上 | 同上 | 同上 |
| 360px | 1 行目に検索、2 行目に「Tentative only」と「新しいタグ」が半分ずつ | 4 つの `IconButton`（`h-8 w-8`・`gap-1`）と本数の列（`w-16`）の右に、名前の列が省略されて残る。横スクロールは出ない | 名前の並びが折り返し、長い名前はチップの中で省略 | 同上 |

- 仮の行の名前の列は 360px で約 120px になるが、名前は 1 行で省略され `title` で全体を確かめ
  られる（今の 3 つの操作のときと同じ扱いで、1 つ増えた分だけ短くなる）。
- 却下の窓・統合の窓は `ModalFrame` の今の幅の扱い（狭い幅で全幅）に従う。

## Review criteria

判定は実機で見て行う（library-ui.md 5）。「ある」だけでは満たさない（Q-4）。

- **視覚的階層**: ライブラリを開いて、仮のタグが付いた動画と付いていない動画のカードを並べた
  とき、題名 → タグの名前 の順に目が行き、破線の丸は名前を読んだあとに気づく程度で、色は名前
  より薄い。カードの面・チップの面・文字の色は仮でも確定でも同じ。管理画面でも同じで、名前の
  列は名前が主のまま、目印は名前の後ろに小さく付くだけである。「確定する」は他の `IconButton`
  と同じ重さで、アクセント色・面・枠が無い。「Tentative only」は押している間だけ選んでいる形
  （`accent-soft` の面）になり、押していないときは検索の入力より目立たない（UI品質「視覚的階層」
  「要求を満たしたことにならないもの」）。
- **情報密度**: 1280×800 で管理画面を開いたとき、仮の行と確定した行の高さが同じで、この feature
  の前と同じ行数（014「Visual review criteria」の 12 行以上）が 1 画面に見える。仮の行に増えて
  いるのは目印と `IconButton` 1 つだけで、文字のラベルや 2 行目は増えていない。却下した名前は
  閉じた見出し 1 行だけを占め、開くまで名前は見えない（UI品質「情報密度」）。
- **余白のリズム**: 操作の行・件数の行・一覧の間隔、行の `py-2`、カードの題名とタグの行の間隔
  （`gap-1`）、再生画面の題名とタグの間隔（`gap-2`）が、この feature の前と同じ。チップの高さ
  （`h-5`・`h-6`）も同じで、目印の分だけ横に広がるだけである。却下した名前の一群は一覧から
  `mt-6` 離れ、一覧の行の並びの一部に見えない（UI品質「余白のリズムとタイポグラフィ」）。
- **タイポグラフィ**: 新しい文字の大きさ・太さが無い。名前は仮でも確定でも同じ書式で、
  目印はアイコンだけで文字を足さない。件数の行の「3 of 12 tags」は検索のときと同じ書式。
- **操作の優先順位**: 「Tentative only」を 1 回押すと仮のタグだけが並び、各行で「確定する」を
  1 回押すだけでその行が片付き（絞り込み中は一覧から消え）、次の行の「改名」にフォーカスが
  移る。却下は「その他の操作」→「却下する…」→「Reject」の 3 回で、確定より 2 回多い。仮の行の
  メニューに「削除…」は無く、確定した行のメニューは今のままである（受け入れ条件 12）。却下した
  名前の × は 1 回で消え、確認の窓は出ない（受け入れ条件 13）。
- **片付けの結果**: 仮のタグ「高画質」を却下すると、行が消え、「Rejected names」の件数が 1 増え、
  開くと「高画質」のチップがある（受け入れ条件 8）。その名前を「新しいタグ」で作ると、チップが
  消える（受け入れ条件 14）。仮のタグを確定・改名・シノニムの追加・統合先にすると、その行の
  目印と「確定する」が消える（受け入れ条件 7・11）。
- **キーボード**: 管理画面で Tab は 検索 → 「Tentative only」→ 「新しいタグ」→ 行の名前 →
  「確定する」→ 「改名」→ 「シノニム」→ 「その他の操作」→ … → 「Rejected names」の見出し →
  （開いていれば）各名前の × と進む。`/` で検索へ移る。却下の窓の Esc は窓だけを閉じる。
- **ゲスト**: ログアウトして公開された動画のカードと再生画面を見ると、仮でも確定でもタグが出ず、
  管理画面には入れない（受け入れ条件 4）。
- **要求を満たしたことにならない例**（UI品質）: 仮のタグのチップや行が警告色・別の面の色・
  バッジを持ち、確定したタグより先に目に入る。管理画面のどこかに仮のタグの件数が通知やバッジの
  形で出る（「Rejected names」の件数は却下した名前の数で、これに当たらない）。仮のタグを
  見つけるのに「Tentative only」以外の手（全行を読む）が要る。仮の行の高さが確定した行と違う。
  却下した名前が別の画面や窓にあり、却下の直後に同じ画面で確かめられない。

## Colour

新しく使う組は無い。目印の `fg-subtle` はアイコンだけに使い、文字には使わない。「Tentative only」の
押している形（`link` on `accent-soft`）は `IconButton` の `active` と `SegmentedControl` の選んでいる
形と同じ組である。却下した名前のチップは `fg` on `elevated`、説明と件数は `fg-muted` on `bg`、
失敗の行は `danger` on `bg`（管理画面の本文）と `danger` on `elevated`（却下の窓。014 が `pairs` に
含めた組）。`tokens.test.ts` の `pairs` には足さない。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前だけを決める（014 と同じ範囲）。

- 目印: `aria-hidden` のアイコンと、視覚的に隠した「Tentative」または読み上げ名の「(tentative)」。
- 「Tentative only」: `aria-pressed`。件数の行は今の `role="status"`（`polite`）で絞り込みの
  結果を伝える。
- 「確定する」: 読み上げ名「Confirm」、送信中は `aria-busy`。
- 却下した名前: 見出しのボタンは `aria-expanded`、中身の `ul` に `aria-label`「Rejected names」、
  × は「Allow "〈名〉" again」。失敗の行は `role="alert"`。
