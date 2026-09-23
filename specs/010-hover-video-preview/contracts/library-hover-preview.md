# Contract: Library Hover Preview

## Eligibility

preview は次のすべてを満たすときだけ開始できる。

- grid view の `VideoCard` が mounted している。
- `previewState === "done"` かつ `previewUrl` がある。
- `pointerenter` を発生させた `event.pointerType === "mouse"`。
- selection checkbox の直接操作中ではない。

`playable`、`thumbnailUrl`、primary-input media query は eligibility に使わない。keyboard focus と touch/pen contact は開始条件にしない。

## Visual States

| State | Thumbnail surface |
| --- | --- |
| default / pending / failed | current thumbnail または placeholder |
| delay waiting | default surface のまま。layout と overlay は変えない |
| previewing | 同じ aspect-ratio surface 内で muted preview を置換表示 |
| playback rejected / media error | video を除去し default surface へ即時復帰 |

preview は card dimensions、metadata 行、selection checkbox、progress bar、watched/unplayable indicators、focus ring を移動・非表示にしない。新しい badge、説明文、loading spinner、controls、timeline は追加しない。

## Playback And Cleanup

- source は API が返す `previewUrl` のみ。`streamUrl`/`transcodeUrl` fallback を持たない。
- `<video muted playsInline>` とし、audio controls と progress persistence を持たない。
- `LibraryPage` は active card ID と reset epoch だけを所有する。別 card が active になったとき、または filter/sort/page/list追加、grid/list 切替、viewport resize で epoch が変わったとき、残存 card を含む以前の preview を停止する。
- pointer leave、coordinator の active/reset change、click navigation、unmount、`play()` rejection、media error で timer を cancel し、pause、source/resource 解放、default surface 復帰を行う。
- 再度 eligible hover した場合は新しい lifecycle として開始できる。

## Existing Interaction Priority

- click/Enter navigation、checkbox selection、keyboard focus、touch scroll/tap を変更しない。
- selection mode でも checkbox hit target と card feedback を preview より優先する。
- `prefers-reduced-motion` では transition を抑制するが、利用者が mouse hover したときの video 内容自体は隠さない。design stage で最終 motion rule を確定する。

## Quality Evidence

- **視覚的階層**: title、checkbox、progress、status indicator が preview より上位の既存位置と contrast を保つ。
- **情報密度**: 新規 text/badge/control を増やさず、一覧の同時表示 card 数を変えない。
- **余白**: card、grid gap、thumbnail aspect ratio、metadata padding に layout shift がない。
- **タイポグラフィ**: font family、size、weight、line clamp を変更しない。
- **操作優先順位**: selection/navigation/focus/touch が hover playback より優先される。

実装 PR は 360px、768px、1280px の before/preview/fallback screenshots、reduced-motion、keyboard、touch、mouse、selection の結果を添付する。unit tests は eligibility、delay/cancel、cleanup、error recovery、source URL、progress API 非呼び出しを証明し、network evidence は preview endpoint 以外の media endpoint が呼ばれないことを示す。
