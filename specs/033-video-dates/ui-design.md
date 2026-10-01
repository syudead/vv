# UI Design: 動画の更新日時とファイルの作成日時を持ち、表示・並び替え・外部 API で使う

**Feature**: [parent Issue #630](https://github.com/syudead/vv/issues/630) ・
[plan.md](plan.md) ・ [contracts/screen-api.md](contracts/screen-api.md) ・
[research.md R-7](research.md#r-7-応答の項目は-updatedatvv-上の更新日時と-filecreatedat所在の作成日時で画面と外部連携で同じ名前にする)・
[R-8](research.md#r-8-再生画面はタグと公開の設定を変えたあと動画を取り直し新しいドメインイベントは足さない)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け・ツールバーと再生画面の構成: [ライブラリ UI](../../docs/design-docs/library-ui.md)
  （「6. 一覧の構成」のツールバー、「8. 再生画面の構成」）
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`。値は写さず、名前で呼ぶ
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- 「Video facts」の 2 行（項目の形・アイコン・読み上げ名・`title`・折り返し）:
  [specs/012-video-detail-ia/ui-design.md「Video facts」](../012-video-detail-ia/ui-design.md#video-facts)
  と今の [`web/src/player/VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx)
- 情報の行に項目を足した先例（サムネイルの項目・バージョンの項目）:
  [specs/029-video-overrides/ui-design.md「Thumbnail fact」](../029-video-overrides/ui-design.md#thumbnail-fact)、
  [specs/030-video-versions/ui-design.md「Versions fact」](../030-video-versions/ui-design.md#versions-fact)
- 並べ替えのメニュー・向きの切り替え・`md` 未満のまとめ:
  [specs/013-library-search/ui-design.md「Sort and direction」](../013-library-search/ui-design.md#sort-and-direction)
  と今の [`web/src/videoList/SortControls.tsx`](../../web/src/videoList/SortControls.tsx)・
  [`listCriteria.ts`](../../web/src/videoList/listCriteria.ts)
- ゲストの並べ替え（「最近再生した順」を除く形）:
  [specs/016-single-account-auth/ui-design.md「Guest degradation」](../016-single-account-auth/ui-design.md#guest-degradation)
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は意図を
  示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる

この feature が画面に足すのは次の 2 つで、どちらも新しい色・半径・影のトークンを足さない。

1. 再生画面（`/videos/:id`）の情報の行の**更新日時と作成日時の項目**（要件 4、受け入れ条件 1〜5）
2. ライブラリとフォルダ画面のツールバーの並べ替えの**「作成日」**（要件 5、受け入れ条件 6・8）

一覧のカード・リスト表示の行・フォルダのカード・グループのカード・関連動画の行は変えない（対象外
「一覧のカードに更新日時や作成日時を表示すること」）。既存の「更新日」の並べ替えは名前・アイコン・
向き・言い換えのどれも変えない（要件 7）。所有者とゲストで出すものは同じである（R-7。追加日と
同じく公開の動画の事実）。

## Words

英語の文言は、この画面で既に使っている語との衝突だけで決める。

| 日時 | 情報の行の名前 | 並べ替えの名前 | 理由 |
| --- | --- | --- | --- |
| 追加日（今のまま） | Added | Date added | 変えない |
| 更新日時（vv 上の編集） | **Edited** | — | 下の説明 |
| ファイルの更新日時（今のまま） | — | Date modified | 変えない（要件 7） |
| 作成日時（ファイル） | **Created** | **Date created** | 下の説明 |

- 更新日時を **Edited** にし、「Updated」にしないのは、同じ画面の語との衝突を避けるためである。
  並べ替えの「Date modified」はファイルの mtime で、英語では modified と updated が同じ意味に
  読める。上部バーの「更新」（取り込み）も、API の `updatedAt` を素直に写した「Updated」と同じ
  語になる。所有者が表示名・タグ・公開の設定・代表サムネイルを**編集した**日時であることを、
  題名の編集ボタン（lucide `Pencil`、029）と同じ語で言う。親 Issue の「UI品質」が求める
  「追加日・更新日時・作成日時の違いが言葉で区別できる」は、Added / Edited / Created の 3 語が
  互いに同じ意味に読めないことで満たす。API の名前は R-7 のまま `updatedAt` で、画面の語とは
  別に決めてよい（API は親 Issue の語を写し、画面は同じ画面の他の語との区別を優先する）。
- 作成日時を **Created** / **Date created** にするのは、ファイルの管理画面（Finder・Explorer）が
  「Date added」「Date modified」「Date created」の 3 語に収束しており、そこでの意味（追加・
  ファイルの更新・ファイルの作成）が vv の意味と同じだからである。並べ替えの「Date created」と
  「Date modified」は語が違うので、親 Issue の「UI品質」が求める「作成日と更新日（ファイル）の
  違いが名前で分かる」を満たす。「File created」のようにファイルを名前に出す案は採らない。
  「Date modified」がファイルを名前に出していないので、2 つの間で揃わず、かえって
  「modified は ファイルではない」と読ませる。
- 向きの言い換えは追加日・更新日と同じ「oldest first / newest first」。

## Video facts

ファイルの情報の行（`VideoFacts` の 1 行目、`ul`「File details」）に、追加日の**あと**に 2 つの項目を
足す。所有者にもゲストにも出す。行の順は次のとおりで、長さ・サイズ・追加日の 3 つの位置と形は
変えない（親 Issue「今ある長さ・サイズ・追加日と並べて」）。

1. 長さ（`Clock`、今のまま）
2. サイズ（`HardDrive`、今のまま）
3. 追加日（`CalendarPlus`、今のまま）
4. **更新日時**（Edited）
5. **作成日時**（Created）
6. バージョンの項目（030、あるときだけ）
7. サムネイルの項目（029、所有者で指定されているときだけ）

- 項目の形は既存の `Fact` と同じ: アイコン（`size-4`、`text-fg-subtle`、`aria-hidden`）→ 視覚的に
  隠した名前 → 値。文字は行と同じ `text-sm text-fg-muted tabular-nums`、項目の中は `gap-1.5`、項目の
  間は `gap-x-4`（`sm` 以上 `gap-x-5`）。ラベルの文字・区切り線・枠・アクセント色は使わない
  （親 Issue「今の追加日の表示と同じ形式で並ぶ」、UI品質「視覚的階層」）。
- **値は日付だけ**（`formatDate`。例 `Sep 27, 2026`）で、3 つの日付が同じ書式で並ぶ。
  日時まで出す案は採らない。同じ行の長さ・サイズと並ぶ段で時刻が付くと、その項目だけが
  長く・重くなり、「追加日と同じ形式」から外れる。
- **時刻は `title` で読める**。日付の 3 項目（追加日・更新日時・作成日時）の `title` は
  「名前 + 日時」（例 `Edited Sep 27, 2026, 3:04 PM`、`formatDateTime`）にする。今の追加日の
  `title` は名前だけなので、追加日もこの形に揃える。同じ日の中で編集したときに更新日時が
  進んだことを、画面を見て確かめられるようにするためである（受け入れ条件 1）。
  読み上げ名（隠した名前）は今と同じ名前だけで、値は後ろの文字で読まれる。
- **アイコン**は 3 つの日付で違う絵にし、名前を読まなくても見分けが付くようにする
  （UI品質「見た目でも区別できる」）。
  - 追加日: `CalendarPlus`（今のまま）
  - 更新日時: lucide `PencilLine`。題名の編集ボタンの `Pencil` と同じ家族で、「所有者が直した」を
    示す。カレンダーの家族（`CalendarCog` など）にしないのは、追加日の `CalendarPlus` と並べた
    とき小さな印の違いだけになり、`size-4` では読めないからである。
  - 作成日時: lucide `FileClock`。「ファイル自身の時刻」を示し、サイズの `HardDrive` と同じく
    ファイルの事実であることを絵で言う。長さの `Clock` とは、書類の輪郭の有無で見分けが付く。
    `FilePlus` は追加日の `CalendarPlus` と「+」が重なるので使わない。
- 順を追加日 → 更新日時 → 作成日時にするのは、追加日と更新日時が vv の中の時系列（更新日時は
  追加日以降）で、その 2 つを隣に置くと「一度も編集していない動画は 2 つが同じ日付」
  （受け入れ条件 3）がそのまま読めるからである。作成日時はファイルの事実なので、その後ろに置く。
  作成日時を時系列どおり先頭にする案は、追加日の位置が動き、既存の 3 つの並びを変える。
- 作成日時が取れない環境（mtime の代用）でも、項目の名前・アイコン・形は同じで、代用である
  ことを画面には出さない。見る人にとって値は「ファイルの作成日時として最善のもの」であり、
  印を付けても直す手段が無い（対象外「作成日時を所有者が手で直すこと」）。
- 値が読めない（空）ときは、他の日付と同じく空の値で項目を出す（`formatDate` の既存の扱い）。
  必須の項目なので、通常は起きない。
- 右端の操作（カメラ・「ファイルを開く」「パスをコピー」）、失敗の 1 行、技術情報の行、行の間隔
  （`gap-3`）は変えない。項目が 2 つ増えた分は、他の項目と同じく折り返す（下の「Responsive
  behaviour」）。

### Refresh after edits

受け入れ条件 1 は、編集のあと再生画面の更新日時がその日付になって見えることである。

- 表示名の保存・代表サムネイルの指定と解除は、応答の `Video` で画面の動画を置き換える今の経路で
  更新日時も入れ替わる。
- タグの付け外し・公開の切り替えは、成功したあと動画を取り直す（R-8）。取り直しの間、情報の行は
  前の値のまま置き、スケルトンや空の値には戻さない。更新日時の項目だけが変わるので、行全体を
  消すと変わった場所が分からなくなる。
- 取り直しが失敗したときは、その操作の失敗の行（タグ・公開の切り替えの既存の形）には出さず、
  前の値のままにする。次に動画を開き直すと正しい値になる。

## Sort and direction

並べ替えの種類を 7 つから **8 つ**にする。ライブラリとフォルダ画面は `SortControls` を共有するので、
両方に出る（要件 5）。既存の 7 種の名前・アイコン・向き・言い換えは変えない（要件 7）。

| 種類 | 名前 | アイコン | 選んだときの向き | 昇順／降順の言い換え |
| --- | --- | --- | --- | --- |
| 追加日 | Date added | `CalendarArrowDown` | 降順 | oldest first／newest first |
| 更新日（ファイル） | Date modified | `CalendarClock` | 降順 | oldest first／newest first |
| **作成日** | **Date created** | **`FileClock`** | 降順 | oldest first／newest first |
| 題名 | Title | `ArrowDownAZ` | 昇順 | — |
| 長さ | Length | `Timer` | 降順 | shortest first／longest first |
| ファイルサイズ | File size | `HardDrive` | 降順 | smallest first／largest first |
| 最近再生した順 | Recently played | `History` | 降順 | least recently played／most recently played |
| ランダム | Random | `Shuffle` | — | — |

- **位置**は「Date modified」の直後。日付の 3 種（追加・更新・作成）を隣り合わせ、名前の
  「Date …」で一群に読めるようにする。題名の前に入るので、題名以降の 5 種は 1 つ下がる。
  Finder の並べ替えの順（Added → Modified → Created）とも同じで、既に知っている順を崩さない。
- **アイコン**は情報の行の作成日時と同じ `FileClock`。ファイルサイズの `HardDrive` が情報の行と
  メニューで同じであるのと同じ扱いで、画面をまたいで「同じもの」と分かる。追加日・更新日の
  カレンダーの家族と絵が違うので、`size-4` でも 3 つの日付の種類を見分けられる。
- 選ぶと `createdDesc`、向きの切り替えで `createdAsc`。向きのボタンの読み上げ名とツールチップは
  今の形「Descending (newest first). Press for ascending」。
- 端末の設定（`viewPreferences`）と URL（`sort=createdDesc`・`createdAsc`）の往復は、他の種類と
  同じ。ゲストにも出す（所有者のデータに依らない。R-7）。ゲストのメニューは「最近再生した順」を
  除いた **7 種**になる。
- `md` 未満の「表示と並び順」のまとめでは、2 列のラジオが所有者で 4 行（8 種）、ゲストで 4 行
  （7 種、最後の行は左だけ）になる。「Date created」は「Date modified」の右隣（2 行目の右）に
  並び、3 つの日付が 1 行目の左から 2 行目の右へ続けて読める。向きの `SegmentedControl` は
  今のまま。
- メニューボタンの文字は今と同じく種類の名前だけ（「Date created」）で、向きは出さない。
  ボタンの幅は「Date modified」と同程度で、ツールバーの他の部品の位置を動かさない。

## Responsive behaviour

幅の境界は Tailwind の既定だけを使い、CSS で出し分ける（library-ui.md 4）。判定する幅は
360px・768px・1280px。

| 幅 | 情報の行 | 並べ替え |
| --- | --- | --- |
| 360px（`sm` 未満） | 5 項目が `gap-x-4` で 2〜3 行に折り返す。折り返しは項目の境で起き、項目の中（アイコンと値）は切れない。右端の操作は今と同じく行の末尾に回る。横スクロールは出ない | 「表示と並び順」のまとめの中。2 列 × 4 行のラジオで、「Date created」の文字が 1 行に収まり切れない（`truncate`）ことが無い |
| 768px（`md`） | 5 項目が 1〜2 行。`gap-x-5` | ツールバーのメニューボタンに「Date created」が 1 行で収まり、向きのボタンと接したまま |
| 1280px（`lg` の 2 列） | 左の列の幅で 5 項目（バージョン・サムネイルの項目を含めて 7 項目まで）が 1〜2 行。プレイヤーの幅の上限の計算（`max-w-[calc(…)]`、012）は変えない。情報の行が 2 行になる動画でも、ふだんは左の列がスクロールしない範囲に収まる（行の高さは `text-sm` の 1 行分の増加） | 同上 |

## Review criteria

判定は実機で見て行う（library-ui.md 5）。「ある」だけでは満たさない（Q-4）。

- **視覚的階層（再生画面）**: 動画のページを開いて、プレイヤー → 題名 → タグ → 公開の切り替えの
  順に目が行き、Edited・Created は長さ・サイズ・追加日と同じ強さで、それらより先に読まれない。
  この feature の前の画面と並べたとき、題名・タグ・情報の 2 行の位置と間隔に違いが無く、違いは
  項目が 2 つ多いことだけ（UI品質「今の追加日の表示と同じ形式で並ぶ」）。
- **区別（再生画面）**: 情報の行で、3 つの日付が名前を読まなくても絵で見分けられる（`CalendarPlus`・
  `PencilLine`・`FileClock` の 3 つが `size-4` で互いに違って見える）。hover の `title` で
  「Added …」「Edited …」「Created …」の 3 語が読め、どれが vv の操作でどれがファイルの事実かを
  説明の文無しに言える。同じ日付が 2 つ並ぶ（編集していない動画の Added と Edited）とき、
  日付の書式が同じで、片方だけ時刻が付いたり太字になったりしていない。
- **情報密度（再生画面）**: 情報の行にラベルの文字・区切り線・「(file)」のような補足が増えていない。
  日付は年月日だけで、時刻は `title` にしか無い（UI品質「情報密度」）。
- **余白のリズム**: 項目の間隔（`gap-x-4`／`gap-x-5`）と行の間隔（`gap-3`）が前と同じ。360px で
  折り返した 2 行目・3 行目の左端が 1 行目の左端と揃い、項目の途中で折れていない。
- **タイポグラフィ**: 日付の文字は長さ・サイズと同じ `text-sm text-fg-muted tabular-nums` で、
  3 つの日付の桁が揃って見える。アイコンは `text-fg-subtle` で、文字より薄い。
- **編集の結果**: 表示名を保存すると、画面を離れずに Edited の日付がその日になり、`title` の時刻が
  保存した時刻になる（受け入れ条件 1）。タグの付け外し・公開の切り替え・代表サムネイルの指定と
  解除のあとも同じ。その間、情報の行が消えたり空になったりしない。再生して閉じて開き直しても、
  再スキャンしても Edited は変わらない（受け入れ条件 2）。編集していない動画の Added と Edited が
  同じ日付・同じ `title` の時刻（受け入れ条件 3）。
- **作成日時の値**: 作成日時が取れる環境では、Created の `title` の日時が OS で見えるファイルの
  作成日時と一致し（受け入れ条件 4）、取れない環境では mtime と一致する（受け入れ条件 5）。
  どちらでも項目の見た目は同じ。
- **並べ替えのメニュー**: メニューを開くと「Date added」「Date modified」「Date created」が
  この順に並び、アイコンが 3 つとも違う。「Date created」と「Date modified」のどちらが
  ファイルの作成でどちらがファイルの更新かを、名前だけで言える（UI品質「名前で分かる」）。
  題名以降の順と名前は前と同じ。
- **並べ替えの結果**: 「Date created」を選ぶと一覧が作成日時の新しい順になり、向きのボタンで
  古い順になる（受け入れ条件 6）。ページを読み直しても URL の `sort=createdDesc` で同じ並びに
  戻り、ライブラリとフォルダ画面の両方で選べる（要件 5）。「Date modified」の並びは前と同じ
  （受け入れ条件 8）。グループのカードは、メンバーの最も新しい作成日時の位置に出る。
- **操作の優先順位（一覧）**: 並べ替えの種類が 1 つ増えても、検索・絞り込み・表示倍率・向きの
  ボタンの位置と幅が動かない。メニューを開いて 1 回押すだけで作成日の並びになり、向きは
  その右のボタン 1 回で変わる。
- **ゲスト**: ログアウトして同じ動画を開くと、Edited と Created が所有者と同じ値で出る。
  並べ替えのメニューに「Date created」があり、「Recently played」が無い 7 種。
- **要求を満たしたことにならない例**（UI品質）: 日付の項目に時刻が常に出て、追加日より長い。
  3 つの日付のアイコンが同じか、カレンダーの小さな印の違いだけで見分けられない。情報の行に
  「Updated」と並べ替えの「Date modified」が同時にあり、どちらがファイルかを説明の文で補って
  いる。一覧のカードやリスト表示の行に Edited・Created が出ている。既存の「Date modified」の
  名前や順が変わっている。並べ替えの新しい種類が「Date created (file)」のような補足を持つ。

## Colour

新しく使う組は無い。日付の文字は今の情報の行と同じ `fg-muted` on `bg`、アイコンは `fg-subtle`
（印だけに使い、文字には使わない）。メニューの項目は今の `ui/Menu` のまま。新しいトークンは
足さず、`tokens.test.ts` の `pairs` にも足さない。

## Accessibility

親 Issue は読み上げ・対比の設計を求めていないので、名前だけを決める（012・029・030 と同じ範囲）。

- 情報の行の項目の隠した名前: 「Edited」「Created」。値は後ろの日付の文字で読まれる。`title` は
  「Edited Sep 27, 2026, 3:04 PM」の形で、追加日も「Added …」に揃える。
- 並べ替えのラジオの名前: 「Date created」。向きのボタンの読み上げ名は今の形
  「Descending (newest first). Press for ascending」。
