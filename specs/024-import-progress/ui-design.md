# UI Design: 取り込みの進捗と結果

**Feature**: [parent Issue #444](https://github.com/syudead/vv/issues/444) ・
[plan.md](plan.md) ・ [contracts/scan-api.md](contracts/scan-api.md)

見た目の規則は次の文書に従う。
- シェル: [ライブラリ UI](../../docs/design-docs/library-ui.md)
- role token: [`web/src/index.css`](../../web/src/index.css)
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)

進捗表示の置き場所と操作は、[動画取り込みの進捗表示 UI](../012-scan-progress/ui-design.md)
（以下 012）が決めた形をそのまま使う。具体的には次のとおりである。
- 右下に1つだけ固定する。
- 押すと `/settings#scan-status` へ移る。
- hover と focus で概要を開く。
- 失敗の通知を閉じる button を出す。
- 設定の section はメディアフォルダより前に置く。
- 取得の一時失敗では最後の状態を保つ。

本書は、012 のうち**何を示すか**を置き換える。新しい色・半径・影の token は足さない。

置き換えるのは次の3つである。
- 012「Floating Indicator」の文言の行
- 012「Summary Popover」の表示内容
- 012「Settings Scan Status」の 2〜4 番目（件数の行と時刻の行）

012 の「States」の表は、本書の [States](#states) が置き換える。画面の単位が実装されたとき、
012 はその旨を記して本書を指す（[plan.md](plan.md) の画面の単位）。

画面の言葉は英語である。
- 置き場: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md) のとおり、文字列のカタログ
  [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) に置く。今の `shell.scan` と
  `settings.scanStatus` の鍵を、本書の言葉に置き換える。
- 本書の英語の位置づけ: 文言の意図を示す案である。実装後はカタログが正本になる。
- 本数と数: `videos(n)` と `formatNumber`（`selectPlural` を含む）で、単数と複数を分ける。
- 日時: `formatDateTime` で書く。
- 利用者のデータ（ファイル名・フォルダ名）: 訳さずに引数として埋め込む。
- 本文: 設計文書なので日本語で書く（023 [research.md R-8](../023-english-i18n/research.md)）。

## Screen Boundary

- 右下の本体、概要（popover）、設定の「Scan status」は、同じ `Scan` を読む1つの表示モデル
  （今の `scanPresentation.ts` の役目）から作る。3つの面は、同じ要素を同じ順に並べ、面が広い
  ほど要素を足す（要件 8）。

  | 要素 | 右下の本体 | 概要 | 設定 |
  | --- | --- | --- | --- |
  | 状態 | ○ | ○ | ○ |
  | 進み具合（本数） | ○（数字だけ） | ○（バーと文） | ○（バーと文と対象の説明） |
  | 今の処理、または完了の時刻 | — | ○ | ○ |
  | 問題の本数 | ○ | ○ | ○ |
  | 問題の一覧 | — | — | ○ |
  | 取り込み自体の失敗の理由と再試行 | — | — | ○ |

- 設定の section には、この表にない数字を置かない。今の「Processed / Total / Failed」、
  「Preparation left」、「Started / Finished」は消す。
- 問題の一覧の行から、再生画面（`/videos/{id}`）へ移る。一覧は設定の section の中に置き、
  別の画面や dialog にはしない。

## Words

状態・進み具合・今の処理は、次の言葉で表す。

**状態の言葉**:

| `Scan.status` | 状態の言葉 | アイコン |
| --- | --- | --- |
| `finding` | Scanning | `RefreshCw`（回転） |
| `running` | Scanning | `RefreshCw`（回転） |
| `done` | Done | `CheckCircle2` |
| `partial` | Some failed | `AlertTriangle` |
| `failed` | Scan failed | `XCircle` |

開始を要求してから最初の `Scan` が届くまでは、今と同じく「Starting」とする。

**進み具合の文**: 「M of N videos done」とする。
- 分母には、変化のあったファイルと、準備が残っていた動画が入る。
- 済みには、失敗して終わった動画も入る。使えるかどうかは、問題の本数が答える。
- 右下の本体では、同じ値を「M / N」と書く。
- 設定では、進み具合の文の下に、分母の意味を1行で添える（要件 9）: 「Counts changed files and
  videos that still needed preparing, not the whole library.」
- `videos.total = 0` で終わったときは、進み具合の文の代わりに「No changed files were found.」と
  し、バーも数字も出さない（受け入れ条件 11）。
- `finding` のあいだは数字を出さない。

**今の処理の行**: 「〈何をしているか〉 · 〈ファイル名〉」の順にする。何をしているかを先に
置くのは、長いファイル名を末尾で省略しても動作の言葉が残るようにするためである。

