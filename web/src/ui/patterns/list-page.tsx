import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// 一覧ページ（骨格）。見出しの行 → ツールバー → 帯（効いている絞り込み）→ 本体（グリッドか
// 表）→ 選択バーの順に縦に並べる。外側の余白と領域の間の間隔は骨格が持ち、画面は中身を
// 差し込むだけにする。読み込み中・空・エラーの状態も本体の位置に差し込む。
//
// toolbarPlacement="topBar" は動画を見て回る一覧（ライブラリ・フォルダ）の形で、ツールバーを
// ページの中に置かず、画面が shell の TopBarPortal で共通のトップバーの中央へ入れる（toolbar の
// 枠はそのための置き場で、ポータルなのでページの中には何も描かない）。見出しの行は
// PageHeader の variant="list"、末尾には選択バーに隠れない余白を取る。
// 規則は web/registry/rules/patterns.md の List page。

export interface ListPageProps {
  /** PageHeader（topBar では variant="list"）か、フォルダ画面の見出しとパンくずの帯。 */
  header: ReactNode;
  /** Toolbar。topBar では Toolbar の placement="topBar" を TopBarPortal で包んだもの。 */
  toolbar?: ReactNode;
  /** ツールバーの置き場。既定は page。 */
  toolbarPlacement?: "page" | "topBar";
  /** 見出しの下の帯（効いている絞り込みのタグ）。無ければ省く。 */
  band?: ReactNode;
  /** 選択があるときの SelectionBar。本体の下に貼り付く。 */
  selectionBar?: ReactNode;
  /** 本体: CardGrid か DataTable と LoadMoreRow、または状態のブロック。 */
  children: ReactNode;
}

export function ListPage({
  header,
  toolbar,
  toolbarPlacement = "page",
  band,
  selectionBar,
  children,
}: ListPageProps) {
  const topBar = toolbarPlacement === "topBar";
  return (
    <div
      data-slot="list-page"
      data-toolbar-placement={toolbarPlacement}
      className={cn(
        "flex flex-col",
        topBar
          ? "w-full gap-4 px-3 pt-4 pb-selection-bar-clearance sm:px-4"
          : "min-h-full gap-3 p-3 sm:p-4",
      )}
    >
      {topBar && toolbar}
      {header}
      {!topBar && toolbar}
      {band}
      <div data-slot="list-page-body" className="flex flex-1 flex-col gap-3">
        {children}
      </div>
      {selectionBar}
    </div>
  );
}
