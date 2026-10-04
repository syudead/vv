import type { ReactNode } from "react";

// 一覧ページ（骨格）。見出し行 → ツールバー → 本体（グリッドか表）→ 選択バーの順に
// 縦に並べる。外側の余白と領域の間の間隔は骨格が持ち、画面は中身を差し込むだけにする。
// 読み込み中・空・エラーの状態も本体の位置に差し込む。
// 規則は web/registry/rules/patterns.md の List page。

export interface ListPageProps {
  /** PageHeader（題・件数・主操作）。 */
  header: ReactNode;
  /** Toolbar（検索・絞り込み・表示の切り替え）。 */
  toolbar?: ReactNode;
  /** 選択があるときの SelectionBar。本体の下に貼り付く。 */
  selectionBar?: ReactNode;
  /** 本体: CardGrid か DataTable と LoadMoreRow、または状態のブロック。 */
  children: ReactNode;
}

export function ListPage({ header, toolbar, selectionBar, children }: ListPageProps) {
  return (
    <div data-slot="list-page" className="flex min-h-full flex-col gap-3 p-3 sm:p-4">
      {header}
      {toolbar}
      <div data-slot="list-page-body" className="flex flex-1 flex-col gap-3">
        {children}
      </div>
      {selectionBar}
    </div>
  );
}
