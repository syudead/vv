import type { ReactNode } from "react";

// 見出しつきのまとまりの一覧（区画）。まとまりの見出しと、その行を 1 枚の card の面に
// 線で区切って並べる。まとまりの間・見出しと面の間・行の間の間隔、見出しの見た目と
// トップバーの下に貼り付くこと、面の見た目はこの区画が持ち、画面は見出しの文言と行の中身を
// 渡すだけにする。規則は web/registry/rules/patterns.md の Sections。

export interface GroupedListProps {
  /** 一覧の名前（「Watch history」）。支援技術に読まれる。 */
  label: string;
  /** GroupedListGroup を並べる。 */
  children: ReactNode;
}

function GroupedListRoot({ label, children }: GroupedListProps) {
  return (
    <ul data-slot="grouped-list" aria-label={label} className="flex flex-col gap-6">
      {children}
    </ul>
  );
}

export interface GroupedListGroupProps {
  /** まとまりの見出し（「Today」）。h2 になる。 */
  heading: ReactNode;
  /** GroupedListItem を並べる。 */
  children: ReactNode;
}

/**
 * GroupedListGroup は見出しと、その行の面である。見出しはまとまりの行が流れる間トップバーの
 * 下に貼り付き、次のまとまりの見出しに押し出される。
 */
function GroupedListGroup({ heading, children }: GroupedListGroupProps) {
  return (
    <li data-slot="grouped-list-group" className="flex flex-col gap-2">
      <h2 className="sticky top-navbar z-10 bg-background py-2 text-sm font-semibold text-foreground">
        {heading}
      </h2>
      <ul className="flex flex-col divide-y divide-border rounded-md border border-border bg-card text-card-foreground">
        {children}
      </ul>
    </li>
  );
}

export interface GroupedListItemProps {
  /** 行の中身。左から右へ並び、縦は中央に揃う。 */
  children: ReactNode;
}

/** GroupedListItem はまとまりの 1 行である。行の間に隙間は無く、線で区切る。 */
function GroupedListItem({ children }: GroupedListItemProps) {
  return (
    <li data-slot="grouped-list-item" className="flex min-w-0 items-center gap-3 p-2">
      {children}
    </li>
  );
}

/** GroupedList は一覧の全体で、`GroupedList.Group` と `GroupedList.Item` を持つ。 */
const GroupedList = Object.assign(GroupedListRoot, {
  Group: GroupedListGroup,
  Item: GroupedListItem,
});

export { GroupedList, GroupedListGroup, GroupedListItem };
