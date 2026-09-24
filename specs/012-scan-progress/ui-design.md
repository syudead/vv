# UI Design: 動画取り込みの進捗表示

**Feature**: [parent Issue #170](https://github.com/syudead/vv/issues/170) | **Plan**: [plan.md](plan.md)

見た目の規則、シェル、一覧の密度は
[ライブラリ UI: 見た目の規則と一覧の構成](../../docs/design-docs/library-ui.md) と
[`web/src/index.css`](../../web/src/index.css) の role token に従う。本書は、取り込み進捗の
フローティング表示と設定画面の「取り込み状況」がそれらに足すものだけを定める。新しい色、
半径、影の token は追加しない。

## Screen Boundary

- フローティング進捗は route より上に1度だけ配置し、ライブラリ、フォルダ、設定、再生画面で
  同じ表示インスタンスを使う。通常画面では右下に固定する。再生画面は独立したシアターモードの
  まま保ち、進捗表示は通常画面と同じ右下に固定する。再生画面の右上には閉じる × があるため
  （[動画詳細画面の UI](../012-video-detail-ia/ui-design.md)「Close」）、そこへは置かない。
- トップバーの「ライブラリを更新」は待機時の開始操作として残す。取り込み中の割合、件数、
  失敗結果はトップバーへ出さず、開始ボタンは実行中であることだけを短く示す。
- 設定画面には `id="scan-status"` の section をメディアフォルダ設定より前に置く。`/settings#scan-status`
  を直接開いたときも、現在または直近の取り込み状況が最初に確認できる。
- フローティング表示と設定 section は `scanPresentation.ts` の共有表示モデルから同じ状態名、
  progress、件数、時刻、失敗理由を読む。画面ごとに `done + failed > 0` や総数未確定を
  別解釈しない。

## Floating Indicator

インジケーターは補助的な運用状態であり、ライブラリカード、検索、フィルタ、フォルダ移動、
再生への導線より弱く見せる。どの画面・どの幅でも右下に `fixed` で置き、本文の layout と scroll へ
参加しない。route 種別の判定は `app/App.tsx` が所有し、Toast の置き方だけを切り替える。

- 待機中と一度も取り込みがない状態では表示しない。
- 実行中はアイコン、状態名、確定している場合だけ割合を1行で表示する。総数が 0 または未確定の
  間は割合を出さず、「確認中」または「件数確認中」と読む文言にする。
- 完了と一部失敗は同じ位置で結果表示へ切り替え、8秒間表示したあと自動で閉じる。hover または
  focus で概要を読んでいる間は閉じず、pointer/focus が外れてから残り時間を再開する。一部失敗は
  件数と文言で失敗を示す。
- 全体失敗は利用者が閉じるか設定詳細へ進むまで表示を残す。失敗時だけインジケーター内に小さな
  icon button を出し、accessible name は「取り込み失敗の通知を閉じる」とする。Tab 順では
  設定詳細へ進む trigger の直後に置き、Enter または Space で閉じる。失敗状態は文言と icon を
  組み合わせ、色だけに依存しない。
- 失敗の強調は、`bg-elevated` の面では `text-fg` と icon、`border-border-strong`、必要なら
  `danger-soft` の wash で示す。`text-danger` は `bg` 上の補助文または badge に限る。実装で
  `danger` を `elevated` や `surface` 上の文字として使う場合は、同じ PR で `tokens.test.ts` の
  contrast pair に追加する。
- 表面は `bg-elevated`、`border-border-strong`、`shadow-elevated` を使う。角丸は既存の
  compact control に合わせて `rounded-md` までに留め、ページ section のような大きな card にはしない。
- 360px でも右端と下端から安全領域を残す。既存 Toast と同時に出る場合は Toast より上へ逃がし、
  どちらの文言も読める位置関係にする。再生画面では閉じる × と重ならない。

## Summary Popover

概要は hover と keyboard focus で同じ内容を開く。hover できない端末では、activation が
設定詳細への導線になるため、概要の存在を操作条件にしない。

- 表示内容は、状態、progress bar、処理済み件数、総件数、失敗件数だけにする。開始時刻、
  完了時刻、失敗理由、再試行は設定 section に寄せる。
- progress bar は総数が 1 以上に確定している実行中、完了、一部失敗で determinate とする。
  総数未確定、開始要求中、取得の一時失敗では indeterminate 表示にし、0% や 100% を連想させる
  数値を出さない。`done(total=0, completed=0)` は完了状態として文言と時刻を表示するが、概要に
  determinate bar や 100% は出さない。
- Popover は画面中央側へ開くことを基本にし、右下では左上へ展開する。Radix の
  collision handling で画面外へはみ出さない。360pxの再生画面ではplayer下の空き領域へ開き、本文を
  長時間覆わない横幅へ収める。件数は折り返さず表のように読める。
- pointer を trigger から popover へ移しても閉じない。Escape、focus 移動、pointer leave で閉じる。
- trigger の click、tap、Enter、Space は popover toggle ではなく `/settings#scan-status` への移動に使う。

## Settings Scan Status

「取り込み状況」は設定画面の運用情報として、ページタイトルより弱く、メディアフォルダ設定より
先に読める位置へ置く。既存 section の余白、区切り、文字サイズを継承し、独立した dashboard
のように大きくしない。

上から次の順に並べる。

1. 見出し「取り込み状況」と、現在の状態を示す短い status badge。
2. 状態説明と progress bar。未実行と `done(total=0, completed=0)` では空または 100% の bar を
   出さず、それぞれ「まだ取り込んでいません」「対象はありませんでした」の文言で区別する。
3. 件数の行。処理済み、総件数、失敗件数を tabular numbers で揃える。総数未確定の場合は
   「確認中」とし、0 件とは区別する。
4. 時刻の行。開始時刻、完了時刻を表示し、未確定の値は空欄ではなく「未完了」などの文言にする。
5. 全体失敗だけ、折り返し可能な理由と主操作の「再試行」を表示する。長い理由は section 幅内で
   折り返し、操作を押し出さない。

取得の一時失敗中は、最後に取得できた scan があればそれを残したまま補足として「最新状態を再確認中」
を出す。最後の scan が無い場合は、空の progress ではなく取得失敗の文言と自動再試行中であることを
表示する。

## States

| State              | Floating indicator                                                                       | Summary popover                        | Settings section                              |
| ------------------ | ---------------------------------------------------------------------------------------- | -------------------------------------- | --------------------------------------------- |
| 未実行             | 表示しない                                                                               | なし                                   | 「まだ取り込んでいません」。progress bar なし |
| 開始要求中         | 「開始中」                                                                               | indeterminate、件数確認中              | 開始要求中として自動更新中                    |
| 総数未確定の実行中 | 「取り込み中」                                                                           | indeterminate、処理済みと失敗件数      | 実行中、総件数は確認中                        |
| 総数確定の実行中   | 「取り込み中 NN%」                                                                       | determinate、状態と件数                | 同じ割合と件数、開始時刻                      |
| 0件完了            | 8秒間「完了」                                                                            | bar なし、対象なし                     | 対象なし、開始時刻、完了時刻                  |
| 完了               | 8秒間「完了」                                                                            | determinate、最終件数                  | 完了、開始時刻、完了時刻                      |
| 一部失敗           | 期限中だけ「一部失敗」                                                                   | determinate、失敗件数                  | 一部失敗、最終件数、時刻                      |
| 全体失敗           | 確認まで「失敗」、閉じる button                                                          | indeterminate または最終件数、失敗件数 | 失敗理由と再試行                              |
| 取得一時失敗       | 既知の scan があれば最後の状態を保ち、補足だけ弱く表示。既知の scan が無ければ表示しない | 最新状態を再確認中                     | 最後の状態または自動再試行中                  |

## Responsive Layout

- 360px: 通常画面と再生画面で右下に収まる幅にし、本文、player control、映像情報、詳細 tab、
  モバイルナビゲーション、Toast と重ならない。再生画面のPopoverはplayer下の空き領域へ開き、
  長い文言は2行まで自然に折り返す。設定 section は1列。
- 768px: サイドバーや toolbar と視覚的に競合しない右下に置く。再生画面でも右下とし、プレイヤーの
  右上に重なる閉じる × を避ける。Popover は件数を2列相当で読める幅を取り、主要操作を覆い続けない。
- 1280px: インジケーターは画面端の補助表示として小さく保つ。Popover が大きな card に見えない
  密度を保ち、一覧 grid の主従を崩さない。
- 200% 拡大と長い失敗理由では、インジケーターの label は折り返しを許し、設定 section の理由は
  `break-words` 相当で欄内に収める。

## Accessibility

- インジケーター trigger は button または button 相当の要素とし、accessible name に状態と遷移先を
  含める。例: 「取り込み中 70%。取り込み状況を開く」。
- trigger は Tab で到達でき、focus-visible outline は既存の `link` 色を使う。Enter と Space は
  `/settings#scan-status` へ移動する。
- 概要は hover だけに依存せず、focus で同じ内容を表示する。Escape で閉じる。
- 進捗の通知は `role="status"` を使い、2秒ごとの polling を毎回読み上げない。状態変化、総数確定、
  完了、一部失敗、全体失敗のような意味のある節目を優先する。
- progress bar は accessible name と value を持つ。indeterminate では `aria-valuenow` を付けず、
  文言で総数未確定を伝える。
- 状態は色に加えて文言と lucide icon で区別する。追加する icon は `RefreshCw`、`CheckCircle2`、
  `AlertTriangle`、`XCircle` の既存体系に近い線アイコンを使う。
- 設定 section へ hash 移動したときは、`id="scan-status"` の section を scroll 位置の対象にし、
  その中の見出しへ programmatic focus を移す。見出しは `tabindex="-1"` を持ち、visible focus ring
  は出してよい。実装 PR では click、Enter、Space の後に「取り込み状況」の見出しまたは直後の
  status summary が active element になることを確認する。ブラウザ標準の hash scroll を尊重し、
  余分な page-level scroll owner を増やさない。

## Observable Review Criteria

実装 PR は [quickstart.md](quickstart.md) の mock 応答列を使い、360px、768px、1280px で
通常画面と再生画面の表示、概要表示を撮る。UI 画像は次を含める。

- 総数未確定の実行中: 割合が出ず、indeterminate と文言で待っていることが分かる。
- 総数確定の実行中: 通常画面右下の短い表示、概要、設定 section が同じ割合と件数を示す。
- 再生中: どの幅でも右下の表示と概要が同じ scan の更新を続け、閉じる ×、player control、属性情報を覆わない。
- 0件完了: 完了状態と時刻は分かるが、100% の progress として見えない。
- 完了または一部失敗: 同じ位置で結果へ切り替わり、8秒後に自動で閉じ、一覧や操作より強くなりすぎない。
- 全体失敗: 見落とさない強さがあり、閉じる button、設定 section の理由と再試行がある。
- 初回取得失敗: 既知の scan がない間はフローティング表示を出さず、設定 section は自動再試行中の
  取得失敗として見える。
- Toast 併存: 360px を含めて Toast とインジケーターの文言と操作が重ならない。
- `/settings#scan-status` 直接表示: メディアフォルダ設定より前に「取り込み状況」が見える。

見る観点は次のとおり。

- **視覚的階層**: 右下の表示は補助状態として控えめで、失敗だけが必要な強さを持つ。設定 section は
  メディアフォルダより先に読めるが、ページタイトルより強くない。
- **情報密度**: インジケーター本体は短い状態と数値だけ、概要は判断に必要な件数だけ、詳細は
  時刻・理由・再試行までを引き受ける。
- **余白のリズム**: フローティング表示は本文を押し上げず、画面端と Toast から安全な余白を取る。
  設定 section は既存 section 間隔に揃う。
- **タイポグラフィ**: 状態名、割合、件数、補足の強弱が明確で、数字は tabular numbers で読みやすい。
- **操作の優先順位**: hover/focus の概要確認と click/tap/keyboard の詳細移動が分かれ、失敗時は
  設定 section の再試行が主操作として見える。

操作の確認では、mouse hover、keyboard focus、Escape、click、tap、Enter、Space、失敗通知の
閉じる button、8秒の完了通知期限、画面移動、reload、重複開始、完了直前の navigation を通す。
支援技術の確認では、trigger と閉じる button の accessible name、hash 遷移後の focus、progress bar
の name/value、概要に重複した操作がないこと、polling のたびに読み上げを割り込まないことを
アクセシビリティツリーまたは screen reader で確認する。native screen reader の実音声を確認できない
環境では、PR の残余リスクに明記する。
