# UI Design: 設定画面

**Parent Issue**: #62

**Spec**: [spec.md](spec.md)

**Plan**: [plan.md](plan.md)

## Existing Decisions

見た目と共通操作は
[ライブラリUIの見た目の規則](../../docs/design-docs/library-ui-design-system.md)、
[コンパクトな動画ライブラリUI](../../docs/design-docs/modern-library-ui.md)、
`web/src/index.css`の既存token、`web/src/ui`のButton・Toast・Tooltipに従う。
新しい色、書体、radius、shadow tokenは追加しない。設定画面は既存AppShell内に置き、
Sidebarの「設定」を`/settings`へのNavLinkへ変える。

## Screen

### 設定 `/settings`

画面は、ページ見出し「設定」と1つの設定section「メディアフォルダ」だけを持つ。
sectionをcardで囲まず、見出し、説明、操作、folder rowsを同じcontent columnへ置く。

表示順は次のとおり。

1. `h1`「設定」
2. `h2`「メディアフォルダ」
3. 説明「動画を探すサーバー上のフォルダです。変更後は上部の『ライブラリを更新』から
   取り込みを実行してください。取り込みは自動では始まりません。」
4. 登録済みfolder rows、またはempty state
5. 登録済みfolderがある場合、その後にFolderPlus icon付きprimary action「フォルダを追加」。
   一覧取得失敗時も同じ位置に無効状態で表示する

content columnはwide desktopでも読み取り幅を広げ続けず、左をAppShellのcontent edgeへ揃える。
ページ見出しとsection見出しは既存画面の文字scaleを使い、marketing pageのような大見出しや
装飾面を作らない。

### Folder row

folder 1件を1行として表示し、行間は`border-border`で区切る。行をcardにはしない。

- 左: Folder iconと正規化済み絶対path。pathは省略記号だけにせず複数行へ折り返せる。
- 右: Pencil iconの「フォルダを変更」とTrash2 iconの「フォルダを削除」。どちらも
  IconButtonとTooltipを使い、同じ文言をaccessible nameにする。
- 一覧全体のedit mode、checkbox、保存・キャンセルactionは置かない。
- mutation中は対象行だけに「変更中…」または「削除中…」を表示し、その行のactionsを無効にする。
  他のrowを処理中に見せる表現はしない。

0件では、行領域に「メディアフォルダが設定されていません」と「フォルダを追加すると、
手動で取り込めるようになります。」を表示し、同じprimary actionを置く。illustrationや
空状態専用cardは置かない。空状態には現在値がないため、説明の直後に追加actionを置く。

## Folder Picker

「フォルダを追加」または行の変更actionからmodal folder pickerを開く。追加時はserverが返す
root/drive一覧から始め、変更時は現在のfolderを最初のlocationとして開く。現在のfolderを
列挙できない場合は、その理由と「ルートへ戻る」をpicker内に表示する。

pickerの構成は次の順とする。

1. Dialog title: 追加時「メディアフォルダを追加」、変更時「メディアフォルダを変更」
2. X iconの「閉じる」
3. ArrowUp iconの親directory actionと、折り返し可能な現在path
4. 現在directory直下のdirectory list。fileとsymlinkは表示しない
5. secondary action「キャンセル」
6. primary action: 追加時「このフォルダを追加」、変更時「このフォルダに変更」

directory rowはFolder icon、directory名、ChevronRight iconを持つ全幅buttonで、activateすると
そのdirectoryへ移動する。root/driveは移動の起点として表示するが選択確定できない。空directoryでも
現在位置が有効なら確定できる。変更前と同じpath、登録済みpathとの重複、登録済みpathとの親子重複では
primary actionを無効にして理由を表示する。pathを入力・編集するtext fieldは置かない。

360pxではpickerをviewport全体のdialogとして表示し、headerとfooterを固定、directory listだけを
scrollさせる。768pxと1280pxでは中央modalとし、viewport内に収まる最大高を持たせる。
背景は`bg-overlay`、dialog面は`bg-elevated`、境界とshadowは既存tokenを使う。

追加はprimary actionで直ちに1件をPOSTする。変更はprimary actionの後、同じdialog内の確認stepへ
進む。確認stepは旧pathと新pathを並べ、次を伝える。

> 変更前のフォルダだけにある動画は一覧から外れます。再生位置と視聴済み状態は残ります。
> 取り込みは自動では始まりません。

確認stepのactionsは「戻る」とdanger action「変更する」。戻ると移動済みlocationとfocusを保持する。

## Delete Confirmation

削除actionは確認dialogを開く。対象pathを省略せず示し、次を伝える。

> このフォルダだけにある動画は一覧から外れます。再生位置と視聴済み状態は残ります。
> 取り込みは自動では始まりません。

actionsは「キャンセル」とdanger action「削除する」。初期focusは「キャンセル」に置く。
folder名の再入力や一覧全体の確認は要求しない。

## States And Feedback

