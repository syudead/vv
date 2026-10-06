import type { ReactNode } from "react";

// 一覧ページ（骨格）。見出し行 → ツールバー → 帯（効いている絞り込み）→ 本体（グリッドか
// 表）→ 選択バーの順に縦に並べる。ツールバーを共通のトップバーに置く一覧（ライブラリ・
// フォルダ）は、toolbar に TopBarPortal で包んだ Toolbar（placement="topBar"）を渡す。
// ポータルなので、ページの中には何も描かない。外側の余白と領域の間の間隔は骨格が持ち、画面は中身を差し込むだけにする。
// 読み込み中・空・エラーの状態も本体の位置に差し込む。
// 規則は web/registry/rules/patterns.md の List page。

export interface ListPageProps {
  /** PageHeader（題・件数・主操作）。 */
  header: ReactNode;
  /** Toolbar（検索・絞り込み・表示の切り替え）。トップバーに置くときは TopBarPortal で包む。 */
  toolbar?: ReactNode;
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
  band,
  selectionBar,
  children,
}: ListPageProps) {
  return (
    <div data-slot="list-page" className="flex min-h-full flex-col gap-3 p-3 sm:p-4">
      {header}
      {toolbar}
      {band}
      <div data-slot="list-page-body" className="flex flex-1 flex-col gap-3">
        {children}
      </div>
      {selectionBar}
    </div>
  );
}
