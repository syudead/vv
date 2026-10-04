import type { ReactNode } from "react";

// 管理表ページ（骨格）。見出しの下に帯（タブ・検索・操作）を置き、その下に表を置く。
// 外側の余白と領域の間隔は骨格が持つ。読み込み中・空・エラーの状態は表の位置に入れる。
// 規則は web/registry/rules/patterns.md の Admin table page。

export interface AdminTablePageProps {
  /** PageHeader（題・件数・主操作）。 */
  header: ReactNode;
  /** 帯: Tabs の TabsList と Toolbar（検索・一括の操作）。 */
  band: ReactNode;
  /** 選択があるときの SelectionBar。 */
  selectionBar?: ReactNode;
  /** DataTable、または状態のブロック。 */
  children: ReactNode;
}

export function AdminTablePage({
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
