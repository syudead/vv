import type { ReactNode, Ref } from "react";

// 管理表ページ（骨格）。見出しの下に帯（タブ・効いている絞り込み・操作）を置き、その下に
// 表を置く。読みやすい幅（max-w-4xl）で中央に寄せ、見出しと帯はトップバーの直下に
// 貼り付ける（行を送っても、見出しの操作・選択バー・タブに届く）。検索・絞り込み・並び順の Toolbar（placement="topBar"）は TopBarPortal で包んで
// toolbar に渡し、共通のトップバーの中央に入れる（ポータルなので、ページの中には描かない）。
// 外側の余白と領域の間隔は骨格が持つ。読み込み中・空・エラーの状態は表の位置に入れる。
// 規則は web/registry/rules/patterns.md の Admin table page。

export interface AdminTablePageProps {
  /** TopBarPortal で包んだ Toolbar（検索・絞り込み・並び順）。 */
  toolbar?: ReactNode;
  /** PageHeader（題・件数・主操作）。選んでいる間は SelectionBar（placement="header"）。 */
  header: ReactNode;
  /** 帯: Tabs の TabsList と効いている絞り込み。 */
  band: ReactNode;
  /** 選択があるときの、下端に浮かせる SelectionBar。見出しの行に置くなら header に入れる。 */
  selectionBar?: ReactNode;
  /** 貼り付く帯（見出しと band）。行が帯の下に隠れないよう、画面が高さを測るのに使う。 */
  bandRef?: Ref<HTMLDivElement>;
  /** DataTable、または状態のブロック。 */
  children: ReactNode;
}

export function AdminTablePage({
  toolbar,
  header,
  band,
  selectionBar,
  bandRef,
  children,
}: AdminTablePageProps) {
  return (
    <div
      data-slot="admin-table-page"
      className="mx-auto flex min-h-full w-full max-w-4xl flex-col gap-3 p-3 sm:p-4"
    >
      {toolbar}
      <div
        ref={bandRef}
        data-slot="admin-table-page-band"
        className="sticky top-navbar z-35 -mx-3 -mt-3 flex flex-col gap-3 bg-background px-3 pt-3 pb-1 sm:-mx-4 sm:-mt-4 sm:px-4 sm:pt-4"
      >
        {header}
        {band}
      </div>
      <div data-slot="admin-table-page-body" className="flex flex-1 flex-col gap-3">
        {children}
      </div>
      {selectionBar}
    </div>
  );
}
