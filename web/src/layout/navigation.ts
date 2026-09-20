import type { IconName } from "./icons";

/**
 * ナビゲーションの静的な表である（data-model.md 2.）。
 *
 * **この表が「機能する / 表示のみ」の唯一の真実**であり、描画側は kind で
 * 分岐するだけにする。原案にあって裏側の無い入口（コレクション・タグ・
 * お気に入り・未整理・画像タブ）は、<button disabled> でも aria-disabled でも
 * なく **対話要素にしない**形で置く（R-503 / FR-005）。
 *
 * 機能が付くときは行を書き換える（plan.md の G7）。行を live にするという
 * ことは利用者が行える操作が増えるということなので、そのときは FR-013 の
 * 見直しも要る。
 */

/** NavSection は項目が置かれる区画である。並び順は表の順序そのものである。 */
export type NavSection = "library";

interface NavItemBase {
  /** 表の中で一意。テストが指す名前である */
  id: string;
  /** 画面に出る文言。原案のものをそのまま使う（spec の Assumptions） */
  label: string;
  /** layout/icons.tsx の 1 つ */
  icon: IconName;
  section: NavSection;
}

/** LiveNavItem は機能する項目である。行き先を必ず持つ。 */
export interface LiveNavItem extends NavItemBase {
  kind: "live";
  to: string;
}

/**
 * InertNavItem は表示のみの項目である。
 *
 * to を `never` にしてあるので、行き先を書くと**型で落ちる**。表示のみの項目に
 * 行き先が付く＝押せてしまう、という取り違えを機械で塞ぐための宣言である
 * （data-model.md 2. の不変条件）。
 */
export interface InertNavItem extends NavItemBase {
  kind: "inert";
  to?: never;
}

export type NavItem = LiveNavItem | InertNavItem;

/**
 * navItems は data-model.md 2.「値」の 14 行をそのまま持つ。
 *
 * live は all-videos と tab-videos の 2 つだけである。**この 2 つ以外が live に
 * なったら、それは利用者が行える操作が増えたということ**で、FR-013 の違反で
 * ある。layout/placeholders.test.tsx がこの数を確かめる。
 */
export const navItems: readonly NavItem[] = [
  {
    id: "all-videos",
    label: "ライブラリ",
    icon: "film",
    kind: "live",
    to: "/",
    section: "library",
  },
  {
    id: "recent",
    label: "最近追加",
    icon: "clock",
    kind: "inert",
    section: "library",
  },
  {
    id: "in-progress",
    label: "視聴途中",
    icon: "play",
    kind: "inert",
    section: "library",
  },
];

/** itemsIn は 1 つの区画の項目を、表の順序のまま返す。 */
export function itemsIn(section: NavSection): readonly NavItem[] {
  return navItems.filter((item) => item.section === section);
}