| `activity.kind` | 何をしているか |
| --- | --- |
| （`finding` で activity が無い） | Looking for files…（ファイル名なし） |
| `registering` | Adding to the library |
| `probe` | Analyzing |
| `thumbnail` | Creating the thumbnail |
| `seekThumbnail` | Creating seek thumbnails |
| `preview` | Creating the preview |

`running` のあいだに `activity` が一瞬無くなっても、行を空にせず、直前の値を出したままにする。
行の高さは1行で固定する（受け入れ条件 14）。

**完了の時刻の行**: 終わった後は、今の処理の行と同じ位置に時刻を置く。
- `done`: 「Finished 〈日時〉」
- `partial`: 「Finished 〈日時〉」。状態の言葉が「Some failed」なので、時刻の行では言い分けない。

日時は `Scan.settledAt` を `formatDateTime` で書く（例: Sep 28, 2026, 3:04 PM）。実行中は時刻を
出さない（要件 4）。`failed` では時刻を出さず、同じ位置には「The scan couldn't finish.」
を出す。`failed` の `Scan` には時刻が無い
（[contracts/scan-api.md §2](contracts/scan-api.md#2-scan)）。いつの結果かは、失敗の理由と
再試行が補う。

**問題の本数**:
- 失敗は「N failed」と書き、`AlertTriangle` を添える。
- 代用は「N to check」と書き、`Info` を添える。
- 0 の方は出さない。
- 右下の本体では、失敗があれば失敗だけを、無ければ要確認を出す。

**問題の影響と理由**: 一覧の各行は、主な種類の影響を1文目に、理由を2文目に出す。主な種類は
`kinds` の先頭である（[contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)）。
影響は「その動画がどうなっているか」を利用者の言葉で書く（要件 5）。

| `kind` | 影響 | 理由 |
| --- | --- | --- |
| `unreadable` | Not added to the library. | The file couldn't be read. |
| `changed_during_import` | Not added to the library. | The file changed during the scan. |
| `register_failed` | Not added to the library. | Saving it to the library failed. |
| `probe_failed` | It may not play. | It couldn't be analyzed as a video. |
| `thumbnail_failed` | It has no thumbnail. | The thumbnail couldn't be created. |
| `seek_thumbnail_failed` | No thumbnails appear while seeking. | The seek thumbnails couldn't be created. |
| `preview_failed` | No preview appears in the list. | The preview couldn't be created. |
| `thumbnail_first_frame` | The thumbnail uses the first frame instead. | The frame at the usual position couldn't be read. |
| `seek_thumbnail_full_decode` | The seek thumbnails were rebuilt from the whole video. | The usual method didn't work. |

2つ目以降の種類は、影響だけを「Also: 〈影響〉 〈影響〉」の1行にまとめる。
カタログは、この表を `kind` をキーにした `Record` で持つ。種類の漏れは、型検査で見つかる。

## Floating Indicator

012 と同じ位置、同じ面（`bg-elevated`・`border-border-strong`・`shadow-elevated`・`rounded-md`）
を使う。本体は1行で、左から次の順に並べる。
1. アイコン
2. 状態の言葉
3. 「M / N」
4. 問題の本数

- 状態の言葉を `font-medium` の `text-fg` にし、ほかの要素より強くする。数字は `tabular-nums`
  にする。問題の本数は、進み具合と同じ文字の大きさと色（`text-fg`）で出し、アイコンで区別する。
  アイコンだけに意味色を付ける: 失敗は `text-danger`、要確認は `text-warning`。
- 今の処理は本体に出さない。本体の幅が処理ごとに伸び縮みしないようにするためである。
  右端を固定しているので、数字の桁が変わっても右端は動かない。
- **見えている期間**:
  - `finding`・`running` のあいだは出し続ける。
  - `done` は、012 の完了と同じく 8 秒で閉じる。代用だけのときも同じである。
  - `partial` は、`failed` と同じく、閉じる button で閉じるか設定へ移るまで残す。使えない
    動画があることを、離席していた所有者にも見せるためである。閉じる button の名前は
    「Dismiss the scan result notice」とする。
- 押したとき・Enter・Space の動作は 012 と同じで、`/settings#scan-status` へ移る。

## Summary Popover

012 と同じ開き方と閉じ方を使う。中身は、上から次の3行とバーにする。

1. 状態の行: アイコンと状態の言葉（`font-semibold`）。右端に問題の本数を出す。失敗と要確認が
   両方あれば、両方並べる。
2. バー（012 と同じ `ScanProgressBar`）と、その下の進み具合の文。
3. 今の処理の行、または完了の時刻の行。
   - `text-sm` の `text-fg-muted` で1行に収め、長いファイル名は末尾を省略する。
   - 省略した全体（登録フォルダの表示名 / 相対パス / ファイル名）は `title` で読める。

`partial` と、`done` で要確認があるときは、3 行目の下に「See Settings for the list.」を
`text-xs` で添える。本体を押すと移る先と同じなので、リンクにはしない。

仕事の種類ごとの内訳（`ProcessingBreakdown`）は出さない。

## Settings Scan Status

012 の section の枠、見出し、見出しへの focus の移し方は変えない。中身は上から次の順にする。

1. **見出しの行**: 「Scan status」と、状態の badge（アイコン＋状態の言葉）。badge の後ろに
   問題の本数を同じ形の badge で並べる。
2. **進み具合**: バー、進み具合の文、分母の意味の1行。
3. **今の処理の行、または完了の時刻の行**: 概要と同じ1行。高さは固定する。
4. **取り込み自体の失敗**（`failed` のときだけ）: 012 と同じく、理由を `text-danger` で書き、
   主操作の「Retry」を置く。理由の文は、023 が決めた `scanErrorText(errorCode, errorPath)` で書く。理由は、問題の一覧より上に置く。取り込み自体の失敗を、
   部分的な問題より先に読ませるためである。
5. **問題の一覧**（問題が1本以上あるときだけ）。

2 と 3 の並びと形は、概要と同じにする。設定で足すのは、分母の意味の1行、4、5 だけである。

### Issue List

- **見出し**: 「Videos with problems」（`h3`、`text-sm font-semibold`）。その右に失敗と要確認の
  本数を並べる。一覧の上は、区切り線（`border-border`）で 1〜3 と分ける。
- **並び**: API の順（失敗を先に、同じ重さの中はファイル名の順）のまま出す。
- **行**: 左に種類の目印の列、右に文の列を置く。
  - 目印: アイコンと言葉。失敗は `AlertTriangle` と「Failed」を `text-danger` で、要確認は
    `Info` と「Check」を `text-warning` で書く。色だけで区別しない（Issue「色だけに頼らない」）。
  - 文の列: 次の4行にする。
    1. ファイル名: `text-sm font-medium text-fg`、1行、末尾を省略する。
    2. 置き場所: 「登録フォルダの表示名 / 相対パス」を `text-xs text-fg-muted` の1行で書き、
       末尾（ファイルに近い側）を優先して残す。フォルダのカードの登録パスと同じ省略の仕方
       である（library-ui.md §6）。
    3. 影響と理由: `text-sm text-fg`。影響の後に理由を続け、折り返しを許す。
    4. ほかの種類: 「Also: …」の行。種類が2つ以上のときだけ出す。
  - ファイル名と置き場所の省略した全体は、`title` で読める。同じ名前のファイルは、置き場所の
    行で見分ける（Edge Cases）。一覧に出る行は、必ず登録フォルダの中にある
    （[contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)）ので、
    置き場所の行が空になることは無い。
- **動画へ移る**:
  - 登録された動画（`videoId` がある）の行: 行全体を `/videos/{id}` へのリンクにする。右端に
    `ChevronRight` を置き、hover で `hover-wash` を敷き、focus-visible で 012 と同じ `link` の
    輪郭を出す。リンクの名前は、ファイル名と影響を含める。
  - 未登録のファイルの行: リンクにせず、右端に何も置かない。
- **長い一覧**: 一覧は、高さに上限（画面の高さのおよそ半分）を持つ縦スクロールの領域にする。
  - 領域の中には、最初の 50 件を出す。
  - 末尾に「Show more」の button を置き、押すと次の 50 件を足す。
  - 領域より下の「メディアフォルダ」の section の位置は、一覧が何件でも変わらない
    （Issue「押し流さない」）。
  - 領域にはキーボードで入れる（中の行のリンクで辿れる。リンクの無い行だけが続く場合も、
    領域自身に `tabIndex=0` と名前「Videos with problems」を付ける）。
  - 採らない案: 先頭の数件だけ出して「すべて表示」で展開する形は、展開すると数千件が
    メディアフォルダの section を押し流す。
- **入れ替わり**: 新しい取り込みで `Scan.id` が変わったら、一覧を読み直して先頭へ戻す。
  同じ取り込みの中で `issues.revision` が変わったら、読み直しても領域のスクロール位置を保つ。

## States

| 状態 | 右下の本体 | 概要 | 設定 |
| --- | --- | --- | --- |
| 未実行 | 出さない | なし | 「No scan has run yet」。バー・数字・一覧なし |
| 開始中 | 「Starting」 | 数字なしのバー、「Looking for files…」 | 同じ |
| `finding` | 「Scanning」 | 数字なしのバー、「Looking for files…」 | 同じ |
| `running` | 「Scanning M / N」（＋問題の本数） | バー、「M of N videos done」、今の処理 | 同じ＋分母の意味＋（問題があれば）一覧 |
| `done`（N ≥ 1） | 8 秒「Done N / N」（＋要確認の本数） | 満ちたバー、「N of N videos done」、「Finished 〈日時〉」 | 同じ＋（要確認があれば）一覧 |
| `done`（N = 0） | 8 秒「Done」 | バーなし、「No changed files were found.」、「Finished 〈日時〉」 | 同じ |
| `partial` | 閉じるまで「Some failed · K failed」 | 満ちたバー、済みの文、「Finished 〈日時〉」、設定への案内 | 同じ＋一覧 |
| `failed` | 閉じるまで「Scan failed」 | その時点のバーと文、「The scan couldn't finish.」 | 同じ＋理由と再試行＋（あれば）一覧 |
| 取得の一時失敗 | 012 と同じ | 012 と同じ | 012 と同じ。一覧は最後に読めたものを残す |

`running` は、走査が閉じて準備だけが残るあいだも含む。準備だけが残るあいだを、別の状態の
言葉や別の数字にしない（要件 1・2）。

## Responsive Layout

判定する幅は 360px・768px・1280px である。

- 360px:
  - 右下の本体: 1行に収める。問題の本数を含めて収まらないときは、「M / N」を残し、
    問題の本数をアイコンと数字だけにする（言葉は名前に残す）。
  - 概要: 012 と同じ幅に収める。今の処理の行は1行で省略する。
  - 設定の一覧の行: 目印の列を、ファイル名の行の頭に寄せた1列にする。文の列は画面の幅いっぱいを
    使う。
- 768px 以上: 一覧の行は目印の列と文の列の2列にする。目印の列の幅は「Failed」「Check」の長い方が
  収まる幅で揃え、行ごとに文の列の開始位置を変えない。
- 1280px: 設定の section は今の最大幅（`max-w-4xl` の中）のまま広げない。一覧の文の列は
  section の幅で折り返す。

どの幅でも、次の3つは位置も高さも変えない。
- 右下の本体
- 概要の今の処理の行
- 設定の今の処理の行

## Accessibility

Issue の「支援技術」の項に従い、012 の Accessibility を次のように変える。

- `role="status"` で読み上げるのは次の3つの節目だけにする。今の処理が変わっても、進み具合が
  変わっても読み上げない。
  - 完了: 「The scan is complete.」。要確認があれば「N videos are worth checking.」を続ける。
  - 一部失敗: 「The scan finished with some failures. N videos may not be usable.」
  - 失敗: 「The scan failed.」
- バーの名前は「Progress of the videos in this scan」とし、`aria-valuetext` を「M of N videos
  done」にする。`finding` では `aria-valuenow` を付けない（012 と同じ）。
- 右下の本体の名前は「〈状態の言葉〉 M of N videos done, 〈問題の本数〉. Open the scan status」とする。
- 問題の一覧は `ul` とし、各行のリンクを Tab で辿れる。

## Review Criteria

360px・768px・1280px の各幅で、次を目で見て判定する。

1. **階層**: どの面でも、状態の言葉が最も強く読め、次に進み具合と問題の本数、最後に今の
   処理の行が読める。内部の段階名（走査・準備・仕事の種類。今の「Preparing」「Preparation left」
   など）が、状態の言葉や数字のラベルとして現れない。
2. **1つの進み具合**: 10 本の取り込みの間じゅう、右下と設定の数字が「/ 10」のまま増える。
   単位の違う数字に切り替わらない。登録が終わった後も「完了」や満ちたバーにならない。
3. **止まって見えない**: 取り込み中、概要と設定の今の処理の行が、ファイル名と動作の言葉を
   変えながら進む。そのあいだ、右下の本体、概要の行、設定の行の位置と高さが動かない。
4. **使えるか・見るべきか**: 終わった後、右下を一目見て「完了」「一部失敗」のどちらかと、
   失敗・要確認の本数が分かる。設定では、その本数と同じ数の行が一覧にある。
5. **一覧の読みやすさ**:
   - 失敗の行が要確認の行より先に並ぶ。
   - 目印の言葉とアイコンで、失敗と要確認を色なしでも区別できる。
   - 長いファイル名でも行が崩れず、同じ名前のファイルを置き場所の行で見分けられる。
   - 登録された動画の行から再生画面へ移れる。
6. **押し流さない**: 問題が数百件あっても、設定の「メディアフォルダ」の section の位置が
   変わらない。「Show more」で最後の行まで辿れる。
7. **密度**: 設定の「Scan status」は、概要と同じ要素に、分母の意味・失敗の理由・一覧だけを
   足した形に見える。概要にない数字の欄が並んでいない。
8. **重ならない**: 次の組み合わせのどれでも、はみ出しも重なりもしない。
   - 右下の本体と概要
   - Toast
   - 再生画面の操作
   - 長いファイル名
