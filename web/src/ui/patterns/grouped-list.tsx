import type { ReactNode, Ref } from "react";

// 見出しつきのまとまりの一覧（区画）。まとまりの見出しの下に、その行を地の面のまま並べる。
// 行に card の面も行の間の線も無く、まとまりの間の隙間が行の間より広いことだけで区切りを
// 見せる。まとまりの間・見出しと行の間・行の上下の余白、見出しの見た目とトップバーの下に
// 貼り付くことはこの区画が持ち、画面は見出しの文言と行の中身を渡すだけにする。規則は
// web/registry/rules/patterns.md の Sections。

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
  /** 見出しの同じ行の後ろに弱い色で添える語（「Oct 10」）。 */
  detail?: ReactNode;
  /** GroupedListItem を並べる。 */
  children: ReactNode;
}

/**
 * GroupedListGroup は見出しと、その行である。見出しはまとまりの行が流れる間トップバーの
 * 下に貼り付き、次のまとまりの見出しに押し出される。`detail` は「·」の後ろに同じ 1 行で
 * 続く（「Today · Oct 10」）。「·」は支援技術に読ませない。
 */
function GroupedListGroup({ heading, detail, children }: GroupedListGroupProps) {
  return (
    <li data-slot="grouped-list-group" className="flex flex-col gap-2">
      <h2 className="sticky top-navbar z-10 flex flex-wrap items-baseline gap-x-1 bg-background py-2">
        <span className="text-sm font-semibold text-foreground">{heading}</span>
        {detail !== undefined && (
          <>
            {" "}
            <span aria-hidden="true" className="text-xs text-muted-foreground">
              ·
            </span>{" "}
            <span className="text-xs text-muted-foreground">{detail}</span>
          </>
        )}
      </h2>
      <ul className="flex flex-col">{children}</ul>
    </li>
  );
}

export interface GroupedListItemProps {
  /** 行の中身。左から右へ並び、縦は中央に揃う。 */
  children: ReactNode;
  ref?: Ref<HTMLLIElement>;
}

/**
 * GroupedListItem はまとまりの 1 行である。地の面の上に上下の余白だけで置き、横の余白は
 * 持たないので、行の最初の要素は見出しと揃う。行の間に隙間も線も無い。
 */
function GroupedListItem({ children, ref }: GroupedListItemProps) {
  return (
    <li
      ref={ref}
      data-slot="grouped-list-item"
      className="flex min-w-0 items-center gap-3 py-2"
    >
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
