---
source: specs/055-activity/ui-design.md
sourceHash: 9f98e40ad52674b05867214967974cb55d63c18d9bc530e6a22371e33a97f6fc
---

# UI 設計: Activity {#ui-design-activity}

出典: [design-system.md](../../docs/design-docs/design-system.md) とその規則 ([patterns.md](../../web/registry/rules/patterns.md)、[components.md](../../web/registry/rules/components.md)、[foundations.md](../../web/registry/rules/foundations.md))。親 Issue を書いたときに依頼者が渡した参考画像 (ファイルのアップローダーのアクティビティ画面: 実行ごとに枠線のある 1 枚のカード、状態のアイコン、出どころのチップを持つファイル名、右端の結果と時刻、実行中のカードのライブの進捗バー、失敗の赤いカード、下端の件数の帯)。データと操作は [plan.md](plan.md) と [contracts/screen-api.md](contracts/screen-api.md) である。

## この形にした理由 {#why-this-shape}

Activity は項目ごとに 1 枚のカードの一覧で、依頼者の参考画像が描くとおり、各カードは広い画面で 1 行である。状態は先頭のアイコンであり、問題は目が読む前に見つける色付きのカードであり、結果は目が行を読み終える右端にある。どの絞り込みも、状態を含めて、同じ形のドロップダウンボタンの並びの 1 つなので、どの絞り込みもほかより重要には見えず、ツールバーは 1 行のままである。

| 採用しなかった案 | 理由 |
| --- | --- |
| History の、枠線のない日ごとにまとめた文字の行 | 枠線のない行は互いに溶け合う。依頼者は見てすぐに退けた |
| ドロップダウンの絞り込みの横に、下線のタブとしての状態 | 一覧を絞るという同じ行為に 2 つの形がある。依頼者は 1 つを求めた |
| ほかの絞り込みのための 2 行目のツールバー | 依頼者は 1 行を求めた |
| ライブラリの画面の単一の `FilterMenu` のポップオーバー | 各軸の今の値は、何も開かずにツールバーに見えなければならない |
| 参考画像にあるような、ヘッダーの "Pause all" と "Retry failed" | 親 Issue は一括の操作を範囲外にしている |

## 文言 {#words}

`web/src/i18n/en.ts` が持つことになる画面の文言で、注記がない限り `t.activity` の下にある。`{name}` などはカタログの関数の引数である。

