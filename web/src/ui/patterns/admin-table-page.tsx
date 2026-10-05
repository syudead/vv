import type { ReactNode } from "react";

// 管理表ページ（骨格）。見出しの下に帯（タブ・効いている絞り込み・操作）を置き、その下に
// 表を置く。検索・絞り込み・並び順の Toolbar（placement="topBar"）は TopBarPortal で包んで
// toolbar に渡し、共通のトップバーの中央に入れる（ポータルなので、ページの中には描かない）。
// 外側の余白と領域の間隔は骨格が持つ。読み込み中・空・エラーの状態は表の位置に入れる。
// 規則は web/registry/rules/patterns.md の Admin table page。

export interface AdminTablePageProps {
  /** TopBarPortal で包んだ Toolbar（検索・絞り込み・並び順）。 */
  toolbar?: ReactNode;
  /** PageHeader（題・件数・主操作）。 */
  header: ReactNode;
  /** 帯: Tabs の TabsList と効いている絞り込み。 */
  band: ReactNode;
  /** 選択があるときの SelectionBar。 */
  selectionBar?: ReactNode;
  /** DataTable、または状態のブロック。 */
  children: ReactNode;
}

export function AdminTablePage({
  toolbar,
  header,
  band,
  selectionBar,
  children,
}: AdminTablePageProps) {
  return (
    <div
      data-slot="admin-table-page"
      className="flex min-h-full flex-col gap-3 p-3 sm:p-4"
    >
      {toolbar}
      {header}
      <div data-slot="admin-table-page-band" className="flex flex-col gap-3">
        {band}
      </div>
      <div data-slot="admin-table-page-body" className="flex flex-1 flex-col gap-3">
        {children}
      </div>
      {selectionBar}
    </div>
  );
}
