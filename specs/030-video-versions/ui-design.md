# UI Design: 同じ動画の別バージョンを束ねる

**Feature**: [parent Issue #572](https://github.com/syudead/vv/issues/572) ・
[plan.md](plan.md) ・ [contracts/screen-api.md](contracts/screen-api.md) ・
[research.md R-8](research.md#r-8-集まりは動画の-id-で指しバージョンの一覧束ねる代表外す候補の経路を足す)・
[R-9](research.md#r-9-束ねの変化は-domainvideobundlechanged-を発行し画面の-video-の知らせに写す)・
[R-11](research.md#r-11-集まりの再生位置がそのバージョンの尺以上なら画面が最初から再生する)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・一覧と再生画面の構成: [ライブラリ UI](../../docs/design-docs/library-ui.md)
  （「6. 一覧の構成」の選択バー、「8. 再生画面の構成」）
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`。値は写さず、名前で呼ぶ
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- 再生画面の列・「Video facts」の 2 行と右端の操作・失敗の 1 行・Esc の例外:
  [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) と今の
  [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx)
- 情報の行の項目に画像と操作を足した先例（サムネイルの項目）:
  [specs/029-video-overrides/ui-design.md「Thumbnail fact」](../029-video-overrides/ui-design.md#thumbnail-fact)
- 選択バーのポップオーバーと失敗の行、管理画面の窓（`ModalFrame`）と行の密度:
  [specs/014-video-tags/ui-design.md「Selection bar」「Tag management page」](../014-video-tags/ui-design.md#selection-bar)
- 選択バーのメニューと上限の扱い、ゲストに出さないものの扱い:
  [specs/016-single-account-auth/ui-design.md「Selection bar」「Guest degradation」](../016-single-account-auth/ui-design.md#selection-bar)
- 再生画面の従の行のメニュー、関連動画の列の行の密度、今の行の `aria-current`:
  [specs/017-folder-groups/ui-design.md「Group line」「Member list」](../017-folder-groups/ui-design.md#group-line)
- 設定画面の一覧の行（置き場所の省略の仕方、リンクの行）:
  [specs/024-import-progress/ui-design.md「Issue List」](../024-import-progress/ui-design.md#issue-list)
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足すのは次の 3 つで、どれも新しい色・半径・影のトークンを足さない。

1. 再生画面（`/videos/:id`）の情報の行の**バージョンの項目**と、開いたときの**バージョンの一覧**
   （要件 6・7・12、受け入れ条件 7・8・9・12）
2. ライブラリの選択バーの**「Bundle as versions」**と、代表を選ぶ**窓**（要件 11、受け入れ条件 6）
3. 所有者だけの**候補の画面**（`/duplicates`）と、サイドバーの入口（要件 9・10、受け入れ条件 3・4・5）

一覧のカード・リスト表示の行・フォルダのカード・グループのカード・関連動画の行は変えない。
代表以外のバージョンはどの一覧にも出ず（要件 5）、代表の 1 件は今の動画の 1 件と同じ見た目である
（UI品質「視覚的階層」「要求を満たしたことにならない変更」）。束ねたことをカードに印で出さない。

## Why here and not elsewhere

- バージョンの導線を**情報の行の項目**にし、プレイヤーの操作バー・題名の横・関連動画の列に
  置かないのは次の理由による。
  - 親 Issue は「詳細画面の情報の 1 つ」「ファイルの情報と並ぶ従の情報」と言い、UI品質は「ほかに
    バージョンがあることと、その数が 1 行で分かる程度」を求める。情報の行の項目は、長さ・サイズ・
    追加日と同じ段（`text-sm text-fg-muted`）に「3 versions」と並ぶだけで、この条件をそのまま
    満たす。029 のサムネイルの項目が、同じ行に状態と操作を置いた先例である。
  - 操作バーは 360px で 1 行に収める前提で、画質・字幕・速度のあと余地が無い（029「Why here」）。
    プレイヤーの中に置くと再生操作と同じ重さになる。
  - 関連動画の列は「次に見るもの」の列で、同じ動画の別ファイルは関連でも次でもない。
- 各バージョンを**押して開く浮き出し（`ui/Popover`）**に並べ、常に開いた欄やタブにしないのは、
  UI品質「情報密度」（違いは導線を開いたときに初めて並べる）と「要求を満たしたことにならない
  変更」（常に開いた大きな欄やタブ）による。比較対象の現行製品（メディアサーバーの項目画面の
  バージョンの切り替え）も、押すと開く一覧に収束している。
- 選択バーからの束ねを**窓（`ModalFrame`）**にし、ポップオーバーやメニューにしないのは、代表を
  選ぶ操作が「どちらのタグ・再生位置・公開の設定が集まりのものになるか」を決める、取り消しに
  手間の要る選択だからである。タグの統合（014「Merge and delete」）と同じく、選ぶ相手の一覧と
  結果の 1 文を同じ窓で読ませる。候補の画面の「Same video」も同じ窓を使い、代表の選び方を
  2 か所で別にしない。
- 候補の一覧を**サイドバーの所有者だけの入口 `/duplicates`** にし、設定画面の節や一覧の絞り込みに
  しないのは次の理由による。
  - 候補を確かめて束ねる作業は、タグの整理と同じ「ライブラリを整える」作業で、取り込みの設定
    ではない。設定画面は登録フォルダ・変換・API トークンの置き場で、既に長い。
  - 一覧の絞り込みは 1 本ずつを出す形で、「2 本を並べて違いを見る」ができない。
  - 写真・動画の管理の現行製品（重複の確認の画面）は、サイドバーの専用の入口に収束している。
  - サイドバーに件数の印は付けない。件数だけを返す経路が無く、シェルを開くたびに候補の全件を
    読むことになるため。件数は画面の見出しの行で分かる。

## Words

| 場所 | 英語（案） |
| --- | --- |
| 情報の行の項目（本数） | 3 versions |
| 項目の読み上げ名・`title` | 3 versions of this video. Show versions |
| バージョンの一覧の読み上げ名 | Versions |
| 一覧の中の代表の印 | Representative |
| 一覧の中の今の動画の印（視覚的に隠す） | Now playing |
| 一覧の各行のリンクの読み上げ名 | Play {題名}, {解像度} {コンテナ} {サイズ} |
| 行のメニューの引き金 | More actions for {題名} |
| メニューの項目 | Make representative / Remove from versions |
| 一覧の失敗の 1 行 | Couldn't change the versions: {理由} |
| 外したときのトースト | Removed "{題名}" from the versions |
| 選択バーの操作 | Bundle as versions |
| 窓の題名 | Bundle as versions |
| 窓の説明 | These {N} videos become versions of one video. Pick the one to show in the library. The library keeps that video's tags, position and visibility; the others' are set aside and come back if you remove them. |
| 窓の一覧の読み上げ名 | Representative |
| 窓の行の補足（既に束ねてある動画） | Already {N} versions — all of them join |
| 窓の主操作 | Bundle |
| 窓の失敗の 1 行 | Couldn't bundle: {理由} |
| 束ねたときのトースト | Bundled {N} videos as versions of "{題名}" |
| サイドバーの入口・`document.title` | Duplicates |
| 候補の画面の `h1` | Possible duplicates |
| 件数の行 | 12 pairs / 1 pair / Showing 200 of 340 pairs |
| 組の見出しの行 | Same length · similar frames |
| 「同じ動画」 | Same video… |
| 「違う動画」 | Different videos |
| 却下のトースト | Marked as different videos. They won't be suggested again |
| 組が消えていたときのトースト | This pair is no longer a candidate |
| 空の状態 | No possible duplicates / When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself. |
| 読み込み失敗 | Couldn't load the candidates / Retry |
| `too_few_videos` | Select at least two videos |
| `too_many_videos` | Too many videos selected（今の上限の文と同じ形） |
| `representative_not_selected` | Pick which video to show in the library |
| `not_bundled` | This video isn't bundled with others |

- 「version」の語は、動画ページ・選択バー・窓で 1 つの意味（同じ動画の別ファイル）に使う。候補の
  画面だけ「duplicates」を使うのは、そこに並ぶのはまだ束ねていない「重複かもしれない 2 本」で、
  利用者がそう呼ぶものだからである。決めたあとは同じ「versions」になる。
- 理由の 4 つは `web/src/i18n/errors.ts` の `reason` の表に足す（[contracts/screen-api.md §2〜§4](contracts/screen-api.md#2-post-apivideo-bundles)）。
  `video_not_found` は今の文のまま。

## Video page

### Versions fact

`Video.versions` があり `count` が 2 以上のとき、ファイルの情報の行（`VideoFacts` の 1 行目）の
長さ・サイズ・追加日の**あと**（所有者ではサムネイルの項目の前）に項目を 1 つ置く。所有者にも
ゲストにも出す（要件 12）。

- 中身は左から、lucide `Layers`（`size-4`、`text-fg-subtle`、`aria-hidden`）→ 「3 versions」
  （`tabular-nums`）→ `ChevronDown`（`size-3.5`）。項目の中は `gap-1.5`、他の項目との間は今の
  `gap-x-4`（`sm` 以上 `gap-x-5`）。
- 項目全体が `PopoverTrigger` の `button` で、文字は他の項目と同じ `text-sm text-fg-muted`、hover で
  `text-fg`。面も枠も持たず、アクセント色を使わない（UI品質「タイポグラフィ」「視覚的階層」）。
  `-mx-1 px-1 rounded-sm` で hover の `bg-hover-wash` を文字の周りだけに出す。読み上げ名は
  「3 versions of this video. Show versions」、`title` も同じ。
- `count` が 1 のとき（ほかのバージョンのファイルが全部消えている）は項目を出さない。切り替える
  相手が無く、「1 version」は意味を持たない。集まりと値は残り、ファイルが戻れば項目も戻る
  （Edge Case「代表のファイルがディスクから消えても」）。
- 情報の行の他の項目・右端の操作・技術情報の行・行の間隔（`gap-3`）は変えない（UI品質「余白の
  リズム」）。項目が増えた分は、他の項目と同じく折り返す。

### Versions list

項目を押すと、項目の下に `ui/Popover`（`side="bottom"`、`align="start"`）を開く。中は上から次のとおり。

- 幅は `w-[min(28rem,calc(100vw-2rem))]`。面は今の `PopoverContent` のまま（`bg-elevated`、
  `shadow-elevated`）。
- 一覧は `ul`（読み上げ名「Versions」）で、`divide-y divide-border` の行の並び。行の順は応答の順
  （代表が先頭、続きは題名の自然順。[contracts §1](contracts/screen-api.md#1-get-apivideosidversions)）。
  行は `py-2`、6 本を超えたら `max-h-80 overflow-y-auto` で中だけをスクロールさせる。
- 各行は `grid grid-cols-[1fr_auto] gap-x-2` で、左に 2 行の文字、右に所有者だけの操作を置く。
  - 1 行目: 題名（`text-sm text-fg`、1 行で省略、`title` に全体）。代表の行は題名の右に
    `text-xs text-fg-muted` の「Representative」を添える（`shrink-0`）。色や太さで区別せず、
    文字で示す。
  - 2 行目: 違い（`text-xs text-fg-muted tabular-nums`、1 行で省略）。解像度 → コンテナ → 映像
    コーデック → サイズ → 置き場所 の順で、項目の間は `text-fg-subtle` の「·」。分からない
    値（解析前）は項目ごと省く。置き場所は「登録フォルダの表示名 / 相対パス」を `folder` から
    作り、末尾（ファイルに近い側）を優先して残す（024「Issue List」と同じ省略）。所有者では行の
    `title` に絶対パス（`location.path`）を入れ、ゲストでは相対の置き場所だけにする
    （016「Guest degradation」）。
  - 左の 2 行全体が**再生への `Link`**（`/videos/{id}`）で、読み上げ名は「Play {題名}, {違い}」。
    hover で行に `bg-hover-wash`。`state.from` は今の画面の `backTo` をそのまま渡し、戻り先を
    変えない。再生中（`status.playing || status.ended`）に選んだときは `autoplay` を付け、移った
    先で再生を続ける（前後のつまみと同じ）。
  - **今の動画の行**: リンクにせず `aria-current="true"`、面を `bg-active-wash`、左端に
    `border-l-2 border-accent`（017「Member list」の今のメンバーと同じ）。視覚的に隠した
    「Now playing」を題名の前に置く。押せないので hover の面も変えない。
  - **所有者の操作**（右の列）: `IconButton`（ghost・`sm`、lucide `Ellipsis`、`text-fg-muted`、
    hover で `text-fg`）が `ui/Menu` を開く（読み上げ名「More actions for {題名}」）。項目は上から
    「Make representative」（lucide `Star`。代表の行では出さない）、区切り線、「Remove from
    versions」（lucide `Unlink`）。再生は行そのもの、代表の変更はメニューの先頭、外すのは区切りの
    下の最後、という順で UI品質「操作の優先順位」（再生 → 代表 → 外す）を表す。外すのを
    `text-danger` にしない。取り消せる操作（また束ねられる）で、削除ではない。
  - **ゲスト**は右の列が無く、行はリンクだけである（要件 12）。
- 一覧の下に失敗の 1 行を置く場所を持つ（下の「Failure」）。
- 開いたときの最初のフォーカスは、今の動画以外の最初の行のリンク（無ければ最初の操作）。
  Tab は行の順に、行のリンク → その行の `Ellipsis` → 次の行 と進む。
- 開いている間の Esc は浮き出しを閉じるだけにし、画面を閉じない。フォーカスは項目に戻る。
  012「Interaction details」の Esc の例外（速度のメニュー・吹き出し・`role="menu"`）に、
  この `role="dialog"` の浮き出しを足す。行のメニューが開いているときの Esc はメニューだけを
  閉じる（`ui/Menu` の既定）。
- 中身は開くたびに `GET /api/videos/{id}/versions` で取る。取るまでは行の場所に `Skeleton`
  （`h-10`）を `count` 本。取れなければ中に `text-xs text-danger` の 1 行（`role="alert"`）
  「Couldn't load the versions」と ghost・`sm` の「Retry」。`video` の知らせで動画を取り直したとき
  （`useVideoDetail`）、浮き出しが開いていれば一覧も取り直す。閉じているときは次に開くときに取る。

### Make representative

- メニューの「Make representative」を押すと `POST /api/videos/{id}/make-representative` を送る。
  送っている間はその行の `Ellipsis` を `LoaderCircle`（`animate-spin`、`aria-disabled`）にし、
  ほかの行の操作は押せるままにする。
- `200` を受けたら応答の `VideoVersions` で一覧を差し替える。「Representative」の印がその行へ
  移り、行の順が変わる（代表が先頭）。浮き出しは開いたまま、トーストは出さない。印が移ることが
  結果である。あわせて動画を取り直す（`versions.representativeId` が変わる）。
- 一覧の 1 件の題名とサムネイルが新しい代表のものになる（要件 7）のは、`video` の知らせを受けた
  一覧の取り直し（`useItemRefresh`）で起きる。この画面では伝えない。ライブラリの控えは捨てない。
  戻ったときの一覧は、控えの上に届いた知らせで差し替わっている。

### Unbundle

- メニューの「Remove from versions」を押すと `POST /api/videos/{id}/unbundle` を送る。確認の窓は
  出さない。また束ねればもとに戻り、外した動画は束ねる前の値（要件 3）に戻るだけで、失うものが
  無い。送っている間の見え方は代表の変更と同じ。
- `200` を受けたら:
  - 外したのが**別の行**なら、その行を一覧から消し、項目の本数を減らす。トースト「Removed
    "{題名}" from the versions」。残りが 1 本になったら集まりは解けている（契約 §4）ので、浮き出しを
    閉じ、動画を取り直し、項目が消える。
  - 外したのが**今の動画**なら、浮き出しを閉じ、応答の `Video` で動画を差し替える（`versions` が
    無くなり、`tags`・`progress`・`public` が自分の値になる）。項目が消え、タグの並びと公開の
    切り替えが自分の値で描き直される。同じトースト。
- フォーカスは、浮き出しが閉じたときは項目のあった場所に最も近い要素（技術情報の行の前の操作
  か、右端の操作の先頭）へ、開いたままのときは次の行のリンク（無ければ前の行）へ移す。

### Failure

- 代表の変更・外すのどちらも、失敗したら一覧の下に `text-xs text-danger` の 1 行（`role="alert"`、
  先頭に `AlertCircle` `size-4`）「Couldn't change the versions: {理由}」を出す。理由は `errorText`
  （上の「Words」）。浮き出しと一覧は開いたまま残す。次に操作したとき、または閉じたときに消える。
- `404 video_not_found`・`400 not_bundled`（別のタブで先に変わった）: 行を出したうえで一覧と動画を
  取り直す。
- 情報の行の下の失敗の 1 行（012・029 の場所）は使わない。浮き出しの中で起きたことは浮き出しの
  中で伝える。

### Resume position

- 集まりの `progress.positionMs` がそのバージョンの `durationMs` 以上のときは 0 から再生する
  （R-11、Edge Case「バージョンの長さが違う」）。判断は `pageDecisions.resumePosition` に足す。
  プレイヤーの中に「最初から再生します」の文言は出さない。位置の数字は操作バーが示す。
- 代表以外のバージョンを再生しても、再生位置の保存は今のまま `PUT /api/videos/{id}/progress` で、
  集まりの位置が進む（要件 2、受け入れ条件 7）。画面は鍵を選ばない（R-2）。

### Group line と versions の関係

グループのメンバーであり集まりのメンバーでもある動画は、題名の上のグループ名の行と、情報の
行のバージョンの項目の両方を持つ。2 つは別の事実（置き場所のまとまりと、同じ動画の別ファイル）で、
1 つにまとめない。「続けて再生」の並びには代表だけが出る（要件 5、data-model.md §4）。

## Selection bar

### Bundle action

- 「公開」の直後、縦線の前に「Bundle as versions」（lucide `Layers` + 文言、`Button` の ghost・`sm`）を
  置く。`sm` 未満の下の段では、タグの 2 つ・「公開」に続く 4 つ目になり、今の規則（3 つが収まらない
  幅で右端に回る）をそのまま受ける: 4 つが 1 行に収まる幅では等分、収まらなければ「公開」と
  「Bundle as versions」が次の段の右端に回る。`SelectionBar` の容器の問い合わせの幅を 4 つ分に
  直す。
- 選んだ本数が **2 本未満のときは出さない**（親 Issue の要件 11、plan の受け入れ条件「1 本の選択では
  出ない」）。`disabled` にして理由を添える案は、選択の大半（1 本）で常に灰色の操作が見えることに
  なるので採らない。2 本目を選んだときに現れる。
- 上限（タグの操作と同じ `maxVideoTagsSelection`）を超える選択では、タグの操作と同じく `disabled`
  にし同じ理由を添える（library-ui.md §6）。
- 押すと下の「Bundle dialog」を開く。ポップオーバーではなく窓なのは「Why here」のとおり。

### Bundle dialog

`ModalFrame` の窓「Bundle as versions」。中は上から次のとおり。

- 説明の 1 段落（`text-sm text-fg-muted`、上の「Words」の「窓の説明」）。何が起きるかを選ぶ前に
  読ませる。
- 代表の一覧（`radiogroup`、読み上げ名「Representative」）。`divide-y divide-border` の行の並びで、
  枠やカードで囲わない。各行は `label` で、左に `input[type=radio]`（`size-4`、`accent-accent`。
  `Checkbox` と同じ大きさと色の扱い）、右に 2〜3 行の文字。
  - 1 行目: 題名（`text-sm text-fg`、1 行で省略、`title` に全体）。
  - 2 行目: 違い（「Versions list」の 2 行目と同じ書式と順）。
  - 3 行目（あるときだけ）: `text-xs text-fg-muted` で、手で付けたタグの名前を「·」でつないだ
    もの（`tags` のうちフォルダ名からだけ付いたものを除く）。どのタグが集まりのものになり、
    どれが脇に置かれるかを、選ぶ前に見せるためである。既に集まりに属する動画（`versions` が
    ある）は、この行の先頭に「Already 3 versions — all of them join」を置く（契約 §2）。
  - 行は `py-2`。6 本を超えたら `max-h-80 overflow-y-auto` で中だけをスクロールさせる。窓の
    残りの高さは `ModalFrame` の規則に従う。
  - 行の並びは、選択バーからは選んだ順（`selectedIds` の順）、候補の画面からは組の順（id の昇順）。
- 一覧の中身は `GET /api/videos/{id}` を選んだ id ごとに取る（選択バーからは一覧の項目を
  持たない id もあるため。候補の画面からは組の 2 本の `Video` をそのまま渡し、取らない）。取るまでは
  行の場所に `Skeleton`（`h-12`）。1 本でも取れなければ（404 を含む）、一覧の場所に `text-sm
  text-danger` の 1 行（`role="alert"`）「Couldn't load the selected videos」と ghost・`sm` の
  「Retry」を出し、「Bundle」を `disabled` にする。
- 操作の行（`border-t border-border`、右寄せ、`gap-2`）: secondary「Cancel」（最初のフォーカス）
  と primary「Bundle」。**代表を選ぶまで「Bundle」は `disabled`**。既定で先頭を選んでおく案は、
  読まずに押した 1 回でどちらの値が残るかが決まってしまうので採らない（014「Merge and delete」と
  同じ扱い）。
- 「Bundle」を押すと `POST /api/video-bundles`（`videoIds` は選んだ id、`representativeId` は選んだ
  代表）を送る。送っている間は両方のボタンを `disabled` にし、「Bundle」に `LoaderCircle`
  （設定画面のフォルダの削除の確認と同じ）。
- `200` を受けたら窓を閉じ、トースト「Bundled {N} videos as versions of "{題名}"」（N は応答の
  `items.length`、題名は代表の）。
  - 選択バーから: 選択を解除し、ライブラリの一覧を取り直す（代表以外の項目が一覧から消える。
    受け入れ条件 6）。取り直しの間、格子の位置は保つ。フォーカスは「すべて選択」のあった場所へ
    移す（バーは消えているので、格子の最初のカード）。
  - 候補の画面から: 下の「Deciding」。
- 失敗したときは窓を開いたまま、操作の行の上に `text-sm text-danger` の 1 行（`role="alert"`）
  「Couldn't bundle: {理由}」を出す。選んだ代表は残す。`404 video_not_found`（選んだ中に消えた
  動画がある）は、行を出したうえで一覧を取り直す。
- Esc と「Cancel」は何も送らずに閉じ、フォーカスは引き金（「Bundle as versions」、または候補の
  画面の「Same video…」）に戻る。

### Card

一覧のカード・リスト表示の行は変えない。束ねたあとの代表の 1 件は、束ねる前と同じ見た目である
（UI品質「一覧の 1 件は、今の動画の 1 件と同じ見た目にする」）。

## Duplicates page

### Entry

- サイドバーの上段の「タグ」の直後に「Duplicates」（lucide `Layers`、`/duplicates`、所有者だけ。
  `navEntries` の `ownerOnly`）を足す。展開・レール・ドロワーの見え方は他の項目と同じ。件数の印は
  付けない（「Why here」）。
- 経路は `/tags` と同じ形（`AppShell` の中、使うときだけ読み込む）。ゲストがこの URL を開いた
  ときの扱いは `/tags` と同じ（所有者だけの応答の 401 はゲートが扱う。016「Gate」）。
- `document.title` は「Duplicates」。

### Layout

- タグ管理画面と同じ本文の幅と余白（`mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-8`）。上から
  `h1`「Possible duplicates」（`text-xl font-semibold`）、件数の行、組の一覧。トップバーには何も
  置かない。検索や絞り込みは置かない（候補は多くて 200 組で、決めれば減る）。
- 件数の行は `text-xs text-fg-muted tabular-nums`、`role="status"`・`aria-live="polite"`。
  「12 pairs」。`total` が 200 を超えるときは「Showing 200 of 340 pairs」。
- 組の一覧は `ul`、`divide-y divide-border` の行の並び。枠やカードで囲わない。1 組は `py-4`
  （タグの行より広い。中に 2 本の動画が入るため）。

### Pair

1 組（`li`）は上から次の 2 段である。

1. **見出しの行**（`flex items-center justify-between gap-3`）: 左に `text-xs text-fg-muted` の
   「Same length · similar frames」（なぜ候補なのかを 1 行で言う。`distance` の数字は出さない。
   閾値の内側という事実しか意味を持たず、利用者が比べられる値ではない）。右に操作を 2 つ
   （`gap-2`）: `Button` の secondary・`sm`「Different videos」と primary・`sm`「Same video…」
   （lucide `Layers`）。DOM の順もこの順で、組の中で最初に Tab が届くのは「Different videos」、
   次が「Same video…」である。判断が先、動画の中身は後（UI品質「候補一覧では「同じ動画」
   「違う動画」の判断を先に置く」）。「Same video…」を primary にするのは、この画面に来た人が
   主にする判断が「同じ」で、「違う」は例外の記録だからである。
2. **2 本の並び**（`mt-3 grid gap-3 sm:grid-cols-2`）: 組の `videos` を id の昇順で左・右（`sm`
   未満は上・下）に置く。各 1 本は次のとおり（関連動画の行と同じ密度、017「Member list」）。
   - `flex gap-3`。左にサムネイル（`w-40 shrink-0`、`aspect-video`、`rounded-md`、`bg-surface`、
     無ければ `ImageOff` の箱。右下に長さの札。関連動画の行と同じ部品）。
   - 右に 3 行: 題名（`text-sm font-medium text-fg`、2 行で省略、`title` に全体）、違い
     （「Versions list」の 2 行目と同じ書式。ただし置き場所は次の行へ分ける）、置き場所（`text-xs
     text-fg-muted`、末尾を優先して 1 行で省略、`title` に絶対パス）。
   - 1 本全体は `/videos/{id}` への `Link`（`state.from` は `/duplicates`）。hover で `bg-hover-wash`
     （行の `-m-1.5 p-1.5` の外形）。読み上げ名は「{題名} {長さ}」（関連動画の行と同じ）。
     見比べたいときに再生画面を開いて戻れる。戻り先は候補の画面で、決めていない組はそのまま
     残っている。
   - 2 本のサムネイルは同じ大きさで、どちらかを強調しない。違いは 2 行目の数字で読む。
     解像度・サイズが大きい方に印を付ける案は採らない。「良い方」を機械が示すと、代表の
     選択を誘導する。
- 組の見出しの行と 2 本の並びの間は `gap-3`、組と組の間は `divide-y` と `py-4`。組の中に線や
  枠は無い。

### Deciding

- **「Same video…」**: 「Bundle dialog」を開く。一覧は組の 2 本（応答の `Video` をそのまま。
  取り直さない）。「Bundle」で `200` を受けたら窓を閉じ、その組を一覧から消し、件数を減らし、
  トースト（「Bundle dialog」と同じ文言）を出す。一覧の取り直しは `scan` の知らせに任せる。
  フォーカスは次の組の「Different videos」へ、無ければ前の組、1 組も無ければ `h1` へ移す。
- **「Different videos」**: 確認の窓を出さず、`POST /api/version-candidates/dismiss` を送る。
  送っている間はその組の 2 つのボタンを `disabled` にし、押した方に `LoaderCircle`。`204` で
  組を消し、件数を減らし、トースト「Marked as different videos. They won't be suggested again」。
  取り消せない（要件 10）が、結果は「候補に出なくなる」だけで、手動で束ねる道（選択バー）は
  残るので、確認の窓を挟まない。トーストの文で「二度と出ない」ことを言う。フォーカスの移し方は
  「Same video…」と同じ。
- **消えていた組**（`404 video_not_found`。確かめている間に片方がスキャンで消えた）: トースト
  「This pair is no longer a candidate」を出し、一覧を取り直す（Edge Case）。
- そのほかの失敗: トースト「Couldn't bundle: {理由}」（窓の中なら窓の 1 行）または
  `errorText` の文。組は残す。

### Refresh

- 開いたときに `GET /api/version-candidates` を読む。所有者のシェルが受ける `scan` の知らせで取り
  直す（`fingerprint` の job の成否で流れる。契約 §6）。取り直しの間、一覧の見た目は変えない
  （骨組みにしない）。応答が届いたら差し替え、スクロールの位置は保つ。新しい組が先頭に足され、
  片方が消えた組は消える（取り込み中に候補が増える・減ることに耐える）。
- 開いている窓の組が取り直しで消えていても、窓は閉じない。「Bundle」の 404 で伝える。

### States

| 状態 | 見え方 |
| --- | --- |
| 読み込み中 | 件数の行は「Loading…」、一覧の場所に `Skeleton`（`h-24`）を 3 組 |
| 読み込み失敗 | `EmptyState`（danger・`AlertCircle`）「Couldn't load the candidates」、`Button`「Retry」 |
| 候補が 1 組も無い | `EmptyState`（lucide `Layers`）「No possible duplicates」、説明「When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself.」。ボタンは置かない。取り込みの入口は上部バーの「更新」にある |
| 決めて 0 組になった | 同じ空の状態に切り替わる。件数の行は「0 pairs」のまま残さず、空の状態の見出しが件数の代わりになる |
| 操作の失敗 | 上の「Deciding」 |

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md 4）。判定する幅は
360px・768px・1280px（再生画面は `lg` の 2 列）。

| 幅 | バージョンの項目と一覧 | 選択バー | 窓 | 候補の画面 |
| --- | --- | --- | --- | --- |
| 1280px | 長さ・サイズ・追加日・「3 versions」が 1 行。浮き出しは `28rem` で項目の左端にそろう | 「Bundle as versions」がタグの 2 つ・「公開」に続く 4 つ目で 1 行 | 幅 `max-w-lg`、一覧は 6 本まで伸び、それ以上は中でスクロール | 2 本が左右に並び、サムネイルは `w-40`。見出しの行の右に 2 つのボタン |
| 768px | 同上 | 同上（`sm` 以上の 1 行） | 同上 | 同上 |
| 360px | 項目は他の項目と同じく折り返す。浮き出しは画面幅から `2rem` を引いた幅で、行の 2 行目は省略される。横スクロールは出ない | 下の段に 4 つが収まらず、「公開」と「Bundle as versions」が次の段の右端に回る（今の規則） | 画面幅いっぱい（`ModalFrame` の既定）。行の 2 行目は省略 | 2 本が上下に積まれ、それぞれサムネイルと 3 行の文字は横に並んだまま。見出しの行のボタンは折り返して右寄せのまま |

- 浮き出しの行の 2 行目は、どの幅でも 1 行で省略し、全体は行の `title` で読める。
- 候補の画面の置き場所の行は、どの幅でも末尾（ファイル名に近い側）を残す。

## Review criteria

判定は実機で見て行う（library-ui.md 5）。「ある」だけでは満たさない（Q-4）。

- **視覚的階層（再生画面）**: 束ねた動画のページを開いて、プレイヤー → 題名 → タグ の順に目が
  行き、「3 versions」は長さ・サイズ・追加日と同じ強さで、それらより先に読まれない。束ねていない
  動画のページと並べたとき、題名・タグ・情報の 2 行の位置と間隔に違いが無く、違いは項目が 1 つ
  多いことだけ（UI品質「視覚的階層」「余白のリズム」）。
- **視覚的階層（一覧）**: 束ねた代表のカードと、束ねていない動画のカードを並べたとき、見分けが
  付かない。カードに印・数字・帯が増えていない（受け入れ条件 6、UI品質「要求を満たしたことに
  ならない変更」）。リスト表示の行も同じ。
- **情報密度（再生画面）**: 浮き出しを開く前に見えるバージョンの情報は「3 versions」の 1 項目だけで、
  各バージョンの解像度・サイズ・場所はどこにも出ていない。開くと、各行の 2 行目で 2 本の違い
  （解像度・コーデック・サイズ・場所）が数字で読め、行の間に説明の文や見出しが無い（UI品質
  「情報密度」）。
- **タイポグラフィ**: 項目の文字と浮き出しの行の題名は、情報の行と同じ `text-sm`。行の 2 行目は
  技術情報の行と同じ `text-xs` の従の色。太字・大きな見出し・アクセント色の文字が、浮き出しの
  中に無い（UI品質「タイポグラフィ」）。「Representative」は文字の印で、色や太さで代表を示して
  いない。
- **操作の優先順位（再生画面）**: 浮き出しを開いてから、行を 1 回押すだけで別のバージョンの
  ファイルが再生され（受け入れ条件 7）、再生中に選んだときは移った先で再生が続く。代表の変更は
  `Ellipsis` → 先頭の項目の 2 回、外すのは `Ellipsis` → 区切りの下の 2 回で、行の面には出ていない。
  最初に触れるのは今までどおり再生で、浮き出しを開かない限り画面は変わらない。
- **代表の変更と外すことの結果**: 代表を B に替えると、浮き出しの「Representative」が B の行へ
  移り、ライブラリへ戻ると 1 件の題名とサムネイルが B のもので、タグは X のまま（受け入れ条件 8）。
  B を外すと B のページのタグが Y に戻り、ライブラリに B が独立した 1 件として現れ、A の集まりの
  タグは X のまま（受け入れ条件 9）。どちらも、ページを離れずに結果が浮き出しの中で見える。
- **再開の位置**: 集まりの再生位置がそのバージョンの尺以上のとき、そのバージョンが 0 から
  始まる。操作バーの位置が 0 で、文言や層は出ない。
- **選択バー**: 1 本を選んだバーに「Bundle as versions」が無く、2 本目を選ぶと現れる。押した窓で
  代表を選ぶまで「Bundle」が押せず、選んで押すと一覧に代表の 1 件だけが残り、選択が解けている
  （受け入れ条件 6）。360px で下の段が崩れず、横スクロールが出ない。
- **窓**: 説明の 1 段落 → 行の並び → 操作の順で、行に枠が無く、各行の 3 行目でどのタグが残るかが
  選ぶ前に分かる。既に束ねてある動画の行に「Already N versions — all of them join」が出る。
- **候補の画面（階層と密度）**: 1 組を見て、まず右上の 2 つのボタンに目が行き、次に 2 本の
  サムネイルと題名、最後に違いの数字と置き場所が読める。2 本のサムネイルは同じ大きさで、
  どちらかが強調されていない。組の中に線・枠・カードが無く、組と組は線 1 本で分かれる。
  360px で 2 本が上下に積まれ、はみ出しも重なりも無い。
- **候補の画面（判断）**: 「Same video…」→ 代表を選ぶ → 「Bundle」で組が消え、ライブラリに
  1 件だけが出る（受け入れ条件 3・6）。「Different videos」で組が消え、再スキャン後もその組が
  出ない（受け入れ条件 5）。取り込み中に新しい組が先頭に足されても、読んでいた組の位置が
  動かない。
- **状態の見え方**: 送っている間は押したボタンか `Ellipsis` に回転の印が出て、押し直しても二重に
  送られない。失敗は赤い 1 行がその場（浮き出しの下、窓の操作の上）に出て、開いたものは閉じない。
  候補の画面の失敗だけがトーストである（行が消えているか、消すべきかを 1 行で言えるため）。
- **キーボード**: 再生画面で Tab が項目に届き、Enter で開くと最初の別のバージョンの行に
  フォーカスがあり、Esc で閉じて項目に戻り、もう一度 Esc で画面が閉じる。候補の画面では、Tab が
  組ごとに「Different videos」→「Same video…」→ 左の動画 → 右の動画 の順に進む。
- **ゲスト**: ログアウトして束ねた動画を開くと、「3 versions」の項目と浮き出しがあり、行を押すと
  B が再生できる（受け入れ条件 12）。行に `Ellipsis` が無い。サイドバーに「Duplicates」が無く、
  選択バーそのものが無い。
- **要求を満たしたことにならない例**（UI品質）: 一覧に代表以外のバージョンが出る。再生画面に
  バージョンの一覧が常に開いた欄やタブとして出ている。「3 versions」が題名の隣やプレイヤーの
  中にあり、タグより先に目に入る。束ねたカードに印や本数が出て、ほかのカードと見た目が
  そろわない。候補の画面で、2 本のどちらかが「おすすめ」として先に強調されている。

## Colour

新しく使う組は無い。浮き出しと窓の文字は `fg`・`fg-muted` on `elevated`、失敗の行は `danger` on
`elevated`（014 の「Accessibility」で足した組）、候補の画面の文字は `fg`・`fg-muted` on `bg`、
トーストは今のまま。`fg-subtle` は印（`Layers`・「·」）だけに使い、文字には使わない。
`bg-hover-wash`・`bg-active-wash`・`border-accent` は文字ではなく、`pairs` の対象外である。
新しいトークンは足さない。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前だけを決める（012・029 と同じ範囲）。

- 項目の読み上げ名: 「3 versions of this video. Show versions」。浮き出しは `role="dialog"` で
  読み上げ名「Versions」。
- 行のリンク: 「Play {題名}, {違い}」。今の動画の行は `aria-current="true"` と隠した「Now
  playing」。代表の印「Representative」は見える文字で、隠さない。
- 行のメニューの引き金: 「More actions for {題名}」。
- 窓の一覧: `radiogroup`、読み上げ名「Representative」。各 `label` は題名と違いを含む。
- 候補の画面: 各組は `li` で、見出しの行の「Same length · similar frames」を組の名前にはしない
  （全組で同じ文になる）。2 本のリンクの読み上げ名は「{題名} {長さ}」。ボタンの読み上げ名は
  見える文言に「for {題名 A} and {題名 B}」を続ける（「Same video, for A and B」）。
- 失敗の行は `role="alert"`。回転の印は `aria-hidden`、送信中の要素は `aria-disabled` か `disabled`。