| 場所 | 文言 | 補足 |
| --- | --- | --- |
| サイドバーの項目 (`t.shell.nav.activity`) | `Activity` | |
| サイドバーの件数、読み上げ | `1 needs attention`、`{count} need attention` | `selectPlural`。見える文字は数である |
| ページと文書のタイトル | `Activity` | |
| 検索 | `Search file names` | プレースホルダーとラベル |
| 絞り込みのボタン | `Status`、`Feature`、`Decided by`、`Period` | それぞれの後に今の値が続く |
| Status の値 | `All`、`Running`、`Needs attention`、`Done` | `All` 以外は、メニューでそれぞれ件数を持つ |
| Feature の値 | `All`、`Import`、`Scan`、`Duplicate check`、`Auto-tagging`、`Work info`、`Tags`、`Versions`、`Media folders`、`Drag and drop` | 複数にチェックを付けられる。ボタンは 1 つの名前か `{first} +{n}` を示す |
| Decided by の値 | `All`、`AI`、`Rule`、`You` | |
| Period の値 | `Any time`、`Today`、`Last 7 days`、`Last 30 days` | |
| クリア | `Clear filters` | いずれかの絞り込みが設定されている間、ボタンの後に |
| 状態のアイコン、読み上げ | `Running`、`Waiting`、`Needs attention`、`Done`、`Done by you` | |
| チップ: 種類 | `Import`、`Scan`、`Duplicate check`、`Auto-tagging`、`Work info fetch`、`Bundled`、`Removed from versions`、`Made representative`、`Marked different`、`Tag added`、`Tag removed`、`Tag kept`、`Tag rejected`、`Work info applied`、`Media folder removed`、`Drag and drop undone`、`Undo` | 種類ごとに 1 つで、データモデルの順 |
| 結果: 実行中の取り込み | `{stage}` とバー、`{done}/{total}` | `{stage}`: `Analyzing`、`Thumbnail`、`Seek thumbnails`、`Preview`、`Duplicate fingerprint` |
| 結果: 待機中 | `Waiting` | |
| 結果: 失敗した取り込み | `Failed at {stage}: {detail}` | `{stage}` は小文字 |
| 結果: ほかの失敗 | `Failed: {detail}` | |
| 結果: 失敗した重複の確認 | `Failed · tries again at {time}` | `{time}`: 今日なら `14:05`、そうでなければ `Oct 17, 14:05` |
| 結果: 中断 | `Stopped when vv shut down` | |
| 結果: 接続なし | `Needs the AI connection to decide` | |
| 結果: 判定できない | `Couldn't tell whether these are the same video` | モデルが答えたときは `AI {percent}` とともに |
| 結果: 同じ | `Same video · bundled by AI {percent}`、`Same video · bundled by rule` | |
| 結果: 異なる | `Different videos by AI {percent}` | |
| 結果: 自動タグ付け | `+ {name} {percent}` を ` · ` でつなぐ。提案のタグの後に `(suggested)`。`No tags added` | |
| 結果: 所有者のタグの操作 | `+ {name}` または `− {name}` · `by you` | |
| 結果: メディアフォルダの削除 | `1 video left the library · by you`、`{count} videos left the library · by you` | |
| 結果: 置き換え | `Replaced by a newer run` | |
| 結果: 所有者が決めた | `You decided on Duplicates` | |
| 結果: なくなった | `A video left the library` | |
| 結果: 取り消し | `Undid {chip} · {result}`、`Undid tag {name}` | `undoes` から |
| 結果: すでに戻された | `Already changed back by you` | |
| 取り消し済みの接尾辞 | `· Undone by you` | 取り消された項目の結果の後に |
| ライブラリにない | `Not in the library` | 対象に動画がないとき、結果の末尾を置き換える |
| 時刻 | `Just now`、`{n} minutes ago`、`{n} hours ago`、`Yesterday {time}`、`{date} {time}` | ツールチップに完全な日付と時刻 |
| アイコンボタン (ツールチップと読み上げ) | `Retry`、`Undo`、`Undo tags…`、`Open`、`Open in Duplicates`、`Work info`、`Settings` | |
| フッター | `{n} running`、`{n} need attention`、`{n} done` | running は `counts.running` と同じく、処理中の項目とキューにある項目を数える |
| 取り消しの後のトースト | `Undone. vv won't decide this again on its own` | |
| すでに戻されていた取り消しの後のトースト | `Already changed back. vv won't decide this again on its own` | |
| 再試行か取り消しが失敗したときのトースト | `Couldn't retry: {reason}`、`Couldn't undo: {reason}` | `{reason}` はエラーのカタログから |
| エラー `activity_not_retryable` (エラーのカタログ) | `This can no longer be retried` | |
| エラー `activity_not_undoable` (エラーのカタログ) | `This can no longer be undone` | |
| 空、項目がまったくない | `No activity yet` / `Imports, auto-tagging, duplicate checks and your changes to tags and versions show up here.` | タイトル / 説明 |
| 空、絞り込みに何も合わない | `Nothing matches these filters` / `No entries match these filters.` / `Clear filters` | タイトル / 説明 / 操作 |
| 読み込みの失敗 | `Couldn't load the activity` | `Retry` とともに |

## サイドバーの項目 {#sidebar-entry}

