# UI Design: 動画ページで表示名を編集し、今の場面を代表サムネイルにする

**Feature**: [parent Issue #517](https://github.com/syudead/vv/issues/517) ・
[plan.md](plan.md) ・ [contracts/screen-api.md](contracts/screen-api.md) ・
[research.md R-4](research.md#r-4-サムネイルの位置の指定は要求の中で生成してから記録する)・
[R-8](research.md#r-8-画面の-api-は動画ごとの-2-つの-put-にし空の表示名と-null-の位置が解除である)・
[R-10](research.md#r-10-表示名の規則はタグ名の規則にそろえ上限は-200-符号位置にする)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・再生画面の構成: [ライブラリ UI](../../docs/design-docs/library-ui.md)
  （「6. 一覧の構成」のカード、「8. 再生画面の構成」）
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`。値は写さず、名前で呼ぶ
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- 再生画面の列・題名の書式・「Video facts」の 2 行と右端の操作・失敗の 1 行:
  [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md)
  と今の [`web/src/player/VideoPage.tsx`](../../web/src/player/VideoPage.tsx)・
  [`VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx)
- 題名とタグのまとまり・「タグを追加」の入力とキー操作:
  [specs/014-video-tags/ui-design.md「Video page tags」](../014-video-tags/ui-design.md#video-page-tags)
- 題名の上のグループ名の行（題名より従の 1 行の書式）:
  [specs/017-folder-groups/ui-design.md「Group line」](../017-folder-groups/ui-design.md#group-line)
- 公開の切り替え（送信中・失敗の行の扱い）:
  [specs/016-single-account-auth/ui-design.md「Visibility toggle」](../016-single-account-auth/ui-design.md#visibility-toggle)
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足すのは、再生画面（`/videos/:id`）の**所有者**だけの 3 つである。

1. 題名の**編集**（要件 1・2、受け入れ条件 1・2）
2. 表示名が設定されているときの、題名の下の**ファイル名の行**（要件 7、受け入れ条件 7）
3. ファイルの情報の行の**サムネイルの項目と操作**（要件 5・6、受け入れ条件 5・6）

それ以外は変えない。一覧のカード・フォルダ画面・リスト表示・関連動画・グループのカードは `title` を
描くだけなので、表示名は API の `title` に載って自然に出る（要件 1、[contracts/screen-api.md §0](contracts/screen-api.md#0-video-の差分)）。
カードにファイル名は出ない（UI品質「要求を満たしたことにならない例」）。ゲストの再生画面は今の
ままで、3 つのどれも出ない（要件 9）。プレイヤーの中（操作バー・層）は変えない。新しい色・半径・影の
token は足さず、`tokens.test.ts` の `pairs` にも足さない（下の「Colour」）。

## Why here and not elsewhere

- 題名の編集を**その場**（`h1` の位置）で行い、設定画面や別の窓へ移さないのは、要件「動画ページから
  移動せずに完了する」と、比較対象の現行製品（ストレージ・写真・動画管理の画面の名前の変更）が
  収束している「名前の場所で直す」形に従うためである。ダイアログにすると題名の文字の大きさで
  確かめられず、狭い幅ではプレイヤーが隠れる。
- 「今の場面をサムネイルにする」を**プレイヤーの操作バーではなく**、ファイルの情報の行の右端の
  操作（「ファイルを開く」「パスをコピー」の隣）に置くのは次の理由による。
  - 操作バーの部品は再生操作と同じ重さになる。編集は「再生操作より目立たせない」（UI品質「視覚的
    階層」「操作の優先順位」）。
  - 操作バーは 360px でも 1 行に収める前提で、画質・字幕・速度を足した今、もう 1 つ足す余地が無い
    （specs/027 の「Responsive behaviour」）。
  - サムネイルを場面から選ばせる現行製品（動画の公開サービスの「このフレームを使う」）は、
    プレイヤーの下にその操作を置いている。
  - 指定の結果と解除の入口（下の「Thumbnail fact」）も同じ行にまとまり、状態・操作・失敗が
    1 か所で読める。
- ファイル名の行を**表示名が設定されているときだけ**出すのは、UI品質「情報密度」（元のファイル名は
  表示名があるときだけ、題名より従の扱いで見られればよい）による。行があること自体が「この動画は
  上書きされている」の印になる（要件 7）。

## Words

| 場所 | 英語（案） |
| --- | --- |
| 題名の横の編集ボタンの読み上げ名・ツールチップ | Edit name |
| 編集の入力の読み上げ名 | Display name |
| 編集の入力のプレースホルダー | ファイル名由来の題名そのもの（利用者のデータ。翻訳しない） |
| 保存 | Save |
| 取り消し | Cancel |
| ファイル名の行の読み上げ用のラベル・`title` | File name |
| サムネイルの項目の読み上げ用のラベル・`title`・画像の `alt` | Thumbnail at 1:23（時刻は `formatDuration` の書式） |
| サムネイルを指定するボタンの読み上げ名・ツールチップ | Use current frame as thumbnail |
| 指定を解除する × の読み上げ名・ツールチップ | Use automatic thumbnail |
| 名前を保存できなかった 1 行 | Couldn't save the name: {理由} |
| サムネイルを指定・解除できなかった 1 行 | Couldn't change the thumbnail: {理由} |
| `display_name_control_characters` | The name can't contain control characters |
| `display_name_too_long` | The name can't be longer than {limit} characters |
| `duration_unknown` | The video's length isn't known yet |
| `thumbnail_position_out_of_range` | The position is past the end of the video |
| `thumbnail_frame_unavailable` | No image could be made from this frame |

- 理由の 5 つは `web/src/i18n/errors.ts` の `reason` の表に足す（[contracts/screen-api.md §3](contracts/screen-api.md#3-足す-codereason)）。
  `file_unavailable`・`video_not_found` は今の文のまま。
- 「Thumbnail」と「name」の語は、一覧のカードと同じもの（サムネイル）・題名の編集（名前）を指す。
  「display name」という語は入力の読み上げ名にだけ使い、画面に見える文言には出さない。所有者に
  とっては「動画の名前」であり、「表示名」と「ファイル名」の区別はファイル名の行が見せる。

## Title editing

### Placement and weight

- 題名（`h1`）の右に、編集の入口として `IconButton`（`size="sm"`、ghost、lucide `Pencil`）を 1 つ
  置く。`h1` と同じ行に `flex items-start gap-2` で並べ、`h1` は `min-w-0 flex-1`、ボタンは
  `shrink-0`。ボタンの縦の中心は題名の**1 行目**にそろえる（`h-8` のボタンを `text-2xl`・
  `leading-snug` の 1 行目に合わせるため、`sm` 以上は `mt-0.5`、それ未満は `-mt-0.5` 程度）。
  題名が折り返しても、ボタンは右上に留まり、下へ落ちない。
- 色は「ファイルを開く」「パスをコピー」と同じ `text-fg-muted`、hover で `text-fg`。面も枠も
  無い。アクセント色は使わない。題名（`text-xl`〜`text-2xl`・`font-semibold`・`text-fg`）より
  先に目に入らず、タグのチップ（`bg-elevated` の面を持つ）より弱い（UI品質「視覚的階層」）。
- 常に見せる。ポイントしたときだけ出す形にしないのは、hover の無い端末で入口が消えるからである。
  ゲストには置かない（`h1` だけ。今のまま）。
- 題名の書式は変えない。表示名も、ファイル名由来の題名と同じ `h1` の書式で出る（UI品質
  「タイポグラフィ」）。

### Edit mode

編集ボタンを押すと、`h1` の位置がそのまま**入力**に変わる（編集中は `h1` を描かない）。

- 入力は `input[type=text]`。文字の大きさ・太さ・行間は題名と同じ（`text-xl`〜`text-2xl`・
  `font-semibold`・`text-fg`・`leading-snug`）。面は `bg-field`、枠は `border border-border`、
  `rounded-md`、`px-2 py-1`。文字の左端を題名の左端にそろえるため `-mx-2`（グループ名の行の
  `-ml-2` と同じ考え方）。フォーカスで `border-accent`（検索欄・「タグを追加」と同じ）。幅は
  列いっぱい（`w-full`）。
- 初期値は今の `title`（表示名があればそれ、無ければファイル名由来の題名）。全選択した状態で
  フォーカスを入力に移す。プレースホルダーは `fileTitle` で、入力を空にすると「空 ＝ ファイル名に
  戻る」がその場で見える（説明の文は足さない）。
- 入力の右（`sm` 未満は下）に、**Save**（`Button` primary・`sm`）と **Cancel**（ghost・`sm`）を
  `gap-2` で置く。編集中だけ現れる一時の状態なので、Save のアクセント色は「編集を再生操作より
  目立たせない」に反しない。`sm` 未満では入力が 1 行目、2 つのボタンが 2 行目の左寄せ。
- 編集中は、題名の下のファイル名の行（下の「File name line」）を出さない。入力のプレースホルダーが
  その役を負う。タグ・公開の切り替え・ファイルの情報は編集中もそのまま見える。
- キー: Enter で保存、Esc で取り消し。入力にフォーカスがある間は再生画面のキーの操作（Space・
  F・M・C・0・Esc）は効かない（今の `keyboard.ts` の `isEditable`。014「Add input」と同じ）。
  画面を閉じるのは、入力の外で押した Esc だけである。
- 取り消し（Cancel・Esc）は何も送らず、`h1` に戻し、フォーカスを編集ボタンへ返す。

### Save

- 保存は `PUT /api/videos/{id}/display-name` を 1 回送る（[contracts/screen-api.md §1](contracts/screen-api.md#1-put-apivideosiddisplay-name)）。
  - 入力の値が今の `title` と同じで、表示名が設定されていないときは、何も送らずに取り消しと同じに
    する（ファイル名と同じ表示名を作らない）。
  - 空（空白だけを含む）は解除として送る（`displayName: ""`。Edge Case「空文字や空白だけ」）。
    表示名が設定されていない動画で空を送っても、応答は `200` で何も変わらない。
- 送信中は入力を `readOnly`、Save を `aria-disabled`（`disabled` にしない。フォーカスを失わない
  ため。016「Visibility toggle」と同じ）にし、Save のアイコンを lucide `LoaderCircle`
  （`animate-spin motion-reduce:animate-none`）にする。Cancel は押せるままにし、押したら応答を
  待たずに `h1` へ戻す（届いた応答は動画の差し替えにだけ使う）。
- `200` を受けたら、応答の `Video` で `useVideoDetail` の動画を差し替え、`h1` に戻す。題名・
  ファイル名の行・`document.title` が新しい値になる。フォーカスは編集ボタンへ返す。トーストは
  出さない。題名そのものが変わることが結果である（受け入れ条件 1・2）。
- 失敗したら編集を続ける。入力の文字は残し、入力とボタンの行の**すぐ下**に 1 行
  （`role="alert"`、`text-sm text-danger`、先頭に lucide `AlertCircle` `size-4`）で
  「Couldn't save the name: {理由}」を出す（012「Video facts」の開けなかったときの行と同じ形）。
  理由は `errorText`（上の「Words」）。次に保存したとき、取り消したとき、別の動画へ移ったときに
  消える。帯・トースト・ダイアログは使わない。
  - `display_name_too_long`（Edge Case「長すぎる」）: 理由に `limit` の 200 が入る。入力に
    `maxLength` は付けない。符号位置の数え方がブラウザと違い、貼り付けた文字が黙って切れる
    からである。
  - `404 video_not_found`: 動画を取り直す（消えていれば今の「動画が消えた」の層になる）。
- 編集中に `/api/events` の `video` で動画が取り直されても（別のタブや外部 API からの変更。
  Edge Case「同時に変更」）、入力の文字は書き換えない。保存すればこちらが後勝ちになり、
  取り消せば取り直した題名が見える。

## File name line

表示名が設定されている（`displayName` がある）動画で、所有者にだけ、題名のすぐ下に 1 行置く
（要件 7、受け入れ条件 7）。

- 中身は左から、lucide `FileVideo`（`size-3.5`、`text-fg-subtle`、`aria-hidden`）→ `fileTitle`
  （`text-fg-muted`、1 行で省略、`title` に「File name: 〈ファイル名〉」）。全体は `text-xs`
  （`sm` 以上 `text-sm`）。グループ名の行と同じ書式で、題名より小さく従の色（UI品質
  「タイポグラフィ」「元のファイル名は補助情報の扱い」）。読み上げ用に視覚的に隠した「File name」を
  値の前に置く。
- `h1`（と編集ボタン）の行とこの行を `flex flex-col gap-1` の 1 つの小さなまとまりにし、その
  まとまりを今の題名とタグのまとまり（`gap-2`）の中に置く。題名とファイル名の間（`gap-1`）を
  題名とタグの間（`gap-2`）より狭くして、ファイル名が題名に属する補足で、タグと同格でないことを
  間隔で示す。まとまりの外側の間隔（`gap-2`・`gap-5`）は変えない（UI品質「余白のリズム」）。
- 押せない。リンクにもボタンにもしない（Q-5: 裏側が無い要素だが、文字色が `fg-muted` で印も
  hover の面も無く、押せると誤認する要素は無い。価値は要件 7 そのもの）。
- 表示名が無い動画には出さない。ゲストには出さない（ゲストの応答に `displayName`・`fileTitle` は
  無い）。
- 広い画面（`lg` 以上）の左の列の高さの予算（プレイヤーの `max-w-[calc(max(100dvh-17rem,15rem)*…)]`）
  は変えない。この 1 行で列がはみ出す窓では、グループ名の行と同じく左の列がスクロールする
  （012「`lg`（1024px）以上」）。

## Thumbnail fact

ファイルの情報の行（`VideoFacts` の 1 行目）に、所有者だけ、次の 2 つを足す。

### Item（指定されているときだけ）

`thumbnailPositionMs` があるときだけ、長さ・サイズ・追加日の**あと**に 4 つ目の項目として置く。

- 中身は左から、lucide `Image`（`size-4`、`text-fg-subtle`、`aria-hidden`）→ 今の代表サムネイルの
  小さな画像（`img`、`thumbnailUrl`、高さ `h-6`、16:9 の幅、`rounded-sm`、`overflow-hidden`、
  `bg-surface`、`object-cover`）→ 位置（`formatDuration(thumbnailPositionMs)`、`tabular-nums`）→
  解除の ×。項目の中は `gap-1.5`、他の項目との間は今の `gap-x-4`（`sm` 以上 `gap-x-5`）。
- 小さな画像を置くのは、「指定した場面の画像になった」ことをこの画面で確かめられるようにする
  ためである。一覧のカードは戻らないと見えず、位置の数字だけでは黒い場面を選んだことに
  気づけない。画像の `alt` は「Thumbnail at 1:23」。読み込めない間・失敗したときは `bg-surface`
  の空の箱のままにする。
- 時刻に `Clock` の長さと見分けが付くのは、隣に画像と `Image` の印があるためである。文字の
  ラベルは他の項目と同じく出さず、視覚的に隠した「Thumbnail at 1:23」と `title` で持つ。
- 解除の ×: `size-6` の正方形のボタン、lucide `X`（`size-3`）、`text-fg-muted`、hover で
  `text-fg`（タグのチップの × と同じ大きさ）。読み上げ名は「Use automatic thumbnail」。押すと
  `PUT /api/videos/{id}/thumbnail-position` に `positionMs: null` を送る。送信中は × を
  `LoaderCircle`（回転）にし、`aria-disabled`。`200` で項目が消え、画像は自動の位置に戻る
  （受け入れ条件 6）。
- 幅が足りなければ行の他の項目と同じく折り返す。

### Capture button

- 情報の行の右端の操作の一群（`ml-auto`）の**先頭**（「ファイルを開く」の左）に、`IconButton`
  （`size="sm"`、ghost、lucide `Camera`、`text-fg-muted`、hover で `text-fg`）を置く。読み上げ名・
  ツールチップは「Use current frame as thumbnail」。「ファイルを開く」「パスをコピー」の 2 つは
  隣り合ったまま右へ寄る。所在の無い動画（右の一群が今は無い）でも、所有者でプレイヤーが
  出ていればこのボタンだけの一群を置く。
- 押すと、押した瞬間のプレイヤーの論理上の再生位置（再生位置の保存が読むのと同じ値）を
  ミリ秒で `PUT /api/videos/{id}/thumbnail-position` に送る。再生中でも一時停止中でも 1 回の
  操作で済み、時刻を入力させない（UI品質「操作の優先順位」、plan の受け入れ条件）。再生は
  止めない。
- 押せる条件: プレイヤーが出ていて（`showPlayer`）、映像の上に状態の層（読み込み失敗・取り込み中・
  読み取り失敗・再生できない・再生失敗）も再生終了の層（と次の予告）も無いとき。それ以外は
  `aria-disabled`（見た目は `opacity-50`）にする。層が映像を覆っている間は「今表示している場面」が
  無く、再生終了の位置は尺以上で受け付けられない（契約 §2）からである。データ待ちの読み込み中の
  輪・再接続中・タッチの中央操作は映像を覆わないので押せる。
- 送信中（数秒。生成が終わるまで応答が返らない）はアイコンを `LoaderCircle`（回転）にし、
  `aria-disabled`。もう一度押しても送らない。別のタブや外部 API との競合は、サーバーの「最後に
  記録した位置の画像が残る」（契約 §2）が扱う。
- `200` を受けたら応答の `Video` で動画を差し替える。指定の項目（上の「Item」）が現れる、または
  位置と画像が新しくなる。トーストは出さない。項目の画像と位置が変わることが結果である
  （受け入れ条件 5）。一覧のカードは `video` 通知で新しい `thumbnailUrl` を読む。
- `thumbnailState` が `pending` の動画（取り込みの job がまだ作っていない）でも押せる。生成は
  要求の中で行われ、応答で `done` になる。

### Failure

- 指定・解除のどちらも、失敗したら情報の行のすぐ下（技術情報の行の上。開けなかったときの行と
  同じ場所）に 1 行出す。`role="alert"`、`text-sm text-danger`、先頭に `AlertCircle`（`size-4`）、
  文は「Couldn't change the thumbnail: {理由}」。理由は `errorText`（上の「Words」）。
- 前の項目（位置と画像）は残す。`thumbnail_frame_unavailable` ではサーバーも前の画像を残す
  （Edge Case「生成に失敗した場合」、契約 §2）。
- 開けなかったときの行と同時には出さず、後から起きたものが前の行を置き換える（この場所は
  一度に 1 行）。次に指定・解除・開く操作をしたとき、または別の動画へ移ったときに消える。
- `404 video_not_found`・`file_unavailable`: 行を出したうえで動画を取り直す。

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md 4）。判定する幅は
360px・768px・1280px（`lg` の 2 列）。

| 幅 | 題名と編集ボタン | 編集中 | ファイル名の行 | サムネイルの項目と操作 |
| --- | --- | --- | --- | --- |
| 1280px | `text-2xl` の題名の右上に `h-8` のボタン | 入力・Save・Cancel が 1 行 | `text-sm` | 長さ・サイズ・追加日・サムネイルが 1 行、右端に 3 つの操作 |
| 768px | 同上 | 同上 | `text-sm` | 同上（1 行に収まる） |
| 360px | `text-xl` の題名の右上に `h-8` のボタン。題名は折り返す | 入力が 1 行目、Save・Cancel が 2 行目の左寄せ | `text-xs`、1 行で省略 | 項目は折り返し、右端の操作は最後の行の右に寄る。横スクロールは出ない |

- 編集ボタンは `shrink-0` で、長い題名でも押せる大きさ（`h-8 w-8`）のまま右上に残る。
- 表示名が長いとき（Edge Case）: 題名は今と同じく折り返して全体を出し（`[overflow-wrap:anywhere]`）、
  一覧のカードは今の 2 行の省略（`line-clamp-2`）と `title` で収まる。カードは変えない。

## Review criteria

判定は実機で見て行う（library-ui.md 5）。「ある」だけでは満たさない（Q-4）。

- **視覚的階層**: 再生画面を開いて、プレイヤー → 題名 → タグ の順に目が行き、編集ボタンと
  カメラのボタンはそのあとで気づく。どちらも「ファイルを開く」「パスをコピー」と同じ重さで、
  アクセント色・面・枠が無い。ファイル名の行は題名の下で明らかに小さく薄く、題名より先に
  読まれない。表示名を設定した動画と設定していない動画を並べて開いたとき、題名の大きさ・太さ・
  色に違いが無い（UI品質「タイポグラフィ」）。
- **情報密度**: 表示名の無い動画の題名の周りは、編集ボタン 1 つ以外は今と同じで、ラベル・
  説明の文・枠が増えていない。表示名のある動画で増えるのはファイル名の 1 行だけ。指定していない
  動画の情報の行は、カメラのボタン 1 つ以外は今と同じ（UI品質「ヘッダの情報量を増やさない」）。
- **余白のリズム**: 題名とタグの間・タグと公開の切り替えの間・そのまとまりと情報の行の間が、
  この feature の前と同じ。ファイル名の行は題名に寄り（タグより近い）、編集中の入力の文字の
  左端は題名の左端と同じ。プレイヤーと題名の間隔は変わらない（UI品質「余白のリズム」）。
- **タイポグラフィ**: 編集を始めても文字の大きさ・太さ・行間が題名と同じで、入力に変わった
  ことは面と枠だけで分かる。入力を空にしたとき、プレースホルダーにファイル名由来の題名が薄く
  見える。サムネイルの項目の時刻は `tabular-nums` で、長さの数字と同じ字形。
- **操作の優先順位**: 一時停止してカメラのボタンを 1 回押すだけで、数秒後に情報の行の項目に
  その場面の小さな画像と位置が出る（受け入れ条件 5）。その間、再生・シーク・音量・タグの操作は
  すべてそのまま使える。時刻を入力する欄はどこにも無い。× 1 回で項目が消える（受け入れ条件 6）。
  題名の編集は Enter で保存され、画面を離れない（受け入れ条件 1）。入力を空にして保存すると
  題名がファイル名由来に戻り、ファイル名の行が消える（受け入れ条件 2）。
- **状態の見え方**: 保存中は Save に回転の印、指定中はカメラが回転の印、解除中は × が回転の印に
  なり、押し直しても二重に送られない。失敗は赤い 1 行がその場（入力の下、または情報の行の下）に
  出て、題名・前の画像・前の位置は変わらない（Edge Case「長すぎる」「生成に失敗」）。トースト・
  ダイアログは出ない。
- **キーボード**: Tab で編集ボタン → 入力（全選択）→ Save → Cancel → タグ → … と進み、Esc は
  入力の中では取り消し、外では画面を閉じる。Space・F・M は入力の中では文字入力になり、再生を
  止めない。
- **ゲスト**: ログアウトして同じ動画を開くと、題名が表示名のまま（受け入れ条件 1）、編集ボタン・
  ファイル名の行・サムネイルの項目・カメラのボタンのどれも無い（受け入れ条件 9）。
- **要求を満たしたことにならない例**（UI品質）: 編集が設定画面や別の窓でしか始められない。
  サムネイルの指定に時刻の入力欄がある。表示名を設定した動画のカード・フォルダ画面・
  リスト表示・関連動画・グループのカードのどこかにファイル名が出る。編集ボタンやカメラの
  ボタンがアクセント色や面を持ち、タグや再生操作より先に目に入る。

## Colour

新しく使う組は無い。入力は `fg` on `field`（`pairs` にある）、失敗の行は `danger` on `bg`
（開けなかったときの行と同じ）、従の行は `fg-muted` on `bg`。`fg-subtle` は印だけに使い、文字には
使わない。小さな画像の下地 `surface` は画像の代わりで、文字を載せない。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前だけを決める（012 と同じ範囲）。

- 編集ボタンの読み上げ名: 「Edit name」。入力の読み上げ名: 「Display name」。
- ファイル名の行: 視覚的に隠した「File name」を値の前に置き、`title` にも同じ名前を入れる。
- サムネイルの項目: 視覚的に隠した「Thumbnail at 1:23」と、画像の `alt`。× は「Use automatic
  thumbnail」。カメラは「Use current frame as thumbnail」。
- 失敗の 2 種類の行は `role="alert"`。回転の印は `aria-hidden`、送信中の要素は `aria-disabled`。
