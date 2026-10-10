import type { ReactNode } from "react";

// 一覧ページ（骨格）。見出し行 → ツールバー → 帯（効いている絞り込み）→ 本体（グリッドか
// 表）→ 選択バーの順に縦に並べる。ツールバーを共通のトップバーに置く一覧（ライブラリ・
// フォルダ）は、toolbar に TopBarPortal で包んだ Toolbar（placement="topBar"）を渡す。
// ポータルなので、ページの中には何も描かない。外側の余白と領域の間の間隔は骨格が持ち、画面は中身を差し込むだけにする。
// 読み込み中・空・エラーの状態も本体の位置に差し込む。
// toolbarRow="header" は lg から見出しとツールバーを 1 行に並べる。aside は lg から本体の
// 右の list-aside の幅の欄で、トップバーの下に貼り付く。lg 未満では帯と本体の間に幅いっぱいに
// 置き、狭い形は中の区画が決める。
// 規則は web/registry/rules/patterns.md の List page。

export interface ListPageProps {
  /** PageHeader（題・件数・主操作）。 */
  header: ReactNode;
  /** Toolbar（検索・絞り込み・表示の切り替え）。トップバーに置くときは TopBarPortal で包む。 */
  toolbar?: ReactNode;
  /**
   * 見出しとツールバーの並べ方。stacked（既定）は縦に重ねる。header は lg から見出しを
   * 自分の幅で、ツールバーを残りの幅で 1 行に置き、lg 未満では重ねる。
   */
  toolbarRow?: "stacked" | "header";
  /** 見出しの下の帯（効いている絞り込みのタグ）。無ければ省く。 */
  band?: ReactNode;
  /**
   * 本体の横の欄（日付へ移る一覧など、本体を絞る道具）。lg から本体の右に貼り付き、lg 未満
   * では帯と本体の間に置く。無ければ本体が幅いっぱいになる。
   */
  aside?: ReactNode;
  /** 選択があるときの SelectionBar。本体の下に貼り付く。 */
  selectionBar?: ReactNode;
  /** 本体: CardGrid か DataTable と LoadMoreRow、または状態のブロック。 */
  children: ReactNode;
}

export function ListPage({
  header,
  toolbar,
  toolbarRow = "stacked",
  band,
  aside,
  selectionBar,
  children,
}: ListPageProps) {
  const body = (
    <div data-slot="list-page-body" className="flex min-w-0 flex-1 flex-col gap-3">
      {children}
    </div>
  );
  return (
    <div data-slot="list-page" className="flex min-h-full flex-col gap-3 p-3 sm:p-4">
      {toolbarRow === "header" ? (
        <div
          data-slot="list-page-header-row"
          className="flex flex-col gap-3 lg:flex-row lg:items-center lg:gap-4"
        >
          <div className="min-w-0 lg:shrink-0">{header}</div>
          {toolbar !== undefined && <div className="min-w-0 lg:flex-1">{toolbar}</div>}
        </div>
      ) : (
        <>
          {header}
          {toolbar}
        </>
      )}
      {band}
      {aside !== undefined ? (
        <div className="flex flex-1 flex-col gap-3 lg:grid lg:grid-cols-list-aside lg:items-start lg:gap-6">
          <aside
            data-slot="list-page-aside"
            className="min-w-0 lg:sticky lg:top-navbar lg:order-last lg:max-h-list-aside-max lg:overflow-y-auto"
          >
            {aside}
          </aside>
          {body}
        </div>
      ) : (
        body
      )}
      {selectionBar}
    </div>
  );
}