`Activity` はメインのナビゲーションの最後の項目で、`Duplicates` の後にあり、所有者だけのもので、lucide の `Activity` アイコンを持つ。1 つ以上の項目が対応を必要とする間、レールの項目は右上の角に丸い `destructive` の件数を持つ ([デザインシステムの変更](#changes-to-the-design-system))。実行中の項目は印を加えない。件数は、どの画面でも `activity` の通知で更新される。

## Activity のページ {#activity-page}

`PageHeader` にタイトル `Activity` を持つ `ListPage` で、次に 1 行の `Toolbar`、次にカード、次にフッターの帯が続く。

### ツールバー {#toolbar}

1 行で、左から右へ: 検索欄、次に同じ形の 4 つの絞り込みのボタン `Status`、`Feature`、`Decided by`、`Period`、そしていずれかが設定されている間は `Clear filters`。

| 部分 | 形 |
| --- | --- |
| 検索 | 検索アイコン付きの `Input`、`sm`。所有者が入力すると、間を置いた後に絞り込む |
| 絞り込みのボタン | 軸の名前と `text-muted-foreground` の今の値、そして山形のアイコンを持つ `outline` `sm` の `Button`。既定以外の値にすると、値を含めて `primary` の文字の `primary-soft` になる |
| Status のメニュー | ラジオの項目を持つ `DropdownMenu`。各項目は右端に `tabular-nums` の件数を持つ。`Needs attention` の件数は 0 を超えると `destructive` になる |
| Feature のメニュー | チェックボックスの項目とその件数を持つ `DropdownMenu`。所有者の操作の機能は区切り線の後に置く |
| Decided by と Period のメニュー | ラジオの項目を持つ `DropdownMenu` |
| Clear filters | `ghost` `sm` の `Button` |

すべての絞り込みはページの URL (`status`、`feature`、`decider`、`period`、`q`) にあり、履歴に積まれるので、戻ると前の絞り込みに戻る。

### カード {#cards}

カードは `gap-2` で積み重なる。各カードは `muted` の面上の 1 行で、`border` の線と `rounded-lg` の角を持ち、高さは少なくとも名前付きの段階 `activity-card`、`px-4` である。

| 部分 | 内容 | 形 |
| --- | --- | --- |
| 状態のアイコン | 回転する `LoaderCircle` (実行中)、`Clock` (待機中)、`CircleAlert` (要対応)、`CircleCheck` (完了)、`User` (所有者による完了) | `size-5`。`primary`、`muted-foreground`、理由により `destructive` か `warning`、`success`、`muted-foreground` |
| 対象 | ファイル名。組は `{a} ↔ {b}`。スキャンは `Manual scan` か `Watch scan`。フォルダはそのパス。動画を持つ要素は動画のページへリンクする | `text-sm font-medium`、1 行。カードの幅が足りなくなると最初に切られる |
| チップ | 種類 | `Badge` `secondary`。決して切られない |
| 結果 | 結果の文言。AI の割合は `foreground` | `text-sm text-muted-foreground`、右寄せ、1 行。チップより先に切られる |
| 進み具合 | 実行中の取り込みについて: 段階、幅が名前付きの段階 `activity-bar` のバー、`primary` の `{done}/{total}` | `Progress` |
| 時刻 | 相対時刻 | `text-sm text-muted-foreground`、幅は名前付きの段階 `activity-time`、右寄せ |
| 操作 | 項目が提供するアイコンボタン | `ghost` `icon-sm`。時刻がそろうよう、固定幅の末尾の列に置く |

| カードの状態 | 枠線と面 |
| --- | --- |
| 実行中 (処理中) | `primary` の枠線、`primary-soft` の面 |
| 要対応: `failed`、`interrupted` | `destructive-strong` の枠線、`destructive-soft` の面。結果は `destructive` |
| 要対応: `no_connection`、`undecided` | `warning` の枠線、`warning-soft` の面。結果は `warning` |
| それ以外すべて | `border`、`muted` |

| 操作 | アイコン | 示すとき | その後 |
| --- | --- | --- | --- |
| Retry | `RotateCw` | `retryRun` | 先頭に新しい実行中のカードが現れ、このカードは `Replaced by a newer run` と示す。重複の確認のカードは、代わりに実行中になる。スキャンのカードは、スキャンが失敗せずに終わるまで残る |
| Undo | `Undo2` | `undoable` で、`auto_tag` でない | トースト。カードに `· Undone by you` が加わる。先頭に所有者による新しいカード |
| Undo tags… | `Undo2` | 取り消されていないタグを持つ `auto_tag` | 加えたタグとその割合の `DropdownMenu` を開く。1 つを選ぶとそのタグを取り消し、同じトーストを示す |
| Open | `ExternalLink` | ライブラリにある動画の対象 | 動画のページ |
| Open in Duplicates | `ExternalLink` | `candidate` が true の組 | `/duplicates` |
| Work info | `FileText` | 動画がライブラリにある `work_info_fetch` | `/videos/{id}/edit` |
| Settings | `Settings` | 理由 `no_connection` | `/settings` |

リクエストが保留の間、そのボタンは `Spinner` を示し、そのカードのすべてのボタンが無効になる。再試行も取り消しも確認を求めない。取り消しは自動の決定の前の状態に戻し、所有者はそれを手でもう一度できる。失敗したリクエストはカードをそのままにし、トーストを示す。

カードは `createdAt` で新しい順に並び、所有者がスクロールするにつれてページごとに読み込まれ、末尾に `LoadMoreRow` がある。

### フッターの帯 {#footer-band}

カードの下の、上に `border` を持ち、高さが `activity-card` の帯である。左から `{n} running` と `{n} need attention` で、各数は `font-semibold` (`primary`、`destructive`)、そして右端に `{n} done`。数は絞り込みのない要求の `counts` である ([contracts/screen-api.md](contracts/screen-api.md#get-apiactivity))。そのため `running` はキューにある項目を含む。サイドバーと同じく、絞り込みにかかわらずログ全体を数えるので、所有者はほかの何を見ている間も、進行中の作業と問題を見られる。

### ライブの更新 {#live-updates}

`activity` の通知で、ページは件数と読み込んだすべてのページを取り直し、カードをその場で置き換える。絞り込みに合わなくなったカードは出ていく。新しいカードは先頭に加わる。終わった段階ごとに通知が 1 つ送られるので、実行中の取り込みのバーは通知のたびに進む。

### 状態 {#states}

| 状態 | ページが示すもの |
| --- | --- |
| 読み込み中 | ツールバーの下に `LoadingState` |
| 読み込みの失敗 | `Couldn't load the activity` と `Retry` を持つ `ErrorState` |
| 項目がまったくない | `Activity` アイコン、`No activity yet` とその説明を持つ `EmptyState` |
| 絞り込みに何も合わない | `ListFilter` アイコン、`Nothing matches these filters`、その説明、`Clear filters` を持つ `EmptyState` |
| さらに読み込み中、さらに読み込むのに失敗 | すべての一覧と同じく `LoadMoreRow` |

ゲストにはサイドバーの項目がなく、`/activity` は、所有者だけのどの画面とも同じく、ゲストをサインインに送る ([016 UI 設計](../016-single-account-auth/ui-design.md))。

## デザインシステムの変更 {#changes-to-the-design-system}

| 追加 | 何か | 理由 |
| --- | --- | --- |
| 絞り込みのドロップダウンボタン | 軸の名前、その値、山形のアイコンを持ち、`DropdownMenu` を開く `outline` `sm` の `Button`。既定でないときは `primary-soft`。components.md の `DropdownMenu` の隣に、ショーケースの項目とともに入る | Activity は 4 つの軸で絞り込み、それぞれの値が見えている必要がある。ライブラリの画面の `FilterMenu` はそれらを 1 つのボタンの裏に隠す |
| カードの一覧 (`CardList` のセクション) | `muted` の上の 1 行の枠線付きカードの積み重ねで、`gap-2`、呼び出し側が設定する 4 つの色調 (既定、`primary`、`destructive`、`warning`) と、時刻と操作のための末尾の固定の列を持つ。patterns.md に例のブロックとともに入る | 枠線付きの行を描くセクションはない。`GroupedList` の行には枠線がなく、`DataTable` は 360px で 1 行に収まらないヘッダーと列を持つ |
| レールの件数 | レールの項目が件数を受け取り、アイコンの右上の角に丸い `destructive` のピルとして描く。99 を超えると `99+`、0 では何もない。狭い幅のドロワーでは、行の末尾の `SidebarMenuBadge`。components.md の Sidebar に入る | `SidebarMenuBadge` はサイドバーがアイコンに畳まれると隠れ、vv のレールは常にその状態である |
| 名前付きのサイズ `activity-card`、`activity-time`、`activity-bar` | カードの最小の高さ (52px)、時刻の列 (104px)、進捗バー (160px) | 間隔の尺度には 52px、104px、160px の段階がない |

## レスポンシブな振る舞い {#responsive-behaviour}

| 幅 | 配置 |
| --- | --- |
| 360px | ツールバーは横にスクロールする 1 行である: 行の上に欄を開く検索のアイコンボタン、次に絞り込みのボタン。各カードは 2 行である: 状態のアイコンと対象で、操作のボタンは右で両方の行にまたがる。対象の下に、チップ、結果 (切られる)、短い時刻 (`15m`)。フッターは `done` を除く 2 つの件数を示す |
| 768px | 360px と同じで、アイコンボタンの代わりに検索欄がある。4 つの絞り込みのボタンが収まる |
| 1280px | 検索欄と 4 つのボタンを持つ 1 行のツールバー。各カードは 1 行である: アイコン、対象、チップ、右に押しやった結果、時刻、操作 |

## レビューの基準 {#review-criteria}

1. 1280px で各カードは 1 行で、結果、時刻、操作が一覧の下まで列にそろう。360px で各カードは 2 行で、画面より広いカードはない。
2. 失敗したカードは赤で、AI を待つカードは琥珀色で、文字を読む前に目立ち、実行中の取り込みはそのバーが動くのを示す。
3. 120 文字のファイル名は、チップや結果より先に省略記号で切られる。状態のアイコン、時刻、操作は決して切られない。
4. 5 つの絞り込みの操作部品はすべて同じ高さ、枠線、書体を持つ。設定された絞り込みは、開かなくてもボタンにその値が読める。
5. Status のメニューの件数は各選択肢が一覧するカードの数に等しく、フッターとサイドバーの件数は、絞り込みにかかわらずログ全体の件数に等しい。
6. カードはその項目が許すボタンだけを示す: 終わった取り込みは Open だけを示す。失敗した取り込みは Retry を示す。自動のまとめは Undo と Open を示す。取り消された項目は Undo を示さない。
7. ページを開いたまま、終わった取り込みは再読み込みなしにそのカードを実行中から完了に変え、所有者が読んでいたカードは動かない。
8. カードの間隔とページの余白は、どの幅でも `ListPage` と `CardList` の段階に一致する。