| State | Presentation and behaviour |
| --- | --- |
| Initial loading | section見出しを残し、folder rowと同じ高さのSkeletonを表示する。page全体をspinnerへ置き換えない |
| List load failure | row領域に理由と「再試行」を表示する。row領域の後に追加actionを残すが、現在値を確認できないため無効にし、その理由をerror本文で示す |
| Picker loading | header、現在path、footerを固定し、directory listだけをSkeletonへ置き換える |
| Picker load failure | 現在pathを維持し、list領域に理由、「再試行」、「ルートへ戻る」を表示する |
| Invalid candidate | dialogを閉じず、footer直前に具体的な理由を表示する。以前の設定は変更しない |
| Add success | dialogを閉じ、返された1 rowを一覧へ追加し、Toast「追加しました。反映するには取り込みを実行してください」を出す |
| Change success | dialogを閉じ、対象rowだけを応答値へ置き換え、Toast「変更しました。反映するには取り込みを実行してください」を出す |
| Delete success | dialogを閉じ、対象rowだけを除き、Toast「削除しました。取り込みは自動では始まりません」を出す。focusは次のrow、無ければ追加actionへ移す |
| Version conflict | 最新一覧を再取得し、対象row付近に「別の画面で変更されました。内容を確認してやり直してください」を表示する。古い値を再送しない |
| Scan running | 追加・変更・削除actionを無効にし、section冒頭に「取り込み中はメディアフォルダを変更できません」を表示する |
| Scan starts during submit | dialogを維持して同じ説明を表示し、入力済みlocationを失わない |

成功後も自動scanは開始しない。Toastだけに依存せず、一覧の更新結果そのものを成功の恒久的な表示とする。
TopBarの既存scan状態はそのまま使う。folderが0件のときはscanを開始できず、設定画面のempty stateが
必要な次の操作を示す。

## Responsive Behaviour

- **360px**: AppShellは既存drawerを使い、「設定」選択後に閉じる。folder rowはpathを上、actionsを
  下端右へ置き、長い単語にも`overflow-wrap`を適用する。追加actionはrowsの後で全幅にする。
  pickerはfull-screen dialogにする。
- **768px**: 既存rail sidebarを維持する。folder rowはpathとactionsを横並びにし、追加actionは
  rowsの後でcontent幅には広げない。pickerは中央modalにする。
- **1280px**: 既存expanded sidebarを維持する。content columnを無制限に広げず、path比較に必要な幅を
  保つ。余白を埋めるためにrowやbuttonを拡大しない。

どの幅でもpage自体の横scroll、pathとactionの重なり、dialog footerの画面外流出を許さない。

## Keyboard And Assistive Technology

- Sidebarの「設定」は現在位置をNavLinkのactive stateで示し、mobileではactivate後にdrawerを閉じる。
- modalは`dialog` name、focus trap、Escape close、close後のtriggerへのfocus復帰を持つ。背景は操作不能にする。
- directory listはTab順に1か所だけ入り、ArrowUp/ArrowDownとHome/Endでrow focusを移動し、
  EnterまたはArrowRightでfocused directoryを開き、ArrowLeftで親へ戻る。pointer操作と同じ結果にする。
- location変更は現在pathを`aria-live="polite"`で通知する。loading完了、listing error、mutation errorも
  statusまたはalertとして読み上げる。
- icon-only actionsはvisible Tooltipとaccessible nameを持つ。色だけで通常・danger・disabledを区別しない。
- picker、変更確認、削除確認の全actionへTab/Shift+Tabで到達できる。破壊的確認の初期focusを
  danger actionへ置かない。
- `prefers-reduced-motion`では既存base ruleに従い、画面内のすべてのanimationとtransitionを抑える。

実装PRでは、キーボードだけで「設定を開く → folder追加 → directoryを2階層移動 → 追加 →
変更確認を戻る → 削除確認をキャンセル」まで行い、focusの消失と背景への脱出がないことを確認する。
支援技術では見出し順、現在path、directory button名、処理結果、errorが意味の通る順に読まれることを確認する。

## Visual Review Criteria

実装PRは[UI変更のスクリーンショット手順](../../docs/how-to/ui-change-screenshots.md)に従い、最低限
次を添付する。

1. 1280px: 複数folderを持つ設定画面。見出し、説明、rows、row actions、追加の階層を確認する。
2. 768px: 深いdirectoryを表示したpicker。現在path、list、footer actionsが競合しないことを確認する。
3. 360px: 長い日本語pathを持つfolder rowsとfull-screen picker。横scroll、欠落、重なりがないことを確認する。
4. 360pxまたは768px: 変更確認または削除確認。対象path、影響説明、cancel/dangerの優先順位を確認する。

レビューでは次をすべて満たすこと。

- **Visual hierarchy**: 「設定」→「メディアフォルダ」→現在値→対象row actions→追加の順に見え、
  補足文やdanger actionが通常時の追加actionより強く見えない。
- **Information density**: 複数pathを一度に比較でき、1設定項目をhero、nested card、大きなempty
  illustrationで引き伸ばしていない。
- **Spacing rhythm**: AppShellの既存content edgeとrow間隔へ揃い、section間よりrow内の間隔が狭い。
- **Typography**: 既存のpage/section/body/muted textの体系を使い、pathだけが不自然に大きい、または
  小さくならない。
- **Action priority**: 追加は通常時のprimary、変更はsecondary、削除は確認後だけdangerになる。
  bulk saveに見えるactionがない。

次のいずれかがあれば非達成とする: page sectionを浮いたcardにする、path text inputを置く、folder
rowsを一括編集する、scanが自動開始したように見せる、長いpathがactionsを押し出す、disabled理由が
画面上にない、focusがmodal外へ抜ける、folderを変更していないrowが処理中に見える。
