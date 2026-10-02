# UI Design: 動画とグループをお気に入りにする

**Feature**: [parent Issue #574](https://github.com/syudead/vv/issues/574) ・
[plan.md](plan.md) ・ [contracts/screen-api.md](contracts/screen-api.md) ・
[research.md R-2](research.md#r-2-付け外しは所有者だけの-1-つの経路-put-apifavorites-で動画の-id-とフォルダを-1-つの取引で受け無いものは数えずに飛ばす)・
[R-6](research.md#r-6-画面はドメインイベントを足さず公開の切り替えと同じ購読の仕組みで一覧と再生画面に反映し再生画面は動画を取り直す)・
[R-7](research.md#r-7-複数選択は選んだグループをグループとして覚え一括のお気に入りではグループのメンバーを動画として送らない)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・一覧と再生画面の構成: [ライブラリ UI](../../docs/design-docs/library-ui.md)
  （「6. 一覧の構成」のカード・グループのカード・リスト表示・選択バー・ツールバー、「8. 再生画面の構成」）
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`。値は写さず、名前で呼ぶ
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- カードの箱と選択のチェックの出し方（hover・フォーカス・`hover:none` の端末）:
  今の [`web/src/videoList/VideoCard.tsx`](../../web/src/videoList/VideoCard.tsx) と
  [`web/src/library/GroupCard.tsx`](../../web/src/library/GroupCard.tsx)、
  [specs/017-folder-groups/ui-design.md「Group card」](../017-folder-groups/ui-design.md#group-card)
- カードの印の先例（公開の印）: [specs/016-single-account-auth/ui-design.md「Visibility toggle」の「Card」](../016-single-account-auth/ui-design.md#card)
- 再生画面の情報の行と右端の二次的な操作の一群: [specs/012-video-detail-ia/ui-design.md「Video facts」](../012-video-detail-ia/ui-design.md#video-facts)、
  [specs/029-video-overrides/ui-design.md「Capture button」](../029-video-overrides/ui-design.md#capture-button)
  と今の [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx)
- 選択バーの構成とメニューの先例（「公開」）: [specs/016-single-account-auth/ui-design.md「Selection bar」](../016-single-account-auth/ui-design.md#selection-bar)、
  [specs/030-video-versions/ui-design.md「Bundle action」](../030-video-versions/ui-design.md#bundle-action)
  と今の [`web/src/library/SelectionBar.tsx`](../../web/src/library/SelectionBar.tsx)・
  [`VisibilityMenu.tsx`](../../web/src/library/VisibilityMenu.tsx)
- 絞り込みのポップオーバーと並べ替えのメニュー: [specs/013-library-search/ui-design.md「Sort and direction」「Filter menu」](../013-library-search/ui-design.md#sort-and-direction)、
  [specs/033-video-dates/ui-design.md「Sort and direction」](../033-video-dates/ui-design.md#sort-and-direction)
  と今の [`web/src/videoList/FilterMenu.tsx`](../../web/src/videoList/FilterMenu.tsx)・
  [`SortControls.tsx`](../../web/src/videoList/SortControls.tsx)・[`listCriteria.ts`](../../web/src/videoList/listCriteria.ts)
- ゲストへの縮退: [specs/016-single-account-auth/ui-design.md「Guest degradation」](../016-single-account-auth/ui-design.md#guest-degradation)
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足すのは次の 5 つで、どれも所有者だけに出る。新しい色・半径・影のトークンは
足さない。

1. ライブラリとフォルダ画面の**カードと行の付け外し**（動画のカード・行、グループのカード・行。
   要件 6・7、受け入れ条件 1・3）
2. 再生画面の**二次的な操作の段の付け外し**（要件 6・7、受け入れ条件 1・6）
3. 選択バーの**「Favorite」のメニュー**（要件 6、受け入れ条件 7）
4. 絞り込みの**「Favorites only」**（要件 8・9・11、受け入れ条件 4・5・9）
5. 並べ替えの**「Date favorited」**（要件 10・11、受け入れ条件 8）

変えないもの: 普通のフォルダのカード（`FolderCard`。対象外）、再生画面のグループの行（「Group line」。
要件 6 の再生画面の入口は**再生中の動画**のお気に入りだけで、グループのお気に入りはライブラリの
グループのカードと複数選択から付ける）、関連動画とメンバーの並びの行、再生終了の層、サイドバー
（お気に入りだけの画面や入口は作らない。UI品質「要求を満たしたことにならない変更」）。ゲストの画面は
どこも変わらない（「Guest degradation」）。

## Words

英語の文言は、同じ画面で既に使っている語との衝突だけで決める。

| 場所 | 英語（案） |
| --- | --- |
| 付け外しの読み上げ名（カード・行、動画） | Favorite "〈題名〉" |
| 付け外しの読み上げ名（カード・行、グループ） | Favorite group "〈名〉" |
| 再生画面の付け外しの読み上げ名・ツールチップ | Favorite |
| 付け外しの状態 | `aria-pressed`（true が お気に入り） |
| 再生画面で変えられなかった 1 行 | Couldn't change the favorite: {理由} |
| カード・行で変えられなかったトースト | Couldn't change the favorite: {理由} |
| 選択バーの引き金 | Favorite |
| 選択バーのメニューの項目 | Add to favorites ／ Remove from favorites |
| 一括で付けたトースト | Added {N} items to favorites（N は `appliedVideos + appliedFolders`。1 なら item） |
| 一括で外したトースト | Removed {N} items from favorites |
| 一括で変えられなかったトースト | Couldn't change the favorites: {理由} |
| 一括の上限の理由 | 今の `too_many_videos` の文（タグ・公開と同じ）。数えるのは送る `videoIds` と `folders` の合計 |
| 絞り込みのチェック | Favorites only |
| 並べ替えの種類 | Date favorited |
| 並べ替えの向きの言い換え | oldest first ／ newest first |

- 「Favorite」を動詞としても名詞としても使い、「Like」「Save」「Star」は使わない。「Star」は段階の
  評価（対象外）に読め、「Save」はダウンロードに読め、「Like」は他の人に見える反応に読める。
  お気に入りは所有者だけの印で、オンとオフしか持たない（要件 1・12）。
- 一括のトーストの単位は **items** で、videos ではない。グループは 1 つの項目として付くので
  （要件 6）、`appliedVideos + appliedFolders` を videos と呼ぶと、選んだ本数（メンバーを数える）と
  食い違う。ライブラリの件数の行が項目を items と数えるのと同じである。
- 並べ替えは「Date favorited」で、「Recently favorited」にしない。親 Issue の語は「お気に入りにした
  日時」で、メニューの「Date added」「Date modified」「Date created」と同じ「Date …」の形にすると、
  向きの言い換え（oldest first／newest first）もその 3 つと同じ語で済む。「Recently played」が
  「Recently」なのは、再生は何度も起き「最後に再生した」ことを言う必要があるからで、お気に入りの
  日時は付けた 1 回の日時である。

## Mark

印は lucide `Heart` の 1 つで、オンは塗り（`fill-current`）、オフは線だけにする。どの入口でも同じ
絵・同じ塗りの規則にし、付け外しの操作そのものが今の状態を見せる（要件 7）。

- **星ではなくハート**にするのは、段階の評価を持たない（要件 1、対象外「評価」）ことを絵で
  言うためである。星は 5 段階の評価の絵として定着していて、1 つだけでも「何点か」を期待させる。
  写真のアプリやメディアサーバーの「お気に入り」はハートに収束している。
- **オンの色は `text-link`**（押下・選択の状態の色）で、赤にしない。赤（`danger`）は危険と失敗の
  意味別の色で、お気に入りに使うとカードの上で「再生できない」の警告と競う（library-ui.md §1）。
  `IconButton` の `active` の状態（`bg-accent-soft text-link`）と同じ色なので、再生画面では
  その状態をそのまま使う。
- **オフは今の文字の色**（カードの上の面では `text-fg`、行と再生画面では `text-fg-muted`）の線だけの
  ハートで、面も枠も足さない。付いていないカードが今より騒がしくならないように、オフの印は
  ポイントかフォーカスしたときだけ現れる（下の「Card」）。
- 塗りに加えて `aria-pressed` が状態を持つ。色の違い（`link` と `fg`）だけに頼らず、塗りの有無で
  見分けられる。

## Card

ライブラリの格子（`VideoCard`・`GroupCard`）とフォルダ画面・その検索結果の動画のカードに、
所有者だけ、付け外しを置く。

### Placement

- サムネイルの枠の**右上**（`top-2 right-2`）に、選択のチェック（左上）と対になる位置で置く。
  動画のカードは `z-20`（チェックと同じ）、グループのカードは `z-30`（そのチェックと同じ。
  前に出たサムネイルより上）。
- 部品は `button`（`type="button"`、`aria-pressed`）で、カードのリンク（`Link`）の**外**、同じ
  `article` の中に置く（チェックと同じ。リンクの中にボタンを入れない）。DOM の順は チェック →
  リンク → **付け外し** → タグの行。Tab で先に届くのは今までどおりカードのリンク（開く）で、
  お気に入りはその次である（UI品質「操作の優先順位」）。
- 大きさは `size-6` の正方形（タグのチップの × と同じ。029「Thumbnail fact」の解除の × と同じ
  大きさ）、中のハートは `size-4`。面は長さの印と同じ `rounded-sm bg-navbar/90 backdrop-blur-sm` で、
  明るいサムネイルの上でも線のハートが読めるようにする。チェック（`size-5`）より一回り大きいのは、
  チェックが選択モードでだけ主に押されるのに対し、こちらはふだんの一覧で押す操作だからである。
- 行を増やさず、題名・タグの行・長さの印・公開の印・進捗の帯の位置と間隔は変えない（UI品質
  「情報密度」「余白のリズム」）。公開の印（`Globe`、右下の長さの面の中）とは角が違うので重ならない。
- **採らない置き場所**: 長さの印の中（公開の印と同じ場所）。リンクの中なので押せる印にできず、
  要件 7（状態と操作が同じ場所）を満たさない。題名の行の右端。2 行の題名と幅を取り合い、
  題名が 1 行で省略されるカードが増える。hover で出る操作のメニュー。付けるまでに 1 回増え、
  「付けるたびに選択の画面を挟む」に当たる。

### When the mark is shown

| 状態 | 格子のカード | リスト表示の行 |
| --- | --- | --- |
| オン | 常に見える（`opacity-100`） | 常に見える |
| オフ | `opacity-0`。カードを hover、カードの中にフォーカス（`group-focus-within`）、`hover:none` の端末で `opacity-100`（チェックと同じ条件） | 行を hover、行の中にフォーカス、`hover:none` の端末で見える |

- オフのハートを常に出さないのは、UI品質「付いていないときは、カードの見た目を今より騒がしく
  しない」による。付いているカードだけにハートがあるので、一覧を眺めるだけでお気に入りが分かる
  （UI品質「視覚的階層」）。
- 選択モード（1 件以上選んでいる）でも同じ条件で出し、押せる（plan の受け入れ条件）。チェックの
  ように常に出さないのは、選択モードの印はチェックだけで足りるからである。
- hover のプレビュー（動画・フォルダの絵柄）との関係はチェックと同じにする: ポインタがこのボタンに
  入ったらプレビューを止め（`data-preview-checkbox` と同じ扱いで、ボタンの上からプレビューを
  始めない）、プレビュー中は面を不透明の `bg-navbar` にする。
- 再生できない動画の全面の警告（`bg-overlay`）の上にも出る（`z-20` で警告より前）。お気に入りは
  再生可否と無関係である。

### Pressing

- 押すと `PUT /api/favorites` を 1 回送る。動画のカード・行は `videoIds: [id]`、グループのカード・
  行は `folders: [group.folder]`、`favorite` は今の状態の反対。確認の窓は出さない（UI品質）。
- 押してもカードのリンク（開く）と選択のチェックは動かない。`click` の伝播を止め、選択モードでも
  選択を切り替えない。
- 送っている間は `aria-disabled` にし、ハートを `LoaderCircle`（`animate-spin`、動きを減らす設定では
  止める）にする。もう一度押しても送らない。
- 応答を受けてから印を変える。動画のカード・行は一覧の項目の `favorite` をその場で差し替え、
  グループのカード・行は `GET /api/folders/{rootId}/group` で取り直して差し替える（R-6、017
  「Refresh and removal」）。取り直しの間、カードの見た目は変えない。`appliedFolders` が 0
  （もうグループではない）で取り直しが 404 のときは、そのカードを一覧から外す（017 と同じく
  何も伝えない）。
- 失敗したときはトースト「Couldn't change the favorite: {理由}」を出し、印は変えない。カードには
  失敗の行を置く場所が無い。
- グループに付けてもメンバーの動画のカードの印は変わらず、メンバーに付けてもグループのカードの
  印は変わらない（要件 4、受け入れ条件 3）。
- **お気に入りのみで絞り込んだ一覧**でカードから外しても、その場では一覧から外さず、ハートが線に
  なるだけである。一覧を取り直したとき（条件を変える、ページを読み込み直す）に消える（R-6）。
  その場で消すと隣のカードが動いて続けて押せない。「Date favorited」で並べた一覧で付け外しても、
  並びはその場では変えない（同じ理由）。
- 再生画面から戻ったときは一覧を取り直さず控え（`listSnapshot`）を復元するので、控えのそのカードの
  印は再生画面で変えた状態になっている（受け入れ条件 1、`listSnapshot` への反映）。お気に入りのみで
  絞り込んだ一覧から開いた動画を外して戻っても、そのカードは線のハートで同じ位置に残り、取り直す
  まで消えない（plan「再生画面にお気に入りの付け外しを置く」の Acceptance）。別のタブで変えた状態は、次に一覧を
  取り直したときに追う（Edge Case）。

### List view row

リスト表示（ライブラリだけ）の動画の行（`VideoRow`）とグループの行（`GroupRow`）は、題名の列の
**直後**に `w-8` の列を足し、そこに同じ `button`（`size-6`、ハート `size-4`）を置く。面は付けない
（行の面は `surface` で、サムネイルの上ではない）。オフは `text-fg-muted`、hover で `text-fg`、
オンは `text-link` の塗り。出す条件は上の表のとおりで、行のチェック（常に薄く見せる）とは違い、
オフは隠す。薄い印が 1 行に 2 つ並ぶと、どちらが選択か読みにくい。

## Video page

再生画面（`/videos/:id`）の情報の行の右端の操作の一群（`VideoFacts` の `actionsRef`、`ml-auto`）の
**先頭**（「Use current frame as thumbnail」の左）に置く（UI品質「操作の優先順位」: 再生 → 題名 →
タグ → 公開の切り替えの後、ファイルを開くなどの二次的な操作と同じ段）。

- 部品は `IconButton`（`size="sm"`、ghost、lucide `Heart`）で、`aria-pressed` を持つ。オフは他の
  2 つと同じ `text-fg-muted`、hover で `text-fg`。オンは `IconButton` の `active`（`bg-accent-soft
  text-link`）にし、ハートを塗る。読み上げ名とツールチップは「Favorite」。
- 先頭に置くのは、一群の中でこれだけが状態を持つ操作で、情報の行から目が右へ流れて最初に
  着く位置にあると、状態が「読める」からである。029 の「Capture button」が言う「先頭」は、
  その 3 つ（撮る・開く・コピー）の中での先頭のままで、本書がその左に 1 つ足す。
- 所有者では、所在が無い動画・プレイヤーの出ていない動画でも、この一群をこのボタンだけで出す
  （029 が撮るボタンで既にそうしているのと同じ）。
- 押すと `PUT /api/favorites`（`videoIds: [id]`）を送る。送っている間は `aria-disabled`、アイコンを
  `LoaderCircle`（回転）にする。`200` を受けたら `GET /api/videos/{id}` で動画を取り直し（R-6、
  plan）、取り直した `favorite` で塗りと `aria-pressed` が変わる。取り直しの間と失敗したときは
  前の状態のまま（033「Refresh after edits」と同じ）。トーストは出さない。塗りが変わることが
  結果である。
- 失敗したときは、情報の行の直下の 1 行（撮る・開くと共有する `role="alert"` の行）に
  「Couldn't change the favorite: {理由}」を出す。次に何かの操作をしたとき、または別の動画へ
  移ったときに消える。
- 一覧のカードで変えた状態は、`useVideoDetail` の購読で再生画面の 1 件に反映される（別タブは次の
  取り直し）。
- 見終わった動画でも印は残り、再生は先頭から始まる（要件 5、受け入れ条件 6）。再生終了の層・
  次の予告は変えない。
- `Video.group` がある動画（グループのメンバー）でも、ここで付くのはその動画のお気に入りだけで
  ある（要件 6）。グループの行（「Group line」）にお気に入りは置かない。

## Selection bar

「Remove tag」の**直後**、「Visibility」の前に「Favorite」（lucide `Heart` + 文言 + `ChevronDown`、
`Button` の ghost・`sm`）を置く。並びは 件数 → Add tag → Remove tag → **Favorite** → Visibility →
（2 本以上で）Bundle as versions → 縦線 → Select all → 解除。

- 押すと `ui/Menu` を上に開き（「公開」と同じ `side="top"`）、項目は「Add to favorites」（`Heart`）と
  「Remove from favorites」（lucide `HeartOff`）の 2 つ。「公開」と同じく 2 つのボタンにしないのは、
  `sm` 以上の 1 行が文言のボタン 5〜6 つになり、「Select all」との区切りより先に目に入るまとまりが
  長くなりすぎるためである。1 つの切り替えボタンにしないのは、「Select all」で読んでいないページを
  含めると今の状態が分からず、ボタンの塗りを決められないからである（016 と同じ理由）。
- タグの 2 つの直後に置くのは、お気に入りもタグも「印を付ける」操作で、公開は「見せる範囲を
  変える」操作だからである。公開の前に 1 つ入るので「Visibility」と「Bundle as versions」は右へ
  動く。
- 選んだ項目の今の状態は示さない。両方の項目は常に押せ、既に同じ状態のものは誤りにならない
  （Edge Case、[contracts/screen-api.md §1](contracts/screen-api.md#1-put-apifavorites)）。
- 確定すると `PUT /api/favorites` を 1 回送る。`folders` は選んだグループ（グループのカードの
  チェックで選んだもの、「Select all」の応答の `groups`）、`videoIds` は選んだ id のうち選んだ
  グループのメンバーでないもの（R-7）。グループのメンバーを 1 本でも外すと、そのグループは
  グループとしてではなく残ったメンバーが動画として送られる（017「Pressing and selection」の
  「全メンバーが選択に入っているときだけ選択中」と同じ考え方）。
- トースト「Added N items to favorites」「Removed N items from favorites」（N は
  `appliedVideos + appliedFolders`）を出し、選択は残す。カードの印は応答を受けてから変える
  （動画はその場、グループは取り直し）。失敗はトースト「Couldn't change the favorites: {理由}」で、
  選択を残す。
- 上限（20,000）は**送る数**（`videoIds` と `folders` の合計）で判定する。超えるときはタグ・公開と
  同じく引き金を `disabled` にし、同じ理由を添える。選んだ本数 `count`（メンバーを数える）では
  判定しない。メンバーが 20,000 本を超えるグループ 1 つは、タグ・公開では押せないままで、
  お気に入りは `folders` 1 つとして押せる（plan）。
- 「Select all」の無効の条件は、選択が応答の `ids` と同じ集合であることに加え、選んだグループが
  応答の `groups` と同じであること（plan）。メンバーを 1 本外して戻すと、id はそろっても
  グループはグループとして選ばれていないので、また押せる。

### Layout

- **`sm` 以上**: 今の 1 行に「Favorite」が 1 つ増える。収まる幅では今のとおり中身の幅の 1 行である。
  1 行が画面の幅（`px-4` の内側）に収まらないときは、今の `nowrap` のまま画面からはみ出させず、
  バーの幅を画面の幅いっぱいにして、次の順に 2 行目へ回す。
  1. 縦線から後ろの「Select all」と解除を **2 行目の右端**に回す。1 行目は件数と操作のまとまりになる。
  2. それでも 1 行目が収まらないとき（「12 videos selected」の件数で、2 本以上を選んで「Bundle as
     versions」を含む 5 つの操作になる 640〜767px。文言のボタン 5 つと間だけで 1 行目の幅を超える）は、
     `sm` 未満と同じ切れ目で操作のまとまりも割り、「Add tag」「Remove tag」を件数と 1 行目に残し、
     「Favorite」以降（Favorite → Visibility → Bundle as versions）を順のまま 2 行目に、縦線・
     「Select all」・解除の前に置く。2 行目は右端に寄せる。

  1 で済む幅では操作のまとまりを割らない。区切りより前が「選んだものに何をするか」の一群だから
  である（library-ui.md §6）。2 で割るときも、タグの 2 つと「Favorite」以降の 2 つのまとまりに
  分けるだけで、順は 1 行のときと同じに読める。操作名は短縮しない（library-ui.md §6）。どちらの
  段まで要るかは件数と操作の実際の幅で決まり、判定する幅（640・768px）で実機で確かめる。
  「Bundle as versions」を含む 5 つは今でも 768px で 1 行に収まっていないので、この規則はそれも直す。
- **`sm` 未満**: 上の段は今のまま（件数・Select all・解除）。下の段は「Add tag」「Remove tag」
  「Favorite」「Visibility」の 4 つ（2 本以上で「Bundle as versions」を足した 5 つ）を等分し、
  収まらない幅では「Add tag」「Remove tag」をその段に残し、「Favorite」以降を順のまま次の段の
  右端に回す。そこにも収まらなければ、さらに次の段の右端に回る。`SelectionBar` の容器の
  問い合わせの幅を 4 つ分・5 つ分に直す（4 つ: いちばん広い 1 つの幅 × 4 + 間 × 3、5 つは同様）。
  5 つが 1 段に収まる幅は `sm` 未満に無いので、2 本以上を選んだときは常に 3 段以上になる。
- 360px（iPhone SE）・390px: 件数の段 → 「Add tag」「Remove tag」の段 → 「Favorite」「Visibility」が
  右端に並ぶ段。2 本以上では「Bundle as versions」がその下の段の右端。
- 操作名を短縮しない（library-ui.md §6）。「Favorite」はアイコンだけにしない。

## Filter menu

絞り込みのポップオーバー（`FilterMenu`）に、所有者だけ、「Favorites only」のチェックを足す。

- 位置は視聴状態の `fieldset` の**直下**、「Playable only」の上。形は「Playable only」と同じ
  `label` + `input[type=checkbox]`（`size-4 accent-accent`、`text-sm text-fg`）で、アイコンは
  付けない。視聴状態の下に置くのは、どちらも所有者自身の印（見た・付けた）で、再生可否は
  ファイルの性質だからである。2 つのチェックの間は視聴状態の下の `mb-4` より詰めた `gap-2`
  （同じ性質の 2 行）。
- 入れると `favorite=true` で一覧を取り、URL に `fav=1` が付く。ボタンの数字に数え、ボタンは
  `bg-accent-soft text-link` になる（今の規則）。「Clear filters」で視聴状態・再生可否と一緒に
  外れる。並べ替えは残る。
- 検索語・タグ・視聴状態・再生可否とは AND で組み合わさる（要件 8・9）。一致が無いときは今の
  「No videos match these conditions」のままで、お気に入り向けの文言は足さない。文の補足
  「change the filters」が当てはまる。
- お気に入りのみの一覧で、お気に入りでないグループに属するお気に入りの動画は、動画のカードで
  出る（要件 9、受け入れ条件 4）。見た目は検索でメンバーが単独で出るときと同じで、区別の印は
  付けない。
- フォルダ画面も同じポップオーバーなので同じチェックが出る（要件 11）。フォルダ画面では
  グループは無く、動画のお気に入りだけで絞る。
- ゲストには出さない（「Guest degradation」）。

## Sort and direction

並べ替えの種類を 8 つから **9 つ**にする。ライブラリとフォルダ画面は `SortControls` を共有するので、
両方に出る（要件 11）。既存の 8 種の名前・アイコン・向き・言い換えは変えない。

| 種類 | 名前 | アイコン | 選んだときの向き | 昇順／降順の言い換え |
| --- | --- | --- | --- | --- |
| 追加日 | Date added | `CalendarArrowDown` | 降順 | oldest first／newest first |
| 更新日（ファイル） | Date modified | `CalendarClock` | 降順 | oldest first／newest first |
| 作成日 | Date created | `FileClock` | 降順 | oldest first／newest first |
| 題名 | Title | `ArrowDownAZ` | 昇順 | — |
| 長さ | Length | `Timer` | 降順 | shortest first／longest first |
| ファイルサイズ | File size | `HardDrive` | 降順 | smallest first／largest first |
| 最近再生した順 | Recently played | `History` | 降順 | least recently played／most recently played |
| **お気に入りにした日時** | **Date favorited** | **`CalendarHeart`** | 降順 | oldest first／newest first |
| ランダム | Random | `Shuffle` | — | — |

- **位置**は「Recently played」の直後、「Random」の前。所有者だけの 2 種（再生・お気に入り）が
  隣り合い、ゲストのメニューはその 2 つを除いた **7 種**で、残りの順は変わらない。
- **アイコン**は `CalendarHeart`。カレンダーの家族（追加・更新）で「日時」であることを、ハートで
  カードと再生画面の印と「同じもの」であることを言う。`Heart` そのものにしないのは、並べ替えの
  メニューの中で「お気に入りのみ」の絞り込みと取り違えないためである。
- 選ぶと `favoritedDesc`（最後に付けたものが先頭。受け入れ条件 8）、向きの切り替えで
  `favoritedAsc`。どちらの向きでもお気に入りでない項目は末尾にまとまる（Edge Case、R-4）。
  向きのボタンの読み上げ名とツールチップは今の形「Descending (newest first). Press for ascending」。
  グループの項目はグループをお気に入りにした日時の位置に出る。
- 端末の設定（`viewPreferences`）と URL（`sort=favoritedDesc`・`favoritedAsc`）の往復は、他の種類と
  同じ。
- `md` 未満の「表示と並び順」のまとめでは、2 列のラジオが所有者で 5 行（9 種）、ゲストで 4 行
  （7 種）になる。メニューの順を `grid-cols-2` に行優先で流し込み、並びを組み替えない。所有者では
  4 行目が「Recently played」「Date favorited」、5 行目が「Random」（左だけ）。ゲストでは 4 行目が
  「Random」（左だけ）。「Date favorited」の文字が 2 列の幅で 1 行に収まり、`truncate` で切れない。
- メニューボタンの文字は種類の名前だけ（「Date favorited」）。ボタンの幅は「Date modified」と
  同程度で、ツールバーの他の部品の位置を動かさない。

## Guest degradation

ゲストとして描くとき、次を**出さない**。`disabled` にも `aria-disabled` にもしない（016 と同じ）。

| 場所 | 出さないもの | 代わりに |
| --- | --- | --- |
| カード・行（ライブラリ・フォルダ画面・検索結果） | お気に入りの印と付け外し | 応答に `favorite` が無い。公開の印と同じく、ゲストには意味が無い |
| 再生画面 | 情報の行の右端の付け外し | 応答に `location` も `favorite` も無いので、右端の一群ごと出ない（今のとおり） |
| 選択バー | — | ゲストには選択バーそのものが無い |
| ツールバー | 絞り込みの「Favorites only」、並べ替えの「Date favorited」 | ゲストの並べ替えは 7 種。URL に `fav=1`・`sort=favoritedAsc`・`favoritedDesc` が残っていたときは、`watch`・`played*` と同じく既定に丸めてから要求し、URL も直す（[guest-api.md §3](../016-single-account-auth/contracts/guest-api.md#3-ゲストが使えない条件) の表に足す）。端末に保存した並び順が `favorited*` のときも同じく丸め、保存値は書き換えない |

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md §4）。判定する幅は
360px・640px・768px・1280px。

| 幅 | カード | 選択バー | 再生画面 | ツールバー |
| --- | --- | --- | --- | --- |
| 360px（`sm` 未満） | 格子は全幅の 1 列。ハートはサムネイルの右上に `top-2 right-2` で、チェック（左上）と同じ高さ。`hover:none` の端末では線のハートが全カードに見え、長さの印・公開の印と重ならない | 3 段（2 本以上で 4 段）。「Favorite」「Visibility」が右端に並び、文字が切れない | 情報の行の右端の操作（4 つ）が行の末尾に回り、横スクロールは出ない | 「Favorites only」はポップオーバーの中。並べ替えは「表示と並び順」のまとめの 2 列 × 5 行 |
| 640px（`sm`） | 2 列 | 1 本の選択では件数と 4 つの操作が 1 行目、「Select all」と解除が 2 行目の右端。2 本以上（5 つの操作）では件数と「Add tag」「Remove tag」が 1 行目、「Favorite」「Visibility」「Bundle as versions」・縦線・「Select all」・解除が 2 行目の右端。画面からはみ出さない | 同上 | 同上 |
| 768px（`md`） | 2〜3 列。印の大きさは変わらない | 1 本の選択では 1 行。2 本以上（「Bundle as versions」あり）では「Select all」と解除が 2 行目の右端で、それでも 1 行目が収まらなければ「Favorite」以降も 2 行目（Layout の 2）。画面からはみ出さない | 左の列に操作の一群が 1 行で収まる | 並べ替えのメニューボタンに「Date favorited」が 1 行で収まり、向きのボタンと接したまま |
| 1280px（`lg` の 2 列） | 4〜5 列。表示倍率を変えても印の位置は角のまま | 常に 1 行 | 同上 | 同上 |

## Review criteria

判定は実機で見て行う（library-ui.md §5）。「ある」だけでは満たさない（Q-4）。

- **視覚的階層（カード）**: 所有者のライブラリで、お気に入りのカードは塗りのハートが角に 1 つあるだけで、
  サムネイルと題名より先に目に入らない。お気に入りでないカードは、この feature の前と並べて
  違いが無い（ポイントしていないとき）。画面を眺めるだけで、どのカードがお気に入りかを数えられる
  （UI品質「視覚的階層」）。塗りのハートが `link` の色で、赤・黄など意味別の色になっていない。
- **情報密度・余白のリズム（カード）**: カードの高さ、題名とタグの行の間隔（`gap-1`・`pt-1`・`pb-3`）、
  長さの印と公開の印の位置が前と同じ。ハートを置いたことで題名が 1 行短くなったり、タグの行が
  詰まったりしていない。リスト表示の行の高さが前と同じで、題名の列が 1 列分（`w-8`）だけ狭い。
- **操作の優先順位（カード）**: ハートを押しても再生画面が開かず、選択モードで押しても選択が
  変わらない。Tab はチェック → カード（開く）→ ハート → タグの順に着く。ポイントしていない
  カードで、キーボードでカードに着くとハートが現れる。タッチの端末（360px）ではポイントなしで
  ハートが見え、1 回のタップで付く。付けるときに確認も選択の画面も挟まない。
- **状態と操作が同じ場所（要件 7）**: カード・行・再生画面のどれでも、押した場所のハートが塗りに
  変わり、もう一度押すと線に戻る。再生画面で付けた動画が、戻ったライブラリのカードでも塗りに
  なっている（受け入れ条件 1）。グループのカードで付けても、メンバーの動画のカード（フォルダ画面・
  検索結果）のハートは線のまま（受け入れ条件 3）。
- **再生画面の階層**: プレイヤー → 題名 → タグ → 公開の切り替え → 情報の行の順に目が行き、
  ハートは「ファイルを開く」「パスをコピー」と同じ大きさ・同じ薄さの一群の先頭にある。オンでも
  `bg-accent-soft` の小さな面と塗りだけで、題名や公開の切り替えより目立たない。失敗の行は
  情報の行の直下の 1 行で、他の失敗（開けない・サムネイル）と同じ場所。
- **選択バー**: 動画 2 本とグループ 1 つを選んで「Favorite」→「Add to favorites」で、3 枚のカードの
  ハートが塗りになり、グループのメンバーのカードは変わらず、トーストが「Added 3 items to
  favorites」（受け入れ条件 7）。「Favorite」が「Remove tag」と「Visibility」の間にあり、`sm`
  以上で「Select all」の区切りより前のまとまりとして読める。640px で 2 本以上を選んでも画面から
  はみ出さず、2 行目に回った「Favorite」以降が 1 行目の操作の続きとして読める。360px で 3 段（2 本以上で
  4 段）になり、どの段の文字も切れない。
- **絞り込み**: 「Favorites only」を入れると、お気に入りの動画とお気に入りのグループだけが出て、
  お気に入りでないグループのお気に入りのメンバーは動画のカードで出る（受け入れ条件 4・5）。
  絞り込みのボタンの数字に数えられ、タグの絞り込みと同時に使うと両方に当たるものだけ
  （受け入れ条件 9）。「Clear filters」で外れ、URL の `fav=1` が消える。フォルダ画面でも同じ
  チェックがある（要件 11）。
- **並べ替え**: メニューに「Date favorited」が「Recently played」の直後にあり、アイコンが他の 8 つと
  違って見える。選ぶと最後に付けたものが先頭で、お気に入りでない項目が末尾にまとまる
  （受け入れ条件 8）。向きを変えても末尾は末尾のまま。ページを読み直しても `sort=favoritedDesc`
  で同じ並び。`md` 未満のまとめで「Date favorited」が 4 行目の右にある。
- **ゲスト**: ログアウトして同じライブラリ・フォルダ画面・再生画面を開くと、ハートがどこにも無く、
  絞り込みに「Favorites only」が無く、並べ替えが 7 種。所有者で `?fav=1&sort=favoritedDesc` を
  開いた URL をゲストで開くと、既定の一覧になり URL が直る（受け入れ条件 10）。
- **要求を満たしたことにならない例**（UI品質）: 付けるときに確認の窓やメニューが出る。カードに
  「お気に入り」の文字の行や帯が増えている。オフのハートが全カードに常に見え、格子が星取り表の
  ように見える。塗りのハートが赤で、再生できない警告と同じ系統に見える。再生画面のハートが
  題名の横や公開の切り替えの隣にあり、再生より先に目に入る。お気に入りの一覧がライブラリと
  別の画面になっている。ゲストの画面に線のハートや無効のチェックが残っている。

## Colour

新しく使う組は無い。塗りのハートは `link`（再生画面では `bg-accent-soft` の上、カードでは
`bg-navbar/90` の面の上、行では `surface` の上）、線のハートは `fg`（カードの面の上）と `fg-muted`
（行・再生画面）。`link` on `accent-soft`、`link` on `surface`、`fg` on `navbar`、`fg-muted` on `surface` は
`pairs` にある。`bg-navbar/90` は半透明で、上に載るのは文字でなく印なので、`pairs` の対象外である
（017「Accessibility」と同じ扱い）。新しいトークンは足さず、`pairs` にも足さない。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前と状態だけを決める（029・030・033 と同じ
範囲）。

- 付け外しは `button` の `aria-pressed` で、読み上げ名は「Words」の表のとおり（カードは題名・
  グループ名を含み、再生画面は「Favorite」）。送っている間は `aria-disabled`。
- 選択バーのメニューの項目は文言で区別できる。トーストは今のトーストの仕組み（`role="status"`）。
- 絞り込みのチェックの名前は「Favorites only」、並べ替えのラジオの名前は「Date favorited」。
  向きのボタンの読み上げ名は今の形。
