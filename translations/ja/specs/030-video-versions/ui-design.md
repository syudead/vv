---
source: specs/030-video-versions/ui-design.md
sourceHash: cde3704756ca140b6ef4b87b3e496c46a95041bdb9d65eb5aae6340ddaac3d7c
---

# UI 設計: 同じ動画のバージョンをまとめる {#ui-design-bundle-versions-of-the-same-video}

**機能**: [親 Issue #572](https://github.com/syudead/vv/issues/572) · [plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) · [research.md R-8](research.md#r-8-bundles-are-addressed-by-video-id-new-routes-for-listing-versions-bundling-the-representative-removal-and-candidates) · [R-9](research.md#r-9-a-bundle-change-publishes-domainvideobundlechanged-mapped-to-the-screens-video-notification) · [R-11](research.md#r-11-when-the-bundles-playback-position-is-at-or-past-that-versions-duration-the-screen-plays-from-the-beginning)

出典: 視覚的な規則は次の文書から来ており、ここでは決め直さない。

- 色、操作の状態、幅のブレークポイント、ライブラリとプレーヤーのレイアウト: [Library UI](../../docs/design-docs/library-ui.md)（[List layout](../../docs/design-docs/library-ui.md#list-layout) の選択バー、ライブラリのレイアウト、[Video page layout](../../docs/design-docs/library-ui.md#video-page-layout) のプレーヤー画面のレイアウト）
- 役割のトークン: [`web/src/index.css`](../../web/src/index.css) の `@theme`。名前で参照し、値を写さない
- コントラストを確かめる組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- プレーヤー画面の列、2 行の「Video facts」とその右側の操作、1 行の失敗の行、Esc の例外: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) と現在の [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx)
- 情報の行の項目に画像と操作を加えた前例（サムネイルの項目）: [specs/029-video-overrides/ui-design.md「Thumbnail fact」](../029-video-overrides/ui-design.md#thumbnail-fact)
- 選択バーのポップオーバーと失敗の行、管理用のダイアログ（`ModalFrame`）、行の密度: [specs/014-video-tags/ui-design.md「Selection bar」と「Tag management page」](../014-video-tags/ui-design.md#selection-bar)
- 選択バーのメニューと上限、ゲストから隠すもの: [specs/016-single-account-auth/ui-design.md「Selection bar」と「Guest degradation」](../016-single-account-auth/ui-design.md#selection-bar)
- プレーヤーの 2 行目のメニュー、関連動画の列の行の密度、現在の行の `aria-current`: [specs/017-folder-groups/ui-design.md「Group line」と「Member list」](../017-folder-groups/ui-design.md#group-line)
- 設定画面の一覧の行（場所の縮め方、リンクの行）: [specs/024-import-progress/ui-design.md「Issue List」](../024-import-progress/ui-design.md#issue-list)
- 画面の文言の置き場所とその書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。この文書の英語は意図を示す。実装の後は、カタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正である

この機能は画面に 3 つのものを加え、どれも色、角丸、影のトークンを加えない。

1. プレーヤー画面（`/videos/:id`）で、情報の行の**バージョンの項目**と、開いたときの**バージョンの一覧**（要件 6、7、12。受け入れ条件 7、8、9、12）
2. ライブラリの選択バーの **"Bundle as versions"** と、代表を選ぶ**ダイアログ**（要件 11。受け入れ条件 6）
3. 所有者だけの**候補の画面**（`/duplicates`）と、そのサイドバーの項目（要件 9、10。受け入れ条件 3、4、5）

ライブラリのカード、リスト表示の行、フォルダのカード、グループのカード、関連動画の行は変わらない。代表でないバージョンはどの一覧にも現れず（要件 5）、代表の項目は今日のどの動画の項目とも同じ見た目である（`UI品質` の「視覚的な階層」と「要件を満たさない変更」）。まとめたことはカードに印を付けない。

## この形にした理由 {#why-this-shape}

バージョンの入口は、プレーヤーの操作バー、タイトルの隣、関連動画の列ではなく、**情報の行の項目**である。

- 親 Issue はこれを「詳細画面の情報の 1 つ」「ファイルの詳細と並ぶ従の情報」と呼び、`UI品質` は「ほかのバージョンがあることと、その数が 1 行で分かる程度」を求めている。情報の行の項目は、長さ、サイズ、追加日と同じ段（`text-sm text-fg-muted`）に「3 versions」として並び、そのままでこれを満たす。029 のサムネイルの項目は、状態と操作を同じ行に置いた前例である。
- 操作バーは 360px で 1 行に収まらなければならず、画質、字幕、速度の後に余地はない（029「Why here」）。プレーヤーの中では、再生の操作と同じ重みを持ってしまう。
- 関連動画の列は「次に見るもの」である。同じ動画の別のファイルは、関連でも次でもない。

バージョンは、常に開いたパネルやタブではなく、**押して開くポップオーバー（`ui/Popover`）**に並べる。理由は `UI品質` の「情報の密度」（違いは入口を開いてから初めて並べる）と「要件を満たさない変更」（常に開いた大きなパネルやタブ）である。比べられる現在の製品（メディアサーバーの項目画面でのバージョンの切り替え）も、押して開く一覧に落ち着いている。

選択バーからまとめる操作は、ポップオーバーやメニューではなく**ダイアログ（`ModalFrame`）**である。代表を選ぶことは「誰のタグ、再生位置、公開状態がまとまりのものになるか」を決め、元に戻すのに手間がかかる選択だからである。タグの統合（014「Merge and delete」）と同じく、ダイアログは選ぶ一覧と 1 文の結果を一緒に示す。候補の画面の「Same video」も同じダイアログを使うので、代表を 2 通りの方法で選ぶことはない。

候補の一覧は、設定画面の節やライブラリの絞り込みではなく、**所有者だけのサイドバーの項目 `/duplicates`** である。

- 候補を確かめてまとめることは、タグの整理と同じく「ライブラリの片付け」であり、取り込みの設定ではない。設定画面はメディアフォルダ、変換、API トークンを持ち、すでに長い。
- ライブラリの絞り込みは 1 つの項目に 1 つの動画を示し、「2 つを並べて違いを見る」ことができない。
- 現在の写真と動画の管理製品（重複を確認する画面）は、専用のサイドバーの項目に落ち着いている。
- サイドバーは件数のバッジを示さない。件数だけを返すルートはなく、シェルは開くたびにすべての候補を読むことになる。件数は画面の見出しの行で見える。

## 文言 {#words}

| 場所 | 文言（案） |
| --- | --- |
| 情報の行の項目（件数） | 3 versions |
| 項目のアクセシブルな名前と `title` | 3 versions of this video. Show versions |
| バージョンの一覧のアクセシブルな名前 | Versions |
| 一覧の代表の印 | Representative |
| 一覧の現在の動画の印（見た目では隠す） | Now playing |
| 各行のリンクのアクセシブルな名前 | Play {title}, {resolution} {container} {size} |
| 行のメニューを開くボタン | More actions for {title} |
| メニューの項目 | Make representative / Remove from versions |
| 一覧の失敗の行 | Couldn't change the versions: {reason} |
| 外したときのトースト | Removed "{title}" from the versions |
| 選択バーの操作 | Bundle as versions |
| ダイアログのタイトル | Bundle as versions |
| ダイアログの説明 | These {N} videos become versions of one video. Pick the one to show in the library. The library keeps that video's tags, position and visibility; the others' are set aside and come back if you remove them. |
| ダイアログの一覧のアクセシブルな名前 | Representative |
| ダイアログの行の注記（すでにまとめた動画） | Already {N} versions — all of them join |
| ダイアログの主な操作 | Bundle |
| ダイアログの失敗の行 | Couldn't bundle: {reason} |
| まとめたときのトースト | Bundled {N} videos as versions of "{title}" |
| サイドバーの項目と `document.title` | Duplicates |
| 候補の画面の `h1` | Possible duplicates |
| 件数の行 | 12 pairs / 1 pair / Showing 200 of 340 pairs |
| ペアの見出しの行 | Same length · similar frames |
| 「同じ動画」 | Same video… |
| 「別の動画」 | Different videos |
| 退けたときのトースト | Marked as different videos. They won't be suggested again |
| ペアがなくなったときのトースト | This pair is no longer a candidate |
| 空の状態 | No possible duplicates / When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself. |
| 読み込みの失敗 | Couldn't load the candidates / Retry |
| `too_few_videos` | Select at least two videos |
| `too_many_videos` | Too many videos selected（現在の上限の文言と同じ形） |
| 選択バーの操作を無効にする理由（`maxVideoTagsSelection` ではなく `maxBundleSelection` を超えたとき） | Bundle up to 20 videos at a time |
| `representative_not_selected` | Pick which video to show in the library |
| `not_bundled` | This video isn't bundled with others |

- 動画ページ、選択バー、ダイアログでは、「Version」は 1 つの意味（同じ動画の別のファイル）を持つ。「duplicates」を使うのは候補の画面だけである。そこに並ぶのはまだまとめていない 2 つの動画で、「重複かもしれない」ものであり、利用者もそう呼ぶ。決めた後は、それらも「versions」になる。
- 4 つの理由は `web/src/i18n/errors.ts` の `reason` の表に加える（[contracts/screen-api.md、`POST /api/video-bundles`](contracts/screen-api.md#post-apivideo-bundles) から [`POST /api/videos/{id}/unbundle`](contracts/screen-api.md#post-apivideosidunbundle) まで）。`video_not_found` は現在の文言のままである。

## 動画ページ {#video-page}

### バージョンの項目 {#versions-fact}

`Video.versions` があり `count` が 2 以上のとき、ファイルの情報の行（`VideoFacts` の 1 行目）の長さ、サイズ、追加日の**後**（所有者ではサムネイルの項目の前）に 1 つの項目を置く。所有者にもゲストにも示す（要件 12）。

- 左から右へ: lucide の `Layers`（`size-4`、`text-fg-subtle`、`aria-hidden`） → 「3 versions」（`tabular-nums`） → `ChevronDown`（`size-3.5`）。項目の中は `gap-1.5` で、ほかの項目との間隔は現在の `gap-x-4`（`sm` からは `gap-x-5`）のままである。
- 項目全体が `PopoverTrigger` の `button` である。文字はほかの項目と同じ `text-sm text-fg-muted` で、ホバーで `text-fg` になる。面も枠もなく、アクセントの色もない（`UI品質` の「文字の組み方」と「視覚的な階層」）。`-mx-1 px-1 rounded-sm` で、ホバーの `bg-hover-wash` を文字の周りに詰める。アクセシブルな名前は「3 versions of this video. Show versions」で、`title` も同じである。
- `count` が 1 のとき（ほかのバージョンのファイルがすべてない）は、項目を示さない。切り替える先がなく、「1 version」には意味がない。まとまりとその値は残り、ファイルが戻ると項目も戻る（Edge Case「代表のファイルがディスクから消えても」）。
- 情報の行のほかの項目、右側の操作、技術的な詳細の行、行の間隔（`gap-3`）は変わらない（`UI品質` の「余白のリズム」）。加えた項目は、ほかの項目と同じく折り返す。

### バージョンの一覧 {#versions-list}

項目を押すと、その下に `ui/Popover` が開く（`side="bottom"`、`align="start"`）。中身は上から下へ次のとおりである。

- 幅は `w-[min(28rem,calc(100vw-2rem))]`。面は現在の `PopoverContent`（`bg-elevated`、`shadow-elevated`）である。
- 一覧は `divide-y divide-border` の行を持つ `ul`（アクセシブルな名前は「Versions」）である。行は応答の順に従う（代表が先で、その後はタイトルの自然な順。[contracts、`GET /api/videos/{id}/versions`](contracts/screen-api.md#get-apivideosidversions)）。行は `py-2` で、6 行を超えると `max-h-80 overflow-y-auto` で中だけをスクロールする。
- 各行は `grid grid-cols-[1fr_auto] gap-x-2` で、左に 2 行の文字、右に所有者だけの操作がある。
  - 1 行目: タイトル（`text-sm text-fg`、1 行で省略し、全文は `title`）。代表の行は、タイトルの右に `text-xs text-fg-muted` で「Representative」を加える（`shrink-0`）。印は色や太さではなく文字で付ける。
  - 2 行目: 違い（`text-xs text-fg-muted tabular-nums`、1 行で省略）を、解像度 → コンテナ → 映像のコーデック → サイズ → 場所の順に、`text-fg-subtle` の「·」で区切って示す。分からない値（調べる前）は項目ごとに省く。場所は `folder` から作る「メディアフォルダの表示名 / 相対パス」で、縮めるときは末尾（ファイルに近い側）を残す（024「Issue List」と同じ縮め方）。所有者では行の `title` が絶対パス（`location.path`）を持ち、ゲストでは相対的な場所だけを持つ（016「Guest degradation」）。
  - 左の 2 行はまとめて、アクセシブルな名前「Play {title}, {differences}」を持つ**再生への `Link`**（`/videos/{id}`）である。ホバーで行が `bg-hover-wash` になる。`state.from` は現在の画面の `backTo` をそのまま渡すので、戻る先は変わらない。再生中（`status.playing || status.ended`）に選ぶと `autoplay` を加え、新しいページで再生が続く（前と次の操作と同じ）。
  - **現在の動画の行**はリンクではない。`aria-current="true"`、面は `bg-active-wash`、左端に `border-l-2 border-accent` を持つ（017「Member list」の現在のメンバーと同じ）。見た目では隠した「Now playing」がタイトルの前にある。押せないので、ホバーの面は変わらない。
  - **所有者の操作**（右の列）: `IconButton`（ghost、`sm`、lucide の `Ellipsis`、`text-fg-muted`、ホバーで `text-fg`）が `ui/Menu`（アクセシブルな名前は「More actions for {title}」）を開く。項目は上から「Make representative」（lucide の `Star`。代表の行には示さない）、区切り、「Remove from versions」（lucide の `Unlink`）である。再生は行そのもの、代表の変更はメニューの最初の項目、外す操作は区切りの下の最後であり、この順が `UI品質` の「操作の優先順位」（再生 → 代表 → 外す）を表す。外す操作は `text-danger` ではない。元に戻せ（もう一度まとめる）、削除ではないからである。
  - **ゲスト**には右の列がなく、行はリンクだけである（要件 12）。
- 一覧の下に、1 行の失敗の行を置く場所がある（後述の「失敗」を参照）。
- 開くと、フォーカスは現在の動画以外の最初の行のリンクへ移る（それがなければ最初の操作へ）。Tab は行の順に移る: 行のリンク → その行の `Ellipsis` → 次の行。
- 開いている間、Esc は画面ではなくポップオーバーだけを閉じ、フォーカスは項目に戻る。この `role="dialog"` のポップオーバーを、012「Interaction details」の Esc の例外（速度のメニュー、ツールチップ、`role="menu"`）に加える。行のメニューが開いているときは、Esc はメニューだけを閉じる（`ui/Menu` の既定）。
- 中身は、開くたびに `GET /api/videos/{id}/versions` で取得する。届くまでは、`count` 個の `Skeleton`（`h-10`）が行を埋める。取得に失敗すると、中に `text-xs text-danger` の 1 行（`role="alert"`）「Couldn't load the versions」と ghost の `sm` の「Retry」を示す。`video` の通知（`useVideoDetail`）で動画を取得し直すと、開いているポップオーバーは一覧も取得し直す。閉じているものは次に開いたときに取得する。

### 代表にする {#make-representative}

- メニューの「Make representative」を押すと `POST /api/videos/{id}/make-representative` を送る。送信中、その行の `Ellipsis` は `LoaderCircle`（`animate-spin`、`aria-disabled`）になる。ほかの行の操作は押せるままである。
- `200` では、一覧を応答の `VideoVersions` で置き換える。「Representative」の印がその行へ移り、順が変わる（代表が先）。ポップオーバーは開いたままで、トーストは出さない。印が移ることが結果である。動画も取得し直す（`versions.representativeId` が変わる）。
- ライブラリの項目のタイトルとサムネイルが新しい代表のものになる（要件 7）のは、ライブラリが `video` の通知で取得し直したとき（`useItemRefresh`）である。この画面はそれを知らせない。ライブラリのキャッシュは捨てない。戻ったときには、届いた通知によって、一覧はキャッシュの上で更新されている。

### まとまりから外す {#unbundle}

- メニューの「Remove from versions」を押すと `POST /api/videos/{id}/unbundle` を送る。確認のダイアログはない。もう一度まとめれば元に戻り、外した動画はまとめる前の値に戻るだけなので（要件 3）、何も失われない。送信中の見た目は代表の変更と同じである。
- `200` では次のとおりである。

  | 外した行 | 結果 |
  | --- | --- |
  | **ほかの行** | その行が一覧から抜け、項目の件数が減る。トースト「Removed "{title}" from the versions」。バージョンが 1 つだけ残るときは、まとまりは解消されている（[contracts/screen-api.md、`POST /api/videos/{id}/unbundle`](contracts/screen-api.md#post-apivideosidunbundle)）。ポップオーバーは閉じ、動画を取得し直し、項目は消える |
  | **現在の動画** | ポップオーバーが閉じ、動画を応答の `Video` で置き換える（`versions` はなくなり、`tags`、`progress`、`public` はその動画自身の値）。項目は消え、タグの一覧と公開の切り替えはその動画自身の値で描き直す。トーストは同じ |

- ポップオーバーが閉じたときは、フォーカスは項目があった場所に最も近い要素（技術的な詳細の行の前の操作、または右側の最初の操作）へ移る。開いたままのときは、次の行のリンク（それがなければ前の行）へ移る。

### 失敗 {#failure}

- 代表の変更や外す操作が失敗すると、一覧の下に `text-xs text-danger` の 1 行（`role="alert"`、前に `AlertCircle` の `size-4`）「Couldn't change the versions: {reason}」が出る。理由は `errorText`（前述の「文言」）から来る。ポップオーバーと一覧は開いたままである。この行は次の操作か、ポップオーバーが閉じたときに消える。
- `404 video_not_found` と `400 not_bundled`（別のタブで先に変えた）: 行を示し、一覧と動画を取得し直す。
- 情報の行の下の失敗の行（012 と 029 の場所）は使わない。ポップオーバーの中で起きたことは、ポップオーバーの中で知らせる。

### 再開位置 {#resume-position}

- まとまりの `progress.positionMs` がそのバージョンの `durationMs` 以上のとき、再生は 0 から始まる（R-11。Edge Case「バージョンの長さが違う」）。この判断は `pageDecisions.resumePosition` に加える。プレーヤーには「最初から再生します」の知らせは出ない。位置は操作バーが示す。
- 代表でないバージョンを再生すると、今と同じく `PUT /api/videos/{id}/progress` で位置を保存し、まとまりの位置が進む（要件 2、受け入れ条件 7）。画面がキーを選ぶことはない（R-2）。

### グループの行とバージョン {#group-line-and-versions}

グループのメンバーであり、まとまりのメンバーでもある動画は、タイトルの上のグループ名の行と、情報の行のバージョンの項目の両方を持つ。両者は別の情報（場所によるまとまりと、同じ動画の別のファイル）であり、1 つにしない。「次に再生」の並びは代表だけを示す（要件 5、[data-model.md、Shown videos and listing](data-model.md#shown-videos-and-listing)）。

## 選択バー {#selection-bar}

### まとめる操作 {#bundle-action}

- 「Bundle as versions」（lucide の `Layers` と文字、ghost の `sm` の `Button`）は、「Visibility」のすぐ後、縦の区切り線の前に置く。`sm` 未満の下の段では、2 つのタグの操作と「Visibility」の後の 4 つ目で、現在の規則（3 つが収まらないときは右端へ移す）に従う。4 つが 1 行に収まるときは幅を等分し、収まらないときは「Visibility」と「Bundle as versions」が次の段の右端へ移る。`SelectionBar` のコンテナクエリの幅は 4 つに合わせて調整する。
- **選択が 2 つ未満のときは示さない**（親 Issue の要件 11。計画の受け入れ「1 つの選択では示さない」）。理由を添えて `disabled` にする案は採らなかった。ほとんどの選択（動画 1 つ）で、灰色の操作が常に見えてしまう。2 つ目の動画を選ぶと現れる。
- **選択がまとめる上限 `maxBundleSelection`（20）を超えるとき**は、タグの操作の上限と同じく、理由「Bundle up to 20 videos at a time」を添えて `disabled` にする（library-ui.md、[List layout](../../docs/design-docs/library-ui.md#list-layout)）。タグの操作の上限（`maxVideoTagsSelection`、20,000）はここでは使わない。ダイアログは選んだ動画ごとに `GET /api/videos/{id}` を送り、動画ごとに 1 つの代表の行を並べるので、「Select all」で数千を選んで開くと、その数の要求を一度に送り、画面が固まる。行を読んで代表を選ぶことも、数十程度までしか成り立たない。この上限は画面側だけのもので、[contracts/screen-api.md、`POST /api/video-bundles`](contracts/screen-api.md#post-apivideo-bundles) のサーバーの上限（`too_many_videos`）は変わらない。
- 押すと、後述の「まとめるダイアログ」が開く。ポップオーバーではなくダイアログにする理由は「この形にした理由」にある。

### まとめるダイアログ {#bundle-dialog}

`ModalFrame` のダイアログ「Bundle as versions」である。中身は上から下へ次のとおりである。

- 1 段落の説明（`text-sm text-fg-muted`、「文言」の「ダイアログの説明」）。利用者は選ぶ前に、何が起きるかを読む。
- 代表の一覧（`radiogroup`、アクセシブルな名前は「Representative」）: `divide-y divide-border` の行で、枠やカードで囲まない。各行は `label` で、左に `input[type=radio]`（`size-4`、`accent-accent`。`Checkbox` と同じ大きさと色の扱い）、右に 2 行か 3 行の文字を持つ。
  - 1 行目: タイトル（`text-sm text-fg`、1 行で省略し、全文は `title`）。
  - 2 行目: 違い（「バージョンの一覧」の 2 行目と同じ書式と順）。
  - 3 行目（あるときだけ）: `text-xs text-fg-muted` で、手で付けたタグの名前を「·」でつないで示す（`tags` からフォルダ名だけから来るものを除いたもの）。どのタグがまとまりのものになり、どのタグが脇に置かれるかを、選ぶ前に示す。すでにまとまりにある動画（`versions` を持つ）は、この行を「Already 3 versions — all of them join」で始める（[contracts/screen-api.md、`POST /api/video-bundles`](contracts/screen-api.md#post-apivideo-bundles)）。
  - 行は `py-2` である。6 行を超えると、`max-h-80 overflow-y-auto` で中だけをスクロールする。ダイアログのほかの部分の高さは `ModalFrame` の規則に従う。
  - 行の順: 選択バーからは選択の順（`selectedIds`）、候補の画面からはペアの順（id の昇順）。
- 一覧の中身は、選んだ id ごとに `GET /api/videos/{id}` で取得する（最大 `maxBundleSelection`、20）。選択バーからは、一覧の項目が読み込まれていない id があるからである。候補の画面からは、ペアの 2 つの `Video` をそのまま渡し、何も取得しない。読み込むまでは `Skeleton`（`h-12`）が行を埋める。どれか 1 つでも失敗すると（404 を含む）、`text-sm text-danger` の 1 行（`role="alert"`）「Couldn't load the selected videos」と ghost の `sm` の「Retry」が一覧に代わり、「Bundle」は `disabled` になる。
- 操作の行（`border-t border-border`、右寄せ、`gap-2`）: 副の「Cancel」（最初のフォーカス）と主の「Bundle」。**代表を選ぶまで「Bundle」は `disabled` である。** 最初の行をあらかじめ選んでおく案は採らなかった。読まずに 1 回押すだけで、誰の値が残るかが決まってしまう（014「Merge and delete」と同じ扱い）。
- 「Bundle」を押すと `POST /api/video-bundles` を送る（`videoIds` は選んだ id、`representativeId` は選んだ代表）。送信中は両方のボタンが `disabled` で、「Bundle」は `LoaderCircle` を示す（設定画面のフォルダ削除の確認と同じ）。
- `200` では、ダイアログが閉じ、トースト「Bundled {N} videos as versions of "{title}"」が出る（N は応答の `items.length`、タイトルは代表のもの）。
  - 選択バーから: 選択を解除し、ライブラリの一覧を取得し直す（代表でない項目は一覧から抜ける。受け入れ条件 6）。取得し直す間、グリッドの位置は保つ。フォーカスは「Select all」があった場所へ移る（バーはなくなるので、グリッドの最初のカード）。
  - 候補の画面から: 後述の「判断」を参照。
- 失敗すると、ダイアログは開いたままで、操作の行の上に `text-sm text-danger` の 1 行（`role="alert"`）「Couldn't bundle: {reason}」が出る。選んだ代表は保つ。`404 video_not_found`（選んだ動画がなくなった）では、行を示し、一覧を取得し直す。
- Esc と「Cancel」は送らずに閉じる。フォーカスは開いたボタン（「Bundle as versions」、候補の画面では「Same video…」）へ戻る。

### カード {#card}

ライブラリのカードとリスト表示の行は変わらない。まとめた後も、代表の項目はまとめる前と同じ見た目である（`UI品質` の「一覧の項目は今日の動画の項目と同じ見た目」）。

## 重複のページ {#duplicates-page}

### 入口 {#entry}

- 「Duplicates」（lucide の `Layers`、`/duplicates`、所有者だけ。`navEntries` の `ownerOnly`）を、サイドバーの上の部分の「Tags」のすぐ後に加える。レールとドロワーの表示では、ほかの項目と同じく振る舞う。件数のバッジはない（「この形にした理由」）。
- ルートは `/tags` と同じ形である（`AppShell` の中で、使うときだけ読み込む）。ゲストがこの URL を開いたときは `/tags` と同じく扱う（所有者だけの応答の 401 はゲートが扱う。016「Gate」）。
- `document.title` は「Duplicates」である。

### レイアウト {#layout}

- 本体の幅と余白はタグ管理画面と同じである（`mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-8`）。上から、`h1`「Possible duplicates」（`text-xl font-semibold`）、件数の行、ペアの一覧である。上部バーには何も置かない。検索も絞り込みもない（最大 200 ペアで、判断するにつれて数は減る）。
- 件数の行は `text-xs text-fg-muted tabular-nums`、`role="status"`、`aria-live="polite"` で、「12 pairs」と示す。`total` が 200 を超えるときは「Showing 200 of 340 pairs」と示す。
- ペアの一覧は `divide-y divide-border` の行を持つ `ul` で、枠やカードで囲まない。ペアは `py-4` である（2 つの動画を持つので、タグの行より広い）。

### ペア {#pair}

ペア（`li`）は上から 2 つの段を持つ。

1. **見出しの行**（`flex items-center justify-between gap-3`）: 左に `text-xs text-fg-muted` で「Same length · similar frames」（候補である理由を 1 行で言う）。`distance` の数値は示さない。意味があるのはしきい値の内にあることだけで、利用者が比べられる値ではない。右に 2 つの操作（`gap-2`）: 副の `sm` の `Button`「Different videos」と、主の `sm` の「Same video…」（lucide の `Layers`）。DOM の順も同じなので、ペアの中の最初の Tab の止まり先は「Different videos」、次が「Same video…」である。判断が先で、動画の中身が後である（`UI品質` の「候補の一覧では「同じ動画」と「別の動画」の判断を先に置く」）。「Same video…」が主なのは、この画面にいる人の主な判断は「同じ」で、「別」は例外を記録するものだからである。
2. **2 つの動画**（`mt-3 grid gap-3 sm:grid-cols-2`）: ペアの `videos` を id の昇順で左右に置く（`sm` 未満では上下）。それぞれは次のとおりである（関連動画の行と同じ密度。017「Member list」）。
   - `flex gap-3`。左にサムネイル（`w-40 shrink-0`、`aspect-video`、`rounded-md`、`bg-surface`。ないときは `ImageOff` の箱。右下に長さのバッジ。関連動画の行と同じコンポーネント）。
   - 右に 3 行: タイトル（`text-sm font-medium text-fg`、2 行で省略し、全文は `title`）、違い（「バージョンの一覧」の 2 行目と同じ書式だが、場所は次の行へ移る）、場所（`text-xs text-fg-muted`、末尾を残して 1 行で省略し、絶対パスは `title`）。
   - 動画全体が `/videos/{id}` への `Link` である（`state.from` は `/duplicates`）。ホバーで `bg-hover-wash` になる（行の `-m-1.5 p-1.5` による輪郭）。アクセシブルな名前は「{title} {length}」である（関連動画の行と同じ）。利用者はプレーヤー画面を開いて比べ、戻ってこられる。戻る先は候補の画面で、決めていないペアはまだそこにある。
   - 2 つのサムネイルは同じ大きさで、どちらも強調しない。違いは 2 行目の数値から読む。解像度やサイズの大きい方に印を付ける案は採らなかった。機械が「良い方」を指すと、代表の選択を誘導してしまう。
- 見出しの行と 2 つの動画は `gap-3` で分け、ペアどうしは `divide-y` と `py-4` で分ける。ペアの中に線や枠はない。

### 判断 {#deciding}

| 操作 | 振る舞い |
| --- | --- |
| **「Same video…」** | ペアの 2 つの動画で「まとめるダイアログ」を開く（応答の `Video` をそのまま使い、取得し直さない）。「Bundle」で `200` になると、ダイアログが閉じ、ペアが一覧から抜け、件数が減り、トーストが出る（「まとめるダイアログ」と同じ文言）。一覧の取得し直しは `scan` の通知に任せる。フォーカスは次のペアの「Different videos」へ移り、なければ前のペアのもの、ペアが残らないときは `h1` へ移る |
| **「Different videos」** | 確認のダイアログなしで `POST /api/version-candidates/dismiss` を送る。送信中はペアの 2 つのボタンが `disabled` で、押した方が `LoaderCircle` を示す。`204` ではペアが一覧から抜け、件数が減り、トースト「Marked as different videos. They won't be suggested again」が出る。フォーカスは「Same video…」と同じく移る |
| **ペアがなくなった**（`404 video_not_found`。利用者が確かめている間に、スキャンで片方が消えた） | トースト「This pair is no longer a candidate」を出し、一覧を取得し直す（Edge Case） |
| そのほかの失敗 | トースト「Couldn't bundle: {reason}」（ダイアログの中ではダイアログの行）、または `errorText` の文言。ペアは残る |

「Different videos」は元に戻せない（要件 10）が、その結果は「候補として示されなくなる」ことだけで、手でまとめること（選択バー）はまだできるので、確認のダイアログは挟まない。トーストは、それが戻らないことを言う。

### 更新 {#refresh}

- `GET /api/version-candidates` は開いたときに読む。所有者のシェルが受け取る `scan` の通知（`fingerprint` のジョブの成功時と失敗時に送る。[contracts/screen-api.md、`/api/events`](contracts/screen-api.md#apievents)）で取得し直す。取得し直す間、一覧は変わらない見た目である（スケルトンはない）。応答が届くと一覧を置き換え、スクロール位置は保つ。新しいペアは先頭に加わり、片方がなくなったペアは消える（取り込み中に候補が加わったり消えたりすることに対応する）。
- 開いているダイアログのペアが取得し直しで消えても、ダイアログは閉じない。「Bundle」の 404 がそれを知らせる。

### 状態 {#states}

| 状態 | 画面が示すもの |
| --- | --- |
| 読み込み中 | 件数の行は「Loading…」と示し、3 つの `Skeleton`（`h-24`）が一覧を埋める |
| 読み込みの失敗 | `EmptyState`（danger、`AlertCircle`）「Couldn't load the candidates」と `Button`「Retry」 |
| 候補がない | `EmptyState`（lucide の `Layers`）「No possible duplicates」、説明「When a scan finds files that look like the same video, they show up here for you to confirm. You can also select videos in the library and bundle them yourself.」。ボタンはない。取り込みの入口は上部バーの「Refresh library」である |
| 判断して 0 ペアになった | 同じ空の状態に切り替わる。件数の行は「0 pairs」のまま残らず、空の状態の見出しが件数に代わる |
| 操作の失敗 | 前述の「判断」を参照 |

## 幅による振る舞い {#responsive-behaviour}

ブレークポイントは Tailwind の既定だけで、CSS で適用する（library-ui.md、[Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception)）。判断する幅は 360px、768px、1280px である（プレーヤー画面は `lg` から 2 列になる）。

| 幅 | バージョンの項目と一覧 | 選択バー | ダイアログ | 候補の画面 |
| --- | --- | --- | --- | --- |
| 1280px | 長さ、サイズ、追加日、「3 versions」が 1 行に並ぶ。ポップオーバーは `28rem` で、項目の左端にそろえる | 「Bundle as versions」は 2 つのタグの操作と「Visibility」の後の 4 つ目で、1 行に並ぶ | 幅は `max-w-lg`。一覧は 6 行まで伸び、その後は中をスクロールする | 2 つの動画を横に並べ、サムネイルは `w-40`。見出しの行の右に 2 つのボタン |
| 768px | 同じ | 同じ（`sm` から 1 行） | 同じ | 同じ |
| 360px | 項目はほかの項目と同じく折り返す。ポップオーバーは画面の幅から `2rem` を引いた幅で、行の 2 行目は省略する。横スクロールはない | 下の段に 4 つが収まらないので、「Visibility」と「Bundle as versions」が次の段の右端へ移る（現在の規則） | 画面の幅いっぱい（`ModalFrame` の既定）。行の 2 行目は省略する | 2 つの動画は縦に重なり、それぞれサムネイルと 3 行を横に並べたまま保つ。見出しの行のボタンは折り返し、右寄せのままである |

- ポップオーバーの行の 2 行目は、どの幅でも 1 行で省略する。全文は行の `title` にある。
- 候補の画面の場所の行は、どの幅でも末尾（ファイル名に近い側）を残す。

## レビューの基準 {#review-criteria}

実際の画面を見て判断する（library-ui.md、[Layout verified by people, not machines](../../docs/design-docs/library-ui.md#layout-verified-by-people-not-machines)）。「ある」だけでは基準を満たさない（Q-4）。

1. **視覚的な階層（プレーヤー画面）**: まとめた動画のページで、視線はプレーヤー → タイトル → タグと進み、「3 versions」は長さ、サイズ、追加日と同じ重みで、それらより先に読まれない。まとめていない動画のページと並べると、タイトル、タグ、2 行の情報の行の位置と間隔は同じで、違いは項目が 1 つ多いことだけである（`UI品質` の「視覚的な階層」と「余白のリズム」）。
2. **視覚的な階層（ライブラリ）**: まとめた代表のカードと、まとめていない動画のカードは見分けられない。カードに印、数字、帯を加えない（受け入れ条件 6。`UI品質` の「要件を満たさない変更」）。リスト表示の行も同じである。
3. **情報の密度（プレーヤー画面）**: ポップオーバーを開く前のバージョンの情報は「3 versions」の項目だけで、どのバージョンの解像度、サイズ、場所もどこにも現れない。開くと、各行の 2 行目が 2 つの違い（解像度、コーデック、サイズ、場所）を数値で示し、行の間に説明の文や見出しはない（`UI品質` の「情報の密度」）。
4. **文字の組み方**: 項目の文字とポップオーバーの行のタイトルは、情報の行と同じく `text-sm` である。行の 2 行目は、技術的な詳細の行と同じく従の色の `text-xs` である。ポップオーバーには太字、大きな見出し、アクセントの色の文字がない（`UI品質` の「文字の組み方」）。「Representative」は文字の印で、代表を色や太さで示さない。
5. **操作の優先順位（プレーヤー画面）**: ポップオーバーを開いた後、行を 1 回押すと別のバージョンのファイルを再生し（受け入れ条件 7）、再生中に選ぶと新しいページで再生が続く。代表の変更は 2 回押す（`Ellipsis` → 最初の項目）、外す操作も 2 回（`Ellipsis` → 区切りの下）で、どちらも行の表面にはない。最初に触れるのはやはり再生で、ポップオーバーを開かない限り画面は変わらない。
6. **代表の変更と外す操作の結果**: 代表を B に変えると、ポップオーバーの「Representative」が B の行へ移る。ライブラリに戻ると、1 つの項目のタイトルとサムネイルは B のもので、タグは X のままである（受け入れ条件 8）。B を外すと、B のページのタグは Y に戻り、B はライブラリに別の項目として現れ、A のまとまりのタグは X のままである（受け入れ条件 9）。どちらの結果も、ページを離れずにポップオーバーの中で見える。
7. **再開位置**: まとまりの再生位置がそのバージョンの長さ以上のとき、そのバージョンは 0 から始まる。操作バーの位置は 0 で、知らせや層は現れない。
8. **選択バー**: 動画を 1 つ選んだときはバーに「Bundle as versions」がなく、2 つ目を選ぶと現れる。ダイアログでは、代表を選ぶまで「Bundle」を押せない。選んで押すと、一覧には代表だけが残り、選択は解除される（受け入れ条件 6）。21 以上を選ぶと「Bundle as versions」は押せず、その理由が読め、ダイアログは開かない。360px で下の段は崩れず、横スクロールはない。
9. **ダイアログ**: 順は説明の段落 → 行 → 操作である。行には枠がなく、各行の 3 行目が、選ぶ前にどのタグが残るかを示す。すでにまとめた動画の行は「Already N versions — all of them join」を示す。
10. **候補の画面（階層と密度）**: ペアを見ると、視線はまず右上の 2 つのボタン、次に 2 つのサムネイルとタイトル、最後に違いの数値と場所へ進む。2 つのサムネイルは同じ大きさで、どちらも強調しない。ペアの中に線、枠、カードはなく、ペアどうしは 1 本の線で分かれる。360px で 2 つの動画は、はみ出しも重なりもなく縦に重なる。
11. **候補の画面（判断）**: 「Same video…」 → 代表を選ぶ → 「Bundle」でペアが消え、ライブラリは 1 つの項目を示す（受け入れ条件 3、6）。「Different videos」でペアが消え、スキャンし直しても戻らない（受け入れ条件 5）。取り込み中に新しいペアが先頭に加わっても、読んでいるペアは動かない。
12. **状態の表示**: 送信中は、押したボタンか `Ellipsis` がスピナーを示し、もう一度押しても二重に送らない。失敗はその場の赤い行（ポップオーバーの一覧の下、ダイアログの操作の上）で、開いているものは開いたままである。トーストになるのは候補の画面の失敗だけである（行がなくなったのか、外すべきなのかを 1 行で言えるため）。
13. **キーボード**: プレーヤー画面では Tab が項目に届き、Enter で開くとフォーカスはほかの最初のバージョンの行にある。Esc で閉じて項目に戻り、2 回目の Esc で画面を閉じる。候補の画面では Tab はペアごとに「Different videos」 → 「Same video…」 → 左の動画 → 右の動画と移る。
14. **ゲスト**: ログアウトした状態で、まとめた動画には「3 versions」の項目とポップオーバーがあり、行を押すと B を再生する（受け入れ条件 12）。行に `Ellipsis` はない。サイドバーに「Duplicates」はなく、選択バーはまったくない。
15. **要件を満たさない例**（`UI品質`）: 代表でないバージョンが一覧に現れる。バージョンの一覧がプレーヤー画面に常に開いたパネルやタブとして現れる。「3 versions」がタイトルの隣かプレーヤーの中にあり、タグより先に目に入る。まとめたカードに印や件数があり、ほかのカードと違って見える。候補の画面で 2 つのうち 1 つが「おすすめ」として先に強調される。

## 色 {#colour}

新しい組は使わない。ポップオーバーとダイアログの文字は `elevated` の上の `fg` と `fg-muted`、失敗の行は `elevated` の上の `danger`（014「Accessibility」で加えた組）、候補の画面の文字は `bg` の上の `fg` と `fg-muted` で、トーストは変わらない。`fg-subtle` は印（`Layers`、「·」）にだけ使い、文字には使わない。`bg-hover-wash`、`bg-active-wash`、`border-accent` は文字ではなく、`pairs` の外にある。新しいトークンは加えない。

## アクセシビリティ {#accessibility}

親 Issue はスクリーンリーダーやコントラストの設計を求めていないので、決めるのは名前だけである（012 と 029 と同じ範囲）。

- 項目のアクセシブルな名前:「3 versions of this video. Show versions」。ポップオーバーは `role="dialog"` で、アクセシブルな名前は「Versions」である。
- 行のリンク:「Play {title}, {differences}」。現在の動画の行は `aria-current="true"` と隠した「Now playing」を持つ。「Representative」の印は見える文字で、隠さない。
- 行のメニューを開くボタン:「More actions for {title}」。
- ダイアログの一覧: `radiogroup`、アクセシブルな名前は「Representative」。各 `label` はタイトルと違いを含む。
- 候補の画面: 各ペアは `li` である。見出しの行の「Same length · similar frames」はペアの名前に使わない（どのペアでも同じだからである）。2 つのリンクのアクセシブルな名前は「{title} {length}」である。ボタンのアクセシブルな名前は、見える文字に「for {title A} and {title B}」を付け足す（「Same video, for A and B」）。
- 失敗の行は `role="alert"` である。スピナーは `aria-hidden` で、送信中の要素は `aria-disabled` か `disabled` である。
