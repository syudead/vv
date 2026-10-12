---
source: specs/029-video-overrides/ui-design.md
sourceHash: 9d5a4804b247aa791442cb668935f993392c883505e9976e0321fadec7485e62
---

# UI 設計: 動画ページで表示名を編集し、現在のフレームを代表サムネイルにする {#ui-design-editing-the-display-name-and-using-the-current-frame-as-the-representative-thumbnail-on-the-video-page}

**機能**: [親 Issue #517](https://github.com/syudead/vv/issues/517) · [plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) · [research.md R-4](research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded) · [R-8](research.md#r-8-the-screen-api-is-two-puts-per-video-an-empty-display-name-and-a-null-position-clear) · [R-10](research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit)

出典: 視覚的な規則は次の文書に従い、ここでは決め直さない。

| 項目 | 出典 |
| --- | --- |
| 色、操作の状態、幅のブレークポイント、再生画面の構造 | [Library UI](../../docs/design-docs/library-ui.md) (一覧の構造は [List layout](../../docs/design-docs/library-ui.md#list-layout) のカード、再生画面の構造は [Video page layout](../../docs/design-docs/library-ui.md#video-page-layout)) |
| 役割のトークン | [`web/src/index.css`](../../web/src/index.css) の `@theme`。名前で参照し、値を写さない |
| テストで確かめるコントラストの組 | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| 再生画面の列、タイトルの書式、右側に操作を持つ 2 行の「Video facts」、失敗の行 | [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) と現在の [`web/src/player/VideoPage.tsx`](../../web/src/player/VideoPage.tsx)、[`VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx) |
| タイトルとタグのまとまり、「Add tag」の入力欄とそのキー | [specs/014-video-tags/ui-design.md「Video page tags」](../014-video-tags/ui-design.md#video-page-tags) |
| タイトルの上のグループ名の行 (タイトルに従う 1 行の書式) | [specs/017-folder-groups/ui-design.md「Group line」](../017-folder-groups/ui-design.md#group-line) |
| 公開範囲の切り替え (送信中の扱いと失敗の行) | [specs/016-single-account-auth/ui-design.md「Visibility toggle」](../016-single-account-auth/ui-design.md#visibility-toggle) |
| 文言の置き場所とその書式 | [Screen strings and formatting (i18n)](../../docs/design-docs/i18n.md)。ここの英語は意図の下書きで、実装の後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正である |

この機能は再生画面 (`/videos/:id`) に 3 つのものを加え、どれも**所有者**だけに出す。

1. タイトルの**編集** (要件 1、2。受け入れ条件 1、2)。
2. 表示名を設定したときの、タイトルの下の**ファイル名の行** (要件 7。受け入れ条件 7)。
3. ファイルの詳細の行の**サムネイルの項目と操作** (要件 5、6。受け入れ条件 5、6)。

ほかは何も変わらない。一覧のカード、フォルダの画面、リスト表示、関連動画、グループのカードは `title` だけを描くので、表示名は API の `title` を通してそこに現れる (要件 1、[contracts/screen-api.md, `Video` changes](contracts/screen-api.md#video-changes))。カードはファイル名を出さない (`UI品質` の「要件を満たさない」例)。ゲストの再生画面は今のままで、3 つのどれも出さない (要件 9)。プレーヤーの内側 (コントロールバー、レイヤー) は変わらない。色、角丸、影のトークンは新しく加えず、`tokens.test.ts` の `pairs` にも何も加えない (後の「色」を参照)。

## ほかの場所ではなくここに置く理由 {#why-here-and-not-elsewhere}

タイトルは設定や別のウィンドウではなく**その場で** (`h1` のある場所で) 編集する。要件が「動画ページを離れずに完了する」と言い、比べた現在の製品 (ストレージでの名前の変更、写真と動画の管理画面) が「名前のある場所で名前を直す」に落ち着いているからだ。ダイアログでは、タイトルの文字の大きさで名前を確かめられず、狭い幅ではプレーヤーを隠してしまう。

「現在のフレームをサムネイルにする」は、次の理由で、**プレーヤーのコントロールバーではなく**ファイルの詳細の行の右端の操作 (「Open file」と「Copy path」の隣) に置く。

- コントロールバーの部品は再生の操作と同じ重みを持つが、編集は「再生の操作より目立たない」ものにする (`UI品質` の「視覚的な階層」と「操作の優先度」)。
- コントロールバーは 360px でも 1 行に収める前提で、画質、字幕、速度を加えた今はもう 1 つの操作を置く余地がない (specs/027「Responsive behaviour」)。
- フレームからサムネイルを選ばせる現在の製品 (動画公開サービスの「このフレームを使う」) は、その操作をプレーヤーの下に置いている。
- 選んだ結果と、それを解除する入口 (後の「サムネイルの情報」を参照) が同じ行にあるので、状態、操作、失敗が 1 か所で読める。

ファイル名の行は、`UI品質` の「情報の密度」により、**表示名を設定したときだけ**出す (元のファイル名は、表示名があるときにタイトルに従う情報として見えればよい)。行があること自体が「この動画は上書きされている」ことを示す (要件 7)。

## 文言 {#words}

| 場所 | 文言 | 備考 |
| --- | --- | --- |
| タイトルの隣の編集ボタンのアクセシブルな名前とツールチップ | Edit name | |
| 編集の入力欄のアクセシブルな名前 | Display name | |
| 編集の入力欄のプレースホルダー | ファイル名のタイトルそのもの | 利用者のデータで、翻訳しない |
| 保存 | Save | |
| 取り消し | Cancel | |
| ファイル名の行のアクセシブルなラベルと `title` | File name | |
| サムネイルの項目のアクセシブルなラベル、`title`、画像の `alt` | Thumbnail at 1:23 | 時刻は `formatDuration` の書式を使う |
| サムネイルを設定するボタンのアクセシブルな名前とツールチップ | Use current frame as thumbnail | |
| それを解除する × のアクセシブルな名前とツールチップ | Use automatic thumbnail | |
| 名前を保存できなかったときの行 | Couldn't save the name: {reason} | |
| サムネイルを設定または解除できなかったときの行 | Couldn't change the thumbnail: {reason} | |
| `display_name_control_characters` | The name can't contain control characters | |
| `display_name_too_long` | The name can't be longer than {limit} characters | |
| `duration_unknown` | The video's length isn't known yet | |
| `thumbnail_position_out_of_range` | The position is past the end of the video | |
| `thumbnail_frame_unavailable` | No image could be made from this frame | |

- 5 つの理由は `web/src/i18n/errors.ts` の `reason` の表に加える ([contracts/screen-api.md, Added `code` and `reason` values](contracts/screen-api.md#added-code-and-reason-values))。`file_unavailable` と `video_not_found` は今の文言のままにする。
- 「Thumbnail」と「name」は、一覧のカード (サムネイル) とタイトルの編集 (名前) と同じものを指す。「display name」という言葉は入力欄のアクセシブルな名前にだけ使い、見える文言には現れない。所有者にとってそれは「動画の名前」であり、表示名とファイル名の違いはファイル名の行が示す。

## タイトルの編集 {#title-editing}

### 配置と重み {#placement-and-weight}

- 編集の入口として、タイトル (`h1`) の右に `IconButton` (`size="sm"`、ghost、lucide `Pencil`) を 1 つ置く。`flex items-start gap-2` で `h1` と行を共有し、`h1` は `min-w-0 flex-1`、ボタンは `shrink-0` にする。ボタンの縦の中心はタイトルの**1 行目**にそろえる (`h-8` のボタンを `text-2xl` `leading-snug` の 1 行目に合わせるため、`sm` 以上ではおよそ `mt-0.5`、未満では `-mt-0.5`)。タイトルが折り返しても、ボタンは右上にとどまり、下へ落ちない。
- 色は `text-fg-muted` で、ホバーで `text-fg` になり、「Open file」や「Copy path」と同じだ。面も枠もなく、アクセントの色もない。タイトル (`text-xl` から `text-2xl`、`font-semibold`、`text-fg`) より先に目に入らず、タグのチップ (`bg-elevated` の面を持つ) より弱い (`UI品質` の「視覚的な階層」)。
- 常に見える。ホバーのない機器では入口が消えてしまうので、ポインターのホバー時だけに出すことはしない。ゲストには出さない (今と同じく `h1` だけ)。
- タイトルの書式は変わらない。表示名は、ファイル名のタイトルと同じ `h1` の書式で現れる (`UI品質` の「文字の組み方」)。

### 編集中 {#edit-mode}

編集ボタンを押すと、`h1` の場所がその場で**入力欄**に変わる (編集中は `h1` を描かない)。

- 入力欄は `input[type=text]` だ。文字の大きさ、太さ、行の高さはタイトルと同じにする (`text-xl` から `text-2xl`、`font-semibold`、`text-fg`、`leading-snug`)。面は `bg-field`、枠は `border border-border`、`rounded-md`、`px-2 py-1` だ。`-mx-2` で文字の左端をタイトルの左端にそろえる (グループ名の行の `-ml-2` と同じ考え方)。フォーカスで `border-accent` になる (検索欄や「Add tag」と同じ)。幅は列いっぱい (`w-full`) だ。
- 初期値は現在の `title` (表示名があればそれ、なければファイル名のタイトル) だ。フォーカスは文字を全選択した状態で入力欄に移る。プレースホルダーは `fileTitle` なので、入力欄を空にすると「空 = ファイル名に戻る」ことがその場で見える (説明の文は加えない)。
- 入力欄の右 (`sm` 未満では下) に、**Save** (`Button` primary、`sm`) と **Cancel** (ghost、`sm`) を `gap-2` で置く。この一時的な状態の間だけ現れるので、Save のアクセントの色は「編集は再生の操作より目立たない」を崩さない。`sm` 未満では、入力欄を 1 行目に、2 つのボタンを 2 行目に左寄せで置く。
- 編集中は、タイトルの下のファイル名の行 (後の「ファイル名の行」を参照) を隠し、入力欄のプレースホルダーがその役を担う。タグ、公開範囲の切り替え、ファイルの詳細は編集中も見えたままにする。
- キー: Enter で保存し、Esc で取り消す。入力欄にフォーカスがある間は、再生画面のキー (Space、F、M、C、0、Esc) は働かない (現在の `keyboard.ts` の `isEditable`。014「Add input」と同じ)。入力欄の外で押した Esc だけが画面を閉じる。
- 取り消し (Cancel または Esc) は何も送らず、`h1` に戻り、フォーカスを編集ボタンに戻す。

### 保存 {#save}

- 保存は `PUT /api/videos/{id}/display-name` を 1 回送る ([contracts/screen-api.md, `PUT /api/videos/{id}/display-name`](contracts/screen-api.md#put-apivideosiddisplay-name))。
  - 入力値が現在の `title` と等しく、表示名が設定されていないときは、何も送らず、Cancel として働く (ファイル名と同じ表示名は作らない)。
  - 空 (空白だけを含む) は解除として送る (`displayName: ""`。境界条件「空または空白だけ」)。表示名のない動画に空を送ると `200` を返し、何も変わらない。
- 送信中は、入力欄を `readOnly`、Save を `aria-disabled` にし (フォーカスを失わないよう `disabled` にはしない。016「Visibility toggle」と同じ)、Save のアイコンを lucide `LoaderCircle` (`animate-spin motion-reduce:animate-none`) にする。Cancel は押せるままで、押すと応答を待たずに `h1` に戻る (届いた応答は動画を置き換えるためだけに使う)。
- `200` では、応答の `Video` で `useVideoDetail` の動画を置き換え、表示を `h1` に戻す。タイトル、ファイル名の行、`document.title` が新しい値になる。フォーカスは編集ボタンに戻る。トーストは出さない。変わったタイトルが結果である (受け入れ条件 1、2)。
- 失敗では編集を続ける。入力欄の文字は残し、入力欄とボタンの行の**すぐ下**に「Couldn't save the name: {reason}」という 1 行を出す (`role="alert"`、`text-sm text-danger`、先頭に lucide `AlertCircle` `size-4`。012「Video facts」の開けなかったときの行と同じ形)。理由は `errorText` だ (前の「文言」を参照)。この行は、次の保存、取り消し、別の動画への移動で消える。バナー、トースト、ダイアログは出さない。
  - `display_name_too_long` (境界条件「長すぎる」): 理由には 200 という `limit` を含める。入力欄に `maxLength` は付けない。ブラウザはコードポイントを違う数え方で数え、貼り付けた文字が黙って切られるからだ。
  - `404 video_not_found`: 動画を取り直す (消えていれば、現在の「動画が消えた」レイヤーが現れる)。
- 編集中に `/api/events` の `video` イベントで動画を取り直したとき (別のタブや外部 API からの変更。境界条件「同時の変更」)、入力欄の文字は上書きしない。保存すると、この変更が最後の書き込みになる。取り消すと、取り直したタイトルが出る。

## ファイル名の行 {#file-name-line}

表示名を設定した (`displayName` がある) 動画では、タイトルのすぐ下に 1 行を置き、所有者だけに出す (要件 7。受け入れ条件 7)。

- 中身は左から、lucide `FileVideo` (`size-3.5`、`text-fg-subtle`、`aria-hidden`) → `fileTitle` (`text-fg-muted`、1 行に切り詰め、`title` に `File name: {file name}` を設定) だ。行全体は `text-xs` (`sm` 以上では `text-sm`) だ。グループ名の行と同じ書式で、タイトルより小さく、従の色にする (`UI品質` の「文字の組み方」と「元のファイル名は補助の情報」)。スクリーンリーダーのために、値の前に視覚的に隠した「File name」を置く。
- `h1` の行 (編集ボタンを含む) とこの行は、`flex flex-col gap-1` で 1 つの小さなまとまりを作り、現在のタイトルとタグのまとまり (`gap-2`) の中に置く。タイトルとファイル名の間 (`gap-1`) はタイトルとタグの間 (`gap-2`) より狭いので、ファイル名がタグと並ぶものではなく、タイトルに属する補足であることが間隔で分かる。まとまりの外の間隔 (`gap-2`、`gap-5`) は変わらない (`UI品質` の「余白のリズム」)。
- 押せない。リンクでもボタンでもない (Q-5: 背後に何もない要素だが、文字の色は `fg-muted` で、印もホバーの面もないので、押せるように見せるものはない。その価値は要件 7 そのものである)。
- 表示名のない動画には出さない。ゲストには出さない (ゲストへの応答には `displayName` も `fileTitle` もない)。
- 広い画面 (`lg` 以上) での左列の高さの配分 (プレーヤーの `max-w-[calc(max(100dvh-8.25rem,15rem)*…)]`) は変わらない。この行で列があふれるウィンドウでは、グループ名の行と同じく、左列がスクロールする (012「`lg` (1024px) and above」)。

## サムネイルの情報 {#thumbnail-fact}

ファイルの詳細の行 (`VideoFacts` の 1 行目) に 2 つのものを加え、所有者だけに出す。

### 項目 (設定したときだけ) {#item-only-when-set}

`thumbnailPositionMs` があるときだけ、長さ、サイズ、追加日の**後に** 4 つ目の項目を置く。

- 中身は左から、lucide `Image` (`size-4`、`text-fg-subtle`、`aria-hidden`) → 現在の代表サムネイルの小さな画像 (`img`、`thumbnailUrl`、高さ `h-6`、16:9 の幅、`rounded-sm`、`overflow-hidden`、`bg-surface`、`object-cover`) → 位置 (`formatDuration(thumbnailPositionMs)`、`tabular-nums`) → 解除の × だ。項目の中は `gap-1.5`、項目の間は現在の `gap-x-4` (`sm` 以上では `gap-x-5`) だ。
- 小さな画像により、利用者は「画像が選んだフレームになった」ことをこの画面で確かめられる。一覧のカードは戻らないと見えず、位置の数字だけでは黒いフレームを選んだことが分からない。画像の `alt` は「Thumbnail at 1:23」だ。読み込み中や失敗の後は、空の `bg-surface` の箱のままにする。
- 時刻は、隣に画像と `Image` の印があるので、`Clock` の長さと区別できる。ほかの項目と同じく文字のラベルは出さず、視覚的に隠した「Thumbnail at 1:23」と `title` がそれを担う。
- 解除の ×: `size-6` の正方形のボタンで、lucide `X` (`size-3`)、`text-fg-muted`、ホバーで `text-fg` (タグのチップの × と同じ大きさ)。アクセシブルな名前は「Use automatic thumbnail」だ。押すと `PUT /api/videos/{id}/thumbnail-position` に `positionMs: null` を送る。送信中は × が `LoaderCircle` (回転) になり、`aria-disabled` になる。`200` では項目が消え、画像は自動の位置に戻る (受け入れ条件 6)。
- 幅が足りないときは、行のほかの項目と同じく折り返す。

### キャプチャのボタン {#capture-button}

- 詳細の行の右端の操作のまとまり (`ml-auto`) の**先頭**、「Open file」の左に、`IconButton` (`size="sm"`、ghost、lucide `Camera`、`text-fg-muted`、ホバーで `text-fg`) を置く。アクセシブルな名前とツールチップは「Use current frame as thumbnail」だ。「Open file」と「Copy path」は隣り合ったまま右へずれる。場所を持たない動画 (今は右側のまとまりがない) では、見ているのが所有者でプレーヤーが表示されているときに、このボタンだけのまとまりを置く。
- 押すと、その時点のプレーヤーの論理的な再生位置 (保存する再生位置が読むのと同じ値) をミリ秒で `PUT /api/videos/{id}/thumbnail-position` に送る。再生中でも一時停止中でも 1 回の操作で済み、時刻は入力しない (`UI品質` の「操作の優先度」と plan の受け入れ条件)。再生は止まらない。
- 押せるのは次のときだ。プレーヤーが表示されている (`showPlayer`)。この動画の最初のフレームが読み込まれ、論理的な再生位置が定まっている (最初の `loadedmetadata` の後で、再開位置を適用した後。現在の `PlayerStatus` にはそうした合図がないので、implement で `VideoPlayer` が画面に伝える手段を加える)。状態のレイヤー (読み込みの失敗、取り込み中、読み取りの失敗、再生できない、再生の失敗) も再生終了のレイヤー (次の動画の通知を含む) も絵を覆っていない。それ以外では `aria-disabled` (`opacity-50`) にする。最初の読み込みの前は、画面にまだフレームがなく、位置は 0 か再開位置を適用する前の値なので、利用者が見ているフレームと違う値を送ってしまう。レイヤーが絵を覆っている間は「いま表示しているフレーム」がなく、終了時の位置は長さ以上で拒否される ([contracts/screen-api.md, `PUT /api/videos/{id}/thumbnail-position`](contracts/screen-api.md#put-apivideosidthumbnail-position))。定まった後は、データを待つ間の読み込みのスピナー、再接続、中央のタッチ操作は絵を覆わず位置を保つので、ボタンは押せるままだ。
- 送信中 (数秒。応答は生成が終わってから返る) は、アイコンが `LoaderCircle` (回転) になり、`aria-disabled` になる。もう一度押しても何も送らない。別のタブや外部 API との競合は、サーバーの「最後に記録した位置の画像が残る」で扱う ([contracts/screen-api.md, `PUT /api/videos/{id}/thumbnail-position`](contracts/screen-api.md#put-apivideosidthumbnail-position))。
- `200` では、応答の `Video` で動画を置き換える。項目 (前の「項目」を参照) が現れるか、その位置と画像が更新される。トーストは出さない。項目の変わった画像と位置が結果である (受け入れ条件 5)。一覧のカードは `video` の通知を通して新しい `thumbnailUrl` を読む。
- `thumbnailState` が `pending` の動画 (取り込みのジョブがまだ作っていない) でも押せる。生成はリクエストの中で行われ、応答は `done` を持つ。

### 失敗 {#failure}

- 設定でも解除でも、失敗すると詳細の行のすぐ下 (技術的な詳細の行の上。開けなかったときの行と同じ場所) に 1 行を出す。`role="alert"`、`text-sm text-danger`、先頭に `AlertCircle` (`size-4`) で、文言は「Couldn't change the thumbnail: {reason}」だ。理由は `errorText` だ (前の「文言」を参照)。
- 前の項目 (位置と画像) は残す。`thumbnail_frame_unavailable` では、サーバーも前の画像を残す (境界条件「生成が失敗する」、[contracts/screen-api.md, `PUT /api/videos/{id}/thumbnail-position`](contracts/screen-api.md#put-apivideosidthumbnail-position))。
- 開けなかったときの行と同時には出さず、後のものが前のものを置き換える (この場所は一度に 1 行を持つ)。次の設定、解除、開く操作、または別の動画への移動で消える。
- `404 video_not_found` と `file_unavailable`: 行を出し、動画を取り直す。

## レスポンシブ動作 {#responsive-behaviour}

Tailwind の標準ブレークポイントだけを使い、CSS で切り替える (library-ui.md 4)。判定する幅は 360px、768px、1280px (`lg` の 2 列) だ。

| 幅 | タイトルと編集ボタン | 編集中 | ファイル名の行 | サムネイルの項目と操作 |
| --- | --- | --- | --- | --- |
| 1280px | `text-2xl` のタイトルの右上に `h-8` のボタン | 入力欄、Save、Cancel を 1 行に | `text-sm` | 長さ、サイズ、追加日、サムネイルを 1 行に。右端に 3 つの操作 |
| 768px | 上と同じ | 上と同じ | `text-sm` | 上と同じ (1 行に収まる) |
| 360px | `text-xl` のタイトルの右上に `h-8` のボタン。タイトルは折り返す | 1 行目に入力欄。2 行目に Save と Cancel を左寄せで | `text-xs`、1 行に切り詰め | 項目は折り返し、右端の操作は最後の行の右に移る。横スクロールはない |

- 編集ボタンは `shrink-0` で、長いタイトルでも押せる大きさ (`h-8 w-8`) のまま右上にとどまる。
- 長い表示名 (境界条件): タイトルは今と同じく折り返して全体を出し (`[overflow-wrap:anywhere]`)、一覧のカードは現在の 2 行の切り詰め (`line-clamp-2`) と `title` で収める。カードは変わらない。

## 確認の基準 {#review-criteria}

実機で見て判定する (library-ui.md 5)。「ある」だけでは合格にしない (Q-4)。

1. **視覚的な階層**: 再生画面を開くと、目はプレーヤー → タイトル → タグと進み、編集ボタンとカメラのボタンはその後に目に入る。どちらも「Open file」や「Copy path」と同じ重みで、アクセントの色も面も枠もない。ファイル名の行はタイトルの下ではっきり小さく薄く、タイトルより先に読まれない。表示名のある動画とない動画を並べて開いても、タイトルの大きさ、太さ、色は違わない (`UI品質` の「文字の組み方」)。
2. **情報の密度**: 表示名のない動画のタイトルの周りは、編集ボタン 1 つを除いて今と同じで、ラベル、説明の文、枠は加わらない。表示名のある動画では、ファイル名の行だけが加わる。サムネイルを選んでいない動画の詳細の行は、カメラのボタン 1 つを除いて今と同じだ (`UI品質` の「見出しの情報を増やさない」)。
3. **余白のリズム**: タイトルとタグの間、タグと公開範囲の切り替えの間、そのまとまりと詳細の行の間の間隔は、この機能の前と同じだ。ファイル名の行はタイトルの近く (タグより近く) にあり、編集中の入力欄の文字の左端はタイトルの左端に一致する。プレーヤーとタイトルの間の間隔は変わらない (`UI品質` の「余白のリズム」)。
4. **文字の組み方**: 編集を始めてもタイトルの文字の大きさ、太さ、行の高さは保たれるので、入力欄への変化は面と枠だけで分かる。入力欄を空にすると、ファイル名のタイトルがプレースホルダーとして薄く出る。サムネイルの項目の時刻は `tabular-nums` で、長さの数字と同じ字形を使う。
5. **操作の優先度**: 一時停止してカメラのボタンを 1 回押すと、数秒後に詳細の行の項目にそのフレームの小さな画像と位置が現れる (受け入れ条件 5)。その間も、再生、シーク、音量、タグの操作はすべて働き続ける。時刻を入力する欄はどこにもない。× を 1 回押すと項目が消える (受け入れ条件 6)。タイトルの編集は画面を離れずに Enter で保存できる (受け入れ条件 1)。入力欄を空にして保存すると、タイトルはファイル名のタイトルに戻り、ファイル名の行が消える (受け入れ条件 2)。
6. **見える状態**: 保存中は Save が、設定中はカメラが、解除中は × がスピナーを出し、もう一度押しても二重に送らない。失敗はその場に赤い行を出し (入力欄の下か詳細の行の下)、タイトル、前の画像、前の位置は変わらない (境界条件「長すぎる」と「生成が失敗する」)。トーストもダイアログも出ない。
7. **キーボード**: Tab は編集ボタン → 入力欄 (全選択) → Save → Cancel → タグ → … と進む。Esc は入力欄の中では取り消し、外では画面を閉じる。Space、F、M は入力欄の中では文字を入力し、再生を止めない。
8. **ゲスト**: サインアウトして同じ動画を開くと、タイトルは表示名のままで (受け入れ条件 1)、編集ボタン、ファイル名の行、サムネイルの項目、カメラのボタンのどれもない (受け入れ条件 9)。
9. **要件を満たさない例** (`UI品質`): 編集を設定や別のウィンドウでしか始められない。サムネイルの選択に時刻の入力欄がある。表示名のある動画のカード、フォルダの画面、リスト表示、関連動画、グループのカードのどこかにファイル名が出る。編集やカメラのボタンがアクセントの色や面を持ち、タグや再生の操作より先に目に入る。

## 色 {#colour}

新しい組は使わない。入力欄は `field` の上の `fg` (`pairs` にある)、失敗の行は `bg` の上の `danger` (開けなかったときの行と同じ)、従の行は `bg` の上の `fg-muted` だ。`fg-subtle` は印にだけ使い、文字には使わない。小さな画像の背景の `surface` は画像の代わりで、文字を載せない。

## アクセシビリティ {#accessibility}

親 Issue はスクリーンリーダーやコントラストの設計を求めていないので、名前だけを決める (012 と同じ範囲)。

- 編集ボタンのアクセシブルな名前: 「Edit name」。入力欄のアクセシブルな名前: 「Display name」。
- ファイル名の行: 値の前に視覚的に隠した「File name」を置き、`title` も同じ名前を持つ。
- サムネイルの項目: 視覚的に隠した「Thumbnail at 1:23」と画像の `alt`。× は「Use automatic thumbnail」。カメラは「Use current frame as thumbnail」。
- 2 種類の失敗の行はどちらも `role="alert"` だ。スピナーは `aria-hidden` で、送信中の要素は `aria-disabled` だ。
